import { fireEvent, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { errorResponse, jsonResponse, stubFetch } from '../testing/fetchStub'
import { renderApp } from '../testing/renderApp'

afterEach(() => {
  vi.unstubAllGlobals()
})

const taro = { id: 1, name: '田中', login_id: 'taro', role: 'member', must_change_password: false }

function item(over: Record<string, unknown> = {}) {
  return {
    id: 1,
    code: '0042',
    name: '三脚',
    category: '撮影機材',
    model: '',
    owner: 'サークル',
    is_free_use: false,
    location: '部室A棚',
    condition: '良好',
    location_status: '在庫',
    photo_url: '',
    note: '脚のロックが緩い',
    updated_at: '2026-08-16 00:00:00',
    ...over,
  }
}

function routes(over: Record<string, () => Response> = {}) {
  return {
    '/api/me': () => jsonResponse({ user: taro, redirect_to: '/' }),
    '/api/items/0042': () => jsonResponse({ item: item() }),
    '/api/members': () =>
      jsonResponse({
        members: [
          { id: 1, name: '田中' },
          { id: 9, name: '佐藤' },
        ],
      }),
    'POST /api/items/0042/loans': () => jsonResponse({ loan: { id: 1 }, item: item() }, 201),
    ...over,
  }
}

/** 借用日時を送っていれば、その本文を返す。 */
function borrowBody(fetchMock: ReturnType<typeof stubFetch>): unknown {
  const call = fetchMock.mock.calls.find((c) => String(c[0]) === '/api/items/0042/loans')
  const body = (call?.[1] as RequestInit | undefined)?.body
  return body === undefined ? undefined : JSON.parse(String(body))
}

// __借りる前に現状を見せる。__ 報告されないまま次の人が借りるケースが必ず出る。
// 現状表示がないと、前の人の破損が次の人の責任になる。
test('借りる前に状態と備考を出す', async () => {
  stubFetch(routes())

  renderApp('/i/0042/borrow')

  expect(await screen.findByText('脚のロックが緩い')).toBeDefined()
  expect(screen.getByText('良好')).toBeDefined()
})

// 返却予定日を触らずに借りられることが、3タップで終わらせる前提になる。
test('何も触らずに借りると本文が空になる', async () => {
  const fetchMock = stubFetch(routes())

  renderApp('/i/0042/borrow')
  await screen.findByText('脚のロックが緩い')

  fireEvent.click(screen.getByRole('button', { name: 'この内容で借りる' }))

  await screen.findByText('脚のロックが緩い')
  expect(borrowBody(fetchMock)).toEqual({})
})

test('返却予定日の既定を画面に出す', async () => {
  stubFetch(routes())

  renderApp('/i/0042/borrow')

  expect(await screen.findByText(/返却予定日は/)).toBeDefined()
})

// __代理登録は「変更する」の内側。__ 既定のまま進む人のタップ数を増やさない。
test('代理登録は変更するの内側に置く', async () => {
  stubFetch(routes())

  renderApp('/i/0042/borrow')
  await screen.findByText('脚のロックが緩い')

  expect(screen.queryByLabelText('借りる人')).toBeNull()

  fireEvent.click(screen.getByRole('button', { name: '返却予定日や借用日時を変更する' }))

  expect(await screen.findByLabelText('借りる人')).toBeDefined()
})

test('他の人の借用として登録できる', async () => {
  const fetchMock = stubFetch(routes())

  renderApp('/i/0042/borrow')
  await screen.findByText('脚のロックが緩い')

  fireEvent.click(screen.getByRole('button', { name: '返却予定日や借用日時を変更する' }))
  const select = await screen.findByLabelText('借りる人')
  fireEvent.change(select, { target: { value: '9' } })

  // 本人に通知が飛ぶことを、押す前に伝える。
  expect(screen.getByText(/佐藤さんの借用として記録し、本人にメールで知らせます。/)).toBeDefined()

  fireEvent.click(screen.getByRole('button', { name: 'この内容で登録する' }))

  await screen.findByText('脚のロックが緩い')
  expect(borrowBody(fetchMock)).toMatchObject({ user_id: 9 })
})

// 自分を選び直す必要はない。既定が自分なので、一覧に自分を出すと迷わせる。
test('選択肢に自分を出さない', async () => {
  stubFetch(routes())

  renderApp('/i/0042/borrow')
  await screen.findByText('脚のロックが緩い')

  fireEvent.click(screen.getByRole('button', { name: '返却予定日や借用日時を変更する' }))
  const select = await screen.findByLabelText('借りる人')

  const labels = Array.from(select.querySelectorAll('option')).map((o) => o.textContent)
  expect(labels).toEqual(['自分', '佐藤'])
})

// 競合は異常ではない。2人が同時に同じ備品を借りようとしただけ。
test('先に借りられていたらそう伝える', async () => {
  stubFetch(
    routes({
      'POST /api/items/0042/loans': () =>
        errorResponse('他の人が先に借りました', 409, 'already_borrowed'),
    }),
  )

  renderApp('/i/0042/borrow')
  await screen.findByText('脚のロックが緩い')

  fireEvent.click(screen.getByRole('button', { name: 'この内容で借りる' }))

  expect(await screen.findByText(/他の人が先に借りました/)).toBeDefined()
})

// 所在不明でも借りられる。借りた時点で在庫に戻るので、報告を求めない。
test('所在不明なら借りると在庫に戻ることを伝える', async () => {
  stubFetch(
    routes({
      '/api/items/0042': () =>
        jsonResponse({ item: item({ location_status: '所在不明_未確認' }) }),
    }),
  )

  renderApp('/i/0042/borrow')

  expect(await screen.findByText(/借りると在庫に戻ります/)).toBeDefined()
})
