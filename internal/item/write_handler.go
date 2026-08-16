package item

import (
	"errors"
	"log"
	"net/http"

	"github.com/Rengemaru/equipment-management/internal/httpx"
)

// itemRequest は登録・更新の入力。
//
// Code は更新の時だけ意味を持つ。画面が取得した内容をそのまま送り返せるように
// 受け取るが、値は経路のコードと一致していなければならない。
// 変更を許すと、ラベルと実物の対応が壊れる。
type itemRequest struct {
	Code           string         `json:"code"`
	Name           string         `json:"name"`
	Category       string         `json:"category"`
	Model          string         `json:"model"`
	Owner          Owner          `json:"owner"`
	IsFreeUse      bool           `json:"is_free_use"`
	Location       string         `json:"location"`
	Condition      Condition      `json:"condition"`
	LocationStatus LocationStatus `json:"location_status"`
	Note           string         `json:"note"`
}

func (r itemRequest) attributes() Attributes {
	return Attributes{
		Name:           r.Name,
		Category:       r.Category,
		Model:          r.Model,
		Owner:          r.Owner,
		IsFreeUse:      r.IsFreeUse,
		Location:       r.Location,
		Condition:      r.Condition,
		LocationStatus: r.LocationStatus,
		Note:           r.Note,
	}
}

// registerWriteRoutes は admin 専用の経路を登録する。
func (h *Handler) registerWriteRoutes(mux *http.ServeMux) {
	mux.Handle("POST /api/items", h.requireAdmin(http.HandlerFunc(h.handleCreate)))
	mux.Handle("PUT /api/items/{code}", h.requireAdmin(http.HandlerFunc(h.handleUpdate)))

	// DELETE は用意しない。物理削除すると貸出履歴の参照先が消える。
	// 「廃棄」という状態にする操作であることを経路に出す。
	mux.Handle("POST /api/items/{code}/discard", h.requireAdmin(http.HandlerFunc(h.handleDiscard)))
}

func (h *Handler) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req itemRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 登録時は採番するので、コードを指定させない。
	if req.Code != "" {
		httpx.WriteError(w, http.StatusBadRequest,
			"備品コードは自動で採番されます。指定しないでください")
		return
	}

	it, err := h.store.Create(r.Context(), req.attributes())
	if err != nil {
		writeItemError(w, err)
		return
	}

	httpx.JSON(w, http.StatusCreated, map[string]any{"item": newItemResponse(it)})
}

func (h *Handler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")

	var req itemRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 画面から往復した内容をそのまま送れるように受け取るが、変更は許さない。
	// 変えられると、既に貼ってあるラベルと実物の対応が壊れる。
	if req.Code != "" && req.Code != code {
		httpx.WriteError(w, http.StatusBadRequest, "備品コードは変更できません")
		return
	}

	it, err := h.store.Update(r.Context(), code, req.attributes())
	if err != nil {
		writeItemError(w, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{"item": newItemResponse(it)})
}

func (h *Handler) handleDiscard(w http.ResponseWriter, r *http.Request) {
	it, err := h.store.Discard(r.Context(), r.PathValue("code"))
	if err != nil {
		writeItemError(w, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{"item": newItemResponse(it)})
}

// writeItemError は Store のエラーを応答に変える。
//
// 1箇所にまとめる。ハンドラごとに書くと、いつか内部エラーの詳細を
// そのまま返す経路ができる。
func writeItemError(w http.ResponseWriter, err error) {
	var validationErr *validationError

	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "その備品コードは登録されていません")

	case errors.Is(err, ErrDuplicateCode):
		httpx.WriteError(w, http.StatusConflict, ErrDuplicateCode.Error())

	case errors.As(err, &validationErr):
		httpx.WriteError(w, http.StatusBadRequest, err.Error())

	default:
		log.Printf("items: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
	}
}
