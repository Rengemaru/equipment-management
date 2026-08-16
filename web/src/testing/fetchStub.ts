/**
 * テスト用の fetch 差し替え。
 *
 * 本番のコードからは読み込まれない（テストからしか import しない）。
 * 各テストファイルに同じものを書くと、応答の作り方が少しずつずれて
 * 「あちらでは通るのにこちらでは落ちる」の原因になる。
 */

import { vi } from 'vitest'

/** jsonResponse は JSON を返す応答を作る。 */
export function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

/** errorResponse は API のエラー応答を作る。 */
export function errorResponse(message: string, status: number, code?: string): Response {
  return jsonResponse(code === undefined ? { error: message } : { error: message, code }, status)
}

/**
 * stubFetch は経路ごとの応答を差し替える。
 *
 * 鍵はクエリ文字列を除いたパス。条件の組み合わせごとに鍵を書くことになると、
 * 絞り込みのテストが書けない。送ったクエリは戻り値の mock.calls で確かめる。
 *
 * `POST /api/users` のようにメソッドを付けた鍵を先に見る。同じパスで
 * GET と POST の応答が違う経路（一覧と作成）を書き分けるため。
 *
 * 書き忘れた経路は 599 として返す。例外として投げると、クライアントが
 * 「通信できなかった」に変換してしまい、経路の書き忘れが
 * ネットワーク障害のテストに化ける。
 */
export function stubFetch(routes: Record<string, () => Response>) {
  const fn = vi.fn(async (path: string, init?: RequestInit) => {
    const withoutQuery = path.split('?')[0] ?? path
    const method = init?.method ?? 'GET'

    const route = routes[`${method} ${withoutQuery}`] ?? routes[withoutQuery]
    if (!route) {
      return jsonResponse({ error: `想定していない経路: ${method} ${path}` }, 599)
    }
    return route()
  })

  vi.stubGlobal('fetch', fn)
  return fn
}
