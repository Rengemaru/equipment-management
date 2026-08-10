package item

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/Rengemaru/equipment-management/internal/httpx"
)

// photoFormField は multipart のフィールド名。
const photoFormField = "photo"

// registerPhotoRoutes は写真の経路を登録する。
func (h *Handler) registerPhotoRoutes(mux *http.ServeMux) {
	// 表示は member も可。備品を見分けるための情報で、隠す理由がない。
	mux.Handle("GET /api/items/{code}/photo", h.requireLogin(http.HandlerFunc(h.handlePhotoGet)))

	mux.Handle("POST /api/items/{code}/photo", h.requireAdmin(http.HandlerFunc(h.handlePhotoUpload)))
	mux.Handle("DELETE /api/items/{code}/photo", h.requireAdmin(http.HandlerFunc(h.handlePhotoDelete)))
}

func (h *Handler) handlePhotoUpload(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")

	it, err := h.store.ByCode(r.Context(), code)
	if err != nil {
		writeItemError(w, err)
		return
	}

	// 本文の上限を先に決める。ParseMultipartForm はここを超えると
	// エラーになり、メモリを使い切る前に止まる。
	r.Body = http.MaxBytesReader(w, r.Body, maxPhotoBytes+(1<<20))

	file, _, err := r.FormFile(photoFormField)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "写真が添付されていません")
		return
	}
	defer func() { _ = file.Close() }()

	name, err := h.photos.Save(it.Code, file)
	if err != nil {
		writePhotoError(w, err)
		return
	}

	updated, err := h.store.SetPhoto(r.Context(), it.Code, name)
	if err != nil {
		// DBに書けなかったなら、保存したファイルは参照されない。消しておく。
		if rmErr := h.photos.Remove(name); rmErr != nil {
			log.Printf("items: %v", rmErr)
		}
		writeItemError(w, err)
		return
	}

	// 差し替えなら古いファイルを消す。消さないと、参照されないファイルが
	// 増え続ける。失敗しても利用者の操作は完了しているので止めない。
	if it.PhotoPath != "" && it.PhotoPath != name {
		if err := h.photos.Remove(it.PhotoPath); err != nil {
			log.Printf("items: %v", err)
		}
	}

	httpx.JSON(w, http.StatusOK, map[string]any{"item": newItemResponse(updated)})
}

func (h *Handler) handlePhotoDelete(w http.ResponseWriter, r *http.Request) {
	it, err := h.store.ByCode(r.Context(), r.PathValue("code"))
	if err != nil {
		writeItemError(w, err)
		return
	}

	updated, err := h.store.SetPhoto(r.Context(), it.Code, "")
	if err != nil {
		writeItemError(w, err)
		return
	}

	// 先にDBの参照を外してからファイルを消す。順序が逆だと、
	// 消えたファイルを指したままのレコードが残り得る。
	if err := h.photos.Remove(it.PhotoPath); err != nil {
		log.Printf("items: %v", err)
	}

	httpx.JSON(w, http.StatusOK, map[string]any{"item": newItemResponse(updated)})
}

func (h *Handler) handlePhotoGet(w http.ResponseWriter, r *http.Request) {
	it, err := h.store.ByCode(r.Context(), r.PathValue("code"))
	if err != nil {
		writeItemError(w, err)
		return
	}

	if it.PhotoPath == "" {
		httpx.WriteError(w, http.StatusNotFound, "写真は登録されていません")
		return
	}

	f, err := h.photos.Open(it.PhotoPath)
	if err != nil {
		// DBには名前があるがファイルが無い。バックアップからの復元で
		// 画像だけ戻し忘れた時に起きる。原因が分かるようログに残す。
		log.Printf("items: 写真が見つからない code=%s path=%s: %v", it.Code, it.PhotoPath, err)
		httpx.WriteError(w, http.StatusNotFound, "写真は登録されていません")
		return
	}
	defer func() { _ = f.Close() }()

	stat, err := f.Stat()
	if err != nil {
		log.Printf("items: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "サーバ側で問題が起きました")
		return
	}

	// 保存時に形式を確認しているので、ここで推測させない。
	// nosniff を付けるのは、万一別の中身が入っていてもブラウザに
	// HTML や JavaScript として解釈させないため。
	w.Header().Set("Content-Type", ContentTypeFor(it.PhotoPath))
	w.Header().Set("X-Content-Type-Options", "nosniff")

	// ファイル名は差し替えのたびに変わる。同じ名前なら中身も同じなので、
	// 長めにキャッシュしてよい。スマートフォンでの再読み込みが軽くなる。
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("Content-Length", strconv.FormatInt(stat.Size(), 10))

	http.ServeContent(w, r, it.PhotoPath, stat.ModTime(), f)
}

// writePhotoError は写真固有のエラーを応答に変える。
func writePhotoError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrUnsupportedPhoto) {
		httpx.WriteError(w, http.StatusUnsupportedMediaType, ErrUnsupportedPhoto.Error())
		return
	}
	writeItemError(w, err)
}
