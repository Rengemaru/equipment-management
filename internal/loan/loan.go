// Package loan は貸出の記録を扱う。
//
// 借用の記録漏れは致命的（誰が持っているか永久に不明）、返却の記録漏れは
// 自己修復する（本人に聞けば解決）。作り込みは借用側に厚く配分する（CLAUDE.md）。
//
// 貸出中かどうかは loans から導出する。items に「貸出中フラグ」を持たせない。
// 二重管理は必ず不整合を起こす。
package loan

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Rengemaru/equipment-management/internal/db"
	"github.com/Rengemaru/equipment-management/internal/jst"
)

// sqliteTimeLayout は datetime('now') が返す形式。保存はUTC。
const sqliteTimeLayout = "2006-01-02 15:04:05"

// defaultLoanDays は返却予定日の既定値（借用日+14日）。
//
// 日付を必須入力にしない。ここが摩擦の最大の発生源になる
// （docs/m2-implementation-spec.md §3）。
const defaultLoanDays = 14

// items の列に入る値。
//
// item パッケージの定数を参照しない。M2-7 で備品詳細が貸出を参照するため、
// 逆向きの依存をここで作ると循環する。DBの CHECK 制約と対で保つこと。
const (
	conditionDiscarded = "廃棄"
	locationInStock    = "在庫"
)

// 借用が成立しない理由。ハンドラが status と code に振り分ける。
//
// まとめて1つのエラーにしない。理由ごとに利用者への説明が変わり、
// 「他の人が先に借りました」と「廃棄済みです」を同じ扱いにできない。
var (
	// ErrItemNotFound は備品が無い。
	ErrItemNotFound = errors.New("備品が見つからない")

	// ErrFreeUse は自由利用品。追跡対象を減らすことが遵守率を上げる最短経路で、
	// 自由利用品は貸出フローから完全に除外する（CLAUDE.md）。
	ErrFreeUse = errors.New("自由利用品は借用の記録が要らない")

	// ErrDiscarded は廃棄済み。
	ErrDiscarded = errors.New("廃棄済みの備品は借りられない")

	// ErrAlreadyBorrowed は既に誰かが借りている。
	//
	// 競合は異常ではない。2人が同時に同じ三脚を借りようとしただけ。
	ErrAlreadyBorrowed = errors.New("他の人が先に借りている")

	// ErrInvalidUser は借用者に指定された利用者が居ない、または無効。
	ErrInvalidUser = errors.New("その利用者は選べない")

	// ErrInvalidDueDate は返却予定日が読めない、または借用日より前。
	ErrInvalidDueDate = errors.New("返却予定日の指定が不正")

	// ErrNotBorrowed は貸出中でない備品を返そうとしたこと。
	//
	// 二重タップと、他の人が先に返した場合の両方でここに来る。
	// 黙って成功にしない。「返した気になっているが記録が無い」状態を作らない。
	ErrNotBorrowed = errors.New("この備品は貸出中ではない")

	// ErrForbidden は取り消す権限が無いこと。
	//
	// 取り消せるのは借用者本人・登録者・admin の3者。無関係の人が
	// 他人の貸出を消せると、記録の信頼性が根本から崩れる。
	ErrForbidden = errors.New("この貸出を取り消す権限が無い")

	// ErrAlreadyReturned は返却済みの貸出を取り消そうとしたこと。
	//
	// 返却済みの履歴は消せない。取り消しは「そもそも借りていない」を表すもので、
	// 実際に借りて返した事実を後から無かったことにする手段ではない。
	ErrAlreadyReturned = errors.New("返却済みの貸出は取り消せない")

	// ErrAlreadyCancelled は取り消し済み。
	ErrAlreadyCancelled = errors.New("この貸出は取り消し済み")

	// ErrFutureBorrowedAt は借用日時が未来。
	//
	// 事後登録（過去に持ち出した分の記録）は許すが、未来は許さない。
	// 未来の借用は「これから借りる」の言い換えでしかなく、
	// 記録としては借りた時点で入れ直すべきもの。
	ErrFutureBorrowedAt = errors.New("借用日時に未来は指定できない")
)

