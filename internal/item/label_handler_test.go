package item

import (
	"bytes"
	"context"
	"net/http"
	"testing"
)

// 4桁を超えた時に文字列比較だと "10000" < "9999" になり、
// 新しい備品が範囲から静かに漏れる。数値で比べていることを確かめる。
func TestList_コード範囲は数値で比べる(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	insert(t, s, "0001", "三脚", nil)
	insert(t, s, "0042", "カメラ", nil)
	insert(t, s, "9999", "工具箱", nil)
	insert(t, s, "10000", "延長コード", nil)

	tests := []struct {
		name string
		f    Filter
		want []string
	}{
		{name: "範囲を指定しなければ全件", f: Filter{}, want: []string{"0001", "0042", "10000", "9999"}},
		{name: "両端を含む", f: Filter{CodeFrom: 1, CodeTo: 42}, want: []string{"0001", "0042"}},
		{name: "開始だけ", f: Filter{CodeFrom: 9999}, want: []string{"10000", "9999"}},
		{name: "終了だけ", f: Filter{CodeTo: 42}, want: []string{"0001", "0042"}},
		{name: "5桁も範囲に入る", f: Filter{CodeFrom: 43, CodeTo: 10000}, want: []string{"10000", "9999"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items, err := s.List(ctx, tt.f)
			if err != nil {
				t.Fatalf("List: %v", err)
			}

			got := make([]string, 0, len(items))
			for _, it := range items {
				got = append(got, it.Code)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got = %v、%v を期待", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("got = %v、%v を期待", got, tt.want)
				}
			}
		})
	}
}

func TestHandleLabels_PDFを返す(t *testing.T) {
	h, s := newTestHandler(t)

	insert(t, s, "0001", "三脚（大）", nil)
	insert(t, s, "0002", "はんだごて", nil)

	w := get(t, h, "/api/labels.pdf")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	if got := w.Header().Get("Content-Type"); got != "application/pdf" {
		t.Errorf("Content-Type = %q", got)
	}
	// ラベルシールは刷り直しが効かない。ビューアで確認してから
	// 印刷できるよう、ダウンロードさせない。
	if got := w.Header().Get("Content-Disposition"); got != `inline; filename="labels.pdf"` {
		t.Errorf("Content-Disposition = %q", got)
	}
	if !bytes.HasPrefix(w.Body.Bytes(), []byte("%PDF")) {
		t.Errorf("PDFになっていない: %q", w.Body.Bytes()[:min(16, w.Body.Len())])
	}
}

// 範囲や分類の指定が効いていることは、PDFの中身からは読めない
// （字形とQRは圧縮されて入る）。同じ条件で一覧を引き、対象が
// 絞れていることで確かめる。
func TestHandleLabels_範囲と分類で絞る(t *testing.T) {
	h, s := newTestHandler(t)

	insert(t, s, "0001", "三脚", map[string]any{"category": "撮影機材"})
	insert(t, s, "0002", "ドライバー", map[string]any{"category": "工具"})
	insert(t, s, "0003", "カメラ", map[string]any{"category": "撮影機材"})

	items, err := h.store.List(context.Background(), Filter{Category: "撮影機材", CodeFrom: 2, CodeTo: 3})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 || items[0].Code != "0003" {
		t.Fatalf("対象 = %d件、0003 の1件を期待", len(items))
	}

	w := get(t, h, "/api/labels.pdf?from=2&to=3&category=撮影機材")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

// ラベルに印字されたゼロ埋めの形をそのまま貼り付けても通ること。
func TestHandleLabels_ゼロ埋めの指定を受ける(t *testing.T) {
	h, s := newTestHandler(t)

	insert(t, s, "0042", "三脚", nil)

	w := get(t, h, "/api/labels.pdf?from=0042&to=0042")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

// 白紙のシートを刷るとラベルシールが1枚無駄になる。
func TestHandleLabels_対象が無ければ400(t *testing.T) {
	h, s := newTestHandler(t)

	insert(t, s, "0001", "三脚", nil)

	w := get(t, h, "/api/labels.pdf?from=100&to=200")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d。400 を期待", w.Code)
	}
}

// 廃棄済みは棚に並ばない。刷る対象にしない。
func TestHandleLabels_廃棄済みは含めない(t *testing.T) {
	h, s := newTestHandler(t)

	insert(t, s, "0001", "壊れた三脚", map[string]any{"condition": string(ConditionDiscarded)})

	w := get(t, h, "/api/labels.pdf")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d。廃棄済みだけなら対象0件で 400 を期待", w.Code)
	}
}

// 逆順の指定を0件と同じ扱いにすると「その範囲に備品が無い」と読めてしまい、
// 打ち間違いに気付けない。
func TestHandleLabels_不正な指定は400(t *testing.T) {
	h, s := newTestHandler(t)

	insert(t, s, "0001", "三脚", nil)
	insert(t, s, "0002", "カメラ", nil)

	paths := []string{
		"/api/labels.pdf?from=2&to=1",
		"/api/labels.pdf?from=abc",
		"/api/labels.pdf?to=0",
		"/api/labels.pdf?from=-1",
	}

	for _, p := range paths {
		w := get(t, h, p)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d。400 を期待", p, w.Code)
		}
	}
}

// UIで隠すだけにしない。経路が requireAdmin を通っていなければ素通りする。
func TestHandleLabels_adminのみ(t *testing.T) {
	h := newDenyAdminHandler(t)

	w := get(t, h, "/api/labels.pdf")
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d。403 を期待", w.Code)
	}
}
