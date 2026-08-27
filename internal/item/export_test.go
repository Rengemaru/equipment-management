package item

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// exportCSVBody はエクスポートを叩き、本文を返す。
func exportCSVBody(t *testing.T, h *Handler) []byte {
	t.Helper()

	w := get(t, h, "/api/items/export.csv")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	return w.Body.Bytes()
}

// exportRecords は書き出されたCSVを解析して返す。先頭がヘッダ行。
func exportRecords(t *testing.T, body []byte) [][]string {
	t.Helper()

	trimmed := bytes.TrimPrefix(body, utf8BOM)
	records, err := csv.NewReader(bytes.NewReader(trimmed)).ReadAll()
	if err != nil {
		t.Fatalf("CSVとして読めない: %v (%s)", err, trimmed)
	}
	if len(records) == 0 {
		t.Fatal("ヘッダ行すら無い")
	}
	return records
}

// 保険が目的なので、普段の一覧の除外ルールを持ち込まない（m1-spec §8）。
// 廃棄済みが落ちると、控えを見た人が「この備品は無かった」と読む。
func TestExport_廃棄済みも含めて出す(t *testing.T) {
	h, s := newTestHandler(t)

	insert(t, s, "0001", "三脚", nil)
	insert(t, s, "0002", "壊れたカメラ", map[string]any{"condition": string(ConditionDiscarded)})
	insert(t, s, "0003", "ドライバー", nil)

	records := exportRecords(t, exportCSVBody(t, h))

	if len(records) != 4 {
		t.Fatalf("行数 = %d。ヘッダ + 3件を期待: %v", len(records), records)
	}

	// 備品コード順。ラベルを並べた棚と見比べる時にこの順が要る。
	for i, want := range []string{"0001", "0002", "0003"} {
		if records[i+1][0] != want {
			t.Errorf("%d 件目のコード = %q。%q を期待", i, records[i+1][0], want)
		}
	}
	if records[2][6] != string(ConditionDiscarded) {
		t.Errorf("状態 = %q。廃棄をそのまま出すべき", records[2][6])
	}
}

// 列はテンプレートと揃える。先頭の備品コードだけが書き出し専用の列で、
// これが無いと、どのラベルの備品かが分からず控えとして使えない。
func TestExport_ヘッダは備品コードとテンプレートの列(t *testing.T) {
	h, _ := newTestHandler(t)

	records := exportRecords(t, exportCSVBody(t, h))

	want := []string{
		"備品コード", "品名", "分類", "数量", "保管場所",
		"型番・メーカー", "状態", "所有", "自由利用", "備考",
	}
	if len(records[0]) != len(want) {
		t.Fatalf("列数 = %d。%d を期待: %v", len(records[0]), len(want), records[0])
	}
	for i, w := range want {
		if records[0][i] != w {
			t.Errorf("%d 列目 = %q。%q を期待", i, records[0][i], w)
		}
	}

	// csvColumns からずれたら、書き出したものが取り込めなくなる。
	for i, col := range csvColumns {
		if records[0][i+1] != col.header {
			t.Errorf("%d 列目 = %q。csvColumns の %q と揃えるべき",
				i+1, records[0][i+1], col.header)
		}
	}
}

func TestExport_全項目を出す(t *testing.T) {
	h, s := newTestHandler(t)

	insert(t, s, "0042", "三脚（大）", map[string]any{
		"category":    "撮影機材",
		"model":       "Manfrotto MT055",
		"owner":       string(OwnerDepartment),
		"is_free_use": 1,
		"location":    "棚A-1",
		"condition":   string(ConditionNeedsFix),
		"note":        "脚のロックが緩い",
	})

	records := exportRecords(t, exportCSVBody(t, h))
	if len(records) != 2 {
		t.Fatalf("行数 = %d。ヘッダ + 1件を期待", len(records))
	}

	want := []string{
		"0042", "三脚（大）", "撮影機材", "1", "棚A-1",
		"Manfrotto MT055", string(ConditionNeedsFix), string(OwnerDepartment),
		"TRUE", "脚のロックが緩い",
	}
	for i, w := range want {
		if records[1][i] != w {
			t.Errorf("%d 列目 = %q。%q を期待", i, records[1][i], w)
		}
	}
}

// 数量をまとめると、行と備品コードの対応が消える。
func TestExport_数量は常に1(t *testing.T) {
	h, s := newTestHandler(t)

	// 同じ品名・同じ保管場所。取り込みで数量2から展開されたのと同じ形。
	insert(t, s, "0001", "パイプ椅子", map[string]any{"location": "倉庫"})
	insert(t, s, "0002", "パイプ椅子", map[string]any{"location": "倉庫"})

	records := exportRecords(t, exportCSVBody(t, h))
	if len(records) != 3 {
		t.Fatalf("行数 = %d。1レコード1行を期待", len(records))
	}
	for i := 1; i < len(records); i++ {
		if records[i][3] != "1" {
			t.Errorf("%d 行目の数量 = %q。1 を期待", i, records[i][3])
		}
	}
}

