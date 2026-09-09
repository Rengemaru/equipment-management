package loan

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Rengemaru/equipment-management/internal/item"
	"github.com/Rengemaru/equipment-management/internal/jst"
)

// passthrough は認証を通す代わりのミドルウェア。
// 認証そのものは auth パッケージのテストで確かめている。
func passthrough(next http.Handler) http.Handler { return next }

// newTestHandler は Handler と Store、操作している利用者のIDを返す。
func newTestHandler(t *testing.T) (*Handler, *Store, int64) {
	t.Helper()

	s, userID := fixture(t)
	currentUser := func(context.Context) (int64, bool) { return userID, true }

	h := NewHandler(s, item.NewStore(s.sqldb), currentUser, passthrough)
	return h, s, userID
}

// post は経路にリクエストを流す。body が空文字なら本文なしで送る。
func post(t *testing.T, h *Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()

	mux := http.NewServeMux()
	h.Register(mux)

	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(http.MethodPost, path, nil)
	} else {
		r = httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	return w
}

// decodeBorrow は借用の応答を読む。
func decodeBorrow(t *testing.T, w *httptest.ResponseRecorder) (loanResponse, item.Response) {
	t.Helper()

	var got struct {
		Loan loanResponse  `json:"loan"`
		Item item.Response `json:"item"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答が JSON でない: %v (%s)", err, w.Body.String())
	}
	return got.Loan, got.Item
}

// decodeError はエラーの応答から code を読む。
func decodeError(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()

	var got struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答が JSON でない: %v (%s)", err, w.Body.String())
	}
	if got.Error == "" {
		t.Errorf("エラーの文言が空: %s", w.Body.String())
	}
	return got.Code
}

func TestHandleBorrow_空のJSONで借用できる(t *testing.T) {
	h, _, userID := newTestHandler(t)

	w := post(t, h, "/api/items/0001/loans", `{}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (%s)", w.Code, w.Body.String())
	}

	l, it := decodeBorrow(t, w)
	if l.ID == 0 {
		t.Error("貸出IDが返っていない")
	}
	if l.User.ID != userID {
		t.Errorf("借用者 = %d, want %d", l.User.ID, userID)
	}
	if l.IsProxy {
		t.Error("本人の借用が代理登録になっている")
	}
	if l.DueDate == "" {
		t.Error("返却予定日が入っていない")
	}
	if l.ReturnedAt != nil {
		t.Error("登録直後に返却済みになっている")
	}
	// 借用日時は new Date() に渡せる形（RFC3339）で返す。
	if !strings.Contains(l.BorrowedAt, "T") || !strings.HasSuffix(l.BorrowedAt, "+09:00") {
		t.Errorf("借用日時 = %q, want RFC3339（JST）", l.BorrowedAt)
	}

	// 更新後の備品も返す。画面が再取得せずに反映できる。
	if it.Code != "0001" {
		t.Errorf("備品コード = %q, want 0001", it.Code)
	}
}

// 本文なしでも借用できる。3タップで終わらせるため、画面側に本文を組ませない。
func TestHandleBorrow_本文なしで借用できる(t *testing.T) {
	h, _, _ := newTestHandler(t)

	w := post(t, h, "/api/items/0001/loans", "")
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (%s)", w.Code, w.Body.String())
	}
}

func TestHandleBorrow_自由利用品は400(t *testing.T) {
	h, s, _ := newTestHandler(t)
	insertItem(t, s, "0002", "はさみ", map[string]any{"is_free_use": 1})

	w := post(t, h, "/api/items/0002/loans", `{}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", w.Code, w.Body.String())
	}
	if code := decodeError(t, w); code != "free_use" {
		t.Errorf("code = %q, want free_use", code)
	}
}

func TestHandleBorrow_廃棄済みは400(t *testing.T) {
	h, s, _ := newTestHandler(t)
	insertItem(t, s, "0002", "壊れた三脚", map[string]any{"condition": "廃棄"})

	w := post(t, h, "/api/items/0002/loans", `{}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", w.Code, w.Body.String())
	}
	if code := decodeError(t, w); code != "discarded" {
		t.Errorf("code = %q, want discarded", code)
	}
}

