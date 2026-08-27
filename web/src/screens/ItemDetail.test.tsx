import { fireEvent, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { errorResponse, jsonResponse, stubFetch } from '../testing/fetchStub'
import { renderApp } from '../testing/renderApp'

afterEach(() => {
  vi.unstubAllGlobals()
})

const taro = { id: 1, name: '田中', login_id: 'taro', role: 'member', must_change_password: false }

/** item は既定値の揃った1件を作る。テストごとに気にする列だけを渡す。 */
function item(over: Record<string, unknown> = {}) {
  return {
    id: 1,
    code: '0042',
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
    '/api/me': () => jsonResponse({ user: taro, redirect_to: '/' }),
    '/api/items/0042': () => jsonResponse({ item: item() }),
    ...over,
  }
}

test('備品の内容を表示する', async () => {
  stubFetch(routes())

  renderApp('/i/0042')

  expect(await screen.findByRole('heading', { name: '三脚' })).toBeDefined()
  expect(screen.getByText('0042')).toBeDefined()
  expect(screen.getByText('撮影機材')).toBeDefined()
  expect(screen.getByText('SLIK 500')).toBeDefined()
  expect(screen.getByText('部室A棚')).toBeDefined()
  expect(screen.getByText('サークル')).toBeDefined()
})

// __M1では貸出ボタンを置かない。__ 借用はM2で作る。押せないボタンを
// 先に置くと、触った人には壊れているとしか見えない。
test('借りるボタンを置かない', async () => {
  stubFetch(routes())

  renderApp('/i/0042')
  await screen.findByRole('heading', { name: '三脚' })

  expect(screen.queryByRole('button', { name: /借り/ })).toBeNull()
  expect(screen.queryByRole('link', { name: /借り/ })).toBeNull()
})

test('空欄の項目は—で埋める', async () => {
  stubFetch(routes({ '/api/items/0042': () => jsonResponse({ item: item({ model: '', note: '' }) }) }))

  renderApp('/i/0042')
  await screen.findByRole('heading', { name: '三脚' })

  expect(screen.getAllByText('—').length).toBe(2)
})

// 廃棄は物理削除の代わり。ラベルは貼られたままなのでQRを読めばここに来る。
// 引けないと「読み取れない壊れたラベル」に見える。
test('廃棄済みでも表示し、廃棄であることを出す', async () => {
  stubFetch(routes({ '/api/items/0042': () => jsonResponse({ item: item({ condition: '廃棄' }) }) }))

  renderApp('/i/0042')

  expect(await screen.findByRole('heading', { name: '三脚' })).toBeDefined()
  expect(screen.getByText('この備品は廃棄されています。')).toBeDefined()
})

// 記録されなかった事実を「正常」として表示しない（CLAUDE.md）。
test('所在不明を明示する', async () => {
  stubFetch(
    routes({
      '/api/items/0042': () => jsonResponse({ item: item({ location_status: '所在不明_未確認' }) }),
    }),
  )

  renderApp('/i/0042')

  expect(await screen.findByText('所在: 所在不明_未確認')).toBeDefined()
})

// 自由利用品は貸出フローの対象外。記録が要らないことを伝えないと、
// 借用の記録方法を探させることになる。
test('自由利用品は記録が要らないことを出す', async () => {
  stubFetch(
    routes({ '/api/items/0042': () => jsonResponse({ item: item({ is_free_use: true }) }) }),
  )

  renderApp('/i/0042')

  expect(await screen.findByText('自由利用品です。借用の記録は要りません。')).toBeDefined()
})

test('写真があれば表示する', async () => {
  stubFetch(
    routes({
      '/api/items/0042': () =>
        jsonResponse({ item: item({ photo_url: '/api/items/0042/photo' }) }),
    }),
  )

  renderApp('/i/0042')

  const img = await screen.findByRole('img', { name: '三脚の写真' })
  expect(img).toHaveProperty('src', expect.stringContaining('/api/items/0042/photo'))
})

test('写真が無ければ画像を出さない', async () => {
  stubFetch(routes())

  renderApp('/i/0042')
  await screen.findByRole('heading', { name: '三脚' })

  expect(screen.queryByRole('img')).toBeNull()
})

// QRを読んで来た人が最初に見る画面。「エラー」とだけ出しても次の手が
// 分からない。
test('登録の無いコードは心当たりまで出す', async () => {
  stubFetch(
    routes({
      '/api/items/0042': () => errorResponse('その備品コードは登録されていません', 404),
    }),
  )

  renderApp('/i/0042')

  expect(
    await screen.findByRole('heading', { name: '登録されていない備品コードです' }),
  ).toBeDefined()
  expect(screen.getByText(/まだ登録されていない可能性があります/)).toBeDefined()
})

test('取得に失敗したら理由を出す', async () => {
  stubFetch(
    routes({ '/api/items/0042': () => errorResponse('サーバ側で問題が起きました', 500) }),
  )

  renderApp('/i/0042')

  expect(await screen.findByRole('alert')).toHaveProperty(
    'textContent',
    'サーバ側で問題が起きました',
  )
})

// __M1の最重要導線。__ QRから来た未ログインの利用者を認証後にトップへ
// 放り出すと、もう一度QRを読み直させることになり、その一手間が
// 記録漏れの直接原因になる（CLAUDE.md）。
test('未ログインでQRから来ても、ログイン後に同じ備品へ戻る', async () => {
  const fetchMock = stubFetch({
    // 最初は未ログイン。ログイン後の状態はサーバが応答で返すため、
    // /api/me を呼び直す必要はない。
    '/api/me': () => errorResponse('ログインしてください', 401),
    // サーバは next を検証し、安全なら redirect_to として返す。
    '/api/login': () => jsonResponse({ user: taro, redirect_to: '/i/0042' }),
    '/api/items/0042': () => jsonResponse({ item: item() }),
  })

  renderApp('/i/0042')

  // ログイン画面へ送られる。
  await screen.findByRole('heading', { name: 'ログイン' })

  fireEvent.change(screen.getByLabelText('ログインID'), { target: { value: 'taro' } })
  fireEvent.change(screen.getByLabelText('パスワード'), { target: { value: 'password123' } })
  fireEvent.click(screen.getByRole('button', { name: 'ログイン' }))

  // 元の備品へ戻っている。
  expect(await screen.findByRole('heading', { name: '三脚' })).toBeDefined()

  // 復帰先はサーバへ渡している（解釈はサーバが行う）。
  const login = fetchMock.mock.calls.find(([path]) => path === '/api/login')
  expect(JSON.parse(String(login?.[1]?.body))).toMatchObject({ next: '/i/0042' })
})
