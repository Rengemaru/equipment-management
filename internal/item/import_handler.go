package item

import (
	"fmt"
	"net/http"

	"github.com/Rengemaru/equipment-management/internal/httpx"
)

// csvFormField は multipart のフィールド名。
const csvFormField = "file"

// registerImportRoutes はCSVインポートの経路を登録する。
func (h *Handler) registerImportRoutes(mux *http.ServeMux) {
	// プレビューは書き込まないが admin 限定にする。備品マスタを一括で作る
	// 操作の入口で、member に見せる意味がない。
	mux.Handle("POST /api/items/import/preview",
		h.requireAdmin(http.HandlerFunc(h.handleImportPreview)))
}

func (h *Handler) handleImportPreview(w http.ResponseWriter, r *http.Request) {
	// 本文の上限を先に決める。ParseMultipartForm はここを超えると
	// エラーになり、メモリを使い切る前に止まる。
	// multipart の包みぶんだけCSV本体より大きくなるので余裕を持たせる。
	r.Body = http.MaxBytesReader(w, r.Body, maxCSVBytes+(1<<20))

	file, _, err := r.FormFile(csvFormField)
	if err != nil {
		// 大きすぎて弾かれた時もここに来る。片方だけを言うと原因を探せない。
		httpx.WriteError(w, http.StatusBadRequest,
			fmt.Sprintf("CSVファイルを添付してください（%dMBまで）", maxCSVBytes>>20))
		return
	}
	defer func() { _ = file.Close() }()

	preview, err := h.store.PreviewImport(r.Context(), file)
	if err != nil {
		writeItemError(w, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{"preview": newImportPreviewResponse(preview)})
}

// importPreviewResponse はプレビューを返す形。
type importPreviewResponse struct {
	// RowCount は取り込めるCSVの行数。数量ぶんに展開されるため
	// RecordCount とは一致しない。両方を出して、その場で気付けるようにする。
	RowCount    int `json:"row_count"`
	RecordCount int `json:"record_count"`

	// CanImport は確定に進めるか。誤りが1件でもあれば false。
	CanImport bool `json:"can_import"`

	// CodeFrom と CodeTo は採番の予定。確定に進めない間は空。
	CodeFrom string `json:"code_from"`
	CodeTo   string `json:"code_to"`

	Rows   []importRowResponse   `json:"rows"`
	Errors []importErrorResponse `json:"errors"`
}

// importRowResponse は取り込む1行。
//
// 空欄に既定値を入れた後の値を返す。CSVに書いてある通りではなく、
// 実際に登録される内容を見せる。確定してから「状態が勝手に良好になった」と
// 言われる状態にしない。
type importRowResponse struct {
	// Line はCSV上の行番号。ヘッダを1行目として数える。
	// 表計算ソフトで同じ行を開けるようにするため、詰めた番号にしない。
	Line int `json:"line"`

	// Quantity はこの行から作るレコード数。
	Quantity int `json:"quantity"`

	Name      string    `json:"name"`
	Category  string    `json:"category"`
	Model     string    `json:"model"`
	Owner     Owner     `json:"owner"`
	IsFreeUse bool      `json:"is_free_use"`
	Location  string    `json:"location"`
	Condition Condition `json:"condition"`
	Note      string    `json:"note"`
}

// importErrorResponse は1行の誤り。
type importErrorResponse struct {
	Line    int    `json:"line"`
	Message string `json:"message"`
}

func newImportPreviewResponse(p *ImportPreview) importPreviewResponse {
	// 0件でも null ではなく [] を返す。フロント側で
	// 「null かもしれない」の分岐を書かせない。
	rows := make([]importRowResponse, 0, len(p.Rows))
	for _, row := range p.Rows {
		rows = append(rows, importRowResponse{
			Line:      row.Line,
			Quantity:  row.Quantity,
			Name:      row.Attributes.Name,
			Category:  row.Attributes.Category,
			Model:     row.Attributes.Model,
			Owner:     row.Attributes.Owner,
			IsFreeUse: row.Attributes.IsFreeUse,
			Location:  row.Attributes.Location,
			Condition: row.Attributes.Condition,
			Note:      row.Attributes.Note,
		})
	}

	errs := make([]importErrorResponse, 0, len(p.Errors))
	for _, e := range p.Errors {
		errs = append(errs, importErrorResponse{Line: e.Line, Message: e.Message})
	}

	return importPreviewResponse{
		RowCount:    len(p.Rows),
		RecordCount: p.RecordCount,
		CanImport:   p.CanImport(),
		CodeFrom:    p.FirstCode,
		CodeTo:      p.LastCode,
		Rows:        rows,
		Errors:      errs,
	}
}
