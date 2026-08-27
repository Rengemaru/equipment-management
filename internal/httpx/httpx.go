// Package httpx は HTTP の共通部品を持つ。ルーティングの組み立て、
// アクセスログ、panic の握り。
//
// フレームワークは入れない。標準の http.ServeMux で足りる規模で、
// 引き継いだ人が net/http の知識だけで読めることを優先する。
package httpx

import (
	"log"
	"net/http"
)

// Middleware は http.Handler を包む。
type Middleware func(http.Handler) http.Handler

// Chain は Middleware を適用する。先に渡したものが外側になる。
//
// 外側から順に書けるようにしているのは、リクエストが通る順序と
// コードの並びを一致させるため。逆順だと読むたびに頭の中で反転することになる。
func Chain(h http.Handler, middlewares ...Middleware) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}

// NewHandler は mux に標準のミドルウェアを被せたルートハンドラを返す。
//
// ミドルウェアの構成をここ1箇所に決めておく。呼ぶ側それぞれが並べると、
// 順序が食い違ってもテストでは気付けない。
//
// quietPaths は、成功した時だけログを出さないパス。ヘルスチェックのように
// 数秒ごとに叩かれるものを渡す。
func NewHandler(mux *http.ServeMux, logger *log.Logger, quietPaths ...string) http.Handler {
	// AccessLog を外側に置く。Recover を外側にすると、panic した
	// リクエストのログが status 0 のまま出る（Recover が 500 を書くのは
	// AccessLog の defer が走った後になるため）。
	return Chain(mux,
		AccessLog(logger, quietPaths...),
		Recover(logger),
	)
}

// recorder は書き込まれたステータスとバイト数を覚える http.ResponseWriter。
type recorder struct {
	http.ResponseWriter

	status  int
	written int64
	// wroteHeader は WriteHeader が呼ばれたか。
	// panic 後に 500 を書いてよいかの判定に使う。
	wroteHeader bool
}

func (r *recorder) WriteHeader(status int) {
	if r.wroteHeader {
		// 二重呼び出しは net/http が警告を出す。ここで止める。
		return
	}
	r.status = status
	r.wroteHeader = true
	r.ResponseWriter.WriteHeader(status)
}

func (r *recorder) Write(b []byte) (int, error) {
	// WriteHeader を呼ばずに Write すると 200 が書かれる。同じ扱いにする。
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	n, err := r.ResponseWriter.Write(b)
	r.written += int64(n)
	return n, err
}

// Unwrap は http.ResponseController に元の ResponseWriter を渡す。
// これが無いと Flush などが使えなくなる。
func (r *recorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}
