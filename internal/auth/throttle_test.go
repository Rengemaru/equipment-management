package auth

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestLoginDelay_一定回数までは待たせない(t *testing.T) {
	for failures := 0; failures <= freeAttempts; failures++ {
		if d := loginDelay(failures); d != 0 {
			t.Errorf("%d 回目で %v 待たされる。打ち間違いを待たせない", failures, d)
		}
	}
}

func TestLoginDelay_超えたら伸びて頭打ちになる(t *testing.T) {
	prev := time.Duration(0)

	for failures := freeAttempts + 1; failures <= freeAttempts+20; failures++ {
		d := loginDelay(failures)

		if d <= 0 {
			t.Fatalf("%d 回目で待ち時間が 0", failures)
		}
		if d < prev {
			t.Errorf("%d 回目で待ち時間が縮んだ: %v → %v", failures, prev, d)
		}
		// 無制限に伸ばすと、いたずらで他人のログインを事実上止められる。
		if d > maxDelay {
			t.Errorf("%d 回目の待ち時間 %v が上限 %v を超えている", failures, d, maxDelay)
		}
		prev = d
	}
}

func TestThrottle_失敗を数えて成功で消える(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	throttle := NewThrottle(store.sqldb)

	for i := 1; i <= 3; i++ {
		if err := throttle.RecordFailure(ctx, "yamada"); err != nil {
			t.Fatalf("RecordFailure: %v", err)
		}

		n, err := throttle.RecentFailures(ctx, "yamada")
		if err != nil {
			t.Fatalf("RecentFailures: %v", err)
		}
		if n != i {
			t.Errorf("失敗回数 = %d。%d を期待", n, i)
		}
	}

	if err := throttle.Clear(ctx, "yamada"); err != nil {
		t.Fatalf("Clear: %v", err)
	}

	n, err := throttle.RecentFailures(ctx, "yamada")
	if err != nil {
		t.Fatalf("RecentFailures: %v", err)
	}
	if n != 0 {
		t.Errorf("成功後も %d 件残っている", n)
	}
}

// 別人の失敗で待たされないこと。
func TestThrottle_ログインIDごとに数える(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	throttle := NewThrottle(store.sqldb)

	for i := 0; i < 5; i++ {
		if err := throttle.RecordFailure(ctx, "yamada"); err != nil {
			t.Fatalf("RecordFailure: %v", err)
		}
	}

	n, err := throttle.RecentFailures(ctx, "tanaka")
	if err != nil {
		t.Fatalf("RecentFailures: %v", err)
	}
	if n != 0 {
		t.Errorf("別人の失敗が %d 件数えられている", n)
	}
}

// 大文字小文字を揃えて数える。'Yamada' で試して回避できてはいけない。
func TestThrottle_ログインIDを正規化して数える(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	throttle := NewThrottle(store.sqldb)

	if err := throttle.RecordFailure(ctx, "YAMADA"); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}

	n, err := throttle.RecentFailures(ctx, "yamada")
	if err != nil {
		t.Fatalf("RecentFailures: %v", err)
	}
	if n != 1 {
		t.Errorf("失敗回数 = %d。1 を期待（大文字で回避できている）", n)
	}
}

// 期間を過ぎた失敗は数えない。時間が経てば自然に元へ戻る。
func TestThrottle_古い失敗は数えない(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	throttle := NewThrottle(store.sqldb)

	if err := throttle.RecordFailure(ctx, "yamada"); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}

	const q = `UPDATE login_attempts SET attempted_at = datetime('now', '-1 day')`
	if _, err := store.sqldb.ExecContext(ctx, q); err != nil {
		t.Fatalf("時刻の変更: %v", err)
	}

	n, err := throttle.RecentFailures(ctx, "yamada")
	if err != nil {
		t.Fatalf("RecentFailures: %v", err)
	}
	if n != 0 {
		t.Errorf("古い失敗が %d 件数えられている", n)
	}
}

// 何度失敗しても、正しいパスワードなら必ず入れること。
// ここが崩れると、締め出された人がその場で借用を記録できなくなる。
func TestHandleLogin_何度失敗してもロックアウトしない(t *testing.T) {
	h, _ := newTestHandler(t)

	for i := 0; i < freeAttempts+5; i++ {
		w := postLogin(t, h, `{"login_id":"yamada","password":"wrongpassword"}`)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%d 回目: status = %d。401 を期待", i+1, w.Code)
		}
	}

	w := postLogin(t, h, `{"login_id":"yamada","password":"password123"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("正しいパスワードで入れない: status = %d, body = %s", w.Code, w.Body.String())
	}
}

// 待たされていることを応答から読み取れないこと。
// 何回で待たされるかが分かると、その手前で止める攻撃ができる。
func TestHandleLogin_待たされたことを応答に出さない(t *testing.T) {
	h, _ := newTestHandler(t)

	first := postLogin(t, h, `{"login_id":"yamada","password":"wrongpassword"}`)

	for i := 0; i < freeAttempts+3; i++ {
		postLogin(t, h, `{"login_id":"yamada","password":"wrongpassword"}`)
	}

	last := postLogin(t, h, `{"login_id":"yamada","password":"wrongpassword"}`)

	if last.Code != first.Code {
		t.Errorf("status が変わった: %d → %d", first.Code, last.Code)
	}
	if last.Body.String() != first.Body.String() {
		t.Errorf("本文が変わった:\n  %s\n  %s", first.Body.String(), last.Body.String())
	}
	if last.Header().Get("Retry-After") != "" {
		t.Error("Retry-After が付いている。待たされていることが分かる")
	}
}

// ログインに成功すると記録が消え、次から待たされないこと。
func TestHandleLogin_成功すると失敗の記録が消える(t *testing.T) {
	h, store := newTestHandler(t)
	ctx := context.Background()
	throttle := NewThrottle(store.sqldb)

	for i := 0; i < 3; i++ {
		postLogin(t, h, `{"login_id":"yamada","password":"wrongpassword"}`)
	}

	if n, _ := throttle.RecentFailures(ctx, "yamada"); n != 3 {
		t.Fatalf("失敗が記録されていない: %d 件", n)
	}

	postLogin(t, h, `{"login_id":"yamada","password":"password123"}`)

	n, err := throttle.RecentFailures(ctx, "yamada")
	if err != nil {
		t.Fatalf("RecentFailures: %v", err)
	}
	if n != 0 {
		t.Errorf("成功後も %d 件残っている", n)
	}
}

// 存在しないIDへの試行も記録すること。
// 存在するIDだけ記録すると、記録の有無がIDの存在を教えることになる。
func TestHandleLogin_存在しないIDの試行も記録する(t *testing.T) {
	h, store := newTestHandler(t)
	throttle := NewThrottle(store.sqldb)

	postLogin(t, h, `{"login_id":"nobody","password":"wrongpassword"}`)

	n, err := throttle.RecentFailures(context.Background(), "nobody")
	if err != nil {
		t.Fatalf("RecentFailures: %v", err)
	}
	if n != 1 {
		t.Errorf("失敗回数 = %d。1 を期待", n)
	}
}
