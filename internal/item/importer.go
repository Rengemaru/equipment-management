package item

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// ImportPreview は取り込む前に見せる内容。
//
// 確定の前に必ずこれを挟む。テンプレートの2行目には記入例（`三脚（大）` の行）が
// 入っており、記入ガイドに「作業前に削除する」と書かれていても消し忘れは必ず起きる
// （m1-spec §5）。全行を見せて、確定の前に気付ける状態にする。
//
// 備品コードは再利用しないため、余計な行を取り込むと番号を戻せない。
type ImportPreview struct {
	// Rows は取り込める行。誤りのある行は含まない。
	Rows []CSVRow

	// Errors は行ごとの誤り。1件ずつ直させないため全件返す。
	Errors []CSVRowError

	// RecordCount は取り込んだ時に作られるレコード数。
	// 数量ぶんに展開されるため、Rows の数とは一致しない。
	RecordCount int

	// FirstCode と LastCode は採番される備品コードの予定。
	//
	// 確定に進めない状態では空にする。行を直せば採番の範囲も変わるのに、
	// 決まった値のように見せると、この範囲でラベルを刷る人が出る。
	FirstCode string
	LastCode  string
}

// CanImport は確定に進めるか。
//
// 誤りが1件でもあれば進めない。取り込みは全件成功か全件失敗で、
// 誤った行だけを飛ばして入れることはしない（m1-spec §5）。
// 途中まで入った状態は、やり直しの判断がつかなくなる。
func (p *ImportPreview) CanImport() bool {
	return len(p.Errors) == 0 && len(p.Rows) > 0
}

// PreviewImport はCSVを解析し、取り込んだら何が起きるかを返す。
//
// DBは読むだけで、何度呼んでも備品コードを消費しない。採番は確定の時に、
// INSERT と同じトランザクションの中で行う。
//
// 戻り値のエラーは「CSVとして読めない」場合だけ。行ごとの誤りは
// ImportPreview.Errors に入れて返す。
func (s *Store) PreviewImport(ctx context.Context, r io.Reader) (*ImportPreview, error) {
	result, err := ParseCSV(r)
	if err != nil {
		return nil, err
	}

	preview := &ImportPreview{
		Rows:        result.Rows,
		Errors:      result.Errors,
		RecordCount: result.TotalRecords(),
	}

	// 確定できない状態では採番の予定を出さない。
	if !preview.CanImport() {
		return preview, nil
	}

	// あくまで予定。プレビューと確定の間に別の登録が入れば後ろにずれる。
	// 確定の応答で実際のコードを返すこと。
	first, err := nextCodeNumber(ctx, s.sqldb)
	if err != nil {
		return nil, err
	}

	preview.FirstCode = FormatCode(first)
	preview.LastCode = FormatCode(first + int64(preview.RecordCount) - 1)

	return preview, nil
}

// ImportRowsError は取り込めない行があること。
//
// 型で区別するのは、ハンドラが「直せる誤り」と「サーバの不具合」を
// 取り違えないため。行ごとの内容はプレビューで見せる。
type ImportRowsError struct {
	Errors []CSVRowError
}

func (e *ImportRowsError) Error() string {
	return fmt.Sprintf("取り込めない行が%d件あります", len(e.Errors))
}

// ImportOptions は取り込みの指定。
type ImportOptions struct {
	// ExcludeLines は取り込まない行の行番号。プレビューが返した Line を指す。
	//
	// テンプレートの記入例を消し忘れたまま来た時に、CSVを直して
	// 送り直させずに済ませるための逃げ道。手間を増やすと使われなくなる。
	ExcludeLines []int
}

// ImportResult は取り込みの結果。
type ImportResult struct {
	// RecordCount は登録したレコード数。
	RecordCount int

	// FirstCode と LastCode は実際に採番された備品コードの範囲。
	// 続き番号で採るため、この範囲がそのままラベルの印刷範囲になる。
	FirstCode string
	LastCode  string
}

