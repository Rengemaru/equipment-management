package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// 接続プールの上限。数十名規模なので大きくする理由がない。
// 上限を設けないと、リクエストが詰まった時に接続だけが際限なく増える。
const maxOpenConns = 8

// Open は SQLite を開き、疎通を確認した *sql.DB を返す。
//
// スキーマの適用は行わない。呼び出し側で Migrate を呼ぶこと。
// 接続とマイグレーションを1つの関数にまとめると、テストで
// 「スキーマ無しの接続」を作れなくなる。
func Open(ctx context.Context, path string) (*sql.DB, error) {
	if path == "" {
		return nil, fmt.Errorf("DBのパスが空")
	}

	sqldb, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, fmt.Errorf("DB %s のオープン: %w", path, err)
	}

	sqldb.SetMaxOpenConns(maxOpenConns)
	// アイドル上限を同じにする。接続を切ると次に開いた時に PRAGMA を
	// 適用し直すことになり、無駄な上に設定漏れの余地を作る。
	sqldb.SetMaxIdleConns(maxOpenConns)

	// sql.Open は実際には接続しない。ここで初めてファイルの作成・破損が分かる。
	if err := sqldb.PingContext(ctx); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("DB %s への接続: %w", path, err)
	}

	return sqldb, nil
}

// dsn は接続文字列を組み立てる。
//
// PRAGMA は必ず DSN で渡す。`PRAGMA foreign_keys = ON` を Exec しても、
// 効くのはその1接続だけ。database/sql は接続をプールして使い回すため、
// 外部キーが効く接続と効かない接続が混在し、再現しない不整合になる。
func dsn(path string) string {
	q := url.Values{}

	// 書き込みは同時に1つしか通らない。即座に失敗させず、この時間まで待つ。
	q.Add("_pragma", "busy_timeout(5000)")

	// WAL は読み書きを並行させるために必須。DBファイルに永続する設定だが、
	// 新規作成時と、誰かが journal_mode を戻した場合のために毎回指定する。
	q.Add("_pragma", "journal_mode(WAL)")

	// SQLite の外部キーは既定で無効。有効にしないと、参照先の無い loans が作れてしまう。
	q.Add("_pragma", "foreign_keys(1)")

	// 全てのトランザクションを BEGIN IMMEDIATE で開始する。
	// 既定の deferred だと「読んでから書く」トランザクションが2つ重なった時、
	// 書き込みへの昇格でデッドロックし、busy_timeout では回避できない
	// （SQLite は待たずに SQLITE_BUSY を返す）。貸出登録のような
	// 「貸出中か確認してから INSERT する」処理がまさにこの形になる。
	q.Add("_txlock", "immediate")

	return "file:" + path + "?" + q.Encode()
}

// IsUniqueViolation は UNIQUE 制約違反かどうかを返す。
//
// 「そのログインIDは既にある」を 409 で返すために使う。
// エラー文字列で判定すると、ドライバの版が上がった時に静かに壊れて
// 500 を返すようになる。ここでコードを見て判定し、driver への依存を
// このパッケージに閉じ込める。
func IsUniqueViolation(err error) bool {
	var serr *sqlite.Error
	if !errors.As(err, &serr) {
		return false
	}

	// PRIMARY KEY の衝突も UNIQUE 違反として扱う。呼ぶ側から見れば同じこと。
	code := serr.Code()
	return code == sqlite3.SQLITE_CONSTRAINT_UNIQUE ||
		code == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY
}
