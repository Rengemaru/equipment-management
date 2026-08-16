import { useCallback, useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { Link, useSearchParams } from 'react-router'

import { errorMessage } from '../api/client'
import { listItems, updateItem } from '../api/items'
import { CONDITIONS, LOCATION_STATUSES, OWNERS } from '../api/types'
import type { Item, ItemAttributes } from '../api/types'

/**
 * AdminItems は備品マスタの管理画面（運営のみ）。
 *
 * 削除は無い。廃棄は状態であって削除ではなく、行を消すと貸出履歴の
 * 参照先が消える（CLAUDE.md）。廃棄にするには状態を「廃棄」にする。
 */
export default function AdminItems() {
  const [params, setParams] = useSearchParams()

  const query = params.get('q') ?? ''
  const includeDiscarded = params.get('include_discarded') === '1'

  // 検索語だけは入力中の値を画面に持つ。1文字ごとに問い合わせない。
  const [queryInput, setQueryInput] = useState(query)

  const [items, setItems] = useState<Item[] | null>(null)
  const [error, setError] = useState('')

  /** editing は編集中の備品コード。1件ずつしか開かない。 */
  const [editing, setEditing] = useState('')
  const [saved, setSaved] = useState('')

  useEffect(() => {
    let alive = true
    setItems(null)
    setError('')

    void listItems({ query, includeDiscarded }).then(
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
  }, [query, includeDiscarded])

  const update = (key: string, value: string) => {
    const next = new URLSearchParams(params)
    if (value === '') next.delete(key)
    else next.set(key, value)
    setParams(next, { replace: true })
  }

  /** replaceItem は保存した1件だけを差し替える。一覧を取り直さない。 */
  const replaceItem = useCallback((updated: Item) => {
    setItems((prev) => prev?.map((it) => (it.code === updated.code ? updated : it)) ?? null)
    setEditing('')
    setSaved(updated.code)
  }, [])

  return (
    <main className="mx-auto max-w-screen-sm p-4">
      <h1 className="text-xl font-bold">備品マスタ管理</h1>

      <form
        className="mt-4"
        role="search"
        onSubmit={(e: FormEvent<HTMLFormElement>) => {
          e.preventDefault()
          update('q', queryInput)
        }}
      >
        <label className="block text-sm font-medium" htmlFor="q">
          品名・備品コード・型番で検索
        </label>
        <div className="mt-1 flex gap-2">
          <input
            id="q"
            type="search"
            autoCapitalize="none"
            autoCorrect="off"
            spellCheck={false}
            className="w-full rounded border border-gray-300 px-3 py-2 text-base"
            value={queryInput}
            onChange={(e) => setQueryInput(e.target.value)}
          />
          <button type="submit" className="shrink-0 rounded bg-blue-700 px-4 py-2 text-white">
            検索
          </button>
        </div>
      </form>

      <label className="mt-3 flex items-center gap-2 text-sm">
        <input
          type="checkbox"
          className="size-4"
          checked={includeDiscarded}
          onChange={(e) => update('include_discarded', e.target.checked ? '1' : '')}
        />
        廃棄済みも表示する
      </label>

      {error !== '' && (
        <p role="alert" className="mt-4 text-sm text-red-700">
          {error}
        </p>
      )}

      {error === '' && items === null && <p className="mt-4 text-sm text-gray-600">読み込み中…</p>}

      {items !== null && (
        <>
          <p className="mt-4 text-sm text-gray-600">{items.length}件</p>

          <ul className="mt-2 divide-y divide-gray-200">
            {items.map((it) => (
              <li key={it.id} className="py-3">
                <div className="flex items-baseline gap-2">
                  <span className="font-mono text-sm text-gray-600">{it.code}</span>
                  <span className="font-medium">{it.name}</span>
                </div>

                <p className="mt-0.5 text-sm text-gray-600">
                  {[it.category, it.model, it.location].filter((v) => v !== '').join('・')}
                </p>

                <div className="mt-1 flex flex-wrap items-center gap-1">
                  {it.condition !== '良好' && <Badge>{it.condition}</Badge>}
                  {it.location_status !== '在庫' && <Badge>{it.location_status}</Badge>}
                  {it.is_free_use && <Badge>自由利用品</Badge>}
                </div>

                {saved === it.code && editing !== it.code && (
                  <p role="status" className="mt-2 text-sm text-green-800">
                    保存しました
                  </p>
                )}

                {editing === it.code ? (
                  <EditForm
                    item={it}
                    onSaved={replaceItem}
                    onCancel={() => setEditing('')}
                  />
                ) : (
                  <div className="mt-2 flex gap-3">
                    <button
                      className="rounded border border-gray-300 px-3 py-1 text-sm"
                      onClick={() => {
                        setSaved('')
                        setEditing(it.code)
                      }}
                    >
                      編集
                    </button>
                    <Link className="self-center text-sm text-blue-700 underline" to={`/i/${it.code}`}>
                      詳細
                    </Link>
                  </div>
                )}
              </li>
            ))}
          </ul>
        </>
      )}
    </main>
  )
}

/**
 * EditForm は1件分の編集。
 *
 * 全項目を送る。一部だけ送る形にすると、画面で消した備考が消えないなど、
 * 意図と結果がずれる（サーバも全項目を要求する）。
 *
 * 備品コードは出すだけで変えられない。ラベルは貼り替えられないため、
 * 変えると実物との対応が壊れる。
 */
function EditForm({
  item,
  onSaved,
  onCancel,
}: {
  item: Item
  onSaved: (updated: Item) => void
  onCancel: () => void
}) {
  const [attrs, setAttrs] = useState<ItemAttributes>(toAttributes(item))
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)

  const set = <K extends keyof ItemAttributes>(key: K, value: ItemAttributes[K]) => {
    setAttrs((prev) => ({ ...prev, [key]: value }))
  }

  const handleSubmit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    setError('')
    setSaving(true)

    try {
      onSaved(await updateItem(item.code, attrs))
    } catch (err) {
      setError(errorMessage(err))
      setSaving(false)
    }
  }

  return (
    <form className="mt-3 space-y-3 rounded border border-gray-300 p-3" onSubmit={(e) => void handleSubmit(e)}>
      <Text id={`name-${item.code}`} label="品名" value={attrs.name} onChange={(v) => set('name', v)} required />
      <Text
        id={`category-${item.code}`}
        label="分類"
        value={attrs.category}
        onChange={(v) => set('category', v)}
      />
      <Text id={`model-${item.code}`} label="型番" value={attrs.model} onChange={(v) => set('model', v)} />
      <Text
        id={`location-${item.code}`}
        label="保管場所"
        value={attrs.location}
        onChange={(v) => set('location', v)}
      />

      <Choice
        id={`owner-${item.code}`}
        label="所有"
        value={attrs.owner}
        options={OWNERS}
        onChange={(v) => set('owner', v as ItemAttributes['owner'])}
      />
      {/* 廃棄もここで指定する。状態の1つであって削除ではない。 */}
      <Choice
        id={`condition-${item.code}`}
        label="状態"
        value={attrs.condition}
        options={CONDITIONS}
        onChange={(v) => set('condition', v as ItemAttributes['condition'])}
      />
      <Choice
        id={`location_status-${item.code}`}
        label="所在"
        value={attrs.location_status}
        options={LOCATION_STATUSES}
        onChange={(v) => set('location_status', v as ItemAttributes['location_status'])}
      />

      <label className="flex items-center gap-2 text-sm">
        <input
          type="checkbox"
          className="size-4"
          checked={attrs.is_free_use}
          onChange={(e) => set('is_free_use', e.target.checked)}
        />
        自由利用品にする（貸出の記録を求めない）
      </label>

      <div>
        <label className="block text-sm font-medium" htmlFor={`note-${item.code}`}>
          備考
        </label>
        <textarea
          id={`note-${item.code}`}
          rows={2}
          className="mt-1 w-full rounded border border-gray-300 px-3 py-2 text-base"
          value={attrs.note}
          onChange={(e) => set('note', e.target.value)}
        />
      </div>

      {error !== '' && (
        <p role="alert" className="text-sm text-red-700">
          {error}
        </p>
      )}

      <div className="flex gap-2">
        <button
          type="submit"
          disabled={saving}
          className="rounded bg-blue-700 px-4 py-2 text-white disabled:bg-gray-400"
        >
          {saving ? '保存しています…' : '保存'}
        </button>
        <button
          type="button"
          className="rounded border border-gray-300 px-4 py-2"
          onClick={onCancel}
        >
          キャンセル
        </button>
      </div>
    </form>
  )
}

