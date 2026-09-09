package report

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Rengemaru/equipment-management/internal/db"
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

func insertUser(t *testing.T, s *Store, name string) int64 {
	t.Helper()

	res, err := s.sqldb.Exec(
		`INSERT INTO users (name, login_id, password_hash) VALUES (?, ?, 'x')`, name, name)
	if err != nil {
		t.Fatalf("insertUser(%s): %v", name, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("insertUser(%s): %v", name, err)
	}
	return id
}

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

// condition は備品の状態を読む。
func condition(t *testing.T, s *Store, code string) string {
	t.Helper()

	var v string
	if err := s.sqldb.QueryRow(`SELECT condition FROM items WHERE code = ?`, code).Scan(&v); err != nil {
		t.Fatalf("condition(%s): %v", code, err)
	}
	return v
}

// fixture は利用者1人と備品1件を用意する。
func fixture(t *testing.T) (*Store, int64) {
	t.Helper()

	s := newTestStore(t)
	userID := insertUser(t, s, "山田")
	insertItem(t, s, "0001", "三脚（大）", nil)

	return s, userID
}

// 承認を待たない。押した結果が画面に出ないボタンは効かないボタンでしかない。
func TestReport_報告と同時に要修理にする(t *testing.T) {
	s, userID := fixture(t)

	d, err := s.Report(context.Background(), "0001", userID, "脚のロックが割れている")
	if err != nil {
		t.Fatalf("Report: %v", err)
	}

	if d.Status != StatusUnconfirmed {
		t.Errorf("status = %q, want 未確認", d.Status)
	}
	if d.Reporter.ID != userID {
		t.Errorf("報告者 = %d, want %d", d.Reporter.ID, userID)
	}
	if d.Description != "脚のロックが割れている" {
		t.Errorf("説明 = %q", d.Description)
	}
	if got := condition(t, s, "0001"); got != "要修理" {
		t.Errorf("condition = %q, want 要修理（追認を待たない）", got)
	}
}

func TestReport_説明が空なら拒否する(t *testing.T) {
	s, userID := fixture(t)

	if _, err := s.Report(context.Background(), "0001", userID, "   "); !errors.Is(err, ErrEmptyDescription) {
		t.Fatalf("err = %v, want ErrEmptyDescription", err)
	}
}

func TestReport_備品が無ければ見つからない(t *testing.T) {
	s, userID := fixture(t)

	if _, err := s.Report(context.Background(), "9999", userID, "壊れた"); !errors.Is(err, ErrItemNotFound) {
		t.Fatalf("err = %v, want ErrItemNotFound", err)
	}
}

// 誰が借りている間に壊れたのかを後から辿れるようにする。
func TestReport_貸出中なら貸出に紐付ける(t *testing.T) {
	s, userID := fixture(t)

	// 貸出は loan パッケージの担当。ここでは行だけ作る。
	if _, err := s.sqldb.Exec(
		`INSERT INTO loans (item_id, user_id, registered_by, due_date)
		 SELECT id, ?, ?, '2099-01-01' FROM items WHERE code = '0001'`, userID, userID,
	); err != nil {
		t.Fatalf("貸出中にする: %v", err)
	}

	d, err := s.Report(context.Background(), "0001", userID, "脚が曲がった")
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if d.LoanID == nil {
		t.Fatal("loan_id が入っていない")
	}
}

func TestReport_在庫中なら貸出に紐付けない(t *testing.T) {
	s, userID := fixture(t)

	d, err := s.Report(context.Background(), "0001", userID, "傷がある")
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if d.LoanID != nil {
		t.Errorf("loan_id = %v, want nil", *d.LoanID)
	}
}

// 返却済みの貸出には紐付けない。壊れた時点で借りていた人ではない。
func TestReport_返却済みの貸出には紐付けない(t *testing.T) {
	s, userID := fixture(t)

	if _, err := s.sqldb.Exec(
		`INSERT INTO loans (item_id, user_id, registered_by, due_date, returned_at, returned_by)
		 SELECT id, ?, ?, '2099-01-01', datetime('now'), ? FROM items WHERE code = '0001'`,
		userID, userID, userID,
	); err != nil {
		t.Fatalf("貸出を作る: %v", err)
	}

	d, err := s.Report(context.Background(), "0001", userID, "傷がある")
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if d.LoanID != nil {
		t.Errorf("loan_id = %v, want nil", *d.LoanID)
	}
}

// 廃棄は終端状態。報告で復活すると、廃棄したはずのものが一覧に現れる。
func TestReport_廃棄済みは要修理に戻さない(t *testing.T) {
	s := newTestStore(t)
	userID := insertUser(t, s, "山田")
	insertItem(t, s, "0001", "壊れた三脚", map[string]any{"condition": "廃棄"})

	if _, err := s.Report(context.Background(), "0001", userID, "さらに壊れた"); err != nil {
		t.Fatalf("Report: %v", err)
	}

	if got := condition(t, s, "0001"); got != "廃棄" {
		t.Errorf("condition = %q, want 廃棄", got)
	}
}

func TestSetStatus_追認で備品の状態が連動する(t *testing.T) {
	tests := []struct {
		status Status
		want   string
	}{
		{StatusConfirmed, "要修理"}, // 変えない
		{StatusRepaired, "良好"},
		{StatusDiscarded, "廃棄"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			s, userID := fixture(t)
			admin := insertUser(t, s, "運営")
			ctx := context.Background()

			d, err := s.Report(ctx, "0001", userID, "脚が曲がった")
			if err != nil {
				t.Fatalf("Report: %v", err)
			}

			got, err := s.SetStatus(ctx, d.ID, admin, tt.status, "現物を確認した")
			if err != nil {
				t.Fatalf("SetStatus: %v", err)
			}

			if got.Status != tt.status {
				t.Errorf("status = %q, want %q", got.Status, tt.status)
			}
			if got.ConfirmedBy == nil || got.ConfirmedBy.ID != admin {
				t.Errorf("confirmed_by = %v, want %d", got.ConfirmedBy, admin)
			}
			if got.ConfirmedAt == nil {
				t.Error("confirmed_at が入っていない")
			}
			if c := condition(t, s, "0001"); c != tt.want {
				t.Errorf("condition = %q, want %q", c, tt.want)
			}
		})
	}
}

