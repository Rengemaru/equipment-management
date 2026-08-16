import { fireEvent, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { errorResponse, jsonResponse, stubFetch } from '../testing/fetchStub'
import { renderApp } from '../testing/renderApp'

afterEach(() => {
  vi.unstubAllGlobals()
})

const taro = { id: 1, name: '田中', login_id: 'taro', role: 'member', must_change_password: false }
const initial = { ...taro, must_change_password: true }

/** submit はフォームに入力して送信する。 */
function submit(current: string, next: string, confirm: string) {
  fireEvent.change(screen.getByLabelText('現在のパスワード'), { target: { value: current } })
  fireEvent.change(screen.getByLabelText('新しいパスワード'), { target: { value: next } })
  fireEvent.change(screen.getByLabelText('新しいパスワード（確認）'), {
    target: { value: confirm },
  })
  fireEvent.click(screen.getByRole('button', { name: '変更する' }))
}

test('成功するとサーバが示した行き先へ進む', async () => {
  stubFetch({
    '/api/me': () => jsonResponse({ user: initial, redirect_to: '/' }),
    '/api/password': () => jsonResponse({ user: taro, redirect_to: '/' }),
  })

  renderApp('/password')
  await screen.findByRole('heading', { name: 'パスワードの変更' })

  submit('initial-pw', 'new-password', 'new-password')

  expect(await screen.findByRole('heading', { name: '備品管理' })).toBeDefined()
})

// 入力の取り違えはサーバには分からない。ここでしか確認できない。
test('確認が一致しなければサーバへ送らない', async () => {
  const fetchMock = stubFetch({
    '/api/me': () => jsonResponse({ user: taro, redirect_to: '/' }),
  })

  renderApp('/password')
  await screen.findByRole('heading', { name: 'パスワードの変更' })

  submit('current-pw', 'new-password', 'new-passward')

  expect(await screen.findByRole('alert')).toHaveProperty(
    'textContent',
    '新しいパスワードが一致しません',
  )
  expect(fetchMock.mock.calls.some(([path]) => path === '/api/password')).toBe(false)
})

// 長さなどの規則はサーバが持つ。画面に書き写すと、片方だけ直した時に
// 「画面は通すのに登録できない」状態になる。
test('サーバが弾いた理由をそのまま出す', async () => {
  stubFetch({
    '/api/me': () => jsonResponse({ user: taro, redirect_to: '/' }),
    '/api/password': () => errorResponse('パスワードは8文字以上にする', 400),
  })

  renderApp('/password')
  await screen.findByRole('heading', { name: 'パスワードの変更' })

  submit('current-pw', 'short', 'short')

  expect(await screen.findByRole('alert')).toHaveProperty(
    'textContent',
    'パスワードは8文字以上にする',
  )
})

// __ここが 401 でログアウト扱いになると、打ち間違えただけの人が__
// __その場で締め出される。__ セッションは生きている。
test('現在のパスワードを間違えてもログアウトさせない', async () => {
  stubFetch({
    '/api/me': () => jsonResponse({ user: taro, redirect_to: '/' }),
    '/api/password': () => errorResponse('現在のパスワードが違います', 401),
  })

  renderApp('/password')
  await screen.findByRole('heading', { name: 'パスワードの変更' })

  submit('wrong-pw', 'new-password', 'new-password')

  expect(await screen.findByRole('alert')).toHaveProperty(
    'textContent',
    '現在のパスワードが違います',
  )
  // ログイン画面へ飛ばされていない = セッションは保たれている。
  expect(screen.getByRole('heading', { name: 'パスワードの変更' })).toBeDefined()
  expect(screen.queryByRole('heading', { name: 'ログイン' })).toBeNull()
})

// 強制されて来た人に「なぜこの画面なのか」を出さないと、
// 操作を誤ったように見える。
test('初期パスワードのままなら理由を出す', async () => {
  stubFetch({ '/api/me': () => jsonResponse({ user: initial, redirect_to: '/' }) })

  renderApp('/password')

  expect(
    await screen.findByText('初期パスワードのままです。変更するまで他の画面は使えません。'),
  ).toBeDefined()
})

test('変更済みなら理由を出さない', async () => {
  stubFetch({ '/api/me': () => jsonResponse({ user: taro, redirect_to: '/' }) })

  renderApp('/password')
  await screen.findByRole('heading', { name: 'パスワードの変更' })

  expect(
    screen.queryByText('初期パスワードのままです。変更するまで他の画面は使えません。'),
  ).toBeNull()
})
