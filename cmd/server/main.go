// Command server は備品管理システムの HTTP サーバを起動する。
//
// M0 時点では /healthz のみ。Compose のヘルスチェックがこれに依存するため、
// 中身のある機能より先に用意している。
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// ログは標準出力に出し、収集は Docker に任せる。
	log.SetFlags(log.LstdFlags | log.LUTC)

	addr := ":" + env("PORT", "8080")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealthz)

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
		// 部室のネットワークで接続が切れたまま残るのを防ぐ。
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// docker stop は SIGTERM を送る。受け取ってから終了するまでに
	// 処理中のリクエストを捨てないようにする。SQLite への書き込み途中で
	// 落とすと、復旧の手間が記録の信頼性に直結する。
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-shutdown
	log.Print("shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
	log.Print("stopped")
}

// handleHealthz はプロセスが生きていることだけを返す。
// DB 疎通の確認は、DB を導入する M1 で足す。
func handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
