package auth

import (
	"context"
	"errors"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

// ErrInvalidCredentials は認証に失敗したこと。
//
// 「そのIDは無い」「パスワードが違う」「無効化されている」を区別しない。
// 区別できると、存在するログインIDを総当たりで洗い出せる。
// 呼び出し側が誤って区別できないよう、Authenticate は失敗を全てこれに潰す。
var ErrInvalidCredentials = errors.New("ログインIDまたはパスワードが違う")

// dummyHash は存在しないユーザーに対しても照合を走らせるためのハッシュ。
//
// 見つからない時に即座に返すと、応答時間だけで「そのIDは存在しない」と分かる。
// メッセージを揃えても、時間差で同じことが漏れる。
//
// sync.OnceValue にしているのは、起動時に毎回 bcrypt を回さないため。
// 失敗したログインが一度も無ければ計算されない。
var dummyHash = sync.OnceValue(func() []byte {
	// 平文は何でもよい。一致させる相手がいない。
	h, err := bcrypt.GenerateFromPassword([]byte("dummy password for timing"), bcrypt.DefaultCost)
	if err != nil {
		// 生成できないのは bcrypt の実装が壊れている場合だけ。
		// その状態で認証を続けても意味がない。
		panic("dummy hash の生成に失敗: " + err.Error())
	}
	return h
})

// Authenticate はログインIDとパスワードを照合する。
//
// 失敗の理由は返さない。理由を返すと、いつか画面に出て
// 「そのIDは存在します」を教えることになる。
func (s *Store) Authenticate(ctx context.Context, loginID, password string) (*User, error) {
	user, err := s.ByLoginID(ctx, loginID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			// 見つからなくても照合の時間を使う。
			_ = bcrypt.CompareHashAndPassword(dummyHash(), []byte(password))
			return nil, ErrInvalidCredentials
		}
		// DBに触れない等、認証の可否とは別の問題。これはそのまま返す。
		return nil, err
	}

	// 卒業者（is_active = 0）は入れない。ただし理由は伝えない。
	// 「無効化されています」と返すと、そのIDが存在することが分かる。
	if !user.IsActive {
		_ = bcrypt.CompareHashAndPassword(dummyHash(), []byte(password))
		return nil, ErrInvalidCredentials
	}

	if err := user.VerifyPassword(password); err != nil {
		return nil, ErrInvalidCredentials
	}

	return user, nil
}
