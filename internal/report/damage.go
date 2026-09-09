// Package report は破損報告を扱う。
//
// **承認フローを作らない。** 報告は即時反映し、運営は事後に追認する。
// 承認待ちの間システムが「良好」と表示し続ける状態は、紙の台帳より悪い。
// 記録が正しいという嘘を、システムが権威をもって主張することになる（CLAUDE.md）。
//
// 所在不明報告（M3）もこのパッケージに足す。
package report

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// sqliteTimeLayout は datetime('now') が返す形式。保存はUTC。
const sqliteTimeLayout = "2006-01-02 15:04:05"

// items.condition に入る値。
//
// item パッケージを参照しない。DBの CHECK 制約と対で保つこと。
const (
	conditionGood      = "良好"
	conditionNeedsFix  = "要修理"
	conditionDiscarded = "廃棄"
)

// Status は破損報告の状態。DBの CHECK 制約と対で保つ。
type Status string

const (
	// StatusUnconfirmed は報告されたまま。運営がまだ見ていない。
	StatusUnconfirmed Status = "未確認"

	// StatusConfirmed は運営が現物を確認した。状態は要修理のまま。
	StatusConfirmed Status = "確認済み"

	// StatusRepaired は直った。備品は良好に戻る。
	StatusRepaired Status = "修理済み"

	// StatusDiscarded は直せないので廃棄した。
	StatusDiscarded Status = "廃棄"
)

// Valid はDBの CHECK 制約と同じ判定をする。
func (s Status) Valid() bool {
	return s == StatusUnconfirmed || s == StatusConfirmed ||
		s == StatusRepaired || s == StatusDiscarded
}

// condition は追認の結果として備品の状態をどうするか。
//
// 空文字なら変えない。**「確認済み」で要修理に戻さない**のは、
// 既に要修理になっているため（報告の時点で変えている）。
func (s Status) condition() string {
	switch s {
	case StatusRepaired:
		return conditionGood
	case StatusDiscarded:
		return conditionDiscarded
	default:
		return ""
	}
}

var (
	// ErrItemNotFound は備品が無い。
	ErrItemNotFound = errors.New("備品が見つからない")

	// ErrNotFound は報告が無い。
	ErrNotFound = errors.New("破損報告が見つからない")

	// ErrEmptyDescription は破損箇所の説明が空。
	//
	// ここだけは必須にする。「壊れた」とだけ記録されても、
	// 運営が現物を見るまで何も判断できない。
	ErrEmptyDescription = errors.New("破損箇所の説明を入力してください")

	// ErrInvalidStatus は状態の指定が不正。
	ErrInvalidStatus = errors.New("状態の指定が不正")
)

// UserRef は報告に関わる人。
type UserRef struct {
	ID   int64
	Name string
}

// Damage は1件の破損報告。
type Damage struct {
	ID       int64
	ItemID   int64
	ItemCode string
	ItemName string

	// LoanID は貸出中に報告された場合の貸出。在庫中の報告では nil。
	LoanID *int64

	Reporter    UserRef
	ReportedAt  time.Time
	Description string

	Status      Status
	ConfirmedBy *UserRef
	ConfirmedAt *time.Time
	Note        string
}

// Store は damage_reports テーブルへの読み書き。
type Store struct {
	sqldb *sql.DB
}

// NewStore は Store を作る。
func NewStore(sqldb *sql.DB) *Store {
	return &Store{sqldb: sqldb}
}

// Report は破損を1件記録する。
//
// **報告と同時に備品を要修理にする。承認を待たない。** 報告した人から見て、
// 押した結果が画面に出ないボタンは「効かないボタン」でしかない。
func (s *Store) Report(ctx context.Context, code string, reporterID int64, description string) (*Damage, error) {
	description = strings.TrimSpace(description)
	if description == "" {
		return nil, ErrEmptyDescription
	}

	tx, err := s.sqldb.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("破損報告: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var (
		itemID    int64
		condition string
	)
	err = tx.QueryRowContext(ctx,
		`SELECT id, condition FROM items WHERE code = ?`, strings.TrimSpace(code),
	).Scan(&itemID, &condition)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrItemNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("備品の取得: %w", err)
	}

	// 貸出中なら紐付ける。誰が借りている間に壊れたのかを後から辿れるようにする。
	var loanID sql.NullInt64
	err = tx.QueryRowContext(ctx, `
SELECT id FROM loans
WHERE item_id = ? AND returned_at IS NULL AND cancelled_at IS NULL`, itemID).Scan(&loanID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("貸出の取得: %w", err)
	}

	res, err := tx.ExecContext(ctx, `
INSERT INTO damage_reports (item_id, loan_id, reporter_id, description)
VALUES (?, ?, ?, ?)`, itemID, loanID, reporterID, description)
	if err != nil {
		return nil, fmt.Errorf("破損報告: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("破損報告: %w", err)
	}

	// **廃棄済みは要修理に戻さない。** 廃棄は終端状態で、報告によって復活すると、
	// 廃棄したはずのものが一覧に現れる。
	if condition != conditionDiscarded {
		if err := setCondition(ctx, tx, itemID, conditionNeedsFix); err != nil {
			return nil, err
		}
	}

	d, err := queryDamage(ctx, tx, id)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("破損報告: %w", err)
	}

	return d, nil
}

