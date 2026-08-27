package auth

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/Rengemaru/equipment-management/internal/httpx"
)

// adminUserResponse は admin 向けの利用者情報。
//
// 一覧に出す情報は userResponse より多い。誰が卒業済みか、
// 連絡先が入っているかを admin は見る必要がある。
type adminUserResponse struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	LoginID            string `json:"login_id"`
	Email              string `json:"email"`
	Role               Role   `json:"role"`
	IsActive           bool   `json:"is_active"`
	MustChangePassword bool   `json:"must_change_password"`
	CreatedAt          string `json:"created_at"`
}

func newAdminUserResponse(u *User) adminUserResponse {
	return adminUserResponse{
		ID:                 u.ID,
		Name:               u.Name,
		LoginID:            u.LoginID,
		Email:              u.Email,
		Role:               u.Role,
		IsActive:           u.IsActive,
		MustChangePassword: u.MustChangePassword,
		CreatedAt:          u.CreatedAt,
	}
}

// createUserRequest はユーザー作成の入力。
//
// パスワードは受け取らない。admin が考えた文字列を初期パスワードにすると、
// 全員に同じものが配られる。
type createUserRequest struct {
	Name    string `json:"name"`
	LoginID string `json:"login_id"`
	Email   string `json:"email"`
	Role    Role   `json:"role"`
}

// userWithPasswordResponse は初期パスワードを1度だけ返す形。
type userWithPasswordResponse struct {
	User adminUserResponse `json:"user"`

	// InitialPassword はこの応答でしか手に入らない。
	// DBにはハッシュしか無く、再表示はできない。
	InitialPassword string `json:"initial_password"`
}

// registerUserRoutes は admin 専用の経路を登録する。
func (h *Handler) registerUserRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/users", h.RequireAdmin(http.HandlerFunc(h.handleListUsers)))
	mux.Handle("POST /api/users", h.RequireAdmin(http.HandlerFunc(h.handleCreateUser)))

	// 動詞を経路に出す。PATCH で is_active を送る形にすると、
	// 「何をする操作か」がリクエスト本文を読まないと分からなくなる。
	mux.Handle("POST /api/users/{id}/activate",
		h.RequireAdmin(http.HandlerFunc(h.handleSetActive(true))))
	mux.Handle("POST /api/users/{id}/deactivate",
		h.RequireAdmin(http.HandlerFunc(h.handleSetActive(false))))
	mux.Handle("POST /api/users/{id}/reset-password",
		h.RequireAdmin(http.HandlerFunc(h.handleResetPassword)))
}

func (h *Handler) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.store.List(r.Context())
	if err != nil {
		log.Printf("users: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
		return
	}

	// 0件でも null ではなく [] を返す。フロント側で
	// 「null かもしれない」の分岐を書かせない。
	list := make([]adminUserResponse, 0, len(users))
	for _, u := range users {
		list = append(list, newAdminUserResponse(u))
	}

	httpx.JSON(w, http.StatusOK, map[string]any{"users": list})
}

func (h *Handler) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.Role == "" {
		// 省略時は member。admin を作るのは明示した時だけにする。
		req.Role = RoleMember
	}

	password, err := GeneratePassword()
	if err != nil {
		log.Printf("users: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
		return
	}

	user, err := h.store.Create(r.Context(), NewUser{
		Name:     req.Name,
		LoginID:  req.LoginID,
		Email:    req.Email,
		Role:     req.Role,
		Password: password,
		// 発行した文字列は口頭やチャットで渡る。そのまま使い続けさせない。
		MustChangePassword: true,
	})
	if err != nil {
		writeUserError(w, err)
		return
	}

	httpx.JSON(w, http.StatusCreated, userWithPasswordResponse{
		User:            newAdminUserResponse(user),
		InitialPassword: password,
	})
}

// handleSetActive は有効・無効を切り替えるハンドラを返す。
func (h *Handler) handleSetActive(active bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := userIDFromPath(w, r)
		if !ok {
			return
		}

		if err := h.store.SetActive(r.Context(), id, active); err != nil {
			writeUserError(w, err)
			return
		}

		if !active {
			// セッションを切る。Lookup 側でも is_active を見ているため
			// 入れはしないが、行を残す理由がない。
			if err := h.sessions.DeleteByUser(r.Context(), id); err != nil {
				log.Printf("users: %v", err)
			}
		}

		user, err := h.store.ByID(r.Context(), id)
		if err != nil {
			writeUserError(w, err)
			return
		}

		httpx.JSON(w, http.StatusOK, map[string]any{"user": newAdminUserResponse(user)})
	}
}

func (h *Handler) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	id, ok := userIDFromPath(w, r)
	if !ok {
		return
	}

	password, err := h.store.ResetPassword(r.Context(), id)
	if err != nil {
		writeUserError(w, err)
		return
	}

	// 再発行の理由は「入れなくなった」だけでなく「他人に知られた」もある。
	// 古いセッションを残すと、知られたままの状態が続く。
	if err := h.sessions.DeleteByUser(r.Context(), id); err != nil {
		log.Printf("users: %v", err)
	}

	user, err := h.store.ByID(r.Context(), id)
	if err != nil {
		writeUserError(w, err)
		return
	}

	httpx.JSON(w, http.StatusOK, userWithPasswordResponse{
		User:            newAdminUserResponse(user),
		InitialPassword: password,
	})
}

// userIDFromPath は経路から利用者IDを取り出す。
func userIDFromPath(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "利用者IDが不正です")
		return 0, false
	}
	return id, true
}

// writeUserError は Store のエラーを応答に変える。
//
// 1箇所にまとめる。ハンドラごとに書くと、いつか内部エラーの詳細を
// そのまま返す経路ができる。
func writeUserError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "その利用者は見つかりません")

	case errors.Is(err, ErrDuplicateLoginID):
		httpx.WriteError(w, http.StatusConflict, ErrDuplicateLoginID.Error())

	case errors.Is(err, ErrDuplicateEmail):
		httpx.WriteError(w, http.StatusConflict, ErrDuplicateEmail.Error())

	case errors.Is(err, ErrLastAdmin):
		httpx.WriteError(w, http.StatusConflict, ErrLastAdmin.Error())

	default:
		// 入力の誤りは、そのまま見せてよい文言で返している。
		// それ以外は詳細を伏せる。
		var validationErr *validationError
		if errors.As(err, &validationErr) {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		log.Printf("users: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
	}
}
