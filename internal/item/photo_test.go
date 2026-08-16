package item

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pngBytes は小さな PNG を作る。
func pngBytes(t *testing.T) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

// jpegBytes は小さな JPEG を作る。
func jpegBytes(t *testing.T) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, 4, 4))

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("jpeg.Encode: %v", err)
	}
	return buf.Bytes()
}

// uploadPhoto は写真を multipart で送る。
func uploadPhoto(t *testing.T, h *Handler, code, filename string, content []byte) *httptest.ResponseRecorder {
	t.Helper()

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)

	part, err := mw.CreateFormFile(photoFormField, filename)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/items/"+code+"/photo", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	return w
}

// createItem は備品を1件作ってコードを返す。
func createItem(t *testing.T, h *Handler, name string) string {
	t.Helper()

	w := send(t, h, http.MethodPost, "/api/items", `{"name":"`+name+`"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("備品の登録に失敗: %d %s", w.Code, w.Body.String())
	}
	return decodeItem(t, w).Code
}

func TestPhoto_登録して取得できる(t *testing.T) {
	h, _ := newTestHandler(t)
	code := createItem(t, h, "三脚")

	content := pngBytes(t)
	w := uploadPhoto(t, h, code, "写真.png", content)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	// 応答にはファイル名ではなく取得先が入る。
	it := decodeItem(t, w)
	if it.PhotoURL != "/api/items/"+code+"/photo" {
		t.Errorf("PhotoURL = %q", it.PhotoURL)
	}

	got := get(t, h, "/api/items/"+code+"/photo")
	if got.Code != http.StatusOK {
		t.Fatalf("取得: status = %d", got.Code)
	}
	if !bytes.Equal(got.Body.Bytes(), content) {
		t.Error("保存した内容と違う画像が返る")
	}
	if ct := got.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q", ct)
	}
	// 万一別の中身が入っていても、HTML や JavaScript として解釈させない。
	if got.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("nosniff が付いていない")
	}
}

func TestPhoto_JPEGも扱える(t *testing.T) {
	h, _ := newTestHandler(t)
	code := createItem(t, h, "カメラ")

	w := uploadPhoto(t, h, code, "photo.jpg", jpegBytes(t))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	got := get(t, h, "/api/items/"+code+"/photo")
	if ct := got.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("Content-Type = %q", ct)
	}
}

// 拡張子や Content-Type を信用せず、中身で判断すること。
func TestPhoto_画像でないファイルを拒否する(t *testing.T) {
	h, _ := newTestHandler(t)
	code := createItem(t, h, "三脚")

	// .png を名乗るテキスト。
	w := uploadPhoto(t, h, code, "悪意.png", []byte("<script>alert(1)</script>"))
	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d。415 を期待。body = %s", w.Code, w.Body.String())
	}

	// 参照が付いていないこと。
	detail := get(t, h, "/api/items/"+code)
	if it := decodeItem(t, detail); it.PhotoURL != "" {
		t.Errorf("PhotoURL = %q。空を期待", it.PhotoURL)
	}
}

func TestPhoto_空のファイルを拒否する(t *testing.T) {
	h, _ := newTestHandler(t)
	code := createItem(t, h, "三脚")

	w := uploadPhoto(t, h, code, "empty.png", nil)
	if w.Code == http.StatusOK {
		t.Errorf("空のファイルが通ってしまう: %s", w.Body.String())
	}
}

// 利用者が送ってきたファイル名を使わないこと。
// パス区切りを含む名前を送られると保存先の外に書ける。
func TestPhoto_送られたファイル名を使わない(t *testing.T) {
	h, _ := newTestHandler(t)
	code := createItem(t, h, "三脚")

	w := uploadPhoto(t, h, code, "../../../etc/passwd.png", pngBytes(t))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	// 保存先に、送った名前の痕跡が無いこと。
	entries, err := os.ReadDir(h.photos.dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("保存されたファイル数 = %d", len(entries))
	}
	name := entries[0].Name()
	if strings.Contains(name, "..") || strings.Contains(name, "passwd") {
		t.Errorf("送られたファイル名が使われている: %q", name)
	}
	if !strings.HasPrefix(name, code+"-") {
		t.Errorf("ファイル名 = %q。備品コードで始まることを期待", name)
	}
}

// 差し替えたら古いファイルを消すこと。消さないと増え続ける。
func TestPhoto_差し替えると古いファイルを消す(t *testing.T) {
	h, _ := newTestHandler(t)
	code := createItem(t, h, "三脚")

	if w := uploadPhoto(t, h, code, "1.png", pngBytes(t)); w.Code != http.StatusOK {
		t.Fatalf("1回目: %d %s", w.Code, w.Body.String())
	}
	if w := uploadPhoto(t, h, code, "2.jpg", jpegBytes(t)); w.Code != http.StatusOK {
		t.Fatalf("2回目: %d %s", w.Code, w.Body.String())
	}

	entries, err := os.ReadDir(h.photos.dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("保存されたファイル数 = %d。1件を期待（古い写真が残っている）", len(entries))
	}
}

func TestPhoto_削除できる(t *testing.T) {
	h, _ := newTestHandler(t)
	code := createItem(t, h, "三脚")

	if w := uploadPhoto(t, h, code, "1.png", pngBytes(t)); w.Code != http.StatusOK {
		t.Fatalf("登録: %d %s", w.Code, w.Body.String())
	}

	w := send(t, h, http.MethodDelete, "/api/items/"+code+"/photo", "")
	if w.Code != http.StatusOK {
		t.Fatalf("削除: status = %d, body = %s", w.Code, w.Body.String())
	}
	if it := decodeItem(t, w); it.PhotoURL != "" {
		t.Errorf("PhotoURL = %q。空を期待", it.PhotoURL)
	}

	if got := get(t, h, "/api/items/"+code+"/photo"); got.Code != http.StatusNotFound {
		t.Errorf("削除後の取得: status = %d。404 を期待", got.Code)
	}

	entries, err := os.ReadDir(h.photos.dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("ファイルが残っている: %d件", len(entries))
	}
}

func TestPhoto_写真が無ければ404(t *testing.T) {
	h, _ := newTestHandler(t)
	code := createItem(t, h, "三脚")

	if w := get(t, h, "/api/items/"+code+"/photo"); w.Code != http.StatusNotFound {
		t.Errorf("status = %d。404 を期待", w.Code)
	}
}

func TestPhoto_未登録の備品は404(t *testing.T) {
	h, _ := newTestHandler(t)

	if w := uploadPhoto(t, h, "9999", "1.png", pngBytes(t)); w.Code != http.StatusNotFound {
		t.Errorf("status = %d。404 を期待", w.Code)
	}
}

// DBに名前はあるがファイルが無い場合。復元で画像だけ戻し忘れると起きる。
func TestPhoto_ファイルが消えていても500にしない(t *testing.T) {
	h, s := newTestHandler(t)
	code := createItem(t, h, "三脚")

	if w := uploadPhoto(t, h, code, "1.png", pngBytes(t)); w.Code != http.StatusOK {
		t.Fatalf("登録: %d", w.Code)
	}

	it, err := s.ByCode(t.Context(), code)
	if err != nil {
		t.Fatalf("ByCode: %v", err)
	}
	if err := os.Remove(filepath.Join(h.photos.dir, it.PhotoPath)); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if w := get(t, h, "/api/items/"+code+"/photo"); w.Code != http.StatusNotFound {
		t.Errorf("status = %d。404 を期待", w.Code)
	}
}

// 一覧にも取得先が入ること。
func TestPhoto_一覧に取得先が入る(t *testing.T) {
	h, _ := newTestHandler(t)
	code := createItem(t, h, "三脚")
	createItem(t, h, "カメラ")

	if w := uploadPhoto(t, h, code, "1.png", pngBytes(t)); w.Code != http.StatusOK {
		t.Fatalf("登録: %d", w.Code)
	}

	items := decodeItems(t, get(t, h, "/api/items"))
	if len(items) != 2 {
		t.Fatalf("件数 = %d", len(items))
	}

	var withPhoto, withoutPhoto int
	for _, it := range items {
		if it.PhotoURL == "" {
			withoutPhoto++
		} else {
			withPhoto++
		}
	}
	if withPhoto != 1 || withoutPhoto != 1 {
		t.Errorf("写真あり = %d、なし = %d", withPhoto, withoutPhoto)
	}
}

// ---- PhotoStore 単体 ----

func TestPhotoStore_保存先の外に書けない(t *testing.T) {
	dir := t.TempDir()
	p, err := NewPhotoStore(dir)
	if err != nil {
		t.Fatalf("NewPhotoStore: %v", err)
	}

	for _, name := range []string{
		"../outside.png",
		"sub/dir.png",
		`..\outside.png`,
		"",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := p.Open(name); err == nil {
				t.Errorf("%q が開けてしまう", name)
			}
			if err := p.Remove(name); name != "" && err == nil {
				t.Errorf("%q が消せてしまう", name)
			}
		})
	}
}

func TestPhotoStore_保存先を作る(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "まだ無い", "uploads")

	if _, err := NewPhotoStore(dir); err != nil {
		t.Fatalf("NewPhotoStore: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("保存先が作られていない: %v", err)
	}
}

func TestPhotoStore_空のパスを拒否する(t *testing.T) {
	if _, err := NewPhotoStore("  "); err == nil {
		t.Error("空の保存先が通ってしまう")
	}
}

// 応答にファイル名をそのまま出さない。
// 保存先の構成が変わるたびにフロントのURL組み立てを直すことになる。
func TestItemResponse_ファイル名を出さない(t *testing.T) {
	h, s := newTestHandler(t)
	code := createItem(t, h, "三脚")

	if w := uploadPhoto(t, h, code, "1.png", pngBytes(t)); w.Code != http.StatusOK {
		t.Fatalf("登録: %d", w.Code)
	}

	it, err := s.ByCode(t.Context(), code)
	if err != nil {
		t.Fatalf("ByCode: %v", err)
	}

	body := get(t, h, "/api/items/"+code).Body.String()
	if strings.Contains(body, it.PhotoPath) {
		t.Errorf("ファイル名が応答に出ている: %s", body)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		t.Fatalf("応答が JSON でない: %v", err)
	}
	if strings.Contains(string(raw["item"]), "photo_path") {
		t.Error("photo_path が応答に含まれている")
	}
}
