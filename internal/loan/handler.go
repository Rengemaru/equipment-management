package loan

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Rengemaru/equipment-management/internal/httpx"
	"github.com/Rengemaru/equipment-management/internal/item"
	"github.com/Rengemaru/equipment-management/internal/jst"
)

// Middleware は経路に被せる認証・権限のミドルウェア。
type Middleware func(http.Handler) http.Handler

// Actor は要求を出した利用者。
//
// 取り消しの可否に役割が要るため、IDだけでは足りない。
type Actor struct {
	ID      int64
	IsAdmin bool
}

// CurrentUser は要求を出した利用者を返す。
//
// auth パッケージを直接参照しない。参照すると、context に利用者を入れる手段が
// auth の非公開関数だけになり、このパッケージのテストで「ログイン済みの誰か」を
// 作れなくなる。取り出し方だけを外から渡してもらう。
type CurrentUser func(ctx context.Context) (Actor, bool)

// Notifier はメールを送る。notify.Mailer.Send をそのまま渡せる形にしている。
//
// notify パッケージを参照しないのは、テストで送信内容を捕まえるため。
// **送信先が未設定なら notify 側が何もせず nil を返す。**
// ここに「設定されていれば送る」という分岐を書かない（書き忘れた経路ができる）。
type Notifier func(ctx context.Context, to []string, subject, body string) error

// Handler は貸出まわりの HTTP ハンドラ。
type Handler struct {
	store *Store

	// items は応答に載せる備品を引くために使う。
	//
	// 借用の結果として備品の状態が変わり得る（M2-2 の在庫への自動復帰）。
	// 画面に再取得させず、更新後の姿をその場で返す。
	items *item.Store

	// notify は代理登録を本人へ知らせる。
	notify Notifier

	// hostURL は通知に載せる訂正先URLの土台（config.HostURL）。
	hostURL string

	currentUser  CurrentUser
	requireLogin Middleware
}

// NewHandler は Handler を作る。
func NewHandler(store *Store, items *item.Store, notify Notifier, hostURL string, currentUser CurrentUser, requireLogin Middleware) *Handler {
	return &Handler{
		store:        store,
		items:        items,
		notify:       notify,
		hostURL:      hostURL,
		currentUser:  currentUser,
		requireLogin: requireLogin,
	}
}

// Register は担当するルートを mux に登録する。
func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("POST /api/items/{code}/loans", h.requireLogin(http.HandlerFunc(h.handleBorrow)))
	mux.Handle("POST /api/items/{code}/return", h.requireLogin(http.HandlerFunc(h.handleReturn)))
	mux.Handle("POST /api/loans/{id}/cancel", h.requireLogin(http.HandlerFunc(h.handleCancel)))

	// 貸出中一覧は全メンバーが見られる。誰が何を持っているかが
	// 全員に見える状態を作ることが、罰則より強く働く。
	mux.Handle("GET /api/loans", h.requireLogin(http.HandlerFunc(h.handleList)))
	mux.Handle("GET /api/loans/mine", h.requireLogin(http.HandlerFunc(h.handleMine)))
}

// handleList は貸出中の一覧を返す。
func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	f := Filter{
		Query: q.Get("q"),
		// overdue は値を見ない。付いていれば絞る。
		OverdueOnly: q.Has("overdue") && q.Get("overdue") != "0",
	}
	if v := q.Get("user_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "user_id の指定が不正")
			return
		}
		f.UserID = id
	}

	list, err := h.store.ListActive(r.Context(), f)
	if err != nil {
		log.Printf("貸出中一覧: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{"loans": h.responses(list)})
}

// handleMine は自分の貸出中と履歴を1回で返す。
//
// 2回に分けると、片方だけ更新された表示が出る。
func (h *Handler) handleMine(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.currentUser(r.Context())
	if !ok {
		log.Print("handleMine: 利用者を取り出せない")
		httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
		return
	}

	limit := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			httpx.WriteError(w, http.StatusBadRequest, "limit の指定が不正")
			return
		}
		limit = n
	}

	active, returned, err := h.store.ListByUser(r.Context(), actor.ID, limit)
	if err != nil {
		log.Printf("自分の貸出: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"active":   h.responses(active),
		"returned": h.responses(returned),
	})
}

