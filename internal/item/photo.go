package item

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"

	// image.DecodeConfig で使う形式を登録する。
	// 対応するのは JPEG と PNG だけ。標準ライブラリで完結する範囲に留める。
	_ "image/jpeg"
	_ "image/png"
)

const (
	// maxPhotoBytes は受け付ける画像の上限。
	//
	// スマートフォンの写真は3〜5MB程度。10MB あれば足りる。
	// 上限を設けないと、1枚でボリュームを埋められる。
	maxPhotoBytes = 10 << 20

	// photoDirPerm はアップロード先ディレクトリの権限。
	photoDirPerm = 0o755

	// photoFilePerm は保存する画像の権限。
	photoFilePerm = 0o644
)

// ErrUnsupportedPhoto は画像として読めないこと。
var ErrUnsupportedPhoto = errors.New("JPEG または PNG の画像を選んでください")

// PhotoStore は備品写真をファイルシステムに保存する。
//
// 画像をDBに入れない。SQLiteのファイルが肥大し、バックアップの
// たびに全画像を書き出すことになる（-backup は VACUUM INTO で作る）。
type PhotoStore struct {
	dir string
}

// NewPhotoStore は保存先を用意する。
func NewPhotoStore(dir string) (*PhotoStore, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("写真の保存先が空")
	}
	if err := os.MkdirAll(dir, photoDirPerm); err != nil {
		return nil, fmt.Errorf("写真の保存先の作成: %w", err)
	}
	return &PhotoStore{dir: dir}, nil
}

// Save は画像を保存し、ファイル名を返す。
//
// 戻り値はファイル名だけで、ディレクトリを含まない。DBにはこれを入れる。
// 絶対パスを保存すると、UPLOAD_DIR を変えた瞬間に全ての写真が見えなくなる。
func (p *PhotoStore) Save(code string, r io.Reader) (string, error) {
	// 上限まで読む。1バイト多く読んで、超過を検出する。
	data, err := io.ReadAll(io.LimitReader(r, maxPhotoBytes+1))
	if err != nil {
		return "", fmt.Errorf("写真の読み込み: %w", err)
	}
	if len(data) > maxPhotoBytes {
		return "", invalidf("写真が大きすぎます（%dMBまで）", maxPhotoBytes>>20)
	}
	if len(data) == 0 {
		return "", invalidf("写真が空です")
	}

	// 拡張子や Content-Type を信用しない。中身を解釈して形式を決める。
	// 実行可能なファイルに .jpg を付けただけのものを保存させない。
	_, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", ErrUnsupportedPhoto
	}

	ext, ok := extensionFor(format)
	if !ok {
		return "", ErrUnsupportedPhoto
	}

	name, err := photoFilename(code, ext)
	if err != nil {
		return "", err
	}

	// 一時ファイルに書いてから rename する。途中で落ちた時に、
	// 壊れた画像がDBから参照される状態を作らない。
	tmp, err := os.CreateTemp(p.dir, ".tmp-*")
	if err != nil {
		return "", fmt.Errorf("写真の保存: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // rename 済みなら失敗するだけ

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("写真の保存: %w", err)
	}
	if err := tmp.Chmod(photoFilePerm); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("写真の保存: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("写真の保存: %w", err)
	}

	if err := os.Rename(tmpName, filepath.Join(p.dir, name)); err != nil {
		return "", fmt.Errorf("写真の保存: %w", err)
	}

	return name, nil
}

// Open は保存済みの画像を開く。
func (p *PhotoStore) Open(name string) (*os.File, error) {
	safe, err := p.path(name)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(safe)
	if err != nil {
		return nil, fmt.Errorf("写真の読み込み: %w", err)
	}
	return f, nil
}

// Remove は画像を消す。既に無ければ何もしない。
//
// 備品を消すことはないが、写真は差し替えられる。古い方を消さないと、
// 参照されないファイルが増え続ける。
func (p *PhotoStore) Remove(name string) error {
	if name == "" {
		return nil
	}

	safe, err := p.path(name)
	if err != nil {
		return err
	}

	if err := os.Remove(safe); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("写真の削除: %w", err)
	}
	return nil
}

// path はファイル名を保存先のパスに変える。
//
// DBに入っているのは自分で生成した名前だが、ここでも検査する。
// 将来どこかで外部由来の値が渡った時に、../../etc/passwd を読ませない。
func (p *PhotoStore) path(name string) (string, error) {
	if name == "" || name != filepath.Base(name) || strings.ContainsAny(name, `/\`) {
		return "", invalidf("写真のファイル名が不正です")
	}
	return filepath.Join(p.dir, name), nil
}

// photoFilename は保存するファイル名を作る。
//
// 利用者が送ってきたファイル名は使わない。日本語・空白・記号が入り、
// パス区切りを含む名前を送られると保存先の外に書ける。
// 備品コードを頭に付けるのは、ファイルだけを見た時にどの備品か分かるようにするため。
func photoFilename(code, ext string) (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("写真のファイル名の生成: %w", err)
	}

	// コードは採番した数字だが、CSVインポート由来の値も混ざり得る。
	// 英数字以外は落とす。
	var safe strings.Builder
	for _, r := range code {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			safe.WriteRune(r)
		}
	}
	if safe.Len() == 0 {
		safe.WriteString("item")
	}

	return safe.String() + "-" + hex.EncodeToString(buf) + ext, nil
}

// extensionFor は image が返した形式名を拡張子に変える。
func extensionFor(format string) (string, bool) {
	switch format {
	case "jpeg":
		return ".jpg", true
	case "png":
		return ".png", true
	default:
		return "", false
	}
}

// ContentTypeFor はファイル名から Content-Type を決める。
//
// 保存時に形式を確認しているため、拡張子と中身は一致している。
func ContentTypeFor(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png":
		return "image/png"
	default:
		return "image/jpeg"
	}
}
