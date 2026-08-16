// Package config は環境変数を読み、起動可能かどうかを判定する。
//
// 設定の不備は起動時に全て出す。1つずつ落として直させると、
// 4つ足りない時に4回起動し直すことになる。
package config

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Config はサーバの動作に必要な設定。
type Config struct {
	// Port は待ち受けポート。":8080" ではなく "8080"。
	Port string

	// HostURL は QR に埋め込む URL の土台。末尾にスラッシュを含まない。
	HostURL string

	// DBPath は SQLite ファイルのパス。
	DBPath string

	// UploadDir は備品写真の保存先ディレクトリ。
	UploadDir string

	// SessionSecret はセッションCookieの署名鍵。
	SessionSecret []byte

	// CookieSecure は Cookie に Secure 属性を付けるか。
	CookieSecure bool
}

// minSecretLen は SESSION_SECRET の最小長。
// 短い鍵は総当たりで復元でき、セッションを偽造されるとログインを迂回される。
const minSecretLen = 32

// devSecret は .env.example が持つ開発用の値。本番で使われていないか警告するために持つ。
const devSecret = "change-me-this-is-only-for-local-development"

// Load は環境変数を読んで Config を組み立てる。
//
// getenv を引数で受けるのは、テストでプロセスの環境変数を書き換えないため。
// 呼び出し側は config.Load(os.Getenv) とする。
//
// 第2戻り値は警告。起動は妨げないが、運用者に伝えるべきこと。
func Load(getenv func(string) string) (*Config, []string, error) {
	var problems []string
	var warnings []string

	// 必須値には既定値を置かない。
	// 書き込み先やURLを推測すると、間違った場所に書き続けたまま運用が始まる。
	require := func(key string) string {
		v := strings.TrimSpace(getenv(key))
		if v == "" {
			problems = append(problems, fmt.Sprintf("%s が未設定", key))
		}
		return v
	}

	cfg := &Config{
		DBPath:    require("DB_PATH"),
		UploadDir: require("UPLOAD_DIR"),
	}

	// ---- PORT ----
	// 既定値を置いてよい。間違っても待ち受け先が変わるだけで、データは壊れない。
	cfg.Port = strings.TrimSpace(getenv("PORT"))
	if cfg.Port == "" {
		cfg.Port = "8080"
	}
	if n, err := strconv.Atoi(cfg.Port); err != nil || n < 1 || n > 65535 {
		problems = append(problems, fmt.Sprintf("PORT が不正: %q", cfg.Port))
	}

	// ---- HOST_URL ----
	// QRに焼き込まれる。誤った値で印刷したラベルは貼り替えられないため、
	// 「動くが間違っている」状態を作らないよう形式まで検査する。
	cfg.HostURL = require("HOST_URL")
	if cfg.HostURL != "" {
		if err := validateHostURL(cfg.HostURL); err != nil {
			problems = append(problems, fmt.Sprintf("HOST_URL が不正: %v", err))
		}
	}

	// ---- SESSION_SECRET ----
	secret := require("SESSION_SECRET")
	switch {
	case secret == "":
		// require が既に報告している。
	case len(secret) < minSecretLen:
		problems = append(problems, fmt.Sprintf(
			"SESSION_SECRET が短い（%d文字）。%d文字以上にする", len(secret), minSecretLen))
	case secret == devSecret:
		warnings = append(warnings, "SESSION_SECRET が .env.example の値のまま。本番では必ず変更する")
	}
	cfg.SessionSecret = []byte(secret)

	// ---- COOKIE_SECURE ----
	// 既定は true。HTTP運用で無限ログインループを踏んだ人が明示的に落とす。
	// 逆にすると、HTTPS本番で設定を忘れた時に Cookie が平文で流れる。
	// 解釈できない値を false に倒さないこと。危険な側に倒れる。
	raw := strings.TrimSpace(getenv("COOKIE_SECURE"))
	if raw == "" {
		cfg.CookieSecure = true
	} else {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			problems = append(problems, fmt.Sprintf(
				"COOKIE_SECURE が不正: %q（true か false）", raw))
		}
		cfg.CookieSecure = v
	}

	if len(problems) > 0 {
		return nil, warnings, &Error{Problems: problems}
	}

	return cfg, warnings, nil
}

// validateHostURL は QR に埋め込める形かを検査する。
func validateHostURL(raw string) error {
	// 末尾スラッシュを許すと {HOST_URL}/i/{code} が // を含むURLになる。
	// 動くかどうかは経路次第で、印刷後に気付いても直せない。
	if strings.HasSuffix(raw, "/") {
		return errors.New("末尾のスラッシュを外す")
	}

	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("http:// か https:// で始める（%q）", raw)
	}
	if u.Host == "" {
		return fmt.Errorf("ホスト名がない（%q）", raw)
	}

	return nil
}

// Error は設定の不備をまとめて表す。
type Error struct {
	Problems []string
}

func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString("設定に不備がある:")
	for _, p := range e.Problems {
		b.WriteString("\n  - ")
		b.WriteString(p)
	}
	b.WriteString("\n.env.example をコピーして .env を作ること")
	return b.String()
}
