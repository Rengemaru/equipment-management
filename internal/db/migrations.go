package db

import (
	"embed"
	"io/fs"
)

// migrationsFS は連番SQLをバイナリに同梱する。
// デプロイを「バイナリ1つ + SQLiteファイル1つ」で完結させるため、
// SQLファイルを実行環境に配置させない。
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrations は同梱された連番SQLを返す。Migrate に渡して使う。
//
// fs.Sub で migrations/ を根に付け替えている。Migrate は直下の *.sql を見るため。
func Migrations() fs.FS {
	// パスは固定で、埋め込みに失敗すればビルドが通らない。実行時に失敗する余地はない。
	sub, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		panic("migrations の埋め込みが壊れている: " + err.Error())
	}
	return sub
}