// 競合は異常ではない。500 にしない。
func TestHandleBorrow_二重貸出は409(t *testing.T) {
	h, _, _ := newTestHandler(t)

	if w := post(t, h, "/api/items/0001/loans", `{}`); w.Code != http.StatusCreated {
		t.Fatalf("1回目: status = %d, want 201 (%s)", w.Code, w.Body.String())
	}

	w := post(t, h, "/api/items/0001/loans", `{}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (%s)", w.Code, w.Body.String())
	}
	if code := decodeError(t, w); code != "already_borrowed" {
		t.Errorf("code = %q, want already_borrowed", code)
	}
}

func TestHandleBorrow_知らない備品は404(t *testing.T) {
	h, _, _ := newTestHandler(t)

	w := post(t, h, "/api/items/9999/loans", `{}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (%s)", w.Code, w.Body.String())
	}
}

func TestHandleBorrow_無効な利用者は400(t *testing.T) {
	h, s, _ := newTestHandler(t)
	graduated := insertUser(t, s, "卒業生", false)

	w := post(t, h, "/api/items/0001/loans", `{"user_id":`+strconv.FormatInt(graduated, 10)+`}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", w.Code, w.Body.String())
	}
	if code := decodeError(t, w); code != "invalid_user" {
		t.Errorf("code = %q, want invalid_user", code)
	}
}

func TestHandleBorrow_返却予定日が借用日より前なら400(t *testing.T) {
	h, _, _ := newTestHandler(t)

	freezeNow(t, time.Date(2026, 9, 10, 18, 0, 0, 0, jst.Zone))

	body := `{"borrowed_at":"2026-09-10T12:00:00+09:00","due_date":"2026-09-09"}`
	w := post(t, h, "/api/items/0001/loans", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", w.Code, w.Body.String())
	}
	if code := decodeError(t, w); code != "invalid_due_date" {
		t.Errorf("code = %q, want invalid_due_date", code)
	}
}

func TestHandleBorrow_未来の借用日時は400(t *testing.T) {
	h, _, _ := newTestHandler(t)

	w := post(t, h, "/api/items/0001/loans", `{"borrowed_at":"2099-01-01T00:00:00+09:00"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", w.Code, w.Body.String())
	}
	if code := decodeError(t, w); code != "future_borrowed_at" {
		t.Errorf("code = %q, want future_borrowed_at", code)
	}
}

func TestHandleBorrow_代理登録は借用者と登録者を分けて返す(t *testing.T) {
	h, s, registrar := newTestHandler(t)
	borrower := insertUser(t, s, "佐藤", true)

	w := post(t, h, "/api/items/0001/loans", `{"user_id":`+strconv.FormatInt(borrower, 10)+`}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (%s)", w.Code, w.Body.String())
	}

	l, _ := decodeBorrow(t, w)
	if l.User.ID != borrower {
		t.Errorf("借用者 = %d, want %d", l.User.ID, borrower)
	}
	if l.RegisteredBy.ID != registrar {
		t.Errorf("登録者 = %d, want %d", l.RegisteredBy.ID, registrar)
	}
	// 画面に2つのIDを比べさせない。
	if !l.IsProxy {
		t.Error("is_proxy が false")
	}
}

// 綴り違いを黙って無視しない。フロントもこのリポジトリで書くため、早く気付ける方がよい。
func TestHandleBorrow_知らない項目は400(t *testing.T) {
	h, _, _ := newTestHandler(t)

	w := post(t, h, "/api/items/0001/loans", `{"due":"2026-09-24"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", w.Code, w.Body.String())
	}
}
