package loan

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Rengemaru/equipment-management/internal/db"
	"github.com/Rengemaru/equipment-management/internal/jst"
)

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

// insertUser は利用者を1人作り、そのIDを返す。
func insertUser(t *testing.T, s *Store, name string, active bool) int64 {
	t.Helper()

	res, err := s.sqldb.Exec(
		`INSERT INTO users (name, login_id, password_hash, is_active) VALUES (?, ?, 'x', ?)`,
		name, name, boolToInt(active),
	)
	if err != nil {
		t.Fatalf("insertUser(%s): %v", name, err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("insertUser(%s): %v", name, err)
	}
	return id
}

// insertItem は備品を1件作る。columns で列を上書きできる。
func insertItem(t *testing.T, s *Store, code, name string, columns map[string]any) {
	t.Helper()

	q := `INSERT INTO items (code, name`
	values := `) VALUES (?, ?`
	args := []any{code, name}

	for col, v := range columns {
		q += ", " + col
		values += ", ?"
		args = append(args, v)
	}

	if _, err := s.sqldb.Exec(q+values+")", args...); err != nil {
		t.Fatalf("insertItem(%s): %v", code, err)
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// freezeNow は現在時刻を固定する。テストの終わりに戻す。
//
// 実時刻のまま「未来の借用日時」を組み立てると、JSTの日付境界を跨いだ時だけ
// 落ちるテストになる。
func freezeNow(t *testing.T, at time.Time) {
	t.Helper()

	prev := now
	now = func() time.Time { return at }
	t.Cleanup(func() { now = prev })
}

// fixture は利用者1人と備品1件を用意する。
func fixture(t *testing.T) (*Store, int64) {
	t.Helper()

	s := newTestStore(t)
	userID := insertUser(t, s, "山田", true)
	insertItem(t, s, "0001", "三脚（大）", nil)

	return s, userID
}

func TestBorrow_空の入力で自分の借用が成立する(t *testing.T) {
	s, userID := fixture(t)
	freezeNow(t, time.Date(2026, 9, 10, 19, 30, 0, 0, jst.Zone))

	l, err := s.Borrow(context.Background(), "0001", userID, Request{})
	if err != nil {
		t.Fatalf("Borrow: %v", err)
	}

	if l.User.ID != userID {
		t.Errorf("借用者 = %d, want %d", l.User.ID, userID)
	}
	// 登録者は本人でも入れる。NULL可にすると代理かどうかの判定が2通りになる。
	if l.RegisteredBy.ID != userID {
		t.Errorf("登録者 = %d, want %d", l.RegisteredBy.ID, userID)
	}
	if l.IsProxy() {
		t.Error("本人の借用が代理登録として扱われている")
	}
	if l.ReturnedAt != nil {
		t.Error("登録直後に返却済みになっている")
	}

	if l.DueDate != "2026-09-24" {
		t.Errorf("返却予定日 = %q, want 2026-09-24（借用日+%d日）", l.DueDate, defaultLoanDays)
	}
}

func TestBorrow_自由利用品は拒否する(t *testing.T) {
	s := newTestStore(t)
	userID := insertUser(t, s, "山田", true)
	insertItem(t, s, "0001", "はさみ", map[string]any{"is_free_use": 1})

	_, err := s.Borrow(context.Background(), "0001", userID, Request{})
	if !errors.Is(err, ErrFreeUse) {
		t.Fatalf("err = %v, want ErrFreeUse", err)
	}
}

func TestBorrow_廃棄済みは拒否する(t *testing.T) {
	s := newTestStore(t)
	userID := insertUser(t, s, "山田", true)
	insertItem(t, s, "0001", "壊れた三脚", map[string]any{"condition": "廃棄"})

	_, err := s.Borrow(context.Background(), "0001", userID, Request{})
	if !errors.Is(err, ErrDiscarded) {
		t.Fatalf("err = %v, want ErrDiscarded", err)
	}
}

func TestBorrow_二重貸出を拒否する(t *testing.T) {
	s, userID := fixture(t)
	other := insertUser(t, s, "佐藤", true)
	ctx := context.Background()

	if _, err := s.Borrow(ctx, "0001", userID, Request{}); err != nil {
		t.Fatalf("1回目の Borrow: %v", err)
	}

	_, err := s.Borrow(ctx, "0001", other, Request{})
	if !errors.Is(err, ErrAlreadyBorrowed) {
		t.Fatalf("err = %v, want ErrAlreadyBorrowed", err)
	}
}

func TestBorrow_備品が無ければ見つからない(t *testing.T) {
	s, userID := fixture(t)

	_, err := s.Borrow(context.Background(), "9999", userID, Request{})
	if !errors.Is(err, ErrItemNotFound) {
		t.Fatalf("err = %v, want ErrItemNotFound", err)
	}
}

func TestBorrow_代理登録は借用者と登録者を分けて記録する(t *testing.T) {
	s, registrar := fixture(t)
	borrower := insertUser(t, s, "佐藤", true)

	l, err := s.Borrow(context.Background(), "0001", registrar, Request{UserID: borrower})
	if err != nil {
		t.Fatalf("Borrow: %v", err)
	}

	if l.User.ID != borrower {
		t.Errorf("借用者 = %d, want %d", l.User.ID, borrower)
	}
	if l.RegisteredBy.ID != registrar {
		t.Errorf("登録者 = %d, want %d", l.RegisteredBy.ID, registrar)
	}
	if !l.IsProxy() {
		t.Error("代理登録として扱われていない")
	}
	if l.User.Name != "佐藤" {
		t.Errorf("借用者名 = %q, want 佐藤", l.User.Name)
	}
}

func TestBorrow_無効な利用者を借用者にできない(t *testing.T) {
	s, registrar := fixture(t)
	// 卒業者。行は残すが借用者には選べない。
	graduated := insertUser(t, s, "卒業生", false)

	if _, err := s.Borrow(context.Background(), "0001", registrar, Request{UserID: graduated}); !errors.Is(err, ErrInvalidUser) {
		t.Fatalf("卒業者: err = %v, want ErrInvalidUser", err)
	}

	if _, err := s.Borrow(context.Background(), "0001", registrar, Request{UserID: 9999}); !errors.Is(err, ErrInvalidUser) {
		t.Fatalf("存在しないID: err = %v, want ErrInvalidUser", err)
	}
}

func TestBorrow_返却予定日が借用日より前なら拒否する(t *testing.T) {
	s, userID := fixture(t)
	freezeNow(t, time.Date(2026, 9, 10, 18, 0, 0, 0, jst.Zone))

	borrowedAt := time.Date(2026, 9, 10, 12, 0, 0, 0, jst.Zone)
	_, err := s.Borrow(context.Background(), "0001", userID, Request{
		BorrowedAt: borrowedAt,
		DueDate:    "2026-09-09",
	})
	if !errors.Is(err, ErrInvalidDueDate) {
		t.Fatalf("err = %v, want ErrInvalidDueDate", err)
	}
}

func TestBorrow_読めない返却予定日を拒否する(t *testing.T) {
	s, userID := fixture(t)

	_, err := s.Borrow(context.Background(), "0001", userID, Request{DueDate: "2026/09/24"})
	if !errors.Is(err, ErrInvalidDueDate) {
		t.Fatalf("err = %v, want ErrInvalidDueDate", err)
	}
}

func TestBorrow_未来の借用日時を拒否する(t *testing.T) {
	s, userID := fixture(t)
	current := time.Date(2026, 9, 10, 19, 30, 0, 0, jst.Zone)
	freezeNow(t, current)

	_, err := s.Borrow(context.Background(), "0001", userID, Request{
		BorrowedAt: current.Add(24 * time.Hour),
	})
	if !errors.Is(err, ErrFutureBorrowedAt) {
		t.Fatalf("err = %v, want ErrFutureBorrowedAt", err)
	}
}

func TestBorrow_事後登録は借用日を基準に返却予定日を決める(t *testing.T) {
	s, userID := fixture(t)
	freezeNow(t, time.Date(2026, 9, 10, 18, 0, 0, 0, jst.Zone))

	borrowedAt := time.Date(2026, 9, 1, 19, 30, 0, 0, jst.Zone)
	l, err := s.Borrow(context.Background(), "0001", userID, Request{BorrowedAt: borrowedAt})
	if err != nil {
		t.Fatalf("Borrow: %v", err)
	}

	// 現在時刻ではなく借用日+14日。現在を基準にすると、事後登録した人だけ期限が延びる。
	if l.DueDate != "2026-09-15" {
		t.Errorf("返却予定日 = %q, want 2026-09-15", l.DueDate)
	}
	if !l.BorrowedAt.Equal(borrowedAt) {
		t.Errorf("借用日時 = %v, want %v", l.BorrowedAt, borrowedAt)
	}
}

// JST 00:00〜09:00 はUTCでは前日。UTCの日付で計算すると1日ずれる。
func TestBorrow_JSTの未明に借りても返却予定日がずれない(t *testing.T) {
	s, userID := fixture(t)
	freezeNow(t, time.Date(2026, 9, 10, 0, 40, 0, 0, jst.Zone))

	borrowedAt := time.Date(2026, 9, 10, 0, 30, 0, 0, jst.Zone) // UTC では 9/9 15:30
	l, err := s.Borrow(context.Background(), "0001", userID, Request{BorrowedAt: borrowedAt})
	if err != nil {
		t.Fatalf("Borrow: %v", err)
	}

	if l.DueDate != "2026-09-24" {
		t.Errorf("返却予定日 = %q, want 2026-09-24（UTCで計算すると 2026-09-23 になる）", l.DueDate)
	}
}

// 借りられたということは棚に戻っていたということ。
// 見つかった事実を、借りた人の追加操作ではなく借用操作から導く。
func TestBorrow_所在不明の備品を借りると在庫に戻る(t *testing.T) {
	for _, status := range []string{"所在不明_未確認", "所在不明_確定"} {
		t.Run(status, func(t *testing.T) {
			s := newTestStore(t)
			userID := insertUser(t, s, "山田", true)
			insertItem(t, s, "0001", "三脚", map[string]any{"location_status": status})

			if _, err := s.Borrow(context.Background(), "0001", userID, Request{}); err != nil {
				t.Fatalf("Borrow: %v", err)
			}

			if got := locationStatus(t, s, "0001"); got != "在庫" {
				t.Errorf("location_status = %q, want 在庫", got)
			}
		})
	}
}

// 借用が失敗したら所在も動かさない。同じトランザクションで行うことの確認。
func TestBorrow_借用が失敗したら所在は変わらない(t *testing.T) {
	t.Run("廃棄済み", func(t *testing.T) {
		s := newTestStore(t)
		userID := insertUser(t, s, "山田", true)
		insertItem(t, s, "0001", "壊れた三脚", map[string]any{
			"condition":       "廃棄",
			"location_status": "所在不明_確定",
		})

		if _, err := s.Borrow(context.Background(), "0001", userID, Request{}); !errors.Is(err, ErrDiscarded) {
			t.Fatalf("err = %v, want ErrDiscarded", err)
		}
		if got := locationStatus(t, s, "0001"); got != "所在不明_確定" {
			t.Errorf("location_status = %q, want 所在不明_確定", got)
		}
	})

	t.Run("貸出中", func(t *testing.T) {
		s := newTestStore(t)
		userID := insertUser(t, s, "山田", true)
		other := insertUser(t, s, "佐藤", true)
		insertItem(t, s, "0001", "三脚", map[string]any{"location_status": "所在不明_未確認"})

		// 借用APIを通さずに貸出中にする。Borrow で作ると、その時点で在庫に戻ってしまう。
		if _, err := s.sqldb.Exec(
			`INSERT INTO loans (item_id, user_id, registered_by, due_date)
			 SELECT id, ?, ?, '2099-01-01' FROM items WHERE code = '0001'`,
			userID, userID,
		); err != nil {
			t.Fatalf("貸出中にする: %v", err)
		}

		if _, err := s.Borrow(context.Background(), "0001", other, Request{}); !errors.Is(err, ErrAlreadyBorrowed) {
			t.Fatalf("err = %v, want ErrAlreadyBorrowed", err)
		}
		if got := locationStatus(t, s, "0001"); got != "所在不明_未確認" {
			t.Errorf("location_status = %q, want 所在不明_未確認", got)
		}
	})
}

// 在庫のものは触らない。借りるたびに更新日時が動くと、
// 一覧の「最終更新」が貸出の履歴と区別できなくなる。
func TestBorrow_在庫のままなら更新日時を動かさない(t *testing.T) {
	s, userID := fixture(t)

	// 既定値のままだと更新の有無を1秒未満で見分けられない。過去の値を置いておく。
	const before = "2020-01-01 00:00:00"
	if _, err := s.sqldb.Exec(`UPDATE items SET updated_at = ? WHERE code = '0001'`, before); err != nil {
		t.Fatalf("updated_at の設定: %v", err)
	}

	if _, err := s.Borrow(context.Background(), "0001", userID, Request{}); err != nil {
		t.Fatalf("Borrow: %v", err)
	}

	if after := updatedAt(t, s, "0001"); after != before {
		t.Errorf("updated_at が動いた: %q -> %q", before, after)
	}
}

// locationStatus は備品の所在を読む。
func locationStatus(t *testing.T, s *Store, code string) string {
	t.Helper()

	var v string
	if err := s.sqldb.QueryRow(`SELECT location_status FROM items WHERE code = ?`, code).Scan(&v); err != nil {
		t.Fatalf("location_status(%s): %v", code, err)
	}
	return v
}

// updatedAt は備品の更新日時を読む。
func updatedAt(t *testing.T, s *Store, code string) string {
	t.Helper()

	var v string
	if err := s.sqldb.QueryRow(`SELECT updated_at FROM items WHERE code = ?`, code).Scan(&v); err != nil {
		t.Fatalf("updated_at(%s): %v", code, err)
	}
	return v
}

func TestBorrow_取り消した貸出は二重貸出にならない(t *testing.T) {
	s, userID := fixture(t)
	ctx := context.Background()

	l, err := s.Borrow(ctx, "0001", userID, Request{})
	if err != nil {
		t.Fatalf("Borrow: %v", err)
	}

	// 取り消しAPIは M2-4。ここでは部分ユニークインデックスの条件だけを確かめる。
	if _, err := s.sqldb.Exec(
		`UPDATE loans SET cancelled_at = datetime('now'), cancelled_by = ? WHERE id = ?`,
		userID, l.ID,
	); err != nil {
		t.Fatalf("取り消し: %v", err)
	}

	if _, err := s.Borrow(ctx, "0001", userID, Request{}); err != nil {
		t.Fatalf("取り消し後の Borrow: %v", err)
	}
}

func TestBorrow_返却後は再度借りられる(t *testing.T) {
	s, userID := fixture(t)
	ctx := context.Background()

	l, err := s.Borrow(ctx, "0001", userID, Request{})
	if err != nil {
		t.Fatalf("Borrow: %v", err)
	}

	// 返却APIは M2-3。ここでは部分ユニークインデックスの条件だけを確かめる。
	if _, err := s.sqldb.Exec(
		`UPDATE loans SET returned_at = datetime('now'), returned_by = ? WHERE id = ?`,
		userID, l.ID,
	); err != nil {
		t.Fatalf("返却: %v", err)
	}

	if _, err := s.Borrow(ctx, "0001", userID, Request{}); err != nil {
		t.Fatalf("返却後の Borrow: %v", err)
	}
}

func TestOverdueDays(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, jst.Zone)

	tests := []struct {
		name string
		loan Loan
		want int
	}{
		{"期限内", Loan{DueDate: "2026-09-24"}, 0},
		{"当日は超過しない", Loan{DueDate: "2026-09-10"}, 0},
		{"1日超過", Loan{DueDate: "2026-09-09"}, 1},
		{"返却済みは数えない", Loan{DueDate: "2026-09-01", ReturnedAt: &now}, 0},
		{"取り消し済みは数えない", Loan{DueDate: "2026-09-01", CancelledAt: &now}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.loan.OverdueDays(now); got != tt.want {
				t.Errorf("OverdueDays = %d, want %d", got, tt.want)
			}
		})
	}
}

// 保存はUTC。読み書きで同じ時刻に戻ることを確かめる。
func TestBorrow_借用日時をUTCで保存する(t *testing.T) {
	s, userID := fixture(t)
	freezeNow(t, time.Date(2026, 9, 10, 20, 0, 0, 0, jst.Zone))

	borrowedAt := time.Date(2026, 9, 10, 19, 30, 0, 0, jst.Zone)
	if _, err := s.Borrow(context.Background(), "0001", userID, Request{BorrowedAt: borrowedAt}); err != nil {
		t.Fatalf("Borrow: %v", err)
	}

	var stored string
	if err := s.sqldb.QueryRow(`SELECT borrowed_at FROM loans`).Scan(&stored); err != nil {
		t.Fatalf("borrowed_at: %v", err)
	}

	if stored != "2026-09-10 10:30:00" {
		t.Errorf("保存された借用日時 = %q, want 2026-09-10 10:30:00（UTC）", stored)
	}
}
