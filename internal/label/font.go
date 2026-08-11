package label

import (
	"github.com/go-pdf/fpdf"

	"github.com/Rengemaru/equipment-management/assets"
)

// fontFamily は fpdf に登録する日本語フォントの名前。
//
// SetFont に渡す名前で、フォントファイル名とは独立。ここを変えると
// 呼び出し側も全て変わるため、定数で1箇所に閉じる。
const fontFamily = "NotoSansJP"

// newPDF はラベル用のPDFを、日本語フォントを登録した状態で作る。
//
// # 必ずここを通す
//
// fpdf は既定で Latin-1 のフォントしか持たない。日本語フォントを登録せずに
// 品名を書くと、文字化けするか何も出ないまま「成功」して印刷まで進む。
// PDFを作る経路をこの関数に集約して、登録し忘れた経路を作らない。
//
// 用紙はA4縦・単位はミリメートル。ラベルシールの寸法（70 × 33.9mm）が
// ミリで決まっているため、変換を挟まない方が読みやすい。
func newPDF() *fpdf.Fpdf {
	pdf := fpdf.New("P", "mm", "A4", "")

	// バイナリに埋めたTTFをそのまま渡す。ファイルパスを渡す形にすると、
	// 実行環境にフォントを置くことが前提になり、単一バイナリで完結しなくなる。
	pdf.AddUTF8FontFromBytes(fontFamily, "", assets.NotoSansJP)

	return pdf
}
