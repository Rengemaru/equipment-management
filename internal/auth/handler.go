package auth

import (
	"errors"
	"log"
	"net/http"

	"github.com/Rengemaru/equipment-management/internal/httpx"
)

// Handler は認証まわりの HTTP ハンドラ。
type Handler struct {
	store    *Store
	sessions *SessionStore
	throttle *Throttle

	// cookieSecure は COOKIE_SECURE の値。
	// Cookie を設定する箇所と消す箇所で必ず揃える必要があるため、
	// 引数で持ち回さず Handler に持たせる。
	cookieSecure bool
}

// NewHandler は Handler を作る。
func NewHandler(store *Store, sessions *SessionStore, throttle *Throttle, cookieSecure bool) *Handler {
	return &Handler{
		store:        store,
		sessions:     sessions,
		throttle:     throttle,
		cookieSecure: cookieSecure,
	}
}

// Register は担当するルートを mux に登録する。
//
// 経路の文字列をハンドラの隣に置く。main に並べると、
// どのハンドラがどのURLかを2箇所見ないと分からなくなる。
//
// 内部APIは /api/ の下に置く。M4 で作る公開API（トークン認証・個人情報なし）は
// url-design.md により /api/v1/ に置くため、素の /api/ とは別の木にする。
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/login", h.handleLogin)
	mux.HandleFunc("POST /api/logout", h.handleLogout)

	// 自分自身の情報。フロントが起動時にログイン状態を復元するために使う。
	mux.Handle("GET /api/me", h.RequireLogin(http.HandlerFunc(h.handleMe)))
}

// loginRequest はログインの入力。
type loginRequest struct {
	LoginID  string `json:"login_id"`
	Password string `json:"password"`

	// Next は認証後に戻る先。/login?next=/i/0042 の値をそのまま渡す。
	// 検証はサーバ側で行い、安全な値を redirect_to として返す。
	Next string `json:"next"`
}

// userResponse は利用者を返す形。
//
// User をそのまま返さない。列を足した時に、意図しない値が
// APIに現れることになる。
type userResponse struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	LoginID            string `json:"login_id"`
	Role               Role   `json:"role"`
	MustChangePassword bool   `json:"must_change_password"`
}

func newUserResponse(u *User) userResponse {
	return userResponse{
		ID:                 u.ID,
		Name:               u.Name,
		LoginID:            u.LoginID,
		Role:               u.Role,
		MustChangePassword: u.MustChangePassword,
	}
}

// loginResponse はログイン成功時の応答。
type loginResponse struct {
	User userResponse `json:"user"`

	// RedirectTo は認証後に進む先。検証済みで、必ず自サイト内のパスになる。
	//
	// フロント側で next を解釈させず、ここで返した値へ進ませる。
	// 判断を1箇所に集めておかないと、画面が増えるたびに検証の抜けが生まれる。
	// omitempty を付けない。値が無い時に「前の画面に留まる」といった
	// 独自の解釈をさせず、常に行き先を示す。
	RedirectTo string `json:"redirect_to"`
}

func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 直近の失敗回数に応じて待たせる。ロックはしない。
	// 締め出された人はその場で借用を記録できなくなり、記録漏れに直結する。
	//
	// 数えられなくても認証は続ける。総当たり対策が効かないことより、
	// ログインできないことの方が実害が大きい。
	failures, err := h.throttle.RecentFailures(r.Context(), req.LoginID)
	if err != nil {
		log.Printf("login: %v", err)
	}
	h.throttle.Wait(r.Context(), failures)

	user, err := h.store.Authenticate(r.Context(), req.LoginID, req.Password)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			if err := h.throttle.RecordFailure(r.Context(), req.LoginID); err != nil {
				log.Printf("login: %v", err)
			}

			// 401 と文言はここ1箇所だけ。理由ごとに分岐を作ると、
			// いつか片方だけ文言が変わって、存在するIDが分かるようになる。
			// 待たされていることも伝えない。何回で待たされるかが分かる。
			httpx.WriteError(w, http.StatusUnauthorized, "ログインIDまたはパスワードが違います")
			return
		}

		// DBに触れない等。利用者には詳細を見せず、ログに残す。
		log.Printf("login: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
		return
	}

	// 打ち間違えた後に入れた人を、次のログインで待たせない。
	if err := h.throttle.Clear(r.Context(), req.LoginID); err != nil {
		log.Printf("login: %v", err)
	}

	token, expiresAt, err := h.sessions.Create(r.Context(), user.ID)
	if err != nil {
		log.Printf("login: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
		return
	}

	setSessionCookie(w, token, expiresAt, h.cookieSecure)

	// 初期パスワードのままなら、どこへ戻る予定でもパスワード変更へ送る。
	// ここで通すと、変更しないまま使い続けられる。
	redirectTo := httpx.SafeRedirectPath(req.Next)
	if user.MustChangePassword {
		redirectTo = passwordChangePath
	}

	httpx.JSON(w, http.StatusOK, loginResponse{
		User:       newUserResponse(user),
		RedirectTo: redirectTo,
	})
}

// passwordChangePath は初期パスワードのままの利用者を送る先。
// url-design.md の画面一覧に合わせる。
const passwordChangePath = "/password"

// handleLogout はセッションを消す。
//
// ログイン中でなくても 204 を返す。既にログアウトしているなら目的は達しており、
// エラーにするとフロント側に「失敗したのか元々ログインしていないのか」の
// 分岐を作らせることになる。
func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	if err := h.sessions.Delete(r.Context(), tokenFromRequest(r)); err != nil {
		log.Printf("logout: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
		return
	}

	// DB から消せてもブラウザに残っていると、次のリクエストで
	// 無効なCookieを送り続けることになる。
	clearSessionCookie(w, h.cookieSecure)

	w.WriteHeader(http.StatusNoContent)
}

// handleMe はログイン中の利用者を返す。RequireLogin を通っている前提。
func (h *Handler) handleMe(w http.ResponseWriter, r *http.Request) {
	user, ok := UserFrom(r.Context())
	if !ok {
		// ここに来るのは経路の組み立てを誤った時だけ。
		log.Print("me: RequireLogin を通っていない")
		httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
		return
	}

	httpx.JSON(w, http.StatusOK, loginResponse{
		User: newUserResponse(user),
		// 既にログイン済みの利用者に行き先を指示する理由はない。
		// 画面はフロントの経路がそのまま担う。
		RedirectTo: httpx.DefaultRedirect,
	})
}
