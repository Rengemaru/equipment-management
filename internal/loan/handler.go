package loan

import (
	"context"
	"errors"
	"log"
	"net/http"
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
// 作れなくなる。取り出し方だけを外から渡してもらう。
type CurrentUser func(ctx context.Context) (int64, bool)

// Handler は貸出まわりの HTTP ハンドラ。
type Handler struct {
	store *Store

	// items は応答に載せる備品を引くために使う。
	//
	// 借用の結果として備品の状態が変わり得る（M2-2 の在庫への自動復帰）。
	// 画面に再取得させず、更新後の姿をその場で返す。
	items *item.Store

	currentUser  CurrentUser
	requireLogin Middleware
}

// NewHandler は Handler を作る。
func NewHandler(store *Store, items *item.Store, currentUser CurrentUser, requireLogin Middleware) *Handler {
	return &Handler{
		store:        store,
		items:        items,
		currentUser:  currentUser,
		requireLogin: requireLogin,
	}
}

// Register は担当するルートを mux に登録する。
func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("POST /api/items/{code}/loans", h.requireLogin(http.HandlerFunc(h.handleBorrow)))
}

// borrowRequest は借用の入力。**全項目が省略できる。**
//
// 空の本文で自分の借用が成立することが、3タップで終わらせる前提になる。
type borrowRequest struct {
	// UserID は借用者。省略すると自分。
	UserID int64 `json:"user_id"`

	// DueDate は 'YYYY-MM-DD'。省略すると借用日+14日。
	DueDate string `json:"due_date"`

	// BorrowedAt は RFC3339。省略すると現在時刻。
	BorrowedAt string `json:"borrowed_at"`

	Note string `json:"note"`
}

func (h *Handler) handleBorrow(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.currentUser(r.Context())
	if !ok {
		// requireLogin を通っている以上ここには来ない。来たなら経路の組み立てが壊れている。
		log.Print("handleBorrow: 利用者を取り出せない")
		httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
		return
	}

	var req borrowRequest
	// 本文なしを許す。fetch でボディを付けずに叩けることが、
	// 画面側の実装を1行短くする。
	if r.ContentLength != 0 {
		if err := httpx.DecodeJSON(w, r, &req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	borrowedAt, err := parseBorrowedAt(req.BorrowedAt)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	code := r.PathValue("code")
	l, err := h.store.Borrow(r.Context(), code, userID, Request{
		UserID:     req.UserID,
		DueDate:    req.DueDate,
		BorrowedAt: borrowedAt,
		Note:       req.Note,
	})
	if err != nil {
		writeBorrowError(w, err)
		return
	}

	it, err := h.items.ByCode(r.Context(), code)
	if err != nil {
		log.Printf("借用後の備品取得: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
		return
	}

	httpx.JSON(w, http.StatusCreated, map[string]any{
		"loan": newLoanResponse(l, now()),
		"item": item.NewResponse(it),
	})
}

// parseBorrowedAt は借用日時を読む。空ならゼロ値（＝現在時刻）。
func parseBorrowedAt(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}

	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, errors.New("借用日時の形式が不正（例: 2026-09-10T19:30:00+09:00）")
	}
	return t, nil
}

// writeBorrowError は借用が成立しない理由を status と code に振り分ける。
//
// code は文言ではなく識別子。日本語を直した瞬間にフロントの分岐が壊れる形にしない。
func writeBorrowError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrItemNotFound):
		httpx.WriteError(w, http.StatusNotFound, ErrItemNotFound.Error())

	case errors.Is(err, ErrFreeUse):
		httpx.WriteErrorCode(w, http.StatusBadRequest, "free_use", ErrFreeUse.Error())

	case errors.Is(err, ErrDiscarded):
		httpx.WriteErrorCode(w, http.StatusBadRequest, "discarded", ErrDiscarded.Error())

	case errors.Is(err, ErrAlreadyBorrowed):
		// 競合を 500 にしない。2人が同時に同じ備品を借りようとしただけで、
		// 画面は「他の人が先に借りました」と出して読み直せばよい。
		httpx.WriteErrorCode(w, http.StatusConflict, "already_borrowed", "他の人が先に借りました")

	case errors.Is(err, ErrInvalidUser):
		httpx.WriteErrorCode(w, http.StatusBadRequest, "invalid_user", ErrInvalidUser.Error())

	case errors.Is(err, ErrInvalidDueDate):
		httpx.WriteErrorCode(w, http.StatusBadRequest, "invalid_due_date", "返却予定日が借用日より前です")

	case errors.Is(err, ErrFutureBorrowedAt):
		httpx.WriteErrorCode(w, http.StatusBadRequest, "future_borrowed_at", ErrFutureBorrowedAt.Error())

	default:
		log.Printf("借用の登録: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
	}
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

// loanResponse は貸出を返す形。
//
// Loan をそのまま返さない。列を足した時に、意図しない値がAPIに現れる。
type loanResponse struct {
	ID           int64   `json:"id"`
	Item         itemRef `json:"item"`
	User         userRef `json:"user"`
	RegisteredBy userRef `json:"registered_by"`

	// IsProxy は代理登録か。サーバで計算して返す。
	// 画面に2つのIDを比べさせると、比べ忘れた画面ができる。
	IsProxy bool `json:"is_proxy"`

	// BorrowedAt / ReturnedAt は RFC3339（JST）。
	// new Date() にそのまま渡せる形にする。
	BorrowedAt string   `json:"borrowed_at"`
	DueDate    string   `json:"due_date"`
	ReturnedAt *string  `json:"returned_at"`
	ReturnedBy *userRef `json:"returned_by"`

	OverdueDays int    `json:"overdue_days"`
	Note        string `json:"note"`
}

func newLoanResponse(l *Loan, at time.Time) loanResponse {
	res := loanResponse{
		ID:           l.ID,
		Item:         itemRef{Code: l.ItemCode, Name: l.ItemName},
		User:         userRef{ID: l.User.ID, Name: l.User.Name},
		RegisteredBy: userRef{ID: l.RegisteredBy.ID, Name: l.RegisteredBy.Name},
		IsProxy:      l.IsProxy(),
		BorrowedAt:   formatTime(l.BorrowedAt),
		DueDate:      l.DueDate,
		OverdueDays:  l.OverdueDays(at),
		Note:         l.Note,
	}

	if l.ReturnedAt != nil {
		s := formatTime(*l.ReturnedAt)
		res.ReturnedAt = &s
	}
	if l.ReturnedBy != nil {
		res.ReturnedBy = &userRef{ID: l.ReturnedBy.ID, Name: l.ReturnedBy.Name}
	}

	return res
}

// formatTime はJSTの RFC3339 にする。
func formatTime(t time.Time) string {
	return t.In(jst.Zone).Format(time.RFC3339)
}
