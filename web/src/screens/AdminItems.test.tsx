import { fireEvent, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { errorResponse, jsonResponse, stubFetch } from '../testing/fetchStub'
import { renderApp } from '../testing/renderApp'

afterEach(() => {
  vi.unstubAllGlobals()
})

const admin = { id: 1, name: '運営', login_id: 'admin', role: 'admin', must_change_password: false }
const member = { id: 2, name: '田中', login_id: 'taro', role: 'member', must_change_password: false }

function item(over: Record<string, unknown> = {}) {
  return {
    id: 1,
    code: '0001',
    name: '三脚',
    category: '撮影機材',
    model: 'SLIK 500',
    owner: 'サークル',
    is_free_use: false,
    location: '部室A棚',
    condition: '良好',
    location_status: '在庫',
    photo_url: '',
    note: '',
    updated_at: '2026-08-16 00:00:00',
    ...over,
  }
}

function routes(over: Record<string, () => Response> = {}) {
  return {
    '/api/me': () => jsonResponse({ user: admin, redirect_to: '/' }),
    '/api/items': () => jsonResponse({ items: [item()] }),
    '/api/items/0001': () => jsonResponse({ item: item({ name: '三脚（更新後）' }) }),
    ...over,
  }
}

/** lastItemsQuery は最後に一覧を取りに行ったクエリ文字列を返す。 */
function lastItemsQuery(fetchMock: ReturnType<typeof stubFetch>): string {
  const calls = fetchMock.mock.calls.filter(([path]) => path.startsWith('/api/items?'))
  return calls.at(-1)?.[0].slice('/api/items'.length) ?? ''
}

/** putCall は更新の呼び出しを返す。 */
function putCall(fetchMock: ReturnType<typeof stubFetch>) {
  return fetchMock.mock.calls.find(([, init]) => init?.method === 'PUT')
}

test('備品を一覧する', async () => {
  stubFetch(routes())

  renderApp('/admin/items')

  // 見出しは即座に出るが、一覧は取得を待つ。先に一覧を待つこと。
  expect(await screen.findByText('三脚')).toBeDefined()
  expect(screen.getByRole('heading', { name: '備品マスタ管理' })).toBeDefined()
  expect(screen.getByText('0001')).toBeDefined()
})

// __member が備品マスタを書き換えられないことが、スプレッドシート共有ではなく__
// __システム化する主目的（m1-spec §5）。__ 画面でも入口を塞ぐ。
test('member には画面を出さない', async () => {
  stubFetch(routes({ '/api/me': () => jsonResponse({ user: member, redirect_to: '/' }) }))

  renderApp('/admin/items')

  expect(await screen.findByRole('heading', { name: 'この画面は運営のみが使えます' })).toBeDefined()
  expect(screen.queryByRole('heading', { name: '備品マスタ管理' })).toBeNull()
})

// 押した先で「権限がありません」に当たるだけのリンクを出さない。
test('トップの管理リンクは運営にだけ出す', async () => {
  stubFetch({ '/api/me': () => jsonResponse({ user: member, redirect_to: '/' }) })

  renderApp('/')
  await screen.findByRole('heading', { name: '備品管理' })

  expect(screen.queryByRole('link', { name: '備品マスタ管理' })).toBeNull()
})

test('運営にはトップから管理画面へのリンクを出す', async () => {
  stubFetch({ '/api/me': () => jsonResponse({ user: admin, redirect_to: '/' }) })

  renderApp('/')
  await screen.findByRole('heading', { name: '備品管理' })

  expect(screen.getByRole('link', { name: '備品マスタ管理' })).toHaveProperty(
    'href',
    expect.stringContaining('/admin/items'),
  )
})

// 廃棄は物理削除の代わり。運営が棚を整理する時に見られる必要がある。
test('廃棄済みも表示できる', async () => {
  const fetchMock = stubFetch(routes())

  renderApp('/admin/items')
  await screen.findByText('三脚')

  fireEvent.click(screen.getByLabelText('廃棄済みも表示する'))

  await waitFor(() => {
    expect(lastItemsQuery(fetchMock)).toContain('include_discarded=1')
  })
})

// 一部だけ送る形にすると、画面で消した備考が消えない。
test('保存すると全項目を送る', async () => {
  const fetchMock = stubFetch(routes())

  renderApp('/admin/items')
  await screen.findByText('三脚')

  fireEvent.click(screen.getByRole('button', { name: '編集' }))
  fireEvent.change(screen.getByLabelText('保管場所'), { target: { value: '倉庫' } })
  fireEvent.click(screen.getByRole('button', { name: '保存' }))

  await waitFor(() => {
    expect(putCall(fetchMock)).toBeDefined()
  })

  const call = putCall(fetchMock)
  expect(call?.[0]).toBe('/api/items/0001')
  expect(JSON.parse(String(call?.[1]?.body))).toEqual({
    name: '三脚',
    category: '撮影機材',
    model: 'SLIK 500',
    owner: 'サークル',
    is_free_use: false,
    location: '倉庫',
    condition: '良好',
    location_status: '在庫',
    note: '',
  })
})

// __備品コードは採番したら二度と変えない。__ ラベルは貼り替えられない。
test('備品コードは送らない', async () => {
  const fetchMock = stubFetch(routes())

  renderApp('/admin/items')
  await screen.findByText('三脚')

  fireEvent.click(screen.getByRole('button', { name: '編集' }))
  fireEvent.click(screen.getByRole('button', { name: '保存' }))

  await waitFor(() => {
    expect(putCall(fetchMock)).toBeDefined()
  })

  const body = JSON.parse(String(putCall(fetchMock)?.[1]?.body)) as Record<string, unknown>
  expect(body).not.toHaveProperty('code')
})

test('保存すると一覧の表示が変わる', async () => {
  stubFetch(routes())

  renderApp('/admin/items')
  await screen.findByText('三脚')

  fireEvent.click(screen.getByRole('button', { name: '編集' }))
  fireEvent.click(screen.getByRole('button', { name: '保存' }))

  expect(await screen.findByText('三脚（更新後）')).toBeDefined()
  expect(await screen.findByRole('status')).toHaveProperty('textContent', '保存しました')
})

// 廃棄は状態の1つ。専用の経路を使わず、状態を「廃棄」にして全項目を送る。
test('状態を廃棄にできる', async () => {
  const fetchMock = stubFetch(routes())

  renderApp('/admin/items')
  await screen.findByText('三脚')

  fireEvent.click(screen.getByRole('button', { name: '編集' }))
  fireEvent.change(screen.getByLabelText('状態'), { target: { value: '廃棄' } })
  fireEvent.click(screen.getByRole('button', { name: '保存' }))

  await waitFor(() => {
    expect(putCall(fetchMock)).toBeDefined()
  })

  expect(JSON.parse(String(putCall(fetchMock)?.[1]?.body))).toMatchObject({ condition: '廃棄' })
})

test('保存に失敗したら理由を出し、編集を閉じない', async () => {
  stubFetch(routes({ '/api/items/0001': () => errorResponse('品名は必須です', 400) }))

  renderApp('/admin/items')
  await screen.findByText('三脚')

  fireEvent.click(screen.getByRole('button', { name: '編集' }))
  fireEvent.click(screen.getByRole('button', { name: '保存' }))

  expect(await screen.findByRole('alert')).toHaveProperty('textContent', '品名は必須です')
  expect(screen.getByRole('button', { name: '保存' })).toBeDefined()
})

test('キャンセルすると編集を閉じる', async () => {
  stubFetch(routes())

  renderApp('/admin/items')
  await screen.findByText('三脚')

  fireEvent.click(screen.getByRole('button', { name: '編集' }))
  expect(screen.getByLabelText('品名')).toBeDefined()

  fireEvent.click(screen.getByRole('button', { name: 'キャンセル' }))
  expect(screen.queryByLabelText('品名')).toBeNull()
})

// 削除は無い。行を消すと貸出履歴の参照先が消える（CLAUDE.md）。
test('削除の導線を置かない', async () => {
  stubFetch(routes())

  renderApp('/admin/items')
  // 一覧が出てから見る。出る前に見ると、まだ描かれていないだけで通ってしまう。
  await screen.findByText('三脚')

  // 画面のどこにも置かない。廃棄は状態であって削除ではなく、
  // 行を消すと貸出履歴の参照先が消える（CLAUDE.md）。
  expect(screen.queryByRole('button', { name: /削除/ })).toBeNull()
  expect(screen.queryByRole('link', { name: /削除/ })).toBeNull()
})

// ---- CSVエクスポート ----

// __システムが死んでもデータが残るための保険__（m1-spec §8）。
// APIだけあって押す場所が無いと、存在を知る手段が無く使われない。
test('全備品のCSV書き出しへ行ける', async () => {
  stubFetch(routes())

  renderApp('/admin/items')
  await screen.findByRole('heading', { name: '備品マスタ管理' })

  const link = screen.getByRole('link', { name: /全備品をCSVで書き出す/ })

  // サーバがそのまま返すものなので、素の <a> で開く。
  // react-router の Link にすると画面遷移として扱われ、保存されない。
  expect(link).toHaveProperty('href', expect.stringContaining('/api/items/export.csv'))

  // 廃棄済みも含むこと・復元には使えないことを、押す前に書いておく。
  expect(screen.getByText(/廃棄済みも含めた全件/)).toBeDefined()
  expect(screen.getByText(/復元にはバックアップ/)).toBeDefined()
})

// ---- 写真の差し替え・削除 ----

// __CSVで一括登録した備品には、登録フォームを通らないので写真が付かない。__
// M1 の主な入り口は一括登録なので、編集から付けられないと
// ほとんどの備品が一生写真を持てない。
test('編集から写真を追加できる', async () => {
  const withPhoto = { ...item({}), photo_url: '/api/items/0001/photo' }
  const fetchMock = stubFetch({
    ...routes(),
    'POST /api/items/0001/photo': () => jsonResponse({ item: withPhoto }),
  })

  renderApp('/admin/items')
  await screen.findByText('三脚')
  fireEvent.click(screen.getByRole('button', { name: '編集' }))

  const file = new File(['x'], 'tripod.jpg', { type: 'image/jpeg' })
  fireEvent.change(screen.getByLabelText('写真を追加する'), { target: { files: [file] } })

  await waitFor(() => {
    const call = fetchMock.mock.calls.find(([path]) => path === '/api/items/0001/photo')
    expect(call).toBeDefined()
    // multipart で送る。Content-Type は指定しない（境界文字列がずれる）。
    expect(call?.[1]?.body).toBeInstanceOf(FormData)
  })
})

// 写真は選んだ時点で送られる。__ここで編集を閉じると、入力中の他の項目が消える。__
test('写真を差し替えても編集は閉じない', async () => {
  const withPhoto = { ...item({}), photo_url: '/api/items/0001/photo' }
  stubFetch({
    ...routes(),
    'POST /api/items/0001/photo': () => jsonResponse({ item: withPhoto }),
  })

  renderApp('/admin/items')
  await screen.findByText('三脚')
  fireEvent.click(screen.getByRole('button', { name: '編集' }))

  fireEvent.change(screen.getByLabelText('保管場所'), { target: { value: '倉庫' } })

  const file = new File(['x'], 'tripod.jpg', { type: 'image/jpeg' })
  fireEvent.change(screen.getByLabelText('写真を追加する'), { target: { files: [file] } })

  // 差し替えの後も編集は開いたままで、打ちかけの値も残っていること。
  expect(await screen.findByRole('button', { name: '保存' })).toBeDefined()
  expect(screen.getByLabelText('保管場所')).toHaveProperty('value', '倉庫')
})

// 消したファイルは戻らない。1タップの確認を挟む。
test('写真を外すのは確認を挟む', async () => {
  const withPhoto = { ...item({}), photo_url: '/api/items/0001/photo' }
  const fetchMock = stubFetch({
    ...routes(),
    '/api/items': () => jsonResponse({ items: [withPhoto] }),
    'DELETE /api/items/0001/photo': () => jsonResponse({ item: item({}) }),
  })

  renderApp('/admin/items')
  await screen.findByText('三脚')
  fireEvent.click(screen.getByRole('button', { name: '編集' }))

  fireEvent.click(screen.getByRole('button', { name: '写真を外す' }))
  expect(fetchMock.mock.calls.some(([, init]) => init?.method === 'DELETE')).toBe(false)

  fireEvent.click(screen.getByRole('button', { name: '本当に外す' }))

  await waitFor(() => {
    const call = fetchMock.mock.calls.find(([path]) => path === '/api/items/0001/photo')
    expect(call?.[1]?.method).toBe('DELETE')
  })
})

// 写真が無い備品に「外す」を出さない。押せない操作を置かない。
test('写真が無ければ外す導線を出さない', async () => {
  stubFetch(routes())

  renderApp('/admin/items')
  await screen.findByText('三脚')
  fireEvent.click(screen.getByRole('button', { name: '編集' }))

  expect(screen.queryByRole('button', { name: '写真を外す' })).toBeNull()
})
