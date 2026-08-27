package main

import (
	"bytes"
	"context"
	"database/sql"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Rengemaru/equipment-management/internal/auth"
	"github.com/Rengemaru/equipment-management/internal/db"
)

// newTestDB はスキーマ適用済みのDBを返す。
func newTestDB(t *testing.T) *sql.DB {
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

	return sqldb
}

// passwordFromOutput は出力から初期パスワードを取り出す。
func passwordFromOutput(t *testing.T, out string) string {
	t.Helper()

	m := regexp.MustCompile(`初期パスワード\s*:\s*(\S+)`).FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("初期パスワードが出力されていない:\n%s", out)
	}
	return m[1]
}

func TestRunCreateAdmin_作成して表示したパスワードでログインできる(t *testing.T) {
	ctx := context.Background()
	sqldb := newTestDB(t)

	var out bytes.Buffer
	in := createAdminInput{LoginID: "yamada", Name: "山田太郎"}
	if err := runCreateAdmin(ctx, sqldb, in, &out); err != nil {
		t.Fatalf("runCreateAdmin: %v", err)
	}

	store := auth.NewStore(sqldb)
	user, err := store.ByLoginID(ctx, "yamada")
	if err != nil {
		t.Fatalf("ByLoginID: %v", err)
	}

	if user.Role != auth.RoleAdmin {
		t.Errorf("Role = %q。admin を期待", user.Role)
	}
	// 生成した文字列は控えとして残りやすい。そのまま使い続けさせない。
	if !user.MustChangePassword {
		t.Error("MustChangePassword が立っていない")
	}
	if !user.IsActive {
		t.Error("IsActive が false")
	}

	// 表示されたパスワードで実際に認証が通ること。
	// ここが通らないと、作成できても誰もログインできない。
	password := passwordFromOutput(t, out.String())
	if err := user.VerifyPassword(password); err != nil {
		t.Errorf("表示されたパスワードで認証できない: %v", err)
	}
}

func TestRunCreateAdmin_必須の引数が無ければ何も作らない(t *testing.T) {
	tests := []struct {
		name string
		in   createAdminInput
	}{
		{"ログインIDが無い", createAdminInput{Name: "山田太郎"}},
		{"名前が無い", createAdminInput{LoginID: "yamada"}},
		{"両方無い", createAdminInput{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			sqldb := newTestDB(t)

			var out bytes.Buffer
			err := runCreateAdmin(ctx, sqldb, tt.in, &out)
			if err == nil {
				t.Fatal("エラーを期待したが nil")
			}
			// 使い方が分からないと、引数名を探しに行くことになる。
			if !strings.Contains(err.Error(), "-login-id") {
				t.Errorf("使い方が示されていない: %v", err)
			}

			var n int
			if err := sqldb.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
				t.Fatalf("件数の取得: %v", err)
			}
			if n != 0 {
				t.Errorf("ユーザーが %d 件作られている", n)
			}
		})
	}
}

func TestRunCreateAdmin_同じログインIDは分かる形で断る(t *testing.T) {
	ctx := context.Background()
	sqldb := newTestDB(t)

	in := createAdminInput{LoginID: "yamada", Name: "山田太郎"}
	if err := runCreateAdmin(ctx, sqldb, in, &bytes.Buffer{}); err != nil {
		t.Fatalf("1回目: %v", err)
	}

	err := runCreateAdmin(ctx, sqldb, in, &bytes.Buffer{})
	if err == nil {
		t.Fatal("エラーを期待したが nil")
	}
	if !strings.Contains(err.Error(), "yamada") {
		t.Errorf("どのIDが重複したか分からない: %v", err)
	}
}

// 2人目以降の admin も作れること。引き継ぎで admin を増やす場面がある。
func TestRunCreateAdmin_複数のadminを作れる(t *testing.T) {
	ctx := context.Background()
	sqldb := newTestDB(t)

	for _, id := range []string{"yamada", "tanaka"} {
		in := createAdminInput{LoginID: id, Name: id}
		if err := runCreateAdmin(ctx, sqldb, in, &bytes.Buffer{}); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
	}

	var n int
	if err := sqldb.QueryRow(`SELECT COUNT(*) FROM users WHERE role = 'admin'`).Scan(&n); err != nil {
		t.Fatalf("件数の取得: %v", err)
	}
	if n != 2 {
		t.Errorf("admin が %d 人。2人を期待", n)
	}
}

// 実行のたびに違うパスワードになること。
func TestRunCreateAdmin_毎回違うパスワードを発行する(t *testing.T) {
	ctx := context.Background()
	sqldb := newTestDB(t)

	var passwords []string
	for _, id := range []string{"yamada", "tanaka", "suzuki"} {
		var out bytes.Buffer
		in := createAdminInput{LoginID: id, Name: id}
		if err := runCreateAdmin(ctx, sqldb, in, &out); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		passwords = append(passwords, passwordFromOutput(t, out.String()))
	}

	for i := range passwords {
		for j := i + 1; j < len(passwords); j++ {
			if passwords[i] == passwords[j] {
				t.Fatalf("同じパスワードが発行された: %q", passwords[i])
			}
		}
	}
}
