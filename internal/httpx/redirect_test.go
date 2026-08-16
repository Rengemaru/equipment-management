package httpx

import "testing"

// 通してよい復帰先。ここが通らないと、QRから来た人が元のページに戻れない。
func TestSafeRedirectPath_自サイト内のパスを通す(t *testing.T) {
	tests := []string{
		"/",
		"/i/0042",
		"/items",
		"/items?category=工具",
		"/admin/items/import",
		"/i/0042#note",
		"/items?q=%E4%B8%89%E8%84%9A", // パーセントエンコードされた日本語
	}

	for _, next := range tests {
		t.Run(next, func(t *testing.T) {
			if got := SafeRedirectPath(next); got != next {
				t.Errorf("SafeRedirectPath(%q) = %q。そのまま通すべき", next, got)
			}
		})
	}
}

// 弾くべき復帰先。細工したリンクで外部サイトへ飛ばされると、
// ログイン画面に見えるページで認証情報を入力させられる。
func TestSafeRedirectPath_外部への誘導を弾く(t *testing.T) {
	tests := []struct {
		name string
		next string
	}{
		{"スキーム相対", "//evil.com"},
		{"スキーム相対のパス付き", "//evil.com/login"},
		{"バックスラッシュ", `/\evil.com`},
		{"バックスラッシュ2つ", `\\evil.com`},
		{"絶対URL http", "http://evil.com"},
		{"絶対URL https", "https://evil.com/login"},
		{"スキームのみ", "javascript:alert(1)"},
		{"data URL", "data:text/html,<script>alert(1)</script>"},
		{"相対パス", "items"},
		{"親ディレクトリ", "../admin"},
		{"改行入り", "/items\r\nSet-Cookie: session=x"},
		{"タブ入り", "/\t/evil.com"},
		{"空", ""},
		{"ホスト付き", "//user@evil.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SafeRedirectPath(tt.next); got != DefaultRedirect {
				t.Errorf("SafeRedirectPath(%q) = %q。%q に落とすべき",
					tt.next, got, DefaultRedirect)
			}
		})
	}
}

// 判断が付かないものは全て / に落ちること。復帰できない不便より、
// 認証情報を渡してしまう方が重い。
func TestSafeRedirectPath_必ずパスを返す(t *testing.T) {
	inputs := []string{"", "壊れた値", "%%%", "//", "/", "http://", string(rune(0))}

	for _, next := range inputs {
		got := SafeRedirectPath(next)
		if got == "" || got[0] != '/' {
			t.Errorf("SafeRedirectPath(%q) = %q。/ で始まる値を期待", next, got)
		}
	}
}
