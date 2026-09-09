import { fireEvent, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { jsonResponse, stubFetch } from '../testing/fetchStub'
import { renderApp } from '../testing/renderApp'

afterEach(() => {
  vi.unstubAllGlobals()
})

const taro = { id: 1, name: '田中', login_id: 'taro', role: 'member', must_change_password: false }

/** loan は既定値の揃った1件を作る。テストごとに気にする項目だけを渡す。 */
function loan(over: Record<string, unknown> = {}) {
  return {
    id: 1,
    item: { code: '0042', name: '三脚' },
    user: { id: 9, name: '佐藤' },
    registered_by: { id: 9, name: '佐藤' },
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
    '/api/loans': () => jsonResponse({ loans: [loan()] }),
    ...over,
  }
}

// __誰が何を持っているかが全員に見える状態を作る。__ member も見られる。
test('貸出中の備品と借用者を出す', async () => {
  stubFetch(routes())

  renderApp('/loans')

  expect(await screen.findByText('三脚')).toBeDefined()
  expect(screen.getByText(/佐藤 さん/)).toBeDefined()
  expect(screen.getByText(/9月24日/)).toBeDefined()
})

// __期限内であることの印は出さない。__ 全行に付く印は、注意すべき行を埋もれさせる。
test('期限内には印を付けず、超過にだけ付ける', async () => {
  stubFetch(routes())

  renderApp('/loans')
  await screen.findByText('三脚')

  expect(screen.queryByText(/超過/)).toBeNull()
})

test('超過している貸出を目立たせる', async () => {
  stubFetch(routes({ '/api/loans': () => jsonResponse({ loans: [loan({ overdue_days: 3 })] }) }))

  renderApp('/loans')

  expect(await screen.findByText('3日超過')).toBeDefined()
})

// 一覧で見つけた人が、そこから返却まで辿り着けないと意味がない。
test('行から備品の画面へ行ける', async () => {
  stubFetch(routes())

  renderApp('/loans')
  await screen.findByText('三脚')

  expect(screen.getByRole('link', { name: /三脚/ }).getAttribute('href')).toBe('/i/0042')
})

// 絞り込みはURLのクエリに持つ。画面の中だけに持つと、戻る操作で条件が消える。
test('期限超過の絞り込みをクエリに送る', async () => {
  const fetchMock = stubFetch(routes())

  renderApp('/loans')
  await screen.findByText('三脚')

  fireEvent.click(screen.getByRole('button', { name: '期限を過ぎたものだけ' }))

  await screen.findByText('三脚')
  const paths = fetchMock.mock.calls.map((c) => String(c[0]))
  expect(paths.some((p) => p.includes('overdue=1'))).toBe(true)
})

test('検索語をクエリに送る', async () => {
  const fetchMock = stubFetch(routes())

  renderApp('/loans')
  await screen.findByText('三脚')

  fireEvent.change(screen.getByRole('searchbox'), { target: { value: '三脚' } })
  fireEvent.click(screen.getByRole('button', { name: '検索' }))

  await screen.findByText('三脚')
  const paths = fetchMock.mock.calls.map((c) => String(c[0]))
  expect(paths.some((p) => p.includes('q=%E4%B8%89%E8%84%9A'))).toBe(true)
})

// 貸出が無いのは異常ではない。条件を疑わせない文言にする。
test('貸出が無ければその旨を出す', async () => {
  stubFetch(routes({ '/api/loans': () => jsonResponse({ loans: [] }) }))

  renderApp('/loans')

  expect(await screen.findByText('貸出中の備品はありません')).toBeDefined()
})
