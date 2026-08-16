package item

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
)

const (
	// maxQuantity は1行から作れるレコード数の上限。
	//
	// 数量の打ち間違い（1 のつもりが 1000）で、取り消せない量の
	// 備品コードが消費されるのを防ぐ。コードは再利用しない。
	maxQuantity = 999

	// maxCSVBytes は受け付けるCSVの大きさ。
	// テンプレートは200行だが、複数回に分けて取り込む運用も想定して余裕を取る。
	maxCSVBytes = 5 << 20
)

// csvColumn はCSVの列。
type csvColumn struct {
	// header はテンプレートのヘッダ（正規化後）。
	header string
	// required は必須列か。
	required bool
}

// csvColumns は取り込む列の定義。inventory-template.xlsx の「棚卸し」シートに合わせる。
//
// ヘッダは正規化後の形で書く。テンプレートの実物は必須列に
// 全角スペース＋アスタリスクが付いており（`品名　*`）、
// 素朴に完全一致で判定すると1件も取り込めない。
var csvColumns = []csvColumn{
	{header: "品名", required: true},
	{header: "分類", required: true},
	{header: "数量", required: true},
	{header: "保管場所", required: true},
	{header: "型番・メーカー"},
	{header: "状態"},
	{header: "所有"},
	{header: "自由利用"},
	{header: "備考"},
}

// CSVRow は取り込む1行。
type CSVRow struct {
	// Line はCSV上の行番号。1始まりで、ヘッダを1行目として数える。
	// エラーを画面に出す時、利用者が表計算ソフトで探せるようにする。
	Line int

	// Quantity はこの行から作るレコード数。
	Quantity int

	Attributes Attributes
}

// CSVRowError は1行の誤り。
type CSVRowError struct {
	Line    int
	Message string
}

// CSVResult は解析の結果。
//
// 誤りがあっても解析を止めない。1件ずつ直させると、200行のシートで
// 200回やり直すことになる。
type CSVResult struct {
	Rows   []CSVRow
	Errors []CSVRowError
}

// TotalRecords は取り込んだ時に作られるレコード数を返す。
// 数量ぶんに展開されるため、行数とは一致しない。
func (r *CSVResult) TotalRecords() int {
	var n int
	for _, row := range r.Rows {
		n += row.Quantity
	}
	return n
}

// ParseCSV は棚卸しCSVを解析する。
//
// 戻り値のエラーは「CSVとして読めない」場合だけ。行ごとの誤りは
// CSVResult.Errors に入れて返す。
func ParseCSV(r io.Reader) (*CSVResult, error) {
	raw, err := io.ReadAll(io.LimitReader(r, maxCSVBytes+1))
	if err != nil {
		return nil, fmt.Errorf("CSVの読み込み: %w", err)
	}
	if len(raw) > maxCSVBytes {
		return nil, invalidf("CSVが大きすぎます（%dMBまで）", maxCSVBytes>>20)
	}

	text, err := decodeCSVBytes(raw)
	if err != nil {
		return nil, err
	}

	reader := csv.NewReader(strings.NewReader(text))
	// 列数を揃えない。テンプレートの空行は列が欠けることがあり、
	// 揃っていないだけで全体を失敗させる理由がない。
	reader.FieldsPerRecord = -1

	records, err := reader.ReadAll()
	if err != nil {
		return nil, invalidf("CSVとして読めません: %v", err)
	}
	if len(records) == 0 {
		return nil, invalidf("CSVが空です")
	}

	index, err := mapHeader(records[0])
	if err != nil {
		return nil, err
	}

	result := &CSVResult{}
	for i, record := range records[1:] {
		line := i + 2 // ヘッダが1行目

		// 空行は黙って飛ばす。テンプレートは200行の空行を持つため、
		// 弾かないとエラーが200件出る。
		if isEmptyRecord(record) {
			continue
		}

		row, err := parseRecord(line, record, index)
		if err != nil {
			result.Errors = append(result.Errors, CSVRowError{Line: line, Message: err.Error()})
			continue
		}
		result.Rows = append(result.Rows, row)
	}

	return result, nil
}

// decodeCSVBytes は文字コードを判定して UTF-8 の文字列にする。
//
// Excel からの書き出しは Shift_JIS になりがちで、UTF-8 だけを想定すると
// 品名が化けたまま取り込まれる。
func decodeCSVBytes(raw []byte) (string, error) {
	// BOM を落とす。残すと最初の列のヘッダが一致しない。
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})

	if utf8.Valid(raw) {
		return string(raw), nil
	}

	// UTF-8 として読めないなら Shift_JIS とみなす。日本語のテキストが
	// 両方として妥当になることはほぼ無い。
	decoded, _, err := transform.Bytes(japanese.ShiftJIS.NewDecoder(), raw)
	if err != nil {
		return "", invalidf("文字コードを判別できません。UTF-8 か Shift_JIS で保存してください")
	}

	return string(decoded), nil
}

