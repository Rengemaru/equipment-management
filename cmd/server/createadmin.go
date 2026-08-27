package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"

	"github.com/Rengemaru/equipment-management/internal/auth"
)

// createAdminInput は -create-admin の入力。
type createAdminInput struct {
	LoginID string
	Name    string
	Email   string
}

// runCreateAdmin は admin ユーザーを1人作り、初期パスワードを out に出す。
//
// admin を作れるのは admin だけなので、最初の1人を作る手段が無いと誰もログインできない。
// Web からは作らせない。作れてしまうと、URLを知っている人が管理者になれる。
//
// パスワードは受け取らず、この中で生成する。コマンドライン引数で渡すと
// シェルの履歴と ps の出力に平文が残る。
func runCreateAdmin(ctx context.Context, sqldb *sql.DB, in createAdminInput, out io.Writer) error {
	if in.LoginID == "" || in.Name == "" {
		return errors.New("-login-id と -name は必須\n" +
			`  例: /server -create-admin -login-id yamada -name "山田太郎"`)
	}

	password, err := auth.GeneratePassword()
	if err != nil {
		return err
	}

	store := auth.NewStore(sqldb)
	user, err := store.Create(ctx, auth.NewUser{
		Name:     in.Name,
		LoginID:  in.LoginID,
		Email:    in.Email,
		Role:     auth.RoleAdmin,
		Password: password,
		// 初回ログイン時に変更させる。生成した文字列は控えとして
		// 紙やチャットに残りやすく、そのまま使い続けさせない。
		MustChangePassword: true,
	})
	if err != nil {
		if errors.Is(err, auth.ErrDuplicateLoginID) {
			return fmt.Errorf("ログインID %q は既に使われている", in.LoginID)
		}
		return err
	}

	// 標準出力に出す。ログ（標準エラー）に混ぜると Docker の収集先に
	// 初期パスワードが残る。
	fmt.Fprintf(out, `admin を作成しました。

  ログインID     : %s
  名前           : %s
  初期パスワード : %s

このパスワードはもう表示されません。控えてから閉じてください。
初回ログイン時に変更を求められます。
`, user.LoginID, user.Name, password)

	return nil
}
