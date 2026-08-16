package item

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// send は本文付きのリクエストを流す。
func send(t *testing.T, h *Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()

	mux := http.NewServeMux()
	h.Register(mux)

	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	return w
}

// decodeItem は1件の応答を読む。
func decodeItem(t *testing.T, w *httptest.ResponseRecorder) itemResponse {
	t.Helper()

	var got struct {
		Item itemResponse `json:"item"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答が JSON でない: %v (%s)", err, w.Body.String())
	}
	return got.Item
}

func TestHandleCreate_登録するとコードが採番される(t *testing.T) {
	h, _ := newTestHandler(t)

	w := send(t, h, http.MethodPost, "/api/items",
		`{"name":"三脚","category":"撮影機材","location":"棚A","model":"Manfrotto 190"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	it := decodeItem(t, w)
	if it.Code != "0001" {
		t.Errorf("Code = %q。0001 を期待", it.Code)
	}
	if it.Name != "三脚" || it.Category != "撮影機材" {
		t.Errorf("内容が違う: %+v", it)
	}

	// 2件目は 0002。
	w = send(t, h, http.MethodPost, "/api/items", `{"name":"カメラ"}`)
	if got := decodeItem(t, w); got.Code != "0002" {
		t.Errorf("Code = %q。0002 を期待", got.Code)
	}
}

// 空欄はDBの既定値と同じ値になること。
// CSVインポートと結果が食い違わないようにする。
func TestHandleCreate_空欄は既定値になる(t *testing.T) {
	h, _ := newTestHandler(t)

	w := send(t, h, http.MethodPost, "/api/items", `{"name":"三脚"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	it := decodeItem(t, w)
	if it.Category != "未分類" || it.Owner != OwnerCircle ||
		it.Condition != ConditionGood || it.LocationStatus != LocationInStock {
		t.Errorf("既定値が入っていない: %+v", it)
	}
}

// コードは採番するので、指定させない。
func TestHandleCreate_コードの指定を拒否する(t *testing.T) {
	h, _ := newTestHandler(t)

	w := send(t, h, http.MethodPost, "/api/items", `{"code":"0099","name":"三脚"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d。400 を期待", w.Code)
	}
	if !strings.Contains(w.Body.String(), "自動") {
		t.Errorf("理由が分からない: %s", w.Body.String())
	}
}

func TestHandleCreate_入力の誤りは400(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"品名が空", `{"name":""}`},
		{"品名が空白のみ", `{"name":"   "}`},
		{"所有が不正", `{"name":"三脚","owner":"だれか"}`},
		{"状態が不正", `{"name":"三脚","condition":"こわれた"}`},
		{"所在が不正", `{"name":"三脚","location_status":"どこか"}`},
		{"知らない項目", `{"name":"三脚","price":1000}`},
	}

	h, s := newTestHandler(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := send(t, h, http.MethodPost, "/api/items", tt.body)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d。400 を期待。body = %s", w.Code, w.Body.String())
			}
		})
	}

	// 1件も入っていないこと。
	items, err := s.List(context.Background(), Filter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("%d件登録されている", len(items))
	}
}

func TestHandleUpdate_内容を差し替える(t *testing.T) {
	h, _ := newTestHandler(t)

	created := send(t, h, http.MethodPost, "/api/items",
		`{"name":"三脚","category":"撮影機材","location":"棚A","note":"脚のロックが緩い"}`)
	code := decodeItem(t, created).Code

	w := send(t, h, http.MethodPut, "/api/items/"+code,
		`{"code":"`+code+`","name":"三脚（大）","category":"撮影機材","location":"棚B","note":""}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	it := decodeItem(t, w)
	if it.Name != "三脚（大）" || it.Location != "棚B" {
		t.Errorf("更新されていない: %+v", it)
	}
	// 一部だけ送る形にしていないので、空にした備考は消える。
	if it.Note != "" {
		t.Errorf("消したはずの備考が残っている: %q", it.Note)
	}
	// コードは変わらない。
	if it.Code != code {
		t.Errorf("Code = %q。%q のままを期待", it.Code, code)
	}
}

// 備品コードは変更できない。変えられると、貼ってあるラベルと実物の対応が壊れる。
func TestHandleUpdate_コードの変更を拒否する(t *testing.T) {
	h, _ := newTestHandler(t)

	created := send(t, h, http.MethodPost, "/api/items", `{"name":"三脚"}`)
	code := decodeItem(t, created).Code

	w := send(t, h, http.MethodPut, "/api/items/"+code,
		`{"code":"9999","name":"三脚"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d。400 を期待", w.Code)
	}
	if !strings.Contains(w.Body.String(), "変更できません") {
		t.Errorf("理由が分からない: %s", w.Body.String())
	}
}

func TestHandleUpdate_未登録は404(t *testing.T) {
	h, _ := newTestHandler(t)

	w := send(t, h, http.MethodPut, "/api/items/9999", `{"name":"三脚"}`)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d。404 を期待", w.Code)
	}
}

// 廃棄は状態であって削除ではない。行は残る。
func TestHandleDiscard_行を消さずに状態を変える(t *testing.T) {
	h, s := newTestHandler(t)

	created := send(t, h, http.MethodPost, "/api/items", `{"name":"壊れたカメラ"}`)
	code := decodeItem(t, created).Code

	w := send(t, h, http.MethodPost, "/api/items/"+code+"/discard", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if got := decodeItem(t, w); got.Condition != ConditionDiscarded {
		t.Errorf("Condition = %q。廃棄を期待", got.Condition)
	}

	// 行が残っていること。消すと貸出履歴の参照先が消える。
	ctx := context.Background()
	if _, err := s.ByCode(ctx, code); err != nil {
		t.Errorf("行が消えている: %v", err)
	}

	// 一覧の既定からは外れること。
	items, err := s.List(ctx, Filter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("廃棄が一覧の既定に出ている: %d件", len(items))
	}

	// コードは空かない。次の登録は 0002 になる。
	next := send(t, h, http.MethodPost, "/api/items", `{"name":"新しいカメラ"}`)
	if got := decodeItem(t, next); got.Code != "0002" {
		t.Errorf("Code = %q。0002 を期待（廃棄したコードを再利用している）", got.Code)
	}
}

func TestHandleDiscard_未登録は404(t *testing.T) {
	h, _ := newTestHandler(t)

	w := send(t, h, http.MethodPost, "/api/items/9999/discard", "")
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d。404 を期待", w.Code)
	}
}

// DELETE は用意しない。物理削除すると貸出履歴の参照先が消える。
func TestRegister_DELETEを受け付けない(t *testing.T) {
	h, _ := newTestHandler(t)

	created := send(t, h, http.MethodPost, "/api/items", `{"name":"三脚"}`)
	code := decodeItem(t, created).Code

	w := send(t, h, http.MethodDelete, "/api/items/"+code, "")
	if w.Code == http.StatusOK || w.Code == http.StatusNoContent {
		t.Errorf("DELETE が通ってしまう: status = %d", w.Code)
	}
}

func TestStoreCreate_同じ内容でも別のコードになる(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	a, err := s.Create(ctx, Attributes{Name: "三脚"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	b, err := s.Create(ctx, Attributes{Name: "三脚"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if a.Code == b.Code {
		t.Errorf("同じコードが振られた: %s", a.Code)
	}
}

func TestStoreUpdate_未登録はErrNotFound(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	if _, err := s.Update(ctx, "9999", Attributes{Name: "三脚"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v。ErrNotFound を期待", err)
	}
	if _, err := s.Discard(ctx, "9999"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v。ErrNotFound を期待", err)
	}
}
