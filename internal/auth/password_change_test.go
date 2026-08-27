package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// postPasswordChange はパスワード変更APIを叩く。
func postPasswordChange(t *testing.T, h *Handler, c *http.Cookie, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if c != nil {
		req.AddCookie(c)
	}

	return serve(t, h, req)
}

func TestHandlePasswordChange_変更後は新しいパスワードで入れる(t *testing.T) {
	h, _ := newTestHandler(t)
	c := loginAndGetCookie(t, h)

	w := postPasswordChange(t, h, c,
		`{"current_password":"password123","new_password":"newpassword456"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	// 変更後の利用者は must_change_password が下りていること。
	var got loginResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答が JSON でない: %v", err)
	}
	if got.User.MustChangePassword {
		t.Error("must_change_password が下りていない")
	}

	// 古いパスワードでは入れない。
	if old := postLogin(t, h, `{"login_id":"yamada","password":"password123"}`); old.Code != http.StatusUnauthorized {
		t.Errorf("古いパスワードで入れる: status = %d", old.Code)
	}

	// 新しいパスワードで入れる。
	if now := postLogin(t, h, `{"login_id":"yamada","password":"newpassword456"}`); now.Code != http.StatusOK {
		t.Errorf("新しいパスワードで入れない: status = %d, body = %s", now.Code, now.Body.String())
	}
}

// 現在のパスワードを知らない人に変更させない。
// ログイン済みの端末を少し借りただけの人に変えられると、本人が締め出される。
func TestHandlePasswordChange_現在のパスワードが違えば拒否する(t *testing.T) {
	h, _ := newTestHandler(t)
	c := loginAndGetCookie(t, h)

	w := postPasswordChange(t, h, c,
		`{"current_password":"wrongpassword","new_password":"newpassword456"}`)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d。401 を期待", w.Code)
	}

	// 変わっていないこと。
	if now := postLogin(t, h, `{"login_id":"yamada","password":"password123"}`); now.Code != http.StatusOK {
		t.Errorf("元のパスワードで入れなくなっている: %d", now.Code)
	}
}

func TestHandlePasswordChange_短いパスワードを拒否する(t *testing.T) {
	h, _ := newTestHandler(t)
	c := loginAndGetCookie(t, h)

	w := postPasswordChange(t, h, c,
		`{"current_password":"password123","new_password":"short"}`)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d。400 を期待", w.Code)
	}
}

// 初期パスワードのまま「変更した」ことにさせない。
func TestHandlePasswordChange_同じパスワードを拒否する(t *testing.T) {
	h, _ := newTestHandler(t)
	c := loginAndGetCookie(t, h)

	w := postPasswordChange(t, h, c,
		`{"current_password":"password123","new_password":"password123"}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d。400 を期待", w.Code)
	}
	if !strings.Contains(w.Body.String(), "同じ") {
		t.Errorf("理由が分からない: %s", w.Body.String())
	}
}

func TestHandlePasswordChange_未ログインでは叩けない(t *testing.T) {
	h, _ := newTestHandler(t)

	w := postPasswordChange(t, h, nil,
		`{"current_password":"password123","new_password":"newpassword456"}`)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d。401 を期待", w.Code)
	}
}

// 変更したら他の端末のセッションを切ること。
// 盗まれた端末のセッションが1年間生き続けるのでは、変更した意味がない。
func TestHandlePasswordChange_他の端末のセッションを切る(t *testing.T) {
	h, _ := newTestHandler(t)

	other := loginAndGetCookie(t, h) // 別端末に見立てる
	current := loginAndGetCookie(t, h)

	w := postPasswordChange(t, h, current,
		`{"current_password":"password123","new_password":"newpassword456"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.AddCookie(other)
	if got := serve(t, h, req); got.Code != http.StatusUnauthorized {
		t.Errorf("他端末のセッションが生きている: status = %d", got.Code)
	}
}

// 変更した端末はそのまま使えること。
// ここでログアウトさせると、変更した直後にログイン画面へ戻される。
func TestHandlePasswordChange_変更した端末は繋がったまま(t *testing.T) {
	h, _ := newTestHandler(t)
	c := loginAndGetCookie(t, h)

	w := postPasswordChange(t, h, c,
		`{"current_password":"password123","new_password":"newpassword456"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}

	// 応答で配り直された Cookie を使う。
	var renewed *http.Cookie
	for _, got := range w.Result().Cookies() {
		if got.Name == SessionCookieName && got.Value != "" {
			renewed = got
		}
	}
	if renewed == nil {
		t.Fatal("セッションが配り直されていない")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.AddCookie(renewed)
	if got := serve(t, h, req); got.Code != http.StatusOK {
		t.Errorf("変更後に自分の端末が切れている: status = %d", got.Code)
	}
}

// ---- must_change_password の強制 ----

// 初期パスワードのままでは、他のAPIを叩けないこと。
// フロントの画面遷移だけで縛ると、APIを直接叩けば素通りできる。
func TestRequireLogin_初期パスワードのままなら他のAPIを拒否する(t *testing.T) {
	h, _ := newTestHandler(t)
	c := loginAndGetCookie(t, h)

	// 認証を必要とする適当な経路を1つ足して確かめる。
	mux := http.NewServeMux()
	h.Register(mux)
	mux.Handle("GET /api/items", h.RequireLogin(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})))

	req := httptest.NewRequest(http.MethodGet, "/api/items", nil)
	req.AddCookie(c)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d。403 を期待", w.Code)
	}

	// フロントが分岐に使う識別子。文言で分岐させると日本語を直した時に壊れる。
	var got struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答が JSON でない: %v", err)
	}
	if got.Code != CodePasswordChangeRequired {
		t.Errorf("code = %q。%q を期待", got.Code, CodePasswordChangeRequired)
	}
}

