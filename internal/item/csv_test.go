package item

import (
	"strings"
	"testing"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
)

// templateHeader はテンプレートの実物のヘッダ。
// 必須列に全角スペース＋アスタリスクが付いている。
const templateHeader = "品名　*,分類　*,数量　*,保管場所　*,型番・メーカー,状態,所有,自由利用,備考"

// parse は文字列をCSVとして解析する。
func parse(t *testing.T, text string) *CSVResult {
	t.Helper()

	got, err := ParseCSV(strings.NewReader(text))
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	return got
}

// テンプレートをそのまま書き出したCSVが取り込めること。
// ヘッダを完全一致で判定すると、ここで1件も取り込めなくなる。
func TestParseCSV_テンプレートのヘッダを解釈できる(t *testing.T) {
	text := templateHeader + "\n" +
		"三脚（大）,撮影機材,1,棚A-1,Manfrotto MT055,良好,サークル,FALSE,脚のロックが緩い\n"

	got := parse(t, text)

	if len(got.Errors) != 0 {
		t.Fatalf("エラーが出ている: %+v", got.Errors)
	}
	if len(got.Rows) != 1 {
		t.Fatalf("行数 = %d。1行を期待", len(got.Rows))
	}

	row := got.Rows[0]
	if row.Attributes.Name != "三脚（大）" {
		t.Errorf("Name = %q", row.Attributes.Name)
	}
	if row.Attributes.Category != "撮影機材" {
		t.Errorf("Category = %q", row.Attributes.Category)
	}
	if row.Attributes.Location != "棚A-1" {
		t.Errorf("Location = %q", row.Attributes.Location)
	}
	if row.Attributes.Model != "Manfrotto MT055" {
		t.Errorf("Model = %q", row.Attributes.Model)
	}
	if row.Attributes.Note != "脚のロックが緩い" {
		t.Errorf("Note = %q", row.Attributes.Note)
	}
	if row.Quantity != 1 {
		t.Errorf("Quantity = %d", row.Quantity)
	}
	// 行番号はヘッダを1行目として数える。表計算ソフトで探せるようにする。
	if row.Line != 2 {
		t.Errorf("Line = %d。2 を期待", row.Line)
	}
}

// アスタリスクが無いヘッダでも取り込めること（手で作ったCSV）。
func TestParseCSV_アスタリスクの無いヘッダも通す(t *testing.T) {
	text := "品名,分類,数量,保管場所\n三脚,撮影機材,1,棚A\n"

	got := parse(t, text)
	if len(got.Errors) != 0 || len(got.Rows) != 1 {
		t.Fatalf("rows=%d errors=%+v", len(got.Rows), got.Errors)
	}
}

// 数量ぶんのレコードに展開されること。
func TestParseCSV_数量を展開する(t *testing.T) {
	text := templateHeader + "\n" +
		"三脚,撮影機材,3,棚A,,,,,\n" +
		"カメラ,撮影機材,1,棚B,,,,,\n"

	got := parse(t, text)

	if len(got.Rows) != 2 {
		t.Fatalf("行数 = %d", len(got.Rows))
	}
	if got.Rows[0].Quantity != 3 {
		t.Errorf("Quantity = %d。3 を期待", got.Rows[0].Quantity)
	}
	// 「何行が何レコードになるか」をプレビューで見せるために使う。
	if n := got.TotalRecords(); n != 4 {
		t.Errorf("TotalRecords = %d。4 を期待", n)
	}
}

// テンプレートは200行の空行を持つ。弾かないとエラーが200件出る。
func TestParseCSV_空行を飛ばす(t *testing.T) {
	text := templateHeader + "\n" +
		"三脚,撮影機材,1,棚A,,,,,\n" +
		",,,,,,,,\n" +
		",,,,,,,,\n" +
		"   ,  ,,,,,,,\n"

	got := parse(t, text)

	if len(got.Errors) != 0 {
		t.Fatalf("空行でエラーが出ている: %+v", got.Errors)
	}
	if len(got.Rows) != 1 {
		t.Errorf("行数 = %d。1行を期待", len(got.Rows))
	}
}

