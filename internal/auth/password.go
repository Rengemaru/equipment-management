package auth

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// passwordAlphabet は初期パスワードに使う文字。
//
// 紛らわしい文字を外している（0 と O、1 と l と I）。
// 初期パスワードは口頭や紙で渡される前提で、読み違えると
// 「ログインできない」という問い合わせが admin に返ってくる。
// 記号も外す。スマートフォンの入力で記号への切り替えが要るだけ手間が増える。
const passwordAlphabet = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// generatedPasswordLen は生成する初期パスワードの長さ。
//
// 56文字の集合から16文字なので約93ビット。総当たりは現実的でない。
// 初回ログインで変更を強制するため、長期間使われることは想定していないが、
// 変更しないまま運用される可能性は常にある。
const generatedPasswordLen = 16

// GeneratePassword は初期パスワードを生成する。
//
// admin がユーザーを作る時と -create-admin で使う。人が考えた文字列を
// 初期パスワードにすると、全員に同じものが配られることになる。
func GeneratePassword() (string, error) {
	max := big.NewInt(int64(len(passwordAlphabet)))
	out := make([]byte, generatedPasswordLen)

	for i := range out {
		// math/rand は使わない。予測できると、発行済みの初期パスワードを
		// 順に推測できてしまう。
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("パスワードの生成: %w", err)
		}
		out[i] = passwordAlphabet[n.Int64()]
	}

	return string(out), nil
}
