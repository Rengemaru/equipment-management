// Command server は備品管理システムの HTTP サーバを起動する。
//
// サブコマンド:
//
//	-create-admin   最初の admin を作る（Webからは作れない）
//
// デプロイは「バイナリ1つ + SQLiteファイル1つ」で完結させる方針のため、
// 運用に必要な操作もこのバイナリのサブコマンドとして持たせる。
// 本番イメージにはシェルも sqlite3 も入れない。
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Rengemaru/equipment-management/internal/auth"
	"github.com/Rengemaru/equipment-management/internal/config"
	"github.com/Rengemaru/equipment-management/internal/db"
	"github.com/Rengemaru/equipment-management/internal/httpx"
	"github.com/Rengemaru/equipment-management/internal/item"
)

func main() {
	// ログは標準出力に出し、収集は Docker に任せる。
	log.SetFlags(log.LstdFlags | log.LUTC)

	var (
		doCreateAdmin = flag.Bool("create-admin", false, "admin ユーザーを作って終了する")
		loginID       = flag.String("login-id", "", "-create-admin で作るユーザーのログインID")
		name          = flag.String("name", "", "-create-admin で作るユーザーの表示名")
		email         = flag.String("email", "", "-create-admin で作るユーザーのメールアドレス（省略可）")
	)
	flag.Parse()

	// 設定の不備は起動時に全て出して落とす。
	// 不完全な設定で起動させると、間違った場所に書き続けたまま運用が始まる。
	//
	// サブコマンドでも同じ設定を要求する。DB_PATH だけ読む作りにすると、
	// 「create-admin は通るのにサーバが起動しない」状態を作れてしまう。
	cfg, warnings, err := config.Load(os.Getenv)
	for _, w := range warnings {
		log.Printf("warning: %s", w)
	}
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	// 起動時に一度だけ接続する。失敗したら起動しない。
	// 接続できないまま起動すると、リクエストが来て初めて気付くことになる。
	sqldb, err := db.Open(ctx, cfg.DBPath)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer func() { _ = sqldb.Close() }()

	// マイグレーションは起動のたびに適用する。適用済みの分は飛ばされる。
	// デプロイ手順に「マイグレーションを流す」という人手の操作を作らないため。
	//
	// -create-admin でも先に適用する。空のDBに対して最初に実行されるのは
	// こちらなので、ここで適用しないと必ず失敗する。
	if err := db.Migrate(ctx, sqldb, db.Migrations()); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	if *doCreateAdmin {
		in := createAdminInput{LoginID: *loginID, Name: *name, Email: *email}
		if err := runCreateAdmin(ctx, sqldb, in, os.Stdout); err != nil {
			log.Fatalf("create-admin: %v", err)
		}
		return
	}

	log.Printf("db ready: %s", cfg.DBPath)
	if err := runServer(ctx, cfg, sqldb); err != nil {
		log.Fatal(err)
	}
}

// runServer は HTTP サーバを起動し、終了信号を受けるまで動かす。
func runServer(ctx context.Context, cfg *config.Config, sqldb *sql.DB) error {
	addr := ":" + cfg.Port

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealthz(sqldb))

	// 経路の登録は各パッケージに任せる。main が全ルートを知っていると、
	// ハンドラを足すたびに main が育ち、どこに何があるか追えなくなる。
	sessions := auth.NewSessionStore(sqldb, cfg.SessionSecret)
	authHandler := auth.NewHandler(auth.NewStore(sqldb), sessions, auth.NewThrottle(sqldb), cfg.CookieSecure)
	authHandler.Register(mux)

	// 備品の読み取りは member も可。誰が何を持っているかが全員に見える状態を
	// 作ることが、罰則より強く働く（CLAUDE.md）。
	photos, err := item.NewPhotoStore(cfg.UploadDir)
	if err != nil {
		return fmt.Errorf("写真の保存先: %w", err)
	}
	item.NewHandler(item.NewStore(sqldb), photos, cfg.HostURL, authHandler.RequireLogin, authHandler.RequireAdmin).Register(mux)

	// /healthz は Compose のヘルスチェックが数秒ごとに叩く。
	// 成功している間はログに出さない。出すと本当に見たい行が流れる。
	handler := httpx.NewHandler(mux, log.Default(), "/healthz")

	srv := &http.Server{
		Addr:    addr,
		Handler: handler,
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

	stopCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(stopCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}

	log.Print("stopped")
	return nil
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