// BOM 付きで書き出されても最初の列が一致すること。
func TestParseCSV_BOMを除去する(t *testing.T) {
	text := "\uFEFF" + templateHeader + "\n三脚,撮影機材,1,棚A,,,,,\n"

	got := parse(t, text)
	if len(got.Rows) != 1 {
		t.Fatalf("行数 = %d。BOM でヘッダが一致していない", len(got.Rows))
	}
}

// Excel からの書き出しは Shift_JIS になりがち。
func TestParseCSV_ShiftJISを読める(t *testing.T) {
	text := templateHeader + "\n三脚（大）,撮影機材,1,棚A-1,,良好,サークル,FALSE,備考です\n"

	sjis, _, err := transform.Bytes(japanese.ShiftJIS.NewEncoder(), []byte(text))
	if err != nil {
		t.Fatalf("Shift_JIS への変換: %v", err)
	}

	got, err := ParseCSV(strings.NewReader(string(sjis)))
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	if len(got.Rows) != 1 {
		t.Fatalf("行数 = %d, errors = %+v", len(got.Rows), got.Errors)
	}
	if got.Rows[0].Attributes.Name != "三脚（大）" {
		t.Errorf("Name = %q。文字化けしている", got.Rows[0].Attributes.Name)
	}
	if got.Rows[0].Attributes.Note != "備考です" {
		t.Errorf("Note = %q", got.Rows[0].Attributes.Note)
	}
}

func TestParseCSV_空欄は既定値になる(t *testing.T) {
	text := templateHeader + "\n三脚,撮影機材,1,棚A,,,,,\n"

	got := parse(t, text)
	attrs := got.Rows[0].Attributes

	// 画面から登録する時と同じ既定値になること。
	if attrs.Condition != ConditionGood {
		t.Errorf("Condition = %q", attrs.Condition)
	}
	if attrs.Owner != OwnerCircle {
		t.Errorf("Owner = %q", attrs.Owner)
	}
	if attrs.LocationStatus != LocationInStock {
		t.Errorf("LocationStatus = %q", attrs.LocationStatus)
	}
	if attrs.IsFreeUse {
		t.Error("IsFreeUse が true になっている")
	}
}

func TestParseCSV_自由利用の表記を解釈する(t *testing.T) {
	tests := []struct {
		value string
		want  bool
		ok    bool
	}{
		{"TRUE", true, true},
		{"true", true, true},
		{"FALSE", false, true},
		{"1", true, true},
		{"0", false, true},
		{"", false, true},
		{"○", true, true},
		{"たぶん", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			text := templateHeader + "\n三脚,撮影機材,1,棚A,,,," + tt.value + ",\n"
			got := parse(t, text)

			if !tt.ok {
				if len(got.Errors) != 1 {
					t.Fatalf("エラーを期待した: %+v", got)
				}
				return
			}
			if len(got.Rows) != 1 {
				t.Fatalf("行数 = %d, errors = %+v", len(got.Rows), got.Errors)
			}
			if got.Rows[0].Attributes.IsFreeUse != tt.want {
				t.Errorf("IsFreeUse = %v。%v を期待", got.Rows[0].Attributes.IsFreeUse, tt.want)
			}
		})
	}
}

// 全角数字は表計算ソフトの日本語入力でよく起きる。
func TestParseCSV_全角数字の数量を読める(t *testing.T) {
	text := templateHeader + "\n三脚,撮影機材,３,棚A,,,,,\n"

	got := parse(t, text)
	if len(got.Rows) != 1 {
		t.Fatalf("行数 = %d, errors = %+v", len(got.Rows), got.Errors)
	}
	if got.Rows[0].Quantity != 3 {
		t.Errorf("Quantity = %d。3 を期待", got.Rows[0].Quantity)
	}
}

