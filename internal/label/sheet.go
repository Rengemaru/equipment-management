package label

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/go-pdf/fpdf"
)

// ErrNoLabels は印刷対象が1件も無いこと。
//
// 0件でも白紙のPDFは作れるが、ラベルシールは1枚単位で消費する。
// 何も印刷されないシートが1枚無駄になるより、作らない方がよい。
var ErrNoLabels = errors.New("印刷対象の備品が無い")

// Label は1枚分の印字内容。
//
// item.Item をそのまま受け取らないのは、ラベルに出すのが
// 備品コードと品名だけであることを型で示すため。所在も状態も
// 貼った翌日には変わり得るので、ラベルには焼き付けない。
type Label struct {
	Code string
	Name string
}

// A4 24面ラベルシール（70 × 33.9mm、3列×8行）の寸法。単位はミリメートル。
//
// 3列 × 70mm = 210mm でA4の幅ちょうどになるため、左右の余白は0。
// 上下は (297 - 8×33.9) / 2 = 12.9mm ずつ。
// **この値は用紙が決めている。** 印字がずれたときに動かすのは
// contentPad 以下の内側の値であって、ここではない。
const (
	sheetCols = 3
	sheetRows = 8

	labelW = 70.0
	labelH = 33.9

	sheetMarginX = 0.0
	sheetMarginY = 12.9
)

// ラベル内部の配置。
const (
	// contentPadX は左右の内側余白。
	//
	// 5mm取るのは体裁のためではない。左端の列はラベルの左辺が
	// 用紙の左端そのもので、家庭用プリンタの多くはそこから数ミリを
	// 印字できない。QRが欠けたラベルは貼ってから気付いても直せない。
	contentPadX = 5.0

	qrSize  = 26.0 // ラベル高 33.9mm に上下 3.95mm ずつの余白が残る
	qrGap   = 2.0  // QRと文字の間。詰めるとカメラがQRの終端を見失う
	textW   = labelW - contentPadX*2 - qrSize - qrGap
	textPad = contentPadX + qrSize + qrGap

	codeFontSize = 13.0
	codeLineH    = 5.5
	nameFontSize = 8.0
	nameLineH    = 4.0

	// nameMaxLines は品名の行数の上限。
	// 幅32mmでは2行が限界で、3行にすると文字を小さくするしかなくなる。
	nameMaxLines = 2
)

// qrPixelsPerModule はQR画像の1セルあたりのピクセル数。
//
// 負の値で go-qrcode に渡す。全体のピクセル数を指定すると、セル数で
// 割り切れないぶんが特定のセルだけ1px太る形で現れ、カメラが読みにくくなる。
// 26mm に 8px/セル で並べると、25セルのQRで約200px（約200dpi）。
const qrPixelsPerModule = 8

// PDF はラベルシートのPDFを組み立てて返す。
//
// labels の並び順にそのまま面付けする（左上から右へ、行が尽きたら次の行）。
// 24枚を超えるとページを足す。**並べ替えは呼び出し側の責任。**
// 備品コード順に渡せば、印刷したシートと棚を突き合わせられる。
func PDF(hostURL string, labels []Label) ([]byte, error) {
	pdf, err := buildSheet(hostURL, labels)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("ラベルPDFの出力: %w", err)
	}

	return buf.Bytes(), nil
}

// buildSheet は面付けまでを行う。PDFへの書き出しと分けてあるのは、
// テストからページ数などを直接確かめられるようにするため。
func buildSheet(hostURL string, labels []Label) (*fpdf.Fpdf, error) {
	if len(labels) == 0 {
		return nil, ErrNoLabels
	}

	pdf := newPDF()

	// 位置は全て自前で計算する。余白と自動改ページを残すと、
	// ラベルの下端に文字を置いた時点で勝手にページが送られ、
	// 以降のラベルが全て1枚ずつずれる。
	pdf.SetMargins(0, 0, 0)
	pdf.SetAutoPageBreak(false, 0)

	perPage := sheetCols * sheetRows
	for i, l := range labels {
		if i%perPage == 0 {
			pdf.AddPage()
		}

		pos := i % perPage
		x := sheetMarginX + float64(pos%sheetCols)*labelW
		y := sheetMarginY + float64(pos/sheetCols)*labelH

		if err := drawLabel(pdf, hostURL, l, x, y); err != nil {
			return nil, err
		}
	}

	if err := pdf.Error(); err != nil {
		return nil, fmt.Errorf("ラベルPDFの組み立て: %w", err)
	}

	return pdf, nil
}