// UserRef は貸出に関わる人。名前まで持つのは、一覧が毎回 users を
// 引き直さずに済むようにするため。
type UserRef struct {
	ID   int64
	Name string

	// Email は借用者にだけ入る（代理登録の通知に使う）。未設定なら空文字。
	//
	// **APIの応答に載せないこと。** loanResponse は ID と Name だけを写す。
	Email string
}

// Loan は1件の貸出。返却しても行を削除しない。
// 破損・紛失の追跡はこの履歴が唯一の根拠になる（CLAUDE.md）。
type Loan struct {
	ID       int64
	ItemID   int64
	ItemCode string
	ItemName string

	// User は借用者、RegisteredBy は登録者。代理登録では異なる。
	User         UserRef
	RegisteredBy UserRef

	// BorrowedAt はUTC。表示側でJSTに直す。
	BorrowedAt time.Time

	// DueDate は 'YYYY-MM-DD'（JST）。
	DueDate string

	// ReturnedAt が nil なら貸出中。
	ReturnedAt *time.Time
	ReturnedBy *UserRef

	// CancelledAt は誤登録の取り消し。返却とは別の事実として持つ。
	CancelledAt *time.Time

	Note string
}

// IsProxy は代理登録か。
//
// 呼び出し側に2つのIDを比べさせない。比べ忘れた画面ができる。
func (l *Loan) IsProxy() bool { return l.User.ID != l.RegisteredBy.ID }

// OverdueDays は返却予定日を何日過ぎたか。超過していなければ 0。
//
// 判定はJSTの日付で行う。UTCのまま比べると、JST 00:00〜09:00 の間だけ
// 前日として扱われる。
func (l *Loan) OverdueDays(at time.Time) int {
	if l.ReturnedAt != nil || l.CancelledAt != nil {
		return 0
	}

	due, err := jst.ParseDate(l.DueDate)
	if err != nil {
		// DBに入っている値は書き込み時に正規化済み。読めないなら
		// 手で書き換えられている。超過扱いにして騒がない。
		return 0
	}

	if days := jst.DaysBetween(due, at); days > 0 {
		return days
	}
	return 0
}

// now は現在時刻。テストで固定するため定数ではなく var にする。
//
// 借用日時の既定値と「未来かどうか」の判定に使う。実時刻に依存させると、
// JSTの日付境界を跨いだ時だけ落ちるテストになる。
var now = time.Now

// Store は loans テーブルへの読み書き。
type Store struct {
	sqldb *sql.DB
}

// NewStore は Store を作る。
func NewStore(sqldb *sql.DB) *Store {
	return &Store{sqldb: sqldb}
}

// Request は借用の入力。**すべて省略できる。**
//
// 空の入力で自分の借用が成立することが、3タップで終わらせる前提になる
// （docs/m2-implementation-spec.md §9.1）。
type Request struct {
	// UserID は借用者。0 なら登録者本人。
	// 自分以外を指定すると代理登録になる。
	UserID int64

	// DueDate は 'YYYY-MM-DD'（JST）。空なら借用日+14日。
	DueDate string

	// BorrowedAt はゼロ値なら現在時刻。過去を指定すると事後登録になる。
	BorrowedAt time.Time

	Note string
}

