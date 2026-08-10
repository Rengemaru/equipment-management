// Command server は備品管理システムの HTTP サーバを起動する。
//
// 現時点では DB への接続とマイグレーション、/healthz のみ。
// Compose のヘルスチェックが /healthz に依存する。
package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Rengemaru/equipment-management/internal/config"
	"github.com/Rengemaru/equipment-management/internal/db"
)

func main() {
	// ログは標準出力に出し、収集は Docker に任せる。
	log.SetFlags(log.LstdFlags | log.LUTC)

	// 設定の不備は起動時に全て出して落とす。
	// 不完全な設定で起動させると、間違った場所に書き続けたまま運用が始まる。
	cfg, warnings, err := config.Load(os.Getenv)
	for _, w := range warnings {
		log.Printf("warning: %s", w)
	}
	if err != nil {
		log.Fatal(err)
	}

	addr := ":" + cfg.Port

	// 起動時に一度だけ接続する。失敗したら起動しない。
	// 接続できないまま起動すると、リクエストが来て初めて気付くことになる。
	ctx := context.Background()
	sqldb, err := db.Open(ctx, cfg.DBPath)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer func() { _ = sqldb.Close() }()

	// マイグレーションは起動のたびに適用する。適用済みの分は飛ばされる。
	// デプロイ手順に「マイグレーションを流す」という人手の操作を作らないため。
	if err := db.Migrate(ctx, sqldb, db.Migrations()); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	log.Printf("db ready: %s", cfg.DBPath)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealthz(sqldb))

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

// handleHealthz はプロセスが生きていて、DBに触れることを返す。
//
// プロセスの生存だけを見ると、DBのボリュームが外れた状態を healthy と報告する。
// Compose がこれを見て再起動しないため、壊れたまま動き続けることになる。
func handleHealthz(sqldb *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")

		// ヘルスチェック自体が詰まると、応答がないのか遅いのか区別できない。
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		if err := sqldb.PingContext(ctx); err != nil {
			log.Printf("healthz: db: %v", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("db unavailable\n"))
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	}
}
