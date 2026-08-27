package label

import (
	"bytes"
	"errors"
	"image/png"
	"testing"

	"github.com/skip2/go-qrcode"
)

// QRに焼き付くURLの形。貼り替えられないラベルに載るため、
// ここが変わるということは既存のラベルが全部使えなくなるということ。
func TestItemURL_ホストと備品コードをつなぐ(t *testing.T) {
	got := ItemURL("https://binmgr.example.ac.jp", "0042")
	want := "https://binmgr.example.ac.jp/i/0042"

	if got != want {
		t.Errorf("ItemURL = %q。%q を期待", got, want)
	}
}

// HOST_URL は config が検査済みだが、ここを通ると `//i/0042` が
// ラベルに焼き付き、印刷後には直せない。
func TestItemURL_末尾のスラッシュを重ねない(t *testing.T) {
	got := ItemURL("https://binmgr.example.ac.jp/", "0042")
	want := "https://binmgr.example.ac.jp/i/0042"

	if got != want {
		t.Errorf("ItemURL = %q。%q を期待", got, want)
	}
}

// 誤り訂正レベルは M。汚れる環境なので L は避け、H はセルが細かくなって
// 70mm幅のラベルで読みにくくなる（m1-spec §6）。
func TestNewQR_誤り訂正レベルはM(t *testing.T) {
	qr, err := newQR("https://example.test", "0042")
	if err != nil {
		t.Fatalf("newQR: %v", err)
	}

	if qr.Level != qrcode.Medium {
		t.Errorf("Level = %v。qrcode.Medium を期待", qr.Level)
	}
	if qr.Content != "https://example.test/i/0042" {
		t.Errorf("Content = %q", qr.Content)
	}
}

// 空のまま通すと、ホストのトップに飛ぶだけのQRが印刷される。
// 貼ってから気付いても剥がせない。
func TestNewQR_備品コードが空なら作らない(t *testing.T) {
	for _, code := range []string{"", "   "} {
		if _, err := newQR("https://example.test", code); !errors.Is(err, ErrEmptyCode) {
			t.Errorf("code = %q のとき err = %v。ErrEmptyCode を期待", code, err)
		}
	}
}

func TestQRPNG_PNGとして読める(t *testing.T) {
	data, err := QRPNG("https://example.test", "0042", 256)
	if err != nil {
		t.Fatalf("QRPNG: %v", err)
	}

	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("PNGとして読めない: %v", err)
	}

	b := img.Bounds()
	if b.Dx() != b.Dy() {
		t.Errorf("正方形でない: %dx%d", b.Dx(), b.Dy())
	}
	// go-qrcode はセル数の倍数に丸めるため、指定した値ちょうどにはならない。
	// 指定を超えないことと、潰れていないことを見る。
	if b.Dx() == 0 || b.Dx() > 256 {
		t.Errorf("一辺 = %dpx。0 < size <= 256 を期待", b.Dx())
	}
}

// 備品コードが中身に入っていること。同じ絵が出るなら、
// どのラベルを読んでも同じ備品に飛ぶことになる。
func TestQRPNG_備品コードごとに異なる(t *testing.T) {
	a, err := QRPNG("https://example.test", "0042", 256)
	if err != nil {
		t.Fatalf("QRPNG: %v", err)
	}
	b, err := QRPNG("https://example.test", "0043", 256)
	if err != nil {
		t.Fatalf("QRPNG: %v", err)
	}

	if bytes.Equal(a, b) {
		t.Error("別の備品コードで同じ画像が出た")
	}
}

func TestQRPNG_備品コードが空なら作らない(t *testing.T) {
	if _, err := QRPNG("https://example.test", "", 256); !errors.Is(err, ErrEmptyCode) {
		t.Errorf("err = %v。ErrEmptyCode を期待", err)
	}
}
