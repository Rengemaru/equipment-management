package config

import (
	"strings"
	"testing"
)

// validEnv は起動できる最小の環境変数一式を返す。
func validEnv() map[string]string {
	return map[string]string{
		"DB_PATH":        "/data/app.db",
		"UPLOAD_DIR":     "/uploads",
		"HOST_URL":       "http://localhost:8080",
		"SESSION_SECRET": strings.Repeat("s", minSecretLen),
	}
}

// getenvFrom は map を getenv 関数に変える。
func getenvFrom(env map[string]string) func(string) string {
	return func(key string) string { return env[key] }
}

func TestLoad_必須値が揃っていれば読める(t *testing.T) {
	cfg, warnings, err := Load(getenvFrom(validEnv()))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// SMTP は必須ではないので、未設定でも起動できる。
	// ただし「メールが飛ばない理由」が起動ログから分かるよう警告は出す。
	if len(warnings) != 1 || !strings.Contains(warnings[0], "SMTP_HOST") {
		t.Errorf("警告が想定と違う: %v", warnings)
	}

	if cfg.DBPath != "/data/app.db" {
		t.Errorf("DBPath = %q", cfg.DBPath)
	}
	if cfg.UploadDir != "/uploads" {
		t.Errorf("UploadDir = %q", cfg.UploadDir)
	}
	if cfg.HostURL != "http://localhost:8080" {
		t.Errorf("HostURL = %q", cfg.HostURL)
	}
	if string(cfg.SessionSecret) != strings.Repeat("s", minSecretLen) {
		t.Error("SessionSecret が読めていない")
	}
}

// PORT だけは既定値を持つ。間違っても待ち受け先が変わるだけで、データは壊れない。
func TestLoad_PORTは未設定なら8080(t *testing.T) {
	cfg, _, err := Load(getenvFrom(validEnv()))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != "8080" {
		t.Errorf("Port = %q。8080 を期待", cfg.Port)
	}
}

// 不備は1つずつではなく全部返す。1つずつ落とすと、4つ足りない時に4回起動し直すことになる。
func TestLoad_不備をまとめて報告する(t *testing.T) {
	_, _, err := Load(getenvFrom(map[string]string{}))
	if err == nil {
		t.Fatal("エラーを期待したが nil")
	}

	msg := err.Error()
	for _, key := range []string{"DB_PATH", "UPLOAD_DIR", "HOST_URL", "SESSION_SECRET"} {
		if !strings.Contains(msg, key) {
			t.Errorf("%s が報告されていない: %v", key, msg)
		}
	}

	// 直し方まで書く。何が悪いかだけでは、初めて触る人は動かせない。
	if !strings.Contains(msg, ".env.example") {
		t.Errorf("対処方法が示されていない: %v", msg)
	}
}

func TestLoad_必須値が欠けたら起動できない(t *testing.T) {
	for _, key := range []string{"DB_PATH", "UPLOAD_DIR", "HOST_URL", "SESSION_SECRET"} {
		t.Run(key, func(t *testing.T) {
			env := validEnv()
			delete(env, key)

			_, _, err := Load(getenvFrom(env))
			if err == nil {
				t.Fatalf("%s が無いのに起動できてしまう", key)
			}
			if !strings.Contains(err.Error(), key) {
				t.Errorf("どの値が欠けているか分からない: %v", err)
			}
		})
	}
}

// 空白だけの値は未設定と同じ扱いにする。.env に "DB_PATH= " と書かれることがある。
func TestLoad_空白だけの値は未設定として扱う(t *testing.T) {
	env := validEnv()
	env["DB_PATH"] = "   "

	if _, _, err := Load(getenvFrom(env)); err == nil {
		t.Fatal("エラーを期待したが nil")
	}
}

