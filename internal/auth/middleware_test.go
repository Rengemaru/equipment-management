package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// adminOnlyMux は admin 専用の書き込みAPLを1つ持つ mux を返す。
func adminOnlyMux(t *testing.T, h *Handler, reached *bool) *http.ServeMux {
	t.Helper()

	mux := http.NewServeMux()
	h.Register(mux)
	mux.Handle("POST /api/items", h.RequireAdmin(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			*reached = true
			w.WriteHeader(http.StatusCreated)
		})))

	return mux
}

// loginAs は指定の権限の利用者を作ってログインし、Cookie を返す。
func loginAs(t *testing.T, h *Handler, store *Store, loginID string, role Role) *http.Cookie {
	t.Helper()

	in := validUser()
	in.LoginID = loginID
	in.Role = role
	// パスワード変更の強制と混ざらないようにする。ここで見たいのは権限だけ。
	in.MustChangePassword = false

	if _, err := store.Create(context.Background(), in); err != nil {
		t.Fatalf("Create(%s): %v", loginID, err)
	}

	w := postLogin(t, h, `{"login_id":"`+loginID+`","password":"password123"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("ログインに失敗: %d %s", w.Code, w.Body.String())
	}

	for _, c := range w.Result().Cookies() {
		if c.Name == SessionCookieName {
			return c
		}
	}

	t.Fatal("セッションCookieが無い")
	return nil
}

// member が書き込みAPIを叩くと拒否されること。
// これが崩れると、システム化する主目的が失われる。
func TestRequireAdmin_memberの書き込みを拒否する(t *testing.T) {
	h, store := newTestHandler(t)
	c := loginAs(t, h, store, "member1", RoleMember)

	var reached bool
	req := httptest.NewRequest(http.MethodPost, "/api/items", nil)
	req.AddCookie(c)
	w := httptest.NewRecorder()
	adminOnlyMux(t, h, &reached).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d。403 を期待", w.Code)
	}
	// ハンドラまで届いていないこと。届いていれば、権限の判定が
	// ハンドラ側の書き忘れ次第になる。
	if reached {
		t.Error("member のリクエストがハンドラに届いている")
	}
}

func TestRequireAdmin_adminは通す(t *testing.T) {
	h, store := newTestHandler(t)
	c := loginAs(t, h, store, "admin1", RoleAdmin)

	var reached bool
	req := httptest.NewRequest(http.MethodPost, "/api/items", nil)
	req.AddCookie(c)
	w := httptest.NewRecorder()
	adminOnlyMux(t, h, &reached).ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d。201 を期待。body = %s", w.Code, w.Body.String())
	}
	if !reached {
		t.Error("admin のリクエストがハンドラに届いていない")
	}
}

// 未ログインは 403 ではなく 401。フロントはログイン画面へ送る必要がある。
func TestRequireAdmin_未ログインは401(t *testing.T) {
	h, _ := newTestHandler(t)

	var reached bool
	req := httptest.NewRequest(http.MethodPost, "/api/items", nil)
	w := httptest.NewRecorder()
	adminOnlyMux(t, h, &reached).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d。401 を期待", w.Code)
	}
	if reached {
		t.Error("未ログインのリクエストがハンドラに届いている")
	}
}

// 偽のCookieで admin になれないこと。
func TestRequireAdmin_偽のCookieでは通らない(t *testing.T) {
	h, _ := newTestHandler(t)

	var reached bool
	req := httptest.NewRequest(http.MethodPost, "/api/items", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "偽のトークン"})
	w := httptest.NewRecorder()
	adminOnlyMux(t, h, &reached).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d。401 を期待", w.Code)
	}
	if reached {
		t.Error("偽の Cookie でハンドラに届いている")
	}
}

// RequireAdmin は中で RequireLogin を通すので、単独で使っても素通りしないこと。
// 並べて書く形にすると、いつか片方だけ書いた経路ができる。
func TestRequireAdmin_単独で使っても認証される(t *testing.T) {
	h, _ := newTestHandler(t)

	var reached bool
	handler := h.RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/items", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if reached {
		t.Fatal("RequireLogin を通さずにハンドラへ届いている")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d。401 を期待", w.Code)
	}
}

// 権限があっても、初期パスワードのままなら止まること。
func TestRequireAdmin_初期パスワードのadminは止める(t *testing.T) {
	h, store := newTestHandler(t)

	in := validUser()
	in.LoginID = "admin2"
	in.Role = RoleAdmin
	in.MustChangePassword = true
	if _, err := store.Create(context.Background(), in); err != nil {
		t.Fatalf("Create: %v", err)
	}

	loginResp := postLogin(t, h, `{"login_id":"admin2","password":"password123"}`)
	var c *http.Cookie
	for _, got := range loginResp.Result().Cookies() {
		if got.Name == SessionCookieName {
			c = got
		}
	}

	var reached bool
	req := httptest.NewRequest(http.MethodPost, "/api/items", nil)
	req.AddCookie(c)
	w := httptest.NewRecorder()
	adminOnlyMux(t, h, &reached).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d。403 を期待", w.Code)
	}
	if reached {
		t.Error("初期パスワードのままハンドラに届いている")
	}
}

// 無効化された admin は入れないこと。卒業後にセッションが残っていても通さない。
func TestRequireAdmin_無効化されたadminを拒否する(t *testing.T) {
	h, store := newTestHandler(t)
	c := loginAs(t, h, store, "admin3", RoleAdmin)

	if _, err := store.sqldb.Exec(`UPDATE users SET is_active = 0 WHERE login_id = 'admin3'`); err != nil {
		t.Fatalf("無効化: %v", err)
	}

	var reached bool
	req := httptest.NewRequest(http.MethodPost, "/api/items", nil)
	req.AddCookie(c)
	w := httptest.NewRecorder()
	adminOnlyMux(t, h, &reached).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d。401 を期待", w.Code)
	}
	if reached {
		t.Error("無効化された利用者がハンドラに届いている")
	}
}
