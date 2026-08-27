package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rengemaru/equipment-management/internal/db"
)

// backupOf は検証用のバックアップを1つ作って、そのパスを返す。
func backupOf(t *testing.T, loginID string) string {
	t.Helper()

	ctx := context.Background()
	sqldb := newTestDB(t)
	seedUser(t, sqldb, loginID)

	dest := filepath.Join(t.TempDir(), "backup.db")
	if err := runBackup(ctx, sqldb, dest, &bytes.Buffer{}); err != nil {
		t.Fatalf("runBackup: %v", err)
	}

	return dest
}

// __これがM1の仕上げで一番確かめたいこと。__ 取れたことではなく、
// 取ったものから戻せることを見る。復元できないバックアップは、
// 無いことに気付けないぶん無いより悪い。
func TestRestore_バックアップから戻せる(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	src := backupOf(t, "yamada")
	dest := filepath.Join(t.TempDir(), "app.db")

	var out bytes.Buffer
	if err := runRestore(ctx, src, dest, &out); err != nil {
		t.Fatalf("runRestore: %v", err)
	}

	restored, err := db.Open(ctx, dest)
	if err != nil {
		t.Fatalf("戻したDBを開けない: %v", err)
	}
	defer func() { _ = restored.Close() }()

	var loginID string
	err = restored.QueryRowContext(ctx,
		`SELECT login_id FROM users WHERE login_id = ?`, "yamada").Scan(&loginID)
	if err != nil {
		t.Fatalf("戻したDBから利用者を読めない: %v", err)
	}
	if loginID != "yamada" {
		t.Errorf("login_id = %q, want yamada", loginID)
	}

	// 件数まで出す。0件のDBを「戻せた」と報告させない。
	if !strings.Contains(out.String(), "復元しました") {
		t.Errorf("復元の報告が無い:\n%s", out.String())
	}
}

// 既にDBがある状態に上書きできること。実際に使う時は必ずこちら。
// 中身の合わない -wal が残ると、次に開いた時にそれを被せようとする。
func TestRestore_既存のDBとWALを置き換える(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	src := backupOf(t, "yamada")

	// 別の中身を持つ「今動いているDB」を作る。
	dir := t.TempDir()
	dest := filepath.Join(dir, "app.db")
	live, err := db.Open(ctx, dest)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	if err := db.Migrate(ctx, live, db.Migrations()); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}
	seedUser(t, live, "tanaka")
	if err := live.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if err := runRestore(ctx, src, dest, &bytes.Buffer{}); err != nil {
		t.Fatalf("runRestore: %v", err)
	}

	// 消し残しがあってはならない。
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(dest + suffix); err == nil {
			t.Errorf("%s が残っている", dest+suffix)
		}
	}

	restored, err := db.Open(ctx, dest)
	if err != nil {
		t.Fatalf("戻したDBを開けない: %v", err)
	}
	defer func() { _ = restored.Close() }()

	// 戻す元の中身に入れ替わっていること。
	var n int
	if err := restored.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users WHERE login_id = ?`, "tanaka").Scan(&n); err != nil {
		t.Fatalf("件数の取得: %v", err)
	}
	if n != 0 {
		t.Error("元のDBの利用者が残っている（置き換わっていない）")
	}

	if err := restored.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users WHERE login_id = ?`, "yamada").Scan(&n); err != nil {
		t.Fatalf("件数の取得: %v", err)
	}
	if n != 1 {
		t.Error("バックアップの利用者が入っていない")
	}
}

// __壊れたファイルで上書きしてから気付くと、戻す元も戻す先も失う。__
// 先に中身を確かめ、駄目なら戻す先に触らないこと。
func TestRestore_壊れた元では戻す先に触らない(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dir := t.TempDir()

	src := filepath.Join(dir, "broken.db")
	if err := os.WriteFile(src, []byte("これはSQLiteのファイルではない"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	dest := filepath.Join(dir, "app.db")
	const sentinel = "元のまま"
	if err := os.WriteFile(dest, []byte(sentinel), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := runRestore(ctx, src, dest, &bytes.Buffer{}); err == nil {
		t.Fatal("runRestore = nil, want error")
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != sentinel {
		t.Error("戻す先が壊された。検証は上書きより先に行うこと")
	}
}

func TestRestore_元が無ければ失敗する(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	err := runRestore(context.Background(),
		filepath.Join(dir, "ない.db"), filepath.Join(dir, "app.db"), &bytes.Buffer{})
	if err == nil {
		t.Fatal("runRestore = nil, want error")
	}
}

func TestRestore_パスを渡さなければ使い方を出す(t *testing.T) {
	t.Parallel()

	err := runRestore(context.Background(), "", "/data/app.db", &bytes.Buffer{})
	if err == nil {
		t.Fatal("runRestore = nil, want error")
	}
	if !strings.Contains(err.Error(), "-restore") {
		t.Errorf("使い方が出ていない: %v", err)
	}
}

// 同じパスを渡すと、消してから写すため中身が消える。先に止める。
func TestRestore_元と先が同じなら拒否する(t *testing.T) {
	t.Parallel()

	src := backupOf(t, "yamada")

	if err := runRestore(context.Background(), src, src, &bytes.Buffer{}); err == nil {
		t.Fatal("runRestore = nil, want error")
	}

	// 消されていないこと。
	if _, err := os.Stat(src); err != nil {
		t.Errorf("戻す元が消えた: %v", err)
	}
}
