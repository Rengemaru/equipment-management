package item

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

// previewCSV はCSVを multipart で送り、プレビューを取る。
func previewCSV(t *testing.T, h *Handler, content string) *httptest.ResponseRecorder {
	t.Helper()

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)

	part, err := mw.CreateFormFile(csvFormField, "inventory.csv")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/items/import/preview", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	return w
}

// decodePreview はプレビューの応答を読む。
func decodePreview(t *testing.T, w *httptest.ResponseRecorder) importPreviewResponse {
	t.Helper()

	var got struct {
		Preview importPreviewResponse `json:"preview"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答が JSON でない: %v (%s)", err, w.Body.String())
	}
	return got.Preview
}

// 数量ぶんに展開されるため、行数とレコード数は一致しない。
// 何件が何レコードになるかを確定の前に見せる（m1-spec §5）。
func TestImportPreview_行数とレコード数を返す(t *testing.T) {
	h, _ := newTestHandler(t)

	text := templateHeader + "\n" +
		"三脚,撮影機材,1,棚A-1,,,,,\n" +
		"パイプ椅子,備品,10,倉庫,,,,,\n" +
		"ドライバー,工具,3,棚B-2,,,,,\n"

	w := previewCSV(t, h, text)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	got := decodePreview(t, w)
	if got.RowCount != 3 {
		t.Errorf("RowCount = %d。3行を期待", got.RowCount)
	}
	if got.RecordCount != 14 {
		t.Errorf("RecordCount = %d。1+10+3 = 14 を期待", got.RecordCount)
	}
	if !got.CanImport {
		t.Errorf("CanImport = false。誤りが無いので true を期待: %+v", got.Errors)
	}
}

// プレビューは全行を返す。テンプレートの記入例の消し忘れは必ず起きるため、
// 確定の前に目で見て気付ける状態にする（m1-spec §5）。
func TestImportPreview_全行を既定値込みで返す(t *testing.T) {
	h, _ := newTestHandler(t)

	// 状態・所有・自由利用を空欄にする。既定値が入った後の値が返ること。
	text := templateHeader + "\n" +
		"三脚（大）,撮影機材,2,棚A-1,Manfrotto MT055,,,,脚のロックが緩い\n"

	got := decodePreview(t, previewCSV(t, h, text))

	if len(got.Rows) != 1 {
		t.Fatalf("Rows = %d件。1件を期待", len(got.Rows))
	}

	row := got.Rows[0]
	// 行番号はヘッダを1行目として数える。表計算ソフトで同じ行を開けるようにする。
	if row.Line != 2 {
		t.Errorf("Line = %d。2を期待", row.Line)
	}
	if row.Quantity != 2 {
		t.Errorf("Quantity = %d", row.Quantity)
	}
	if row.Name != "三脚（大）" || row.Model != "Manfrotto MT055" {
		t.Errorf("内容が違う: %+v", row)
	}
	if row.Note != "脚のロックが緩い" {
		t.Errorf("Note = %q", row.Note)
	}

	// 空欄は登録時と同じ既定値になる。CSVの見た目ではなく、
	// 実際に登録される内容を見せる。
	if row.Condition != ConditionGood {
		t.Errorf("Condition = %q。空欄は良好になる", row.Condition)
	}
	if row.Owner != OwnerCircle {
		t.Errorf("Owner = %q。空欄はサークルになる", row.Owner)
	}
	if row.IsFreeUse {
		t.Errorf("IsFreeUse = true。空欄は記録が要る備品として扱う")
	}
}

// 1件ずつ直させない。200行のシートで200回やり直すことになる。
func TestImportPreview_誤りを行番号付きで全件返す(t *testing.T) {
	h, _ := newTestHandler(t)

	text := templateHeader + "\n" +
		"三脚,撮影機材,1,棚A-1,,,,,\n" +
		",撮影機材,1,棚A-1,,,,,\n" + // 品名が空
		"椅子,備品,あ,倉庫,,,,,\n" + // 数量が数値でない
		"机,備品,0,倉庫,,,,,\n" // 数量が0

	got := decodePreview(t, previewCSV(t, h, text))

	if len(got.Errors) != 3 {
		t.Fatalf("Errors = %d件。3件まとめて返すことを期待: %+v", len(got.Errors), got.Errors)
	}

	wantLines := []int{3, 4, 5}
	for i, want := range wantLines {
		if got.Errors[i].Line != want {
			t.Errorf("Errors[%d].Line = %d。%d を期待", i, got.Errors[i].Line, want)
		}
		if got.Errors[i].Message == "" {
			t.Errorf("Errors[%d].Message が空。理由が分からないと直せない", i)
		}
	}

	// 誤りがある間は確定に進めない。全件成功か全件失敗のため。
	if got.CanImport {
		t.Errorf("CanImport = true。誤りがあるので false を期待")
	}

	// 誤りのある行を除いた分は返す。何が通って何が落ちたかを見せる。
	if got.RowCount != 1 {
		t.Errorf("RowCount = %d。誤りの無い1行を期待", got.RowCount)
	}
}

// 採番の予定を見せる。ラベルを何番から刷ることになるかが分かる。
func TestImportPreview_採番の予定を返す(t *testing.T) {
	h, s := newTestHandler(t)

	// 既に 0005 まで使われている状態にする。
	insert(t, s, "0005", "既存の備品", nil)

	text := templateHeader + "\n" +
		"三脚,撮影機材,3,棚A-1,,,,,\n"

	got := decodePreview(t, previewCSV(t, h, text))

	if got.CodeFrom != "0006" {
		t.Errorf("CodeFrom = %q。最大値+1 の 0006 を期待", got.CodeFrom)
	}
	if got.CodeTo != "0008" {
		t.Errorf("CodeTo = %q。3レコードぶんの 0008 を期待", got.CodeTo)
	}
}

// プレビューは何度出しても備品コードを消費しない。
// 採番は確定の時に INSERT と同じトランザクションで行う。
func TestImportPreview_コードを消費しない(t *testing.T) {
	h, s := newTestHandler(t)

	text := templateHeader + "\n" +
		"三脚,撮影機材,3,棚A-1,,,,,\n"

	first := decodePreview(t, previewCSV(t, h, text))
	second := decodePreview(t, previewCSV(t, h, text))

	if first.CodeFrom != second.CodeFrom {
		t.Errorf("CodeFrom が変わった: %q → %q", first.CodeFrom, second.CodeFrom)
	}

	items, err := s.List(context.Background(), Filter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("備品が %d件 登録された。プレビューは書き込まない", len(items))
	}
}

// 確定に進めない状態で採番の予定を出さない。
// 行を直せば範囲も変わるのに、この範囲でラベルを刷る人が出る。
func TestImportPreview_確定できない時は採番の予定を出さない(t *testing.T) {
	h, _ := newTestHandler(t)

	text := templateHeader + "\n" +
		",撮影機材,1,棚A-1,,,,,\n" // 品名が空

	got := decodePreview(t, previewCSV(t, h, text))

	if got.CodeFrom != "" || got.CodeTo != "" {
		t.Errorf("CodeFrom = %q, CodeTo = %q。誤りがある間は空を期待", got.CodeFrom, got.CodeTo)
	}
}

// テンプレートは200行の空行を持つ。弾かないとエラーが200件出る。
func TestImportPreview_空行を数に入れない(t *testing.T) {
	h, _ := newTestHandler(t)

	text := templateHeader + "\n" +
		"三脚,撮影機材,1,棚A-1,,,,,\n" +
		",,,,,,,,\n" +
		",,,,,,,,\n"

	got := decodePreview(t, previewCSV(t, h, text))

	if len(got.Errors) != 0 {
		t.Fatalf("空行がエラーになっている: %+v", got.Errors)
	}
	if got.RowCount != 1 {
		t.Errorf("RowCount = %d。1行を期待", got.RowCount)
	}
}

// 中身が1行も無いCSVで確定に進ませない。0件の取り込みは操作の取り違え。
func TestImportPreview_中身が無ければ確定に進めない(t *testing.T) {
	h, _ := newTestHandler(t)

	got := decodePreview(t, previewCSV(t, h, templateHeader+"\n"))

	if got.CanImport {
		t.Errorf("CanImport = true。0行なので false を期待")
	}
	if got.RecordCount != 0 {
		t.Errorf("RecordCount = %d", got.RecordCount)
	}
}

// 0件でも null ではなく [] を返す。
func TestImportPreview_0件でも配列を返す(t *testing.T) {
	h, _ := newTestHandler(t)

	w := previewCSV(t, h, templateHeader+"\n")

	var got struct {
		Preview struct {
			Rows   []importRowResponse   `json:"rows"`
			Errors []importErrorResponse `json:"errors"`
		} `json:"preview"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答が JSON でない: %v (%s)", err, w.Body.String())
	}
	if got.Preview.Rows == nil || got.Preview.Errors == nil {
		t.Errorf("null が返っている: %s", w.Body.String())
	}
}

