package item

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/Rengemaru/equipment-management/internal/httpx"
	"github.com/Rengemaru/equipment-management/internal/label"
)

// labelFilename はブラウザに渡すファイル名。
const labelFilename = "labels.pdf"

// registerLabelRoutes はラベル印刷の経路を登録する。
func (h *Handler) registerLabelRoutes(mux *http.ServeMux) {
	// admin 限定。ラベルを刷るのは備品を棚に並べる側の作業で、
	// member に開く理由がない。
	//
	// 経路の末尾を .pdf にしているのは、ブラウザとプリンタが素直に
	// PDFとして扱うため（export.csv と同じ理由）。
	mux.Handle("GET /api/labels.pdf",
		h.requireAdmin(http.HandlerFunc(h.handleLabels)))
}

// handleLabels は指定範囲のラベルシートをPDFで返す。
//
//	GET /api/labels.pdf?from=0001&to=0050&category=撮影機材
//
// 指定を省いた項目は条件にしない。全て省けば全件になる。
func (h *Handler) handleLabels(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	from, err := parseCodeParam(q.Get("from"))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "開始の備品コードの指定が不正です")
		return
	}
	to, err := parseCodeParam(q.Get("to"))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "終了の備品コードの指定が不正です")
		return
	}

	// 逆順の指定は0件になる。0件と同じ扱いにすると「その範囲に備品が無い」と
	// 読めてしまい、打ち間違いに気付けない。
	if from > 0 && to > 0 && from > to {
		httpx.WriteError(w, http.StatusBadRequest, "備品コードの範囲が逆です")
		return
	}

	// 廃棄済みは含めない（既定の除外のまま）。廃棄は物理削除の代わりで、
	// 棚に並ばないものにラベルを刷る理由がない。
	// 自由利用品は含める。M1のラベルは備品詳細を見るためのQRであって、
	// 貸出の入口ではない。
	items, err := h.store.List(r.Context(), Filter{
		Category: q.Get("category"),
		CodeFrom: from,
		CodeTo:   to,
	})
	if err != nil {
		log.Printf("labels: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
		return
	}

	labels := make([]label.Label, 0, len(items))
	for _, it := range items {
		labels = append(labels, label.Label{Code: it.Code, Name: it.Name})
	}

	pdf, err := label.PDF(h.hostURL, labels)
	if err != nil {
		if errors.Is(err, label.ErrNoLabels) {
			httpx.WriteError(w, http.StatusBadRequest, "指定した条件に合う備品がありません")
			return
		}
		log.Printf("labels: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
		return
	}

	// 全て組み立ててからヘッダを書く。書き始めてから失敗すると、200 のまま
	// 途中までのPDFが渡り、欠けたシートを刷ることになる。
	w.Header().Set("Content-Type", "application/pdf")

	// attachment ではなく inline。ラベルシールは刷り直しが効かないため、
	// ブラウザのビューアで確認してから印刷できる形にする。
	w.Header().Set("Content-Disposition", `inline; filename="`+labelFilename+`"`)

	if _, err := w.Write(pdf); err != nil {
		// 既に応答が始まっており、状態コードを変えられない。
		log.Printf("labels: PDFの送信: %v", err)
	}
}

// parseCodeParam は備品コードの範囲指定を数値にする。空なら0（指定なし）。
//
// "0042" でも "42" でも受ける。画面から貼り付けるのはラベルに印字された
// ゼロ埋めの形だが、手で打つ人は先頭の0を落とす。
func parseCodeParam(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}

	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	// 採番は 0001 から。0 は「指定なし」と区別が付かず、負の値は存在しない。
	if n < 1 {
		return 0, errors.New("備品コードが範囲外")
	}

	return n, nil
}
