package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// adminSession は admin としてログインした Cookie を返す。
func adminSession(t *testing.T, h *Handler, store *Store) *http.Cookie {
	t.Helper()
	return loginAs(t, h, store, "boss", RoleAdmin)
}

// callAPI は Cookie 付きでリクエストを送る。
func callAPI(t *testing.T, h *Handler, c *http.Cookie, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()

	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	if c != nil {
		req.AddCookie(c)
	}

	return serve(t, h, req)
}

func TestHandleCreateUser_初期パスワードを一度だけ返す(t *testing.T) {
	h, store := newTestHandler(t)
	c := adminSession(t, h, store)

	w := callAPI(t, h, c, http.MethodPost, "/api/users",
		`{"name":"田中花子","login_id":"tanaka","role":"member"}`)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var got userWithPasswordResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答が JSON でない: %v", err)
	}
	if got.InitialPassword == "" {
		t.Fatal("初期パスワードが返っていない")
	}
	if !got.User.MustChangePassword {
		t.Error("must_change_password が立っていない")
	}

	// 返された初期パスワードで実際にログインできること。
	// できないと、admin が作ったユーザーを誰も使えない。
	login := postLogin(t, h, fmt.Sprintf(
		`{"login_id":"tanaka","password":%q}`, got.InitialPassword))
	if login.Code != http.StatusOK {
		t.Errorf("初期パスワードでログインできない: %d %s", login.Code, login.Body.String())
	}

	// 一覧に初期パスワードが出ないこと。表示は作成時の1回だけ。
	list := callAPI(t, h, c, http.MethodGet, "/api/users", "")
	if strings.Contains(list.Body.String(), got.InitialPassword) {
		t.Error("一覧に初期パスワードが出ている")
	}
}

// 権限を省略したら member になること。admin は明示した時だけ。
func TestHandleCreateUser_権限の省略はmember(t *testing.T) {
	h, store := newTestHandler(t)
	c := adminSession(t, h, store)

	w := callAPI(t, h, c, http.MethodPost, "/api/users",
		`{"name":"田中花子","login_id":"tanaka"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var got userWithPasswordResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答が JSON でない: %v", err)
	}
	if got.User.Role != RoleMember {
		t.Errorf("Role = %q。member を期待", got.User.Role)
	}
}

func TestHandleCreateUser_入力の誤りは400(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"名前が空", `{"name":"","login_id":"tanaka"}`},
		{"ログインIDが短い", `{"name":"田中","login_id":"ab"}`},
		{"ログインIDに記号", `{"name":"田中","login_id":"tanaka@example"}`},
		{"権限が不正", `{"name":"田中","login_id":"tanaka","role":"superuser"}`},
	}

	h, store := newTestHandler(t)
	c := adminSession(t, h, store)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := callAPI(t, h, c, http.MethodPost, "/api/users", tt.body)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d。400 を期待。body = %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestHandleCreateUser_重複は409(t *testing.T) {
	h, store := newTestHandler(t)
	c := adminSession(t, h, store)

	body := `{"name":"田中花子","login_id":"tanaka"}`
	if w := callAPI(t, h, c, http.MethodPost, "/api/users", body); w.Code != http.StatusCreated {
		t.Fatalf("1人目: %d %s", w.Code, w.Body.String())
	}

	w := callAPI(t, h, c, http.MethodPost, "/api/users", body)
	if w.Code != http.StatusConflict {
		t.Errorf("status = %d。409 を期待", w.Code)
	}
}

// member はユーザーを作れないこと。
func TestHandleCreateUser_memberは拒否される(t *testing.T) {
	h, store := newTestHandler(t)
	c := loginAs(t, h, store, "member1", RoleMember)

	w := callAPI(t, h, c, http.MethodPost, "/api/users",
		`{"name":"田中花子","login_id":"tanaka"}`)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d。403 を期待", w.Code)
	}

	// 作られていないこと。
	users, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, u := range users {
		if u.LoginID == "tanaka" {
			t.Error("member の操作でユーザーが作られている")
		}
	}
}

func TestHandleListUsers_無効化された人も含めて返す(t *testing.T) {
	h, store := newTestHandler(t)
	c := adminSession(t, h, store)

	created := callAPI(t, h, c, http.MethodPost, "/api/users",
		`{"name":"田中花子","login_id":"tanaka"}`)
	var got userWithPasswordResponse
	if err := json.Unmarshal(created.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答が JSON でない: %v", err)
	}

	path := fmt.Sprintf("/api/users/%d/deactivate", got.User.ID)
	if w := callAPI(t, h, c, http.MethodPost, path, ""); w.Code != http.StatusOK {
		t.Fatalf("無効化: %d %s", w.Code, w.Body.String())
	}

	w := callAPI(t, h, c, http.MethodGet, "/api/users", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}

	var list struct {
		Users []adminUserResponse `json:"users"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("応答が JSON でない: %v", err)
	}

	// 卒業者を消さない方針のため、無効化された人も一覧に出る。
	var found bool
	for _, u := range list.Users {
		if u.LoginID == "tanaka" {
			found = true
			if u.IsActive {
				t.Error("無効化が反映されていない")
			}
		}
	}
	if !found {
		t.Error("無効化された人が一覧から消えている")
	}

	// ハッシュが出ていないこと。
	if strings.Contains(w.Body.String(), "$2") {
		t.Error("ハッシュが一覧に出ている")
	}
}

