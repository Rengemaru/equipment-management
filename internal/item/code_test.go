package item

import (
	"context"
	"sync"
	"testing"
)

func TestNextCode_最初は0001(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	got, err := NextCode(ctx, s.sqldb)
	if err != nil {
		t.Fatalf("NextCode: %v", err)
	}
	if got != "0001" {
		t.Errorf("NextCode = %q。0001 を期待", got)
	}
}

func TestNextCode_最大値に1を足す(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	insert(t, s, "0001", "三脚", nil)
	insert(t, s, "0002", "カメラ", nil)

	got, err := NextCode(ctx, s.sqldb)
	if err != nil {
		t.Fatalf("NextCode: %v", err)
	}
	if got != "0003" {
		t.Errorf("NextCode = %q。0003 を期待", got)
	}
}

// 空きを埋めない。埋めると、廃棄した備品のコードが別の備品に付き、
// 古い貸出履歴を見た人が取り違える。
func TestNextCode_空き番号を再利用しない(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	insert(t, s, "0001", "三脚", nil)
	insert(t, s, "0005", "カメラ", nil)

	got, err := NextCode(ctx, s.sqldb)
	if err != nil {
		t.Fatalf("NextCode: %v", err)
	}
	if got != "0006" {
		t.Errorf("NextCode = %q。0006 を期待（0002 を埋めてはいけない）", got)
	}
}

// 廃棄した備品のコードも空かない。物理削除しないため行は残る。
func TestNextCode_廃棄済みのコードも再利用しない(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	insert(t, s, "0001", "三脚", nil)
	insert(t, s, "0002", "壊れたカメラ", map[string]any{"condition": string(ConditionDiscarded)})

	got, err := NextCode(ctx, s.sqldb)
	if err != nil {
		t.Fatalf("NextCode: %v", err)
	}
	if got != "0003" {
		t.Errorf("NextCode = %q。0003 を期待", got)
	}
}

// 桁が増えても壊れないこと。文字列比較だと '9999' > '10000' になる。
func TestNextCode_9999の次は10000(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	insert(t, s, "0001", "三脚", nil)
	insert(t, s, "9999", "最後の備品", nil)

	got, err := NextCode(ctx, s.sqldb)
	if err != nil {
		t.Fatalf("NextCode: %v", err)
	}
	// 頭を切って 0000 に戻すと、既存のコードと衝突して履歴が壊れる。
	if got != "10000" {
		t.Errorf("NextCode = %q。10000 を期待", got)
	}

	insert(t, s, got, "その次", nil)

	got, err = NextCode(ctx, s.sqldb)
	if err != nil {
		t.Fatalf("NextCode: %v", err)
	}
	if got != "10001" {
		t.Errorf("NextCode = %q。10001 を期待", got)
	}
}

// CSVインポートなどで数値でないコードが混ざっても、採番が止まらないこと。
func TestNextCode_数値でないコードを無視する(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	insert(t, s, "0003", "三脚", nil)
	insert(t, s, "旧-A-12", "古い台帳の備品", nil)

	got, err := NextCode(ctx, s.sqldb)
	if err != nil {
		t.Fatalf("NextCode: %v", err)
	}
	if got != "0004" {
		t.Errorf("NextCode = %q。0004 を期待", got)
	}
}

func TestFormatCode_4桁でゼロ詰めする(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{1, "0001"},
		{42, "0042"},
		{999, "0999"},
		{1000, "1000"},
		{9999, "9999"},
		{10000, "10000"},
	}

	for _, tt := range tests {
		if got := FormatCode(tt.n); got != tt.want {
			t.Errorf("FormatCode(%d) = %q。%q を期待", tt.n, got, tt.want)
		}
	}
}

// 同時に登録しても番号が衝突しないこと。
// 採番と INSERT を同じトランザクションに入れる前提を、ここで固定する。
func TestNextCode_同時に登録しても衝突しない(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	const n = 20

	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		codes []string
		errs  []error
	)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// 採番と INSERT を1つのトランザクションで行う。
			// これが本来の使い方で、書き込みAPIもこの形になる。
			tx, err := s.sqldb.BeginTx(ctx, nil)
			if err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
				return
			}
			defer func() { _ = tx.Rollback() }()

			code, err := NextCode(ctx, tx)
			if err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
				return
			}

			if _, err := tx.ExecContext(ctx,
				`INSERT INTO items (code, name) VALUES (?, ?)`, code, "同時登録"); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
				return
			}
			if err := tx.Commit(); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
				return
			}

			mu.Lock()
			codes = append(codes, code)
			mu.Unlock()
		}()
	}

	wg.Wait()

	if len(errs) > 0 {
		t.Fatalf("同時登録が失敗した（%d件）: %v", len(errs), errs[0])
	}
	if len(codes) != n {
		t.Fatalf("登録できたのは %d件。%d件を期待", len(codes), n)
	}

	seen := make(map[string]bool, len(codes))
	for _, c := range codes {
		if seen[c] {
			t.Fatalf("同じ備品コードが2件に振られた: %s", c)
		}
		seen[c] = true
	}
}