// HOST_URL は QR に焼き込まれる。誤った値で印刷したラベルは貼り替えられない。
func TestLoad_HOST_URLの形式を検査する(t *testing.T) {
	tests := []struct {
		name string
		url  string
		ok   bool
	}{
		{"http", "http://localhost:8080", true},
		{"https", "https://equip.example.ac.jp", true},
		{"パス付き", "https://example.ac.jp/equip", true},
		{"末尾スラッシュ", "https://example.ac.jp/", false},
		{"スキームなし", "example.ac.jp", false},
		{"ホストなし", "http://", false},
		{"別スキーム", "ftp://example.ac.jp", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := validEnv()
			env["HOST_URL"] = tt.url

			_, _, err := Load(getenvFrom(env))
			if tt.ok && err != nil {
				t.Errorf("%q は通るべき: %v", tt.url, err)
			}
			if !tt.ok && err == nil {
				t.Errorf("%q は弾くべき", tt.url)
			}
		})
	}
}

func TestLoad_短いSESSION_SECRETを拒否する(t *testing.T) {
	env := validEnv()
	env["SESSION_SECRET"] = "short"

	_, _, err := Load(getenvFrom(env))
	if err == nil {
		t.Fatal("短い鍵が通ってしまう。総当たりでセッションを偽造される")
	}
	if !strings.Contains(err.Error(), "SESSION_SECRET") {
		t.Errorf("原因が分からない: %v", err)
	}
}

