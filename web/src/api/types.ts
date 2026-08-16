/**
 * API のレスポンス型。
 *
 * Go 側の DTO（各ハンドラの `userResponse` や `itemResponse`）と対で保つ。
 * どちらかを変えたら必ず両方を直すこと。
 *
 * プロパティ名は snake_case のまま、APIが返す形をそのまま写す。
 * camelCase に直すと変換層が要り、片方だけ直した時に型は通るのに
 * 値が undefined になる、という一番見つけにくい壊れ方をする。
 */

/** Role は権限。Go の `auth.Role` と対。 */
export type Role = 'admin' | 'member'

/**
 * User はログイン中の利用者。
 *
 * パスワードハッシュもメールアドレスも含まない。含む形は admin 専用の
 * 一覧（`GET /api/users`）だけが返す。
 */
export type User = {
  id: number
  name: string
  login_id: string
  role: Role

  /**
   * must_change_password は初期パスワードのままであること。
   *
   * true の間、サーバは `/api/me` `/api/password` `/api/logout` 以外を
   * 403 で止める。画面側で隠すだけにしない（CLAUDE.md）。
   */
  must_change_password: boolean
}

/**
 * AuthResponse は `/api/login` `/api/me` `/api/password` が返す共通の形。
 *
 * redirect_to は検証済みの自サイト内パス。`next` の解釈はサーバが行うため、
 * フロントはこの値へ進むだけでよい。自分で `next` を読まないこと
 * （オープンリダイレクトの判断を2箇所に分けない）。
 */
export type AuthResponse = {
  user: User
  redirect_to: string
}
