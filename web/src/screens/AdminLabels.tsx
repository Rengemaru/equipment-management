import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router'

import { errorMessage } from '../api/client'
import { itemFilters, listItems } from '../api/items'
import type { FilterOptions } from '../api/items'
import type { Item } from '../api/types'
import { ButtonLink } from '../ui/Button'
import { Alert, Empty } from '../ui/Feedback'
import { Field, FieldGroup, SelectField } from '../ui/Field'
import { Card } from '../ui/List'
import { Screen } from '../ui/Screen'

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
  // 初期値はURLのクエリから取る。CSVで一括登録した直後に
  // 「この範囲を刷る」で来られるようにするため。__採番された範囲を__
  // __人が書き写すと、桁を間違えたぶんだけラベルが無駄になる。__
  const [params] = useSearchParams()

  const [from, setFrom] = useState(params.get('from') ?? '')
  const [to, setTo] = useState(params.get('to') ?? '')
  const [category, setCategory] = useState(params.get('category') ?? '')

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
    <Screen title="QRラベルの印刷" back={{ to: '/admin/items', label: 'マスタ管理' }}>
      <p className="mt-1 px-4 text-[13px] leading-snug text-label-2">
        A4のラベルシート（24面）にQRと備品コード・品名を並べたPDFを作ります。
      </p>

      <FieldGroup header="範囲" footer="空欄なら端まで。「0042」でも「42」でも構いません。">
        <Field
          id="from"
          label="開始コード"
          inputMode="numeric"
          placeholder="0001"
          value={from}
          onChange={(e) => setFrom(e.target.value)}
        />
        <Field
          id="to"
          label="終了コード"
          inputMode="numeric"
          placeholder="0050"
          value={to}
          onChange={(e) => setTo(e.target.value)}
        />
        <SelectField
          id="category"
          label="分類"
          value={category}
          onChange={(e) => setCategory(e.target.value)}
        >
          <option value="">すべて</option>
          {options.categories.map((c) => (
            <option key={c} value={c}>
              {c}
            </option>
          ))}
        </SelectField>
      </FieldGroup>

      {/* 何が含まれ、何が含まれないかは刷る前に読ませる。刷ってから
          「廃棄済みが入っていない」と気付いても、シールは戻らない。 */}
      <p className="mt-3 px-4 text-[13px] leading-snug text-label-2">
        廃棄済みは含まれません。自由利用品は含まれます。
      </p>

      {error !== '' && <Alert>{error}</Alert>}

      {(invalidFrom || invalidTo) && <Alert>備品コードは1以上の数字で指定してください。</Alert>}

      {reversed && <Alert>備品コードの範囲が逆です。</Alert>}

      {items !== null && error === '' && !invalidFrom && !invalidTo && !reversed && (
        <>
          <p className="mt-6 px-4 text-[15px]">
            対象: <strong>{targets.length}件</strong>
          </p>

          {targets.length === 0 ? (
            <Empty
              title="条件に合う備品がありません"
              hint="範囲か分類を見直してください。0件のまま刷ると、ラベルシールが1枚無駄になります。"
            />
          ) : (
            <>
              {/* 何が刷られるかを確定前に見せる。ラベルシールは刷り直しが効かない。 */}
              <Card>
                <ul className="max-h-72 divide-y divide-separator overflow-y-auto">
                  {targets.map((it) => (
                    <li key={it.id} className="flex gap-3 px-4 py-2 text-[15px]">
                      <span className="font-mono tabular-nums text-label-2">{it.code}</span>
                      <span className="truncate">{it.name}</span>
                    </li>
                  ))}
                </ul>
              </Card>

              {/*
                リンクで開く。fetch して組み立て直さないのは、ブラウザの
                PDFビューアで確認してからそのまま印刷できるようにするため。
              */}
              <div className="mt-6">
                <ButtonLink
                  to={`/api/labels.pdf${labelQuery(from, to, category)}`}
                  full
                  target="_blank"
                  rel="noopener"
                >
                  PDFを開く（{targets.length}件）
                </ButtonLink>
              </div>
            </>
          )}
        </>
      )}
    </Screen>
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
