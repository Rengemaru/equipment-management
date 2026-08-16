package auth

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const (
	// attemptWindow は失敗を数える期間。
	attemptWindow = 15 * time.Minute

	// freeAttempts はこの回数までは待たせない。
	//
	// パスワードを打ち間違える人を待たせない範囲にする。
	// スマートフォンからの入力で、大文字小文字や記号の打ち間違いは普通に起きる。
	freeAttempts = 5

	// maxDelay は待たせる上限。
	//
	// 無制限に伸ばすと、いたずらで他人のログインを事実上止められる。
	maxDelay = 8 * time.Second
)

// delayUnit は待ち時間の刻み。テストで短くするため変数にしている。
// 本番の経路でこの値を書き換えないこと。
var delayUnit = time.Second

// Throttle は login_attempts を使って総当たりを鈍らせる。
//
// ロックはしない。ロックすると、いたずらや打ち間違いで締め出された人が
// その場で借用を記録できなくなる。記録されなかった借用は追跡できず、
// 「記録する手間 < 記録しない手間」が崩れる。
// 一方、遅延なら正しいパスワードを知っている本人は必ず入れる。
type Throttle struct {
	sqldb *sql.DB
}

// NewThrottle は Throttle を作る。
func NewThrottle(sqldb *sql.DB) *Throttle {
	return &Throttle{sqldb: sqldb}
}

// loginDelay は失敗回数に対する待ち時間を返す。
//
// freeAttempts を超えた分だけ倍々に伸ばし、maxDelay で頭打ちにする。
// bcrypt の照合だけでも1回あたり数十ミリ秒かかるため、
// これだけでも総当たりは現実的でなくなる。
func loginDelay(failures int) time.Duration {
	over := failures - freeAttempts
	if over <= 0 {
		return 0
	}

	// 1, 2, 4, 8... 秒。指数は上限で頭打ちになるので溢れない。
	delay := delayUnit << min(over-1, 10)
	return min(delay, maxDelay*delayUnit/time.Second)
}

// RecentFailures は直近の失敗回数を返す。
func (t *Throttle) RecentFailures(ctx context.Context, loginID string) (int, error) {
	const q = `
SELECT COUNT(*) FROM login_attempts
WHERE login_id = ? AND attempted_at > datetime('now', ?)`

	modifier := fmt.Sprintf("-%d minutes", int(attemptWindow.Minutes()))

	var n int
	err := t.sqldb.QueryRowContext(ctx, q, NormalizeLoginID(loginID), modifier).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("失敗回数の取得: %w", err)
	}

	return n, nil
}

// RecordFailure は失敗を記録する。
//
// 存在しないログインIDへの試行も記録する。存在するIDだけ記録すると、
// 記録の有無そのものがIDの存在を教えることになる。
func (t *Throttle) RecordFailure(ctx context.Context, loginID string) error {
	const q = `INSERT INTO login_attempts (login_id) VALUES (?)`
	if _, err := t.sqldb.ExecContext(ctx, q, NormalizeLoginID(loginID)); err != nil {
		return fmt.Errorf("失敗の記録: %w", err)
	}

	// 期間外の記録を捨てる。放置すると、総当たりを受けた分だけ
	// 行が残り続ける。
	modifier := fmt.Sprintf("-%d minutes", int(attemptWindow.Minutes()))
	_, _ = t.sqldb.ExecContext(ctx,
		`DELETE FROM login_attempts WHERE attempted_at <= datetime('now', ?)`, modifier)

	return nil
}

// Clear は成功したログインIDの記録を消す。
//
// 消さないと、打ち間違えた後に正しく入れた人が、次のログインでも待たされる。
func (t *Throttle) Clear(ctx context.Context, loginID string) error {
	const q = `DELETE FROM login_attempts WHERE login_id = ?`
	if _, err := t.sqldb.ExecContext(ctx, q, NormalizeLoginID(loginID)); err != nil {
		return fmt.Errorf("失敗記録の削除: %w", err)
	}
	return nil
}

// Wait は失敗回数に応じて待つ。ctx が切れたら中断する。
func (t *Throttle) Wait(ctx context.Context, failures int) {
	delay := loginDelay(failures)
	if delay <= 0 {
		return
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-timer.C:
	case <-ctx.Done():
		// 利用者が待たずに切った場合。待つ理由がなくなる。
	}
}