func TestSetStatus_知らない状態を拒否する(t *testing.T) {
	s, userID := fixture(t)
	ctx := context.Background()

	d, err := s.Report(ctx, "0001", userID, "壊れた")
	if err != nil {
		t.Fatalf("Report: %v", err)
	}

	if _, err := s.SetStatus(ctx, d.ID, userID, Status("修理中"), ""); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("err = %v, want ErrInvalidStatus", err)
	}
}

func TestSetStatus_知らない報告は見つからない(t *testing.T) {
	s, userID := fixture(t)

	if _, err := s.SetStatus(context.Background(), 999, userID, StatusConfirmed, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestList_状態で絞れる(t *testing.T) {
	s, userID := fixture(t)
	insertItem(t, s, "0002", "ドライバー", nil)
	ctx := context.Background()

	first, err := s.Report(ctx, "0001", userID, "脚が曲がった")
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if _, err := s.Report(ctx, "0002", userID, "先が欠けた"); err != nil {
		t.Fatalf("Report: %v", err)
	}
	if _, err := s.SetStatus(ctx, first.ID, userID, StatusRepaired, ""); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}

	all, err := s.List(ctx, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("全件 = %d, want 2", len(all))
	}

	unconfirmed, err := s.List(ctx, StatusUnconfirmed)
	if err != nil {
		t.Fatalf("List(未確認): %v", err)
	}
	if len(unconfirmed) != 1 {
		t.Fatalf("未確認 = %d, want 1", len(unconfirmed))
	}
	if unconfirmed[0].ItemCode != "0002" {
		t.Errorf("未確認の備品 = %q, want 0002", unconfirmed[0].ItemCode)
	}
}

func TestList_知らない状態を拒否する(t *testing.T) {
	s, _ := fixture(t)

	if _, err := s.List(context.Background(), Status("修理中")); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("err = %v, want ErrInvalidStatus", err)
	}
}
