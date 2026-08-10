package auth

import (
	"errors"
	"log"
	"net/http"

	"github.com/Rengemaru/equipment-management/internal/httpx"
)

// Handler は認証まわりの HTTP ハンドラ。
type Handler struct {
	store *Store
}

// NewHandler は Handler を作る。
func NewHandler(store *Store) *Handler {
	return &Handler{store: store}
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

	httpx.JSON(w, http.StatusOK, loginResponse{User: newUserResponse(user)})
}