// 誤りは行番号付きで全件返す。1件ずつ直させると200行のシートで200回やり直すことになる。
func TestParseCSV_誤りを行番号付きで全件返す(t *testing.T) {
	text := templateHeader + "\n" +
		"三脚,撮影機材,1,棚A,,,,,\n" + // 2行目: 正常
		",撮影機材,1,棚A,,,,,\n" + // 3行目: 品名が空
		"カメラ,撮影機材,あ,棚A,,,,,\n" + // 4行目: 数量が数値でない
		"三脚,撮影機材,1,,,,,,\n" + // 5行目: 保管場所が空
		"三脚,撮影機材,0,棚A,,,,,\n" + // 6行目: 数量が0
		"三脚,撮影機材,1,棚A,,こわれた,,,\n" // 7行目: 状態が不正

	got := parse(t, text)

	if len(got.Rows) != 1 {
		t.Errorf("正常な行数 = %d。1行を期待", len(got.Rows))
	}
	if len(got.Errors) != 5 {
		t.Fatalf("エラー数 = %d。5件を期待: %+v", len(got.Errors), got.Errors)
	}

	wantLines := []int{3, 4, 5, 6, 7}
	for i, want := range wantLines {
		if got.Errors[i].Line != want {
			t.Errorf("%d件目の行番号 = %d。%d を期待", i, got.Errors[i].Line, want)
		}
		if got.Errors[i].Message == "" {
			t.Errorf("%d件目の理由が空", i)
		}
	}
}

// 打ち間違いで、取り消せない量の備品コードが消費されるのを防ぐ。
func TestParseCSV_大きすぎる数量を弾く(t *testing.T) {
	text := templateHeader + "\n三脚,撮影機材,10000,棚A,,,,,\n"

	got := parse(t, text)
	if len(got.Errors) != 1 {
		t.Fatalf("エラーを期待した: %+v", got)
	}
	if !strings.Contains(got.Errors[0].Message, "大きすぎ") {
		t.Errorf("理由が分からない: %q", got.Errors[0].Message)
	}
}

// 必須列が無ければ、行の誤りではなく全体の誤りとして返す。
func TestParseCSV_必須列が無ければ解析しない(t *testing.T) {
	text := "品名,分類\n三脚,撮影機材\n"

	_, err := ParseCSV(strings.NewReader(text))
	if err == nil {
		t.Fatal("エラーを期待したが nil")
	}
	for _, want := range []string{"数量", "保管場所"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("不足している列 %q が示されていない: %v", want, err)
		}
	}
}

func TestParseCSV_空のCSVを弾く(t *testing.T) {
	if _, err := ParseCSV(strings.NewReader("")); err == nil {
		t.Error("空のCSVが通ってしまう")
	}
}

// 列の順番が違っても、ヘッダ名で対応が取れること。
func TestParseCSV_列の順番が違っても読める(t *testing.T) {
	text := "数量　*,保管場所　*,品名　*,分類　*,備考\n" +
		"2,棚C,ドライバー,工具,セット品\n"

	got := parse(t, text)
	if len(got.Rows) != 1 {
		t.Fatalf("行数 = %d, errors = %+v", len(got.Rows), got.Errors)
	}

	row := got.Rows[0]
	if row.Attributes.Name != "ドライバー" || row.Attributes.Location != "棚C" ||
		row.Quantity != 2 || row.Attributes.Note != "セット品" {
		t.Errorf("列の対応が取れていない: %+v", row)
	}
}

func TestNormalizeHeader(t *testing.T) {
	tests := map[string]string{
		"品名　*":     "品名",
		"品名 *":     "品名",
		"品名＊":      "品名",
		" 保管場所　* ": "保管場所",
		"型番・メーカー":  "型番・メーカー",
		"\uFEFF品名": "品名",
		"":         "",
	}

	for in, want := range tests {
		if got := normalizeHeader(in); got != want {
			t.Errorf("normalizeHeader(%q) = %q。%q を期待", in, got, want)
		}
	}
}
