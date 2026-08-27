package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

// openAt は指定パスに Open する。失敗したらテストを止める。
func openAt(t *testing.T, path string) *sql.DB {
	t.Helper()

	sqldb, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	t.Cleanup(func() { _ = sqldb.Close() })

	return sqldb
}

func TestOpen_接続できてファイルが作られる(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")
	sqldb := openAt(t, path)

	if err := sqldb.PingContext(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestOpen_パスが空ならエラー(t *testing.T) {
	if _, err := Open(context.Background(), ""); err == nil {
		t.Fatal("エラーを期待したが nil")
	}
}

// sql.Open は遅延接続なので、Open が Ping まで済ませていないと
// 「起動は成功したが最初のリクエストで落ちる」状態になる。
func TestOpen_開けないパスなら起動時にエラーになる(t *testing.T) {
	path := filepath.Join(t.TempDir(), "存在しない階層", "app.db")

	sqldb, err := Open(context.Background(), path)
	if err == nil {
		_ = sqldb.Close()
		t.Fatal("エラーを期待したが nil")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("どのパスで失敗したかがエラーに含まれていない: %v", err)
	}
}

// PRAGMA は接続ごとの設定。プール内の全接続に効いていないと、
// 外部キーが効く接続と効かない接続が混在する。
func TestOpen_PRAGMAがプール内の全接続に効いている(t *testing.T) {
	ctx := context.Background()
	sqldb := openAt(t, filepath.Join(t.TempDir(), "app.db"))

	// 同時に掴んで、確実に別々の接続を開かせる。
	conns := make([]*sql.Conn, 0, maxOpenConns)
	for i := 0; i < maxOpenConns; i++ {
		c, err := sqldb.Conn(ctx)
		if err != nil {
			t.Fatalf("接続 %d の取得: %v", i, err)
		}
		conns = append(conns, c)
	}
	defer func() {
		for _, c := range conns {
			_ = c.Close()
		}
	}()

	for i, c := range conns {
		var foreignKeys int
		if err := c.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
			t.Fatalf("接続 %d の foreign_keys: %v", i, err)
		}
		if foreignKeys != 1 {
			t.Errorf("接続 %d の foreign_keys が %d。1 を期待", i, foreignKeys)
		}

		var journalMode string
		if err := c.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&journalMode); err != nil {
			t.Fatalf("接続 %d の journal_mode: %v", i, err)
		}
		if !strings.EqualFold(journalMode, "wal") {
			t.Errorf("接続 %d の journal_mode が %q。wal を期待", i, journalMode)
		}

		var busyTimeout int
		if err := c.QueryRowContext(ctx, `PRAGMA busy_timeout`).Scan(&busyTimeout); err != nil {
			t.Fatalf("接続 %d の busy_timeout: %v", i, err)
		}
		if busyTimeout == 0 {
			t.Errorf("接続 %d の busy_timeout が 0。待たずに失敗する", i)
		}
	}
}

// PRAGMA の値が正しいだけでなく、実際に参照が拒否されること。
func TestOpen_外部キー制約が実際に効く(t *testing.T) {
	ctx := context.Background()
	sqldb := openAt(t, filepath.Join(t.TempDir(), "app.db"))

	if err := Migrate(ctx, sqldb, Migrations()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// 存在しない user_id を参照するセッション。
	const q = `INSERT INTO sessions (id, user_id, expires_at) VALUES ('s1', 999, '2027-01-01')`
	if _, err := sqldb.ExecContext(ctx, q); err == nil {
		t.Error("参照先の無い user_id が挿入できてしまった")
	}
}

// WAL が有効なら、書き込みトランザクション中でも読み取りが止まらない。
// これが成立しないと、貸出登録のたびに一覧が待たされる。
func TestOpen_書き込み中でも読み取れる(t *testing.T) {
	ctx := context.Background()
	sqldb := openAt(t, filepath.Join(t.TempDir(), "app.db"))

	if err := Migrate(ctx, sqldb, Migrations()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	tx, err := sqldb.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	const insert = `INSERT INTO items (code, name) VALUES ('0001', '三脚')`
	if _, err := tx.ExecContext(ctx, insert); err != nil {
		t.Fatalf("書き込み: %v", err)
	}

	// 未コミットの書き込みトランザクションを開いたまま、別接続から読む。
	var n int
	if err := sqldb.QueryRowContext(ctx, `SELECT COUNT(*) FROM items`).Scan(&n); err != nil {
		t.Fatalf("書き込み中の読み取り: %v", err)
	}
	if n != 0 {
		t.Errorf("未コミットの行が %d 件見えている。0件を期待", n)
	}
}
