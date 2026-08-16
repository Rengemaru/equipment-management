import { fireEvent, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { errorResponse, jsonResponse, stubFetch } from '../testing/fetchStub'
import { renderApp } from '../testing/renderApp'

afterEach(() => {
  vi.unstubAllGlobals()
})

const admin = { id: 1, name: '運営', login_id: 'admin', role: 'admin', must_change_password: false }

function item(over: Record<string, unknown> = {}) {
  return {
    id: 1,
    code: '0043',
    name: '三脚',
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
    ...over,
  }
}

function routes(over: Record<string, () => Response> = {}) {
  return {
    '/api/me': () => jsonResponse({ user: admin, redirect_to: '/' }),
    '/api/items': () => jsonResponse({ item: item() }, 201),
    ...over,
  }
}

/** postCall は指定した経路への POST を返す。 */
function postCall(fetchMock: ReturnType<typeof stubFetch>, path: string) {
  return fetchMock.mock.calls.find(([p, init]) => p === path && init?.method === 'POST')
}

/** fill は必須項目を埋める。 */
function fillName(name: string) {
  fireEvent.change(screen.getByLabelText('品名'), { target: { value: name } })
}

/** pngFile は中身のある小さなファイルを作る。内容は問わない（送信の形だけ見る）。 */
function pngFile() {
  return new File(['dummy'], 'photo.png', { type: 'image/png' })
}

test('登録フォームを表示する', async () => {
  stubFetch(routes())

  renderApp('/admin/items/new')

  expect(await screen.findByRole('heading', { name: '備品を登録' })).toBeDefined()
  expect(screen.getByLabelText('品名')).toBeDefined()
  expect(screen.getByLabelText('写真')).toBeDefined()
})

// __備品コードは人手で振らせない。__ 抜け・重複が必ず起きる（CLAUDE.md）。
test('備品コードを入力させない', async () => {
  stubFetch(routes())

  renderApp('/admin/items/new')
  await screen.findByRole('heading', { name: '備品を登録' })

  expect(screen.queryByLabelText('備品コード')).toBeNull()
  expect(screen.getByText(/自動で採番されます/)).toBeDefined()
})

test('登録すると全項目を送り、コードは送らない', async () => {
  const fetchMock = stubFetch(routes())

  renderApp('/admin/items/new')
  await screen.findByRole('heading', { name: '備品を登録' })

  fillName('三脚')
  fireEvent.change(screen.getByLabelText('保管場所'), { target: { value: '部室A棚' } })
  fireEvent.click(screen.getByRole('button', { name: '登録する' }))

  await waitFor(() => {
    expect(postCall(fetchMock, '/api/items')).toBeDefined()
  })

  const body = JSON.parse(String(postCall(fetchMock, '/api/items')?.[1]?.body)) as Record<
    string,
    unknown
  >
  expect(body).toEqual({
    name: '三脚',
    category: '',
    model: '',
    owner: 'サークル',
    is_free_use: false,
    location: '部室A棚',
    condition: '良好',
    location_status: '在庫',
    note: '',
  })
  expect(body).not.toHaveProperty('code')
})

// ラベルを刷るのに要る番号。見せずに次へ進ませない。
test('採番されたコードを表示する', async () => {
  stubFetch(routes())

  renderApp('/admin/items/new')
  await screen.findByRole('heading', { name: '備品を登録' })

  fillName('三脚')
  fireEvent.click(screen.getByRole('button', { name: '登録する' }))

  expect(await screen.findByRole('heading', { name: '登録しました' })).toBeDefined()
  expect(screen.getByText('0043')).toBeDefined()
})

// 写真は備品が登録された後でないと送れない。順序は入れ替えられない。
test('写真を選ぶと登録後に送る', async () => {
  const fetchMock = stubFetch(
    routes({
      '/api/items/0043/photo': () =>
        jsonResponse({ item: item({ photo_url: '/api/items/0043/photo' }) }),
    }),
  )

  renderApp('/admin/items/new')
  await screen.findByRole('heading', { name: '備品を登録' })

  fillName('三脚')
  fireEvent.change(screen.getByLabelText('写真'), { target: { files: [pngFile()] } })
  fireEvent.click(screen.getByRole('button', { name: '登録する' }))

  expect(await screen.findByRole('heading', { name: '登録しました' })).toBeDefined()

  const call = postCall(fetchMock, '/api/items/0043/photo')
  expect(call).toBeDefined()

  // FormData で送る。Content-Type は指定しない（境界文字列はブラウザが付ける）。
  const init = call?.[1]
  expect(init?.body).toBeInstanceOf(FormData)
  expect((init?.body as FormData).get('photo')).toBeInstanceOf(File)
  expect(init?.headers).toBeUndefined()

  expect(await screen.findByRole('img', { name: '三脚の写真' })).toBeDefined()
})

// 登録自体をやり直させない。もう一度送ると同じ備品が2件でき、
// 採番が1つ無駄になる（番号は再利用しない）。
test('写真だけ失敗しても登録は成功として扱い、送り直せる', async () => {
  let failed = false
  const fetchMock = stubFetch(
    routes({
      '/api/items/0043/photo': () => {
        if (!failed) {
          failed = true
          return errorResponse('JPEG または PNG の画像を選んでください', 415)
        }
        return jsonResponse({ item: item({ photo_url: '/api/items/0043/photo' }) })
      },
    }),
  )

  renderApp('/admin/items/new')
  await screen.findByRole('heading', { name: '備品を登録' })

  fillName('三脚')
  fireEvent.change(screen.getByLabelText('写真'), { target: { files: [pngFile()] } })
  fireEvent.click(screen.getByRole('button', { name: '登録する' }))

  // 登録は成功している。コードも出る。
  expect(await screen.findByRole('heading', { name: '登録しました' })).toBeDefined()
  expect(screen.getByText('0043')).toBeDefined()
  expect(await screen.findByRole('alert')).toHaveProperty(
    'textContent',
    '備品は登録されましたが、写真の添付に失敗しました: JPEG または PNG の画像を選んでください',
  )

  // 送り直せる。備品は作り直さない。
  fireEvent.click(screen.getByRole('button', { name: '写真を送り直す' }))

  await waitFor(() => {
    expect(screen.queryByRole('alert')).toBeNull()
  })
  expect(fetchMock.mock.calls.filter(([p, init]) => p === '/api/items' && init?.method === 'POST'))
    .toHaveLength(1)
})

test('登録に失敗したら理由を出し、フォームに留まる', async () => {
  stubFetch(routes({ '/api/items': () => errorResponse('品名は必須です', 400) }))

  renderApp('/admin/items/new')
  await screen.findByRole('heading', { name: '備品を登録' })

  fillName('　')
  fireEvent.click(screen.getByRole('button', { name: '登録する' }))

  expect(await screen.findByRole('alert')).toHaveProperty('textContent', '品名は必須です')
  expect(screen.getByRole('button', { name: '登録する' })).toBeDefined()
})

// 棚1つ分をまとめて登録する使い方に合わせる。
test('続けて登録すると分類と保管場所は残り、品名は空になる', async () => {
  stubFetch(routes())

  renderApp('/admin/items/new')
  await screen.findByRole('heading', { name: '備品を登録' })

  fireEvent.change(screen.getByLabelText('分類'), { target: { value: '撮影機材' } })
  fireEvent.change(screen.getByLabelText('保管場所'), { target: { value: '部室A棚' } })
  fillName('三脚')
  fireEvent.click(screen.getByRole('button', { name: '登録する' }))

  await screen.findByRole('heading', { name: '登録しました' })
  fireEvent.click(screen.getByRole('button', { name: '続けて登録する' }))

  expect(await screen.findByLabelText('品名')).toHaveProperty('value', '')
  expect(screen.getByLabelText('分類')).toHaveProperty('value', '撮影機材')
  expect(screen.getByLabelText('保管場所')).toHaveProperty('value', '部室A棚')
})

test('マスタ管理から登録画面へ行ける', async () => {
  stubFetch({
    '/api/me': () => jsonResponse({ user: admin, redirect_to: '/' }),
    '/api/items': () => jsonResponse({ items: [] }),
  })

  renderApp('/admin/items')
  await screen.findByRole('heading', { name: '備品マスタ管理' })

  expect(screen.getByRole('link', { name: '備品を登録' })).toHaveProperty(
    'href',
    expect.stringContaining('/admin/items/new'),
  )
})
