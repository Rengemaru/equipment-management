package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"time"
)

// ErrSessionInvalid はセッションが無効なこと。
//
// 「そんなセッションは無い」「期限切れ」「利用者が無効化された」を区別しない。
// 呼び出し側から見れば、どれもログインし直してもらうしかない。
var ErrSessionInvalid = errors.New("セッションが無効")

const (
	// sessionTokenBytes はセッションIDの乱数の長さ。
	// 256ビット。総当たりは現実的でない。
	sessionTokenBytes = 32

	// SessionDuration はセッションの有効期限。
	//
	// 1年。2回目以降にログイン操作を発生させないための長さで、
	// 「記録する手間 < 記録しない手間」を保つ土台になる。
	// ここを短くすると、QRを読むたびにログインを求められて記録されなくなる。
	SessionDuration = 365 * 24 * time.Hour

	// lastSeenInterval は last_seen_at を更新する間隔。
	//
	// リクエストのたびに更新すると、閲覧しかしていない利用者が
	// 常に書き込みを起こす。SQLite の書き込みは同時に1つしか通らない。
	lastSeenInterval = time.Hour

	// sqliteTimeLayout は datetime('now') が返す形式。
	sqliteTimeLayout = "2006-01-02 15:04:05"
)

// SessionStore は sessions テーブルへの読み書き。
type SessionStore struct {
	sqldb  *sql.DB
	secret []byte
}

// NewSessionStore は SessionStore を作る。
//
// secret は SESSION_SECRET。セッションIDの保存形を決めるのに使い、
// 変更すると既存のセッションは全て引けなくなる（＝全員ログアウト）。
func NewSessionStore(sqldb *sql.DB, secret []byte) *SessionStore {
	return &SessionStore{sqldb: sqldb, secret: secret}
}

// storageID は Cookie の値から、DBに保存する識別子を作る。
//
// Cookie の値をそのまま sessions.id に入れない。DBファイルは
// バックアップとして持ち出され、引き継ぎで後任にも渡る。中身が読めれば、
// 有効期限1年のセッションをそのまま使って誰にでもなりすませる。
//
// SESSION_SECRET を混ぜているのは、鍵の変更で全セッションを無効化できるようにするため
// （.env.example にそう書いてある）。ハッシュだけだと鍵を変えても効かない。
//
// bcrypt ではなく SHA-256 を使う。元の値が32バイトの乱数で、
// 総当たりの対象にならないため、伸長は不要。リクエストごとに走る処理でもある。
func (s *SessionStore) storageID(token string) string {
	sum := sha256.Sum256(append(append([]byte{}, s.secret...), []byte(":"+token)...))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// Create はセッションを作り、Cookie に入れる値と有効期限を返す。
//
// 戻り値の token は、この時しか手に入らない（DBには保存形しか無い）。
func (s *SessionStore) Create(ctx context.Context, userID int64) (token string, expiresAt time.Time, err error) {
	raw := make([]byte, sessionTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, fmt.Errorf("セッションIDの生成: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(raw)

	// 期限の計算はSQLite側で行う。Go とSQLで時刻の形式や時差がずれると、
	// 「作った瞬間に期限切れ」のような再現しにくい不具合になる。
	const q = `
INSERT INTO sessions (id, user_id, expires_at)
VALUES (?, ?, datetime('now', ?))
RETURNING expires_at`

	// SessionDuration を SQLite の修飾子に変える。時間単位で渡す。
	modifier := fmt.Sprintf("+%d hours", int(SessionDuration.Hours()))

	var expiresAtText string
	err = s.sqldb.QueryRowContext(ctx, q, s.storageID(token), userID, modifier).Scan(&expiresAtText)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("セッションの作成: %w", err)
	}

	expiresAt, err = time.Parse(sqliteTimeLayout, expiresAtText)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("有効期限の解釈: %w", err)
	}

	// 期限切れを掃除する。1年残るテーブルなので、放置すると
	// 卒業した人の行が延々と積み上がる。ログインは頻繁でないため、
	// ここでやっても負荷にならない。
	if _, err := s.sqldb.ExecContext(ctx,
		`DELETE FROM sessions WHERE expires_at <= datetime('now')`); err != nil {
		// 掃除に失敗してもログインは成立している。止める理由がない。
		return token, expiresAt.UTC(), nil
	}

	return token, expiresAt.UTC(), nil
}

// Lookup はセッションから利用者を引く。
//
// 期限切れ・無効化された利用者は ErrSessionInvalid にする。
func (s *SessionStore) Lookup(ctx context.Context, token string) (*User, error) {
	if token == "" {
		return nil, ErrSessionInvalid
	}

	// 期限と is_active はSQL側で判定する。取得してからGoで弾く形にすると、
	// 判定を書き忘れた経路が1つでもあれば通ってしまう。
	const q = `
SELECT u.id, u.name, u.login_id, u.password_hash, u.must_change_password,
       u.email, u.role, u.is_active, u.created_at, u.updated_at,
       s.last_seen_at
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.id = ?
  AND s.expires_at > datetime('now')
  AND u.is_active = 1`

	storageID := s.storageID(token)

	var (
		u                  User
		email              sql.NullString
		lastSeenAt         sql.NullString
		mustChangePassword int
		isActive           int
		role               string
	)

	err := s.sqldb.QueryRowContext(ctx, q, storageID).Scan(
		&u.ID, &u.Name, &u.LoginID, &u.passwordHash, &mustChangePassword,
		&email, &role, &isActive, &u.CreatedAt, &u.UpdatedAt, &lastSeenAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSessionInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("セッションの取得: %w", err)
	}

	u.Email = email.String
	u.MustChangePassword = mustChangePassword != 0
	u.IsActive = isActive != 0
	u.Role = Role(role)

	s.touch(ctx, storageID, lastSeenAt)

	return &u, nil
}

// touch は last_seen_at を必要な時だけ更新する。
//
// 失敗しても認証は成立しているので、エラーは返さない。
// 最終アクセス時刻は運用の参考であって、判定には使わない。
func (s *SessionStore) touch(ctx context.Context, storageID string, lastSeenAt sql.NullString) {
	if lastSeenAt.Valid {
		t, err := time.Parse(sqliteTimeLayout, lastSeenAt.String)
		if err == nil && time.Since(t.UTC()) < lastSeenInterval {
			return
		}
	}

	_, _ = s.sqldb.ExecContext(ctx,
		`UPDATE sessions SET last_seen_at = datetime('now') WHERE id = ?`, storageID)
}

// Delete はセッションを消す。ログアウトで使う。
//
// 無いセッションを消してもエラーにしない。既に消えているなら目的は達している。
func (s *SessionStore) Delete(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}

	_, err := s.sqldb.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, s.storageID(token))
	if err != nil {
		return fmt.Errorf("セッションの削除: %w", err)
	}
	return nil
}

// DeleteByUser は利用者の全セッションを消す。
//
// パスワード変更時に使う。変更したのに、盗まれた端末のセッションが
// 1年間生き続けるのでは変更した意味がない。
func (s *SessionStore) DeleteByUser(ctx context.Context, userID int64) error {
	_, err := s.sqldb.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	if err != nil {
		return fmt.Errorf("セッションの削除: %w", err)
	}
	return nil
}
