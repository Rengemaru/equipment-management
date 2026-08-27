package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// runHealthcheck は自分自身の /healthz を叩き、正常なら nil を返す。
//
// # なぜバイナリの機能として持つのか
//
// 本番イメージは scratch で、シェルも curl も wget も入っていない。
// Compose の `test: ["CMD-SHELL", "curl ..."]` は成立しない。
// 「バイナリ1つで完結させる」方針どおり、ここに持たせる（CLAUDE.md）。
//
//	healthcheck:
//	  test: ["CMD", "/server", "-healthcheck"]
//
// CMD の配列形式はシェルを介さずに実行されるため、scratch でも動く。
// ENTRYPOINT は使われないので、バイナリのパスを自分で書く必要がある。
//
// # なぜ localhost ではなく 127.0.0.1 なのか
//
// scratch には /etc/hosts が無い。"localhost" は名前解決に失敗する。
// リテラルのIPなら解決器を通らない。
func runHealthcheck(ctx context.Context, port string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	url := "http://" + net.JoinHostPort("127.0.0.1", port) + "/healthz"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	// http.DefaultClient を使わない。既定の Transport は HTTP_PROXY を読む。
	// プロキシの設定が環境に入っていると、自分自身への接続が外へ回り、
	// サーバが健全でも失敗する。ヘルスチェックは必ず直接つなぐ。
	client := &http.Client{Transport: &http.Transport{}}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	// 失敗の理由を Docker のヘルスチェックログに残すため本文を読む。
	// /healthz は数十バイトしか返さないが、経路を間違えて別の応答を
	// 掴んだ時に全部読まないよう上限を付ける。
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s: %s", url, resp.Status, strings.TrimSpace(string(body)))
	}

	// 成功時は何も出さない。Docker は成功したチェックの出力も保持するため、
	// 毎回何か書くと `docker inspect` が同じ行で埋まる。
	return nil
}