// drawLabel は (x, y) を左上とする1枚を描く。枠線は引かない。
// 台紙が切れている位置と印字の枠が1mmずれるだけで、刷り上がりが汚くなる。
func drawLabel(pdf *fpdf.Fpdf, hostURL string, l Label, x, y float64) error {
	// 画像名は備品コード。中身は備品コードだけで決まる（ItemURL）ので、
	// 同じ名前なら同じQRになる。同じ備品を複数枚刷るときに、
	// 生成もPDFへの埋め込みも1回で済ませる。
	imgName := "qr-" + l.Code
	opt := fpdf.ImageOptions{ImageType: "PNG"}
	if pdf.GetImageInfo(imgName) == nil {
		png, err := QRPNG(hostURL, l.Code, -qrPixelsPerModule)
		if err != nil {
			return fmt.Errorf("QRの生成（%s）: %w", l.Code, err)
		}
		pdf.RegisterImageOptionsReader(imgName, opt, bytes.NewReader(png))
	}
	pdf.ImageOptions(imgName, x+contentPadX, y+(labelH-qrSize)/2, qrSize, qrSize, false, opt, 0, "")

	// 折り返しの判定に文字幅を測るため、先にフォントを設定する。
	pdf.SetFont(fontFamily, "", nameFontSize)
	nameLines := wrapText(pdf, l.Name, textW, nameMaxLines)

	// 文字ブロックはラベルの高さの中央に置く。品名が1行か2行かで
	// 行数が変わるため、上端固定にすると1行のラベルだけ上に寄って見える。
	blockH := codeLineH + float64(len(nameLines))*nameLineH
	textY := y + (labelH-blockH)/2

	pdf.SetFont(fontFamily, "", codeFontSize)
	pdf.SetXY(x+textPad, textY)
	pdf.CellFormat(textW, codeLineH, l.Code, "", 0, "L", false, 0, "")

	pdf.SetFont(fontFamily, "", nameFontSize)
	for i, line := range nameLines {
		pdf.SetXY(x+textPad, textY+codeLineH+float64(i)*nameLineH)
		pdf.CellFormat(textW, nameLineH, line, "", 0, "L", false, 0, "")
	}

	return nil
}

// wrapText は width に収まるように折り返す。maxLines に入りきらない分は
// 捨て、末尾を「…」にする。
//
// 単語ではなく文字で折る。品名の大半は日本語で語の区切りが無く、
// 幅も32mmしか無いため、単語単位で折ると1行に1語しか入らない場合が出る。
// 呼び出す前に、測る対象と同じフォント・同じ文字サイズを設定しておくこと。
func wrapText(pdf *fpdf.Fpdf, s string, width float64, maxLines int) []string {
	s = strings.TrimSpace(s)
	if s == "" || maxLines <= 0 {
		return nil
	}

	var (
		lines []string
		cur   []rune
	)
	for _, r := range s {
		next := append(cur, r)
		// 1文字目だけは幅を超えても置く。置かないと1文字も入らない
		// 幅を渡されたときに無限に行が増える。
		if len(cur) == 0 || pdf.GetStringWidth(string(next)) <= width {
			cur = next
			continue
		}

		if len(lines)+1 == maxLines {
			return append(lines, truncate(pdf, string(cur), width))
		}
		lines = append(lines, string(cur))
		cur = []rune{r}
	}

	return append(lines, string(cur))
}

// truncate は末尾に「…」を足しても width に収まるまで後ろを削る。
//
// 品名が切れていることを見えるようにするためのもの。何も足さずに切ると、
// 「三脚（大）」と「三脚（大）用ケース」が同じ表示になる。
func truncate(pdf *fpdf.Fpdf, s string, width float64) string {
	const ellipsis = "…"

	r := []rune(s)
	for len(r) > 0 {
		if pdf.GetStringWidth(string(r)+ellipsis) <= width {
			return string(r) + ellipsis
		}
		r = r[:len(r)-1]
	}

	return ellipsis
}
