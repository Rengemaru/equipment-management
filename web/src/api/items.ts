/**
 * 備品まわりのエンドポイント。
 *
 * 読み取りは member も通る。誰が何を持っているかが全員に見える状態を作る
 * ことが、罰則より強く働く（CLAUDE.md）。
 */

import { request } from './client'
import type { Condition, Item, LocationStatus } from './types'

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
