import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { useSearchParams } from 'react-router'

import { errorMessage } from '../api/client'
import { itemFilters, listItems } from '../api/items'
import type { FilterOptions } from '../api/items'
import { CONDITIONS, LOCATION_STATUSES } from '../api/types'
import type { Condition, Item, LocationStatus } from '../api/types'
import { Alert, Badge, Empty, Loading } from '../ui/Feedback'
import { FieldGroup, SelectField } from '../ui/Field'
import { LinkRow } from '../ui/List'
import { Screen } from '../ui/Screen'

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


  // 絞り込みが1つでも掛かっているか。掛かっていない時に「条件を見直せ」と
  // 出すと、そもそも登録が無いだけの人を条件探しに向かわせることになる。
  const filtered =
    query !== '' || category !== '' || location !== '' || condition !== '' || locationStatus !== ''

  return (
    <Screen title="備品一覧" back={{ to: '/', label: 'トップ' }}>
      <form className="mt-4" onSubmit={handleSearch} role="search">
        <label className="sr-only" htmlFor="q">
          品名・備品コード・型番で検索
        </label>

        {/* iOS の検索欄。枠線ではなく淡い地で表す。虫眼鏡は装飾ではなく、
            「ここに打てば探せる」ことを文字より早く伝える。 */}
        <div className="flex items-center gap-2 rounded-[10px] bg-fill px-2.5">
          <SearchIcon />
          <input
            id="q"
            // type="search" にすると、スマートフォンのキーボードが
            // 改行ではなく「検索」になる。
            type="search"
            placeholder="品名・備品コード・型番"
            autoCapitalize="none"
            autoCorrect="off"
            spellCheck={false}
            className="min-w-0 flex-1 bg-transparent py-2.5 text-[17px] placeholder:text-label-3 focus:outline-none"
            value={queryInput}
            onChange={(e) => setQueryInput(e.target.value)}
          />
          {/* 送信ボタンは出したままにする。キーボードを閉じてから探し直す
              人がいるため、Enter だけに頼らない。 */}
          <button type="submit" className="shrink-0 py-2 pl-1 text-[17px] text-tint">
            検索
          </button>
        </div>
      </form>

      <FieldGroup header="絞り込み">
        <SelectField
          id="category"
          label="分類"
          value={category}
          onChange={(e) => update('category', e.target.value)}
        >
          <FilterOptionList options={options.categories} />
        </SelectField>
        <SelectField
          id="location"
          label="保管場所"
          value={location}
          onChange={(e) => update('location', e.target.value)}
        >
          <FilterOptionList options={options.locations} />
        </SelectField>
        <SelectField
          id="condition"
          label="状態"
          value={condition}
          onChange={(e) => update('condition', e.target.value)}
        >
          <FilterOptionList options={CONDITIONS} />
        </SelectField>
        <SelectField
          id="location_status"
          label="所在"
          value={locationStatus}
          onChange={(e) => update('location_status', e.target.value)}
        >
          <FilterOptionList options={LOCATION_STATUSES} />
        </SelectField>
      </FieldGroup>

      {error !== '' && <Alert>{error}</Alert>}

      {error === '' && items === null && <Loading />}

      {items !== null && (
        <>
          {/* 件数は0でも出す。__「探した結果0件」と「まだ読み込んでいない」は__
              __別のことで、区別が付かないと通信を疑うことになる。__ */}
          <p className="mt-6 px-4 text-[13px] text-label-2">{items.length}件</p>

          {items.length === 0 ? (
            <Empty
              title="該当する備品がありません"
              hint={
                <>
                  {filtered && <>絞り込みを外すと見つかるかもしれません。</>}
                  廃棄済みは既定で除いています。状態で「廃棄」を選ぶと表示されます。
                </>
              }
            />
          ) : (
            <ul className="mt-2 overflow-hidden rounded-group bg-card">
              {items.map((it) => (
                <ItemRow key={it.id} item={it} />
              ))}
            </ul>
          )}
        </>
      )}
    </Screen>
  )
}

/** FilterOptionList は絞り込みの選択肢。先頭の空が「すべて」。 */
function FilterOptionList({ options }: { options: readonly string[] }) {
  return (
    <>
      <option value="">すべて</option>
      {options.map((opt) => (
        <option key={opt} value={opt}>
          {opt}
        </option>
      ))}
    </>
  )
}

/**
 * ItemRow は1件分の表示。
 *
 * 行全体をリンクにする。指で押す的が小さいと、スマートフォンでは
 * 隣の行を開くことになる。
 */
function ItemRow({ item }: { item: Item }) {
  const detail = [item.category, item.model, item.location].filter((v) => v !== '').join('・')

  return (
    <LinkRow to={`/i/${item.code}`}>
      <div className="flex items-center gap-2">
        <span className="truncate text-[17px]">{item.name}</span>

        {/* 良好・在庫は出さない。全ての行に付くと、注意すべき行が埋もれる。 */}
        {item.condition !== '良好' && <Badge tone="warn">{item.condition}</Badge>}
        {item.location_status !== '在庫' && <Badge tone="warn">{item.location_status}</Badge>}
        {item.is_free_use && <Badge tone="info">自由利用品</Badge>}
      </div>

      <p className="mt-0.5 truncate text-[13px] text-label-2">
        {/* 備品コードは等幅で出す。棚に貼ったラベルと見比べるため。 */}
        <span className="font-mono tabular-nums">{item.code}</span>
        {detail !== '' && (
          <>
            {' · '}
            <span>{detail}</span>
          </>
        )}
      </p>
    </LinkRow>
  )
}

/** SearchIcon は検索欄の虫眼鏡。 */
function SearchIcon() {
  return (
    <svg
      viewBox="0 0 16 16"
      className="h-4 w-4 shrink-0 text-label-3"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      aria-hidden="true"
    >
      <circle cx="7" cy="7" r="5" />
      <path d="M11 11l4 4" />
    </svg>
  )
}
