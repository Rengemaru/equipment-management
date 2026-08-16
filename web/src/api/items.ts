/**
 * 備品まわりのエンドポイント。
 *
 * 読み取りは member も通る。誰が何を持っているかが全員に見える状態を作る
 * ことが、罰則より強く働く（CLAUDE.md）。
 */

import { request, requestJSON } from './client'
import type { Condition, Item, ItemAttributes, LocationStatus } from './types'

/**
 * ItemFilter は一覧の絞り込み。空の項目は条件にしない。
 *
 * 廃棄済みは既定で除かれる。ただし `condition: '廃棄'` を指定すれば
 * サーバ側が例外として通す。廃棄を見る手段まで塞ぐと、
 * 「登録したはずのものが消えた」に見える。
 */
export type ItemFilter = {
  /** query は品名・備品コード・型番の部分一致。 */
  query?: string

  category?: string
  location?: string
  condition?: Condition | ''
  locationStatus?: LocationStatus | ''

  /**
   * includeDiscarded は廃棄済みも含めるか。
   *
   * 運営が棚を整理する時に要る。普段の一覧では除いたままにする。
   */
  includeDiscarded?: boolean
}

/** FilterOptions は絞り込みの選択肢。分類と保管場所は自由入力のため固定できない。 */
export type FilterOptions = {
  categories: string[]
  locations: string[]
}

/** listItems は条件に合う備品を返す。 */
export async function listItems(filter: ItemFilter): Promise<Item[]> {
  const params = new URLSearchParams()

  // 空の項目を送らない。送ると「空文字に一致するもの」を探す条件として
  // 解釈されかねず、サーバ側の実装に依存した動きになる。
  appendIfSet(params, 'q', filter.query)
  appendIfSet(params, 'category', filter.category)
  appendIfSet(params, 'location', filter.location)
  appendIfSet(params, 'condition', filter.condition)
  appendIfSet(params, 'location_status', filter.locationStatus)
  if (filter.includeDiscarded === true) {
    params.set('include_discarded', '1')
  }

  const query = params.toString()
  const res = await request<{ items: Item[] }>(`/api/items${query === '' ? '' : `?${query}`}`)
  return res.items
}

/**
 * getItem は備品コードで1件を引く。
 *
 * 廃棄済みも引ける。一覧からは除かれるが、__ラベルは貼られたままなので__
 * QRを読めばここに来る。引けないと「読み取れない壊れたラベル」に見える。
 *
 * 登録が無ければ 404 の ApiError を投げる。
 */
export async function getItem(code: string): Promise<Item> {
  const res = await request<{ item: Item }>(`/api/items/${encodeURIComponent(code)}`)
  return res.item
}

/**
 * createItem は備品を1件登録する。admin のみ。
 *
 * **備品コードは送らない。** システムが `0001` 形式で自動採番する。
 * 人手で振らせると抜け・重複が必ず起きる（CLAUDE.md）。サーバも
 * コードが入っていれば 400 で弾く。採番された値は戻り値で分かる。
 */
export async function createItem(attrs: ItemAttributes): Promise<Item> {
  const res = await requestJSON<{ item: Item }>('/api/items', 'POST', attrs)
  return res.item
}

/**
 * uploadPhoto は写真を添付する。admin のみ。差し替えも同じ経路。
 *
 * 備品が登録済みでないと呼べない。登録より先に写真だけを送る経路は無い。
 *
 * Content-Type を自分で指定しないこと。FormData を渡すとブラウザが
 * 境界文字列付きで組み立てる。手で書くと境界がずれ、サーバが本文を読めない。
 */
export async function uploadPhoto(code: string, file: File): Promise<Item> {
  const form = new FormData()
  form.set('photo', file)

  const res = await request<{ item: Item }>(`/api/items/${encodeURIComponent(code)}/photo`, {
    method: 'POST',
    body: form,
  })
  return res.item
}

/**
 * updateItem は備品の内容を差し替える。admin のみ。
 *
 * 全項目を送る。備品コードは送らない（変更できない）。
 *
 * 廃棄も状態の1つとしてここで指定する。専用の
 * `POST /api/items/{code}/discard` は使わない。更新が全項目を送る形である
 * 以上、状態だけ別経路にすると、__廃棄済みの備品を編集した時に__
 * __状態を送り戻して復活させてしまう__ 経路ができる。
 */
export async function updateItem(code: string, attrs: ItemAttributes): Promise<Item> {
  const res = await requestJSON<{ item: Item }>(
    `/api/items/${encodeURIComponent(code)}`,
    'PUT',
    attrs,
  )
  return res.item
}

/** itemFilters は実際に使われている分類と保管場所を返す。 */
export async function itemFilters(): Promise<FilterOptions> {
  return request<FilterOptions>('/api/items/filters')
}

function appendIfSet(params: URLSearchParams, key: string, value: string | undefined): void {
  const v = value?.trim() ?? ''
  if (v !== '') {
    params.set(key, v)
  }
}
