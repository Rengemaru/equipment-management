import { fireEvent, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { errorResponse, jsonResponse, stubFetch } from '../testing/fetchStub'
import { renderApp } from '../testing/renderApp'

afterEach(() => {
  vi.unstubAllGlobals()
})

const taro = { id: 1, name: '田中', login_id: 'taro', role: 'member', must_change_password: false }

function loan(over: Record<string, unknown> = {}) {
  return {
    id: 7,
    item: { code: '0042', name: '三脚' },
    user: { id: 1, name: '田中' },
    registered_by: { id: 1, name: '田中' },
    is_proxy: false,
    borrowed_at: '2026-09-10T19:30:00+09:00',
    due_date: '2026-09-24',
    returned_at: null,
    returned_by: null,
    overdue_days: 0,
    note: '',
    ...over,
  }
}

function routes(over: Record<string, () => Response> = {}) {
  return {
    '/api/me': () => jsonResponse({ user: taro, redirect_to: '/' }),
    '/api/loans/mine': () => jsonResponse({ active: [loan()], returned: [] }),
    ...over,
  }
}

test('借りているものを出す', async () => {
  stubFetch(routes())

  renderApp('/loans/mine')

  expect(await screen.findByText('三脚')).toBeDefined()
  expect(screen.getByText(/9月24日/)).toBeDefined()
})

test('借りているものが無ければその旨を出す', async () => {
  stubFetch(routes({ '/api/loans/mine': () => jsonResponse({ active: [], returned: [] }) }))

  renderApp('/loans/mine')

  expect(await screen.findByText('借りているものはありません')).toBeDefined()
})

test('返した履歴も同じ画面に出す', async () => {
  stubFetch(
    routes({
      '/api/loans/mine': () =>
        jsonResponse({
          active: [],
          returned: [
            loan({
              id: 8,
              item: { code: '0009', name: '脚立' },
              returned_at: '2026-09-12T10:00:00+09:00',
            }),
          ],
        }),
    }),
  )

  renderApp('/loans/mine')

  expect(await screen.findByText('脚立')).toBeDefined()
  expect(screen.getByText(/9月12日 10:00 に返却/)).toBeDefined()
})

// 返却に確認を挟まない。誤返却は再度借りれば済むが、確認を挟むと記録が飛ぶ。
test('確認を挟まずに返せる', async () => {
  const fetchMock = stubFetch(
    routes({
      'POST /api/items/0042/return': () => jsonResponse({ loan: loan(), item: {} }),
    }),
  )

  renderApp('/loans/mine')
  await screen.findByText('三脚')

  fireEvent.click(screen.getByRole('button', { name: '返す' }))

  await screen.findByText('三脚')
  const calls = fetchMock.mock.calls.map(
    (c) => `${(c[1] as RequestInit | undefined)?.method ?? 'GET'} ${String(c[0])}`,
  )
  expect(calls).toContain('POST /api/items/0042/return')
})

// __取り消しには確認を挟む。__ 返却と違い、記録を無かったことにする操作で、
// 押し間違いを戻せない。
test('取り消しは確認してから送る', async () => {
  const fetchMock = stubFetch(
    routes({
      'POST /api/loans/7/cancel': () => jsonResponse({ loan: loan(), item: {} }),
    }),
  )

  renderApp('/loans/mine')
  await screen.findByText('三脚')

  fireEvent.click(screen.getByRole('button', { name: '借りていない' }))

  // 押しただけでは送らない。
  let calls = fetchMock.mock.calls.map((c) => String(c[0]))
  expect(calls).not.toContain('/api/loans/7/cancel')

  fireEvent.click(screen.getByRole('button', { name: '借りていないので取り消す' }))

  await screen.findByText('三脚')
  calls = fetchMock.mock.calls.map((c) => String(c[0]))
  expect(calls).toContain('/api/loans/7/cancel')
})

// 身に覚えのない借用の出どころが分からないと、本人は何もできない。
test('代理登録なら登録者を出す', async () => {
  stubFetch(
    routes({
      '/api/loans/mine': () =>
        jsonResponse({
          active: [loan({ is_proxy: true, registered_by: { id: 9, name: '佐藤' } })],
          returned: [],
        }),
    }),
  )

  renderApp('/loans/mine')

  expect(await screen.findByText('佐藤 さんが登録しました')).toBeDefined()
})

// 二重タップと「他の人が先に返した」は異常ではない。黙って成功にもしない。
test('すでに返却済みなら読み直して伝える', async () => {
  stubFetch(
    routes({
      'POST /api/items/0042/return': () =>
        errorResponse('この備品は貸出中ではありません', 409, 'not_borrowed'),
    }),
  )

  renderApp('/loans/mine')
  await screen.findByText('三脚')

  fireEvent.click(screen.getByRole('button', { name: '返す' }))

  expect(await screen.findByText('すでに返却されています。')).toBeDefined()
})
