package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestHandler は利用者1人が登録済みの Handler を返す。
func newTestHandler(t *testing.T) (*Handler, *Store) {
	t.Helper()

	store := newTestStore(t)
	if _, err := store.Create(context.Background(), validUser()); err != nil {
		t.Fatalf("Create: %v", err)
	}

	sessions := NewSessionStore(store.sqldb, []byte("test-secret-32-bytes-............"))

	// テストでは Secure を落とす。httptest は HTTP のため、
	// 付けたままだと本物のブラウザと挙動が変わる。
	return NewHandler(store, sessions, NewThrottle(store.sqldb), false), store
}

// serve は Handler が登録した経路にリクエストを流す。
func serve(t *testing.T, h *Handler, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()

	mux := http.NewServeMux()
	h.Register(mux)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	return w
}

// postLogin はログインAPIを叩く。
func postLogin(t *testing.T, h *Handler, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	return serve(t, h, req)
}

// loginAndGetCookie はログインしてセッションCookieを取り出す。
func loginAndGetCookie(t *testing.T, h *Handler) *http.Cookie {
	t.Helper()

	w := postLogin(t, h, `{"login_id":"yamada","password":"password123"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("ログインに失敗: %d %s", w.Code, w.Body.String())
	}

	for _, c := range w.Result().Cookies() {
		if c.Name == SessionCookieName {
			return c
		}
	}

	t.Fatal("セッションCookieが設定されていない")
	return nil
}

func TestHandleLogin_正しい認証情報で200(t *testing.T) {
	h, _ := newTestHandler(t)

	w := postLogin(t, h, `{"login_id":"yamada","password":"password123"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var got loginResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答が JSON でない: %v", err)
	}
	if got.User.LoginID != "yamada" {
		t.Errorf("LoginID = %q", got.User.LoginID)
	}
	if got.User.Role != RoleMember {
		t.Errorf("Role = %q", got.User.Role)
	}
	// フロントが /password へ飛ばす判断に使う。
	if !got.User.MustChangePassword {
		t.Error("must_change_password が返っていない")
	}
}

// パスワードハッシュが応答に出ないこと。
func TestHandleLogin_応答にハッシュを含めない(t *testing.T) {
	h, _ := newTestHandler(t)

	w := postLogin(t, h, `{"login_id":"yamada","password":"password123"}`)

	body := w.Body.String()
	if strings.Contains(body, "$2") || strings.Contains(strings.ToLower(body), "password_hash") {
		t.Errorf("ハッシュが応答に含まれる: %s", body)
	}
}

// 存在しないIDと誤ったパスワードで、応答を区別できないこと。
// 区別できると、存在するログインIDを総当たりで洗い出せる。
func TestHandleLogin_失敗の応答を区別できない(t *testing.T) {
	h, _ := newTestHandler(t)

	cases := map[string]string{
		"存在しないID":  `{"login_id":"nobody","password":"password123"}`,
		"誤ったパスワード": `{"login_id":"yamada","password":"wrongpassword"}`,
		"空のID":     `{"login_id":"","password":"password123"}`,
		"空のパスワード":  `{"login_id":"yamada","password":""}`,
	}

	var first string
	for name, body := range cases {
		w := postLogin(t, h, body)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s: status = %d。401 を期待", name, w.Code)
		}

		if first == "" {
			first = w.Body.String()
			continue
		}
		if w.Body.String() != first {
			t.Errorf("%s: 応答が他と違う\n  %s\n  %s", name, w.Body.String(), first)
		}
	}
}

// 無効化されたユーザーは入れない。ただし理由は伝えない。
func TestHandleLogin_無効化されたユーザーを拒否する(t *testing.T) {
	h, store := newTestHandler(t)

	const q = `UPDATE users SET is_active = 0 WHERE login_id = 'yamada'`
	if _, err := store.sqldb.Exec(q); err != nil {
		t.Fatalf("無効化: %v", err)
	}

	w := postLogin(t, h, `{"login_id":"yamada","password":"password123"}`)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d。401 を期待", w.Code)
	}
	// 「無効です」と返すと、そのIDが存在することが分かる。
	if strings.Contains(w.Body.String(), "無効") {
		t.Errorf("無効化されていることが分かる応答になっている: %s", w.Body.String())
	}
}

