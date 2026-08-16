import { fireEvent, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { errorResponse, jsonResponse, stubFetch } from '../testing/fetchStub'
import { renderApp } from '../testing/renderApp'

afterEach(() => {
  vi.unstubAllGlobals()
})

const admin = { id: 1, name: '運営', login_id: 'admin', role: 'admin', must_change_password: false }

function item(id: number, code: string, name: string) {
  return {
    id,
    code,
    name,
    category: '撮影機材',
    model: '',
    owner: 'サークル',
    is_free_use: false,
    location: '部室A棚',
    condition: '良好',
    location_status: '在庫',
    photo_url: '',
    note: '',
    updated_at: '2026-08-16 00:00:00',
  }
}

// 4桁を超えるコードを混ぜる。文字列で比べると "10000" < "9999" になり、
// 4桁を超えた備品が範囲から静かに漏れる。
const items = [
  item(1, '0001', '三脚'),
  item(2, '0042', '一眼レフ'),
  item(3, '9999', '脚立'),
  item(4, '10000', '工具箱'),
]

function routes(over: Record<string, () => Response> = {}) {
  return {
    '/api/me': () => jsonResponse({ user: admin, redirect_to: '/' }),
    '/api/items': () => jsonResponse({ items }),
    '/api/items/filters': () =>
      jsonResponse({ categories: ['撮影機材', '工具'], locations: ['部室A棚'] }),
    ...over,
  }
}

/** pdfLink は PDF を開くリンクを返す。 */
function pdfLink() {
  return screen.queryByRole('link', { name: /PDFを開く/ })
}

test('対象の一覧と件数を出す', async () => {
  stubFetch(routes())

  renderApp('/admin/labels')

  expect(await screen.findByText('4件')).toBeDefined()
  expect(screen.getByText('三脚')).toBeDefined()
  expect(screen.getByText('工具箱')).toBeDefined()
})

// __コード範囲は数値で比較する。__ 文字列だと "10000" < "9999" になり、
// 4桁を超えた備品が静かに漏れる。
test('範囲は数値で比較する', async () => {
  stubFetch(routes())

  renderApp('/admin/labels')
  await screen.findByText('4件')

  fireEvent.change(screen.getByLabelText('開始コード'), { target: { value: '9999' } })

  await waitFor(() => {
    expect(screen.getByText('2件')).toBeDefined()
  })
  expect(screen.getByText('脚立')).toBeDefined()
  expect(screen.getByText('工具箱')).toBeDefined()
  expect(screen.queryByText('三脚')).toBeNull()
})

test('範囲をPDFのURLに載せる', async () => {
  stubFetch(routes())

  renderApp('/admin/labels')
  await screen.findByText('4件')

  fireEvent.change(screen.getByLabelText('開始コード'), { target: { value: '0001' } })
  fireEvent.change(screen.getByLabelText('終了コード'), { target: { value: '0042' } })

  await waitFor(() => {
    expect(screen.getByText('2件')).toBeDefined()
  })
  expect(pdfLink()).toHaveProperty('href', expect.stringContaining('from=0001&to=0042'))
})

test('分類で絞るとサーバへ渡し、URLにも載せる', async () => {
  const fetchMock = stubFetch(routes())

  renderApp('/admin/labels')
  await screen.findByText('4件')

  fireEvent.change(screen.getByLabelText('分類'), { target: { value: '工具' } })

  await waitFor(() => {
    const calls = fetchMock.mock.calls.filter(([p]) => p.startsWith('/api/items?'))
    expect(calls.at(-1)?.[0]).toContain(`category=${encodeURIComponent('工具')}`)
  })
  expect(pdfLink()).toHaveProperty(
    'href',
    expect.stringContaining(`category=${encodeURIComponent('工具')}`),
  )
})

// 白紙のシートを刷るとラベルシールが1枚無駄になる。サーバは400を返すが、
// その前に画面で止める。
test('対象0件ならPDFのリンクを出さない', async () => {
  stubFetch(routes())

  renderApp('/admin/labels')
  await screen.findByText('4件')

  fireEvent.change(screen.getByLabelText('開始コード'), { target: { value: '20000' } })

  await waitFor(() => {
    expect(screen.getByText('0件')).toBeDefined()
  })
  expect(pdfLink()).toBeNull()
  expect(screen.getByText(/条件に合う備品がありません/)).toBeDefined()
})

// 逆順を0件と同じ扱いにすると「その範囲に備品が無い」と読めてしまい、
// 打ち間違いに気付けない。
test('範囲が逆なら理由を出す', async () => {
  stubFetch(routes())

  renderApp('/admin/labels')
  await screen.findByText('4件')

  fireEvent.change(screen.getByLabelText('開始コード'), { target: { value: '50' } })
  fireEvent.change(screen.getByLabelText('終了コード'), { target: { value: '10' } })

  expect(await screen.findByRole('alert')).toHaveProperty(
    'textContent',
    '備品コードの範囲が逆です。',
  )
  expect(pdfLink()).toBeNull()
})

test('数字でない指定は理由を出す', async () => {
  stubFetch(routes())

  renderApp('/admin/labels')
  await screen.findByText('4件')

  fireEvent.change(screen.getByLabelText('開始コード'), { target: { value: 'あ' } })

  expect(await screen.findByRole('alert')).toHaveProperty(
    'textContent',
    '備品コードは1以上の数字で指定してください。',
  )
  expect(pdfLink()).toBeNull()
})

// 棚に並ばないものにラベルを刷る理由がない。含まれるものを画面に書く。
test('廃棄済みが含まれないことを書く', async () => {
  stubFetch(routes())

  renderApp('/admin/labels')
  await screen.findByText('4件')

  expect(screen.getByText('廃棄済みは含まれません。自由利用品は含まれます。')).toBeDefined()
})

test('一覧の取得に失敗したら理由を出す', async () => {
  stubFetch(routes({ '/api/items': () => errorResponse('サーバ側で問題が起きました', 500) }))

  renderApp('/admin/labels')

  expect(await screen.findByRole('alert')).toHaveProperty(
    'textContent',
    'サーバ側で問題が起きました',
  )
  expect(pdfLink()).toBeNull()
})

test('マスタ管理からラベル印刷へ行ける', async () => {
  stubFetch({
    '/api/me': () => jsonResponse({ user: admin, redirect_to: '/' }),
    '/api/items': () => jsonResponse({ items: [] }),
  })

  renderApp('/admin/items')
  await screen.findByRole('heading', { name: '備品マスタ管理' })

  expect(screen.getByRole('link', { name: 'QRラベルの印刷' })).toHaveProperty(
    'href',
    expect.stringContaining('/admin/labels'),
  )
})
