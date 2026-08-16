import { screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { errorResponse, jsonResponse, stubFetch } from './testing/fetchStub'
import { renderApp } from './testing/renderApp'

afterEach(() => {
  vi.unstubAllGlobals()
})

const taro = { id: 1, name: '田中', login_id: 'taro', role: 'member', must_change_password: false }

test('ログイン済みならトップが表示される', async () => {
  stubFetch({ '/api/me': () => jsonResponse({ user: taro, redirect_to: '/' }) })

  renderApp('/')

  expect(await screen.findByRole('heading', { name: '備品管理' })).toBeDefined()
})

// 知らないURLで白い画面になると、QRを読み間違えた人には
// 「壊れている」としか見えない。
test('割り当てのないURLは見つからないと表示する', async () => {
  stubFetch({ '/api/me': () => errorResponse('ログインしてください', 401) })

  renderApp('/i/0042')

  expect(await screen.findByRole('heading', { name: 'ページが見つかりません' })).toBeDefined()
  expect(screen.getByRole('link', { name: 'トップへ' })).toBeDefined()
})
