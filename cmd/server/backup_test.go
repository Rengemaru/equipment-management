package main

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rengemaru/equipment-management/internal/auth"
	"github.com/Rengemaru/equipment-management/internal/db"
)

// seedUser は検証用に利用者を1人入れる。
func seedUser(t *testing.T, sqldb *sql.DB, loginID string) {
	t.Helper()

	_, err := auth.NewStore(sqldb).Create(context.Background(), auth.NewUser{
		Name:     "山田太郎",
		LoginID:  loginID,
		Role:     auth.RoleAdmin,
		Password: "backup-test-password",
	})
	if err != nil {
		t.Fatalf("利用者の作成: %v", err)
	}
}

// __バックアップの価値は「戻せること」にしかない。__ 作れたことではなく、
// 作ったファイルを開いて中身が読めることを確かめる。
func TestBackupCanBeOpenedAndRead(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sqldb := newTestDB(t)
	seedUser(t, sqldb, "yamada")

	dest := filepath.Join(t.TempDir(), "backup.db")

	var out bytes.Buffer
	if err := runBackup(ctx, sqldb, dest, &out); err != nil {
		t.Fatalf("runBackup: %v", err)
	}

	// 別プロセスから開くのと同じ経路で開く。
	restored, err := db.Open(ctx, dest)
	if err != nil {
		t.Fatalf("バックアップを開けない: %v", err)
	}
	defer func() { _ = restored.Close() }()

	var loginID string
	err = restored.QueryRowContext(ctx, `SELECT login_id FROM users WHERE login_id = ?`, "yamada").
		Scan(&loginID)
	if err != nil {
		t.Fatalf("バックアップから利用者を読めない: %v", err)
	}
	if loginID != "yamada" {
		t.Errorf("login_id = %q, 期待 %q", loginID, "yamada")
	}
}

// 「取れているつもりで0バイト」に気付けるよう、件数と大きさを出す。
func TestBackupReportsContents(t *testing.T) {
	t.Parallel()

	sqldb := newTestDB(t)
	seedUser(t, sqldb, "yamada")

	var out bytes.Buffer
	dest := filepath.Join(t.TempDir(), "backup.db")
	if err := runBackup(context.Background(), sqldb, dest, &out); err != nil {
		t.Fatalf("runBackup: %v", err)
	}

	got := out.String()
	for _, want := range []string{"利用者 1件", "備品 0件", "docker compose cp"} {
		if !strings.Contains(got, want) {
			t.Errorf("出力に %q が無い:\n%s", want, got)
		}
	}
}

// 上書きすると、誤って上書きした結果しか残らない状態になる。
func TestBackupDoesNotOverwrite(t *testing.T) {
	t.Parallel()

	sqldb := newTestDB(t)
	dest := filepath.Join(t.TempDir(), "backup.db")

	if err := os.WriteFile(dest, []byte("大事な前回のバックアップ"), 0o644); err != nil {
		t.Fatalf("準備: %v", err)
	}

	var out bytes.Buffer
	err := runBackup(context.Background(), sqldb, dest, &out)
	if err == nil {
		t.Fatal("既存ファイルがあるのにエラーにならない")
	}

	// 中身が残っていること。エラーを返しても消していたら意味がない。
	data, readErr := os.ReadFile(dest)
	if readErr != nil {
		t.Fatalf("既存ファイルの読み込み: %v", readErr)
	}
	if string(data) != "大事な前回のバックアップ" {
		t.Errorf("既存ファイルが壊れている: %q", data)
	}
}

// VACUUM INTO は出力先のディレクトリを作らない。
func TestBackupCreatesParentDirectory(t *testing.T) {
	t.Parallel()

	sqldb := newTestDB(t)
	dest := filepath.Join(t.TempDir(), "backups", "2026-08", "backup.db")

	var out bytes.Buffer
	if err := runBackup(context.Background(), sqldb, dest, &out); err != nil {
		t.Fatalf("runBackup: %v", err)
	}

	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("バックアップが無い: %v", err)
	}
}

// 出力先を忘れた時に、何を渡せばよいかが分かること。
func TestBackupWithoutPath(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	err := runBackup(context.Background(), newTestDB(t), "", &out)

	if err == nil {
		t.Fatal("パス無しでエラーにならない")
	}
	if !strings.Contains(err.Error(), "-backup") {
		t.Errorf("使い方が示されていない: %v", err)
	}
}

// WAL に残っている書き込みも畳み込まれること。
// app.db だけを写す方式ではここが落ちる。
func TestBackupIncludesUncheckpointedWrites(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sqldb := newTestDB(t)

	// 直前に書いた内容が WAL 側にある状態を作る。
	seedUser(t, sqldb, "just-written")

	dest := filepath.Join(t.TempDir(), "backup.db")
	var out bytes.Buffer
	if err := runBackup(ctx, sqldb, dest, &out); err != nil {
		t.Fatalf("runBackup: %v", err)
	}

	restored, err := db.Open(ctx, dest)
	if err != nil {
		t.Fatalf("バックアップを開けない: %v", err)
	}
	defer func() { _ = restored.Close() }()

	var n int
	if err := restored.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users WHERE login_id = ?`, "just-written").Scan(&n); err != nil {
		t.Fatalf("読み込み: %v", err)
	}
	if n != 1 {
		t.Errorf("直前の書き込みが含まれていない（件数=%d）", n)
	}
}
