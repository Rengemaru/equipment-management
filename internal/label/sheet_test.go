package label

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
)

const testHost = "https://example.test"

// 面付けは用紙が決めている。ここがずれると、刷った全てのラベルが
// 台紙からはみ出す。数字を触ったときに気付けるようにしておく。
func Test面付けがA4に収まる(t *testing.T) {
	const (
		a4W = 210.0
		a4H = 297.0
	)

	if got := sheetMarginX*2 + sheetCols*labelW; got != a4W {
		t.Errorf("横 = %.1fmm、A4の幅 %.1fmm と合わない", got, a4W)
	}
	if got := sheetMarginY*2 + sheetRows*labelH; got != a4H {
		t.Errorf("縦 = %.1fmm、A4の高さ %.1fmm と合わない", got, a4H)
	}

	// 右下のラベルまで用紙の内側にあること。
	last := sheetCols*sheetRows - 1
	x := sheetMarginX + float64(last%sheetCols)*labelW
	y := sheetMarginY + float64(last/sheetCols)*labelH
	if x+labelW > a4W || y+labelH > a4H {
		t.Errorf("最後のラベルの右下 = (%.1f, %.1f) が用紙の外", x+labelW, y+labelH)
	}
}

// QRと文字がラベルの中に収まること。はみ出すと隣のラベルに印字される。
func Testラベルの中身が枠に収まる(t *testing.T) {
	if qrSize > labelH {
		t.Errorf("QR %.1fmm がラベル高 %.1fmm に入らない", qrSize, labelH)
	}
	if got := contentPadX + qrSize + qrGap + textW + contentPadX; got > labelW {
		t.Errorf("横の合計 = %.1fmm、ラベル幅 %.1fmm を超える", got, labelW)
	}
	if textW <= 0 {
		t.Errorf("文字の幅が %.1fmm。品名を置く場所が無い", textW)
	}
}

func TestPDF_ラベルが1枚でもPDFになる(t *testing.T) {
	out, err := PDF(testHost, []Label{{Code: "0042", Name: "三脚（大）"}})
	if err != nil {
		t.Fatalf("PDF: %v", err)
	}

	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Errorf("PDFになっていない: %q", out[:min(16, len(out))])
	}
	// 品名の字形が埋め込まれていること。無いと、印刷する端末に
	// フォントがあるかどうかで刷り上がりが変わる。
	if !bytes.Contains(out, []byte("/FontFile2")) {
		t.Error("フォントがPDFに埋め込まれていない（/FontFile2 が無い）")
	}
	// QRは画像として貼る。無ければ文字だけのラベルが刷られる。
	if !bytes.Contains(out, []byte("/Image")) {
		t.Error("QR画像がPDFに入っていない")
	}
}

func TestBuildSheet_24枚を超えるとページが増える(t *testing.T) {
	tests := []struct {
		count int
		pages int
	}{
		{count: 1, pages: 1},
		{count: 24, pages: 1},
		{count: 25, pages: 2},
		{count: 48, pages: 2},
		{count: 49, pages: 3},
	}

	for _, tt := range tests {
		labels := make([]Label, tt.count)
		for i := range labels {
			labels[i] = Label{Code: fmt.Sprintf("%04d", i+1), Name: "備品"}
		}

		pdf, err := buildSheet(testHost, labels)
		if err != nil {
			t.Fatalf("%d枚: buildSheet: %v", tt.count, err)
		}
		if got := pdf.PageCount(); got != tt.pages {
			t.Errorf("%d枚 → %dページ。%dページを期待", tt.count, got, tt.pages)
		}
	}
}

// 0件で白紙のシートを刷ると、ラベルシールが1枚無駄になる。
func TestPDF_対象が無ければエラー(t *testing.T) {
	if _, err := PDF(testHost, nil); !errors.Is(err, ErrNoLabels) {
		t.Errorf("err = %v、ErrNoLabels を期待", err)
	}
}

// 備品コードが空だと、トップに飛ぶだけのQRが刷られる。
// 貼ってから気付いても剥がせない。
func TestPDF_備品コードが空ならエラー(t *testing.T) {
	labels := []Label{
		{Code: "0001", Name: "正常"},
		{Code: "", Name: "コードが無い"},
	}

	if _, err := PDF(testHost, labels); !errors.Is(err, ErrEmptyCode) {
		t.Errorf("err = %v、ErrEmptyCode を期待", err)
	}
}

func TestWrapText(t *testing.T) {
	pdf := newPDF()
	pdf.AddPage()
	pdf.SetFont(fontFamily, "", nameFontSize)

	t.Run("収まる品名はそのまま1行", func(t *testing.T) {
		got := wrapText(pdf, "三脚（大）", textW, nameMaxLines)
		if len(got) != 1 || got[0] != "三脚（大）" {
			t.Errorf("got = %q、[三脚（大）] を期待", got)
		}
	})

	t.Run("空の品名は行を作らない", func(t *testing.T) {
		if got := wrapText(pdf, "   ", textW, nameMaxLines); got != nil {
			t.Errorf("got = %q、nil を期待", got)
		}
	})

	t.Run("長い品名は折り返す", func(t *testing.T) {
		got := wrapText(pdf, strings.Repeat("あ", 20), textW, nameMaxLines)
		if len(got) != 2 {
			t.Fatalf("got = %q、2行を期待", got)
		}
		for _, line := range got {
			if w := pdf.GetStringWidth(line); w > textW {
				t.Errorf("%q の幅 = %.1fmm、%.1fmm に収まっていない", line, w, textW)
			}
		}
	})

	t.Run("入りきらない品名は末尾を…にする", func(t *testing.T) {
		got := wrapText(pdf, strings.Repeat("あ", 100), textW, nameMaxLines)
		if len(got) != nameMaxLines {
			t.Fatalf("got = %d行、%d行を期待", len(got), nameMaxLines)
		}

		last := got[len(got)-1]
		if !strings.HasSuffix(last, "…") {
			t.Errorf("最終行 = %q。切れたことが分からない", last)
		}
		if w := pdf.GetStringWidth(last); w > textW {
			t.Errorf("最終行の幅 = %.1fmm、%.1fmm に収まっていない", w, textW)
		}
	})
}
