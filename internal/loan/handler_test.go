package loan

import (
	"context"
	"encoding/json"
	"errors"
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

// sentMail は送信したメールを1通ぶん保持する。
type sentMail struct {
	to      []string
	subject string
	body    string
}

// testHandler はテスト用の Handler と、その周辺。
type testHandler struct {
	*Handler
	store *Store

	// actor は操作している利用者。テストの途中で差し替えられる。
	actor Actor

	// sent は送ったメール。1通も送っていなければ空。
	sent []sentMail

	// notifyErr を入れると送信が失敗する。
	notifyErr error
}

const testHostURL = "https://example.test"

// newTestHandler は Handler と Store、操作している利用者のIDを返す。
func newTestHandler(t *testing.T) (*testHandler, *Store, int64) {
	t.Helper()

	s, userID := fixture(t)
	th := &testHandler{store: s, actor: Actor{ID: userID}}

	notify := func(_ context.Context, to []string, subject, body string) error {
		th.sent = append(th.sent, sentMail{to: to, subject: subject, body: body})
		return th.notifyErr
	}
	currentUser := func(context.Context) (Actor, bool) { return th.actor, true }

	th.Handler = NewHandler(s, item.NewStore(s.sqldb), notify, testHostURL, currentUser, passthrough)
	return th, s, userID
}

// post は経路にリクエストを流す。body が空文字なら本文なしで送る。
func post(t *testing.T, h *testHandler, path, body string) *httptest.ResponseRecorder {
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

// 応答の item は更新後の姿。画面が再取得せずに反映できる。
func TestHandleBorrow_所在不明から戻った備品を応答に載せる(t *testing.T) {
	h, s, _ := newTestHandler(t)
	insertItem(t, s, "0002", "行方不明の三脚", map[string]any{"location_status": "所在不明_未確認"})

	w := post(t, h, "/api/items/0002/loans", `{}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (%s)", w.Code, w.Body.String())
	}

	_, it := decodeBorrow(t, w)
	if string(it.LocationStatus) != "在庫" {
		t.Errorf("location_status = %q, want 在庫", it.LocationStatus)
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

func TestHandleReturn_返却できる(t *testing.T) {
	h, s, userID := newTestHandler(t)
	if _, err := s.Borrow(context.Background(), "0001", userID, Request{}); err != nil {
		t.Fatalf("Borrow: %v", err)
	}

	w := post(t, h, "/api/items/0001/return", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", w.Code, w.Body.String())
	}

	l, it := decodeBorrow(t, w)
	if l.ReturnedAt == nil {
		t.Error("returned_at が返っていない")
	}
	if l.ReturnedBy == nil || l.ReturnedBy.ID != userID {
		t.Errorf("returned_by = %v, want %d", l.ReturnedBy, userID)
	}
	if it.Code != "0001" {
		t.Errorf("備品コード = %q, want 0001", it.Code)
	}
}

// 棚に戻っているのを見つけた人が押せること。
func TestHandleReturn_借用者以外でも返却できる(t *testing.T) {
	h, s, finder := newTestHandler(t)
	borrower := insertUser(t, s, "佐藤", true)
	if _, err := s.Borrow(context.Background(), "0001", borrower, Request{}); err != nil {
		t.Fatalf("Borrow: %v", err)
	}

	w := post(t, h, "/api/items/0001/return", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", w.Code, w.Body.String())
	}

	l, _ := decodeBorrow(t, w)
	if l.User.ID != borrower {
		t.Errorf("借用者 = %d, want %d", l.User.ID, borrower)
	}
	if l.ReturnedBy == nil || l.ReturnedBy.ID != finder {
		t.Errorf("returned_by = %v, want %d", l.ReturnedBy, finder)
	}
}

// 二重タップを 200 で黙って成功にしない。
func TestHandleReturn_貸出中でなければ409(t *testing.T) {
	h, _, _ := newTestHandler(t)

	w := post(t, h, "/api/items/0001/return", "")
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (%s)", w.Code, w.Body.String())
	}
	if code := decodeError(t, w); code != "not_borrowed" {
		t.Errorf("code = %q, want not_borrowed", code)
	}
}

func TestHandleReturn_知らない備品は404(t *testing.T) {
	h, _, _ := newTestHandler(t)

	w := post(t, h, "/api/items/9999/return", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (%s)", w.Code, w.Body.String())
	}
}

// 代理登録は本人に知らせる。「本人が記録しなくても穴が埋まる」ための仕組みで、
// 誤登録に気付ける導線が無いと成立しない。
func TestHandleBorrow_代理登録は本人にメールを送る(t *testing.T) {
	h, s, _ := newTestHandler(t)
	borrower := insertUser(t, s, "佐藤", true)
	if _, err := s.sqldb.Exec(`UPDATE users SET email = 'sato@example.test' WHERE id = ?`, borrower); err != nil {
		t.Fatalf("メールアドレスの設定: %v", err)
	}

	w := post(t, h, "/api/items/0001/loans", `{"user_id":`+strconv.FormatInt(borrower, 10)+`}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (%s)", w.Code, w.Body.String())
	}

	if len(h.sent) != 1 {
		t.Fatalf("送ったメール = %d通, want 1", len(h.sent))
	}
	m := h.sent[0]
	if len(m.to) != 1 || m.to[0] != "sato@example.test" {
		t.Errorf("宛先 = %v, want [sato@example.test]", m.to)
	}
	if !strings.Contains(m.subject, "借用が登録されました") {
		t.Errorf("件名 = %q", m.subject)
	}
	// 訂正できる導線を必ず載せる。載せないと、気付いても直せない。
	if !strings.Contains(m.body, testHostURL+"/loans/mine") {
		t.Errorf("本文に訂正先URLが無い: %q", m.body)
	}
	if !strings.Contains(m.body, "0001") || !strings.Contains(m.body, "三脚（大）") {
		t.Errorf("本文に備品が無い: %q", m.body)
	}
	// 誰が登録したかを書く。書かないと本人が誰に確認すればよいか分からない。
	if !strings.Contains(m.body, "山田") {
		t.Errorf("本文に登録者が無い: %q", m.body)
	}
}

func TestHandleBorrow_自分の借用では送らない(t *testing.T) {
	h, s, userID := newTestHandler(t)
	if _, err := s.sqldb.Exec(`UPDATE users SET email = 'yamada@example.test' WHERE id = ?`, userID); err != nil {
		t.Fatalf("メールアドレスの設定: %v", err)
	}

	if w := post(t, h, "/api/items/0001/loans", `{}`); w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (%s)", w.Code, w.Body.String())
	}
	if len(h.sent) != 0 {
		t.Errorf("送ったメール = %d通, want 0", len(h.sent))
	}
}

// メールアドレスは任意項目。未設定の人が居ることは異常ではない。
func TestHandleBorrow_宛先が無くても借用は成立する(t *testing.T) {
	h, s, _ := newTestHandler(t)
	borrower := insertUser(t, s, "佐藤", true)

	w := post(t, h, "/api/items/0001/loans", `{"user_id":`+strconv.FormatInt(borrower, 10)+`}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (%s)", w.Code, w.Body.String())
	}
	if len(h.sent) != 0 {
		t.Errorf("送ったメール = %d通, want 0", len(h.sent))
	}
}

// 通知が送れなかったことを理由に記録を消すと、最も避けたい「記録が無い」状態に戻る。
func TestHandleBorrow_通知に失敗しても借用は成立する(t *testing.T) {
	h, s, _ := newTestHandler(t)
	borrower := insertUser(t, s, "佐藤", true)
	if _, err := s.sqldb.Exec(`UPDATE users SET email = 'sato@example.test' WHERE id = ?`, borrower); err != nil {
		t.Fatalf("メールアドレスの設定: %v", err)
	}
	h.notifyErr = errors.New("SMTP に繋がらない")

	w := post(t, h, "/api/items/0001/loans", `{"user_id":`+strconv.FormatInt(borrower, 10)+`}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (%s)", w.Code, w.Body.String())
	}

	// 記録は残っている。
	if _, err := s.Return(context.Background(), "0001", borrower); err != nil {
		t.Errorf("借用が記録されていない: %v", err)
	}
}

func TestHandleCancel_本人が取り消せる(t *testing.T) {
	h, s, registrar := newTestHandler(t)
	borrower := insertUser(t, s, "佐藤", true)

	l, err := s.Borrow(context.Background(), "0001", registrar, Request{UserID: borrower})
	if err != nil {
		t.Fatalf("Borrow: %v", err)
	}

	// 借用者本人として取り消す。
	h.actor = Actor{ID: borrower}
	w := post(t, h, "/api/loans/"+strconv.FormatInt(l.ID, 10)+"/cancel", `{"reason":"自分は借りていない"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", w.Code, w.Body.String())
	}

	// 取り消した備品は再度借りられる。
	if _, err := s.Borrow(context.Background(), "0001", registrar, Request{}); err != nil {
		t.Errorf("取り消し後の Borrow: %v", err)
	}
}

func TestHandleCancel_無関係の利用者は403(t *testing.T) {
	h, s, registrar := newTestHandler(t)
	borrower := insertUser(t, s, "佐藤", true)
	stranger := insertUser(t, s, "田中", true)

	l, err := s.Borrow(context.Background(), "0001", registrar, Request{UserID: borrower})
	if err != nil {
		t.Fatalf("Borrow: %v", err)
	}

	h.actor = Actor{ID: stranger}
	w := post(t, h, "/api/loans/"+strconv.FormatInt(l.ID, 10)+"/cancel", "")
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (%s)", w.Code, w.Body.String())
	}
}

func TestHandleCancel_返却済みは409(t *testing.T) {
	h, s, userID := newTestHandler(t)
	ctx := context.Background()

	l, err := s.Borrow(ctx, "0001", userID, Request{})
	if err != nil {
		t.Fatalf("Borrow: %v", err)
	}
	if _, err := s.Return(ctx, "0001", userID); err != nil {
		t.Fatalf("Return: %v", err)
	}

	w := post(t, h, "/api/loans/"+strconv.FormatInt(l.ID, 10)+"/cancel", "")
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (%s)", w.Code, w.Body.String())
	}
	if code := decodeError(t, w); code != "already_returned" {
		t.Errorf("code = %q, want already_returned", code)
	}
}

func TestHandleCancel_知らない貸出は404(t *testing.T) {
	h, _, _ := newTestHandler(t)

	if w := post(t, h, "/api/loans/999/cancel", ""); w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (%s)", w.Code, w.Body.String())
	}
	// 数値でないIDも 404。500 にしない。
	if w := post(t, h, "/api/loans/abc/cancel", ""); w.Code != http.StatusNotFound {
		t.Fatalf("数値でないID: status = %d, want 404 (%s)", w.Code, w.Body.String())
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

func TestHandleList_貸出中だけを返す(t *testing.T) {
	h, s, userID := newTestHandler(t)
	insertItem(t, s, "0002", "ドライバー", nil)
	ctx := context.Background()

	if _, err := s.Borrow(ctx, "0001", userID, Request{}); err != nil {
		t.Fatalf("Borrow: %v", err)
	}
	if _, err := s.Borrow(ctx, "0002", userID, Request{}); err != nil {
		t.Fatalf("Borrow: %v", err)
	}
	if _, err := s.Return(ctx, "0002", userID); err != nil {
		t.Fatalf("Return: %v", err)
	}

	loans := decodeLoans(t, get(t, h, "/api/loans"), "loans")
	if len(loans) != 1 {
		t.Fatalf("貸出中 = %d件, want 1", len(loans))
	}
	if loans[0].Item.Code != "0001" {
		t.Errorf("備品 = %q, want 0001", loans[0].Item.Code)
	}
}

// 取り消し済みを一覧に出すと、誤登録が「起きた事実」として全員に見え続ける。
func TestHandleList_取り消し済みは出さない(t *testing.T) {
	h, s, userID := newTestHandler(t)

	l, err := s.Borrow(context.Background(), "0001", userID, Request{})
	if err != nil {
		t.Fatalf("Borrow: %v", err)
	}
	if _, err := s.Cancel(context.Background(), l.ID, Actor{ID: userID}, ""); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	if loans := decodeLoans(t, get(t, h, "/api/loans"), "loans"); len(loans) != 0 {
		t.Errorf("貸出中 = %d件, want 0", len(loans))
	}
}

func TestHandleList_期限超過で絞れる(t *testing.T) {
	h, s, userID := newTestHandler(t)
	insertItem(t, s, "0002", "ドライバー", nil)
	freezeNow(t, time.Date(2026, 9, 10, 12, 0, 0, 0, jst.Zone))
	ctx := context.Background()

	// 期限を過ぎた貸出は、借用日も過去でないと作れない（返却予定日は借用日以降）。
	past := time.Date(2026, 9, 1, 12, 0, 0, 0, jst.Zone)
	if _, err := s.Borrow(ctx, "0001", userID, Request{BorrowedAt: past, DueDate: "2026-09-09"}); err != nil {
		t.Fatalf("Borrow: %v", err)
	}
	if _, err := s.Borrow(ctx, "0002", userID, Request{DueDate: "2026-09-30"}); err != nil {
		t.Fatalf("Borrow: %v", err)
	}

	loans := decodeLoans(t, get(t, h, "/api/loans?overdue=1"), "loans")
	if len(loans) != 1 {
		t.Fatalf("超過 = %d件, want 1", len(loans))
	}
	if loans[0].Item.Code != "0001" {
		t.Errorf("備品 = %q, want 0001", loans[0].Item.Code)
	}
	if loans[0].OverdueDays != 1 {
		t.Errorf("超過日数 = %d, want 1", loans[0].OverdueDays)
	}
}

func TestHandleList_借用者と語句で絞れる(t *testing.T) {
	h, s, userID := newTestHandler(t)
	other := insertUser(t, s, "佐藤", true)
	insertItem(t, s, "0002", "ドライバー", nil)
	ctx := context.Background()

	if _, err := s.Borrow(ctx, "0001", userID, Request{}); err != nil {
		t.Fatalf("Borrow: %v", err)
	}
	if _, err := s.Borrow(ctx, "0002", other, Request{}); err != nil {
		t.Fatalf("Borrow: %v", err)
	}

	byUser := decodeLoans(t, get(t, h, "/api/loans?user_id="+strconv.FormatInt(other, 10)), "loans")
	if len(byUser) != 1 || byUser[0].User.ID != other {
		t.Fatalf("借用者で絞れていない: %+v", byUser)
	}

	byQuery := decodeLoans(t, get(t, h, "/api/loans?q=ドライバー"), "loans")
	if len(byQuery) != 1 || byQuery[0].Item.Code != "0002" {
		t.Fatalf("語句で絞れていない: %+v", byQuery)
	}

	byName := decodeLoans(t, get(t, h, "/api/loans?q=佐藤"), "loans")
	if len(byName) != 1 || byName[0].User.ID != other {
		t.Fatalf("借用者名で絞れていない: %+v", byName)
	}
}

func TestHandleMine_貸出中と履歴を1回で返す(t *testing.T) {
	h, s, userID := newTestHandler(t)
	other := insertUser(t, s, "佐藤", true)
	insertItem(t, s, "0002", "ドライバー", nil)
	insertItem(t, s, "0003", "脚立", nil)
	ctx := context.Background()

	if _, err := s.Borrow(ctx, "0001", userID, Request{}); err != nil {
		t.Fatalf("Borrow: %v", err)
	}
	if _, err := s.Borrow(ctx, "0002", userID, Request{}); err != nil {
		t.Fatalf("Borrow: %v", err)
	}
	if _, err := s.Return(ctx, "0002", userID); err != nil {
		t.Fatalf("Return: %v", err)
	}
	// 他人の貸出は出さない。
	if _, err := s.Borrow(ctx, "0003", other, Request{}); err != nil {
		t.Fatalf("Borrow: %v", err)
	}

	w := get(t, h, "/api/loans/mine")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", w.Code, w.Body.String())
	}

	active := decodeLoans(t, w, "active")
	if len(active) != 1 || active[0].Item.Code != "0001" {
		t.Errorf("貸出中 = %+v", active)
	}
	returned := decodeLoans(t, w, "returned")
	if len(returned) != 1 || returned[0].Item.Code != "0002" {
		t.Errorf("履歴 = %+v", returned)
	}
}

func TestHandleMine_limitの指定が不正なら400(t *testing.T) {
	h, _, _ := newTestHandler(t)

	if w := get(t, h, "/api/loans/mine?limit=0"); w.Code != http.StatusBadRequest {
		t.Errorf("limit=0: status = %d, want 400", w.Code)
	}
	if w := get(t, h, "/api/loans/mine?limit=abc"); w.Code != http.StatusBadRequest {
		t.Errorf("limit=abc: status = %d, want 400", w.Code)
	}
}

// get は経路にリクエストを流す。
func get(t *testing.T, h *testHandler, path string) *httptest.ResponseRecorder {
	t.Helper()

	mux := http.NewServeMux()
	h.Register(mux)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))

	return w
}

// decodeLoans は一覧の応答から key の配列を読む。
func decodeLoans(t *testing.T, w *httptest.ResponseRecorder, key string) []loanResponse {
	t.Helper()

	var got map[string][]loanResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答が JSON でない: %v (%s)", err, w.Body.String())
	}
	return got[key]
}
