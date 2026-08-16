import { useEffect, useState } from 'react'
import { Link } from 'react-router'

import { errorMessage } from '../api/client'
import { itemFilters, listItems } from '../api/items'
import type { FilterOptions } from '../api/items'
import type { Item } from '../api/types'

/**
 * AdminLabels はQRラベルの印刷画面（運営のみ）。
 *
 * PDFはリンクで開く。fetch して組み立て直さない。サーバは
 * `Content-Disposition: inline` で返しており、__ブラウザのPDFビューアで__
 * __確認してから印刷できる__ ことが狙い（ラベルシールは刷り直しが効かない）。
 *
 * 対象0件だとサーバは400を返す。白紙のシートを刷るとラベルシールが1枚
 * 無駄になるため、その前に画面側で件数を出し、0件ならリンクを出さない。
 */
export default function AdminLabels() {
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')
  const [category, setCategory] = useState('')

  const [items, setItems] = useState<Item[] | null>(null)
  const [error, setError] = useState('')
  const [options, setOptions] = useState<FilterOptions>({ categories: [], locations: [] })

  useEffect(() => {
    let alive = true
    setItems(null)
    setError('')

    // 廃棄済みは含めない（既定のまま）。棚に並ばないものにラベルを刷る理由がない。
    // 自由利用品は含める。M1のラベルは備品詳細を見るためのQRであって、
    // 貸出の入口ではない。どちらもラベルの経路と同じ扱いにしてある。
    void listItems({ category }).then(
      (list) => {
        if (alive) setItems(list)
      },
      (err: unknown) => {
        if (alive) setError(errorMessage(err))
      },
    )

    return () => {
      alive = false
    }
  }, [category])

  useEffect(() => {
    let alive = true
    void itemFilters().then(
      (opts) => {
        if (alive) setOptions(opts)
      },
      () => {},
    )
    return () => {
      alive = false
    }
  }, [])

  const fromNum = parseCode(from)
  const toNum = parseCode(to)

  const invalidFrom = from !== '' && fromNum === null
  const invalidTo = to !== '' && toNum === null
  // 逆順の指定は0件になる。0件と同じ扱いにすると「その範囲に備品が無い」と
  // 読めてしまい、打ち間違いに気付けない（サーバも400で弾く）。
  const reversed = fromNum !== null && toNum !== null && fromNum > toNum

  const targets =
    items === null || invalidFrom || invalidTo || reversed
      ? []
      : items.filter((it) => inRange(it.code, fromNum, toNum))

  return (
    <main className="mx-auto max-w-screen-sm p-4">
      <h1 className="text-xl font-bold">QRラベルの印刷</h1>
      <p className="mt-1 text-sm text-gray-600">
        A4のラベルシート（24面）にQRと備品コード・品名を並べたPDFを作ります。
      </p>

      <div className="mt-4 grid grid-cols-2 gap-3">
        <div>
          <label className="block text-sm font-medium" htmlFor="from">
            開始コード
          </label>
          <input
            id="from"
            inputMode="numeric"
            placeholder="0001"
            className="mt-1 w-full rounded border border-gray-300 px-3 py-2 text-base"
            value={from}
            onChange={(e) => setFrom(e.target.value)}
          />
        </div>
        <div>
          <label className="block text-sm font-medium" htmlFor="to">
            終了コード
          </label>
          <input
            id="to"
            inputMode="numeric"
            placeholder="0050"
            className="mt-1 w-full rounded border border-gray-300 px-3 py-2 text-base"
            value={to}
            onChange={(e) => setTo(e.target.value)}
          />
        </div>
      </div>
      <p className="mt-1 text-xs text-gray-600">
        空欄なら端まで。「0042」でも「42」でも構いません。
      </p>

      <div className="mt-3">
        <label className="block text-sm font-medium" htmlFor="category">
          分類
        </label>
        <select
          id="category"
          className="mt-1 w-full rounded border border-gray-300 px-2 py-2 text-base"
          value={category}
          onChange={(e) => setCategory(e.target.value)}
        >
          <option value="">すべて</option>
          {options.categories.map((c) => (
            <option key={c} value={c}>
              {c}
            </option>
          ))}
        </select>
      </div>

      <p className="mt-3 text-xs text-gray-600">
        廃棄済みは含まれません。自由利用品は含まれます。
      </p>

      {error !== '' && (
        <p role="alert" className="mt-4 text-sm text-red-700">
          {error}
        </p>
      )}

      {(invalidFrom || invalidTo) && (
        <p role="alert" className="mt-4 text-sm text-red-700">
          備品コードは1以上の数字で指定してください。
        </p>
      )}

      {reversed && (
        <p role="alert" className="mt-4 text-sm text-red-700">
          備品コードの範囲が逆です。
        </p>
      )}

      {items !== null && error === '' && !invalidFrom && !invalidTo && !reversed && (
        <section className="mt-4">
          <p>
            対象: <strong>{targets.length}件</strong>
          </p>

          {targets.length === 0 ? (
            <p className="mt-2 text-sm text-gray-600">
              条件に合う備品がありません。範囲か分類を見直してください。
            </p>
          ) : (
            <>
              {/* 何が刷られるかを確定前に見せる。ラベルシールは刷り直しが効かない。 */}
              <ul className="mt-2 max-h-64 divide-y divide-gray-200 overflow-y-auto border-y border-gray-200">
                {targets.map((it) => (
                  <li key={it.id} className="flex gap-2 py-1 text-sm">
                    <span className="font-mono text-gray-600">{it.code}</span>
                    <span>{it.name}</span>
                  </li>
                ))}
              </ul>

              {/*
                リンクで開く。fetch して組み立て直さないのは、ブラウザの
                PDFビューアで確認してからそのまま印刷できるようにするため。
              */}
              <a
                className="mt-4 inline-block rounded bg-blue-700 px-4 py-3 text-white"
                href={`/api/labels.pdf${labelQuery(from, to, category)}`}
                target="_blank"
                rel="noopener"
              >
                PDFを開く（{targets.length}件）
              </a>
            </>
          )}
        </section>
      )}

      <div className="mt-6">
        <Link className="text-blue-700 underline" to="/admin/items">
          マスタ管理へ
        </Link>
      </div>
    </main>
  )
}

