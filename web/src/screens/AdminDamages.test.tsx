import { fireEvent, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { jsonResponse, stubFetch } from '../testing/fetchStub'
import { renderApp } from '../testing/renderApp'

afterEach(() => {
  vi.unstubAllGlobals()
})

const admin = { id: 1, name: '運営', login_id: 'admin', role: 'admin', must_change_password: false }
const member = {
  id: 2,
  name: '田中',
  login_id: 'taro',
  role: 'member',
  must_change_password: false,
}

function report(over: Record<string, unknown> = {}) {
  return {
    id: 5,
    item: { code: '0042', name: '三脚' },
    loan_id: null,
    reporter: { id: 2, name: '田中' },
    reported_at: '2026-09-10T19:30:00+09:00',
    description: '脚のロックが割れている',
    status: '未確認',
    confirmed_by: null,
    confirmed_at: null,
    note: '',
    ...over,
  }
}

function routes(over: Record<string, () => Response> = {}) {
  return {
    '/api/me': () => jsonResponse({ user: admin, redirect_to: '/' }),
    '/api/damages': () => jsonResponse({ reports: [report()] }),
    ...over,
  }
}

test('未確認の報告を一覧する', async () => {
  const fetchMock = stubFetch(routes())

  renderApp('/admin/damages')

  expect(await screen.findByText('脚のロックが割れている')).toBeDefined()
  expect(screen.getByText(/田中 さん/)).toBeDefined()

  // 既定は未確認だけ。全件を既定にすると処理済みが積み上がって埋もれる。
  const paths = fetchMock.mock.calls.map((c) => String(c[0]))
  expect(paths.some((p) => p.includes('status=%E6%9C%AA%E7%A2%BA%E8%AA%8D'))).toBe(true)
})

// __画面で隠すだけにしない__のが原則だが、押した先で断られるだけの導線は出さない。
test('memberは開けない', async () => {
  stubFetch(routes({ '/api/me': () => jsonResponse({ user: member, redirect_to: '/' }) }))

  renderApp('/admin/damages')

  expect(
    await screen.findByRole('heading', { name: 'この画面は運営のみが使えます' }),
  ).toBeDefined()
  expect(screen.queryByText('脚のロックが割れている')).toBeNull()
})

// 状態を変えると備品の状態が連動する。__押す前にその結果を書く。__
test('連動する結果を押す前に書く', async () => {
  stubFetch(routes())

  renderApp('/admin/damages')
  await screen.findByText('脚のロックが割れている')

  expect(screen.getByText(/「修理済み」にすると備品の状態が/)).toBeDefined()
})

test('修理済みにできる', async () => {
  const fetchMock = stubFetch(
    routes({
      'POST /api/damages/5/status': () =>
        jsonResponse({ report: report({ status: '修理済み' }), item: {} }),
    }),
  )

  renderApp('/admin/damages')
  await screen.findByText('脚のロックが割れている')

  fireEvent.click(screen.getByRole('button', { name: '修理済みにする' }))

  await screen.findByText('脚のロックが割れている')
  const call = fetchMock.mock.calls.find((c) => String(c[0]) === '/api/damages/5/status')
  const body = (call?.[1] as RequestInit | undefined)?.body
  expect(JSON.parse(String(body))).toEqual({ status: '修理済み', note: '' })
})

// 報告は履歴。誤報告も「そう報告された」という事実で、消す導線を置かない。
test('削除の導線を置かない', async () => {
  stubFetch(routes())

  renderApp('/admin/damages')
  await screen.findByText('脚のロックが割れている')

  expect(screen.queryByRole('button', { name: /削除/ })).toBeNull()
})

// 今の状態のボタンは出さない。押しても何も変わらないボタンになる。
test('今の状態にするボタンは出さない', async () => {
  stubFetch(
    routes({ '/api/damages': () => jsonResponse({ reports: [report({ status: '修理済み' })] }) }),
  )

  renderApp('/admin/damages')
  await screen.findByText('脚のロックが割れている')

  expect(screen.queryByRole('button', { name: '修理済みにする' })).toBeNull()
  expect(screen.getByRole('button', { name: '確認済みにする' })).toBeDefined()
})

// 誰が借りている間に壊れたのかを後から辿れるようにする。
test('貸出中の報告であることを出す', async () => {
  stubFetch(routes({ '/api/damages': () => jsonResponse({ reports: [report({ loan_id: 7 })] }) }))

  renderApp('/admin/damages')

  expect(await screen.findByText(/貸出中に報告/)).toBeDefined()
})
