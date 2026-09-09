import { fireEvent, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { errorResponse, jsonResponse, stubFetch } from './testing/fetchStub'
import { renderApp } from './testing/renderApp'

afterEach(() => {
  vi.unstubAllGlobals()
})

const taro = { id: 1, name: '田中', login_id: 'taro', role: 'member', must_change_password: false }

// M1のトップは備品一覧へのリンクだけ（m1-spec §7）。貸出まわりはM2で作る。
// 先に置くと、押せない項目が並ぶ。
test('ログイン済みならトップから備品一覧へ行ける', async () => {
  stubFetch({ '/api/me': () => jsonResponse({ user: taro, redirect_to: '/' }) })

  renderApp('/')

  expect(await screen.findByRole('heading', { name: '備品管理' })).toBeDefined()
  expect(screen.getByRole('link', { name: '備品一覧' })).toHaveProperty(
    'href',
    expect.stringContaining('/items'),
  )
})

// 知らないURLで白い画面を出さない。
// 備品コードの誤りは /i/{code} 側が扱う（ItemDetail のテスト）。
// ここに来るのは、URLそのものが変わった・打ち間違えた場合。
test('割り当てのないURLは見つからないと表示する', async () => {
  stubFetch({ '/api/me': () => errorResponse('ログインしてください', 401) })

  // 割り当てのある経路を例に使わない。__画面を足した瞬間にこのテストが__
  // __「見つからない」を確かめられなくなる__（実際に /loans を足して起きた）。
  renderApp('/no-such-screen')

  expect(await screen.findByRole('heading', { name: 'ページが見つかりません' })).toBeDefined()
  // 戻る導線はナビゲーションバーに出す（iOS と同じく、行き先の名前だけを出す）。
  expect(screen.getByRole('link', { name: 'トップ' })).toHaveProperty(
    'href',
    expect.stringMatching(/\/$/),
  )
})

// ---- ログアウト ----

// __セッションは1年もつ。__ 押し間違えた人はその場では戻れず、
// パスワードを覚えていなければ運営に再発行を頼むことになる。
// そのぶんの1タップとして確認を挟む。
test('ログアウトは1度の確認を挟む', async () => {
  const fetchMock = stubFetch({
    '/api/me': () => jsonResponse({ user: taro, redirect_to: '/' }),
    'POST /api/logout': () => new Response(null, { status: 204 }),
  })

  renderApp('/')
  await screen.findByRole('heading', { name: '備品管理' })

  fireEvent.click(screen.getByRole('button', { name: 'ログアウト' }))

  // 押しただけでは送らない。
  expect(fetchMock.mock.calls.some(([path]) => path === '/api/logout')).toBe(false)
  expect(screen.getByText(/次に使う時にもう一度パスワードが要ります/)).toBeDefined()

  fireEvent.click(screen.getByRole('button', { name: '本当にログアウトする' }))

  await vi.waitFor(() => {
    if (!fetchMock.mock.calls.some(([path]) => path === '/api/logout')) {
      throw new Error('/api/logout が呼ばれていない')
    }
  })

  // 未ログインになるので、RequireAuth がログイン画面へ送る。
  expect(await screen.findByRole('heading', { name: 'ログイン' })).toBeDefined()
})

test('やめるを押すと確認が閉じ、ログアウトしない', async () => {
  const fetchMock = stubFetch({
    '/api/me': () => jsonResponse({ user: taro, redirect_to: '/' }),
  })

  renderApp('/')
  await screen.findByRole('heading', { name: '備品管理' })

  fireEvent.click(screen.getByRole('button', { name: 'ログアウト' }))
  fireEvent.click(screen.getByRole('button', { name: 'やめる' }))

  expect(screen.getByRole('button', { name: 'ログアウト' })).toBeDefined()
  expect(fetchMock.mock.calls.some(([path]) => path === '/api/logout')).toBe(false)
  // 画面に留まっていること。
  expect(screen.getByRole('heading', { name: '備品管理' })).toBeDefined()
})

// サーバ側を消せなくても、この端末は未ログインとして扱う。
// 「ログアウトを押したのに何も起きない」が一番困る。
test('サーバが失敗してもログアウトはできる', async () => {
  stubFetch({
    '/api/me': () => jsonResponse({ user: taro, redirect_to: '/' }),
    'POST /api/logout': () => errorResponse('サーバ側で問題が起きました', 500),
  })

  renderApp('/')
  await screen.findByRole('heading', { name: '備品管理' })

  fireEvent.click(screen.getByRole('button', { name: 'ログアウト' }))
  fireEvent.click(screen.getByRole('button', { name: '本当にログアウトする' }))

  expect(await screen.findByRole('heading', { name: 'ログイン' })).toBeDefined()
})
