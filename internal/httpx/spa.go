package httpx

import (
	"errors"
	"io/fs"
	"log"
	"net/http"
	"path"
	"strings"
	"time"
)

// indexFile はフロントの入口。
const indexFile = "index.html"

// assetsPrefix はハッシュ付きのファイルが置かれる場所（Vite の既定）。
const assetsPrefix = "/assets/"

// ErrNoFrontend はフロントが組み込まれていないこと。
//
// `npm run build` を通さずにサーバを起動すると、この状態になる。
var ErrNoFrontend = errors.New("フロントエンドが組み込まれていない")

// SPAHandler は組み込んだフロントを配信するハンドラを返す。
//
// # 知らないパスで index.html を返す
//
// 画面の経路はブラウザ側のルータが持つ。`/i/0042` を直接開いた時に
// サーバがファイルを探して404を返すと、__QRから来た人が必ず404を見る。__
// ファイルが無いパスは index.html を返し、あとはブラウザに任せる。
//
// # 拡張子のあるパスは404にする
//
// `/assets/index-abc.js` が見つからない時に index.html を返すと、ブラウザは
// HTMLをJavaScriptとして解釈し、「Unexpected token '<'」とだけ言う。
// 原因がフロントの取り違えだと分かるまで時間を溶かす。
//
// フロントが組み込まれていなければ ErrNoFrontend を返す。呼び出し側が
// 起動を止めるか、案内を出すかを決める。
func SPAHandler(dist fs.FS) (http.Handler, error) {
	index, err := fs.ReadFile(dist, indexFile)
	if err != nil {
		return nil, ErrNoFrontend
	}

	files := http.FileServerFS(dist)

	// index.html の更新時刻は埋め込みでは取れない（embed は0値になる）。
	// 起動時刻を使う。再起動のたびに変わるが、no-cache で返すため影響しない。
	modTime := time.Now()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 経路は "/"（全メソッド）で登録される。"GET /" にすると ServeMux が
		// "/api/" との組み合わせを曖昧と見なして panic するため、
		// メソッドの絞り込みはここで行う。
		if !isRead(r.Method) {
			w.Header().Set("Allow", "GET, HEAD")
			WriteError(w, http.StatusMethodNotAllowed, "その操作はできません")
			return
		}

		upath := path.Clean("/" + r.URL.Path)

		if strings.HasPrefix(upath, assetsPrefix) {
			// ファイル名にハッシュが入っており、中身が変われば名前も変わる。
			// 長く持たせてよい。スマートフォンでの再訪が軽くなる。
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			files.ServeHTTP(w, r)
			return
		}

		if exists(dist, upath) {
			files.ServeHTTP(w, r)
			return
		}

		// 拡張子があるのに見つからないなら、それはファイルの取り違え。
		// index.html を返すと原因が分からなくなる。
		if path.Ext(upath) != "" {
			http.NotFound(w, r)
			return
		}

		// index.html はキャッシュさせない。ここを持たせると、更新後も
		// 古いJavaScriptを指したままの入口が配られ続ける。
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeContent(w, r, indexFile, modTime, strings.NewReader(string(index)))
	}), nil
}

// NoFrontendHandler はフロントが組み込まれていないことを伝えるハンドラ。
//
// 白い画面や 404 にしない。開発中に `npm run build` を忘れた人が、
// サーバの不具合と取り違えないようにする。
func NoFrontendHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isRead(r.Method) {
			w.Header().Set("Allow", "GET, HEAD")
			WriteError(w, http.StatusMethodNotAllowed, "その操作はできません")
			return
		}

		if path.Ext(path.Clean("/"+r.URL.Path)) != "" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(
			"フロントエンドが組み込まれていません。\n\n" +
				"web/ で `npm run build` を実行してからサーバを起動し直してください。\n" +
				"開発中は `npm run dev`（http://localhost:5173）を使えます。\n",
		))
	})
}

// APINotFoundHandler は登録の無い /api/ 以下に JSON で404を返す。
//
// これが無いと SPAHandler に落ち、綴りを間違えたAPIが index.html を返す。
// フロントは200のHTMLをJSONとして読もうとし、原因が経路の誤りだと分からない。
func APINotFoundHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, http.StatusNotFound, "そのAPIはありません")
	})
}

// isRead は本文を読むだけのメソッドか。
func isRead(method string) bool {
	return method == http.MethodGet || method == http.MethodHead
}

// exists はパスに読めるファイルがあるかを返す。ディレクトリは無いものとして扱う。
func exists(dist fs.FS, upath string) bool {
	name := strings.TrimPrefix(upath, "/")
	if name == "" {
		return false
	}

	info, err := fs.Stat(dist, name)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			log.Printf("spa: %v", err)
		}
		return false
	}

	return !info.IsDir()
}
