import { afterEach, expect, test, vi } from 'vitest'

import { jsonResponse, stubFetch } from '../testing/fetchStub'
import { ApiError, onSessionExpired, request, requestJSON } from './client'

afterEach(() => {
  vi.unstubAllGlobals()
})

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

// ログインとパスワード変更の 401 は「入力が違う」であって
// 「セッションが切れた」ではない。区別しないと、現在のパスワードを
// 打ち間違えた人がその場でログアウトさせられる。
test('資格情報を検証する経路の401ではセッション切れにしない', async () => {
  stubFetch({ '/api/password': () => jsonResponse({ error: '現在のパスワードが違います' }, 401) })

  const listener = vi.fn()
  const unsubscribe = onSessionExpired(listener)

  await expect(
    request('/api/password', { method: 'POST' }, { verifiesCredentials: true }),
  ).rejects.toThrow()

  expect(listener).not.toHaveBeenCalled()
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
