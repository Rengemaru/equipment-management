// Package notify はメールを送る。
//
// __SMTP が未設定でも動く。__ 送信先が設定されていなければ、送らずに成功を返す。
// 学内SMTPの可否が未確定なため、メールを前提にすると、使えなかった時点で
// システム全体が起動できなくなる。認証はメールに依存しない設計（M1）で、
// メールは「送れる人にだけ送る」補助手段として扱う
// （docs/m1-implementation-spec.md §4）。
//
// __送信の失敗で呼び出し側の処理を巻き戻さないこと。__ 代理登録の通知が
// 送れなかったからといって借用の記録まで無かったことにすると、
// 記録漏れという最も避けたい状態に戻る。呼び出し側は失敗をログに残して進む。
package notify

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

// Options は SMTP の接続先。
//
// internal/config を import しない。設定の読み込みは cmd/server の仕事で、
// 内部パッケージは素の値だけを受け取る（auth の cookieSecure、
// item の hostURL と同じ形）。
type Options struct {
	// Host が空ならメールを送らない。
	Host string

	// Port は接続先ポート。
	Port string

	// User が空なら認証しない。
	User string

	// Password は User と対で使う。
	Password string

	// From は差出人。"名前 <a@example.com>" の形も受け付ける。
	From string
}

// sendMail は実際の送信。テストで差し替えるため定数ではなく var にする。
//
// net/smtp.SendMail は、相手が STARTTLS に対応していれば自動で張る。
var sendMail = smtp.SendMail

// now は Date ヘッダに入れる時刻。テストで固定するため var にする。
var now = time.Now

// ErrNoRecipient は宛先が1人もいない。
//
// メールアドレスは任意項目（users.email は NULL 可）なので、
// 「送る相手がいない」は異常ではなく普通に起きる。呼び出し側は
// errors.Is で拾って黙って進めてよい。
var ErrNoRecipient = errors.New("宛先がない")

// Mailer はメールを送る。
type Mailer struct {
	opts Options
}

// New は Mailer を作る。
//
// Host が空でも失敗しない。「送らない Mailer」になる。
// 起動時に失敗させると、メールが使えない環境でシステムが動かせなくなる。
func New(opts Options) *Mailer {
	return &Mailer{opts: opts}
}

// Enabled は送信先が設定されているか。
//
// 起動時のログの文言を分けるために公開している。__送るかどうかの判断は
// Send の中で行う__ので、呼び出し側でこれを見て分岐する必要はない。
// 各所に「設定されていれば送る」と書くと、書き忘れた経路が必ずできる。
func (m *Mailer) Enabled() bool { return m.opts.Host != "" }

// Send はメールを送る。
//
// 送信先が未設定なら、何もせずに nil を返す。
//
// ctx は net/smtp が対応していないため送信の中断には使えない。
// 開始前に取り消されていないかだけを見る。
func (m *Mailer) Send(ctx context.Context, to []string, subject, body string) error {
	if !m.Enabled() {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	addrs, err := recipients(to)
	if err != nil {
		return err
	}

	msg, err := m.build(addrs, subject, body)
	if err != nil {
		return err
	}

	if err := sendMail(net.JoinHostPort(m.opts.Host, m.opts.Port), m.auth(), m.envelopeFrom(), addrs, msg); err != nil {
		return fmt.Errorf("メールの送信: %w", err)
	}

	return nil
}

// auth は認証情報。User が空なら nil を返す（認証しない）。
//
// 部室に置く学内のリレーは認証を要求しないことが多い。認証を必須にすると、
// __一番ありそうな構成で送れなくなる。__
func (m *Mailer) auth() smtp.Auth {
	if m.opts.User == "" {
		return nil
	}
	return smtp.PlainAuth("", m.opts.User, m.opts.Password, m.opts.Host)
}

// envelopeFrom は封筒（MAIL FROM）に入れる差出人。
//
// 表示名を含む形は封筒では使えない。ヘッダとは別物として扱う。
func (m *Mailer) envelopeFrom() string {
	a, err := mail.ParseAddress(m.opts.From)
	if err != nil {
		// build が先に同じ検査で弾くため、ここには来ない。
		return m.opts.From
	}
	return a.Address
}

// build はメール本体を組み立てる。
func (m *Mailer) build(to []string, subject, body string) ([]byte, error) {
	from, err := mail.ParseAddress(m.opts.From)
	if err != nil {
		return nil, fmt.Errorf("差出人が不正: %q", m.opts.From)
	}

	var b strings.Builder

	writeHeader(&b, "From", from.String())
	writeHeader(&b, "To", strings.Join(to, ", "))
	writeHeader(&b, "Subject", mime.BEncoding.Encode("UTF-8", sanitizeHeader(subject)))
	writeHeader(&b, "Date", now().Format(time.RFC1123Z))
	writeHeader(&b, "MIME-Version", "1.0")
	writeHeader(&b, "Content-Type", `text/plain; charset="UTF-8"`)
	writeHeader(&b, "Content-Transfer-Encoding", "8bit")

	// ヘッダと本文の区切りは空行。
	b.WriteString("\r\n")
	b.WriteString(toCRLF(body))

	return []byte(b.String()), nil
}

// writeHeader は1行のヘッダを書く。行末は CRLF。
//
// LF だけで送ると受け取らないサーバがある。SMTP の行末は CRLF と決まっている。
func writeHeader(b *strings.Builder, name, value string) {
	b.WriteString(name)
	b.WriteString(": ")
	b.WriteString(value)
	b.WriteString("\r\n")
}

// sanitizeHeader はヘッダ値から改行を落とす。
//
// __件名には利用者名や備品名が入る。__ そこに改行を混ぜられると、
// 別のヘッダや本文を注入できる。値を作る側を信用しない。
//
// 日本語の件名は BEncoding が符号化するので改行も潰れるが、
// ASCII だけの件名は符号化されずにそのまま出る。ここで落としておく。
func sanitizeHeader(v string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(v)
}

// toCRLF は本文の改行を CRLF に揃える。
func toCRLF(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", "\r\n")
}

// recipients は宛先を検査し、封筒に入れる形に揃える。
//
// 空文字は飛ばす。メールアドレスは任意項目なので、未設定の人が
// 宛先の配列に混ざること自体は異常ではない。
func recipients(to []string) ([]string, error) {
	addrs := make([]string, 0, len(to))

	for _, raw := range to {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}

		a, err := mail.ParseAddress(raw)
		if err != nil {
			return nil, fmt.Errorf("宛先が不正: %q", raw)
		}
		addrs = append(addrs, a.Address)
	}

	if len(addrs) == 0 {
		return nil, ErrNoRecipient
	}

	return addrs, nil
}
