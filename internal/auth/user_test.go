package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
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

// validUser は作成できる最小の入力を返す。
func validUser() NewUser {
	return NewUser{
		Name:               "山田太郎",
		LoginID:            "yamada",
		Role:               RoleMember,
		Password:           "password123",
		MustChangePassword: true,
	}
}

func TestCreate_作成して引ける(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	created, err := s.Create(ctx, validUser())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == 0 {
		t.Error("ID が採番されていない")
	}
	if created.CreatedAt == "" {
		t.Error("CreatedAt が入っていない")
	}

	got, err := s.ByLoginID(ctx, "yamada")
	if err != nil {
		t.Fatalf("ByLoginID: %v", err)
	}
	if got.ID != created.ID || got.Name != "山田太郎" {
		t.Errorf("引けた内容が違う: %+v", got)
	}
	if got.Role != RoleMember {
		t.Errorf("Role = %q", got.Role)
	}
	if !got.MustChangePassword {
		t.Error("MustChangePassword が落ちている")
	}
	if !got.IsActive {
		t.Error("IsActive の既定が false になっている")
	}
}

// 'Yamada' と 'yamada' を別人にしない。
func TestCreate_ログインIDを小文字に正規化する(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	in := validUser()
	in.LoginID = "  Yamada  "

	created, err := s.Create(ctx, in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.LoginID != "yamada" {
		t.Errorf("LoginID = %q。正規化されていない", created.LoginID)
	}

	// 大文字で引いても同じ人が返ること。
	got, err := s.ByLoginID(ctx, "YAMADA")
	if err != nil {
		t.Fatalf("ByLoginID: %v", err)
	}
	if got.ID != created.ID {
		t.Error("大文字で引くと別人になる")
	}
}

func TestCreate_ログインIDの重複を区別できる形で返す(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	if _, err := s.Create(ctx, validUser()); err != nil {
		t.Fatalf("1人目の Create: %v", err)
	}

	in := validUser()
	in.Name = "別人"

	_, err := s.Create(ctx, in)
	if !errors.Is(err, ErrDuplicateLoginID) {
		t.Fatalf("err = %v。ErrDuplicateLoginID を期待", err)
	}
}

// メール未設定のユーザーは何人でも作れること。
// 空文字のまま入れると2人目から UNIQUE に引っかかる（NULL ≠ NULL、” = ”）。
func TestCreate_メール未設定を複数作れる(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	for _, id := range []string{"yamada", "tanaka", "suzuki"} {
		in := validUser()
		in.LoginID = id
		in.Email = ""

		if _, err := s.Create(ctx, in); err != nil {
			t.Fatalf("%s の Create: %v", id, err)
		}
	}
}

func TestCreate_メールの重複を弾く(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	in := validUser()
	in.Email = "yamada@example.ac.jp"
	if _, err := s.Create(ctx, in); err != nil {
		t.Fatalf("1人目の Create: %v", err)
	}

	in2 := validUser()
	in2.LoginID = "tanaka"
	in2.Email = "yamada@example.ac.jp"

	_, err := s.Create(ctx, in2)
	if !errors.Is(err, ErrDuplicateEmail) {
		t.Fatalf("err = %v。ErrDuplicateEmail を期待", err)
	}
}

func TestCreate_不正なログインIDを弾く(t *testing.T) {
	tests := []struct {
		name    string
		loginID string
	}{
		{"短すぎる", "ab"},
		{"記号", "yamada@example"},
		{"空白入り", "yama da"},
		{"日本語", "山田"},
		{"記号始まり", "_yamada"},
		{"空", ""},
		{"長すぎる", strings.Repeat("a", 33)},
	}

	ctx := context.Background()
	s := newTestStore(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validUser()
			in.LoginID = tt.loginID

			if _, err := s.Create(ctx, in); err == nil {
				t.Errorf("%q が通ってしまう", tt.loginID)
			}
		})
	}
}

func TestCreate_不正な権限を弾く(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	in := validUser()
	in.Role = "superuser"

	if _, err := s.Create(ctx, in); err == nil {
		t.Error("不正な権限が通ってしまう")
	}
}

func TestByLoginID_居なければErrNotFound(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	_, err := s.ByLoginID(ctx, "nobody")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v。ErrNotFound を期待", err)
	}
}

func TestByID_作成したユーザーを引ける(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	created, err := s.Create(ctx, validUser())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.ByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if got.LoginID != created.LoginID {
		t.Errorf("LoginID = %q", got.LoginID)
	}

	if _, err := s.ByID(ctx, 9999); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v。ErrNotFound を期待", err)
	}
}

