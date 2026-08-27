package label

import (
	"bytes"
	"testing"

	"github.com/Rengemaru/equipment-management/assets"
)

// 改行変換や取得の失敗で中身が壊れたTTFでも、go:embed は黙って通る。
// fpdf に渡す前に、TrueType として妥当な形かを見る。
func TestフォントがTrueTypeとして埋め込まれている(t *testing.T) {
	if len(assets.NotoSansJP) < 1<<20 {
		t.Fatalf("フォントが %dバイト しかない。日本語TTFとして小さすぎる", len(assets.NotoSansJP))
	}

	// sfnt のバージョン。OTTO なら CFF アウトラインで、fpdf では使えない。
	if got := assets.NotoSansJP[:4]; !bytes.Equal(got, []byte{0x00, 0x01, 0x00, 0x00}) {
		t.Errorf("先頭4バイト = %x。TrueType の 00010000 を期待", got)
	}
}

// 日本語フォントを登録せずに品名を書くと、文字化けするか何も出ないまま
// 「成功」して印刷まで進む。登録できていることをPDF生成の成否で見る。
func TestNewPDF_日本語を書いてもエラーにならない(t *testing.T) {
	pdf := newPDF()
	pdf.AddPage()
	pdf.SetFont(fontFamily, "", 12)
	pdf.CellFormat(100, 10, "三脚（大）／棚A-1／0042", "", 1, "L", false, 0, "")

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		t.Fatalf("PDFを出力できない: %v", err)
	}
	if err := pdf.Error(); err != nil {
		t.Fatalf("PDFの組み立てで失敗: %v", err)
	}

	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF")) {
		t.Errorf("PDFになっていない: %q", buf.Bytes()[:min(16, buf.Len())])
	}

	// 字形そのものがPDFに入っていること。/FontFile2 が無いと、
	// 印刷する端末にフォントがあるかどうかで結果が変わる。
	// 大きさで測らないのは、fpdf が使った字だけを抜き出して埋めるため
	// （13文字なら数KBにしかならず、閾値に意味を持たせられない）。
	if !bytes.Contains(buf.Bytes(), []byte("/FontFile2")) {
		t.Error("フォントがPDFに埋め込まれていない（/FontFile2 が無い）")
	}

	// 日本語は1バイトに収まらない。Identity-H で2バイトのまま扱えていること。
	if !bytes.Contains(buf.Bytes(), []byte("/Identity-H")) {
		t.Error("Identity-H になっていない。日本語を1バイト文字として扱う疑い")
	}
}

// 上のテストが意味を持つことの確認。
//
// fpdf が未登録のフォント名を黙って受け流すなら、登録し忘れても
// テストが通ってしまう。エラーになることを確かめておく。
func TestNewPDF_登録していないフォント名はエラーになる(t *testing.T) {
	pdf := newPDF()
	pdf.AddPage()
	pdf.SetFont("登録していないフォント", "", 12)

	if pdf.Error() == nil {
		t.Fatal("未登録のフォントでエラーにならない。登録の有無をPDF生成では確かめられない")
	}
}
