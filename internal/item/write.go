package item

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/Rengemaru/equipment-management/internal/db"
)

// ErrDuplicateCode は同じ備品コードが既にあること。
//
// 自動採番では起きないが、CSVインポートでコードを指定した時に起こり得る。
var ErrDuplicateCode = errors.New("その備品コードは既に使われている")

// validationError は入力の誤り。
//
// 型で区別できるようにしているのは、ハンドラが 400 と 500 を取り違えないため。
// メッセージは利用者にそのまま見せる前提で書くこと。
type validationError struct {
	msg string
}

func (e *validationError) Error() string { return e.msg }

func invalidf(format string, args ...any) error {
	return &validationError{msg: fmt.Sprintf(format, args...)}
}

// Attributes は登録・更新で受け取る内容。
//
// 備品コードは含めない。コードは採番したら二度と変えない。
// ラベルは貼り替えられないため、変えるとラベルと実物の対応が壊れる。
type Attributes struct {
	Name           string
	Category       string
	Model          string
	Owner          Owner
	IsFreeUse      bool
	Location       string
	Condition      Condition
	LocationStatus LocationStatus
	Note           string
}

// normalize は空欄に既定値を入れ、値を検査する。
//
// 既定値はDBの DEFAULT と同じにする。CSVインポートで空欄だった列が
// この値になるため、経路によって結果が変わらないようにする。
func (a *Attributes) normalize() error {
	a.Name = strings.TrimSpace(a.Name)
	a.Category = strings.TrimSpace(a.Category)
	a.Model = strings.TrimSpace(a.Model)
	a.Location = strings.TrimSpace(a.Location)
	a.Note = strings.TrimSpace(a.Note)

	if a.Name == "" {
		return invalidf("品名は必須です")
	}
	if a.Category == "" {
		a.Category = "未分類"
	}
	if a.Owner == "" {
		a.Owner = OwnerCircle
	}
	if a.Condition == "" {
		a.Condition = ConditionGood
	}
	if a.LocationStatus == "" {
		a.LocationStatus = LocationInStock
	}

	if !a.Owner.Valid() {
		return invalidf("所有の指定が不正です: %q", a.Owner)
	}
	if !a.Condition.Valid() {
		return invalidf("状態の指定が不正です: %q", a.Condition)
	}
	if !a.LocationStatus.Valid() {
		return invalidf("所在の指定が不正です: %q", a.LocationStatus)
	}

	return nil
}

// Create は備品を1件登録し、採番された備品コードを持つ Item を返す。
//
// 採番と INSERT を1つのトランザクションで行う。分けると、同時に登録した
// 2件が同じ番号を取る。
func (s *Store) Create(ctx context.Context, attrs Attributes) (*Item, error) {
	if err := attrs.normalize(); err != nil {
		return nil, err
	}

	tx, err := s.sqldb.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("備品の登録: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	code, err := NextCode(ctx, tx)
	if err != nil {
		return nil, err
	}

	if err := insertItem(ctx, tx, code, attrs); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("備品の登録: %w", err)
	}

	return s.ByCode(ctx, code)
}

// insertItem は1件挿入する。CSVインポートからも使う。
func insertItem(ctx context.Context, tx *sql.Tx, code string, attrs Attributes) error {
	const q = `
INSERT INTO items (code, name, category, model, owner, is_free_use,
                   location, condition, location_status, note)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := tx.ExecContext(ctx, q,
		code, attrs.Name, attrs.Category, nullable(attrs.Model), string(attrs.Owner),
		boolToInt(attrs.IsFreeUse), nullable(attrs.Location), string(attrs.Condition),
		string(attrs.LocationStatus), nullable(attrs.Note),
	)
	if err != nil {
		if db.IsUniqueViolation(err) {
			return ErrDuplicateCode
		}
		return fmt.Errorf("備品の登録: %w", err)
	}

	return nil
}

// Update は備品の内容を差し替える。備品コードは変えられない。
//
// 一部だけ送る形にしない。送られなかった項目を「変更なし」と解釈すると、
// 画面で消した備考が消えないなど、意図と結果がずれる。
func (s *Store) Update(ctx context.Context, code string, attrs Attributes) (*Item, error) {
	if err := attrs.normalize(); err != nil {
		return nil, err
	}

	const q = `
UPDATE items
SET name = ?, category = ?, model = ?, owner = ?, is_free_use = ?,
    location = ?, condition = ?, location_status = ?, note = ?,
    updated_at = datetime('now')
WHERE code = ?`

	res, err := s.sqldb.ExecContext(ctx, q,
		attrs.Name, attrs.Category, nullable(attrs.Model), string(attrs.Owner),
		boolToInt(attrs.IsFreeUse), nullable(attrs.Location), string(attrs.Condition),
		string(attrs.LocationStatus), nullable(attrs.Note), strings.TrimSpace(code),
	)
	if err != nil {
		return nil, fmt.Errorf("備品の更新: %w", err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("備品の更新: %w", err)
	}
	if n == 0 {
		return nil, ErrNotFound
	}

	return s.ByCode(ctx, code)
}

// Discard は廃棄にする。
//
// 行は消さない。items を物理削除すると、その備品を含む貸出履歴の
// 参照先が消える（CLAUDE.md）。廃棄は状態であって削除ではない。
func (s *Store) Discard(ctx context.Context, code string) (*Item, error) {
	const q = `
UPDATE items
SET condition = ?, updated_at = datetime('now')
WHERE code = ?`

	res, err := s.sqldb.ExecContext(ctx, q, string(ConditionDiscarded), strings.TrimSpace(code))
	if err != nil {
		return nil, fmt.Errorf("備品の廃棄: %w", err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("備品の廃棄: %w", err)
	}
	if n == 0 {
		return nil, ErrNotFound
	}

	return s.ByCode(ctx, code)
}

// nullable は空文字を NULL に変える。
//
// 空文字で入れると、SQL側で IFNULL と ” の両方を気にすることになる。
// 「未入力」の表し方を NULL に統一する。
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// SetPhoto は写真のファイル名を差し替える。空文字で外す。
//
// ファイルの削除は呼び出し側で行う。DBの更新とファイル操作を1つの関数に
// 入れると、どちらかだけ成功した時の後始末が中で閉じてしまい、
// 呼び出し側から何が起きたか分からなくなる。
func (s *Store) SetPhoto(ctx context.Context, code, filename string) (*Item, error) {
	const q = `
UPDATE items
SET photo_path = ?, updated_at = datetime('now')
WHERE code = ?`

	res, err := s.sqldb.ExecContext(ctx, q, nullable(filename), strings.TrimSpace(code))
	if err != nil {
		return nil, fmt.Errorf("写真の更新: %w", err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("写真の更新: %w", err)
	}
	if n == 0 {
		return nil, ErrNotFound
	}

	return s.ByCode(ctx, code)
}
