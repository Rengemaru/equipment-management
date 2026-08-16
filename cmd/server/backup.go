package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Rengemaru/equipment-management/internal/db"
)

// runBackup は稼働中のDBから一貫したコピーを1ファイル作る。
//
// # なぜバイナリの機能として持つのか
//
// DBは名前付きボリュームに置くためホストから直接見えない。さらに本番イメージは
// 最小構成で、__sqlite3 コマンドもシェルも入っていない。__ そのため
// 「コンテナ内で sqlite3 .backup を叩く」手順は成立しない（CLAUDE.md）。
//
// # なぜファイルコピーではないのか
//
// WAL モードで動いており、app.db だけを写しても直近の書き込みは -wal 側にある。
// 3ファイルを揃えて写しても、写している最中の書き込みで食い違う。
// VACUUM INTO は稼働中でも一貫した1ファイルを作り、WAL の内容も畳み込む。
func runBackup(ctx context.Context, sqldb *sql.DB, dest string, out io.Writer) error {
	if dest == "" {
		return errors.New("-backup には出力先のパスを渡す\n" +
			"  例: /server -backup /data/backup-2026-08-16.db")
	}

	abs, err := filepath.Abs(dest)
	if err != nil {
		return fmt.Errorf("出力先の解決: %w", err)
	}

	// 上書きしない。VACUUM INTO 自体も既存ファイルには書かないが、
	// その時のエラーは原因が読み取りにくい。先に見て理由を返す。
	//
	// 世代を残すのは運用側の判断にする。日付をファイル名に入れて増やせば、
	// 誤って上書きした結果しか残らない状態を避けられる。
	if _, err := os.Stat(abs); err == nil {
		return fmt.Errorf("%s は既にある。別の名前にすること（上書きはしない）", abs)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("出力先の確認: %w", err)
	}

	// VACUUM INTO は出力先のディレクトリを作らない。
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return fmt.Errorf("出力先の作成: %w", err)
	}

	// プレースホルダで渡す。パスは運用者が打つ値だが、クエリを文字列結合で
	// 組み立てない方針を経路ごとに崩さない（CLAUDE.md）。
	if _, err := sqldb.ExecContext(ctx, `VACUUM INTO ?`, abs); err != nil {
		return fmt.Errorf("バックアップの作成: %w", err)
	}

	size, err := verifyBackup(ctx, abs)
	if err != nil {
		// 作ったファイルは残す。中身を見て原因を追えるようにする。
		return fmt.Errorf("バックアップの検証: %w", err)
	}

	fmt.Fprintf(out, `バックアップを作成しました。

  出力先 : %s
  大きさ : %s
  内容   : 利用者 %d件 / 備品 %d件

コンテナの外へ取り出すには:

  docker compose cp app:%s ./%s

取り出したファイルは別の場所に保管すること。同じボリュームに置いたままでは、
ディスクごと失われた時に一緒に消える。
`, abs, humanSize(size.bytes), size.users, size.items, abs, filepath.Base(abs))

	return nil
}

// backupSummary は作ったバックアップの中身。
type backupSummary struct {
	bytes int64
	users int
	items int
}

// verifyBackup は作ったファイルを開いて中身を確かめる。
//
// 作っただけで「取れた」と報告しない。__復元できないバックアップは、__
// __無いことに気付けないぶん、無いより悪い。__ 開けること・壊れていないこと・
// 中身が入っていることをここで見る。
func verifyBackup(ctx context.Context, path string) (backupSummary, error) {
	var summary backupSummary

	info, err := os.Stat(path)
	if err != nil {
		return summary, err
	}
	summary.bytes = info.Size()

	copied, err := db.Open(ctx, path)
	if err != nil {
		return summary, err
	}
	defer func() { _ = copied.Close() }()

	var result string
	if err := copied.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&result); err != nil {
		return summary, fmt.Errorf("integrity_check: %w", err)
	}
	if result != "ok" {
		return summary, fmt.Errorf("integrity_check が ok を返さない: %s", result)
	}

	if err := copied.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&summary.users); err != nil {
		return summary, err
	}
	if err := copied.QueryRowContext(ctx, `SELECT COUNT(*) FROM items`).Scan(&summary.items); err != nil {
		return summary, err
	}

	return summary, nil
}

// humanSize はバイト数を読める形にする。
//
// 「取れているつもりで0バイト」に気付けるよう、桁を見せる。
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dバイト", n)
	}

	div, exp := int64(unit), 0
	for n/div >= unit && exp < 2 {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f%cB", float64(n)/float64(div), "KMG"[exp])
}
