import { fireEvent, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { errorResponse, jsonResponse, stubFetch } from '../testing/fetchStub'
import { renderApp } from '../testing/renderApp'

afterEach(() => {
  vi.unstubAllGlobals()
})

const admin = { id: 1, name: '運営', login_id: 'admin', role: 'admin', must_change_password: false }

function user(over: Record<string, unknown> = {}) {
  return {
    id: 2,
    name: '田中',
    login_id: 'taro',
    email: '',
    role: 'member',
    is_active: true,
    must_change_password: false,
    created_at: '2026-08-16 00:00:00',
    ...over,
  }
}

function routes(over: Record<string, () => Response> = {}) {
  return {
    '/api/me': () => jsonResponse({ user: admin, redirect_to: '/' }),
    '/api/users': () => jsonResponse({ users: [user()] }),
    ...over,
  }
}

/** created は作成の応答。初期パスワードはここでしか手に入らない。 */
const created = () =>
  jsonResponse(
    {
      user: user({ id: 3, name: '佐藤', login_id: 'sato' }),
      initial_password: 'AbCd1234EfGh5678',
    },
    201,
  )

/** fillNewUser は追加フォームを開いて必須項目を埋める。 */
function fillNewUser(name: string, loginID: string) {
  fireEvent.click(screen.getByRole('button', { name: 'ユーザーを追加' }))
  fireEvent.change(screen.getByLabelText('名前'), { target: { value: name } })
  fireEvent.change(screen.getByLabelText('ログインID'), { target: { value: loginID } })
}

function postBody(fetchMock: ReturnType<typeof stubFetch>, path: string) {
  const call = fetchMock.mock.calls.find(([p, init]) => p === path && init?.method === 'POST')
  return call === undefined ? null : (JSON.parse(String(call[1]?.body)) as Record<string, unknown>)
}

test('利用者を一覧する', async () => {
  stubFetch(routes())

  renderApp('/admin/users')

  expect(await screen.findByText('田中')).toBeDefined()
  expect(screen.getByText('taro')).toBeDefined()
})

test('運営・無効・初期パスワードのままを目印で出す', async () => {
  stubFetch(
    routes({
      '/api/users': () =>
        jsonResponse({
          users: [user({ role: 'admin', is_active: false, must_change_password: true })],
        }),
    }),
  )

  renderApp('/admin/users')

  expect(await screen.findByText('運営')).toBeDefined()
  expect(screen.getByText('無効')).toBeDefined()
  expect(screen.getByText('初期パスワードのまま')).toBeDefined()
})

// __パスワードは運営に考えさせない。__ 全員に同じものが配られることになる。
test('追加時にパスワードを入力させない', async () => {
  stubFetch(routes())

  renderApp('/admin/users')
  await screen.findByText('田中')

  fireEvent.click(screen.getByRole('button', { name: 'ユーザーを追加' }))

  expect(screen.queryByLabelText('パスワード')).toBeNull()
  expect(screen.getByText(/自動で発行され/)).toBeDefined()
})

test('追加すると入力どおりに送り、一覧にも足す', async () => {
  const fetchMock = stubFetch(routes({ 'POST /api/users': created }))

  renderApp('/admin/users')
  await screen.findByText('田中')

  fillNewUser('佐藤', 'sato')
  fireEvent.change(screen.getByLabelText('権限'), { target: { value: 'admin' } })
  fireEvent.click(screen.getByRole('button', { name: '追加する' }))

  await screen.findByText('AbCd1234EfGh5678')

  expect(postBody(fetchMock, '/api/users')).toEqual({
    name: '佐藤',
    login_id: 'sato',
    email: '',
    role: 'admin',
  })
  // 一覧を取り直さず、返ってきた1人を足す。
  expect(screen.getByText('佐藤')).toBeDefined()
})

// __初期パスワードはこの応答でしか手に入らない。__ DBにはハッシュしか無い。
test('初期パスワードは控えるまで消さない', async () => {
  stubFetch(routes({ 'POST /api/users': created }))

  renderApp('/admin/users')
  await screen.findByText('田中')

  fillNewUser('佐藤', 'sato')
  fireEvent.click(screen.getByRole('button', { name: '追加する' }))

  await screen.findByText('AbCd1234EfGh5678')
  expect(screen.getByText(/二度と確認できません/)).toBeDefined()

  fireEvent.click(screen.getByRole('button', { name: '控えました' }))

  expect(screen.queryByText('AbCd1234EfGh5678')).toBeNull()
})

test('追加に失敗したら理由を出し、フォームに留まる', async () => {
  stubFetch(
    routes({
      'POST /api/users': () => errorResponse('そのログインIDは既に使われている', 409),
    }),
  )

  renderApp('/admin/users')
  await screen.findByText('田中')

  fillNewUser('佐藤', 'taro')
  fireEvent.click(screen.getByRole('button', { name: '追加する' }))

  expect(await screen.findByRole('alert')).toHaveProperty(
    'textContent',
    'そのログインIDは既に使われている',
  )
  expect(screen.getByRole('button', { name: '追加する' })).toBeDefined()
})

// 利用者は消さない。卒業者は無効化する。消すと貸出履歴が壊れる（CLAUDE.md）。
test('削除の導線を置かず、無効化できる', async () => {
  const fetchMock = stubFetch(
    routes({
      '/api/users/2/deactivate': () => jsonResponse({ user: user({ is_active: false }) }),
    }),
  )

  renderApp('/admin/users')
  await screen.findByText('田中')

  expect(screen.queryByRole('button', { name: /削除/ })).toBeNull()

  fireEvent.click(screen.getByRole('button', { name: '無効にする' }))

  expect(await screen.findByText('無効')).toBeDefined()
  expect(screen.getByRole('button', { name: '有効にする' })).toBeDefined()
  expect(
    fetchMock.mock.calls.some(([p, init]) => p === '/api/users/2/deactivate' && init?.method === 'POST'),
  ).toBe(true)
})

// __最後の admin は無効化できない。__ 全員無効になるとWebから復旧できなくなる。
test('最後の運営を無効化しようとしたら理由を出す', async () => {
  stubFetch(
    routes({
      '/api/users/2/deactivate': () =>
        errorResponse('最後の管理者は無効にできない', 409),
    }),
  )

  renderApp('/admin/users')
  await screen.findByText('田中')

  fireEvent.click(screen.getByRole('button', { name: '無効にする' }))

  expect(await screen.findByRole('alert')).toHaveProperty(
    'textContent',
    '最後の管理者は無効にできない',
  )
  // 無効になっていない。
  expect(screen.getByRole('button', { name: '無効にする' })).toBeDefined()
})

// 自己リセットの導線は作らない（メールに依存するため）。入れなくなった人は運営に頼む。
test('パスワードを再発行すると一度だけ表示する', async () => {
  stubFetch(
    routes({
      '/api/users/2/reset-password': () =>
        jsonResponse({
          user: user({ must_change_password: true }),
          initial_password: 'ZzYy9876XxWw5432',
        }),
    }),
  )

  renderApp('/admin/users')
  await screen.findByText('田中')

  fireEvent.click(screen.getByRole('button', { name: 'パスワードを再発行' }))

  expect(await screen.findByText('ZzYy9876XxWw5432')).toBeDefined()
  expect(screen.getByText('初期パスワードのまま')).toBeDefined()
})

test('取得に失敗したら理由を出す', async () => {
  stubFetch(routes({ '/api/users': () => errorResponse('サーバ側で問題が起きました', 500) }))

  renderApp('/admin/users')

  expect(await screen.findByRole('alert')).toHaveProperty(
    'textContent',
    'サーバ側で問題が起きました',
  )
})

test('member には画面を出さない', async () => {
  stubFetch(
    routes({
      '/api/me': () =>
        jsonResponse({
          user: { ...admin, id: 2, role: 'member' },
          redirect_to: '/',
        }),
    }),
  )

  renderApp('/admin/users')

  expect(await screen.findByRole('heading', { name: 'この画面は運営のみが使えます' })).toBeDefined()
})

test('運営にはトップからユーザー管理へのリンクを出す', async () => {
  stubFetch({ '/api/me': () => jsonResponse({ user: admin, redirect_to: '/' }) })

  renderApp('/')
  await screen.findByRole('heading', { name: '備品管理' })

  expect(screen.getByRole('link', { name: 'ユーザー管理' })).toHaveProperty(
    'href',
    expect.stringContaining('/admin/users'),
  )
})

test('member にはトップのユーザー管理リンクを出さない', async () => {
  stubFetch({
    '/api/me': () => jsonResponse({ user: { ...admin, role: 'member' }, redirect_to: '/' }),
  })

  renderApp('/')
  await screen.findByRole('heading', { name: '備品管理' })

  expect(screen.queryByRole('link', { name: 'ユーザー管理' })).toBeNull()
})
