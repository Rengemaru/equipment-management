package item

import (
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/Rengemaru/equipment-management/internal/httpx"
)

// exportFilename は書き出すファイル名。
//
// 日付を入れない。サーバの時刻と手元の時刻がずれると、いつ取ったものか
// かえって分からなくなる。世代の管理はファイルを置く側の仕事にする。
const exportFilename = "items.csv"

// exportCodeHeader は書き出しにだけ足す列。
//
// csvColumns には無い。取り込み側は常に自動採番するため入力として意味を持たないが、
// 書き出しに無いと「どのラベルの備品か」が分からず、控えとして使えない。
const exportCodeHeader = "備品コード"

// utf8BOM は Excel 向けの目印。
//
// 付けないと Excel が Shift_JIS として開き、品名が全て文字化けする。
// 取り込み側は BOM を落としてから読むので、書き出したCSVはそのまま取り込める。
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// registerExportRoutes はCSVエクスポートの経路を登録する。
func (h *Handler) registerExportRoutes(mux *http.ServeMux) {
	// admin 限定。備品マスタ全件の持ち出しで、member に開く理由がない。
	//
	// 経路の末尾を export.csv にしているのは、ブラウザが素直に
	// CSVとして保存するため。{code} の経路より literal が優先されるので、
	// 「export.csv という備品コード」を探しには行かない。
	mux.Handle("GET /api/items/export.csv",
		h.requireAdmin(http.HandlerFunc(h.handleExport)))
}

func (h *Handler) handleExport(w http.ResponseWriter, r *http.Request) {
	// 廃棄済みも含めて全件出す。目的はシステムが死んだ時の保険であって
	// 普段の一覧ではない（m1-spec §8）。既定の除外を持ち込むと、
	// 控えを見た人が「この備品は無かった」と読むことになる。
	items, err := h.store.List(r.Context(), Filter{IncludeDiscarded: true})
	if err != nil {
		log.Printf("items: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
		return
	}

	// 全件を読んでからヘッダを書く。書き始めてから失敗すると、200 のまま
	// 途中までのCSVが保存され、欠けていることに気付けない。
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+exportFilename+`"`)

	if err := writeCSV(w, items); err != nil {
		// ここでは既に応答が始まっており、状態コードを変えられない。
		// 気付けるようにログにだけ残す。
		log.Printf("items: CSVの書き出し: %v", err)
	}
}

// writeCSV は備品をテンプレートと同じ列でCSVに書き出す。
//
// # 取り込み直しても備品コードは戻らない
//
// 書き出したCSVをそのまま取り込むと、備品コードは新しく採番され直す。
// 取り込みは常に自動採番する仕様で、コードを指定する経路を持たない
// （持たせると、抜けや重複を人手で作れてしまう）。
// つまりこれは「中身を人が読める形で残す控え」であって、DBの複製ではない。
// 元のコードごと戻す必要があるなら -backup サブコマンドを使うこと。
//
// # 所在は書き出さない
//
// location_status は列に無い。テンプレートに無く、取り込み側も受け取らないため、
// 出しても戻せない。出し入れの形が合わない列を足すと、
// 「取り込めば元通り」という誤解を招く。
func writeCSV(w io.Writer, items []*Item) error {
	if _, err := w.Write(utf8BOM); err != nil {
		return fmt.Errorf("CSVの書き出し: %w", err)
	}

	cw := csv.NewWriter(w)
	// Excel 向けに CRLF。LF だけだと、古い Excel が1行として読む。
	cw.UseCRLF = true

	header := make([]string, 0, len(csvColumns)+1)
	header = append(header, exportCodeHeader)
	for _, col := range csvColumns {
		// アスタリスク付きのテンプレート実物ではなく正規化後の名前で出す。
		// 取り込み側は正規化してから照合するので、どちらでも取り込める。
		header = append(header, col.header)
	}
	if err := cw.Write(header); err != nil {
		return fmt.Errorf("CSVの書き出し: %w", err)
	}

	for _, it := range items {
		if err := cw.Write(exportRecord(it)); err != nil {
			return fmt.Errorf("CSVの書き出し: %w", err)
		}
	}

	cw.Flush()
	if err := cw.Error(); err != nil {
		return fmt.Errorf("CSVの書き出し: %w", err)
	}

	return nil
}

// exportRecord は1件を1行にする。列の並びは csvColumns と対で保つ。
func exportRecord(it *Item) []string {
	return []string{
		it.Code,
		it.Name,
		it.Category,
		// 数量は常に1。1レコード = 1つの実物 = 1つの備品コードで、
		// まとめると行とコードの対応が消える。
		"1",
		it.Location,
		it.Model,
		string(it.Condition),
		string(it.Owner),
		formatCSVBool(it.IsFreeUse),
		it.Note,
	}
}

// formatCSVBool は自由利用の列を書く。
//
// parseBool が受け取れる形にする。書き出したものが取り込めない状態は、
// 控えとして使えるかを確かめる術がなくなる。
func formatCSVBool(v bool) string {
	if v {
		return "TRUE"
	}
	return "FALSE"
}
