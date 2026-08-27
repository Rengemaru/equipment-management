// Package assets はバイナリに同梱する静的ファイルを持つ。
//
// ここに置くのは「実行時に外部から差し替わってはいけないもの」だけ。
// デプロイを「バイナリ1つ + SQLiteファイル1つ」で完結させる方針のため、
// 実行環境にファイルを置くことを前提にしない（CLAUDE.md）。
package assets

import _ "embed"

// NotoSansJP はラベルPDFに使う日本語フォント（Noto Sans JP Regular、SIL OFL 1.1）。
//
// # なぜバイナリに埋めるか
//
// ラベルには品名を印字するため、日本語フォントの埋め込みが必須。これを忘れると
// 品名が全て文字化けするか空白になる。Dockerイメージ側にフォントを置く方式は、
// イメージを差し替えた拍子に消えて壊れるので採らない（CLAUDE.md）。
//
// # 差し替える時の条件
//
// fpdf の AddUTF8FontFromBytes は TrueType アウトライン（glyf テーブル）を持つ
// 静的な TTF しか扱えない。次はいずれも使えないので注意すること。
//
//   - OTF（CFF アウトライン）。Noto Sans CJK の配布物の多くはこれで、
//     拡張子を .ttf に変えても動かない
//   - バリアブルフォント（fvar テーブルを持つもの）。
//     Google Fonts の NotoSansJP[wght].ttf はこれ
//
// 入手元とライセンスは fonts/LICENSE-NotoSansJP.txt に残してある。
//
//go:embed fonts/NotoSansJP.ttf
var NotoSansJP []byte