func TestHandleLogin_壊れた本文は400(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"JSONでない", `not json`},
		{"空", ``},
		{"型が違う", `{"login_id":123,"password":"password123"}`},
		{"知らない項目", `{"login_id":"yamada","password":"x","admin":true}`},
	}

	h, _ := newTestHandler(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := postLogin(t, h, tt.body)

			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d。400 を期待。body = %s", w.Code, w.Body.String())
			}
		})
	}
}

// GET では叩けないこと（ServeMux のパターンで弾かれる）。
func TestHandleLogin_GETを受け付けない(t *testing.T) {
	h, _ := newTestHandler(t)

	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/login", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Error("GET で認証できてしまう")
	}
}

// 大文字のログインIDでも同じ人として認証できること。
func TestHandleLogin_ログインIDの大文字小文字を区別しない(t *testing.T) {
	h, _ := newTestHandler(t)

	w := postLogin(t, h, `{"login_id":"YAMADA","password":"password123"}`)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d。200 を期待。body = %s", w.Code, w.Body.String())
	}
}

func TestAuthenticate_失敗を全てErrInvalidCredentialsに潰す(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	if _, err := store.Create(ctx, validUser()); err != nil {
		t.Fatalf("Create: %v", err)
	}

	tests := []struct {
		name     string
		loginID  string
		password string
	}{
		{"存在しないID", "nobody", "password123"},
		{"誤ったパスワード", "yamada", "wrongpassword"},
		{"空のID", "", "password123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := store.Authenticate(ctx, tt.loginID, tt.password)
			if err == nil {
				t.Fatal("エラーを期待したが nil")
			}
			// ErrNotFound などが漏れると、呼び出し側が理由で分岐できてしまう。
			if err.Error() != ErrInvalidCredentials.Error() {
				t.Errorf("err = %v。ErrInvalidCredentials を期待", err)
			}
		})
	}
}

// ---- セッション ----

func TestHandleLogin_成功するとCookieが設定される(t *testing.T) {
	h, _ := newTestHandler(t)

	c := loginAndGetCookie(t, h)

	if c.Value == "" {
		t.Error("Cookie の値が空")
	}
	if !c.HttpOnly {
		t.Error("HttpOnly が付いていない")
	}
}

// 失敗した時に Cookie を配らないこと。
func TestHandleLogin_失敗時はCookieを設定しない(t *testing.T) {
	h, _ := newTestHandler(t)

	w := postLogin(t, h, `{"login_id":"yamada","password":"wrongpassword"}`)

	for _, c := range w.Result().Cookies() {
		if c.Name == SessionCookieName && c.Value != "" {
			t.Errorf("認証に失敗したのに Cookie が配られている: %q", c.Value)
		}
	}
}

func TestHandleMe_Cookieがあれば自分を返す(t *testing.T) {
	h, _ := newTestHandler(t)
	c := loginAndGetCookie(t, h)

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.AddCookie(c)
	w := serve(t, h, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var got loginResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答が JSON でない: %v", err)
	}
	if got.User.LoginID != "yamada" {
		t.Errorf("LoginID = %q", got.User.LoginID)
	}
}

func TestHandleMe_Cookieが無ければ401(t *testing.T) {
	h, _ := newTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	w := serve(t, h, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d。401 を期待", w.Code)
	}
	// HTML ではなく JSON を返すこと。フロントは fetch で受け取る。
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q", ct)
	}
}

// 偽造した Cookie で入れないこと。
func TestHandleMe_偽のCookieを拒否する(t *testing.T) {
	h, _ := newTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "偽のトークン"})
	w := serve(t, h, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d。401 を期待", w.Code)
	}
}

// 無効なCookieは消させる。持ち続けると以後ずっと401になる。
func TestRequireLogin_無効なCookieを消す(t *testing.T) {
	h, _ := newTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "期限切れ相当"})
	w := serve(t, h, req)

	var cleared bool
	for _, c := range w.Result().Cookies() {
		if c.Name == SessionCookieName && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("無効な Cookie が消されていない")
	}
}

func TestHandleLogout_セッションが無効になる(t *testing.T) {
	h, _ := newTestHandler(t)
	c := loginAndGetCookie(t, h)

	logout := httptest.NewRequest(http.MethodPost, "/api/logout", nil)
	logout.AddCookie(c)
	w := serve(t, h, logout)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d。204 を期待", w.Code)
	}

	// ログアウト後、同じ Cookie では入れないこと。
	me := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	me.AddCookie(c)
	if got := serve(t, h, me); got.Code != http.StatusUnauthorized {
		t.Errorf("ログアウト後も入れる: status = %d", got.Code)
	}
}