func TestHandleSetActive_無効化と再有効化ができる(t *testing.T) {
	h, store := newTestHandler(t)
	c := adminSession(t, h, store)

	created := callAPI(t, h, c, http.MethodPost, "/api/users",
		`{"name":"田中花子","login_id":"tanaka"}`)
	var got userWithPasswordResponse
	if err := json.Unmarshal(created.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答が JSON でない: %v", err)
	}

	// 無効化するとログインできなくなること。
	deactivate := fmt.Sprintf("/api/users/%d/deactivate", got.User.ID)
	if w := callAPI(t, h, c, http.MethodPost, deactivate, ""); w.Code != http.StatusOK {
		t.Fatalf("無効化: %d %s", w.Code, w.Body.String())
	}

	login := postLogin(t, h, fmt.Sprintf(
		`{"login_id":"tanaka","password":%q}`, got.InitialPassword))
	if login.Code != http.StatusUnauthorized {
		t.Errorf("無効化後もログインできる: %d", login.Code)
	}

	// 戻せること。間違えて無効化した時に直せないと運用が止まる。
	activate := fmt.Sprintf("/api/users/%d/activate", got.User.ID)
	if w := callAPI(t, h, c, http.MethodPost, activate, ""); w.Code != http.StatusOK {
		t.Fatalf("再有効化: %d %s", w.Code, w.Body.String())
	}

	login = postLogin(t, h, fmt.Sprintf(
		`{"login_id":"tanaka","password":%q}`, got.InitialPassword))
	if login.Code != http.StatusOK {
		t.Errorf("再有効化後にログインできない: %d %s", login.Code, login.Body.String())
	}
}

// 最後の admin を無効化させないこと。
// 全員無効になると、Webからユーザーを操作する手段が無くなる。
func TestHandleSetActive_最後のadminは無効化できない(t *testing.T) {
	h, store := newTestHandler(t)
	c := adminSession(t, h, store)

	me := callAPI(t, h, c, http.MethodGet, "/api/me", "")
	var meResp loginResponse
	if err := json.Unmarshal(me.Body.Bytes(), &meResp); err != nil {
		t.Fatalf("応答が JSON でない: %v", err)
	}

	path := fmt.Sprintf("/api/users/%d/deactivate", meResp.User.ID)
	w := callAPI(t, h, c, http.MethodPost, path, "")

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d。409 を期待。body = %s", w.Code, w.Body.String())
	}

	// まだログインできること。
	if got := callAPI(t, h, c, http.MethodGet, "/api/me", ""); got.Code != http.StatusOK {
		t.Errorf("自分を締め出している: %d", got.Code)
	}
}

// admin が2人いれば片方は無効化できること。
func TestHandleSetActive_adminが複数なら無効化できる(t *testing.T) {
	h, store := newTestHandler(t)
	c := adminSession(t, h, store)

	created := callAPI(t, h, c, http.MethodPost, "/api/users",
		`{"name":"次の代","login_id":"nextboss","role":"admin"}`)
	var got userWithPasswordResponse
	if err := json.Unmarshal(created.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答が JSON でない: %v", err)
	}

	path := fmt.Sprintf("/api/users/%d/deactivate", got.User.ID)
	if w := callAPI(t, h, c, http.MethodPost, path, ""); w.Code != http.StatusOK {
		t.Errorf("status = %d。200 を期待。body = %s", w.Code, w.Body.String())
	}
}

