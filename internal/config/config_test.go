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
	if len(warnings) != 0 {
		t.Errorf("警告が出ている: %v", warnings)
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
