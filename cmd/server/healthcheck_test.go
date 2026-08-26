package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// portOf は httptest のURLから待ち受けポートだけを取り出す。
//
// runHealthcheck は接続先を 127.0.0.1 に固定している（scratch には
// /etc/hosts が無く localhost を解決できないため）。テストからは
// ポートだけを渡して同じ経路を通す。
func portOf(t *testing.T, rawURL string) string {
	t.Helper()

	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("URLの解析: %v", err)
	}
	return u.Port()
}

func TestRunHealthcheck(t *testing.T) {
	t.Run("200なら成功する", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/healthz" {
				t.Errorf("経路 = %q, want /healthz", r.URL.Path)
			}
			_, _ = w.Write([]byte("ok\n"))
		}))
		defer srv.Close()

		if err := runHealthcheck(context.Background(), portOf(t, srv.URL), time.Second); err != nil {
			t.Fatalf("runHealthcheck() = %v, want nil", err)
		}
	})

	// DBのボリュームが外れると /healthz は 503 を返す。プロセスは生きて
	// いるので、接続できたことだけを見ると healthy と報告してしまう。
	t.Run("503なら失敗する", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("db unavailable\n"))
		}))
		defer srv.Close()

		err := runHealthcheck(context.Background(), portOf(t, srv.URL), time.Second)
		if err == nil {
			t.Fatal("runHealthcheck() = nil, want error")
		}
		// 失敗の理由が Docker のヘルスチェックログに残ること。
		// ステータスだけでは、落ちているのか壊れているのか分からない。
		if !strings.Contains(err.Error(), "db unavailable") {
			t.Errorf("エラーに応答本文が含まれない: %v", err)
		}
	})

	t.Run("つながらなければ失敗する", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		port := portOf(t, srv.URL)
		srv.Close() // 誰も待ち受けていないポートにする

		if err := runHealthcheck(context.Background(), port, time.Second); err == nil {
			t.Fatal("runHealthcheck() = nil, want error")
		}
	})

	// 応答が返らない時に、Docker の timeout より先にこちらで諦める。
	// プロセスを殺されると終了コードだけが残り、原因が読めない。
	t.Run("応答が遅ければタイムアウトする", func(t *testing.T) {
		block := make(chan struct{})
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			<-block
		}))
		defer func() {
			close(block)
			srv.Close()
		}()

		start := time.Now()
		if err := runHealthcheck(context.Background(), portOf(t, srv.URL), 100*time.Millisecond); err == nil {
			t.Fatal("runHealthcheck() = nil, want error")
		}
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Errorf("タイムアウトが効いていない: %v", elapsed)
		}
	})
}
