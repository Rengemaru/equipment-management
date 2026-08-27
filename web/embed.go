// Package web はフロントエンドのビルド成果物を Go バイナリに埋め込む。
//
// この宣言が web/ に置いてあるのは、go:embed が親ディレクトリ（..）を
// 辿れないため。assets/fonts.go と同じ理由（CLAUDE.md）。
//
// 埋め込むことで、デプロイが「バイナリ1つ + SQLiteファイル1つ」で完結する。
// 静的ファイルを別に配る形にすると、置き場所と権限の設定が引き継ぎ手順に増える。
package web

import (
	"embed"
	"io/fs"
)

// distFS は `npm run build` の出力。
//
// all: を付けるのは `.gitkeep` を取り込むため。フロントをビルドしていない
// 環境では dist にこれしか無く、__埋め込める中身が1つも無いとコンパイルが落ちる__
// （"contains no embeddable files"）。CI の Go ジョブや、クローン直後の
// `go build ./...` がそれに当たる。
//
//go:embed all:dist
var distFS embed.FS

// Dist はビルド成果物を dist/ を根とした形で返す。
//
// フロントをビルドしていない場合、中身は空に近い状態で返る。ここでは
// 判定しない。「配信できるか」は使う側（httpx.SPAHandler）が決める。
func Dist() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}
