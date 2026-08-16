package item

import (
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"github.com/Rengemaru/equipment-management/internal/httpx"
)

const (
	// csvFormField は multipart のフィールド名。
	csvFormField = "file"

	// excludeFormField は取り込まない行を指す multipart のフィールド名。
	excludeFormField = "exclude_lines"
)

// registerImportRoutes はCSVインポートの経路を登録する。
func (h *Handler) registerImportRoutes(mux *http.ServeMux) {
	// プレビューは書き込まないが admin 限定にする。備品マスタを一括で作る
	// 操作の入口で、member に見せる意味がない。
	mux.Handle("POST /api/items/import/preview",
		h.requireAdmin(http.HandlerFunc(h.handleImportPreview)))

	mux.Handle("POST /api/items/import",
		h.requireAdmin(http.HandlerFunc(h.handleImport)))
}

func (h *Handler) handleImportPreview(w http.ResponseWriter, r *http.Request) {
	file, ok := openCSV(w, r)
	if !ok {
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

func (h *Handler) handleImport(w http.ResponseWriter, r *http.Request) {
	file, ok := openCSV(w, r)
	if !ok {
		return
	}
	defer func() { _ = file.Close() }()

	// FormFile が成功していれば MultipartForm は読める。
	lines, err := parseExcludeLines(r.MultipartForm.Value[excludeFormField])
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := h.store.Import(r.Context(), file, ImportOptions{ExcludeLines: lines})
	if err != nil {
		writeImportError(w, err)
		return
	}

	httpx.JSON(w, http.StatusCreated, map[string]any{
		"result": importResultResponse{
			RecordCount: result.RecordCount,
			CodeFrom:    result.FirstCode,
			CodeTo:      result.LastCode,
		},
	})
}

// openCSV は添付されたCSVを開く。開けなければ応答を書いて false を返す。
func openCSV(w http.ResponseWriter, r *http.Request) (multipart.File, bool) {
	// 本文の上限を先に決める。ParseMultipartForm はここを超えると
	// エラーになり、メモリを使い切る前に止まる。
	// multipart の包みぶんだけCSV本体より大きくなるので余裕を持たせる。
	r.Body = http.MaxBytesReader(w, r.Body, maxCSVBytes+(1<<20))

	file, _, err := r.FormFile(csvFormField)
	if err != nil {
		// 大きすぎて弾かれた時もここに来る。片方だけを言うと原因を探せない。
		httpx.WriteError(w, http.StatusBadRequest,
			fmt.Sprintf("CSVファイルを添付してください（%dMBまで）", maxCSVBytes>>20))
		return nil, false
	}

	return file, true
}

// parseExcludeLines は除外する行番号を読む。
//
// `2,3` のようにまとめた形と、フィールドを繰り返す形の両方を受ける。
// 送り方の違いだけで取り込めないのは、原因が分かりにくい割に得るものがない。
func parseExcludeLines(values []string) ([]int, error) {
	var lines []int

	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}

			n, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("除外する行の指定が数値ではありません: %q", part)
			}
			lines = append(lines, n)
		}
	}

	return lines, nil
}

// writeImportError は取り込み固有のエラーを応答に変える。
func writeImportError(w http.ResponseWriter, err error) {
	var rowsErr *ImportRowsError
	if errors.As(err, &rowsErr) {
		// 行ごとの内容はここでは返さない。エラーの形は httpx.ErrorResponse の
		// 1つに決めてあり、経路ごとに違う形を足すとフロントがエンドポイントの
		// 数だけ分岐を持つことになる。行の中身はプレビューが返す。
		httpx.WriteErrorCode(w, http.StatusBadRequest, "csv_invalid",
			rowsErr.Error()+"。プレビューで内容を確認してください")
		return
	}

	writeItemError(w, err)
}

// importResultResponse は取り込みの結果を返す形。
type importResultResponse struct {
	// RecordCount は登録したレコード数。
	RecordCount int `json:"record_count"`

	// CodeFrom と CodeTo は実際に採番された範囲。予定ではなく確定した値で、
	// そのままラベルの印刷範囲に使える。
	CodeFrom string `json:"code_from"`
	CodeTo   string `json:"code_to"`
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
