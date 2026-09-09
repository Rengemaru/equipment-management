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

/** OWNERS は所有区分の全て。 */
export const OWNERS = ['サークル', '学科'] as const
export type Owner = (typeof OWNERS)[number]

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

  /**
   * loan は貸出中の情報。詳細（`GET /api/items/{code}`）だけに入る。
   *
   * 一覧には入らないため undefined になる。**「借りられるか」はここでは分からない。**
   * is_free_use / condition / loan から画面が組み立てる（判定をサーバに持たせると、
   * 同じ判断がAPIと画面の2箇所に散る）。
   */
  loan?: ItemLoan | null
}

/** LoanUser は貸出に関わる人。 */
export type LoanUser = {
  id: number
  name: string
}

/** ItemLoan は備品詳細に載る貸出。貸出そのものより項目が少ない。 */
export type ItemLoan = {
  id: number
  user: LoanUser
  /** borrowed_at は RFC3339（JST）。 */
  borrowed_at: string
  /** due_date は 'YYYY-MM-DD'。 */
  due_date: string
  /** overdue_days は返却予定日を過ぎた日数。超過していなければ 0。 */
  overdue_days: number
}

/**
 * Loan は1件の貸出。Go の `loan.loanResponse` と対。
 *
 * 返却しても消えない。破損・紛失の追跡はこの履歴が唯一の根拠になる。
 */
export type Loan = {
  id: number
  item: { code: string; name: string }

  /** user は借用者、registered_by は登録者。代理登録では異なる。 */
  user: LoanUser
  registered_by: LoanUser

  /**
   * is_proxy は代理登録か。**サーバが計算して返す。**
   * 画面で2つのidを比べる形にすると、比べ忘れた画面ができる。
   */
  is_proxy: boolean

  borrowed_at: string
  due_date: string

  /** returned_at が null なら貸出中。 */
  returned_at: string | null
  returned_by: LoanUser | null

  overdue_days: number
  note: string
}

/** DAMAGE_STATUSES は破損報告の状態の全て。 */
export const DAMAGE_STATUSES = ['未確認', '確認済み', '修理済み', '廃棄'] as const
export type DamageStatus = (typeof DAMAGE_STATUSES)[number]

/** DamageReport は1件の破損報告。 */
export type DamageReport = {
  id: number
  item: { code: string; name: string }

  /** loan_id は貸出中に報告された場合の貸出。在庫中の報告では null。 */
  loan_id: number | null

  reporter: LoanUser
  reported_at: string
  description: string

  status: DamageStatus
  confirmed_by: LoanUser | null
  confirmed_at: string | null
  note: string
}

/**
 * AdminUser は運営が見る利用者。`GET /api/users` が返す。
 *
 * [[User]] より項目が多い。誰が卒業済みか、連絡先が入っているかを
 * 運営は見る必要がある。__この形を運営以外に見せない。__
 */
export type AdminUser = {
  id: number
  name: string
  login_id: string

  /** email は通知専用で、認証には使わない。未設定なら空文字。 */
  email: string

  role: Role
  is_active: boolean

  /** must_change_password は初期パスワードのまま。まだ一度も使っていない目安になる。 */
  must_change_password: boolean

  created_at: string
}

/**
 * UserWithPassword は初期パスワードを添えた応答。
 *
 * __initial_password はこの応答でしか手に入らない。__ DBにはハッシュしか
 * 無く、再表示はできない。画面から消す前に必ず控えさせること。
 */
export type UserWithPassword = {
  user: AdminUser
  initial_password: string
}

/**
 * ItemAttributes は登録・更新で送る内容。
 *
 * 備品コードを含めない。__コードは採番したら二度と変えない。__
 * ラベルは貼り替えられないため、変えると実物との対応が壊れる。
 *
 * 一部だけ送る形にしない。送らなかった項目を「変更なし」と解釈させると、
 * 画面で消した備考が消えないなど、意図と結果がずれる（サーバも全項目を要求する）。
 */
export type ItemAttributes = {
  name: string
  category: string
  model: string
  owner: Owner
  is_free_use: boolean
  location: string
  condition: Condition
  location_status: LocationStatus
  note: string
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