// 変更に必要な経路は通ること。ここを塞ぐと変更する手段が無くなる。
func TestRequireLogin_初期パスワードでも変更経路は通す(t *testing.T) {
	h, _ := newTestHandler(t)
	c := loginAndGetCookie(t, h)

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.AddCookie(c)
	if w := serve(t, h, req); w.Code != http.StatusOK {
		t.Errorf("/api/me が通らない: status = %d", w.Code)
	}

	w := postPasswordChange(t, h, c,
		`{"current_password":"password123","new_password":"newpassword456"}`)
	if w.Code != http.StatusOK {
		t.Errorf("/api/password が通らない: status = %d, body = %s", w.Code, w.Body.String())
	}
}

// 変更した後は他のAPIも通ること。
func TestRequireLogin_変更後は他のAPIを通す(t *testing.T) {
	h, _ := newTestHandler(t)
	c := loginAndGetCookie(t, h)

	changed := postPasswordChange(t, h, c,
		`{"current_password":"password123","new_password":"newpassword456"}`)
	if changed.Code != http.StatusOK {
		t.Fatalf("変更に失敗: %d %s", changed.Code, changed.Body.String())
	}

	var renewed *http.Cookie
	for _, got := range changed.Result().Cookies() {
		if got.Name == SessionCookieName && got.Value != "" {
			renewed = got
		}
	}

	mux := http.NewServeMux()
	h.Register(mux)
	mux.Handle("GET /api/items", h.RequireLogin(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})))

	req := httptest.NewRequest(http.MethodGet, "/api/items", nil)
	req.AddCookie(renewed)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("変更後も拒否されている: status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestSetPassword_居ない利用者はErrNotFound(t *testing.T) {
	store := newTestStore(t)

	err := store.SetPassword(context.Background(), 9999, "newpassword456")
	if err == nil {
		t.Fatal("エラーを期待したが nil")
	}
}

// ---- 変更後の復帰（m1-spec §フロー 5→6） ----

// QRから来た新入部員が、初回ログインとパスワード変更を終えた後に
// 元の備品ページへ戻ること。
//
// __M1で最も重い経路。__ ここで戻せないとQRを読み直させることになり、
// その一手間が記録漏れの直接原因になる（CLAUDE.md）。
func TestHandlePasswordChange_変更後にnextへ戻す(t *testing.T) {
	h, _ := newTestHandler(t)

	// 1. QRから来て初回ログイン。行き先を持ったまま変更画面へ送られる。
	w := postLogin(t, h, `{"login_id":"yamada","password":"password123","next":"/i/0042"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("login: status = %d, body = %s", w.Code, w.Body.String())
	}

	var afterLogin loginResponse
	if err := json.Unmarshal(w.Body.Bytes(), &afterLogin); err != nil {
		t.Fatalf("応答が JSON でない: %v", err)
	}
	if want := "/password?next=%2Fi%2F0042"; afterLogin.RedirectTo != want {
		t.Fatalf("login redirect_to = %q。%q を期待", afterLogin.RedirectTo, want)
	}

	c := sessionCookie(t, w)

	// 2. 画面がクエリの next をそのまま送り返す。
	got := postPasswordChange(t, h, c,
		`{"current_password":"password123","new_password":"newpassword456","next":"/i/0042"}`)
	if got.Code != http.StatusOK {
		t.Fatalf("password: status = %d, body = %s", got.Code, got.Body.String())
	}

	var afterChange loginResponse
	if err := json.Unmarshal(got.Body.Bytes(), &afterChange); err != nil {
		t.Fatalf("応答が JSON でない: %v", err)
	}
	if afterChange.RedirectTo != "/i/0042" {
		t.Errorf("redirect_to = %q。/i/0042 を期待（QRを読み直させない）", afterChange.RedirectTo)
	}
}

// 検証は変更側でも行うこと。ログインを通さずに直接叩かれても、
// 外部サイトへ飛ばせてはならない。
func TestHandlePasswordChange_外部へのnextを弾く(t *testing.T) {
	dangerous := []string{
		"//evil.example.com",
		"https://evil.example.com/x",
		"http://evil.example.com",
		`/\evil.example.com`,
		"javascript:alert(1)",
	}

	for _, next := range dangerous {
		t.Run(next, func(t *testing.T) {
			h, _ := newTestHandler(t)
			c := loginAndGetCookie(t, h)

			body, err := json.Marshal(passwordChangeRequest{
				CurrentPassword: "password123",
				NewPassword:     "newpassword456",
				Next:            next,
			})
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}

			w := postPasswordChange(t, h, c, string(body))
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
			}

			var got loginResponse
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatalf("応答が JSON でない: %v", err)
			}
			if got.RedirectTo != "/" {
				t.Errorf("redirect_to = %q。/ に落とすこと", got.RedirectTo)
			}
		})
	}
}

// 自分でパスワードを変えに来ただけの人は next を持たない。
// 行き先が無い時は必ずトップを返す（omitempty にしない理由と同じ）。
func TestHandlePasswordChange_nextが無ければトップへ戻す(t *testing.T) {
	h, _ := newTestHandler(t)
	c := loginAndGetCookie(t, h)

	w := postPasswordChange(t, h, c,
		`{"current_password":"password123","new_password":"newpassword456"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var got loginResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答が JSON でない: %v", err)
	}
	if got.RedirectTo != "/" {
		t.Errorf("redirect_to = %q。/ を期待", got.RedirectTo)
	}
}
