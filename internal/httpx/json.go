package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

// maxJSONBytes は受け付ける JSON の上限。
// 画像は multipart で送るため、JSON がこの大きさになることはない。
const maxJSONBytes = 1 << 20 // 1MB

// ErrorResponse はエラー時の本文。
//
// 形を1つに決めておく。ハンドラごとに違う形を返すと、
// フロント側がエンドポイントの数だけ分岐を持つことになる。
type ErrorResponse struct {
	// Error は利用者に見せる文言。
	Error string `json:"error"`

	// Code はフロントが分岐に使う識別子。文言で分岐させると、
	// 日本語を直した瞬間に分岐が壊れる。必要な時だけ入れる。
	Code string `json:"code,omitempty"`
}

// JSON は v を JSON で書く。
//
// エンコードに失敗しても、ステータスは既に送られていて取り消せない。
// 本文が途中で切れることになるが、ログに残せば追える。
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("json encode: %v", err)
	}
}

// WriteError はエラーを JSON で返す。
//
// message は利用者に見せる文言。内部のエラーをそのまま渡さないこと。
// SQL文やファイルパスが画面に出る。
func WriteError(w http.ResponseWriter, status int, message string) {
	JSON(w, status, ErrorResponse{Error: message})
}

// WriteErrorCode は分岐用の識別子を添えてエラーを返す。
//
// フロントが「この場合だけ別の画面へ送る」といった扱いをする時に使う。
func WriteErrorCode(w http.ResponseWriter, status int, code, message string) {
	JSON(w, status, ErrorResponse{Error: message, Code: code})
}

// DecodeJSON はリクエスト本文を dst に読み込む。
//
// 失敗した場合は利用者に見せてよいメッセージを返す。呼び出し側は
// それをそのまま 400 で返せる。
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	// 上限を設けないと、大きな本文を送られただけでメモリを使い切る。
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBytes)

	dec := json.NewDecoder(r.Body)

	// 知らないフィールドを弾く。クライアントもこのリポジトリで書くため、
	// 綴り違いは黙って無視されるより早く気付ける方がよい。
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		return decodeError(err)
	}

	// 本文が2つ続けて入っていないか。1リクエスト1オブジェクトに限る。
	if err := dec.Decode(&struct{}{}); err == nil {
		return errors.New("本文に複数のJSONが含まれている")
	}

	return nil
}

// decodeError は json のエラーを利用者に見せられる形にする。
func decodeError(err error) error {
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	var maxBytesErr *http.MaxBytesError

	switch {
	case errors.As(err, &syntaxErr):
		return fmt.Errorf("JSONの形式が不正（%d文字目）", syntaxErr.Offset)

	case errors.As(err, &typeErr):
		return fmt.Errorf("%s の型が不正", typeErr.Field)

	case errors.As(err, &maxBytesErr):
		return fmt.Errorf("本文が大きすぎる（%dバイトまで）", maxJSONBytes)

	// DisallowUnknownFields のエラーは型を持たない。文字列で見るしかない。
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		field := strings.TrimPrefix(err.Error(), "json: unknown field ")
		return fmt.Errorf("知らない項目が含まれている: %s", field)

	case errors.Is(err, io.EOF):
		return errors.New("本文が空")

	default:
		return errors.New("本文を読めなかった")
	}
}
