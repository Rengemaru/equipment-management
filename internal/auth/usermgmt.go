package auth

import (
	"context"
	"errors"
	"fmt"
)

// ErrLastAdmin は最後の admin を無効化しようとしたこと。
//
// 全員が無効になると、ユーザーを作り直す手段がWebから無くなる。
// 復旧にはコンテナ内で -create-admin を実行する必要があり、
// その手順を知らない代が引き継いだ時点で詰む。
var ErrLastAdmin = errors.New("最後の管理者は無効化できない")

// List は全ユーザーを返す。無効化された人も含む。
//
// 卒業者を消さない方針（CLAUDE.md）のため、一覧は増える一方になる。
// 数十名規模なので、絞り込みはフロント側で足りる。
func (s *Store) List(ctx context.Context) ([]*User, error) {
	// 有効な人を先に、その中で新しい順。admin が普段見たいのは今いる人。
	rows, err := s.sqldb.QueryContext(ctx, selectColumns+`
ORDER BY is_active DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("ユーザー一覧の取得: %w", err)
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, fmt.Errorf("ユーザー一覧の読み取り: %w", err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ユーザー一覧の読み取り: %w", err)
	}

	return users, nil
}

// SetActive は有効・無効を切り替える。
//
// 削除は用意しない。users を消すと貸出履歴が壊れる（CLAUDE.md）。
func (s *Store) SetActive(ctx context.Context, userID int64, active bool) error {
	if !active {
		// 最後の admin を落とさせない。落とすと、Webからは誰も
		// ユーザーを操作できなくなる。
		lastAdmin, err := s.isLastActiveAdmin(ctx, userID)
		if err != nil {
			return err
		}
		if lastAdmin {
			return ErrLastAdmin
		}
	}

	const q = `UPDATE users SET is_active = ?, updated_at = datetime('now') WHERE id = ?`

	res, err := s.sqldb.ExecContext(ctx, q, boolToInt(active), userID)
	if err != nil {
		return fmt.Errorf("有効・無効の切り替え: %w", err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("有効・無効の切り替え: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}

	return nil
}

// isLastActiveAdmin は、その利用者が最後の有効な admin かを返す。
func (s *Store) isLastActiveAdmin(ctx context.Context, userID int64) (bool, error) {
	const q = `
SELECT
  (SELECT COUNT(*) FROM users WHERE role = 'admin' AND is_active = 1),
  (SELECT COUNT(*) FROM users WHERE role = 'admin' AND is_active = 1 AND id = ?)`

	var total, isTarget int
	if err := s.sqldb.QueryRowContext(ctx, q, userID).Scan(&total, &isTarget); err != nil {
		return false, fmt.Errorf("管理者数の確認: %w", err)
	}

	return total <= 1 && isTarget == 1, nil
}

// ResetPassword は admin が発行し直す時に使う。
//
// SetPassword と分けているのは must_change_password の扱いが逆になるため。
// 本人による変更では下ろし、admin による再発行では立てる。
// 発行された文字列は口頭やチャットで渡るため、そのまま使い続けさせない。
func (s *Store) ResetPassword(ctx context.Context, userID int64) (string, error) {
	password, err := GeneratePassword()
	if err != nil {
		return "", err
	}

	hash, err := HashPassword(password)
	if err != nil {
		return "", err
	}

	const q = `
UPDATE users
SET password_hash = ?, must_change_password = 1, updated_at = datetime('now')
WHERE id = ?`

	res, err := s.sqldb.ExecContext(ctx, q, hash, userID)
	if err != nil {
		return "", fmt.Errorf("パスワードの再発行: %w", err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return "", fmt.Errorf("パスワードの再発行: %w", err)
	}
	if n == 0 {
		return "", ErrNotFound
	}

	return password, nil
}