func TestHandleResetPassword_新しいパスワードで入れる(t *testing.T) {
	h, store := newTestHandler(t)
	c := adminSession(t, h, store)

	created := callAPI(t, h, c, http.MethodPost, "/api/users",
		`{"name":"田中花子","login_id":"tanaka"}`)
	var got userWithPasswordResponse
	if err := json.Unmarshal(created.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答が JSON でない: %v", err)
	}

	path := fmt.Sprintf("/api/users/%d/reset-password", got.User.ID)
	w := callAPI(t, h, c, http.MethodPost, path, "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var reset userWithPasswordResponse
	if err := json.Unmarshal(w.Body.Bytes(), &reset); err != nil {
		t.Fatalf("応答が JSON でない: %v", err)
	}
	if reset.InitialPassword == "" {
		t.Fatal("新しいパスワードが返っていない")
	}
	if reset.InitialPassword == got.InitialPassword {
		t.Error("同じパスワードが再発行されている")
	}
	// 再発行された文字列も口頭やチャットで渡る。使い続けさせない。
	if !reset.User.MustChangePassword {
		t.Error("must_change_password が立っていない")
	}

	if old := postLogin(t, h, fmt.Sprintf(
		`{"login_id":"tanaka","password":%q}`, got.InitialPassword)); old.Code != http.StatusUnauthorized {
		t.Errorf("古いパスワードで入れる: %d", old.Code)
	}
	if now := postLogin(t, h, fmt.Sprintf(
		`{"login_id":"tanaka","password":%q}`, reset.InitialPassword)); now.Code != http.StatusOK {
		t.Errorf("再発行されたパスワードで入れない: %d %s", now.Code, now.Body.String())
	}
}

// 再発行の理由には「他人に知られた」も含まれる。古いセッションを残さない。
func TestHandleResetPassword_既存のセッションを切る(t *testing.T) {
	h, store := newTestHandler(t)
	admin := adminSession(t, h, store)

	victim := loginAs(t, h, store, "tanaka", RoleMember)

	var target adminUserResponse
	list := callAPI(t, h, admin, http.MethodGet, "/api/users", "")
	var users struct {
		Users []adminUserResponse `json:"users"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &users); err != nil {
		t.Fatalf("応答が JSON でない: %v", err)
	}
	for _, u := range users.Users {
		if u.LoginID == "tanaka" {
			target = u
		}
	}

	path := fmt.Sprintf("/api/users/%d/reset-password", target.ID)
	if w := callAPI(t, h, admin, http.MethodPost, path, ""); w.Code != http.StatusOK {
		t.Fatalf("再発行: %d %s", w.Code, w.Body.String())
	}

	if got := callAPI(t, h, victim, http.MethodGet, "/api/me", ""); got.Code != http.StatusUnauthorized {
		t.Errorf("再発行後も古いセッションが生きている: %d", got.Code)
	}
}

func TestUserRoutes_不正なIDは400(t *testing.T) {
	h, store := newTestHandler(t)
	c := adminSession(t, h, store)

	for _, path := range []string{
		"/api/users/abc/deactivate",
		"/api/users/0/reset-password",
		"/api/users/-1/activate",
	} {
		t.Run(path, func(t *testing.T) {
			w := callAPI(t, h, c, http.MethodPost, path, "")
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d。400 を期待", w.Code)
			}
		})
	}
}

func TestUserRoutes_居ない利用者は404(t *testing.T) {
	h, store := newTestHandler(t)
	c := adminSession(t, h, store)

	w := callAPI(t, h, c, http.MethodPost, "/api/users/9999/reset-password", "")
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d。404 を期待", w.Code)
	}
}

func TestSetActive_最後のadminはStoreでも止まる(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	in := validUser()
	in.LoginID = "boss"
	in.Role = RoleAdmin
	admin, err := store.Create(ctx, in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// member を何人作っても、admin が1人なら止まること。
	member := validUser()
	member.LoginID = "member1"
	if _, err := store.Create(ctx, member); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := store.SetActive(ctx, admin.ID, false); !errors.Is(err, ErrLastAdmin) {
		t.Errorf("err = %v。ErrLastAdmin を期待", err)
	}
}
