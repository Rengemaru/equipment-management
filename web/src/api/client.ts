/**
 * API クライアントの土台。
 *
 * ここには「どのエンドポイントでも同じこと」だけを書く。経路ごとの関数は
 * 用途別のファイル（`auth.ts` など）に置き、画面を作る時に足していく。
 */

/**
 * ApiError は API の呼び出しが失敗したこと。
 *
 * エラーの形はサーバ側で `httpx.ErrorResponse` の1つに決めてある。
 * こちら側も1つの型に潰す。エンドポイントごとに違う型を作ると、
 * 呼び出し側が経路の数だけ分岐を持つことになる。
 */
export class ApiError extends Error {
  /**
   * status は HTTP のステータス。
   *
   * 通信自体が成立しなかった場合は 0。サーバが返した 4xx/5xx と
   * 「そもそも届かなかった」を呼び出し側で区別できるようにする。
   */
  readonly status: number

  /**
   * code は分岐用の識別子（`httpx.ErrorResponse.Code`）。無い場合は空文字。
   *
   * 文言で分岐しないこと。日本語を直した瞬間に分岐が壊れる。
   */
  readonly code: string

  constructor(status: number, message: string, code = '') {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

/** SessionExpiredListener はセッションが切れたことを受け取る。 */
type SessionExpiredListener = () => void

const sessionExpiredListeners = new Set<SessionExpiredListener>()

/**
 * onSessionExpired は 401 が返った時に呼ばれる関数を登録する。戻り値で解除する。
 *
 * セッションの有効期限は1年だが、無効化・パスワード変更・DBの入れ替えで
 * いつでも切れる。画面ごとに 401 を拾わせると、必ず拾い忘れた画面ができ、
 * そこだけ「ログインしているつもりで何も動かない」状態になる。
 */
export function onSessionExpired(listener: SessionExpiredListener): () => void {
  sessionExpiredListeners.add(listener)
  return () => {
    sessionExpiredListeners.delete(listener)
  }
}

/**
 * request は API を呼び、JSON を返す。
 *
 * 失敗は全て ApiError として投げる。戻り値で成否を返す形にすると、
 * 呼び出し側が確認を忘れた時に、エラーの本文を成功データとして扱ってしまう。
 */
export async function request<T>(path: string, init?: RequestInit): Promise<T> {
  let res: Response
  try {
    res = await fetch(path, {
      ...init,
      // セッションは Cookie で持つ。fetch の既定でも同一オリジンなら送られるが、
      // 明示しておく。開発時は Vite が /api を Go サーバへ中継するため、
      // ブラウザから見れば開発でも本番でも同一オリジンになる。
      credentials: 'same-origin',
    })
  } catch {
    // 部室のネットワークは不安定な前提。「壊れた」ではなく
    // 「今つながらない」と分かる文言にする。
    throw new ApiError(0, 'サーバに接続できませんでした。通信環境を確認してください')
  }

  if (!res.ok) {
    const err = await toApiError(res)

    // ログイン画面で誤ったパスワードを送った時もここを通る。
    // 未ログインの利用者に対しては、受け取る側が何もしない作りにしてある。
    if (res.status === 401) {
      notifySessionExpired()
    }

    throw err
  }

  return (await decodeBody<T>(res)) as T
}

/**
 * requestJSON は JSON を送って JSON を受け取る。
 *
 * サーバは知らない項目を 400 で弾く（`httpx.DecodeJSON` の
 * DisallowUnknownFields）。綴り違いは黙って無視されず、その場で分かる。
 */
export function requestJSON<T>(path: string, method: string, body: unknown): Promise<T> {
  return request<T>(path, {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
}

/** notifySessionExpired は登録された関数を全て呼ぶ。 */
function notifySessionExpired(): void {
  for (const listener of sessionExpiredListeners) {
    listener()
  }
}

/**
 * toApiError は失敗した応答を ApiError に変える。
 *
 * 本文が JSON とは限らない。Vite の中継が落ちている時や、サーバの手前で
 * 止められた時は HTML が返る。それを本文として画面に出すと、利用者には
 * タグの混じった文字列が見えることになる。
 */
async function toApiError(res: Response): Promise<ApiError> {
  let body: unknown
  try {
    body = await res.json()
  } catch {
    return new ApiError(res.status, fallbackMessage(res.status))
  }

  if (typeof body !== 'object' || body === null) {
    return new ApiError(res.status, fallbackMessage(res.status))
  }

  const { error, code } = body as { error?: unknown; code?: unknown }

  return new ApiError(
    res.status,
    typeof error === 'string' && error !== '' ? error : fallbackMessage(res.status),
    typeof code === 'string' ? code : '',
  )
}

/** fallbackMessage は本文から文言を取れなかった時に見せる文言。 */
function fallbackMessage(status: number): string {
  if (status >= 500) {
    return 'サーバ側で問題が起きました'
  }
  if (status === 401) {
    return 'ログインしてください'
  }
  if (status === 403) {
    return 'この操作は許可されていません'
  }
  if (status === 404) {
    return '見つかりませんでした'
  }
  return '操作できませんでした'
}

/**
 * decodeBody は本文を読む。
 *
 * 本文を返さない経路がある（`POST /api/logout` は 204）。無条件に
 * JSON として読むと、成功しているのに例外になる。
 */
async function decodeBody<T>(res: Response): Promise<T | undefined> {
  if (res.status === 204) {
    return undefined
  }

  const text = await res.text()
  if (text === '') {
    return undefined
  }

  return JSON.parse(text) as T
}