// ログインしていなくても 204。既にログアウトしているなら目的は達している。
func TestHandleLogout_未ログインでも204(t *testing.T) {
	h, _ := newTestHandler(t)

	w := serve(t, h, httptest.NewRequest(http.MethodPost, "/api/logout", nil))

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d。204 を期待", w.Code)
	}
}

// ログイン→ログアウト→再ログインが通ること。
func TestSession_ログインし直せる(t *testing.T) {
	h, _ := newTestHandler(t)

	first := loginAndGetCookie(t, h)

	logout := httptest.NewRequest(http.MethodPost, "/api/logout", nil)
	logout.AddCookie(first)
	serve(t, h, logout)

	second := loginAndGetCookie(t, h)
	if second.Value == first.Value {
		t.Error("同じセッションIDが再利用されている")
	}

	me := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	me.AddCookie(second)
	if w := serve(t, h, me); w.Code != http.StatusOK {
		t.Errorf("再ログイン後に入れない: %d", w.Code)
	}
}

// ---- next による復帰 ----

// QRから来た人が、認証後に元の備品ページへ戻れること。
// 戻せないと、もう一度QRを読み直させることになり、記録漏れの直接原因になる。
func TestHandleLogin_nextで元のページに戻す(t *testing.T) {
	h, store := newTestHandler(t)

	// 初期パスワードのままだとパスワード変更へ送られるため、外しておく。
	if _, err := store.sqldb.Exec(`UPDATE users SET must_change_password = 0`); err != nil {
		t.Fatalf("must_change_password の解除: %v", err)
	}

	w := postLogin(t, h, `{"login_id":"yamada","password":"password123","next":"/i/0042"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var got loginResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答が JSON でない: %v", err)
	}
	if got.RedirectTo != "/i/0042" {
		t.Errorf("redirect_to = %q。/i/0042 を期待", got.RedirectTo)
	}
}

// 細工した next で外部サイトへ飛ばせないこと。
func TestHandleLogin_外部へのnextを弾く(t *testing.T) {
	dangerous := []string{
		"//evil.com",
		"https://evil.com/login",
		`/\evil.com`,
		"javascript:alert(1)",
	}

	for _, next := range dangerous {
		t.Run(next, func(t *testing.T) {
			h, store := newTestHandler(t)
			if _, err := store.sqldb.Exec(`UPDATE users SET must_change_password = 0`); err != nil {
				t.Fatalf("must_change_password の解除: %v", err)
			}

			body, err := json.Marshal(loginRequest{
				LoginID: "yamada", Password: "password123", Next: next,
			})
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}

			w := postLogin(t, h, string(body))
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
			}

			var got loginResponse
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatalf("応答が JSON でない: %v", err)
			}
			if got.RedirectTo != "/" {
				t.Errorf("redirect_to = %q。/ に落とすべき", got.RedirectTo)
			}
		})
	}
}

// 初期パスワードのままなら、next より変更画面を優先すること。
// ここで通すと、変更しないまま使い続けられる。
func TestHandleLogin_初期パスワードならnextより変更画面を優先する(t *testing.T) {
	h, _ := newTestHandler(t)

	w := postLogin(t, h, `{"login_id":"yamada","password":"password123","next":"/i/0042"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var got loginResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答が JSON でない: %v", err)
	}
	if got.RedirectTo != "/password" {
		t.Errorf("redirect_to = %q。/password を期待", got.RedirectTo)
	}
}

// next が無くても必ず行き先を返すこと。
func TestHandleLogin_nextが無ければトップを返す(t *testing.T) {
	h, store := newTestHandler(t)
	if _, err := store.sqldb.Exec(`UPDATE users SET must_change_password = 0`); err != nil {
		t.Fatalf("must_change_password の解除: %v", err)
	}

	w := postLogin(t, h, `{"login_id":"yamada","password":"password123"}`)

	var got loginResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答が JSON でない: %v", err)
	}
	if got.RedirectTo != "/" {
		t.Errorf("redirect_to = %q。/ を期待", got.RedirectTo)
	}
}
