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
 * CONDITIONS は状態の全て。DBの CHECK 制約・Go の `item.Condition` と対。
 *
 * 型と選択肢を1つの定義から作る。別々に書くと、値を足した時に
 * 型は通るのに選択肢に出ない、という気付きにくいずれ方をする。
 */
export const CONDITIONS = ['良好', '要修理', '廃棄'] as const
export type Condition = (typeof CONDITIONS)[number]

/**
 * LOCATION_STATUSES は所在の全て。貸出状態とは独立で、
 * 「貸出中かつ所在不明」も起こり得る。
 *
 * `所在不明_未確認` をまとめて「所在不明」と表示しない。記録されなかった
 * 事実を確かなものとして見せないため、不確かさを残したまま出す（CLAUDE.md）。
 */
export const LOCATION_STATUSES = ['在庫', '所在不明_未確認', '所在不明_確定'] as const
export type LocationStatus = (typeof LOCATION_STATUSES)[number]

/** Owner は所有区分。 */
export type Owner = 'サークル' | '学科'

/** Item は1件の備品。 */
export type Item = {
  id: number
  code: string
  name: string

  category: string
  model: string
  owner: Owner

  /**
   * is_free_use は記録不要の自由利用品。貸出フローの対象外にする。
   * 追跡対象を減らすことが遵守率を上げる最短経路（CLAUDE.md）。
   */
  is_free_use: boolean

  location: string
  condition: Condition
  location_status: LocationStatus

  /** photo_url は写真の取得先。無ければ空文字。 */
  photo_url: string

  note: string
  updated_at: string
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
