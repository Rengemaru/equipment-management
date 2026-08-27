package db

import (
	"context"
	"testing"

	_ "modernc.org/sqlite"
)

// 同梱の連番SQLが実際に適用できること。
// ランナー単体のテスト（migrate_test.go）はダミーSQLを使うため、
// 実スキーマの構文エラーはここでしか捕まらない。
func TestMigrations_同梱のSQLが適用できる(t *testing.T) {
	ctx := context.Background()
	sqldb := openTestDB(t)

	if err := Migrate(ctx, sqldb, Migrations()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	for _, name := range []string{"users", "login_attempts", "sessions", "items"} {
		if !tableExists(t, sqldb, name) {
			t.Errorf("テーブル %s が作られていない", name)
		}
	}

	var version int
	var name string
	q := `SELECT version, name FROM schema_migrations ORDER BY version`
	if err := sqldb.QueryRow(q).Scan(&version, &name); err != nil {
		t.Fatalf("適用記録の取得: %v", err)
	}
	if version != 1 || name != "init" {
		t.Errorf("適用記録が (%d, %q)。(1, \"init\") を期待", version, name)
	}
}

// 先のマイルストーンのテーブルを作っていないこと。
// 空のテーブルが存在すると、DBを覗いた人が「実装済みだが使われていない」のか
// 「未実装」のかを区別できなくなる。この判断を後から崩さないための固定。
func TestMigrations_先のマイルストーンのテーブルを作らない(t *testing.T) {
	ctx := context.Background()
	sqldb := openTestDB(t)

	if err := Migrate(ctx, sqldb, Migrations()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// これらを足す時は、このテストから該当の名前を消すこと。
	later := []string{
		"loans",            // M2
		"damage_reports",   // M2
		"missing_reports",  // M3
		"notification_log", // M3
		"inventory_checks", // M3
		"api_tokens",       // M4
	}
	for _, name := range later {
		if tableExists(t, sqldb, name) {
			t.Errorf("M1 では作らないはずのテーブル %s が存在する", name)
		}
	}
}

// 起動のたびに Migrate を呼ぶため、2回目以降が無害であることを実スキーマでも確認する。
func TestMigrations_再実行しても失敗しない(t *testing.T) {
	ctx := context.Background()
	sqldb := openTestDB(t)

	if err := Migrate(ctx, sqldb, Migrations()); err != nil {
		t.Fatalf("1回目の Migrate: %v", err)
	}
	if err := Migrate(ctx, sqldb, Migrations()); err != nil {
		t.Fatalf("2回目の Migrate: %v", err)
	}

	if n := countRows(t, sqldb, `SELECT COUNT(*) FROM schema_migrations`); n != 1 {
		t.Errorf("適用記録が %d 件。1件を期待", n)
	}
}

// CHECK 制約と UNIQUE 制約が生きていること。
// 権限チェックはAPI側でも行うが、role に想定外の値が入らないことはDBで担保する。
func TestMigrations_制約が有効になっている(t *testing.T) {
	ctx := context.Background()
	sqldb := openTestDB(t)

	if err := Migrate(ctx, sqldb, Migrations()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	const insertUser = `INSERT INTO users (name, login_id, password_hash, role) VALUES (?, ?, 'x', ?)`
	if _, err := sqldb.Exec(insertUser, "山田", "yamada", "admin"); err != nil {
		t.Fatalf("正常な users の挿入: %v", err)
	}
	if _, err := sqldb.Exec(insertUser, "田中", "tanaka", "superuser"); err == nil {
		t.Error("role の CHECK 制約が効いていない")
	}
	if _, err := sqldb.Exec(insertUser, "別人", "yamada", "member"); err == nil {
		t.Error("login_id の UNIQUE 制約が効いていない")
	}

	const insertItem = `INSERT INTO items (code, name) VALUES (?, ?)`
	if _, err := sqldb.Exec(insertItem, "0001", "三脚"); err != nil {
		t.Fatalf("正常な items の挿入: %v", err)
	}
	if _, err := sqldb.Exec(insertItem, "0001", "別の三脚"); err == nil {
		t.Error("items.code の UNIQUE 制約が効いていない")
	}

	// 既定値。CSVインポートで空欄だった列がこの値になる。
	var category, condition, locationStatus, owner string
	var isFreeUse int
	q := `SELECT category, condition, location_status, owner, is_free_use FROM items WHERE code = '0001'`
	if err := sqldb.QueryRow(q).Scan(&category, &condition, &locationStatus, &owner, &isFreeUse); err != nil {
		t.Fatalf("既定値の確認: %v", err)
	}
	if category != "未分類" || condition != "良好" || locationStatus != "在庫" || owner != "サークル" || isFreeUse != 0 {
		t.Errorf("既定値が想定と違う: category=%q condition=%q location_status=%q owner=%q is_free_use=%d",
			category, condition, locationStatus, owner, isFreeUse)
	}
}
