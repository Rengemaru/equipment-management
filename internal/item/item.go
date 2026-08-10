// Package item は備品マスタの読み書きを扱う。
//
// 貸出状態はここに持たない。items に「貸出中フラグ」を置くと、loans との
// 二重管理になり必ず不整合を起こす（CLAUDE.md）。貸出は M2 で loans から導出する。
package item

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Condition は状態。DBの CHECK 制約と対で保つ。
type Condition string

const (
	ConditionGood     Condition = "良好"
	ConditionNeedsFix Condition = "要修理"

	// ConditionDiscarded は廃棄。物理削除の代わりに使う。
	// items を消すと貸出履歴の参照先が消える（CLAUDE.md）。
	ConditionDiscarded Condition = "廃棄"
)

// Valid はDBの CHECK 制約と同じ判定をする。
func (c Condition) Valid() bool {
	return c == ConditionGood || c == ConditionNeedsFix || c == ConditionDiscarded
}

// LocationStatus は所在。貸出状態とは独立で、
// 「貸出中かつ所在不明」も起こり得る。
type LocationStatus string

const (
	LocationInStock      LocationStatus = "在庫"
	LocationMissing      LocationStatus = "所在不明_未確認"
	LocationMissingFixed LocationStatus = "所在不明_確定"
)

// Valid はDBの CHECK 制約と同じ判定をする。
func (s LocationStatus) Valid() bool {
	return s == LocationInStock || s == LocationMissing || s == LocationMissingFixed
}

// Owner は所有区分。
type Owner string

const (
	OwnerCircle     Owner = "サークル"
	OwnerDepartment Owner = "学科"
)

// Valid はDBの CHECK 制約と同じ判定をする。
func (o Owner) Valid() bool {
	return o == OwnerCircle || o == OwnerDepartment
}

// ErrNotFound は備品が見つからないこと。
var ErrNotFound = errors.New("備品が見つからない")

// Item は1件の備品。
type Item struct {
	ID   int64
	Code string
	Name string

	Category string
	Model    string
	Owner    Owner

	// IsFreeUse は記録不要の自由利用品。貸出フローの対象外にする。
	// 追跡対象を減らすことが遵守率を上げる最短経路（CLAUDE.md）。
	IsFreeUse bool

	Location       string
	Condition      Condition
	LocationStatus LocationStatus
	PhotoPath      string
	Note           string

	CreatedAt string
	UpdatedAt string
}

// Store は items テーブルへの読み書き。
type Store struct {
	sqldb *sql.DB
}

// NewStore は Store を作る。
func NewStore(sqldb *sql.DB) *Store {
	return &Store{sqldb: sqldb}
}

// Filter は一覧の絞り込み条件。空の項目は条件にしない。
type Filter struct {
	// Query は品名・備品コード・型番の部分一致。
	Query string

	Category       string
	Location       string
	Condition      Condition
	LocationStatus LocationStatus

	// IncludeDiscarded は廃棄済みを含めるか。
	//
	// 既定では除く。廃棄は物理削除の代わりであり、普段の一覧に出し続けると
	// 「棚にあるはずのもの」を探す邪魔になる。状態で明示的に絞れば見られる。
	IncludeDiscarded bool
}

// selectColumns は Item を組み立てる列の並び。scanItem と対で保つ。
const selectColumns = `
SELECT id, code, name, category, model, owner, is_free_use,
       location, condition, location_status, photo_path, note,
       created_at, updated_at
FROM items`

