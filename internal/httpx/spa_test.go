package httpx_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Rengemaru/equipment-management/internal/httpx"
)

// distFS は組み込まれたフロントを模す。Vite の出力と同じ形にする。
func distFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":              {Data: []byte("<!doctype html><div id=root></div>")},
		"assets/index-abc123.js":  {Data: []byte("console.log(1)")},
		"assets/index-def456.css": {Data: []byte("body{}")},
	}
}

func newSPA(t *testing.T) http.Handler {
	t.Helper()

	h, err := httpx.SPAHandler(distFS())
	if err != nil {
		t.Fatalf("SPAHandler: %v", err)
	}
	return h
}

func get(t *testing.T, h http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func TestSPAServesIndex(t *testing.T) {
	t.Parallel()

	rec := get(t, newSPA(t), "/")

	if rec.Code != http.StatusOK {
		t.Fatalf("状態コード = %d, 期待 %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "id=root") {
		t.Errorf("index.html が返っていない: %q", rec.Body.String())
	}
}

// 画面の経路はブラウザ側のルータが持つ。/i/0042 を直接開いた時に
// サーバが404を返すと、QRから来た人が必ず404を見る。
func TestSPAServesIndexForClientRoutes(t *testing.T) {
	t.Parallel()

	h := newSPA(t)

	for _, target := range []string{"/i/0042", "/items", "/admin/items/import", "/login"} {
		rec := get(t, h, target)

		if rec.Code != http.StatusOK {
			t.Errorf("%s: 状態コード = %d, 期待 %d", target, rec.Code, http.StatusOK)
		}
		if !strings.Contains(rec.Body.String(), "id=root") {
			t.Errorf("%s: index.html が返っていない", target)
		}
	}
}

// 更新後も古いJavaScriptを指したままの入口を配り続けないため。
func TestSPAIndexIsNotCached(t *testing.T) {
	t.Parallel()

	rec := get(t, newSPA(t), "/i/0042")

	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, 期待 %q", got, "no-cache")
	}
}

// ファイル名にハッシュが入っており、中身が変われば名前も変わる。
func TestSPAAssetsAreCachedLong(t *testing.T) {
	t.Parallel()

	rec := get(t, newSPA(t), "/assets/index-abc123.js")

	if rec.Code != http.StatusOK {
		t.Fatalf("状態コード = %d, 期待 %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "immutable") {
		t.Errorf("Cache-Control = %q, immutable を期待", got)
	}
	if rec.Body.String() != "console.log(1)" {
		t.Errorf("中身 = %q", rec.Body.String())
	}
}

// index.html を返すと、ブラウザはHTMLをJavaScriptとして解釈し
// 「Unexpected token '<'」とだけ言う。原因が分かるまで時間を溶かす。
func TestSPAMissingAssetIs404(t *testing.T) {
	t.Parallel()

	h := newSPA(t)

	for _, target := range []string{"/assets/nope.js", "/favicon.ico", "/i/0042.png"} {
		rec := get(t, h, target)

		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: 状態コード = %d, 期待 %d（本文=%q）",
				target, rec.Code, http.StatusNotFound, rec.Body.String())
		}
	}
}

// `npm run build` を通さずに起動した場合。呼び出し側が案内を出せるように、
// 判別できるエラーを返す。
func TestSPAHandlerWithoutFrontend(t *testing.T) {
	t.Parallel()

	// .gitkeep だけがある状態（フロントをビルドしていない）。
	_, err := httpx.SPAHandler(fstest.MapFS{".gitkeep": {Data: []byte{}}})

	if !errors.Is(err, httpx.ErrNoFrontend) {
		t.Fatalf("err = %v, 期待 %v", err, httpx.ErrNoFrontend)
	}
}

// 白い画面や404にしない。`npm run build` を忘れた人が、
// サーバの不具合と取り違えないようにする。
func TestNoFrontendHandlerExplains(t *testing.T) {
	t.Parallel()

	rec := get(t, httpx.NoFrontendHandler(), "/items")

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("状態コード = %d, 期待 %d", rec.Code, http.StatusServiceUnavailable)
	}
	if !strings.Contains(rec.Body.String(), "npm run build") {
		t.Errorf("何をすればよいかが書かれていない: %q", rec.Body.String())
	}
}

// 経路は "/"（全メソッド）で登録される。ServeMux が "GET /" と "/api/" の
// 組み合わせを曖昧と見なして panic するため、絞り込みはハンドラ側で行う。
func TestSPARejectsWrites(t *testing.T) {
	t.Parallel()

	h := newSPA(t)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/items", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("状態コード = %d, 期待 %d", rec.Code, http.StatusMethodNotAllowed)
	}
	if got := rec.Header().Get("Allow"); got != "GET, HEAD" {
		t.Errorf("Allow = %q", got)
	}
}

// 綴りを間違えたAPIが index.html を返すと、フロントは200のHTMLをJSONとして
// 読もうとし、原因が経路の誤りだと分からない。
func TestAPINotFoundReturnsJSON(t *testing.T) {
	t.Parallel()

	rec := get(t, httpx.APINotFoundHandler(), "/api/itemz")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("状態コード = %d, 期待 %d", rec.Code, http.StatusNotFound)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Errorf("Content-Type = %q, JSON を期待", got)
	}
	if strings.Contains(rec.Body.String(), "<") {
		t.Errorf("HTMLが混ざっている: %q", rec.Body.String())
	}
}
