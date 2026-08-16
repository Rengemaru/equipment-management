import { act, fireEvent, render, screen } from '@testing-library/react'
import { useState } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

import { request } from '../api/client'
import { jsonResponse, stubFetch } from '../testing/fetchStub'
import { AuthProvider, useAuth } from './AuthProvider'

afterEach(() => {
  vi.unstubAllGlobals()
})

const taro = { id: 1, name: '田中', login_id: 'taro', role: 'member', must_change_password: false }

/** Probe は状態と操作を画面に出すだけの部品。 */
function Probe() {
  const auth = useAuth()
  const [redirectTo, setRedirectTo] = useState('')

  return (
    <div>
      <p>状態: {auth.status}</p>
      {auth.status === 'authenticated' && <p>利用者: {auth.user.name}</p>}
      {auth.status === 'unavailable' && <p>理由: {auth.message}</p>}
      {redirectTo !== '' && <p>行き先: {redirectTo}</p>}

      <button
        onClick={() => {
          void auth.login('taro', 'pw', '/i/0042').then(setRedirectTo)
        }}
      >
        ログイン
      </button>
      <button onClick={() => void auth.logout()}>ログアウト</button>
    </div>
  )
}

function renderProbe() {
  render(
    <AuthProvider>
      <Probe />
    </AuthProvider>,
  )
}

test('起動時に /api/me でログイン状態を復元する', async () => {
  stubFetch({ '/api/me': () => jsonResponse({ user: taro, redirect_to: '/' }) })

  renderProbe()

  expect(await screen.findByText('状態: authenticated')).toBeDefined()
  expect(screen.getByText('利用者: 田中')).toBeDefined()
})

test('401 なら未ログインとして扱う', async () => {
  stubFetch({ '/api/me': () => jsonResponse({ error: 'ログインしてください' }, 401) })

  renderProbe()

  expect(await screen.findByText('状態: anonymous')).toBeDefined()
})

// 繋がらないだけなのにログイン画面を出すと、利用者はIDとパスワードを
// 打ち込んで、また同じ失敗を見ることになる。
test('サーバに聞けなければ未ログインにせず理由を持つ', async () => {
  stubFetch({ '/api/me': () => jsonResponse({ error: 'サーバ側で問題が起きました' }, 500) })

  renderProbe()

  expect(await screen.findByText('状態: unavailable')).toBeDefined()
  expect(screen.getByText('理由: サーバ側で問題が起きました')).toBeDefined()
})

// 行き先はサーバが検証した redirect_to。フロントは next を解釈しない。
test('ログインすると利用者と行き先が入る', async () => {
  stubFetch({
    '/api/me': () => jsonResponse({ error: 'ログインしてください' }, 401),
    '/api/login': () => jsonResponse({ user: taro, redirect_to: '/i/0042' }),
  })

  renderProbe()
  await screen.findByText('状態: anonymous')

  fireEvent.click(screen.getByRole('button', { name: 'ログイン' }))

  expect(await screen.findByText('状態: authenticated')).toBeDefined()
  expect(screen.getByText('行き先: /i/0042')).toBeDefined()
})

test('ログアウトすると未ログインに戻る', async () => {
  stubFetch({
    '/api/me': () => jsonResponse({ user: taro, redirect_to: '/' }),
    '/api/logout': () => new Response(null, { status: 204 }),
  })

  renderProbe()
  await screen.findByText('状態: authenticated')

  fireEvent.click(screen.getByRole('button', { name: 'ログアウト' }))

  expect(await screen.findByText('状態: anonymous')).toBeDefined()
})

// サーバ側を消せなくても、この端末は未ログインとして扱う。
// 「ログアウトを押したのに何も起きない」が一番困る。
// 押した人にできることが無いため投げない。代わりに記録は残す。
test('ログアウトが失敗しても投げずに未ログインへ戻し、警告を残す', async () => {
  stubFetch({
    '/api/me': () => jsonResponse({ user: taro, redirect_to: '/' }),
    '/api/logout': () => jsonResponse({ error: 'サーバ側で問題が起きました' }, 500),
  })
  const consoleWarn = vi.spyOn(console, 'warn').mockImplementation(() => {})

  renderProbe()
  await screen.findByText('状態: authenticated')

  fireEvent.click(screen.getByRole('button', { name: 'ログアウト' }))

  expect(await screen.findByText('状態: anonymous')).toBeDefined()
  expect(consoleWarn).toHaveBeenCalled()

  consoleWarn.mockRestore()
})

// 画面ごとに 401 を拾わせると、必ず拾い忘れた画面ができ、
// そこだけ「ログインしているつもりで何も動かない」状態になる。
test('別のAPIが401を返したらログイン状態を落とす', async () => {
  stubFetch({
    '/api/me': () => jsonResponse({ user: taro, redirect_to: '/' }),
    '/api/items': () => jsonResponse({ error: 'ログインしてください' }, 401),
  })

  renderProbe()
  await screen.findByText('状態: authenticated')

  await act(async () => {
    await expect(request('/api/items')).rejects.toThrow()
  })

  expect(await screen.findByText('状態: anonymous')).toBeDefined()
})

// 被せ忘れた画面が「常に未ログイン」として静かに動くのを防ぐ。
test('Provider の外で useAuth を呼ぶと投げる', () => {
  // React が投げた例外を console.error に出す。テストの出力を汚さない。
  const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})

  expect(() => render(<Probe />)).toThrow('useAuth は AuthProvider の中で呼ぶこと')

  consoleError.mockRestore()
})
