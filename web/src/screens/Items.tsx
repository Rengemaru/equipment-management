import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { Link, useSearchParams } from 'react-router'

import { errorMessage } from '../api/client'
import { itemFilters, listItems } from '../api/items'
import type { FilterOptions } from '../api/items'
import { CONDITIONS, LOCATION_STATUSES } from '../api/types'
import type { Condition, Item, LocationStatus } from '../api/types'

/**
 * Items は備品の一覧・検索画面。
 *
 * 絞り込みの状態はURLのクエリに持つ。画面の中だけに持つと、戻る操作で
 * 条件が消え、探し直しになる。URLに出しておけば「この条件の一覧」を
 * そのまま人に渡せる。
 */
export default function Items() {
  const [params, setParams] = useSearchParams()

  const query = params.get('q') ?? ''
  const category = params.get('category') ?? ''
  const location = params.get('location') ?? ''
  const condition = params.get('condition') ?? ''
  const locationStatus = params.get('location_status') ?? ''

  // 検索語だけは入力中の値を画面に持つ。1文字ごとに問い合わせると、
  // 打っている間ずっと通信が走り、部室の回線では入力が詰まる。
  const [queryInput, setQueryInput] = useState(query)

  const [items, setItems] = useState<Item[] | null>(null)
  const [error, setError] = useState('')
  const [options, setOptions] = useState<FilterOptions>({ categories: [], locations: [] })

  useEffect(() => {
    // 条件を変えた直後に古い応答が届くと、新しい条件の結果を上書きする。
    let alive = true

    setItems(null)
    setError('')

    void listItems({
      query,
      category,
      location,
      condition: condition as Condition | '',
      locationStatus: locationStatus as LocationStatus | '',
    }).then(
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
  }, [query, category, location, condition, locationStatus])

  useEffect(() => {
    // 選択肢が取れなくても画面は使える（検索語と状態での絞り込みは効く）。
    // ここで画面ごと止めない。
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

  /** update は条件を1つ差し替える。 */
  const update = (key: string, value: string) => {
    const next = new URLSearchParams(params)
    if (value === '') {
      next.delete(key)
    } else {
      next.set(key, value)
    }
    // replace にする。絞り込みを変えるたびに履歴が積まれると、
    // 戻る操作が一覧から出るのではなく条件を1つずつ遡ることになる。
    setParams(next, { replace: true })
  }

  const handleSearch = (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    update('q', queryInput)
  }

  return (
    <main className="mx-auto max-w-screen-sm p-4">
      <h1 className="text-xl font-bold">備品一覧</h1>

      <form className="mt-4" onSubmit={handleSearch} role="search">
        <label className="block text-sm font-medium" htmlFor="q">
          品名・備品コード・型番で検索
        </label>
        <div className="mt-1 flex gap-2">
          <input
            id="q"
            // type="search" にすると、スマートフォンのキーボードが
            // 改行ではなく「検索」になる。
            type="search"
            autoCapitalize="none"
            autoCorrect="off"
            spellCheck={false}
            // text-base（16px）未満だと iOS が焦点を当てた瞬間に拡大する。
            className="w-full rounded border border-gray-300 px-3 py-2 text-base"
            value={queryInput}
            onChange={(e) => setQueryInput(e.target.value)}
          />
          <button type="submit" className="shrink-0 rounded bg-blue-700 px-4 py-2 text-white">
            検索
          </button>
        </div>
      </form>

      <div className="mt-3 grid grid-cols-2 gap-2">
        <Select
          id="category"
          label="分類"
          value={category}
          options={options.categories}
          onChange={(v) => update('category', v)}
        />
        <Select
          id="location"
          label="保管場所"
          value={location}
          options={options.locations}
          onChange={(v) => update('location', v)}
        />
        <Select
          id="condition"
          label="状態"
          value={condition}
          options={CONDITIONS}
          onChange={(v) => update('condition', v)}
        />
        <Select
          id="location_status"
          label="所在"
          value={locationStatus}
          options={LOCATION_STATUSES}
          onChange={(v) => update('location_status', v)}
        />
      </div>

      {error !== '' && (
        <p role="alert" className="mt-4 text-sm text-red-700">
          {error}
        </p>
      )}

      {error === '' && items === null && <p className="mt-4 text-sm text-gray-600">読み込み中…</p>}

      {items !== null && (
        <>
          <p className="mt-4 text-sm text-gray-600">{items.length}件</p>

          {items.length === 0 ? (
            <p className="mt-2">
              該当する備品がありません。
              <span className="block text-sm text-gray-600">
                廃棄済みは既定で除いています。状態で「廃棄」を選ぶと表示されます。
              </span>
            </p>
          ) : (
            <ul className="mt-2 divide-y divide-gray-200">
              {items.map((it) => (
                <ItemRow key={it.id} item={it} />
              ))}
            </ul>
          )}
        </>
      )}
    </main>
  )
}

/**
 * ItemRow は1件分の表示。
 *
 * 行全体をリンクにする。指で押す的が小さいと、スマートフォンでは
 * 隣の行を開くことになる。
 */
function ItemRow({ item }: { item: Item }) {
  return (
    <li>
      <Link className="block py-3" to={`/i/${item.code}`}>
        <div className="flex items-baseline gap-2">
          {/* 備品コードは等幅で出す。棚に貼ったラベルと見比べるため。 */}
          <span className="font-mono text-sm text-gray-600">{item.code}</span>
          <span className="font-medium text-blue-800 underline">{item.name}</span>
        </div>

        <p className="mt-0.5 text-sm text-gray-600">
          {[item.category, item.model, item.location].filter((v) => v !== '').join('・')}
        </p>

        <div className="mt-1 flex flex-wrap gap-1">
          {/* 良好・在庫は出さない。全ての行に付くと、注意すべき行が埋もれる。 */}
          {item.condition !== '良好' && <Badge tone="warn">{item.condition}</Badge>}
          {item.location_status !== '在庫' && <Badge tone="warn">{item.location_status}</Badge>}
          {item.is_free_use && <Badge tone="info">自由利用品</Badge>}
        </div>
      </Link>
    </li>
  )
}

function Badge({ tone, children }: { tone: 'warn' | 'info'; children: string }) {
  const color = tone === 'warn' ? 'bg-amber-100 text-amber-900' : 'bg-gray-100 text-gray-700'
  return <span className={`rounded px-2 py-0.5 text-xs ${color}`}>{children}</span>
}

/** Select は絞り込みの1項目。空の選択肢が「指定なし」。 */
function Select({
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
        <option value="">すべて</option>
        {options.map((opt) => (
          <option key={opt} value={opt}>
            {opt}
          </option>
        ))}
      </select>
    </div>
  )
}
