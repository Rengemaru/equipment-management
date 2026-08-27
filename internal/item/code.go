package item

import (
	"context"
	"database/sql"
	"fmt"
)

// codeDigits は備品コードの桁数。
//
// 4桁。数十名のサークルで9999点を超えることは想定していないが、
// 超えた場合は5桁になるだけで破綻しない（後述）。
const codeDigits = 4

// Queryer は *sql.DB と *sql.Tx のどちらでも受けるための最小の口。
type Queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// NextCode は次の備品コードを返す。'0001' 形式。
//
// 人手で振らせない。抜けと重複が必ず起きるうえ、一度貼ったラベルは
// 貼り替えられないため、後から直せない（CLAUDE.md）。
//
// # 空きを埋めない
//
// 0001 と 0005 がある時に返すのは 0006 で、0002 ではない。
// 空きを埋めると、廃棄した備品のコードが別の備品に付く。
// 古い貸出履歴を見た人が、今そのコードを持つ別の備品と取り違える。
// items を物理削除しない方針と合わせて、コードは一度使ったら二度と使わない。
//
// # トランザクションの中で呼ぶこと
//
// 採番と INSERT が別のトランザクションだと、同時に登録した2件が
// 同じ番号を取る。DSN で _txlock=immediate を指定しているため、
// 同じトランザクション内で呼べば書き込みは直列化される。
// UNIQUE 制約も張ってあるので、取りこぼしても重複はDBが弾く。
func NextCode(ctx context.Context, q Queryer) (string, error) {
	n, err := nextCodeNumber(ctx, q)
	if err != nil {
		return "", err
	}

	return FormatCode(n), nil
}

// nextCodeNumber は次に採番される番号を、書式にする前の数値で返す。
//
// CSVインポートのプレビューが「どこからどこまでのコードになるか」を示すのに、
// 連番を足せる形が要る。'0001' から数値に戻す処理を呼び出し側に書かせない。
func nextCodeNumber(ctx context.Context, q Queryer) (int64, error) {
	// CAST を使う。文字列比較だと桁が増えた時に '9999' > '10000' になる。
	// 数値にならないコードは 0 として扱われ、最大値に影響しない。
	const query = `SELECT IFNULL(MAX(CAST(code AS INTEGER)), 0) FROM items`

	var max int64
	if err := q.QueryRowContext(ctx, query).Scan(&max); err != nil {
		return 0, fmt.Errorf("備品コードの採番: %w", err)
	}

	return max + 1, nil
}

// FormatCode は数値を備品コードの形にする。
//
// 9999 を超えたら5桁になる。頭を切って 0000 に戻すと、
// 既存のコードと衝突して過去の履歴が壊れる。桁が増える方を選ぶ。
func FormatCode(n int64) string {
	return fmt.Sprintf("%0*d", codeDigits, n)
}
