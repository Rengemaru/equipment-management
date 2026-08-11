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

// sendCSV はCSVを multipart で送る。exclude は除外する行の指定。
func sendCSV(t *testing.T, h *Handler, path, content string, exclude ...string) *httptest.ResponseRecorder {
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
	for _, v := range exclude {
		if err := mw.WriteField(excludeFormField, v); err != nil {
			t.Fatalf("WriteField: %v", err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	return w
}

// previewCSV はプレビューを取る。
func previewCSV(t *testing.T, h *Handler, content string) *httptest.ResponseRecorder {
	t.Helper()
	return sendCSV(t, h, "/api/items/import/preview", content)
}

// importCSV は取り込みを確定する。
func importCSV(t *testing.T, h *Handler, content string, exclude ...string) *httptest.ResponseRecorder {
	t.Helper()
	return sendCSV(t, h, "/api/items/import", content, exclude...)
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

// decodeImportResult は取り込みの応答を読む。
func decodeImportResult(t *testing.T, w *httptest.ResponseRecorder) importResultResponse {
	t.Helper()

	var got struct {
		Result importResultResponse `json:"result"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答が JSON でない: %v (%s)", err, w.Body.String())
	}
	return got.Result
}

// codesOf は登録されている備品コードを備品コード順で返す。
func codesOf(t *testing.T, s *Store) []string {
	t.Helper()

	items, err := s.List(context.Background(), Filter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	codes := make([]string, 0, len(items))
	for _, it := range items {
		codes = append(codes, it.Code)
	}
	return codes
}

// 数量ぶんに展開し、それぞれに別のコードを採る（m1-spec §5）。
func TestImport_数量ぶんに展開して登録する(t *testing.T) {
	h, s := newTestHandler(t)

	text := templateHeader + "\n" +
		"三脚,撮影機材,1,棚A-1,,,,,\n" +
		"パイプ椅子,備品,3,倉庫,,,,,\n"

	w := importCSV(t, h, text)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	got := decodeImportResult(t, w)
	if got.RecordCount != 4 {
		t.Errorf("RecordCount = %d。1+3 = 4 を期待", got.RecordCount)
	}
	if got.CodeFrom != "0001" || got.CodeTo != "0004" {
		t.Errorf("採番の範囲が違う: %q〜%q", got.CodeFrom, got.CodeTo)
	}

	// 続き番号で採る。この範囲がそのままラベルの印刷範囲になる。
	codes := codesOf(t, s)
	want := []string{"0001", "0002", "0003", "0004"}
	if len(codes) != len(want) {
		t.Fatalf("件数 = %d。4件を期待", len(codes))
	}
	for i, c := range want {
		if codes[i] != c {
			t.Errorf("codes[%d] = %q。%q を期待", i, codes[i], c)
		}
	}

	items, err := s.List(context.Background(), Filter{Query: "パイプ椅子"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("パイプ椅子 = %d件。数量ぶんの3件を期待", len(items))
	}
	// 展開した先も同じ内容になる。空欄は登録時と同じ既定値。
	if items[0].Location != "倉庫" || items[0].Condition != ConditionGood {
		t.Errorf("内容が違う: %+v", items[0])
	}
}

// 既存の続きから採り、空き番号を埋めない。
func TestImport_既存の続きから採番する(t *testing.T) {
	h, s := newTestHandler(t)

	// 0001 と 0005 がある状態。0002〜0004 は空いているが埋めない。
	insert(t, s, "0001", "既存A", nil)
	insert(t, s, "0005", "既存B", nil)

	w := importCSV(t, h, templateHeader+"\n三脚,撮影機材,2,棚A-1,,,,,\n")
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	got := decodeImportResult(t, w)
	if got.CodeFrom != "0006" || got.CodeTo != "0007" {
		t.Errorf("採番の範囲が違う: %q〜%q。0006〜0007 を期待", got.CodeFrom, got.CodeTo)
	}
}

// 全件成功か全件失敗。途中まで入った状態は、やり直しの判断がつかなくなる。
func TestImport_誤りが1件でもあれば何も登録しない(t *testing.T) {
	h, s := newTestHandler(t)

	text := templateHeader + "\n" +
		"三脚,撮影機材,1,棚A-1,,,,,\n" +
		"椅子,備品,あ,倉庫,,,,,\n" // 数量が数値でない

	w := importCSV(t, h, text)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d。400 を期待: %s", w.Code, w.Body.String())
	}

	// 文言で分岐させないための識別子。
	var errResp struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("応答が JSON でない: %v", err)
	}
	if errResp.Code != "csv_invalid" {
		t.Errorf("code = %q。csv_invalid を期待", errResp.Code)
	}

	// 誤りのない行も入れない。
	if codes := codesOf(t, s); len(codes) != 0 {
		t.Errorf("%d件 登録された: %v。全件失敗を期待", len(codes), codes)
	}
}

// 記入例の消し忘れを、CSVを直させずに除ける。手間を増やすと使われなくなる。
func TestImport_指定した行を除外する(t *testing.T) {
	h, s := newTestHandler(t)

	text := templateHeader + "\n" +
		"三脚（大）,撮影機材,1,棚A-1,,,,,\n" + // 2行目＝テンプレートの記入例
		"パイプ椅子,備品,2,倉庫,,,,,\n"

	w := importCSV(t, h, text, "2")
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	if got := decodeImportResult(t, w).RecordCount; got != 2 {
		t.Errorf("RecordCount = %d。除外後の2件を期待", got)
	}

	items, err := s.List(context.Background(), Filter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, it := range items {
		if it.Name == "三脚（大）" {
			t.Errorf("除外した行が登録されている: %+v", it)
		}
	}
}

// まとめた形とフィールドを繰り返す形の両方を受ける。
// 送り方の違いだけで取り込めないのは、原因が分かりにくい割に得るものがない。
func TestImport_除外の指定はカンマ区切りでも繰り返しでも受ける(t *testing.T) {
	text := templateHeader + "\n" +
		"A,備品,1,倉庫,,,,,\n" +
		"B,備品,1,倉庫,,,,,\n" +
		"C,備品,1,倉庫,,,,,\n"

	for _, tc := range []struct {
		name    string
		exclude []string
	}{
		{name: "カンマ区切り", exclude: []string{"2,3"}},
		{name: "繰り返し", exclude: []string{"2", "3"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, s := newTestHandler(t)

			w := importCSV(t, h, text, tc.exclude...)
			if w.Code != http.StatusCreated {
				t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
			}

			items, err := s.List(context.Background(), Filter{})
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(items) != 1 || items[0].Name != "C" {
				t.Errorf("残った備品が違う: %+v", items)
			}
		})
	}
}

// 除外に指定した行が無ければ取り込まない。
//
// 黙って無視すると、行番号がずれたCSVを送った時に、除いたつもりの記入例が
// そのまま入る。備品コードは再利用しないため入ってしまうと戻せない。
func TestImport_除外に無い行を指定したら何も登録しない(t *testing.T) {
	h, s := newTestHandler(t)

	text := templateHeader + "\n三脚,撮影機材,1,棚A-1,,,,,\n"

	w := importCSV(t, h, text, "9")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d。400 を期待: %s", w.Code, w.Body.String())
	}
	if codes := codesOf(t, s); len(codes) != 0 {
		t.Errorf("%d件 登録された: %v", len(codes), codes)
	}
}

func TestImport_全ての行を除外したら400(t *testing.T) {
	h, _ := newTestHandler(t)

	text := templateHeader + "\n三脚,撮影機材,1,棚A-1,,,,,\n"

	w := importCSV(t, h, text, "2")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d。400 を期待: %s", w.Code, w.Body.String())
	}
}

func TestImport_除外の指定が数値でなければ400(t *testing.T) {
	h, _ := newTestHandler(t)

	text := templateHeader + "\n三脚,撮影機材,1,棚A-1,,,,,\n"

	w := importCSV(t, h, text, "２行目")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d。400 を期待: %s", w.Code, w.Body.String())
	}
}

// プレビューで見た通りの件数が入ること。
// 見せた数と違う数が入ると、何が起きたのか誰にも分からなくなる。
func TestImport_プレビューと同じ件数が登録される(t *testing.T) {
	h, s := newTestHandler(t)

	insert(t, s, "0003", "既存", nil)

	text := templateHeader + "\n" +
		"三脚,撮影機材,2,棚A-1,,,,,\n" +
		"椅子,備品,5,倉庫,,,,,\n"

	preview := decodePreview(t, previewCSV(t, h, text))
	result := decodeImportResult(t, importCSV(t, h, text))

	if preview.RecordCount != result.RecordCount {
		t.Errorf("件数が違う: プレビュー %d、取り込み %d", preview.RecordCount, result.RecordCount)
	}
	// 間に別の登録が無ければ、予定した範囲がそのまま実際の範囲になる。
	if preview.CodeFrom != result.CodeFrom || preview.CodeTo != result.CodeTo {
		t.Errorf("採番の範囲が違う: プレビュー %q〜%q、取り込み %q〜%q",
			preview.CodeFrom, preview.CodeTo, result.CodeFrom, result.CodeTo)
	}
}

func TestImport_添付が無ければ400(t *testing.T) {
	h, _ := newTestHandler(t)

	w := send(t, h, http.MethodPost, "/api/items/import", `{}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d。400 を期待: %s", w.Code, w.Body.String())
	}
}

// 権限チェックはUIで隠すだけにしない。経路が requireAdmin を通ること。
func TestImport_adminでなければ拒否される(t *testing.T) {
	h := newDenyAdminHandler(t)

	w := importCSV(t, h, templateHeader+"\n三脚,撮影機材,1,棚A-1,,,,,\n")

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d。403 を期待", w.Code)
	}
}

// 権限チェックはUIで隠すだけにしない。経路が requireAdmin を通ること。
func TestImportPreview_adminでなければ拒否される(t *testing.T) {
	h := newDenyAdminHandler(t)

	w := previewCSV(t, h, templateHeader+"\n三脚,撮影機材,1,棚A-1,,,,,\n")

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d。403 を期待", w.Code)
	}
}

// newDenyAdminHandler は admin を拒否する Handler を返す。
// 経路が requireAdmin を通っていなければ、素通りして 200 系が返る。
func newDenyAdminHandler(t *testing.T) *Handler {
	t.Helper()

	s := newTestStore(t)
	photos, err := NewPhotoStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewPhotoStore: %v", err)
	}

	denyAdmin := func(http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		})
	}

	return NewHandler(s, photos, passthrough, denyAdmin)
}
