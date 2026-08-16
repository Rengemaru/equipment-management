package item

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// passthrough は認証を通す代わりのミドルウェア。
// 認証そのものは auth パッケージのテストで確かめている。
func passthrough(next http.Handler) http.Handler { return next }

// testHostURL は QR に埋め込む土台。末尾にスラッシュを含まない
// （config が検査済みの値が渡ってくる前提）。
const testHostURL = "https://example.test"

// newTestHandler は Handler と Store を返す。
func newTestHandler(t *testing.T) (*Handler, *Store) {
	t.Helper()

	s := newTestStore(t)
	photos, err := NewPhotoStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewPhotoStore: %v", err)
	}
	return NewHandler(s, photos, testHostURL, passthrough, passthrough), s
}

// get は経路にリクエストを流す。
func get(t *testing.T, h *Handler, path string) *httptest.ResponseRecorder {
	t.Helper()

	mux := http.NewServeMux()
	h.Register(mux)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))

	return w
}

// decodeItems は一覧の応答を読む。
func decodeItems(t *testing.T, w *httptest.ResponseRecorder) []itemResponse {
	t.Helper()

	var got struct {
		Items []itemResponse `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答が JSON でない: %v (%s)", err, w.Body.String())
	}
	return got.Items
}

func TestHandleList_一覧を返す(t *testing.T) {
	h, s := newTestHandler(t)

	insert(t, s, "0001", "三脚", map[string]any{"category": "撮影機材"})
	insert(t, s, "0002", "ドライバー", map[string]any{"category": "工具"})

	w := get(t, h, "/api/items")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	items := decodeItems(t, w)
	if len(items) != 2 {
		t.Fatalf("件数 = %d。2件を期待", len(items))
	}
	if items[0].Code != "0001" || items[0].Name != "三脚" {
		t.Errorf("内容が違う: %+v", items[0])
	}
}

// 0件でも null ではなく [] を返す。
// フロント側で「null かもしれない」の分岐を書かせない。
func TestHandleList_0件でも配列を返す(t *testing.T) {
	h, _ := newTestHandler(t)

	w := get(t, h, "/api/items")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("応答が JSON でない: %v", err)
	}
	if string(raw["items"]) != "[]" {
		t.Errorf("items = %s。[] を期待", raw["items"])
	}
}

func TestHandleList_クエリで絞れる(t *testing.T) {
	h, s := newTestHandler(t)

	insert(t, s, "0001", "三脚", map[string]any{"category": "撮影機材", "location": "棚A"})
	insert(t, s, "0002", "ドライバー", map[string]any{"category": "工具", "location": "棚B"})
	insert(t, s, "0003", "三脚（小）", map[string]any{"category": "撮影機材", "location": "棚B"})

	tests := []struct {
		name string
		path string
		want int
	}{
		{"検索語", "/api/items?q=三脚", 2},
		{"分類", "/api/items?category=工具", 1},
		{"保管場所", "/api/items?location=棚B", 2},
		{"組み合わせ", "/api/items?category=撮影機材&location=棚B", 1},
		{"状態", "/api/items?condition=良好", 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := get(t, h, tt.path)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
			}
			if items := decodeItems(t, w); len(items) != tt.want {
				t.Errorf("件数 = %d。%d件を期待", len(items), tt.want)
			}
		})
	}
}

// 知らない値を黙って無視すると、絞り込んだつもりの全件が返る。
func TestHandleList_不正な絞り込みは400(t *testing.T) {
	h, s := newTestHandler(t)
	insert(t, s, "0001", "三脚", nil)

	for _, path := range []string{
		"/api/items?condition=こわれた",
		"/api/items?location_status=どこか",
	} {
		t.Run(path, func(t *testing.T) {
			w := get(t, h, path)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d。400 を期待。body = %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestHandleDetail_備品コードで引ける(t *testing.T) {
	h, s := newTestHandler(t)
	insert(t, s, "0042", "三脚", map[string]any{"note": "脚のロックが緩い"})

	w := get(t, h, "/api/items/0042")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var got struct {
		Item itemResponse `json:"item"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答が JSON でない: %v", err)
	}
	if got.Item.Code != "0042" || got.Item.Name != "三脚" {
		t.Errorf("内容が違う: %+v", got.Item)
	}
	// 借用画面で責任の誤帰属を防ぐため、備考は必ず返す。
	if got.Item.Note != "脚のロックが緩い" {
		t.Errorf("備考が返っていない: %q", got.Item.Note)
	}
}

// QRを読んで来た人が最初に見る画面。何が起きたか分かる文言にする。
func TestHandleDetail_未登録のコードは404(t *testing.T) {
	h, _ := newTestHandler(t)

	w := get(t, h, "/api/items/9999")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d。404 を期待", w.Code)
	}
	if body := w.Body.String(); body == "" {
		t.Error("理由が返っていない")
	}
}

// 廃棄した備品も、コードを指定すれば引けること。
// QRラベルは貼られたままなので、読んだ時に「登録されていない」は誤り。
func TestHandleDetail_廃棄済みでも引ける(t *testing.T) {
	h, s := newTestHandler(t)
	insert(t, s, "0001", "壊れたカメラ", map[string]any{"condition": string(ConditionDiscarded)})

	w := get(t, h, "/api/items/0001")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d。廃棄済みでも引けるべき", w.Code)
	}

	var got struct {
		Item itemResponse `json:"item"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答が JSON でない: %v", err)
	}
	// 状態はそのまま伝える。記録されなかった事実を正常として表示しない。
	if got.Item.Condition != ConditionDiscarded {
		t.Errorf("Condition = %q。廃棄を期待", got.Item.Condition)
	}
}

func TestHandleFilters_使われている分類と保管場所を返す(t *testing.T) {
	h, s := newTestHandler(t)

	insert(t, s, "0001", "三脚", map[string]any{"category": "撮影機材", "location": "棚A"})
	insert(t, s, "0002", "カメラ", map[string]any{"category": "撮影機材", "location": "棚B"})

	w := get(t, h, "/api/items/filters")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var got struct {
		Categories []string `json:"categories"`
		Locations  []string `json:"locations"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答が JSON でない: %v", err)
	}
	if len(got.Categories) != 1 || got.Categories[0] != "撮影機材" {
		t.Errorf("分類 = %v", got.Categories)
	}
	if len(got.Locations) != 2 {
		t.Errorf("保管場所 = %v。2件を期待", got.Locations)
	}
}

// filters が {code} のパターンに吸われないこと。
// 吸われると「filters という備品コード」を探しに行って 404 になる。
func TestRegister_filtersが詳細に吸われない(t *testing.T) {
	h, _ := newTestHandler(t)

	w := get(t, h, "/api/items/filters")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d。200 を期待（詳細の経路に吸われている）", w.Code)
	}
}
