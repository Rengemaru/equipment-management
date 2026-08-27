import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { errorResponse, jsonResponse, stubFetch } from '../testing/fetchStub'
import { renderApp } from '../testing/renderApp'

afterEach(() => {
  vi.unstubAllGlobals()
})

const taro = { id: 1, name: '田中', login_id: 'taro', role: 'member', must_change_password: false }

/** item は既定値の揃った1件を作る。テストごとに気にする列だけを渡す。 */
function item(over: Record<string, unknown>) {
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

/**
 * routes は一覧画面が使う経路をまとめる。
 *
 * 一覧の応答は呼び出しごとに変わるため、直近のクエリを記録できるよう
 * 関数で受ける。
 */
function routes(items: unknown[]) {
  return {
    '/api/me': () => jsonResponse({ user: taro, redirect_to: '/' }),
    '/api/items': () => jsonResponse({ items }),
    '/api/items/filters': () =>
      jsonResponse({ categories: ['撮影機材', '工具'], locations: ['部室A棚', '倉庫'] }),
  }
}

/** lastItemsQuery は最後に一覧を取りに行ったクエリ文字列を返す。 */
function lastItemsQuery(fetchMock: ReturnType<typeof stubFetch>): string {
  // /api/items/filters は別経路なので除く。
  const calls = fetchMock.mock.calls.filter(([path]) => path.startsWith('/api/items?'))
  return calls.at(-1)?.[0].slice('/api/items'.length) ?? ''
}

test('一覧を表示する', async () => {
  stubFetch(routes([item({}), item({ id: 2, code: '0002', name: '六角レンチ' })]))

  renderApp('/items')

  expect(await screen.findByText('三脚')).toBeDefined()
  expect(screen.getByText('六角レンチ')).toBeDefined()
  expect(screen.getByText('2件')).toBeDefined()
})

// 行全体をリンクにする。指で押す的が小さいと、スマートフォンでは
// 隣の行を開くことになる。
test('各行から詳細へ行ける', async () => {
  stubFetch(routes([item({ code: '0042' })]))

  renderApp('/items')
  const list = within(await screen.findByRole('list'))

  expect(list.getByRole('link', { name: /三脚/ })).toHaveProperty(
    'href',
    expect.stringContaining('/i/0042'),
  )
})

// 備品コードは棚に貼ったラベルと見比べるためのもの。品名と並べて出す。
test('備品コードと分類・保管場所を出す', async () => {
  stubFetch(routes([item({})]))

  renderApp('/items')

  expect(await screen.findByText('0001')).toBeDefined()
  expect(screen.getByText('撮影機材・SLIK 500・部室A棚')).toBeDefined()
})

// 記録されなかった事実を「正常」として表示しない。不確かな状態は
// 不確かなまま出す（CLAUDE.md）。
// 絞り込みの選択肢にも同じ文字列が出るため、一覧の中だけを見る。
test('要修理・所在不明・自由利用品を明示する', async () => {
  stubFetch(
    routes([
      item({ condition: '要修理', location_status: '所在不明_未確認', is_free_use: true }),
    ]),
  )

  renderApp('/items')
  const list = within(await screen.findByRole('list'))

  expect(list.getByText('要修理')).toBeDefined()
  expect(list.getByText('所在不明_未確認')).toBeDefined()
  expect(list.getByText('自由利用品')).toBeDefined()
})

// 全ての行に付くと、注意すべき行が埋もれる。
test('良好・在庫は目印を出さない', async () => {
  stubFetch(routes([item({})]))

  renderApp('/items')
  const list = within(await screen.findByRole('list'))

  expect(list.queryByText('良好')).toBeNull()
  expect(list.queryByText('在庫')).toBeNull()
})

test('検索語をサーバへ渡す', async () => {
  const fetchMock = stubFetch(routes([item({})]))

  renderApp('/items')
  await screen.findByText('三脚')

  fireEvent.change(screen.getByLabelText('品名・備品コード・型番で検索'), {
    target: { value: '三脚' },
  })
  fireEvent.click(screen.getByRole('button', { name: '検索' }))

  await waitFor(() => {
    expect(lastItemsQuery(fetchMock)).toBe(`?q=${encodeURIComponent('三脚')}`)
  })
})

test('分類で絞るとサーバへ渡す', async () => {
  const fetchMock = stubFetch(routes([item({})]))

  renderApp('/items')
  await screen.findByText('三脚')

  fireEvent.change(screen.getByLabelText('分類'), { target: { value: '工具' } })

  await waitFor(() => {
    expect(lastItemsQuery(fetchMock)).toContain(`category=${encodeURIComponent('工具')}`)
  })
})

test('状態と所在で絞るとサーバへ渡す', async () => {
  const fetchMock = stubFetch(routes([item({})]))

  renderApp('/items')
  await screen.findByText('三脚')

  fireEvent.change(screen.getByLabelText('状態'), { target: { value: '廃棄' } })
  await waitFor(() => {
    expect(lastItemsQuery(fetchMock)).toContain(`condition=${encodeURIComponent('廃棄')}`)
  })

  fireEvent.change(screen.getByLabelText('所在'), { target: { value: '所在不明_未確認' } })
  await waitFor(() => {
    expect(lastItemsQuery(fetchMock)).toContain(
      `location_status=${encodeURIComponent('所在不明_未確認')}`,
    )
  })
})

// URLに条件を出しておけば「この条件の一覧」をそのまま人に渡せる。
test('URLのクエリを初期条件として読む', async () => {
  const fetchMock = stubFetch(routes([item({})]))

  renderApp(
    `/items?q=${encodeURIComponent('三脚')}&category=${encodeURIComponent('撮影機材')}`,
  )

  await screen.findByText('三脚')

  const query = lastItemsQuery(fetchMock)
  expect(query).toContain(`q=${encodeURIComponent('三脚')}`)
  expect(query).toContain(`category=${encodeURIComponent('撮影機材')}`)
  expect(screen.getByLabelText('品名・備品コード・型番で検索')).toHaveProperty('value', '三脚')
})

// 廃棄済みは既定で除かれる。「登録したはずのものが消えた」に見えないよう、
// 出し方を書いておく。
test('0件のときは理由と次の手を出す', async () => {
  stubFetch(routes([]))

  renderApp('/items')

  expect(await screen.findByText('0件')).toBeDefined()
  expect(screen.getByText(/該当する備品がありません/)).toBeDefined()
  expect(screen.getByText(/状態で「廃棄」を選ぶと表示されます/)).toBeDefined()
})

test('取得に失敗したら理由を出す', async () => {
  stubFetch({
    '/api/me': () => jsonResponse({ user: taro, redirect_to: '/' }),
    '/api/items': () => errorResponse('サーバ側で問題が起きました', 500),
    '/api/items/filters': () => jsonResponse({ categories: [], locations: [] }),
  })

  renderApp('/items')

  expect(await screen.findByRole('alert')).toHaveProperty(
    'textContent',
    'サーバ側で問題が起きました',
  )
})

// 選択肢が取れなくても、検索語と状態での絞り込みは効く。画面ごと止めない。
test('選択肢の取得に失敗しても一覧は出す', async () => {
  stubFetch({
    '/api/me': () => jsonResponse({ user: taro, redirect_to: '/' }),
    '/api/items': () => jsonResponse({ items: [item({})] }),
    '/api/items/filters': () => errorResponse('サーバ側で問題が起きました', 500),
  })

  renderApp('/items')

  expect(await screen.findByText('三脚')).toBeDefined()
  expect(screen.getByLabelText('状態')).toBeDefined()
})
