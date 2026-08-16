package item

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Rengemaru/equipment-management/internal/db"
)

// newTestStore はスキーマ適用済みの Store を返す。
func newTestStore(t *testing.T) *Store {
	t.Helper()

	ctx := context.Background()
	sqldb, err := db.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = sqldb.Close() })

	if err := db.Migrate(ctx, sqldb, db.Migrations()); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}

	return NewStore(sqldb)
}

// insert はテスト用に1件入れる。書き込みAPIはまだ無いので直接入れる。
func insert(t *testing.T, s *Store, code, name string, columns map[string]any) {
	t.Helper()

	q := `INSERT INTO items (code, name`
	values := `) VALUES (?, ?`
	args := []any{code, name}

	for col, v := range columns {
		q += ", " + col
		values += ", ?"
		args = append(args, v)
	}

	if _, err := s.sqldb.Exec(q+values+")", args...); err != nil {
		t.Fatalf("insert(%s): %v", code, err)
	}
}

func TestList_備品コード順に返す(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	// わざと順不同で入れる。
	insert(t, s, "0003", "工具箱", nil)
	insert(t, s, "0001", "三脚", nil)
	insert(t, s, "0002", "カメラ", nil)

	items, err := s.List(ctx, Filter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("件数 = %d。3件を期待", len(items))
	}

	// ラベルを並べた棚と見比べる時にこの順が要る。
	for i, want := range []string{"0001", "0002", "0003"} {
		if items[i].Code != want {
			t.Errorf("%d 件目 = %s。%s を期待", i, items[i].Code, want)
		}
	}
}

func TestList_既定値が読める(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	insert(t, s, "0001", "三脚", nil)

	items, err := s.List(ctx, Filter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	it := items[0]
	if it.Category != "未分類" {
		t.Errorf("Category = %q", it.Category)
	}
	if it.Owner != OwnerCircle {
		t.Errorf("Owner = %q", it.Owner)
	}
	if it.Condition != ConditionGood {
		t.Errorf("Condition = %q", it.Condition)
	}
	if it.LocationStatus != LocationInStock {
		t.Errorf("LocationStatus = %q", it.LocationStatus)
	}
	if it.IsFreeUse {
		t.Error("IsFreeUse の既定が true になっている")
	}
	// NULL の列は空文字で受ける。フロントに null を渡さない。
	if it.Model != "" || it.Location != "" || it.Note != "" || it.PhotoPath != "" {
		t.Errorf("NULL の列が空文字になっていない: %+v", it)
	}
}

func TestList_品名とコードと型番で検索できる(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	insert(t, s, "0001", "三脚", map[string]any{"model": "Manfrotto 190"})
	insert(t, s, "0002", "カメラ", map[string]any{"model": "EOS R6"})
	insert(t, s, "0010", "三脚（小）", nil)

	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{"品名の部分一致", "三脚", []string{"0001", "0010"}},
		{"備品コード", "0002", []string{"0002"}},
		{"型番", "EOS", []string{"0002"}},
		{"一致なし", "存在しない", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items, err := s.List(ctx, Filter{Query: tt.query})
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(items) != len(tt.want) {
				t.Fatalf("件数 = %d。%d件を期待", len(items), len(tt.want))
			}
			for i, code := range tt.want {
				if items[i].Code != code {
					t.Errorf("%d 件目 = %s。%s を期待", i, items[i].Code, code)
				}
			}
		})
	}
}

// LIKE の特殊文字を打ち消さないと、品名に % を含む検索が全件一致になる。
func TestList_LIKEの特殊文字を打ち消す(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	insert(t, s, "0001", "三脚", nil)
	insert(t, s, "0002", "アルコール50%", nil)
	insert(t, s, "0003", "型番_A", nil)

	tests := []struct {
		query string
		want  string
	}{
		{"%", "0002"},
		{"_", "0003"},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			items, err := s.List(ctx, Filter{Query: tt.query})
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(items) != 1 {
				t.Fatalf("件数 = %d。1件を期待（特殊文字が効いている）", len(items))
			}
			if items[0].Code != tt.want {
				t.Errorf("Code = %s。%s を期待", items[0].Code, tt.want)
			}
		})
	}
}

func TestList_分類と保管場所で絞れる(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	insert(t, s, "0001", "三脚", map[string]any{"category": "撮影機材", "location": "棚A"})
	insert(t, s, "0002", "ドライバー", map[string]any{"category": "工具", "location": "棚B"})
	insert(t, s, "0003", "カメラ", map[string]any{"category": "撮影機材", "location": "棚B"})

	items, err := s.List(ctx, Filter{Category: "撮影機材"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("分類での絞り込み: %d件。2件を期待", len(items))
	}

	items, err = s.List(ctx, Filter{Location: "棚B"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("保管場所での絞り込み: %d件。2件を期待", len(items))
	}

	items, err = s.List(ctx, Filter{Category: "撮影機材", Location: "棚B"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 || items[0].Code != "0003" {
		t.Errorf("組み合わせでの絞り込みが効いていない: %+v", items)
	}
}

// 廃棄は既定で出さない。物理削除の代わりであり、
// 普段の一覧に出し続けると探す邪魔になる。
func TestList_廃棄は既定で除外する(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	insert(t, s, "0001", "三脚", nil)
	insert(t, s, "0002", "壊れたカメラ", map[string]any{"condition": string(ConditionDiscarded)})

	items, err := s.List(ctx, Filter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 || items[0].Code != "0001" {
		t.Fatalf("廃棄が既定で出ている: %+v", items)
	}

	// 状態で明示すれば見られること。記録は消えていない。
	items, err = s.List(ctx, Filter{Condition: ConditionDiscarded})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 || items[0].Code != "0002" {
		t.Errorf("廃棄で絞り込めない: %+v", items)
	}

	items, err = s.List(ctx, Filter{IncludeDiscarded: true})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("include_discarded が効いていない: %d件", len(items))
	}
}

func TestList_所在で絞れる(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	insert(t, s, "0001", "三脚", nil)
	insert(t, s, "0002", "カメラ", map[string]any{"location_status": string(LocationMissing)})

	items, err := s.List(ctx, Filter{LocationStatus: LocationMissing})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 || items[0].Code != "0002" {
		t.Errorf("所在での絞り込みが効いていない: %+v", items)
	}
}

func TestByCode_引けて居なければErrNotFound(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	insert(t, s, "0042", "三脚", nil)

	it, err := s.ByCode(ctx, "0042")
	if err != nil {
		t.Fatalf("ByCode: %v", err)
	}
	if it.Name != "三脚" {
		t.Errorf("Name = %q", it.Name)
	}

	if _, err := s.ByCode(ctx, "9999"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v。ErrNotFound を期待", err)
	}
}

func TestCategoriesとLocations_使われている値を返す(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	insert(t, s, "0001", "三脚", map[string]any{"category": "撮影機材", "location": "棚A"})
	insert(t, s, "0002", "カメラ", map[string]any{"category": "撮影機材", "location": "棚B"})
	insert(t, s, "0003", "ドライバー", map[string]any{"category": "工具"})

	categories, err := s.Categories(ctx)
	if err != nil {
		t.Fatalf("Categories: %v", err)
	}
	// 重複しないこと。
	if len(categories) != 2 {
		t.Errorf("分類 = %v。2件を期待", categories)
	}

	locations, err := s.Locations(ctx)
	if err != nil {
		t.Fatalf("Locations: %v", err)
	}
	// NULL の保管場所は出ないこと。
	if len(locations) != 2 {
		t.Errorf("保管場所 = %v。2件を期待", locations)
	}
}
