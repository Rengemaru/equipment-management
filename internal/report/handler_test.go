package report

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/Rengemaru/equipment-management/internal/httpx"
	"github.com/Rengemaru/equipment-management/internal/item"
)

// passthrough は認証を通す代わりのミドルウェア。
// 認証そのものは auth パッケージのテストで確かめている。
func passthrough(next http.Handler) http.Handler { return next }

// denyAdmin は member が admin 限定の経路を叩いた時の代わり。
//
// **どの経路に requireAdmin が被さっているか**をこのパッケージで確かめるために使う。
// 並べ書きで片方だけ admin にし忘れた経路は、これで落ちる。
func denyAdmin(http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteError(w, http.StatusForbidden, "この操作には管理者権限が必要です")
	})
}

// newTestHandler は admin として叩ける Handler を返す。
func newTestHandler(t *testing.T) (*Handler, *Store, int64) {
	t.Helper()

	s, userID := fixture(t)
	currentUser := func(context.Context) (int64, bool) { return userID, true }

	h := NewHandler(s, item.NewStore(s.sqldb), currentUser, passthrough, passthrough)
	return h, s, userID
}

// newMemberHandler は member として叩く Handler を返す（admin 限定の経路は 403）。
func newMemberHandler(t *testing.T) (*Handler, *Store, int64) {
	t.Helper()

	s, userID := fixture(t)
	currentUser := func(context.Context) (int64, bool) { return userID, true }

	h := NewHandler(s, item.NewStore(s.sqldb), currentUser, passthrough, denyAdmin)
	return h, s, userID
}

func do(t *testing.T, h *Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()

	mux := http.NewServeMux()
	h.Register(mux)

	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	return w
}

func decodeReport(t *testing.T, w *httptest.ResponseRecorder) (damageResponse, item.Response) {
	t.Helper()

	var got struct {
		Report damageResponse `json:"report"`
		Item   item.Response  `json:"item"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答が JSON でない: %v (%s)", err, w.Body.String())
	}
	return got.Report, got.Item
}

func TestHandleReport_報告できる(t *testing.T) {
	h, _, userID := newTestHandler(t)

	w := do(t, h, http.MethodPost, "/api/items/0001/damages", `{"description":"脚のロックが割れている"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (%s)", w.Code, w.Body.String())
	}

	d, it := decodeReport(t, w)
	if d.Status != StatusUnconfirmed {
		t.Errorf("status = %q, want 未確認", d.Status)
	}
	if d.Reporter.ID != userID {
		t.Errorf("報告者 = %d, want %d", d.Reporter.ID, userID)
	}
	// 追認の前から画面に反映される。
	if string(it.Condition) != "要修理" {
		t.Errorf("condition = %q, want 要修理", it.Condition)
	}
}

func TestHandleReport_説明が空なら400(t *testing.T) {
	h, _, _ := newTestHandler(t)

	w := do(t, h, http.MethodPost, "/api/items/0001/damages", `{"description":""}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", w.Code, w.Body.String())
	}
}

func TestHandleReport_知らない備品は404(t *testing.T) {
	h, _, _ := newTestHandler(t)

	w := do(t, h, http.MethodPost, "/api/items/9999/damages", `{"description":"壊れた"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (%s)", w.Code, w.Body.String())
	}
}

// 報告は全員できる。報告のハードルを上げると、壊れたまま次の人が借りる。
func TestHandleReport_memberでも報告できる(t *testing.T) {
	h, _, _ := newMemberHandler(t)

	w := do(t, h, http.MethodPost, "/api/items/0001/damages", `{"description":"傷がある"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (%s)", w.Code, w.Body.String())
	}
}

func TestHandleList_未確認だけを引ける(t *testing.T) {
	h, s, userID := newTestHandler(t)
	ctx := context.Background()

	d, err := s.Report(ctx, "0001", userID, "脚が曲がった")
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if _, err := s.SetStatus(ctx, d.ID, userID, StatusRepaired, ""); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}

	w := do(t, h, http.MethodGet, "/api/damages?status=未確認", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", w.Code, w.Body.String())
	}

	var got struct {
		Reports []damageResponse `json:"reports"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答が JSON でない: %v", err)
	}
	if len(got.Reports) != 0 {
		t.Errorf("未確認 = %d件, want 0", len(got.Reports))
	}
}

func TestHandleSetStatus_追認できる(t *testing.T) {
	h, s, userID := newTestHandler(t)

	d, err := s.Report(context.Background(), "0001", userID, "脚が曲がった")
	if err != nil {
		t.Fatalf("Report: %v", err)
	}

	w := do(t, h, http.MethodPost, "/api/damages/"+strconv.FormatInt(d.ID, 10)+"/status",
		`{"status":"修理済み","note":"部品を交換した"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", w.Code, w.Body.String())
	}

	got, it := decodeReport(t, w)
	if got.Status != StatusRepaired {
		t.Errorf("status = %q, want 修理済み", got.Status)
	}
	if string(it.Condition) != "良好" {
		t.Errorf("condition = %q, want 良好", it.Condition)
	}
}

func TestHandleSetStatus_知らない報告は404(t *testing.T) {
	h, _, _ := newTestHandler(t)

	if w := do(t, h, http.MethodPost, "/api/damages/999/status", `{"status":"確認済み"}`); w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (%s)", w.Code, w.Body.String())
	}
	// 数値でないIDも 404。500 にしない。
	if w := do(t, h, http.MethodPost, "/api/damages/abc/status", `{"status":"確認済み"}`); w.Code != http.StatusNotFound {
		t.Fatalf("数値でないID: status = %d, want 404", w.Code)
	}
}

// **UIで隠すだけにしない。** 追認の経路が admin 限定であることを経路の側で確かめる。
func TestRegister_追認の経路はadmin限定(t *testing.T) {
	h, s, userID := newMemberHandler(t)

	d, err := s.Report(context.Background(), "0001", userID, "脚が曲がった")
	if err != nil {
		t.Fatalf("Report: %v", err)
	}

	if w := do(t, h, http.MethodGet, "/api/damages", ""); w.Code != http.StatusForbidden {
		t.Errorf("GET /api/damages: status = %d, want 403", w.Code)
	}

	w := do(t, h, http.MethodPost, "/api/damages/"+strconv.FormatInt(d.ID, 10)+"/status",
		`{"status":"修理済み"}`)
	if w.Code != http.StatusForbidden {
		t.Errorf("POST /api/damages/{id}/status: status = %d, want 403", w.Code)
	}

	// 状態も動いていないこと。
	if c := condition(t, s, "0001"); c != "要修理" {
		t.Errorf("condition = %q, want 要修理（member の追認は通らない）", c)
	}
}
