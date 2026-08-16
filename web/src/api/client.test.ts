import { afterEach, expect, test, vi } from 'vitest'

import { ApiError, onSessionExpired, request, requestJSON } from './client'

afterEach(() => {
  vi.unstubAllGlobals()
})

/** stubFetch は経路ごとの応答を差し替える。 */
function stubFetch(routes: Record<string, () => Response>) {
  const fn = vi.fn(async (path: string, _init?: RequestInit) => {
    const route = routes[path]
    if (!route) {
      // 経路を書き忘れたテストが「通信できなかった」に化けないよう、
      // 応答として返す。ApiError の message に経路が出る。
      return new Response(JSON.stringify({ error: `想定していない経路: ${path}` }), { status: 599 })
    }
    return route()
  })
  vi.stubGlobal('fetch', fn)
  return fn
}

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

test('成功した応答を JSON として返す', async () => {
  stubFetch({ '/api/me': () => jsonResponse({ user: { name: '田中' } }) })

  await expect(request('/api/me')).resolves.toEqual({ user: { name: '田中' } })
})

// POST /api/logout は 204 を返す。無条件に JSON として読むと、
// 成功しているのに例外になる。
test('本文の無い応答でも例外にしない', async () => {
  stubFetch({ '/api/logout': () => new Response(null, { status: 204 }) })

  await expect(request('/api/logout', { method: 'POST' })).resolves.toBeUndefined()
})

test('セッションは Cookie で送る', async () => {
  const fetchMock = stubFetch({ '/api/me': () => jsonResponse({}) })

  await request('/api/me')

  expect(fetchMock.mock.calls[0]?.[1]).toMatchObject({ credentials: 'same-origin' })
})

test('エラーの本文から文言と識別子を取り出す', async () => {
  stubFetch({
    '/api/items': () =>
      jsonResponse({ error: '初期パスワードのままです', code: 'password_change_required' }, 403),
  })

  const err = await request('/api/items').catch((e: unknown) => e)

  expect(err).toBeInstanceOf(ApiError)
  expect(err).toMatchObject({
    status: 403,
    message: '初期パスワードのままです',
    code: 'password_change_required',
  })
})

// Vite の中継が落ちている時などは HTML が返る。本文をそのまま画面に出すと、
// 利用者にはタグの混じった文字列が見えることになる。
test('本文が JSON でなければ既定の文言にする', async () => {
  stubFetch({
    '/api/items': () =>
      new Response('<html><body>502 Bad Gateway</body></html>', {
        status: 502,
        headers: { 'Content-Type': 'text/html' },
      }),
  })

  const err = await request('/api/items').catch((e: unknown) => e)

  expect(err).toMatchObject({ status: 502, message: 'サーバ側で問題が起きました', code: '' })
})

// 部室のネットワークは不安定な前提。サーバが返した 4xx/5xx と
// 「そもそも届かなかった」を呼び出し側が区別できる必要がある。
test('通信できなければ status 0 で投げる', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(() => Promise.reject(new TypeError('Failed to fetch'))),
  )

  const err = await request('/api/me').catch((e: unknown) => e)

  expect(err).toBeInstanceOf(ApiError)
  expect(err).toMatchObject({ status: 0 })
})

test('401 を受け取ると登録した関数を呼ぶ', async () => {
  stubFetch({ '/api/items': () => jsonResponse({ error: 'ログインしてください' }, 401) })

  const listener = vi.fn()
  const unsubscribe = onSessionExpired(listener)

  await expect(request('/api/items')).rejects.toThrow()

  expect(listener).toHaveBeenCalledTimes(1)
  unsubscribe()
})

test('解除した関数は呼ばれない', async () => {
  stubFetch({ '/api/items': () => jsonResponse({ error: 'ログインしてください' }, 401) })

  const listener = vi.fn()
  onSessionExpired(listener)()

  await expect(request('/api/items')).rejects.toThrow()

  expect(listener).not.toHaveBeenCalled()
})

test('requestJSON は JSON として送る', async () => {
  const fetchMock = stubFetch({ '/api/login': () => jsonResponse({}) })

  await requestJSON('/api/login', 'POST', { login_id: 'taro' })

  const init = fetchMock.mock.calls[0]?.[1] as RequestInit
  expect(init.method).toBe('POST')
  expect(init.headers).toMatchObject({ 'Content-Type': 'application/json' })
  expect(init.body).toBe('{"login_id":"taro"}')
})
