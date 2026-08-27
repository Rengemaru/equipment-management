// Package label は備品ラベル（QR + 備品コード + 品名）の生成を扱う。
//
// ラベルは一度貼ったら貼り替えられない。ここで作るものは後から直せないため、
// 埋め込むURLも誤り訂正レベルも、実物を印刷して読めることを確かめてから変えること。
package label

import (
	"errors"
	"strings"

	"github.com/skip2/go-qrcode"
)

// ErrEmptyCode は備品コードが空であること。
//
// 空のまま通すと、ホストのトップに飛ぶだけのQRが印刷される。
// 貼ってから気付いても剥がせない。
var ErrEmptyCode = errors.New("備品コードが空")

// ItemURL はQRに埋め込むURLを組み立てる。
//
// `{host}/i/{code}` の形（url-design.md §1）。`/i/` と短くしてあるのは、
// URLが短いほどQRのセル数が減り、小さいラベルでも読めるため。
// **この形は後から変更できない。** 貼り替えられないラベルに焼き付く。
func ItemURL(hostURL, code string) string {
	// 末尾のスラッシュを落とす。HOST_URL は config が検査済みだが、
	// ここを通ると `//i/0042` のようなURLがラベルに焼き付き、印刷後に直せない。
	// 二重に見るだけの価値がある。
	return strings.TrimRight(hostURL, "/") + "/i/" + code
}

// QRPNG は備品のQRをPNGで返す。
//
// size は画像の一辺のピクセル数。負の値を渡すと1セルあたりのピクセル数として
// 扱われる（go-qrcode の仕様）。印刷解像度に合わせるのは呼び出し側の仕事。
func QRPNG(hostURL, code string, size int) ([]byte, error) {
	qr, err := newQR(hostURL, code)
	if err != nil {
		return nil, err
	}

	return qr.PNG(size)
}

// newQR はQRを組み立てる。
//
// # 誤り訂正レベルは M
//
// 部室で使われ、工具箱や機材に貼られて汚れる前提のため L は避ける。
// かといって H にするとセルが細かくなり、70mm幅のラベルでは
// スマートフォンの標準カメラで読みにくくなる（m1-spec §6）。
//
// # 余白を消さない
//
// go-qrcode は既定でクワイエットゾーン（周囲の余白）を付ける。これを消すと
// 背景と一体化して読み取れなくなる。ラベルが小さいからと削らないこと。
func newQR(hostURL, code string) (*qrcode.QRCode, error) {
	if strings.TrimSpace(code) == "" {
		return nil, ErrEmptyCode
	}

	return qrcode.New(ItemURL(hostURL, code), qrcode.Medium)
}
