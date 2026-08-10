// Package auth は ID + パスワードによる認証と、ユーザーの読み書きを扱う。
//
// 認証はメールに一切依存しない。学内SMTPの可否が未確定な段階で
// ログイン手段をメールに依存させると、SMTP が使えない場合に何も動かなくなる。
package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"

	"github.com/Rengemaru/equipment-management/internal/db"
)

// Role は権限。member は備品マスタを書き換えられない。
type Role string

const (
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

// Valid はDBの CHECK 制約と同じ判定をする。
func (r Role) Valid() bool {
	return r == RoleAdmin || r == RoleMember
}

var (
	// ErrNotFound はユーザーが見つからないこと。
	//
	// 「IDが存在しない」と「パスワードが違う」を呼び出し側で区別できるが、
	// 利用者へのメッセージでは必ず同じにすること。区別できると、
	// 存在するログインIDを総当たりで洗い出せる。
	ErrNotFound = errors.New("ユーザーが見つからない")

	// ErrDuplicateLoginID は同じログインIDが既にあること。
	ErrDuplicateLoginID = errors.New("そのログインIDは既に使われている")

	// ErrDuplicateEmail は同じメールアドレスが既にあること。
	ErrDuplicateEmail = errors.New("そのメールアドレスは既に使われている")

	// ErrPasswordMismatch はパスワードが一致しないこと。
	ErrPasswordMismatch = errors.New("パスワードが違う")
)

// loginIDPattern は許可するログインIDの形。
// 記号を広く許すと、URLやCSVに入れた時のエスケープを都度考えることになる。
var loginIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{2,31}$`)

// hashCost は bcrypt のコスト。既定（10）で、数十名の規模なら十分。
// ログインは長期セッションのため1年に数回しか起きない。
//
// 変数にしているのはテストのため。このパッケージのテストは何十回も
// ハッシュ化するので、既定のままだと -race 込みで分単位になり、
// CI で回すには重すぎる。本番の経路でこの値を書き換えないこと。
var hashCost = bcrypt.DefaultCost

const (
	// minPasswordLen は最小の長さ。数十名の身内利用なので、
	// 複雑さの要求（記号を含める等）は課さない。長さだけ見る。
	minPasswordLen = 8

	// maxPasswordBytes は bcrypt が受け付ける上限。
	// 日本語は1文字3バイトなので、24文字で超える。文字数ではなくバイト数で見る。
	maxPasswordBytes = 72
)

// User は1人のユーザー。
type User struct {
	ID                 int64
	Name               string
	LoginID            string
	Email              string // 未設定は空文字。DBには NULL で入る
	Role               Role
	MustChangePassword bool
	IsActive           bool
	CreatedAt          string
	UpdatedAt          string

	// passwordHash は外に出さない。小文字始まりにしているのは、
	// encoding/json が触れないようにするため。APIのレスポンスに
	// User をそのまま載せてもハッシュが漏れない。
	passwordHash string
}

// VerifyPassword は平文を照合する。一致すれば nil。
func (u *User) VerifyPassword(plain string) error {
	err := bcrypt.CompareHashAndPassword([]byte(u.passwordHash), []byte(plain))
	if err != nil {
		// bcrypt の内部エラーをそのまま返さない。呼び出し側が
		// 「ハッシュが壊れている」と「パスワードが違う」を取り違える。
		return ErrPasswordMismatch
	}
	return nil
}

// NewUser は作成するユーザーの入力。
type NewUser struct {
	Name     string
	LoginID  string
	Email    string
	Role     Role
	Password string

	// MustChangePassword は次回ログイン時にパスワード変更を強制するか。
	// admin が発行した初期パスワードでは true にする。
	MustChangePassword bool
}

// Store は users テーブルへの読み書き。
type Store struct {
	sqldb *sql.DB
}

// NewStore は Store を作る。
func NewStore(sqldb *sql.DB) *Store {
	return &Store{sqldb: sqldb}
}

// HashPassword は平文を bcrypt でハッシュ化する。
//
// 平文は保存しないし、ログにも出さない。この関数の外に平文のまま
// 出ていく経路を作らないこと。
func HashPassword(plain string) (string, error) {
	if err := ValidatePassword(plain); err != nil {
		return "", err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(plain), hashCost)
	if err != nil {
		return "", fmt.Errorf("パスワードのハッシュ化: %w", err)
	}

	return string(hash), nil
}

// ValidatePassword は長さだけを検査する。
func ValidatePassword(plain string) error {
	if utf8.RuneCountInString(plain) < minPasswordLen {
		return fmt.Errorf("パスワードは%d文字以上にする", minPasswordLen)
	}
	// bcrypt は72バイトを超えると黙って切るのではなくエラーを返す版がある。
	// 先に弾いて、原因の分かるメッセージにする。
	if len(plain) > maxPasswordBytes {
		return fmt.Errorf("パスワードが長すぎる（%dバイト。%dバイトまで）",
			len(plain), maxPasswordBytes)
	}
	return nil
}

// NormalizeLoginID は照合用に正規化する。
// 'Yamada' と 'yamada' を別人にしないため、小文字に寄せる。
func NormalizeLoginID(loginID string) string {
	return strings.ToLower(strings.TrimSpace(loginID))
}

// ValidateLoginID は形を検査する。正規化済みの値を渡すこと。
func ValidateLoginID(loginID string) error {
	if !loginIDPattern.MatchString(loginID) {
		return fmt.Errorf(
			"ログインIDは英数字・ハイフン・アンダースコアの3〜32文字にする（英数字で始める）: %q",
			loginID)
	}
	return nil
}

// Create はユーザーを作る。パスワードは平文で受け取り、この中でハッシュ化する。
//
// ハッシュを受け取る形にすると、呼び出し側それぞれがハッシュ化することになり、
// いつか平文のまま渡す経路ができる。
func (s *Store) Create(ctx context.Context, in NewUser) (*User, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, errors.New("名前が空")
	}

	loginID := NormalizeLoginID(in.LoginID)
	if err := ValidateLoginID(loginID); err != nil {
		return nil, err
	}

	if !in.Role.Valid() {
		return nil, fmt.Errorf("権限が不正: %q", in.Role)
	}

	hash, err := HashPassword(in.Password)
	if err != nil {
		return nil, err
	}

	const q = `
INSERT INTO users (name, login_id, password_hash, must_change_password, email, role)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING id, created_at, updated_at`

	u := &User{
		Name:               name,
		LoginID:            loginID,
		Email:              strings.TrimSpace(in.Email),
		Role:               in.Role,
		MustChangePassword: in.MustChangePassword,
		IsActive:           true,
		passwordHash:       hash,
	}

	err = s.sqldb.QueryRowContext(ctx, q,
		u.Name, u.LoginID, hash, boolToInt(in.MustChangePassword), emailValue(u.Email), string(in.Role),
	).Scan(&u.ID, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if db.IsUniqueViolation(err) {
			// どちらの列で衝突したかはメッセージに含まれる。
			if strings.Contains(err.Error(), "users.email") {
				return nil, ErrDuplicateEmail
			}
			return nil, ErrDuplicateLoginID
		}
		return nil, fmt.Errorf("ユーザーの作成: %w", err)
	}

	return u, nil
}

// ByLoginID はログインIDで引く。大文字小文字は区別しない。
func (s *Store) ByLoginID(ctx context.Context, loginID string) (*User, error) {
	return s.queryOne(ctx, selectColumns+` WHERE login_id = ?`, NormalizeLoginID(loginID))
}

// ByID はIDで引く。
func (s *Store) ByID(ctx context.Context, id int64) (*User, error) {
	return s.queryOne(ctx, selectColumns+` WHERE id = ?`, id)
}

// selectColumns は User を組み立てる列の並び。scanUser と対で保つ。
const selectColumns = `
SELECT id, name, login_id, password_hash, must_change_password, email, role, is_active, created_at, updated_at
FROM users`

func (s *Store) queryOne(ctx context.Context, query string, args ...any) (*User, error) {
	u, err := scanUser(s.sqldb.QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("ユーザーの取得: %w", err)
	}
	return u, nil
}

// row は *sql.Row と *sql.Rows のどちらでも受けられるようにする。
type row interface {
	Scan(dest ...any) error
}

func scanUser(r row) (*User, error) {
	var (
		u                  User
		email              sql.NullString
		mustChangePassword int
		isActive           int
		role               string
	)

	err := r.Scan(
		&u.ID, &u.Name, &u.LoginID, &u.passwordHash, &mustChangePassword,
		&email, &role, &isActive, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	u.Email = email.String
	u.MustChangePassword = mustChangePassword != 0
	u.IsActive = isActive != 0
	u.Role = Role(role)

	return &u, nil
}

// emailValue は空文字を NULL に変える。
//
// 空文字のまま入れると、メール未設定のユーザーが2人目から
// UNIQUE 制約に引っかかる（SQLite では ” = ” だが NULL ≠ NULL）。
func emailValue(email string) any {
	if email == "" {
		return nil
	}
	return email
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