// Import はCSVを取り込む。
//
// # 全件成功か全件失敗
//
// 採番から INSERT までを1つのトランザクションで行う。途中まで取り込まれた状態は、
// どこまで入ったのか分からず、やり直しの判断がつかなくなる（m1-spec §5）。
// 誤りのある行を飛ばして残りを入れることもしない。
//
// # プレビューと同じ入力を送り直させる
//
// 解析した結果をサーバに覚えさせない。持たせると、確定するまでの間だけ存在する
// 状態が増え、期限切れの扱いを考えることになる。CSVは大きくないので送り直す方が安い。
func (s *Store) Import(ctx context.Context, r io.Reader, opts ImportOptions) (*ImportResult, error) {
	result, err := ParseCSV(r)
	if err != nil {
		return nil, err
	}

	// 1件でも誤りがあれば何も入れない。
	if len(result.Errors) > 0 {
		return nil, &ImportRowsError{Errors: result.Errors}
	}

	rows, err := excludeLines(result.Rows, opts.ExcludeLines)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, invalidf("取り込む行がありません")
	}

	tx, err := s.sqldb.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("CSVの取り込み: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// 採番はトランザクションの中で1度だけ行い、あとは続き番号にする。
	// 1件ごとに最大値を引き直すと、同時に取り込んだ2件が同じ番号を取る。
	first, err := nextCodeNumber(ctx, tx)
	if err != nil {
		return nil, err
	}

	next := first
	for _, row := range rows {
		// 数量ぶんに展開する。1行が数量の数だけのレコードになり、
		// それぞれに別のコードが付く（m1-spec §5）。
		for i := 0; i < row.Quantity; i++ {
			if err := insertItem(ctx, tx, FormatCode(next), row.Attributes); err != nil {
				return nil, err
			}
			next++
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("CSVの取り込み: %w", err)
	}

	return &ImportResult{
		RecordCount: int(next - first),
		FirstCode:   FormatCode(first),
		LastCode:    FormatCode(next - 1),
	}, nil
}

// excludeLines は指定された行番号を除いた行を返す。
//
// 指定された行が無ければ誤りとして返す。黙って無視すると、行番号がずれたCSVを
// 送った時に、除いたつもりの記入例がそのまま入る。備品コードは再利用しないため、
// 入ってしまうと番号を戻せない。
func excludeLines(rows []CSVRow, lines []int) ([]CSVRow, error) {
	if len(lines) == 0 {
		return rows, nil
	}

	excluded := make(map[int]bool, len(lines))
	for _, line := range lines {
		excluded[line] = true
	}

	kept := make([]CSVRow, 0, len(rows))
	for _, row := range rows {
		if excluded[row.Line] {
			delete(excluded, row.Line)
			continue
		}
		kept = append(kept, row)
	}

	if len(excluded) > 0 {
		return nil, invalidf(
			"除外に指定した行がCSVにありません: %s（プレビューを取り直してください）",
			joinInts(missingLines(excluded)))
	}

	return kept, nil
}

// missingLines は残った行番号を昇順で返す。map の順序は毎回変わるため、
// そのまま並べるとメッセージの並びが実行のたびに変わる。
func missingLines(excluded map[int]bool) []int {
	lines := make([]int, 0, len(excluded))
	for line := range excluded {
		lines = append(lines, line)
	}

	// 件数は多くても数件。素朴な挿入で足りる。
	for i := 1; i < len(lines); i++ {
		for j := i; j > 0 && lines[j] < lines[j-1]; j-- {
			lines[j], lines[j-1] = lines[j-1], lines[j]
		}
	}

	return lines
}

func joinInts(values []int) string {
	parts := make([]string, 0, len(values))
	for _, v := range values {
		parts = append(parts, fmt.Sprint(v))
	}
	return strings.Join(parts, "・")
}