// CSVとして読めないものは 400。500 にすると、直せる誤りなのか
// サーバの不具合なのかが利用者に分からない。
func TestImportPreview_列が足りなければ400(t *testing.T) {
	h, _ := newTestHandler(t)

	w := previewCSV(t, h, "品名,分類\n三脚,撮影機材\n")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d。400 を期待: %s", w.Code, w.Body.String())
	}
}

func TestImportPreview_添付が無ければ400(t *testing.T) {
	h, _ := newTestHandler(t)

	w := send(t, h, http.MethodPost, "/api/items/import/preview", `{}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d。400 を期待: %s", w.Code, w.Body.String())
	}
}

// 権限チェックはUIで隠すだけにしない。経路が requireAdmin を通ること。
func TestImportPreview_adminでなければ拒否される(t *testing.T) {
	s := newTestStore(t)
	photos, err := NewPhotoStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewPhotoStore: %v", err)
	}

	// admin を拒否するミドルウェアを挿す。経路が素通りしていれば 200 が返る。
	denyAdmin := func(http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		})
	}
	h := NewHandler(s, photos, passthrough, denyAdmin)

	w := previewCSV(t, h, templateHeader+"\n三脚,撮影機材,1,棚A-1,,,,,\n")

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d。403 を期待", w.Code)
	}
}
