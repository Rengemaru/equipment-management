package auth

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/Rengemaru/equipment-management/internal/httpx"
)

// CodePasswordChangeRequired は初期パスワードのままであることを示す識別子。
//
// フロントはこれを見て /password へ送る。文言で分岐させると、
// 日本語を直した瞬間に分岐が壊れる。
const CodePasswordChangeRequired = "password_change_required"

// ErrSamePassword は新しいパスワードが今と同じこと。
var ErrSamePassword = errors.New("今と同じパスワードは使えない")

// SetPassword はパスワードを差し替え、must_change_password を下ろす。
func (s *Store) SetPassword(ctx context.Context, userID int64, newPassword string) error {
	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}

	const q = `
UPDATE users
SET password_hash = ?, must_change_password = 0, updated_at = datetime('now')
WHERE id = ?`

	res, err := s.sqldb.ExecContext(ctx, q, hash, userID)
	if err != nil {
		return fmt.Errorf("パスワードの更新: %w", err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("パスワードの更新: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}

	return nil
}

// passwordChangeRequest はパスワード変更の入力。
type passwordChangeRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// handlePasswordChange はパスワードを変更する。RequireLogin を通っている前提。
func (h *Handler) handlePasswordChange(w http.ResponseWriter, r *http.Request) {
	user, ok := UserFrom(r.Context())
	if !ok {
		log.Print("password: RequireLogin を通っていない")
		httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
		return
	}

	var req passwordChangeRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 今のパスワードを確認する。ログイン済みの端末を borrowed しただけの人に
	// パスワードを変えられると、本人が締め出される。
	if err := user.VerifyPassword(req.CurrentPassword); err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "現在のパスワードが違います")
		return
	}

	if err := ValidatePassword(req.NewPassword); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 初期パスワードのまま「変更した」ことにさせない。
	if req.NewPassword == req.CurrentPassword {
		httpx.WriteError(w, http.StatusBadRequest, ErrSamePassword.Error())
		return
	}

	if err := h.store.SetPassword(r.Context(), user.ID, req.NewPassword); err != nil {
		log.Printf("password: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
		return
	}

	// 他の端末のセッションを全て切る。パスワードを変えたのに、
	// 盗まれた端末のセッションが1年間生き続けるのでは変更した意味がない。
	if err := h.sessions.DeleteByUser(r.Context(), user.ID); err != nil {
		log.Printf("password: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
		return
	}

	// 今の端末は繋ぎ直す。ここでログアウトさせると、
	// 変更した直後にログイン画面へ戻されることになる。
	token, expiresAt, err := h.sessions.Create(r.Context(), user.ID)
	if err != nil {
		log.Printf("password: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
		return
	}
	setSessionCookie(w, token, expiresAt, h.cookieSecure)

	updated := *user
	updated.MustChangePassword = false

	httpx.JSON(w, http.StatusOK, loginResponse{
		User:       newUserResponse(&updated),
		RedirectTo: httpx.DefaultRedirect,
	})
}
