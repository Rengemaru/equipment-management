// Command server は備品管理システムの HTTP サーバを起動する。
//
// サブコマンド:
//
//	-create-admin   最初の admin を作る（Webからは作れない）
//	-backup <path>  稼働中でも一貫したDBのコピーを作る
//	-restore <path> バックアップから戻す（__サーバを止めてから実行する__）
//	-healthcheck    自分自身の /healthz を叩く（Compose のヘルスチェック用）
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
	"github.com/Rengemaru/equipment-management/internal/loan"
	"github.com/Rengemaru/equipment-management/internal/notify"
	"github.com/Rengemaru/equipment-management/web"
)

func main() {
	// ログは標準出力に出し、収集は Docker に任せる。
	log.SetFlags(log.LstdFlags | log.LUTC)

	var (
		doCreateAdmin = flag.Bool("create-admin", false, "admin ユーザーを作って終了する")
		loginID       = flag.String("login-id", "", "-create-admin で作るユーザーのログインID")
		name          = flag.String("name", "", "-create-admin で作るユーザーの表示名")
		email         = flag.String("email", "", "-create-admin で作るユーザーのメールアドレス（省略可）")

		backupPath  = flag.String("backup", "", "指定したパスにDBのコピーを作って終了する")
		restorePath = flag.String("restore", "", "指定したバックアップからDBを戻して終了する（サーバを止めてから実行する）")

		doHealthcheck = flag.Bool("healthcheck", false, "自分自身の /healthz を叩いて終了する（コンテナのヘルスチェック用）")
	)
	flag.Parse()

	// ヘルスチェックは設定の読み込みより前で捌く。
	//
	// これは動いているサーバと同じコンテナで、数秒ごとに実行される。
	// __DBを開いてはならないし、マイグレーションを走らせてはならない。__
	// 見たいのは「サーバが応答するか」だけで、必要なのは PORT だけ。
	//
	// タイムアウトはヘルスチェック側の timeout より短くする。長いと
	// Docker がプロセスを殺し、応答が無いのか遅いのか区別できなくなる。
	if *doHealthcheck {
		if err := runHealthcheck(context.Background(), config.PortFromEnv(os.Getenv), 3*time.Second); err != nil {
			log.Fatalf("healthcheck: %v", err)
		}
		return
	}

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

	// 復元は DB を開く前に捌く。
	//
	// ここより後ろに置くと、__戻す先を開いた時点で空のDBと -wal が作られ、__
	// __これから消すファイルを自分で用意することになる。__
	// マイグレーションも、戻した後の中身に対して runRestore が改めて流す。
	if *restorePath != "" {
		if err := runRestore(ctx, *restorePath, cfg.DBPath, os.Stdout); err != nil {
			log.Fatalf("restore: %v", err)
		}
		return
	}

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

	if *backupPath != "" {
		if err := runBackup(ctx, sqldb, *backupPath, os.Stdout); err != nil {
			log.Fatalf("backup: %v", err)
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
	items := item.NewStore(sqldb)
	item.NewHandler(items, photos, cfg.HostURL, authHandler.RequireLogin, authHandler.RequireAdmin).Register(mux)

	// メールは「送れる人にだけ送る」補助手段。未設定でも起動する。
	// 起動時に一度だけ状態をログに出す。送れているつもりで送れていない状態が
	// 一番気付きにくい。
	mailer := notify.New(notify.Options{
		Host:     cfg.SMTP.Host,
		Port:     cfg.SMTP.Port,
		User:     cfg.SMTP.User,
		Password: cfg.SMTP.Password,
		From:     cfg.SMTP.From,
	})
	if mailer.Enabled() {
		log.Printf("mail: %s 経由で送信する", cfg.SMTP.Host)
	} else {
		log.Print("mail: SMTP_HOST が未設定のため送信しない")
	}

	// 貸出。loan は auth を参照しない（テストでログイン済みの利用者を
	// 作れなくなるため）。context からの取り出し方だけをここで渡す。
	currentUser := func(ctx context.Context) (loan.Actor, bool) {
		u, ok := auth.UserFrom(ctx)
		if !ok {
			return loan.Actor{}, false
		}
		return loan.Actor{ID: u.ID, IsAdmin: u.Role == auth.RoleAdmin}, true
	}
	loan.NewHandler(loan.NewStore(sqldb), items, mailer.Send, cfg.HostURL, currentUser, authHandler.RequireLogin).Register(mux)

	// 登録の無い /api/ は JSON で404を返す。これが無いと下の "/" に落ち、
	// 綴りを間違えたAPIが index.html を返す。フロントは200のHTMLをJSONとして
	// 読もうとし、原因が経路の誤りだと分からなくなる。
	mux.Handle("/api/", httpx.APINotFoundHandler())

	// 残り全部がフロント。画面の経路はブラウザ側のルータが持つため、
	// 知らないパスでも index.html を返す（/i/0042 を直接開いた時に404にしない）。
	if err := registerFrontend(mux); err != nil {
		return err
	}

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

// registerFrontend は組み込んだフロントを "/" に割り当てる。
//
// フロントが入っていなくても起動は止めない。`-create-admin` の直後や、
// APIだけを確かめたい時に、画面が無いというだけで起動できないのは不便すぎる。
// 代わりに、何をすればよいかを返すハンドラを置いてログに残す。
func registerFrontend(mux *http.ServeMux) error {
	dist, err := web.Dist()
	if err != nil {
		return fmt.Errorf("フロントエンド: %w", err)
	}

	spa, err := httpx.SPAHandler(dist)
	if err != nil {
		if !errors.Is(err, httpx.ErrNoFrontend) {
			return fmt.Errorf("フロントエンド: %w", err)
		}

		log.Print("warning: フロントエンドが組み込まれていない（web/ で npm run build を実行すること）")
		mux.Handle("/", httpx.NoFrontendHandler())
		return nil
	}

	// "GET /" では登録できない。ServeMux が "/api/"（全メソッド）との
	// 組み合わせを曖昧と見なして起動時に panic する
	// （"matches fewer methods but has a more general path pattern"）。
	// メソッドの絞り込みはハンドラ側で行う。
	mux.Handle("/", spa)
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
