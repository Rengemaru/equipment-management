package item

import (
	"context"
	"io"
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
