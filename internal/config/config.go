// Package config は環境変数を読み、起動可能かどうかを判定する。
//
// 設定の不備は起動時に全て出す。1つずつ落として直させると、
// 4つ足りない時に4回起動し直すことになる。
package config

import (
	"errors"
	"fmt"
	"net/mail"
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

	// SMTP はメールの送信先。Host が空ならメールを送らない。
	SMTP SMTPConfig
}

// SMTPConfig はメールの送信先。
//
// __未設定でも起動する。__ 学内SMTPの可否が未確定なため、メールを必須にすると
// 使えなかった時点でシステム全体が動かせなくなる。認証はメールに依存しない
// 設計（M1）で、メールは「送れる人にだけ送る」補助手段として扱う。
type SMTPConfig struct {
	// Host が空ならメールを送らない。
	Host string

	// Port は接続先ポート。未設定なら defaultSMTPPort。
	Port string

	// User が空なら認証しない。学内のリレーは認証不要のことが多い。
	User string

	// Password は User と対で使う。
	Password string

	// From は差出人。"名前 <a@example.com>" の形も受け付ける。
	From string
}

// Enabled はメールを送る設定になっているか。
func (c SMTPConfig) Enabled() bool { return c.Host != "" }

// minSecretLen は SESSION_SECRET の最小長。
// 短い鍵は総当たりで復元でき、セッションを偽造されるとログインを迂回される。
const minSecretLen = 32

// devSecret は .env.example が持つ開発用の値。本番で使われていないか警告するために持つ。
const devSecret = "change-me-this-is-only-for-local-development"

// defaultPort は PORT が未設定のときに使う値。
const defaultPort = "8080"

// defaultSMTPPort は SMTP_PORT が未設定のときに使う値。
// 587 は submission ポート。25 は多くの環境で塞がれている。
const defaultSMTPPort = "587"

// PortFromEnv は PORT だけを読む。未設定なら既定値を返す。
//
// Load を通さずにポートだけが要る経路のために分けてある（-healthcheck）。
// ヘルスチェックは動いているサーバと同じコンテナで数秒ごとに走るため、
// DB_PATH や SESSION_SECRET まで要求すると、見たいもの（サーバが応答するか）と
// 関係のない理由で失敗する。
//
// 既定値を呼び出し側に書き写さないこと。片方だけ変えると、
// __サーバは 9000 で待ち、ヘルスチェックは 8080 を叩き続ける__ 状態になる。
func PortFromEnv(getenv func(string) string) string {
	port := strings.TrimSpace(getenv("PORT"))
	if port == "" {
		return defaultPort
	}
	return port
}

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
	cfg.Port = PortFromEnv(getenv)
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

	// ---- SMTP ----
	// 未設定なら送信をスキップして動く。メールが使えないだけで
	// システムが起動できない形にはしない。
	//
	// ただし「途中まで設定されている」のは落とす。__送れているつもりで
	// 送れていない状態が一番気付きにくい。__
	//
	// Password だけは前後の空白を削らない。鍵の一部かもしれない。
	cfg.SMTP = SMTPConfig{
		Host:     strings.TrimSpace(getenv("SMTP_HOST")),
		Port:     strings.TrimSpace(getenv("SMTP_PORT")),
		User:     strings.TrimSpace(getenv("SMTP_USER")),
		Password: getenv("SMTP_PASSWORD"),
		From:     strings.TrimSpace(getenv("SMTP_FROM")),
	}
	problems = append(problems, validateSMTP(&cfg.SMTP)...)
	if !cfg.SMTP.Enabled() {
		warnings = append(warnings, "SMTP_HOST が未設定。メールは送らずに動作する")
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

// validateSMTP は SMTP の設定を検査し、既定値を埋める。
//
// Host が空なら何も見ない。メールを使わない運用なので、
// 他の値が残っていても害はない。
func validateSMTP(c *SMTPConfig) []string {
	if c.Host == "" {
		return nil
	}

	var problems []string

	if c.Port == "" {
		c.Port = defaultSMTPPort
	}
	if n, err := strconv.Atoi(c.Port); err != nil || n < 1 || n > 65535 {
		problems = append(problems, fmt.Sprintf("SMTP_PORT が不正: %q", c.Port))
	}

	// 差出人が無いと多くのサーバは受け取らない。受け取られないことは
	// 実際に送るまで分からないので、起動時に落とす。
	switch {
	case c.From == "":
		problems = append(problems, "SMTP_HOST があるのに SMTP_FROM が未設定")
	default:
		if _, err := mail.ParseAddress(c.From); err != nil {
			problems = append(problems, fmt.Sprintf("SMTP_FROM が不正: %q", c.From))
		}
	}

	// 片方だけの認証情報は、ほぼ確実に書き忘れ。
	// 認証を要求するサーバに無認証で繋ぎに行き、送信時に初めて失敗する。
	if (c.User == "") != (c.Password == "") {
		problems = append(problems, "SMTP_USER と SMTP_PASSWORD は両方設定するか、両方空にする")
	}

	return problems
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