// 雛形の値のままでも起動はできる（開発を止めない）が、黙って通さない。
func TestLoad_雛形のSESSION_SECRETは警告する(t *testing.T) {
	env := validEnv()
	env["SESSION_SECRET"] = devSecret

	_, warnings, err := Load(getenvFrom(env))
	if err != nil {
		t.Fatalf("起動できるべき: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatal("警告が出ていない")
	}
	if !strings.Contains(warnings[0], "SESSION_SECRET") {
		t.Errorf("警告の内容が分からない: %v", warnings)
	}
}

// 既定を true にする。HTTPS本番で設定を忘れた時に Cookie が平文で流れる方が危険。
func TestLoad_COOKIE_SECUREの既定はtrue(t *testing.T) {
	cfg, _, err := Load(getenvFrom(validEnv()))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.CookieSecure {
		t.Error("CookieSecure の既定が false になっている")
	}
}

func TestLoad_COOKIE_SECUREを解釈する(t *testing.T) {
	tests := []struct {
		raw    string
		want   bool
		wantOK bool
	}{
		{"true", true, true},
		{"false", false, true},
		{"1", true, true},
		{"0", false, true},
		// 解釈できない値を false に倒さない。危険な側に倒れる。
		{"yes", false, false},
		{"はい", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			env := validEnv()
			env["COOKIE_SECURE"] = tt.raw

			cfg, _, err := Load(getenvFrom(env))
			if !tt.wantOK {
				if err == nil {
					t.Fatalf("%q は弾くべき", tt.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.CookieSecure != tt.want {
				t.Errorf("CookieSecure = %v。%v を期待", cfg.CookieSecure, tt.want)
			}
		})
	}
}

func TestLoad_PORTの形式を検査する(t *testing.T) {
	for _, raw := range []string{"abc", "0", "70000", "-1"} {
		t.Run(raw, func(t *testing.T) {
			env := validEnv()
			env["PORT"] = raw

			if _, _, err := Load(getenvFrom(env)); err == nil {
				t.Errorf("PORT=%q が通ってしまう", raw)
			}
		})
	}
}

// PortFromEnv は Load を通さずに呼ばれる（-healthcheck）。
// Load 側の既定値と食い違うと、サーバが待つポートとヘルスチェックが叩く
// ポートがずれ、健全なのに unhealthy と報告され続ける。
func TestPortFromEnv_Loadと同じ値を返す(t *testing.T) {
	for _, raw := range []string{"", "  ", "8080", "9000"} {
		env := validEnv()
		env["PORT"] = raw

		cfg, _, err := Load(getenvFrom(env))
		if err != nil {
			t.Fatalf("PORT=%q: Load: %v", raw, err)
		}
		if got := PortFromEnv(getenvFrom(env)); got != cfg.Port {
			t.Errorf("PORT=%q: PortFromEnv = %q, Load = %q", raw, got, cfg.Port)
		}
	}
}

// ---- SMTP ----
//
// 未設定なら送信をスキップして動く（CLAUDE.md のM2タスク）。
// 一方で「途中まで設定されている」は落とす。送れているつもりで
// 送れていない状態が一番気付きにくい。

// smtpEnv は SMTP を有効にした環境変数一式を返す。
func smtpEnv() map[string]string {
	env := validEnv()
	env["SMTP_HOST"] = "smtp.example.test"
	env["SMTP_FROM"] = "noreply@example.test"
	return env
}

func TestLoad_SMTPが未設定でも起動できる(t *testing.T) {
	cfg, _, err := Load(getenvFrom(validEnv()))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SMTP.Enabled() {
		t.Error("SMTP_HOST が無いのに Enabled = true")
	}
}

func TestLoad_SMTPが揃っていれば有効になる(t *testing.T) {
	cfg, warnings, err := Load(getenvFrom(smtpEnv()))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.SMTP.Enabled() {
		t.Fatal("Enabled = false")
	}
	if cfg.SMTP.Host != "smtp.example.test" {
		t.Errorf("Host = %q", cfg.SMTP.Host)
	}
	// 未設定なら submission ポート。25 は多くの環境で塞がれている。
	if cfg.SMTP.Port != defaultSMTPPort {
		t.Errorf("Port = %q。既定の %q を期待", cfg.SMTP.Port, defaultSMTPPort)
	}
	if len(warnings) != 0 {
		t.Errorf("警告が出ている: %v", warnings)
	}
}

func TestLoad_SMTP_FROMが無ければ落とす(t *testing.T) {
	env := smtpEnv()
	delete(env, "SMTP_FROM")

	if _, _, err := Load(getenvFrom(env)); err == nil {
		t.Fatal("差出人が無いまま起動できてしまう")
	}
}

func TestLoad_SMTP_FROMの形式を検査する(t *testing.T) {
	env := smtpEnv()
	env["SMTP_FROM"] = "not-an-address"

	if _, _, err := Load(getenvFrom(env)); err == nil {
		t.Fatal("不正な差出人が通ってしまう")
	}
}

func TestLoad_SMTP_PORTの形式を検査する(t *testing.T) {
	for _, raw := range []string{"0", "70000", "abc", "-1"} {
		env := smtpEnv()
		env["SMTP_PORT"] = raw

		if _, _, err := Load(getenvFrom(env)); err == nil {
			t.Errorf("SMTP_PORT=%q が通ってしまう", raw)
		}
	}
}

func TestLoad_認証情報は片方だけだと落とす(t *testing.T) {
	// 認証を要求するサーバに無認証で繋ぎに行き、送信時に初めて失敗する。
	// 起動時に気付ける方がよい。
	only := []map[string]string{
		{"SMTP_USER": "mailer"},
		{"SMTP_PASSWORD": "secret"},
	}

	for _, over := range only {
		env := smtpEnv()
		for k, v := range over {
			env[k] = v
		}

		if _, _, err := Load(getenvFrom(env)); err == nil {
			t.Errorf("%v だけで通ってしまう", over)
		}
	}
}

func TestLoad_認証情報が両方あれば通る(t *testing.T) {
	env := smtpEnv()
	env["SMTP_USER"] = "mailer"
	env["SMTP_PASSWORD"] = "secret"

	cfg, _, err := Load(getenvFrom(env))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SMTP.User != "mailer" || cfg.SMTP.Password != "secret" {
		t.Errorf("User = %q, Password = %q", cfg.SMTP.User, cfg.SMTP.Password)
	}
}

func TestLoad_SMTPを使わないなら残った値を見ない(t *testing.T) {
	// メールをやめた時に、SMTP_HOST だけコメントアウトして
	// 他を残すことは普通に起きる。それで起動できなくなると困る。
	env := validEnv()
	env["SMTP_PORT"] = "これは不正な値"
	env["SMTP_USER"] = "mailer"

	if _, _, err := Load(getenvFrom(env)); err != nil {
		t.Fatalf("SMTP_HOST が無いのに落ちた: %v", err)
	}
}
