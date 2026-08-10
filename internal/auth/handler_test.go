package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestHandler は admin 1人が登録済みの Handler を返す。
func newTestHandler(t *testing.T) (*Handler, *Store) {
	t.Helper()

	store := newTestStore(t)
	if _, err := store.Create(context.Background(), validUser()); err != nil {
		t.Fatalf("Create: %v", err)
	}

	return NewHandler(store), store
}

// postLogin はログインAPIを叩く。
func postLogin(t *testing.T, h *Handler, body string) *httptest.ResponseRecorder {
	t.Helper()

	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	return w
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