// BOM が無いと Excel が Shift_JIS として開き、品名が全て文字化けする。
func TestExport_BOM付きのUTF8で出す(t *testing.T) {
	h, s := newTestHandler(t)
	insert(t, s, "0001", "三脚", nil)

	body := exportCSVBody(t, h)
	if !bytes.HasPrefix(body, utf8BOM) {
		t.Fatalf("BOM が無い: % x", body[:min(len(body), 8)])
	}
	if !bytes.Contains(body, []byte("三脚")) {
		t.Error("UTF-8 のまま出ていない")
	}
}

// LF だけだと古い Excel が全体を1行として読む。
func TestExport_改行はCRLF(t *testing.T) {
	h, s := newTestHandler(t)
	insert(t, s, "0001", "三脚", nil)

	body := string(exportCSVBody(t, h))
	if !strings.Contains(body, "\r\n") {
		t.Fatalf("CRLF が無い: %q", body)
	}
	// CR を伴わない LF が混ざっていないこと。
	if strings.Count(body, "\n") != strings.Count(body, "\r\n") {
		t.Errorf("LF だけの改行が混ざっている: %q", body)
	}
}

// ブラウザがCSVとして保存するために要る。
func TestExport_ダウンロード用のヘッダを付ける(t *testing.T) {
	h, _ := newTestHandler(t)

	w := get(t, h, "/api/items/export.csv")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}

	if ct := w.Header().Get("Content-Type"); ct != "text/csv; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
	if cd := w.Header().Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment;") ||
		!strings.Contains(cd, exportFilename) {
		t.Errorf("Content-Disposition = %q", cd)
	}
}

// 1件も無くてもヘッダ行だけのCSVを返す。空の応答だと、
// 書き出しに失敗したのか備品が無いのかを区別できない。
func TestExport_0件でもヘッダ行を返す(t *testing.T) {
	h, _ := newTestHandler(t)

	records := exportRecords(t, exportCSVBody(t, h))
	if len(records) != 1 {
		t.Fatalf("行数 = %d。ヘッダ行だけを期待: %v", len(records), records)
	}
}

// 書き出したものが取り込めない状態だと、控えとして使えるかを確かめる術がなくなる。
func TestExport_書き出したCSVをそのまま取り込める(t *testing.T) {
	h, s := newTestHandler(t)

	insert(t, s, "0001", "三脚", map[string]any{
		"category": "撮影機材", "location": "棚A-1", "is_free_use": 1,
	})
	insert(t, s, "0002", "壊れたカメラ", map[string]any{
		"category": "撮影機材", "location": "棚A-2",
		"condition": string(ConditionDiscarded),
	})

	body := string(exportCSVBody(t, h))

	// 別のDBへ取り込む。控えから作り直す状況をそのまま再現する。
	restored, rs := newTestHandler(t)

	w := importCSV(t, restored, body)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	items, err := rs.List(context.Background(), Filter{IncludeDiscarded: true})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("件数 = %d。2件を期待", len(items))
	}
	if items[0].Name != "三脚" || items[0].Location != "棚A-1" || !items[0].IsFreeUse {
		t.Errorf("内容が戻っていない: %+v", items[0])
	}
	if items[1].Condition != ConditionDiscarded {
		t.Errorf("状態 = %q。廃棄が戻るべき", items[1].Condition)
	}
}

// 取り込みは常に自動採番するため、控えを取り込んでも元のコードは戻らない。
// 仕様であって不具合ではない。元のコードごと戻すのは -backup の役目。
func TestExport_取り込み直すと備品コードは採番し直される(t *testing.T) {
	h, s := newTestHandler(t)

	insert(t, s, "0100", "三脚", map[string]any{"location": "棚A-1"})

	body := string(exportCSVBody(t, h))

	restored, rs := newTestHandler(t)
	if w := importCSV(t, restored, body); w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	items, err := rs.List(context.Background(), Filter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("件数 = %d。1件を期待", len(items))
	}
	if items[0].Code != "0001" {
		t.Errorf("Code = %q。採番し直されて 0001 になるはず", items[0].Code)
	}
}

// export.csv が {code} のパターンに吸われないこと。
// 吸われると「export.csv という備品コード」を探しに行って 404 になる。
func TestRegister_exportが詳細に吸われない(t *testing.T) {
	h, _ := newTestHandler(t)

	w := get(t, h, "/api/items/export.csv")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d。200 を期待（詳細の経路に吸われている）", w.Code)
	}
	// 詳細の応答（JSON）が返っていないこと。
	var jsonBody map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &jsonBody); err == nil {
		t.Errorf("JSON が返っている: %s", w.Body.String())
	}
}
