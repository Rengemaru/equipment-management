// Package db は SQLite への接続とスキーマのマイグレーションを扱う。
package db

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strconv"
)

// migrationFilePattern は連番SQLのファイル名。`0001_init.sql` の形式。
//
// 連番を人手で振る前提のため、桁数は固定しない（0001 でも 10 でも動く）。
// 名前の部分を必須にしているのは、`0002.sql` のような何をするか分からない
// ファイル名を履歴に残さないため。数年後に読む人にとっては名前が唯一の手がかりになる。
var migrationFilePattern = regexp.MustCompile(`^(\d+)_([0-9a-zA-Z_-]+)\.sql$`)

// migration は適用対象の1ファイル。
type migration struct {
	version int
	name    string
	file    string
}

// Migrate は fsys 直下の連番SQLのうち、まだ適用されていないものをバージョン順に適用する。
//
// 各ファイルは1つのトランザクションで実行する。途中で失敗したファイルは丸ごと巻き戻り、
// そのファイルは未適用のまま残る。中途半端に適用された状態を作らないための境界。
//
// 適用済みファイルの内容が後から書き換えられていないかは検証しない。
// 「既存ファイルを書き換えない」は規約（CLAUDE.md）で守る。ハッシュ照合を入れると
// 仕組みが増える割に、規約を破った人を止められるわけではない。
func Migrate(ctx context.Context, sqldb *sql.DB, fsys fs.FS) error {
	all, err := loadMigrations(fsys)
	if err != nil {
		return err
	}

	if err := ensureSchemaMigrations(ctx, sqldb); err != nil {
		return err
	}

	applied, err := appliedVersions(ctx, sqldb)
	if err != nil {
		return err
	}

	pending, err := selectPending(all, applied)
	if err != nil {
		return err
	}

	for _, m := range pending {
		sqlText, err := fs.ReadFile(fsys, m.file)
		if err != nil {
			return fmt.Errorf("マイグレーション %s の読み込み: %w", m.file, err)
		}
		if err := applyOne(ctx, sqldb, m, string(sqlText)); err != nil {
			return err
		}
	}

	return nil
}

// loadMigrations は fsys 直下の *.sql を読み、バージョン順に並べて返す。
func loadMigrations(fsys fs.FS) ([]migration, error) {
	files, err := fs.Glob(fsys, "*.sql")
	if err != nil {
		return nil, fmt.Errorf("マイグレーションの列挙: %w", err)
	}

	migrations := make([]migration, 0, len(files))
	seen := make(map[int]string, len(files))

	for _, f := range files {
		base := path.Base(f)
		match := migrationFilePattern.FindStringSubmatch(base)
		if match == nil {
			return nil, fmt.Errorf("マイグレーションのファイル名が不正: %s（0001_init.sql の形式にする）", base)
		}

		version, err := strconv.Atoi(match[1])
		if err != nil {
			return nil, fmt.Errorf("マイグレーション %s のバージョン番号: %w", base, err)
		}
		if version <= 0 {
			return nil, fmt.Errorf("マイグレーション %s のバージョン番号は1以上にする", base)
		}

		// 0001 と 1 のような、同じ番号を指す別ファイルを弾く。
		// どちらが先に適用されるかが環境で変わり、再現しない不整合になる。
		if prev, dup := seen[version]; dup {
			return nil, fmt.Errorf("マイグレーションのバージョンが重複: %s と %s", prev, base)
		}
		seen[version] = base

		migrations = append(migrations, migration{version: version, name: match[2], file: f})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})

	return migrations, nil
}

// ensureSchemaMigrations は適用履歴を記録するテーブルを用意する。
// ランナー自身が持つテーブルなので、連番SQL側では作らない。
func ensureSchemaMigrations(ctx context.Context, sqldb *sql.DB) error {
	const q = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    INTEGER PRIMARY KEY,
    name       TEXT NOT NULL,
    applied_at TEXT NOT NULL DEFAULT (datetime('now'))
)`
	if _, err := sqldb.ExecContext(ctx, q); err != nil {
		return fmt.Errorf("schema_migrations の作成: %w", err)
	}
	return nil
}

// appliedVersions は適用済みのバージョン集合を返す。
func appliedVersions(ctx context.Context, sqldb *sql.DB) (map[int]bool, error) {
	rows, err := sqldb.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("適用済みバージョンの取得: %w", err)
	}
	defer rows.Close()

	applied := make(map[int]bool)
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("適用済みバージョンの読み取り: %w", err)
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("適用済みバージョンの読み取り: %w", err)
	}

	return applied, nil
}

// selectPending は未適用のマイグレーションを返す。
//
// 適用済みの最大バージョンより小さい未適用ファイルがあれば、エラーにして何も適用しない。
// ブランチを分けて開発すると 0005 が適用済みの環境に後から 0004 が現れることがあり、
// 黙って適用すると環境ごとにスキーマの適用順が変わる。手で直す判断を人間に渡す。
func selectPending(all []migration, applied map[int]bool) ([]migration, error) {
	maxApplied := 0
	for v := range applied {
		if v > maxApplied {
			maxApplied = v
		}
	}

	var pending []migration
	for _, m := range all {
		if applied[m.version] {
			continue
		}
		if m.version < maxApplied {
			return nil, fmt.Errorf(
				"マイグレーション %s が未適用だが、より新しい %d が適用済み。連番を振り直すか手で適用する",
				m.file, maxApplied)
		}
		pending = append(pending, m)
	}

	return pending, nil
}

// applyOne は1ファイルを1トランザクションで適用し、履歴に記録する。
func applyOne(ctx context.Context, sqldb *sql.DB, m migration, sqlText string) error {
	tx, err := sqldb.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("マイグレーション %s のトランザクション開始: %w", m.file, err)
	}
	// 成功時は Commit 済みで Rollback が失敗するだけなので、戻り値は捨てる。
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, sqlText); err != nil {
		return fmt.Errorf("マイグレーション %s の適用: %w", m.file, err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, name) VALUES (?, ?)`,
		m.version, m.name); err != nil {
		return fmt.Errorf("マイグレーション %s の記録: %w", m.file, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("マイグレーション %s の確定: %w", m.file, err)
	}

	return nil
}
