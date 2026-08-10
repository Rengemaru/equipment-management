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

	// cookieSecure は COOKIE_SECURE の値。
	// Cookie を設定する箇所と消す箇所で必ず揃える必要があるため、
	// 引数で持ち回さず Handler に持たせる。
	cookieSecure bool
}

// NewHandler は Handler を作る。
func NewHandler(store *Store, sessions *SessionStore, cookieSecure bool) *Handler {
	return &Handler{store: store, sessions: sessions, cookieSecure: cookieSecure}
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
}

func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	user, err := h.store.Authenticate(r.Context(), req.LoginID, req.Password)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			// 401 と文言はここ1箇所だけ。理由ごとに分岐を作ると、
			// いつか片方だけ文言が変わって、存在するIDが分かるようになる。
			httpx.WriteError(w, http.StatusUnauthorized, "ログインIDまたはパスワードが違います")
			return
		}

		// DBに触れない等。利用者には詳細を見せず、ログに残す。
		log.Printf("login: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
		return
	}

	token, expiresAt, err := h.sessions.Create(r.Context(), user.ID)
	if err != nil {
		log.Printf("login: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
		return
	}

	setSessionCookie(w, token, expiresAt, h.cookieSecure)

	httpx.JSON(w, http.StatusOK, loginResponse{User: newUserResponse(user)})
}

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

	httpx.JSON(w, http.StatusOK, loginResponse{User: newUserResponse(user)})
}