/** toAttributes は表示用の Item から送信用の形を作る。 */
function toAttributes(item: Item): ItemAttributes {
  return {
    name: item.name,
    category: item.category,
    model: item.model,
    owner: item.owner,
    is_free_use: item.is_free_use,
    location: item.location,
    condition: item.condition,
    location_status: item.location_status,
    note: item.note,
  }
}

function Badge({ children }: { children: string }) {
  return <span className="rounded bg-amber-100 px-2 py-0.5 text-xs text-amber-900">{children}</span>
}

function Text({
  id,
  label,
  value,
  onChange,
  required,
}: {
  id: string
  label: string
  value: string
  onChange: (value: string) => void
  required?: boolean
}) {
  return (
    <div>
      <label className="block text-sm font-medium" htmlFor={id}>
        {label}
      </label>
      <input
        id={id}
        required={required}
        // text-base（16px）未満だと iOS が焦点を当てた瞬間に拡大する。
        className="mt-1 w-full rounded border border-gray-300 px-3 py-2 text-base"
        value={value}
        onChange={(e) => onChange(e.target.value)}
      />
    </div>
  )
}

function Choice({
  id,
  label,
  value,
  options,
  onChange,
}: {
  id: string
  label: string
  value: string
  options: readonly string[]
  onChange: (value: string) => void
}) {
  return (
    <div>
      <label className="block text-sm font-medium" htmlFor={id}>
        {label}
      </label>
      <select
        id={id}
        className="mt-1 w-full rounded border border-gray-300 px-2 py-2 text-base"
        value={value}
        onChange={(e) => onChange(e.target.value)}
      >
        {options.map((opt) => (
          <option key={opt} value={opt}>
            {opt}
          </option>
        ))}
      </select>
    </div>
  )
}
