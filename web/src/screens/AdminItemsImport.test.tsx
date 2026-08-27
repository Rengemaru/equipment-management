import { fireEvent, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { errorResponse, jsonResponse, stubFetch } from '../testing/fetchStub'
import { renderApp } from '../testing/renderApp'

afterEach(() => {
  vi.unstubAllGlobals()
})

const admin = { id: 1, name: '運営', login_id: 'admin', role: 'admin', must_change_password: false }

/** row はプレビューの1行を作る。 */
function row(over: Record<string, unknown> = {}) {
  return {
    line: 3,
    quantity: 1,
    name: '三脚',
    category: '撮影機材',
    model: '',
    owner: 'サークル',
    is_free_use: false,
    location: '部室A棚',
    condition: '良好',
    note: '',
    ...over,
  }
}

function preview(over: Record<string, unknown> = {}) {
  return {
    row_count: 1,
    record_count: 1,
    can_import: true,
    code_from: '0043',
    code_to: '0043',
    rows: [row()],
    errors: [],
    ...over,
  }
}

function routes(over: Record<string, () => Response> = {}) {
  return {
    '/api/me': () => jsonResponse({ user: admin, redirect_to: '/' }),
    '/api/items/import/preview': () => jsonResponse({ preview: preview() }),
    '/api/items/import': () =>
      jsonResponse({ result: { record_count: 1, code_from: '0043', code_to: '0043' } }, 201),
    ...over,
  }
}

function csvFile() {
  return new File(['品名　*\n三脚\n'], 'inventory.csv', { type: 'text/csv' })
}

/** choose はファイルを選ぶ。 */
function choose() {
  fireEvent.change(screen.getByLabelText('CSVファイル'), { target: { files: [csvFile()] } })
}

function importCall(fetchMock: ReturnType<typeof stubFetch>) {
  return fetchMock.mock.calls.find(([p]) => p === '/api/items/import')
}

test('ファイルを選ぶまで確認できない', async () => {
  stubFetch(routes())

  renderApp('/admin/items/import')

  const button = await screen.findByRole('button', { name: '内容を確認する' })
  expect(button).toHaveProperty('disabled', true)
})

// __確定の前に必ずプレビューを挟む。__ 備品コードは再利用しないため、
// 余計な行を取り込むと番号を戻せない（m1-spec §5）。
test('確認するまで取り込みボタンを出さない', async () => {
  stubFetch(routes())

  renderApp('/admin/items/import')
  await screen.findByRole('button', { name: '内容を確認する' })

  choose()

  expect(screen.queryByRole('button', { name: 'この内容で取り込む' })).toBeNull()
})

test('確認すると取り込む内容と採番の予定を出す', async () => {
  stubFetch(routes())

  renderApp('/admin/items/import')
  await screen.findByRole('button', { name: '内容を確認する' })

  choose()
  fireEvent.click(screen.getByRole('button', { name: '内容を確認する' }))

  expect(await screen.findByText('三脚')).toBeDefined()
  expect(screen.getByText(/1行から/)).toBeDefined()
  expect(screen.getByText(/0043 〜 0043/)).toBeDefined()
  // 予定であることを書く。決まった値に見せると、この範囲でラベルを刷る人が出る。
  expect(screen.getByText(/予定です/)).toBeDefined()
})

// 1行が数量の数だけのレコードになる。行数と件数が違うことをその場で分かるようにする。
test('数量ぶんに展開された件数を出す', async () => {
  stubFetch(
    routes({
      '/api/items/import/preview': () =>
        jsonResponse({
          preview: preview({ row_count: 1, record_count: 5, rows: [row({ quantity: 5 })] }),
        }),
    }),
  )

  renderApp('/admin/items/import')
  await screen.findByRole('button', { name: '内容を確認する' })

  choose()
  fireEvent.click(screen.getByRole('button', { name: '内容を確認する' }))

  expect(await screen.findByText('5件')).toBeDefined()
  expect(screen.getByText('×5')).toBeDefined()
})

// 誤りが1件でもあれば取り込めない。全件成功か全件失敗で、
// 誤った行だけを飛ばして入れることはしない（m1-spec §5）。
test('誤りがあれば行番号付きで全件出し、取り込ませない', async () => {
  stubFetch(
    routes({
      '/api/items/import/preview': () =>
        jsonResponse({
          preview: preview({
            can_import: false,
            code_from: '',
            code_to: '',
            rows: [],
            errors: [
              { line: 3, message: '品名は必須です' },
              { line: 5, message: '数量が数値ではありません' },
            ],
          }),
        }),
    }),
  )

  renderApp('/admin/items/import')
  await screen.findByRole('button', { name: '内容を確認する' })

  choose()
  fireEvent.click(screen.getByRole('button', { name: '内容を確認する' }))

  expect(await screen.findByText('3行目: 品名は必須です')).toBeDefined()
  expect(screen.getByText('5行目: 数量が数値ではありません')).toBeDefined()
  expect(screen.queryByRole('button', { name: 'この内容で取り込む' })).toBeNull()
})

// テンプレートの記入例の消し忘れは必ず起きる。CSVを直させずに除けるようにする。
test('取り込まない行を選ぶと除外行として送る', async () => {
  const fetchMock = stubFetch(
    routes({
      '/api/items/import/preview': () =>
        jsonResponse({
          preview: preview({
            row_count: 2,
            record_count: 2,
            rows: [row({ line: 2, name: '三脚（大）' }), row({ line: 3 })],
          }),
        }),
    }),
  )

  renderApp('/admin/items/import')
  await screen.findByRole('button', { name: '内容を確認する' })

  choose()
  fireEvent.click(screen.getByRole('button', { name: '内容を確認する' }))
  await screen.findByText('三脚（大）')

  fireEvent.click(screen.getByLabelText('2行目を取り込まない'))
  fireEvent.click(screen.getByRole('button', { name: 'この内容で取り込む' }))

  await waitFor(() => {
    expect(importCall(fetchMock)).toBeDefined()
  })

  const body = importCall(fetchMock)?.[1]?.body as FormData
  expect(body.getAll('exclude_lines')).toEqual(['2'])
  expect(body.get('file')).toBeInstanceOf(File)
})

// 行を除くと採番の範囲がずれる。決まった値のように見せない。
test('除外を選ぶと採番の予定を出さない', async () => {
  stubFetch(routes())

  renderApp('/admin/items/import')
  await screen.findByRole('button', { name: '内容を確認する' })

  choose()
  fireEvent.click(screen.getByRole('button', { name: '内容を確認する' }))
  await screen.findByText('三脚')

  expect(screen.getByText(/0043 〜 0043/)).toBeDefined()

  fireEvent.click(screen.getByLabelText('3行目を取り込まない'))

  expect(screen.queryByText(/採番の予定/)).toBeNull()
})

// 予定ではなく確定した値。この範囲がそのままラベルの印刷範囲になる。
test('取り込むと実際に採番されたコードを出す', async () => {
  stubFetch(
    routes({
      '/api/items/import': () =>
        jsonResponse({ result: { record_count: 12, code_from: '0043', code_to: '0054' } }, 201),
    }),
  )

  renderApp('/admin/items/import')
  await screen.findByRole('button', { name: '内容を確認する' })

  choose()
  fireEvent.click(screen.getByRole('button', { name: '内容を確認する' }))
  await screen.findByText('三脚')

  fireEvent.click(screen.getByRole('button', { name: 'この内容で取り込む' }))

  expect(await screen.findByRole('heading', { name: '取り込みました' })).toBeDefined()
  expect(screen.getByText('12件を登録しました。')).toBeDefined()
  expect(screen.getByText('0043 〜 0054')).toBeDefined()
})

// 除外に指定した行がCSVに無ければサーバが弾く。黙って無視すると、
// 行がずれたCSVで記入例が入り、備品コードを戻せなくなる。
test('取り込みに失敗したら理由を出す', async () => {
  stubFetch(
    routes({
      '/api/items/import': () =>
        errorResponse(
          '除外に指定した行がCSVにありません: 2（プレビューを取り直してください）',
          400,
        ),
    }),
  )

  renderApp('/admin/items/import')
  await screen.findByRole('button', { name: '内容を確認する' })

  choose()
  fireEvent.click(screen.getByRole('button', { name: '内容を確認する' }))
  await screen.findByText('三脚')

  fireEvent.click(screen.getByRole('button', { name: 'この内容で取り込む' }))

  expect(await screen.findByRole('alert')).toHaveProperty(
    'textContent',
    '除外に指定した行がCSVにありません: 2（プレビューを取り直してください）',
  )
})

test('CSVとして読めなければ理由を出す', async () => {
  stubFetch(
    routes({
      '/api/items/import/preview': () => errorResponse('必須の列がありません: 品名', 400),
    }),
  )

  renderApp('/admin/items/import')
  await screen.findByRole('button', { name: '内容を確認する' })

  choose()
  fireEvent.click(screen.getByRole('button', { name: '内容を確認する' }))

  expect(await screen.findByRole('alert')).toHaveProperty(
    'textContent',
    '必須の列がありません: 品名',
  )
})

// 見ている内容と送るファイルが食い違ったまま確定できてしまうのを防ぐ。
test('別のファイルを選ぶとプレビューを消す', async () => {
  stubFetch(routes())

  renderApp('/admin/items/import')
  await screen.findByRole('button', { name: '内容を確認する' })

  choose()
  fireEvent.click(screen.getByRole('button', { name: '内容を確認する' }))
  await screen.findByText('三脚')

  choose()

  expect(screen.queryByText('三脚')).toBeNull()
  expect(screen.queryByRole('button', { name: 'この内容で取り込む' })).toBeNull()
})

test('マスタ管理から取り込み画面へ行ける', async () => {
  stubFetch({
    '/api/me': () => jsonResponse({ user: admin, redirect_to: '/' }),
    '/api/items': () => jsonResponse({ items: [] }),
  })

  renderApp('/admin/items')
  await screen.findByRole('heading', { name: '備品マスタ管理' })

  expect(screen.getByRole('link', { name: 'CSVで一括登録' })).toHaveProperty(
    'href',
    expect.stringContaining('/admin/items/import'),
  )
})
