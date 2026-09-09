package report

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/Rengemaru/equipment-management/internal/httpx"
	"github.com/Rengemaru/equipment-management/internal/item"
	"github.com/Rengemaru/equipment-management/internal/jst"
)

// Middleware は経路に被せる認証・権限のミドルウェア。
type Middleware func(http.Handler) http.Handler

// CurrentUser は要求を出した利用者のIDを返す。
//
// auth パッケージを直接参照しない。参照すると、context に利用者を入れる手段が
// auth の非公開関数だけになり、このパッケージのテストで「ログイン済みの誰か」を
// 作れなくなる。
type CurrentUser func(ctx context.Context) (int64, bool)

// Handler は報告まわりの HTTP ハンドラ。
type Handler struct {
	store *Store

	// items は応答に載せる備品を引くために使う。
	// 報告すると condition が変わるため、更新後の姿をその場で返す。
	items *item.Store

	currentUser  CurrentUser
	requireLogin Middleware

	// requireAdmin は追認の経路に使う。
	//
	// **UIで隠すだけにしない。** member が備品の状態を確定できないことは、
	// 権限をシステムで担保する目的そのもの。
	requireAdmin Middleware
}

// NewHandler は Handler を作る。
func NewHandler(store *Store, items *item.Store, currentUser CurrentUser, requireLogin, requireAdmin Middleware) *Handler {
	return &Handler{
		store:        store,
		items:        items,
		currentUser:  currentUser,
		requireLogin: requireLogin,
		requireAdmin: requireAdmin,
	}
}

// Register は担当するルートを mux に登録する。
func (h *Handler) Register(mux *http.ServeMux) {
	// 報告は全員できる。報告のハードルを上げると、壊れたまま次の人が借りる。
	mux.Handle("POST /api/items/{code}/damages", h.requireLogin(http.HandlerFunc(h.handleReport)))

	// 追認は admin だけ。
	mux.Handle("GET /api/damages", h.requireAdmin(http.HandlerFunc(h.handleList)))
	mux.Handle("POST /api/damages/{id}/status", h.requireAdmin(http.HandlerFunc(h.handleSetStatus)))
}

// reportRequest は破損報告の入力。
type reportRequest struct {
	// Description は破損箇所の説明。必須。
	Description string `json:"description"`
}

func (h *Handler) handleReport(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.currentUser(r.Context())
	if !ok {
		log.Print("handleReport: 利用者を取り出せない")
		httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
		return
	}

	var req reportRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	code := r.PathValue("code")
	d, err := h.store.Report(r.Context(), code, userID, req.Description)
	if err != nil {
		switch {
		case errors.Is(err, ErrItemNotFound):
			httpx.WriteError(w, http.StatusNotFound, ErrItemNotFound.Error())

		case errors.Is(err, ErrEmptyDescription):
			httpx.WriteErrorCode(w, http.StatusBadRequest, "empty_description", ErrEmptyDescription.Error())

		default:
			log.Printf("破損報告: %v", err)
			httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
		}
		return
	}

	it, err := h.items.ByCode(r.Context(), code)
	if err != nil {
		log.Printf("破損報告後の備品取得: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
		return
	}

	httpx.JSON(w, http.StatusCreated, map[string]any{
		"report": newDamageResponse(d),
		"item":   item.NewResponse(it),
	})
}

func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	status := Status(r.URL.Query().Get("status"))

	list, err := h.store.List(r.Context(), status)
	if err != nil {
		if errors.Is(err, ErrInvalidStatus) {
			httpx.WriteError(w, http.StatusBadRequest, ErrInvalidStatus.Error())
			return
		}
		log.Printf("破損報告の一覧: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
		return
	}

	reports := make([]damageResponse, 0, len(list))
	for _, d := range list {
		reports = append(reports, newDamageResponse(d))
	}

	httpx.JSON(w, http.StatusOK, map[string]any{"reports": reports})
}

// statusRequest は追認の入力。
type statusRequest struct {
	Status Status `json:"status"`
	Note   string `json:"note"`
}

// handleSetStatus は運営の追認を記録する。
//
// 経路を PUT /api/damages/{id} にしていないのは、M1 の
// POST /api/items/{code}/discard と形を揃えるため。
// **追認は「全項目を送り直す更新」ではなく、1つの状態遷移。**
func (h *Handler) handleSetStatus(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.currentUser(r.Context())
	if !ok {
		log.Print("handleSetStatus: 利用者を取り出せない")
		httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
		return
	}

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, ErrNotFound.Error())
		return
	}

	var req statusRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	d, err := h.store.SetStatus(r.Context(), id, userID, req.Status, req.Note)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			httpx.WriteError(w, http.StatusNotFound, ErrNotFound.Error())

		case errors.Is(err, ErrInvalidStatus):
			httpx.WriteError(w, http.StatusBadRequest, ErrInvalidStatus.Error())

		default:
			log.Printf("破損報告の更新: %v", err)
			httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
		}
		return
	}

	it, err := h.items.ByID(r.Context(), d.ItemID)
	if err != nil {
		log.Printf("追認後の備品取得: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"report": newDamageResponse(d),
		"item":   item.NewResponse(it),
	})
}

// userRef は応答に載せる人。
type userRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// itemRef は応答に載せる備品。詳細は "item" 側に入る。
type itemRef struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// damageResponse は破損報告を返す形。
//
// Damage をそのまま返さない。列を足した時に、意図しない値がAPIに現れる。
type damageResponse struct {
	ID   int64   `json:"id"`
	Item itemRef `json:"item"`

	// LoanID は貸出中に報告された場合の貸出。在庫中の報告では null。
	LoanID *int64 `json:"loan_id"`

	Reporter    userRef `json:"reporter"`
	ReportedAt  string  `json:"reported_at"`
	Description string  `json:"description"`

	Status      Status   `json:"status"`
	ConfirmedBy *userRef `json:"confirmed_by"`
	ConfirmedAt *string  `json:"confirmed_at"`
	Note        string   `json:"note"`
}

func newDamageResponse(d *Damage) damageResponse {
	res := damageResponse{
		ID:          d.ID,
		Item:        itemRef{Code: d.ItemCode, Name: d.ItemName},
		LoanID:      d.LoanID,
		Reporter:    userRef{ID: d.Reporter.ID, Name: d.Reporter.Name},
		ReportedAt:  formatTime(d.ReportedAt),
		Description: d.Description,
		Status:      d.Status,
		Note:        d.Note,
	}

	if d.ConfirmedBy != nil {
		res.ConfirmedBy = &userRef{ID: d.ConfirmedBy.ID, Name: d.ConfirmedBy.Name}
	}
	if d.ConfirmedAt != nil {
		s := formatTime(*d.ConfirmedAt)
		res.ConfirmedAt = &s
	}

	return res
}

// formatTime はJSTの RFC3339 にする。
func formatTime(t time.Time) string {
	return t.In(jst.Zone).Format(time.RFC3339)
}