// mapHeader はヘッダ行から「列名 → 位置」を作る。
func mapHeader(header []string) (map[string]int, error) {
	index := make(map[string]int, len(header))
	for i, h := range header {
		normalized := normalizeHeader(h)
		if normalized == "" {
			continue
		}
		// 同じ列名が2つあれば先勝ち。後ろの空列を拾わないため。
		if _, exists := index[normalized]; !exists {
			index[normalized] = i
		}
	}

	var missing []string
	for _, col := range csvColumns {
		if !col.required {
			continue
		}
		if _, ok := index[col.header]; !ok {
			missing = append(missing, col.header)
		}
	}
	if len(missing) > 0 {
		return nil, invalidf(
			"必要な列がありません: %s（棚卸しシートをそのままCSVで書き出してください）",
			strings.Join(missing, "・"))
	}

	return index, nil
}

// normalizeHeader はヘッダを照合できる形にそろえる。
//
// テンプレートの必須列は `品名　*` のように全角スペース＋アスタリスクが付く。
// 空白・アスタリスク・BOM を落としてから比べる。
func normalizeHeader(h string) string {
	// U+FEFF。文字としてのBOMが列名に残ることがある。
	h = strings.TrimPrefix(h, "\uFEFF")

	var b strings.Builder
	for _, r := range h {
		switch {
		case unicode.IsSpace(r), r == '　': // 半角・全角の空白
		case r == '*', r == '＊':
		default:
			b.WriteRune(r)
		}
	}

	return b.String()
}

// isEmptyRecord は全ての列が空かを返す。
func isEmptyRecord(record []string) bool {
	for _, v := range record {
		if strings.TrimSpace(v) != "" {
			return false
		}
	}
	return true
}

// parseRecord は1行を CSVRow に変える。
func parseRecord(line int, record []string, index map[string]int) (CSVRow, error) {
	get := func(header string) string {
		i, ok := index[header]
		if !ok || i >= len(record) {
			return ""
		}
		return strings.TrimSpace(record[i])
	}

	quantity, err := parseQuantity(get("数量"))
	if err != nil {
		return CSVRow{}, err
	}

	freeUse, err := parseBool(get("自由利用"))
	if err != nil {
		return CSVRow{}, err
	}

	attrs := Attributes{
		Name:      get("品名"),
		Category:  get("分類"),
		Model:     get("型番・メーカー"),
		Owner:     Owner(get("所有")),
		IsFreeUse: freeUse,
		Location:  get("保管場所"),
		Condition: Condition(get("状態")),
		Note:      get("備考"),
	}

	// 空欄の既定値と値の検査は、画面から登録する時と同じ処理を通す。
	// 経路によって取り込める値が変わると、CSVで入れたものだけが
	// 画面で編集できないといったことが起きる。
	if err := attrs.normalize(); err != nil {
		return CSVRow{}, err
	}

	// 保管場所は必須列。normalize では必須にしていない（画面からは省略できる）ため、
	// ここで見る。
	if attrs.Location == "" {
		return CSVRow{}, errors.New("保管場所は必須です")
	}

	return CSVRow{Line: line, Quantity: quantity, Attributes: attrs}, nil
}

// parseQuantity は数量を読む。
func parseQuantity(v string) (int, error) {
	if v == "" {
		return 0, errors.New("数量は必須です")
	}

	// 全角数字で書かれることがある。表計算ソフトの日本語入力ではよく起きる。
	v = toHalfWidthDigits(v)

	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("数量が数値ではありません: %q", v)
	}
	if n < 1 {
		return 0, fmt.Errorf("数量は1以上にしてください: %d", n)
	}
	if n > maxQuantity {
		// 打ち間違いで、取り消せない量の備品コードが消費されるのを防ぐ。
		return 0, fmt.Errorf("数量が大きすぎます（%dまで）: %d", maxQuantity, n)
	}

	return n, nil
}

// parseBool は自由利用の列を読む。
func parseBool(v string) (bool, error) {
	switch strings.ToUpper(v) {
	case "":
		// 空欄は「記録が要る備品」。追跡対象から外すのは明示した時だけにする。
		return false, nil
	case "TRUE", "1", "○", "はい", "YES":
		return true, nil
	case "FALSE", "0", "×", "いいえ", "NO":
		return false, nil
	default:
		return false, fmt.Errorf("自由利用は TRUE か FALSE にしてください: %q", v)
	}
}

// toHalfWidthDigits は全角数字を半角に直す。
func toHalfWidthDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '０' && r <= '９' {
			b.WriteRune(r - '０' + '0')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
