package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	_ "modernc.org/sqlite"
)

// openTestDB は使い捨ての SQLite ファイルを開く。
// :memory: は接続ごとに別のDBになるため、database/sql の接続プール越しだと
// テストが通ったり落ちたりする。ファイルにして再現性を取る。
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	sqldb, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = sqldb.Close() })

	return sqldb
}

// countRows は1つの整数を返すクエリを実行する。
func countRows(t *testing.T, sqldb *sql.DB, query string) int {
	t.Helper()

	var n int
	if err := sqldb.QueryRow(query).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

// tableExists はテーブルの有無を返す。
func tableExists(t *testing.T, sqldb *sql.DB, name string) bool {
	t.Helper()

	q := `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`
	var n int
	if err := sqldb.QueryRow(q, name).Scan(&n); err != nil {
		t.Fatalf("sqlite_master の参照: %v", err)
	}
	return n > 0
}

func TestMigrate_初回適用(t *testing.T) {
	ctx := context.Background()
	sqldb := openTestDB(t)

	fsys := fstest.MapFS{
		"0001_users.sql": {Data: []byte(`CREATE TABLE users (id INTEGER PRIMARY KEY);`)},
		"0002_items.sql": {Data: []byte(`CREATE TABLE items (id INTEGER PRIMARY KEY);`)},
	}

	if err := Migrate(ctx, sqldb, fsys); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	for _, name := range []string{"users", "items", "schema_migrations"} {
		if !tableExists(t, sqldb, name) {
			t.Errorf("テーブル %s が作られていない", name)
		}
	}

	if n := countRows(t, sqldb, `SELECT COUNT(*) FROM schema_migrations`); n != 2 {
		t.Errorf("適用記録が %d 件。2件を期待", n)
	}
}

// 1ファイルに複数の文が入っていても全て実行されること。
// 実際の 0001_init.sql はテーブルとインデックスを十数個含むため、これが動かないと成立しない。
func TestMigrate_1ファイル内の複数文を全て実行する(t *testing.T) {
	ctx := context.Background()
	sqldb := openTestDB(t)

	fsys := fstest.MapFS{
		"0001_init.sql": {Data: []byte(`
CREATE TABLE items (id INTEGER PRIMARY KEY, code TEXT NOT NULL);
CREATE UNIQUE INDEX idx_items_code ON items(code);
INSERT INTO items (code) VALUES ('0001');
`)},
	}

	if err := Migrate(ctx, sqldb, fsys); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	if n := countRows(t, sqldb, `SELECT COUNT(*) FROM items`); n != 1 {
		t.Errorf("items が %d 件。1件を期待（複数文が実行されていない可能性）", n)
	}

	q := `SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'idx_items_code'`
	if n := countRows(t, sqldb, q); n != 1 {
		t.Error("インデックスが作られていない")
	}
}

// 適用順がファイル名の辞書順ではなくバージョン番号順であること。
// 桁が増えた時（0009 の次が 00010 ではなく 0010）に効いてくる。
func TestMigrate_バージョン番号順に適用する(t *testing.T) {
	ctx := context.Background()
	sqldb := openTestDB(t)

	// 辞書順だと "10_..." が "2_..." より前に来る。
	// 2 が作ったテーブルに 10 が列を足すため、順序を誤ると失敗する。
	fsys := fstest.MapFS{
		"2_create.sql":   {Data: []byte(`CREATE TABLE items (id INTEGER PRIMARY KEY);`)},
		"10_add_col.sql": {Data: []byte(`ALTER TABLE items ADD COLUMN note TEXT;`)},
	}

	if err := Migrate(ctx, sqldb, fsys); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	if _, err := sqldb.Exec(`INSERT INTO items (note) VALUES ('x')`); err != nil {
		t.Errorf("列が追加されていない: %v", err)
	}
}

func TestMigrate_再実行しても二重適用されない(t *testing.T) {
	ctx := context.Background()
	sqldb := openTestDB(t)

	// INSERT を含める。二重適用されたら件数が増えるので検知できる。
	fsys := fstest.MapFS{
		"0001_seed.sql": {Data: []byte(`
CREATE TABLE items (id INTEGER PRIMARY KEY, code TEXT);
INSERT INTO items (code) VALUES ('0001');
`)},
	}

	if err := Migrate(ctx, sqldb, fsys); err != nil {
		t.Fatalf("1回目の Migrate: %v", err)
	}
	if err := Migrate(ctx, sqldb, fsys); err != nil {
		t.Fatalf("2回目の Migrate: %v", err)
	}

	if n := countRows(t, sqldb, `SELECT COUNT(*) FROM items`); n != 1 {
		t.Errorf("items が %d 件。1件を期待（二重適用されている）", n)
	}
	if n := countRows(t, sqldb, `SELECT COUNT(*) FROM schema_migrations`); n != 1 {
		t.Errorf("適用記録が %d 件。1件を期待", n)
	}
}

func TestMigrate_後から増えたファイルだけを適用する(t *testing.T) {
	ctx := context.Background()
	sqldb := openTestDB(t)

	first := fstest.MapFS{
		"0001_users.sql": {Data: []byte(`CREATE TABLE users (id INTEGER PRIMARY KEY);`)},
	}
	if err := Migrate(ctx, sqldb, first); err != nil {
		t.Fatalf("1回目の Migrate: %v", err)
	}

	second := fstest.MapFS{
		"0001_users.sql": {Data: []byte(`CREATE TABLE users (id INTEGER PRIMARY KEY);`)},
		"0002_items.sql": {Data: []byte(`CREATE TABLE items (id INTEGER PRIMARY KEY);`)},
	}
	// 0001 が再実行されるなら CREATE TABLE users が衝突して失敗する。
	if err := Migrate(ctx, sqldb, second); err != nil {
		t.Fatalf("2回目の Migrate: %v", err)
	}

	if !tableExists(t, sqldb, "items") {
		t.Error("追加された 0002 が適用されていない")
	}
	if n := countRows(t, sqldb, `SELECT COUNT(*) FROM schema_migrations`); n != 2 {
		t.Errorf("適用記録が %d 件。2件を期待", n)
	}
}

func TestMigrate_途中で失敗したらファイル単位で巻き戻る(t *testing.T) {
	ctx := context.Background()
	sqldb := openTestDB(t)

	fsys := fstest.MapFS{
		"0001_ok.sql": {Data: []byte(`CREATE TABLE users (id INTEGER PRIMARY KEY);`)},
		// 1文目は成功し、2文目が失敗する。1文目だけが残ってはいけない。
		"0002_broken.sql": {Data: []byte(`
CREATE TABLE items (id INTEGER PRIMARY KEY);
CREATE TABLE items (id INTEGER PRIMARY KEY);
`)},
	}

	err := Migrate(ctx, sqldb, fsys)
	if err == nil {
		t.Fatal("エラーを期待したが nil")
	}
	if !strings.Contains(err.Error(), "0002_broken.sql") {
		t.Errorf("どのファイルで失敗したかがエラーに含まれていない: %v", err)
	}

	if !tableExists(t, sqldb, "users") {
		t.Error("成功した 0001 まで巻き戻っている")
	}
	if tableExists(t, sqldb, "items") {
		t.Error("失敗した 0002 の1文目が残っている（トランザクションが効いていない）")
	}
	if n := countRows(t, sqldb, `SELECT COUNT(*) FROM schema_migrations`); n != 1 {
		t.Errorf("適用記録が %d 件。1件を期待（失敗した分が記録されている）", n)
	}
}

// 失敗を直して再実行すれば続きから適用できること。
// 巻き戻るだけで再開できないと、手でDBを直す羽目になる。
func TestMigrate_失敗を直せば続きから適用できる(t *testing.T) {
	ctx := context.Background()
	sqldb := openTestDB(t)

	broken := fstest.MapFS{
		"0001_ok.sql":     {Data: []byte(`CREATE TABLE users (id INTEGER PRIMARY KEY);`)},
		"0002_broken.sql": {Data: []byte(`CREATE TABLE items (id INTEGER PRIMARY KEY;`)},
	}
	if err := Migrate(ctx, sqldb, broken); err == nil {
		t.Fatal("エラーを期待したが nil")
	}

	fixed := fstest.MapFS{
		"0001_ok.sql":     {Data: []byte(`CREATE TABLE users (id INTEGER PRIMARY KEY);`)},
		"0002_broken.sql": {Data: []byte(`CREATE TABLE items (id INTEGER PRIMARY KEY);`)},
	}
	if err := Migrate(ctx, sqldb, fixed); err != nil {
		t.Fatalf("修正後の Migrate: %v", err)
	}

	if !tableExists(t, sqldb, "items") {
		t.Error("修正した 0002 が適用されていない")
	}
}

func TestMigrate_ファイル名が不正なら何も適用しない(t *testing.T) {
	ctx := context.Background()
	sqldb := openTestDB(t)

	fsys := fstest.MapFS{
		"0001_users.sql": {Data: []byte(`CREATE TABLE users (id INTEGER PRIMARY KEY);`)},
		// 連番が無い。どの順で適用すべきか決められない。
		"init.sql": {Data: []byte(`CREATE TABLE items (id INTEGER PRIMARY KEY);`)},
	}

	err := Migrate(ctx, sqldb, fsys)
	if err == nil {
		t.Fatal("エラーを期待したが nil")
	}
	if !strings.Contains(err.Error(), "init.sql") {
		t.Errorf("原因のファイル名がエラーに含まれていない: %v", err)
	}

	// 1件でも適用してしまうと、直して再実行した時の状態が読めなくなる。
	if tableExists(t, sqldb, "users") {
		t.Error("不正なファイルがあるのに 0001 が適用されている")
	}
}

func TestMigrate_バージョンが重複していたら適用しない(t *testing.T) {
	ctx := context.Background()
	sqldb := openTestDB(t)

	// 0001 と 1 は同じバージョンを指す。どちらが先かが環境で変わる。
	fsys := fstest.MapFS{
		"0001_users.sql": {Data: []byte(`CREATE TABLE users (id INTEGER PRIMARY KEY);`)},
		"1_items.sql":    {Data: []byte(`CREATE TABLE items (id INTEGER PRIMARY KEY);`)},
	}

	err := Migrate(ctx, sqldb, fsys)
	if err == nil {
		t.Fatal("エラーを期待したが nil")
	}
	if !strings.Contains(err.Error(), "重複") {
		t.Errorf("重複が原因だと分かるエラーになっていない: %v", err)
	}
	if tableExists(t, sqldb, "users") {
		t.Error("重複があるのに適用されている")
	}
}

// ブランチを分けて開発すると、0002 適用済みの環境に後から 0001 相当が現れることがある。
// 黙って適用すると環境ごとに適用順が変わるため、人間に判断を戻す。
func TestMigrate_適用済みより古い未適用があればエラーにする(t *testing.T) {
	ctx := context.Background()
	sqldb := openTestDB(t)

	first := fstest.MapFS{
		"0002_items.sql": {Data: []byte(`CREATE TABLE items (id INTEGER PRIMARY KEY);`)},
	}
	if err := Migrate(ctx, sqldb, first); err != nil {
		t.Fatalf("1回目の Migrate: %v", err)
	}

	second := fstest.MapFS{
		"0001_users.sql": {Data: []byte(`CREATE TABLE users (id INTEGER PRIMARY KEY);`)},
		"0002_items.sql": {Data: []byte(`CREATE TABLE items (id INTEGER PRIMARY KEY);`)},
	}

	err := Migrate(ctx, sqldb, second)
	if err == nil {
		t.Fatal("エラーを期待したが nil")
	}
	if !strings.Contains(err.Error(), "0001_users.sql") {
		t.Errorf("原因のファイル名がエラーに含まれていない: %v", err)
	}
	if tableExists(t, sqldb, "users") {
		t.Error("順序が逆転しているのに適用されている")
	}
}

// マイグレーションが1件も無くても落ちないこと。
// 空の状態でも schema_migrations は作られる（次の適用で使う）。
func TestMigrate_ファイルが無くても成功する(t *testing.T) {
	ctx := context.Background()
	sqldb := openTestDB(t)

	if err := Migrate(ctx, sqldb, fstest.MapFS{}); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if !tableExists(t, sqldb, "schema_migrations") {
		t.Error("schema_migrations が作られていない")
	}
}
