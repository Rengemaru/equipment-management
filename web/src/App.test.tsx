import { screen } from '@testing-library/react'
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

  renderApp('/loans')

  expect(await screen.findByRole('heading', { name: 'ページが見つかりません' })).toBeDefined()
  expect(screen.getByRole('link', { name: 'トップへ' })).toBeDefined()
})