/**
 * labelQuery は印刷範囲のクエリを作る。
 *
 * 空の項目は送らない。サーバは空を「指定なし」として扱うが、
 * 送らない方がURLを見た時に何を指定したかが分かる。
 */
function labelQuery(from: string, to: string, category: string): string {
  const params = new URLSearchParams()
  if (from.trim() !== '') params.set('from', from.trim())
  if (to.trim() !== '') params.set('to', to.trim())
  if (category !== '') params.set('category', category)

  const query = params.toString()
  return query === '' ? '' : `?${query}`
}

/** parseCode は範囲指定を数値にする。空なら null（指定なし）。不正でも null。 */
function parseCode(raw: string): number | null {
  const trimmed = raw.trim()
  if (trimmed === '') return null
  if (!/^\d+$/.test(trimmed)) return null

  const n = Number(trimmed)
  // 採番は 0001 から。0 は「指定なし」と区別が付かない（サーバも同じ判定）。
  return n >= 1 ? n : null
}

/**
 * inRange は備品コードが範囲に入るかを返す。
 *
 * __数値として比較する。__ 文字列で比べると "10000" < "9999" になり、
 * 4桁を超えた備品が範囲から静かに漏れる（サーバも CAST して比べている）。
 */
function inRange(code: string, from: number | null, to: number | null): boolean {
  const n = Number(code)
  if (!Number.isFinite(n)) return false
  if (from !== null && n < from) return false
  if (to !== null && n > to) return false
  return true
}
