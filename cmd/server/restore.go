package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Rengemaru/equipment-management/internal/db"
)

// runRestore は -backup で作ったファイルから DB を戻す。
//
// # なぜバイナリの機能として持つのか
//
// -backup と同じ理由。DBは名前付きボリュームにあってホストから直接見えず、
// 本番イメージには sqlite3 もシェルも入っていない。
// 「コンテナ内で cp して chown する」手順は成立しない（CLAUDE.md）。
//
// __復元できないバックアップは、無いことに気付けないぶん無いより悪い。__
// 取る側だけを作って戻す側を手順書に逃がすと、実際に必要になった日に
// 「手順どおりにやったのに動かない」ところから始めることになる。
//
// # サーバを止めてから実行すること
//
// 動いているサーバの足元でファイルを差し替えると壊れる。
// 停止中のサービスに対して使い捨てのコンテナで実行する:
//
//	docker compose stop
//	docker compose cp ./backup-2026-08-27.db app:/data/restore.db
//	docker compose run --rm app -restore /data/restore.db
//	docker compose start
func runRestore(ctx context.Context, src, dest string, out io.Writer) error {
	if src == "" {
		return errors.New("-restore には戻す元のパスを渡す\n" +
			"  例: /server -restore /data/restore.db")
	}

	srcAbs, err := filepath.Abs(src)
	if err != nil {
		return fmt.Errorf("戻す元の解決: %w", err)
	}
	destAbs, err := filepath.Abs(dest)
	if err != nil {
		return fmt.Errorf("戻す先の解決: %w", err)
	}
	if srcAbs == destAbs {
		return fmt.Errorf("戻す元と戻す先が同じ: %s", srcAbs)
	}

	// __先に中身を確かめる。__ 壊れたファイルで上書きしてから気付くと、
	// 戻す元も戻す先も失う。
	before, err := verifyBackup(ctx, srcAbs)
	if err != nil {
		return fmt.Errorf("戻す元が使えない（%s）: %w", srcAbs, err)
	}

	fmt.Fprintf(out, `戻す元を確認しました。

  ファイル : %s
  大きさ   : %s
  内容     : 利用者 %d件 / 備品 %d件

`, srcAbs, humanSize(before.bytes), before.users, before.items)

	// WAL と SHM を残したまま本体だけ差し替えない。中身の合わない WAL が
	// 残ると、次に開いた時にそれを被せようとする。3つまとめて消す。
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(destAbs + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("戻す先の削除（%s）: %w", destAbs+suffix, err)
		}
	}

	if err := copyFile(srcAbs, destAbs); err != nil {
		return fmt.Errorf("戻す先への書き出し: %w", err)
	}

	// 古いバックアップから戻すと、スキーマが今より後ろにいることがある。
	// ここで追いつかせる。起動時にも適用されるが、戻した直後に
	// 「開けて中身も揃っている」ことまで見てから終わりたい。
	restored, err := db.Open(ctx, destAbs)
	if err != nil {
		return fmt.Errorf("戻した後の接続: %w", err)
	}
	defer func() { _ = restored.Close() }()

	if err := db.Migrate(ctx, restored, db.Migrations()); err != nil {
		return fmt.Errorf("戻した後のマイグレーション: %w", err)
	}

	after, err := verifyBackup(ctx, destAbs)
	if err != nil {
		return fmt.Errorf("戻した後の検証: %w", err)
	}
	if after.users != before.users || after.items != before.items {
		return fmt.Errorf("件数が合わない（元: 利用者%d/備品%d, 先: 利用者%d/備品%d）",
			before.users, before.items, after.users, after.items)
	}

	fmt.Fprintf(out, `復元しました。

  戻す先 : %s
  内容   : 利用者 %d件 / 備品 %d件

サーバを起動し直すこと:

  docker compose start
`, destAbs, after.users, after.items)

	return nil
}

// copyFile は中身をそのまま写す。
//
// os.Rename ではない。戻す元をそのまま残す。__1回で決まらなかった時に、__
// __もう一度やり直せる状態を保つ。__
func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	// 0600。DBには個人情報と貸出履歴が入る。同じマシンの他の利用者に
	// 読ませる理由がない。
	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}

	if _, err := io.Copy(f, in); err != nil {
		_ = f.Close()
		// 中途半端なファイルを残さない。次の起動が空でないDBとして開いて
		// しまい、原因が読めなくなる。
		_ = os.Remove(dest)
		return err
	}

	return f.Close()
}
