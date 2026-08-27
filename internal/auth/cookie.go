package auth

import (
	"net/http"
	"time"
)

// SessionCookieName は Cookie の名前。
const SessionCookieName = "session"

// setSessionCookie はセッションCookieを設定する。
func setSessionCookie(w http.ResponseWriter, token string, expiresAt time.Time, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:  SessionCookieName,
		Value: token,
		Path:  "/",

		// JavaScript から読めないようにする。読めると、
		// どこか1箇所のXSSでセッションを持ち出される。
		HttpOnly: true,

		// HTTPS でないと Secure Cookie はブラウザに保存されず、
		// ログインが無限ループする。オンプレでHTTP運用する場合に
		// 落とせるよう、環境変数から渡す。
		Secure: secure,

		// Lax にする。Strict だと、外部のリンクやQRリーダーから
		// 遷移してきた時にCookieが送られず、ログイン済みなのに
		// ログイン画面に飛ばされる。QRから入る導線が主なので致命的になる。
		SameSite: http.SameSiteLaxMode,

		Expires: expiresAt,
		// Expires だけだと端末の時計がずれている時に効かない。両方入れる。
		MaxAge: int(time.Until(expiresAt).Seconds()),
	})
}

// clearSessionCookie は Cookie を消す。
//
// 属性は設定時と揃える。Path や Secure が違うと、ブラウザは
// 別の Cookie とみなして消してくれない。
func clearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
}

// tokenFromRequest は Cookie からセッションの値を取り出す。
func tokenFromRequest(r *http.Request) string {
	c, err := r.Cookie(SessionCookieName)
	if err != nil {
		return ""
	}
	return c.Value
}