func TestVerifyPassword_正しい時だけ通る(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	if _, err := s.Create(ctx, validUser()); err != nil {
		t.Fatalf("Create: %v", err)
	}

	u, err := s.ByLoginID(ctx, "yamada")
	if err != nil {
		t.Fatalf("ByLoginID: %v", err)
	}

	if err := u.VerifyPassword("password123"); err != nil {
		t.Errorf("正しいパスワードが通らない: %v", err)
	}
	if err := u.VerifyPassword("password124"); !errors.Is(err, ErrPasswordMismatch) {
		t.Errorf("err = %v。ErrPasswordMismatch を期待", err)
	}
	if err := u.VerifyPassword(""); err == nil {
		t.Error("空のパスワードが通ってしまう")
	}
}

// 平文をDBに入れない。
func TestCreate_平文をDBに保存しない(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	if _, err := s.Create(ctx, validUser()); err != nil {
		t.Fatalf("Create: %v", err)
	}

	var hash string
	q := `SELECT password_hash FROM users WHERE login_id = 'yamada'`
	if err := s.sqldb.QueryRowContext(ctx, q).Scan(&hash); err != nil {
		t.Fatalf("password_hash の取得: %v", err)
	}

	if strings.Contains(hash, "password123") {
		t.Fatal("平文が保存されている")
	}
	// bcrypt のハッシュは $2a$ などで始まる。
	if !strings.HasPrefix(hash, "$2") {
		t.Errorf("bcrypt ハッシュに見えない: %q", hash)
	}
}

func TestValidatePassword_長さを検査する(t *testing.T) {
	tests := []struct {
		name     string
		password string
		ok       bool
	}{
		{"8文字ちょうど", "12345678", true},
		{"7文字", "1234567", false},
		{"空", "", false},
		{"日本語8文字", "ぱすわーどです８", true},
		// bcrypt は72バイトまで。日本語は1文字3バイトなので24文字で超える。
		{"日本語25文字", strings.Repeat("あ", 25), false},
		{"英数72文字", strings.Repeat("a", 72), true},
		{"英数73文字", strings.Repeat("a", 73), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePassword(tt.password)
			if tt.ok && err != nil {
				t.Errorf("通るべき: %v", err)
			}
			if !tt.ok && err == nil {
				t.Error("弾くべき")
			}
		})
	}
}

// 長すぎるパスワードで bcrypt が失敗する前に、原因の分かるエラーを返すこと。
func TestHashPassword_長すぎるパスワードを弾く(t *testing.T) {
	_, err := HashPassword(strings.Repeat("a", 73))
	if err == nil {
		t.Fatal("エラーを期待したが nil")
	}
	if !strings.Contains(err.Error(), "長すぎる") {
		t.Errorf("原因が分からない: %v", err)
	}
}

// 同じパスワードでも毎回違うハッシュになること（ソルトが効いている）。
func TestHashPassword_毎回違うハッシュになる(t *testing.T) {
	a, err := HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	b, err := HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	if a == b {
		t.Error("同じハッシュになっている。ソルトが効いていない")
	}
}

// User を JSON にしてもハッシュが出ないこと。
// passwordHash を小文字始まりにしている理由がこれ。
func TestUser_JSONにハッシュが出ない(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	if _, err := s.Create(ctx, validUser()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	u, err := s.ByLoginID(ctx, "yamada")
	if err != nil {
		t.Fatalf("ByLoginID: %v", err)
	}

	encoded, err := jsonMarshal(u)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if strings.Contains(encoded, "$2") || strings.Contains(strings.ToLower(encoded), "hash") {
		t.Errorf("ハッシュが JSON に出ている: %s", encoded)
	}
}

// db パッケージの UNIQUE 判定が、実際のドライバのエラーで動くこと。
func TestIsUniqueViolation_実際のエラーで判定できる(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	if _, err := s.Create(ctx, validUser()); err != nil {
		t.Fatalf("Create: %v", err)
	}

	const q = `INSERT INTO users (name, login_id, password_hash) VALUES ('別人', 'yamada', 'x')`
	_, err := s.sqldb.ExecContext(ctx, q)
	if err == nil {
		t.Fatal("重複が通ってしまう")
	}
	if !db.IsUniqueViolation(err) {
		t.Errorf("UNIQUE 違反と判定されない: %v", err)
	}

	// 無関係なエラーを UNIQUE 違反と誤判定しないこと。
	if db.IsUniqueViolation(sql.ErrNoRows) {
		t.Error("無関係なエラーを UNIQUE 違反と判定している")
	}
}

// jsonMarshal は encoding/json の薄い包み。テストの意図を読みやすくするために置く。
func jsonMarshal(v any) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}
