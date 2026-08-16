import { screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { errorResponse, jsonResponse, stubFetch } from '../testing/fetchStub'
import { renderApp } from '../testing/renderApp'

afterEach(() => {
  vi.unstubAllGlobals()
})

const taro = { id: 1, name: '田中', login_id: 'taro', role: 'member', must_change_password: false }
const initial = { ...taro, must_change_password: true }

// QRから来た人をログイン後にトップへ放り出すと、もう一度QRを読み直させる
// ことになり、その一手間が記録漏れの直接原因になる。
test('未ログインなら元の場所を next に持ってログイン画面へ送る', async () => {
  stubFetch({ '/api/me': () => errorResponse('ログインしてください', 401) })

  renderApp('/password')

  expect(await screen.findByRole('heading', { name: 'ログイン' })).toBeDefined()

  // 画面に出ないため、送信時に載ることで確かめる（Login のテストで検証する）。
  // ここでは復帰先を持ったままログイン画面に居ることだけを見る。
  expect(screen.getByLabelText('ログインID')).toBeDefined()
})

// 繋がらないだけなのにログイン画面を出すと、利用者はIDとパスワードを
// 打ち込んで、また同じ失敗を見ることになる。
test('サーバに聞けなければログイン画面に送らず理由を出す', async () => {
  stubFetch({ '/api/me': () => errorResponse('サーバ側で問題が起きました', 500) })

  renderApp('/')

  expect(await screen.findByText('サーバに接続できません。')).toBeDefined()
  expect(screen.getByText('サーバ側で問題が起きました')).toBeDefined()
  expect(screen.getByRole('button', { name: '再試行' })).toBeDefined()
  expect(screen.queryByRole('heading', { name: 'ログイン' })).toBeNull()
})

// サーバは /api/me 等以外を403で止める。画面側でも行き先を示さないと、
// 「権限がありません」だけが出て何をすればよいか分からない。
test('初期パスワードのままなら変更画面へ送る', async () => {
  stubFetch({ '/api/me': () => jsonResponse({ user: initial, redirect_to: '/' }) })

  renderApp('/')

  expect(await screen.findByRole('heading', { name: 'パスワードの変更' })).toBeDefined()
  expect(screen.getByText('初期パスワードのままです。変更するまで他の画面は使えません。')).toBeDefined()
})

test('初期パスワードのままでも変更画面自体は開ける', async () => {
  stubFetch({ '/api/me': () => jsonResponse({ user: initial, redirect_to: '/' }) })

  renderApp('/password')

  expect(await screen.findByRole('heading', { name: 'パスワードの変更' })).toBeDefined()
})

test('変更済みならそのまま画面を出す', async () => {
  stubFetch({ '/api/me': () => jsonResponse({ user: taro, redirect_to: '/' }) })

  renderApp('/')

  expect(await screen.findByRole('heading', { name: '備品管理' })).toBeDefined()
})
