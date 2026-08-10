package item

import (
	"errors"
	"log"
	"net/http"

	"github.com/Rengemaru/equipment-management/internal/httpx"
)

// Middleware は経路に被せる認証・権限のミドルウェア。
//
// auth パッケージを直接参照しない。item が auth を参照し auth が item を
// 参照する形になると、片方を読むのにもう片方が要る。
type Middleware func(http.Handler) http.Handler

// Handler は備品まわりの HTTP ハンドラ。
type Handler struct {
	store *Store

	// requireLogin はログイン必須。member も通す。
	requireLogin Middleware

	// requireAdmin は admin 限定。備品マスタの書き込みに使う。
	//
	// member が備品マスタを書き換えられないことが、スプレッドシート共有ではなく
	// システム化する主目的（m1-spec §5）。UIで隠すだけにせず必ず通す。
	requireAdmin Middleware

	// photos は写真の保存先。
	photos *PhotoStore
}

// NewHandler は Handler を作る。
func NewHandler(store *Store, photos *PhotoStore, requireLogin, requireAdmin Middleware) *Handler {
	return &Handler{
		store:        store,
		photos:       photos,
		requireLogin: requireLogin,
		requireAdmin: requireAdmin,
	}
}

// Register は担当するルートを mux に登録する。
//
// 読み取りは member も可。誰が何を持っているかが全員に見える状態を作ることが、
// 罰則より強く働く（CLAUDE.md）。
func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("GET /api/items", h.requireLogin(http.HandlerFunc(h.handleList)))
	mux.Handle("GET /api/items/filters", h.requireLogin(http.HandlerFunc(h.handleFilters)))
	mux.Handle("GET /api/items/{code}", h.requireLogin(http.HandlerFunc(h.handleDetail)))

	h.registerWriteRoutes(mux)
	h.registerPhotoRoutes(mux)
}

// itemResponse は備品を返す形。
//
// Item をそのまま返さない。列を足した時に、意図しない値がAPIに現れる。
type itemResponse struct {
	ID             int64          `json:"id"`
	Code           string         `json:"code"`
	Name           string         `json:"name"`
	Category       string         `json:"category"`
	Model          string         `json:"model"`
	Owner          Owner          `json:"owner"`
	IsFreeUse      bool           `json:"is_free_use"`
	Location       string         `json:"location"`
	Condition      Condition      `json:"condition"`
	LocationStatus LocationStatus `json:"location_status"`
	// PhotoURL は写真の取得先。無ければ空文字。
	//
	// ファイル名（photo_path）をそのまま返さない。保存先の構成が変わるたびに
	// フロント側のURL組み立てを直すことになる。
	PhotoURL  string `json:"photo_url"`
	Note      string `json:"note"`
	UpdatedAt string `json:"updated_at"`
}

func newItemResponse(it *Item) itemResponse {
	return itemResponse{
		ID:             it.ID,
		Code:           it.Code,
		Name:           it.Name,
		Category:       it.Category,
		Model:          it.Model,
		Owner:          it.Owner,
		IsFreeUse:      it.IsFreeUse,
		Location:       it.Location,
		Condition:      it.Condition,
		LocationStatus: it.LocationStatus,
		PhotoURL:       photoURL(it),
		Note:           it.Note,
		UpdatedAt:      it.UpdatedAt,
	}
}

func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	f := Filter{
		Query:            q.Get("q"),
		Category:         q.Get("category"),
		Location:         q.Get("location"),
		Condition:        Condition(q.Get("condition")),
		LocationStatus:   LocationStatus(q.Get("location_status")),
		IncludeDiscarded: q.Get("include_discarded") == "1",
	}

	// 知らない値を黙って無視すると、絞り込んだつもりの全件が返る。
	// 「探しているものが無い」ではなく「条件が効いていない」に気付けるようにする。
	if f.Condition != "" && !f.Condition.Valid() {
		httpx.WriteError(w, http.StatusBadRequest, "状態の指定が不正です")
		return
	}
	if f.LocationStatus != "" && !f.LocationStatus.Valid() {
		httpx.WriteError(w, http.StatusBadRequest, "所在の指定が不正です")
		return
	}

	items, err := h.store.List(r.Context(), f)
	if err != nil {
		log.Printf("items: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
		return
	}

	// 0件でも null ではなく [] を返す。フロント側で
	// 「null かもしれない」の分岐を書かせない。
	list := make([]itemResponse, 0, len(items))
	for _, it := range items {
		list = append(list, newItemResponse(it))
	}

	httpx.JSON(w, http.StatusOK, map[string]any{"items": list})
}

func (h *Handler) handleDetail(w http.ResponseWriter, r *http.Request) {
	it, err := h.store.ByCode(r.Context(), r.PathValue("code"))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			// QRを読んで来た人が最初に見る画面。何が起きたか分かる文言にする。
			httpx.WriteError(w, http.StatusNotFound, "その備品コードは登録されていません")
			return
		}
		log.Printf("items: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{"item": newItemResponse(it)})
}

// handleFilters は絞り込みの選択肢を返す。
//
// 分類と保管場所は自由入力のため固定の一覧を持てない。
// フロントが全件を取得して集計する形にすると、絞り込みのたびに全件が要る。
func (h *Handler) handleFilters(w http.ResponseWriter, r *http.Request) {
	categories, err := h.store.Categories(r.Context())
	if err != nil {
		log.Printf("items: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
		return
	}

	locations, err := h.store.Locations(r.Context())
	if err != nil {
		log.Printf("items: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"categories": orEmpty(categories),
		"locations":  orEmpty(locations),
	})
}

// photoURL は写真の取得先を返す。
func photoURL(it *Item) string {
	if it.PhotoPath == "" {
		return ""
	}
	return "/api/items/" + it.Code + "/photo"
}

func orEmpty(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}
