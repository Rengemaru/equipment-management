package notify

import (
	"context"
	"errors"
	"mime"
	"net/mail"
	"net/smtp"
	"strings"
	"testing"
	"time"
)

// capture は sendMail に渡された引数を記録する。
//
// 実際に接続してしまうと、テストがネットワークとメールサーバに依存する。
type capture struct {
	called  bool
	addr    string
	from    string
	to      []string
	msg     []byte
	hasAuth bool

	// sendErr を入れると、送信が失敗したことにできる。
	sendErr error
}

func (c *capture) send(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
	c.called = true
	c.addr = addr
	c.from = from
	c.to = to
	c.msg = msg
	c.hasAuth = a != nil
	return c.sendErr
}

// stub は sendMail と now を差し替え、記録用の器を返す。
func stub(t *testing.T) *capture {
	t.Helper()

	c := &capture{}

	origSend := sendMail
	sendMail = c.send
	t.Cleanup(func() { sendMail = origSend })

	// Date ヘッダを固定する。時刻が入ると内容を突き合わせられない。
	origNow := now
	now = func() time.Time { return time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { now = origNow })

	return c
}

// header は組み立てたメールからヘッダを1つ読む。
func (c *capture) header(t *testing.T, name string) string {
	t.Helper()

	m, err := mail.ReadMessage(strings.NewReader(string(c.msg)))
	if err != nil {
		t.Fatalf("mail.ReadMessage: %v\n%s", err, c.msg)
	}
	return m.Header.Get(name)
}

// body は組み立てたメールの本文を読む。
func (c *capture) body(t *testing.T) string {
	t.Helper()

	_, rest, found := strings.Cut(string(c.msg), "\r\n\r\n")
	if !found {
		t.Fatalf("ヘッダと本文の区切りがない:\n%s", c.msg)
	}
	return rest
}

// enabled は送信先の設定が揃った Options を返す。
func enabled() Options {
	return Options{
		Host: "smtp.example.test",
		Port: "587",
		From: "備品管理 <noreply@example.test>",
	}
}

