package auth

import (
	"strings"
	"testing"
)

func TestGeneratePassword_そのまま登録できる(t *testing.T) {
	got, err := GeneratePassword()
	if err != nil {
		t.Fatalf("GeneratePassword: %v", err)
	}

	// 生成した値が自前の検査を通らないと、admin がユーザーを作れない。
	if err := ValidatePassword(got); err != nil {
		t.Errorf("生成した %q が検査を通らない: %v", got, err)
	}
	if len(got) != generatedPasswordLen {
		t.Errorf("長さ = %d。%d を期待", len(got), generatedPasswordLen)
	}
}

// 初期パスワードは口頭や紙で渡る。読み違える文字を含めない。
func TestGeneratePassword_紛らわしい文字を含まない(t *testing.T) {
	// 1000回引いて、一度も出ないことを確かめる。
	for i := 0; i < 1000; i++ {
		got, err := GeneratePassword()
		if err != nil {
			t.Fatalf("GeneratePassword: %v", err)
		}
		if strings.ContainsAny(got, "0O1lI") {
			t.Fatalf("紛らわしい文字が含まれる: %q", got)
		}
	}
}

func TestGeneratePassword_毎回違う(t *testing.T) {
	seen := make(map[string]bool)

	for i := 0; i < 100; i++ {
		got, err := GeneratePassword()
		if err != nil {
			t.Fatalf("GeneratePassword: %v", err)
		}
		if seen[got] {
			t.Fatalf("同じパスワードが2回出た: %q", got)
		}
		seen[got] = true
	}
}