// SetStatus は運営の追認を記録する。承認ではなく追認。
//
// 状態に応じて備品の状態も動かす（docs/m2-implementation-spec.md §7）。
func (s *Store) SetStatus(ctx context.Context, id int64, by int64, status Status, note string) (*Damage, error) {
	if !status.Valid() {
		return nil, ErrInvalidStatus
	}

	tx, err := s.sqldb.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("破損報告の更新: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var itemID int64
	err = tx.QueryRowContext(ctx, `SELECT item_id FROM damage_reports WHERE id = ?`, id).Scan(&itemID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("破損報告の取得: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
UPDATE damage_reports
SET status = ?, note = ?, confirmed_by = ?, confirmed_at = datetime('now'),
    updated_at = datetime('now')
WHERE id = ?`, string(status), strings.TrimSpace(note), by, id)
	if err != nil {
		return nil, fmt.Errorf("破損報告の更新: %w", err)
	}

	if c := status.condition(); c != "" {
		if err := setCondition(ctx, tx, itemID, c); err != nil {
			return nil, err
		}
	}

	d, err := queryDamage(ctx, tx, id)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("破損報告の更新: %w", err)
	}

	return d, nil
}

// List は報告を新しい順に返す。status が空なら全件。
func (s *Store) List(ctx context.Context, status Status) ([]*Damage, error) {
	if status != "" && !status.Valid() {
		return nil, ErrInvalidStatus
	}

	query := selectColumns
	var args []any
	if status != "" {
		query += "\nWHERE d.status = ?"
		args = append(args, string(status))
	}
	query += "\nORDER BY d.reported_at DESC, d.id DESC"

	rows, err := s.sqldb.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("破損報告の一覧: %w", err)
	}
	defer rows.Close()

	var list []*Damage
	for rows.Next() {
		d, err := scanDamage(rows)
		if err != nil {
			return nil, fmt.Errorf("破損報告の読み取り: %w", err)
		}
		list = append(list, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("破損報告の読み取り: %w", err)
	}

	return list, nil
}

// setCondition は備品の状態を変える。
//
// item パッケージを通さずに書くのは、報告と同じトランザクションに
// 入れるため。分けると、報告だけ残って状態が変わらない行ができる。
func setCondition(ctx context.Context, tx *sql.Tx, itemID int64, condition string) error {
	const q = `UPDATE items SET condition = ?, updated_at = datetime('now') WHERE id = ?`

	if _, err := tx.ExecContext(ctx, q, condition, itemID); err != nil {
		return fmt.Errorf("備品の状態の更新: %w", err)
	}
	return nil
}

// selectColumns は Damage を組み立てる列の並び。scanDamage と対で保つ。
const selectColumns = `
SELECT d.id, d.item_id, i.code, i.name, d.loan_id,
       d.reporter_id, ru.name, d.reported_at, d.description,
       d.status, d.confirmed_by, cu.name, d.confirmed_at, d.note
FROM damage_reports d
JOIN items i       ON i.id  = d.item_id
JOIN users ru      ON ru.id = d.reporter_id
LEFT JOIN users cu ON cu.id = d.confirmed_by`

func queryDamage(ctx context.Context, q querier, id int64) (*Damage, error) {
	d, err := scanDamage(q.QueryRowContext(ctx, selectColumns+"\nWHERE d.id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("破損報告の取得: %w", err)
	}
	return d, nil
}

// querier は *sql.DB と *sql.Tx のどちらでも受けられるようにする。
type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// row は *sql.Row と *sql.Rows のどちらでも受けられるようにする。
type row interface {
	Scan(dest ...any) error
}

func scanDamage(r row) (*Damage, error) {
	var (
		d             Damage
		loanID        sql.NullInt64
		reportedAt    string
		status        string
		confirmedBy   sql.NullInt64
		confirmedName sql.NullString
		confirmedAt   sql.NullString
		note          sql.NullString
	)

	err := r.Scan(
		&d.ID, &d.ItemID, &d.ItemCode, &d.ItemName, &loanID,
		&d.Reporter.ID, &d.Reporter.Name, &reportedAt, &d.Description,
		&status, &confirmedBy, &confirmedName, &confirmedAt, &note,
	)
	if err != nil {
		return nil, err
	}

	d.Status = Status(status)
	d.Note = note.String
	if loanID.Valid {
		id := loanID.Int64
		d.LoanID = &id
	}
	if confirmedBy.Valid {
		d.ConfirmedBy = &UserRef{ID: confirmedBy.Int64, Name: confirmedName.String}
	}

	if d.ReportedAt, err = parseStoredTime(reportedAt); err != nil {
		return nil, err
	}
	if confirmedAt.Valid {
		t, err := parseStoredTime(confirmedAt.String)
		if err != nil {
			return nil, err
		}
		d.ConfirmedAt = &t
	}

	return &d, nil
}

// parseStoredTime はDBに入っている時刻を読む。保存はUTC。
func parseStoredTime(s string) (time.Time, error) {
	t, err := time.ParseInLocation(sqliteTimeLayout, s, time.UTC)
	if err != nil {
		return time.Time{}, fmt.Errorf("時刻 %q の読み取り: %w", s, err)
	}
	return t, nil
}
