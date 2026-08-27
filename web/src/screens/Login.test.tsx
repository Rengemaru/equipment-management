import { fireEvent, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { errorResponse, jsonResponse, stubFetch } from '../testing/fetchStub'
import { renderApp } from '../testing/renderApp'

afterEach(() => {
  vi.unstubAllGlobals()
})

const taro = { id: 1, name: '田中', login_id: 'taro', role: 'member', must_change_password: false }

const anonymous = () => errorResponse('ログインしてください', 401)

/** submit はフォームに入力して送信する。 */
function submit(loginID: string, password: string) {
  fireEvent.change(screen.getByLabelText('ログインID'), { target: { value: loginID } })
  fireEvent.change(screen.getByLabelText('パスワード'), { target: { value: password } })
  fireEvent.click(screen.getByRole('button', { name: 'ログイン' }))
}

test('フォームを表示する', async () => {
  stubFetch({ '/api/me': anonymous })

  renderApp('/login')

  expect(await screen.findByRole('heading', { name: 'ログイン' })).toBeDefined()
  expect(screen.getByLabelText('ログインID')).toBeDefined()
  expect(screen.getByLabelText('パスワード')).toBeDefined()
})

test('成功するとサーバが示した行き先へ進む', async () => {
  stubFetch({
    '/api/me': anonymous,
    '/api/login': () => jsonResponse({ user: taro, redirect_to: '/' }),
  })

  renderApp('/login')
  await screen.findByRole('heading', { name: 'ログイン' })

  submit('taro', 'password123')

  expect(await screen.findByRole('heading', { name: '備品管理' })).toBeDefined()
})

// next の安全性はサーバが判断する。フロントは受け取った値をそのまま渡し、
// 応答の redirect_to へ進む。判断を2箇所に分けない。
test('next をそのままサーバへ渡す', async () => {
  const fetchMock = stubFetch({
    '/api/me': anonymous,
    '/api/login': () => jsonResponse({ user: taro, redirect_to: '/' }),
  })

  renderApp('/login?next=%2Fi%2F0042')
  await screen.findByRole('heading', { name: 'ログイン' })

  submit('taro', 'password123')
  await screen.findByRole('heading', { name: '備品管理' })

  const call = fetchMock.mock.calls.find(([path]) => path === '/api/login')
  expect(JSON.parse(String(call?.[1]?.body))).toEqual({
    login_id: 'taro',
    password: 'password123',
    next: '/i/0042',
  })
})

// 存在しないIDと誤ったパスワードで文言を変えない（m1-spec §3）。
// サーバが1つの文言に潰しているので、こちらはそのまま出す。
test('失敗するとサーバの文言を出し、画面に留まる', async () => {
  stubFetch({
    '/api/me': anonymous,
    '/api/login': () => errorResponse('ログインIDまたはパスワードが違います', 401),
  })

  renderApp('/login')
  await screen.findByRole('heading', { name: 'ログイン' })

  submit('taro', 'wrong')

  expect(await screen.findByRole('alert')).toHaveProperty(
    'textContent',
    'ログインIDまたはパスワードが違います',
  )
  expect(screen.getByRole('heading', { name: 'ログイン' })).toBeDefined()
})

// 部室のネットワークは不安定な前提。「壊れた」ではなく
// 「今つながらない」と分かる必要がある。
test('通信できない時もその旨を出す', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (path: string) => {
      if (path === '/api/login') {
        throw new TypeError('Failed to fetch')
      }
      return anonymous()
    }),
  )

  renderApp('/login')
  await screen.findByRole('heading', { name: 'ログイン' })

  submit('taro', 'password123')

  expect(await screen.findByRole('alert')).toHaveProperty(
    'textContent',
    'サーバに接続できませんでした。通信環境を確認してください',
  )
})

test('ログイン済みならトップへ送る', async () => {
  stubFetch({ '/api/me': () => jsonResponse({ user: taro, redirect_to: '/' }) })

  renderApp('/login')

  expect(await screen.findByRole('heading', { name: '備品管理' })).toBeDefined()
})