// Borrow は借用を1件記録する。registeredBy は操作した人。
//
// 検査から INSERT までを1つのトランザクションで行う。DSN で
// _txlock=immediate を指定しているため、ここは BEGIN IMMEDIATE で始まる。
func (s *Store) Borrow(ctx context.Context, code string, registeredBy int64, req Request) (*Loan, error) {
	current := now()

	borrowedAt := req.BorrowedAt
	if borrowedAt.IsZero() {
		borrowedAt = current
	}
	// 端末の時計のずれで弾かないよう1分だけ猶予を持たせる。
	if borrowedAt.After(current.Add(time.Minute)) {
		return nil, ErrFutureBorrowedAt
	}

	dueDate, err := resolveDueDate(req.DueDate, borrowedAt)
	if err != nil {
		return nil, err
	}

	tx, err := s.sqldb.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("借用の登録: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	itemID, err := borrowableItem(ctx, tx, code)
	if err != nil {
		return nil, err
	}

	userID, err := borrower(ctx, tx, registeredBy, req.UserID)
	if err != nil {
		return nil, err
	}

	const q = `
INSERT INTO loans (item_id, user_id, registered_by, borrowed_at, due_date, note)
VALUES (?, ?, ?, ?, ?, ?)`

	res, err := tx.ExecContext(ctx, q,
		itemID, userID, registeredBy,
		borrowedAt.UTC().Format(sqliteTimeLayout), dueDate, strings.TrimSpace(req.Note),
	)
	if err != nil {
		// 二重貸出は部分ユニークインデックスが拒否する。
		// アプリ側で「貸出中か」を SELECT してから INSERT する形にすると競合に負ける。
		if db.IsUniqueViolation(err) {
			return nil, ErrAlreadyBorrowed
		}
		return nil, fmt.Errorf("借用の登録: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("借用の登録: %w", err)
	}

	// 借りられたということは棚に戻っていたということ。
	// 見つかった事実を借りた人に報告させない（追加操作を求めない）。
	if err := restoreFromMissing(ctx, tx, itemID); err != nil {
		return nil, err
	}

	l, err := queryLoan(ctx, tx, id)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("借用の登録: %w", err)
	}

	return l, nil
}

// Return は返却を記録する。returnedBy は操作した人。
//
// **借用者本人でなくてもよい。** 棚に戻っているのを見つけた人が処理できないと、
// 記録が永久にズレたままになる。誰が戻したかは returned_by に残る
// （docs/m2-implementation-spec.md §6）。
//
// 返却時に状態を訊かない。破損があれば破損報告を押してもらう。
// 借用の記録漏れは致命的だが、返却の記録漏れは本人に聞けば自己修復する。
// UIもAPIも返却側は薄くてよい。
func (s *Store) Return(ctx context.Context, code string, returnedBy int64) (*Loan, error) {
	tx, err := s.sqldb.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("返却の登録: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// 備品が無いのか、貸出中でないのかを区別する。
	// どちらも 404 にすると、コードの打ち間違いと二重タップが同じ表示になる。
	var itemID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM items WHERE code = ?`, strings.TrimSpace(code)).Scan(&itemID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrItemNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("備品の取得: %w", err)
	}

	// 廃棄済みでも返せる。持ち出したまま廃棄扱いになった備品を戻せないと、
	// 貸出中の行が永久に残る。
	const find = `
SELECT id FROM loans
WHERE item_id = ? AND returned_at IS NULL AND cancelled_at IS NULL`

	var loanID int64
	err = tx.QueryRowContext(ctx, find, itemID).Scan(&loanID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotBorrowed
	}
	if err != nil {
		return nil, fmt.Errorf("貸出の取得: %w", err)
	}

	const q = `
UPDATE loans
SET returned_at = datetime('now'), returned_by = ?, updated_at = datetime('now')
WHERE id = ?`

	if _, err := tx.ExecContext(ctx, q, returnedBy, loanID); err != nil {
		return nil, fmt.Errorf("返却の登録: %w", err)
	}

	l, err := queryLoan(ctx, tx, loanID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("返却の登録: %w", err)
	}

	return l, nil
}

// Cancel は誤登録の貸出を取り消す。by は操作した人。
//
// **行は削除しない。** cancelled_at を立てる。「返却済み」と「そもそも借りていない」は
// 別の事実で、取り消しを返却として記録すると、借りていない人の返却履歴が残り、
// 破損の追跡時に誤った経路をたどることになる（docs/m2-implementation-spec.md §5）。
//
// 取り消せるのは未返却の貸出だけ。返却済みの履歴は消せない。
func (s *Store) Cancel(ctx context.Context, loanID int64, by Actor, reason string) (*Loan, error) {
	tx, err := s.sqldb.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("貸出の取り消し: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	const find = `
SELECT user_id, registered_by, returned_at, cancelled_at
FROM loans WHERE id = ?`

	var (
		userID       int64
		registeredBy int64
		returnedAt   sql.NullString
		cancelledAt  sql.NullString
	)
	err = tx.QueryRowContext(ctx, find, loanID).Scan(&userID, &registeredBy, &returnedAt, &cancelledAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("貸出の取得: %w", err)
	}

	// 権限を先に見る。返却済みかどうかを、権限の無い人に教えない。
	if !by.IsAdmin && by.ID != userID && by.ID != registeredBy {
		return nil, ErrForbidden
	}
	if cancelledAt.Valid {
		return nil, ErrAlreadyCancelled
	}
	if returnedAt.Valid {
		return nil, ErrAlreadyReturned
	}

	const q = `
UPDATE loans
SET cancelled_at = datetime('now'), cancelled_by = ?, cancel_reason = ?,
    updated_at = datetime('now')
WHERE id = ?`

	if _, err := tx.ExecContext(ctx, q, by.ID, strings.TrimSpace(reason), loanID); err != nil {
		return nil, fmt.Errorf("貸出の取り消し: %w", err)
	}

	l, err := queryLoan(ctx, tx, loanID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("貸出の取り消し: %w", err)
	}

	return l, nil
}

// resolveDueDate は返却予定日を決める。空なら借用日+14日。
//
// 借用日を基準にする。現在時刻を基準にすると、事後登録で過去の借用を
// 入れた人だけ返却期限が延びる。
func resolveDueDate(due string, borrowedAt time.Time) (string, error) {
	due = strings.TrimSpace(due)
	if due == "" {
		return jst.FormatDate(jst.Date(borrowedAt).AddDate(0, 0, defaultLoanDays)), nil
	}

	d, err := jst.ParseDate(due)
	if err != nil {
		return "", ErrInvalidDueDate
	}
	if d.Before(jst.Date(borrowedAt)) {
		return "", ErrInvalidDueDate
	}

	// 読めた値を書き戻す。'2026-9-1' のような形を DB に入れると、
	// 文字列で比較する経路（期限超過の抽出）で静かに外れる。
	return jst.FormatDate(d), nil
}

// borrowableItem は備品が借りられる状態かを見て、item_id を返す。
func borrowableItem(ctx context.Context, tx *sql.Tx, code string) (int64, error) {
	const q = `SELECT id, is_free_use, condition FROM items WHERE code = ?`

	var (
		id        int64
		isFreeUse int
		condition string
	)
	err := tx.QueryRowContext(ctx, q, strings.TrimSpace(code)).Scan(&id, &isFreeUse, &condition)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrItemNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("備品の取得: %w", err)
	}

	// UIで借用ボタンを隠すだけにしない。APIを直接叩けば素通りできる（CLAUDE.md）。
	if isFreeUse != 0 {
		return 0, ErrFreeUse
	}
	if condition == conditionDiscarded {
		return 0, ErrDiscarded
	}

	// location_status は見ない。所在不明でも借用は成立し、
	// 成立した時点で在庫へ戻す（restoreFromMissing）。
	return id, nil
}

// restoreFromMissing は所在不明の備品を在庫に戻す。
//
// 借用と同じトランザクションで行う。分けると、借用だけ通って復帰が漏れた行が
// 残り、貸出中なのに所在不明という記録になる。
//
// 在庫のものは触らない（updated_at を動かさない）。借りるたびに更新日時が
// 変わると、一覧の「最終更新」が貸出の履歴と区別できなくなる。
//
// 該当する missing_reports を発見済みにするのは M3。M2 にそのテーブルは無い
// （docs/m2-implementation-spec.md §3）。
func restoreFromMissing(ctx context.Context, tx *sql.Tx, itemID int64) error {
	const q = `
UPDATE items
SET location_status = ?, updated_at = datetime('now')
WHERE id = ? AND location_status <> ?`

	if _, err := tx.ExecContext(ctx, q, locationInStock, itemID, locationInStock); err != nil {
		return fmt.Errorf("所在の復帰: %w", err)
	}
	return nil
}

// borrower は借用者のIDを決める。requested が 0 なら登録者本人。
func borrower(ctx context.Context, tx *sql.Tx, registeredBy, requested int64) (int64, error) {
	if requested == 0 || requested == registeredBy {
		// 操作している本人。セッションが通っている以上、有効な利用者。
		return registeredBy, nil
	}

	const q = `SELECT 1 FROM users WHERE id = ? AND is_active = 1`

	var ok int
	err := tx.QueryRowContext(ctx, q, requested).Scan(&ok)
	if errors.Is(err, sql.ErrNoRows) {
		// 卒業者（is_active = 0）もここで弾く。削除はしないため行は残っている。
		return 0, ErrInvalidUser
	}
	if err != nil {
		return 0, fmt.Errorf("借用者の確認: %w", err)
	}

	return requested, nil
}

// selectColumns は Loan を組み立てる列の並び。scanLoan と対で保つ。
const selectColumns = `
SELECT l.id, l.item_id, i.code, i.name,
       l.user_id, bu.name, bu.email,
       l.registered_by, ru.name,
       l.borrowed_at, l.due_date,
       l.returned_at, l.returned_by, rbu.name,
       l.cancelled_at, l.note
FROM loans l
JOIN items i        ON i.id  = l.item_id
JOIN users bu       ON bu.id = l.user_id
JOIN users ru       ON ru.id = l.registered_by
LEFT JOIN users rbu ON rbu.id = l.returned_by`

// queryLoan は1件引く。トランザクションの中からも外からも使えるようにする。
func queryLoan(ctx context.Context, q querier, id int64) (*Loan, error) {
	l, err := scanLoan(q.QueryRowContext(ctx, selectColumns+"\nWHERE l.id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("貸出の取得: %w", err)
	}
	return l, nil
}

// ErrNotFound は貸出が見つからないこと。
var ErrNotFound = errors.New("貸出が見つからない")

// querier は *sql.DB と *sql.Tx のどちらでも受けられるようにする。
type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// row は *sql.Row と *sql.Rows のどちらでも受けられるようにする。
type row interface {
	Scan(dest ...any) error
}

func scanLoan(r row) (*Loan, error) {
	var (
		l            Loan
		borrowerMail sql.NullString
		borrowedAt   string
		returnedAt   sql.NullString
		returnedBy   sql.NullInt64
		returnedName sql.NullString
		cancelledAt  sql.NullString
	)

	err := r.Scan(
		&l.ID, &l.ItemID, &l.ItemCode, &l.ItemName,
		&l.User.ID, &l.User.Name, &borrowerMail,
		&l.RegisteredBy.ID, &l.RegisteredBy.Name,
		&borrowedAt, &l.DueDate,
		&returnedAt, &returnedBy, &returnedName,
		&cancelledAt, &l.Note,
	)
	if err != nil {
		return nil, err
	}

	l.User.Email = borrowerMail.String

	l.BorrowedAt, err = parseStoredTime(borrowedAt)
	if err != nil {
		return nil, err
	}
	if l.ReturnedAt, err = parseStoredTimePtr(returnedAt); err != nil {
		return nil, err
	}
	if l.CancelledAt, err = parseStoredTimePtr(cancelledAt); err != nil {
		return nil, err
	}
	if returnedBy.Valid {
		l.ReturnedBy = &UserRef{ID: returnedBy.Int64, Name: returnedName.String}
	}

	return &l, nil
}

// parseStoredTime はDBに入っている時刻を読む。保存はUTC。
func parseStoredTime(s string) (time.Time, error) {
	t, err := time.ParseInLocation(sqliteTimeLayout, s, time.UTC)
	if err != nil {
		return time.Time{}, fmt.Errorf("時刻 %q の読み取り: %w", s, err)
	}
	return t, nil
}

func parseStoredTimePtr(s sql.NullString) (*time.Time, error) {
	if !s.Valid {
		return nil, nil
	}
	t, err := parseStoredTime(s.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