// responses は一覧を応答の形に直す。
func (h *Handler) responses(list []*Loan) []loanResponse {
	at := now()

	out := make([]loanResponse, 0, len(list))
	for _, l := range list {
		out = append(out, newLoanResponse(l, at))
	}
	return out
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
	actor, ok := h.currentUser(r.Context())
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
	l, err := h.store.Borrow(r.Context(), code, actor.ID, Request{
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

	h.notifyProxyBorrow(r.Context(), l)

	httpx.JSON(w, http.StatusCreated, map[string]any{
		"loan": newLoanResponse(l, now()),
		"item": item.NewResponse(it),
	})
}

// handleReturn は返却を記録する。
//
// **本文を読まない。** 返却に入力を足さない（確認ダイアログも出さない）。
// 誤返却は再度借りれば済むが、入力を求めると記録そのものが飛ぶ。
func (h *Handler) handleReturn(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.currentUser(r.Context())
	if !ok {
		log.Print("handleReturn: 利用者を取り出せない")
		httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
		return
	}

	code := r.PathValue("code")
	l, err := h.store.Return(r.Context(), code, actor.ID)
	if err != nil {
		switch {
		case errors.Is(err, ErrItemNotFound):
			httpx.WriteError(w, http.StatusNotFound, ErrItemNotFound.Error())

		case errors.Is(err, ErrNotBorrowed):
			// 二重タップと、他の人が先に返した場合の両方がここに来る。
			// 200 で黙って成功にしない。画面は 409 を受けて読み直せばよく、
			// 「返した気になっているが記録が無い」より遥かに良い。
			httpx.WriteErrorCode(w, http.StatusConflict, "not_borrowed", "この備品は貸出中ではありません")

		default:
			log.Printf("返却の登録: %v", err)
			httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
		}
		return
	}

	it, err := h.items.ByCode(r.Context(), code)
	if err != nil {
		log.Printf("返却後の備品取得: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"loan": newLoanResponse(l, now()),
		"item": item.NewResponse(it),
	})
}

// notifyProxyBorrow は代理登録された借用者本人へ知らせる。
//
// **送信の失敗で借用を巻き戻さない。** 通知が送れなかったことを理由に記録を消すと、
// 最も避けたい「記録が無い」状態に戻る。ログに残して先へ進む
// （docs/m2-implementation-spec.md §4）。
func (h *Handler) notifyProxyBorrow(ctx context.Context, l *Loan) {
	// 自分で自分の借用を登録した場合は送らない。
	if !l.IsProxy() {
		return
	}
	// メールアドレスは任意項目。未設定の人が居るのは異常ではない。
	if l.User.Email == "" {
		return
	}

	subject := "[備品] あなたの名前で借用が登録されました"
	body := strings.Join([]string{
		l.User.Name + " さん",
		"",
		l.RegisteredBy.Name + " さんが、あなたの名前で借用を登録しました。",
		"",
		"備品: " + l.ItemCode + " " + l.ItemName,
		"借用日時: " + formatTime(l.BorrowedAt),
		"返却予定日: " + l.DueDate,
		"",
		"身に覚えがない場合は、次のページから取り消してください。",
		h.hostURL + "/loans/mine",
		"",
	}, "\n")

	if err := h.notify(ctx, []string{l.User.Email}, subject, body); err != nil {
		log.Printf("代理登録の通知（貸出 %d）: %v", l.ID, err)
	}
}

// cancelRequest は取り消しの入力。
type cancelRequest struct {
	// Reason は任意。必須にすると、他人の誤登録を直す側に手間を課すことになる。
	Reason string `json:"reason"`
}

// handleCancel は誤登録の貸出を取り消す。
func (h *Handler) handleCancel(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.currentUser(r.Context())
	if !ok {
		log.Print("handleCancel: 利用者を取り出せない")
		httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
		return
	}

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, ErrNotFound.Error())
		return
	}

	var req cancelRequest
	if r.ContentLength != 0 {
		if err := httpx.DecodeJSON(w, r, &req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	l, err := h.store.Cancel(r.Context(), id, actor, req.Reason)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			httpx.WriteError(w, http.StatusNotFound, ErrNotFound.Error())

		case errors.Is(err, ErrForbidden):
			httpx.WriteError(w, http.StatusForbidden, ErrForbidden.Error())

		case errors.Is(err, ErrAlreadyReturned):
			httpx.WriteErrorCode(w, http.StatusConflict, "already_returned", ErrAlreadyReturned.Error())

		case errors.Is(err, ErrAlreadyCancelled):
			httpx.WriteErrorCode(w, http.StatusConflict, "already_cancelled", ErrAlreadyCancelled.Error())

		default:
			log.Printf("貸出の取り消し: %v", err)
			httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
		}
		return
	}

	it, err := h.items.ByCode(r.Context(), l.ItemCode)
	if err != nil {
		log.Printf("取り消し後の備品取得: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
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
