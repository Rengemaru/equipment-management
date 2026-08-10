package auth

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/Rengemaru/equipment-management/internal/httpx"
)

// contextKey はこのパッケージ専用のキーの型。
// string を直接使うと、他のパッケージが同じ名前で上書きできてしまう。
type contextKey struct{}

var userContextKey = contextKey{}

// UserFrom はミドルウェアが入れた利用者を取り出す。
//
// RequireLogin を通っていれば必ず取れる。取れないのは経路の組み立てを
// 誤った場合なので、呼び出し側は ok を見て 500 を返すこと。
// 「取れなければ未ログインとして扱う」と書くと、認証を通していない経路が
// 黙って動いてしまう。
func UserFrom(ctx context.Context) (*User, bool) {
	u, ok := ctx.Value(userContextKey).(*User)
	return u, ok
}

// withUser は利用者を context に入れた Request を返す。
func withUser(r *http.Request, u *User) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), userContextKey, u))
}

// RequireLogin はログイン済みでなければ 401 を返すミドルウェア。
//
// APIに対して使う。画面側の「ログイン画面へ送って元のページに戻す」は
// フロントエンドが 401 を見て行う。サーバがリダイレクトを返すと、
// fetch の応答として受け取ったフロントがHTMLをJSONとして解釈することになる。
func (h *Handler) RequireLogin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := h.sessions.Lookup(r.Context(), tokenFromRequest(r))
		if err != nil {
			if errors.Is(err, ErrSessionInvalid) {
				// 期限切れのCookieを持ち続けると、以後ずっと401になる。
				// ここで消して、ログインし直せる状態にする。
				clearSessionCookie(w, h.cookieSecure)
				httpx.WriteError(w, http.StatusUnauthorized, "ログインしてください")
				return
			}

			log.Printf("session lookup: %v", err)
			httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
			return
		}

		next.ServeHTTP(w, withUser(r, user))
	})
}