func TestSend_SMTPが未設定なら送らずに成功する(t *testing.T) {
	c := stub(t)

	// Host が空。これが「メールを使わない運用」の形。
	m := New(Options{From: "noreply@example.test"})

	if err := m.Send(context.Background(), []string{"a@example.test"}, "件名", "本文"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if c.called {
		t.Error("送信先が未設定なのに送ろうとしている")
	}
	if m.Enabled() {
		t.Error("Enabled = true。Host が空なら false")
	}
}

func TestSend_接続先と封筒を組み立てる(t *testing.T) {
	c := stub(t)

	err := New(enabled()).Send(context.Background(), []string{"a@example.test", "b@example.test"}, "件名", "本文")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	if c.addr != "smtp.example.test:587" {
		t.Errorf("addr = %q", c.addr)
	}
	// 封筒には表示名を入れない。多くのサーバが拒否する。
	if c.from != "noreply@example.test" {
		t.Errorf("from = %q。表示名を含めない", c.from)
	}
	if len(c.to) != 2 || c.to[0] != "a@example.test" || c.to[1] != "b@example.test" {
		t.Errorf("to = %v", c.to)
	}
}

func TestSend_日本語の件名を符号化する(t *testing.T) {
	c := stub(t)

	if err := New(enabled()).Send(context.Background(), []string{"a@example.test"}, "借用が登録されました", "本文"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// 生の UTF-8 がヘッダに出ると、受信側で文字化けする。
	if strings.Contains(string(c.msg), "借用が登録されました") {
		t.Error("件名が符号化されずに生で入っている")
	}

	var dec mime.WordDecoder
	got, err := dec.DecodeHeader(c.header(t, "Subject"))
	if err != nil {
		t.Fatalf("DecodeHeader: %v", err)
	}
	if got != "借用が登録されました" {
		t.Errorf("Subject = %q", got)
	}
}

func TestSend_件名の改行でヘッダを注入できない(t *testing.T) {
	c := stub(t)

	// 件名には利用者名や備品名が入る。改行を混ぜられると別のヘッダを足せる。
	// ASCII だけの件名は符号化されずに出るため、ここが素通りすると実際に通る。
	subject := "hello\r\nBcc: attacker@example.test"

	if err := New(enabled()).Send(context.Background(), []string{"a@example.test"}, subject, "本文"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if got := c.header(t, "Bcc"); got != "" {
		t.Errorf("Bcc = %q。ヘッダが注入されている:\n%s", got, c.msg)
	}
}

func TestSend_本文の改行をCRLFに揃える(t *testing.T) {
	c := stub(t)

	if err := New(enabled()).Send(context.Background(), []string{"a@example.test"}, "件名", "1行目\n2行目"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if got := c.body(t); got != "1行目\r\n2行目" {
		t.Errorf("body = %q", got)
	}
}

func TestSend_利用者名が無ければ認証しない(t *testing.T) {
	c := stub(t)

	// 学内のリレーは認証を要求しないことが多い。
	if err := New(enabled()).Send(context.Background(), []string{"a@example.test"}, "件名", "本文"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if c.hasAuth {
		t.Error("SMTP_USER が空なのに認証しようとしている")
	}
}

func TestSend_利用者名があれば認証する(t *testing.T) {
	c := stub(t)

	opts := enabled()
	opts.User = "mailer"
	opts.Password = "secret"

	if err := New(opts).Send(context.Background(), []string{"a@example.test"}, "件名", "本文"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !c.hasAuth {
		t.Error("SMTP_USER があるのに認証していない")
	}
}

func TestSend_宛先が1人もいなければErrNoRecipient(t *testing.T) {
	c := stub(t)

	// メールアドレスは任意項目。未設定の人だけが対象になることは普通に起きる。
	err := New(enabled()).Send(context.Background(), []string{"", "  "}, "件名", "本文")
	if !errors.Is(err, ErrNoRecipient) {
		t.Fatalf("err = %v。ErrNoRecipient を期待", err)
	}
	if c.called {
		t.Error("宛先が無いのに送ろうとしている")
	}
}

func TestSend_不正な宛先を弾く(t *testing.T) {
	c := stub(t)

	if err := New(enabled()).Send(context.Background(), []string{"not-an-address"}, "件名", "本文"); err == nil {
		t.Fatal("不正な宛先が通っている")
	}
	if c.called {
		t.Error("検査に落ちたのに送ろうとしている")
	}
}

func TestSend_不正な差出人を弾く(t *testing.T) {
	c := stub(t)

	opts := enabled()
	opts.From = "not-an-address"

	if err := New(opts).Send(context.Background(), []string{"a@example.test"}, "件名", "本文"); err == nil {
		t.Fatal("不正な差出人が通っている")
	}
	if c.called {
		t.Error("検査に落ちたのに送ろうとしている")
	}
}

func TestSend_取り消された文脈では送らない(t *testing.T) {
	c := stub(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := New(enabled()).Send(ctx, []string{"a@example.test"}, "件名", "本文"); err == nil {
		t.Fatal("取り消されているのに成功している")
	}
	if c.called {
		t.Error("取り消されているのに送ろうとしている")
	}
}

func TestSend_送信の失敗を包んで返す(t *testing.T) {
	c := stub(t)
	c.sendErr = errors.New("接続できない")

	err := New(enabled()).Send(context.Background(), []string{"a@example.test"}, "件名", "本文")
	if err == nil {
		t.Fatal("失敗が握りつぶされている")
	}
	// 原因を握りつぶすと、設定が悪いのか相手が落ちているのか分からなくなる。
	if !strings.Contains(err.Error(), "接続できない") {
		t.Errorf("err = %v。原因が残っていない", err)
	}
}
