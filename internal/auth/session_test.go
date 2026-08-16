package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestSessions は Store と SessionStore の組を返す。
func newTestSessions(t *testing.T) (*Store, *SessionStore, *User) {
	t.Helper()

	store := newTestStore(t)
	user, err := store.Create(context.Background(), validUser())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	return store, NewSessionStore(store.sqldb, []byte("test-secret-32-bytes-............")), user
}

func TestSession_作って引ける(t *testing.T) {
	ctx := context.Background()
	_, sessions, user := newTestSessions(t)

	token, expiresAt, err := sessions.Create(ctx, user.ID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if token == "" {
		t.Fatal("トークンが空")
	}

	// 1年。QRを読むたびにログインを求めない土台になる。
	want := time.Now().UTC().Add(SessionDuration)
	if diff := expiresAt.Sub(want); diff > time.Minute || diff < -time.Minute {
		t.Errorf("有効期限 = %v。約 %v を期待", expiresAt, want)
	}

	got, err := sessions.Lookup(ctx, token)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got.ID != user.ID {
		t.Errorf("別の利用者が返った: %d", got.ID)
	}
}

// Cookie の値をそのままDBに入れない。
// DBファイルはバックアップとして持ち出され、引き継ぎで後任にも渡る。
func TestSession_トークンをそのまま保存しない(t *testing.T) {
	ctx := context.Background()
	_, sessions, user := newTestSessions(t)

	token, _, err := sessions.Create(ctx, user.ID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	var stored string
	if err := sessions.sqldb.QueryRow(`SELECT id FROM sessions`).Scan(&stored); err != nil {
		t.Fatalf("id の取得: %v", err)
	}

	if stored == token {
		t.Fatal("Cookie の値がそのまま保存されている")
	}
	if strings.Contains(stored, token) {
		t.Fatal("保存された値からトークンが読み取れる")
	}
}

// SESSION_SECRET を変えると全員ログアウトになること（.env.example の記述どおり）。
func TestSession_鍵を変えると引けなくなる(t *testing.T) {
	ctx := context.Background()
	_, sessions, user := newTestSessions(t)

	token, _, err := sessions.Create(ctx, user.ID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	rotated := NewSessionStore(sessions.sqldb, []byte("another-secret-32-bytes.........."))
	if _, err := rotated.Lookup(ctx, token); err == nil {
		t.Fatal("鍵を変えても引けてしまう")
	}
}

func TestSession_知らないトークンは無効(t *testing.T) {
	ctx := context.Background()
	_, sessions, _ := newTestSessions(t)

	for _, token := range []string{"", "でたらめ", strings.Repeat("a", 43)} {
		if _, err := sessions.Lookup(ctx, token); err == nil {
			t.Errorf("%q が通ってしまう", token)
		}
	}
}

func TestSession_期限切れは無効(t *testing.T) {
	ctx := context.Background()
	_, sessions, user := newTestSessions(t)

	token, _, err := sessions.Create(ctx, user.ID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// 期限を過去にする。
	const q = `UPDATE sessions SET expires_at = datetime('now', '-1 day')`
	if _, err := sessions.sqldb.ExecContext(ctx, q); err != nil {
		t.Fatalf("期限の変更: %v", err)
	}

	if _, err := sessions.Lookup(ctx, token); err == nil {
		t.Fatal("期限切れのセッションで通ってしまう")
	}
}

// 卒業して無効化された人は、セッションが生きていても入れないこと。
func TestSession_無効化された利用者は入れない(t *testing.T) {
	ctx := context.Background()
	_, sessions, user := newTestSessions(t)

	token, _, err := sessions.Create(ctx, user.ID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	const q = `UPDATE users SET is_active = 0 WHERE id = ?`
	if _, err := sessions.sqldb.ExecContext(ctx, q, user.ID); err != nil {
		t.Fatalf("無効化: %v", err)
	}

	if _, err := sessions.Lookup(ctx, token); err == nil {
		t.Fatal("無効化された利用者が通ってしまう")
	}
}

func TestSession_削除すると引けなくなる(t *testing.T) {
	ctx := context.Background()
	_, sessions, user := newTestSessions(t)

	token, _, err := sessions.Create(ctx, user.ID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := sessions.Delete(ctx, token); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := sessions.Lookup(ctx, token); err == nil {
		t.Fatal("削除後も引けてしまう")
	}

	// 無いものを消してもエラーにしない。
	if err := sessions.Delete(ctx, token); err != nil {
		t.Errorf("2回目の Delete: %v", err)
	}
}

// パスワード変更時に全端末のセッションを切るために使う。
func TestSession_利用者単位で全て消せる(t *testing.T) {
	ctx := context.Background()
	_, sessions, user := newTestSessions(t)

	var tokens []string
	for i := 0; i < 3; i++ {
		token, _, err := sessions.Create(ctx, user.ID)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		tokens = append(tokens, token)
	}

	if err := sessions.DeleteByUser(ctx, user.ID); err != nil {
		t.Fatalf("DeleteByUser: %v", err)
	}

	for i, token := range tokens {
		if _, err := sessions.Lookup(ctx, token); err == nil {
			t.Errorf("%d 番目のセッションが残っている", i)
		}
	}
}

// 期限切れの行が積み上がらないこと。
func TestSession_作成時に期限切れを掃除する(t *testing.T) {
	ctx := context.Background()
	_, sessions, user := newTestSessions(t)

	if _, _, err := sessions.Create(ctx, user.ID); err != nil {
		t.Fatalf("Create: %v", err)
	}
	const expire = `UPDATE sessions SET expires_at = datetime('now', '-1 day')`
	if _, err := sessions.sqldb.ExecContext(ctx, expire); err != nil {
		t.Fatalf("期限の変更: %v", err)
	}

	if _, _, err := sessions.Create(ctx, user.ID); err != nil {
		t.Fatalf("Create: %v", err)
	}

	var n int
	if err := sessions.sqldb.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&n); err != nil {
		t.Fatalf("件数の取得: %v", err)
	}
	if n != 1 {
		t.Errorf("セッションが %d 件。1件を期待（期限切れが残っている）", n)
	}
}

// last_seen_at は初回のアクセスで入ること。
func TestSession_最終アクセス時刻を記録する(t *testing.T) {
	ctx := context.Background()
	_, sessions, user := newTestSessions(t)

	token, _, err := sessions.Create(ctx, user.ID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := sessions.Lookup(ctx, token); err != nil {
		t.Fatalf("Lookup: %v", err)
	}

	var lastSeen *string
	if err := sessions.sqldb.QueryRow(`SELECT last_seen_at FROM sessions`).Scan(&lastSeen); err != nil {
		t.Fatalf("last_seen_at の取得: %v", err)
	}
	if lastSeen == nil {
		t.Fatal("last_seen_at が入っていない")
	}
}

// ---- Cookie とミドルウェア ----

func TestSetSessionCookie_安全な属性が付く(t *testing.T) {
	w := httptest.NewRecorder()
	setSessionCookie(w, "token", time.Now().Add(time.Hour), true)

	c := w.Result().Cookies()[0]
	if !c.HttpOnly {
		t.Error("HttpOnly が付いていない。XSS でセッションを持ち出される")
	}
	if !c.Secure {
		t.Error("Secure が反映されていない")
	}
	// Strict だと、QRリーダーや外部リンクから来た時にCookieが送られない。
	if c.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite = %v。Lax を期待", c.SameSite)
	}
	if c.Path != "/" {
		t.Errorf("Path = %q", c.Path)
	}
}

// HTTP運用では Secure を落とせること。落とせないとログインが無限ループする。
func TestSetSessionCookie_Secureを落とせる(t *testing.T) {
	w := httptest.NewRecorder()
	setSessionCookie(w, "token", time.Now().Add(time.Hour), false)

	if w.Result().Cookies()[0].Secure {
		t.Error("COOKIE_SECURE=false でも Secure が付いている")
	}
}

func TestClearSessionCookie_設定時と属性が揃う(t *testing.T) {
	set := httptest.NewRecorder()
	setSessionCookie(set, "token", time.Now().Add(time.Hour), true)
	setCookie := set.Result().Cookies()[0]

	clear := httptest.NewRecorder()
	clearSessionCookie(clear, true)
	clearCookie := clear.Result().Cookies()[0]

	// Path や Secure が違うと、ブラウザは別の Cookie とみなして消さない。
	if clearCookie.Name != setCookie.Name || clearCookie.Path != setCookie.Path ||
		clearCookie.Secure != setCookie.Secure || clearCookie.SameSite != setCookie.SameSite {
		t.Errorf("属性が揃っていない\n  設定: %+v\n  削除: %+v", setCookie, clearCookie)
	}
	if clearCookie.MaxAge >= 0 {
		t.Errorf("MaxAge = %d。負の値を期待", clearCookie.MaxAge)
	}
}
