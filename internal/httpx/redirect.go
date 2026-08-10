package httpx

import (
	"net/url"
	"strings"
)

// DefaultRedirect は復帰先が使えない時の行き先。
const DefaultRedirect = "/"

// SafeRedirectPath は復帰先として安全なパスだけを通す。
//
// QRから来た未ログインの利用者は /login?next=/i/0042 に送られ、認証後に
// 元の備品ページへ戻る。ここで戻せないと、もう一度QRを読み直させることになり、
// その一手間が記録漏れの直接原因になる。
//
// 一方、渡された値をそのまま使うと、細工したリンクで外部サイトへ飛ばせる
// （オープンリダイレクト）。「ログイン画面に見えるページ」へ送られると、
// 利用者はそこにIDとパスワードを入力する。
//
// そのため「自サイト内の相対パス」だけを通し、判断が付かないものは
// 全て / に落とす。迷ったら弾く方に倒すこと。復帰できない不便より、
// 認証情報を渡してしまう方が重い。
func SafeRedirectPath(next string) string {
	if next == "" {
		return DefaultRedirect
	}

	// 制御文字を含むものは弾く。改行が入ると、そのまま Location ヘッダに
	// 使った時にヘッダを増やされる。
	if strings.ContainsFunc(next, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return DefaultRedirect
	}

	// 自サイト内に限る。http://evil.com のような絶対URLはここで落ちる。
	if !strings.HasPrefix(next, "/") {
		return DefaultRedirect
	}

	// //evil.com はスキーム相対URL。ブラウザは外部サイトとして解釈する。
	if strings.HasPrefix(next, "//") {
		return DefaultRedirect
	}

	// /\evil.com も同じ。バックスラッシュをスラッシュとして扱うブラウザがあり、
	// //evil.com と同じ意味になる。
	if strings.HasPrefix(next, `/\`) {
		return DefaultRedirect
	}

	// ここまでの判定を抜けても、解釈するとホストが現れる形が残り得る。
	// 最後に標準の解析器に確認させる。
	u, err := url.Parse(next)
	if err != nil || u.Scheme != "" || u.Host != "" {
		return DefaultRedirect
	}

	return next
}