// List は条件に合う備品を返す。
//
// 件数を絞らない。数百件までは一度に返した方が、フロント側で
// 絞り込みも並べ替えも自由にできる。数千件に育ったら見直すこと。
func (s *Store) List(ctx context.Context, f Filter) ([]*Item, error) {
	var (
		where []string
		args  []any
	)

	if q := strings.TrimSpace(f.Query); q != "" {
		// 部分一致。LIKE の特殊文字は打ち消す。打ち消さないと、
		// 品名に % を含む検索が全件一致になる。
		pattern := "%" + escapeLike(q) + "%"
		where = append(where, `(name LIKE ? ESCAPE '\' OR code LIKE ? ESCAPE '\' OR IFNULL(model, '') LIKE ? ESCAPE '\')`)
		args = append(args, pattern, pattern, pattern)
	}
	if v := strings.TrimSpace(f.Category); v != "" {
		where = append(where, `category = ?`)
		args = append(args, v)
	}
	if v := strings.TrimSpace(f.Location); v != "" {
		where = append(where, `location = ?`)
		args = append(args, v)
	}
	if f.Condition != "" {
		where = append(where, `condition = ?`)
		args = append(args, string(f.Condition))
	}
	if f.LocationStatus != "" {
		where = append(where, `location_status = ?`)
		args = append(args, string(f.LocationStatus))
	}
	if !f.IncludeDiscarded && f.Condition != ConditionDiscarded {
		where = append(where, `condition <> ?`)
		args = append(args, string(ConditionDiscarded))
	}

	query := selectColumns
	if len(where) > 0 {
		query += "\nWHERE " + strings.Join(where, "\n  AND ")
	}
	// 備品コード順。ラベルを並べた棚と見比べる時にこの順が要る。
	query += "\nORDER BY code"

	rows, err := s.sqldb.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("備品一覧の取得: %w", err)
	}
	defer rows.Close()

	var items []*Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, fmt.Errorf("備品一覧の読み取り: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("備品一覧の読み取り: %w", err)
	}

	return items, nil
}

// ByCode は備品コードで引く。QRから来た画面が使う。
func (s *Store) ByCode(ctx context.Context, code string) (*Item, error) {
	return s.queryOne(ctx, selectColumns+` WHERE code = ?`, strings.TrimSpace(code))
}

// ByID はIDで引く。
func (s *Store) ByID(ctx context.Context, id int64) (*Item, error) {
	return s.queryOne(ctx, selectColumns+` WHERE id = ?`, id)
}

// Categories は登録されている分類を返す。絞り込みの選択肢に使う。
//
// 分類は自由入力のため、固定の一覧を持てない。実際に使われている値を出す。
func (s *Store) Categories(ctx context.Context) ([]string, error) {
	const q = `SELECT DISTINCT category FROM items WHERE category <> '' ORDER BY category`
	return s.stringColumn(ctx, q)
}

// Locations は登録されている保管場所を返す。
func (s *Store) Locations(ctx context.Context) ([]string, error) {
	const q = `
SELECT DISTINCT location FROM items
WHERE location IS NOT NULL AND location <> ''
ORDER BY location`
	return s.stringColumn(ctx, q)
}

func (s *Store) stringColumn(ctx context.Context, query string) ([]string, error) {
	rows, err := s.sqldb.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("一覧の取得: %w", err)
	}
	defer rows.Close()

	var values []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("一覧の読み取り: %w", err)
		}
		values = append(values, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("一覧の読み取り: %w", err)
	}

	return values, nil
}

func (s *Store) queryOne(ctx context.Context, query string, args ...any) (*Item, error) {
	it, err := scanItem(s.sqldb.QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("備品の取得: %w", err)
	}
	return it, nil
}

// row は *sql.Row と *sql.Rows のどちらでも受けられるようにする。
type row interface {
	Scan(dest ...any) error
}

func scanItem(r row) (*Item, error) {
	var (
		it        Item
		model     sql.NullString
		location  sql.NullString
		photoPath sql.NullString
		note      sql.NullString
		isFreeUse int
		owner     string
		condition string
		locStatus string
	)

	err := r.Scan(
		&it.ID, &it.Code, &it.Name, &it.Category, &model, &owner, &isFreeUse,
		&location, &condition, &locStatus, &photoPath, &note,
		&it.CreatedAt, &it.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	it.Model = model.String
	it.Location = location.String
	it.PhotoPath = photoPath.String
	it.Note = note.String
	it.IsFreeUse = isFreeUse != 0
	it.Owner = Owner(owner)
	it.Condition = Condition(condition)
	it.LocationStatus = LocationStatus(locStatus)

	return &it, nil
}

// escapeLike は LIKE の特殊文字を打ち消す。
//
// 打ち消さないと、品名に % を含む検索が全件一致になる。
// バックスラッシュ自体も対象にする。先に処理しないと、
// 後から足したエスケープ文字を二重に打ち消すことになる。
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "%", `\%`)
	s = strings.ReplaceAll(s, "_", `\_`)
	return s
}
