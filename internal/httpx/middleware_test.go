package httpx

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestLogger はログの出力先を捕まえるロガーを返す。
func newTestLogger() (*log.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return log.New(&buf, "", 0), &buf
}

// do はハンドラに1回リクエストを流す。
func do(h http.Handler, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = "192.0.2.1:1234"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestAccessLog_1リクエストに1行出す(t *testing.T) {
	logger, buf := newTestLogger()

	h := AccessLog(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, "hello")
	}))

	do(h, http.MethodPost, "/items")

	got := buf.String()
	if n := strings.Count(strings.TrimSpace(got), "\n"); n != 0 {
		t.Errorf("1行を期待したが %d 行: %q", n+1, got)
	}
	for _, want := range []string{"POST", "/items", "201", "5B", "192.0.2.1"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q が含まれていない: %q", want, got)
		}
	}
}

// WriteHeader を呼ばないハンドラでも 200 として記録されること。
func TestAccessLog_暗黙の200を記録する(t *testing.T) {
	logger, buf := newTestLogger()

	h := AccessLog(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))

	do(h, http.MethodGet, "/healthz")

	if !strings.Contains(buf.String(), "200") {
		t.Errorf("200 が記録されていない: %q", buf.String())
	}
}

// クエリ文字列を出さない。next の行き先や検索語がログに残り続けても得るものが少ない。
func TestAccessLog_クエリ文字列を出さない(t *testing.T) {
	logger, buf := newTestLogger()

	h := AccessLog(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	do(h, http.MethodGet, "/login?next=/i/0042&q=秘密")

	got := buf.String()
	if strings.Contains(got, "next=") || strings.Contains(got, "秘密") {
		t.Errorf("クエリが出ている: %q", got)
	}
	if !strings.Contains(got, "/login") {
		t.Errorf("パスが出ていない: %q", got)
	}
}

// ヘルスチェックは数秒ごとに来る。成功している間は黙る。
func TestAccessLog_quietPathは成功時に出さない(t *testing.T) {
	logger, buf := newTestLogger()

	h := AccessLog(logger, "/healthz")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	do(h, http.MethodGet, "/healthz")

	if buf.Len() != 0 {
		t.Errorf("成功したヘルスチェックが記録されている: %q", buf.String())
	}
}

// 黙るのは成功時だけ。落ちているのに何も出ないと、気付く手段が無くなる。
func TestAccessLog_quietPathでも失敗時は出す(t *testing.T) {
	logger, buf := newTestLogger()

	h := AccessLog(logger, "/healthz")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))

	do(h, http.MethodGet, "/healthz")

	if !strings.Contains(buf.String(), "503") {
		t.Errorf("失敗したヘルスチェックが記録されていない: %q", buf.String())
	}
}

func TestRecover_panicを500にする(t *testing.T) {
	logger, buf := newTestLogger()

	h := Recover(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("何かが壊れた")
	}))

	w := do(h, http.MethodGet, "/items")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d。500 を期待", w.Code)
	}

	got := buf.String()
	if !strings.Contains(got, "何かが壊れた") {
		t.Errorf("panic の内容が記録されていない: %q", got)
	}
	// スタックが無いと、どこで落ちたか分からない。
	if !strings.Contains(got, "goroutine") {
		t.Errorf("スタックトレースが記録されていない: %q", got)
	}
	// 500 の本文に内部情報を出さない。
	if strings.Contains(w.Body.String(), "何かが壊れた") {
		t.Errorf("panic の内容が利用者に返っている: %q", w.Body.String())
	}
}

// 本文を書いた後の panic ではステータスを差し替えられない。
// 無理に書くと壊れた本文が繋がる。ログだけ残す。
func TestRecover_書き込み後のpanicは本文を壊さない(t *testing.T) {
	logger, buf := newTestLogger()

	h := Recover(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "途中まで")
		panic("その後で壊れた")
	}))

	w := do(h, http.MethodGet, "/items")

	if w.Code != http.StatusOK {
		t.Errorf("status = %d。書き込み済みの 200 のままを期待", w.Code)
	}
	if w.Body.String() != "途中まで" {
		t.Errorf("本文が壊れている: %q", w.Body.String())
	}
	if !strings.Contains(buf.String(), "その後で壊れた") {
		t.Errorf("panic が記録されていない: %q", buf.String())
	}
}

// net/http が「静かに接続を切る」ために使う値。握ると意図が壊れる。
func TestRecover_ErrAbortHandlerは投げ直す(t *testing.T) {
	logger, _ := newTestLogger()

	h := Recover(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic(http.ErrAbortHandler)
	}))

	defer func() {
		v := recover()
		if v == nil {
			t.Fatal("投げ直されていない")
		}
		if v != http.ErrAbortHandler {
			t.Errorf("別の値が投げられた: %v", v)
		}
	}()

	do(h, http.MethodGet, "/items")
}

// panic しても、そのリクエストのログは残ること。
// 何も出ないと、届いていないのか落ちたのか区別できない。
func TestNewHandler_panicしたリクエストもログに残る(t *testing.T) {
	logger, buf := newTestLogger()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /boom", func(w http.ResponseWriter, r *http.Request) {
		panic("壊れた")
	})

	w := do(NewHandler(mux, logger, "/healthz"), http.MethodGet, "/boom")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d。500 を期待", w.Code)
	}

	got := buf.String()
	if !strings.Contains(got, "panic:") {
		t.Errorf("panic が記録されていない: %q", got)
	}
	// アクセスログ側に 500 が出ること（Recover を内側に置いている理由）。
	if !strings.Contains(got, "GET /boom 500") {
		t.Errorf("アクセスログに 500 が出ていない: %q", got)
	}
}

func TestChain_先に渡したものが外側になる(t *testing.T) {
	var order []string

	mark := func(name string) Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name)
				next.ServeHTTP(w, r)
			})
		}
	}

	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		order = append(order, "handler")
	}), mark("外"), mark("内"))

	do(h, http.MethodGet, "/")

	want := []string{"外", "内", "handler"}
	if len(order) != len(want) {
		t.Fatalf("順序 = %v。%v を期待", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("順序 = %v。%v を期待", order, want)
		}
	}
}
