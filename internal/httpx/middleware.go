package httpx

import (
	"log"
	"net/http"
	"runtime/debug"
	"slices"
	"time"
)

// AccessLog は1リクエストごとに1行を出す。
//
// クエリ文字列は出さない。検索語や next の行き先が残り続けても得るものが少ない。
// ログは Docker が集める前提で、閲覧範囲を絞れない。
//
// quietPaths に挙げたパスは、2xx / 3xx の間はログを出さない。
// ヘルスチェックは数秒ごとに来るため、そのまま出すとログが埋まって
// 本当に見たい行が流れる。失敗した時は出す。
func AccessLog(logger *log.Logger, quietPaths ...string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &recorder{ResponseWriter: w, status: http.StatusOK}

			// panic で巻き戻る時もログを残す。何も出ないと、
			// リクエストが届いていないのか落ちたのか区別できない。
			defer func() {
				if slices.Contains(quietPaths, r.URL.Path) && rec.status < 400 {
					return
				}
				logger.Printf("%s %s %d %dB %s %s",
					r.Method,
					r.URL.Path,
					rec.status,
					rec.written,
					time.Since(start).Round(time.Millisecond),
					r.RemoteAddr,
				)
			}()

			next.ServeHTTP(rec, r)
		})
	}
}

// Recover は panic を握って 500 を返す。
//
// 1リクエストの不具合でプロセスごと落とすと、他の全員が巻き添えになる。
// 部室のマシンは誰かが見ている前提が無く、落ちたまま気付かれない。
func Recover(logger *log.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec, ok := w.(*recorder)
			if !ok {
				rec = &recorder{ResponseWriter: w, status: http.StatusOK}
				w = rec
			}

			defer func() {
				v := recover()
				if v == nil {
					return
				}

				// net/http が「静かに接続を切る」ために使う値。
				// 握ると意図が壊れるので、そのまま投げ直す。
				if v == http.ErrAbortHandler {
					panic(v)
				}

				// スタックまで出す。どこで落ちたか分からないログは、
				// 出ていないのとあまり変わらない。
				logger.Printf("panic: %s %s: %v\n%s", r.Method, r.URL.Path, v, debug.Stack())

				// 途中まで書いた後の panic では、ステータスも本文も差し替えられない。
				// 無理に書くと壊れた本文が繋がるだけなので、ログだけ残す。
				if rec.wroteHeader {
					return
				}
				http.Error(rec, "internal server error", http.StatusInternalServerError)
			}()

			next.ServeHTTP(w, r)
		})
	}
}
