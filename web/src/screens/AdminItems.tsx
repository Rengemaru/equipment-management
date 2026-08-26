import { useCallback, useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { Link, useSearchParams } from 'react-router'

import { errorMessage } from '../api/client'
import { listItems, updateItem } from '../api/items'
import type { Item, ItemAttributes } from '../api/types'
import { Button } from '../ui/Button'
import { Alert, Badge, Loading } from '../ui/Feedback'
import { FieldGroup, SwitchField } from '../ui/Field'
import { ItemFields } from '../ui/ItemFields'
import { LinkRow, List } from '../ui/List'
import { Screen } from '../ui/Screen'

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
    <Screen title="備品マスタ管理" back={{ to: '/', label: 'トップ' }}>
      <List>
        <LinkRow to="/admin/items/new">
          <span className="text-[17px]">備品を登録</span>
        </LinkRow>
        <LinkRow to="/admin/items/import">
          <span className="text-[17px]">CSVで一括登録</span>
        </LinkRow>
        <LinkRow to="/admin/labels">
          <span className="text-[17px]">QRラベルの印刷</span>
        </LinkRow>
      </List>

      <form
        className="mt-6"
        role="search"
        onSubmit={(e: FormEvent<HTMLFormElement>) => {
          e.preventDefault()
          update('q', queryInput)
        }}
      >
        <label className="sr-only" htmlFor="q">
          品名・備品コード・型番で検索
        </label>
        <div className="flex items-center gap-2 rounded-[10px] bg-fill px-2.5">
          <input
            id="q"
            type="search"
            placeholder="品名・備品コード・型番"
            autoCapitalize="none"
            autoCorrect="off"
            spellCheck={false}
            className="min-w-0 flex-1 bg-transparent py-2.5 text-[17px] placeholder:text-label-3 focus:outline-none"
            value={queryInput}
            onChange={(e) => setQueryInput(e.target.value)}
          />
          <button type="submit" className="shrink-0 py-2 pl-1 text-[17px] text-tint">
            検索
          </button>
        </div>
      </form>

      <FieldGroup>
        <SwitchField
          id="include-discarded"
          label="廃棄済みも表示する"
          checked={includeDiscarded}
          onChange={(v) => update('include_discarded', v ? '1' : '')}
        />
      </FieldGroup>

      {error !== '' && <Alert>{error}</Alert>}

      {error === '' && items === null && <Loading />}

      {items !== null && (
        <>
          <p className="mt-6 px-4 text-[13px] text-label-2">{items.length}件</p>

          <ul className="mt-2 space-y-3">
            {items.map((it) => (
              <li key={it.id} className="overflow-hidden rounded-group bg-card">
                <div className="px-4 py-3">
                  <div className="flex items-center gap-2">
                    <span className="truncate text-[17px]">{it.name}</span>
                    {it.condition !== '良好' && <Badge tone="warn">{it.condition}</Badge>}
                    {it.location_status !== '在庫' && (
                      <Badge tone="warn">{it.location_status}</Badge>
                    )}
                    {it.is_free_use && <Badge tone="info">自由利用品</Badge>}
                  </div>

                  <p className="mt-0.5 truncate text-[13px] text-label-2">
                    <span className="font-mono tabular-nums">{it.code}</span>
                    {[it.category, it.model, it.location].filter((v) => v !== '').length > 0 && (
                      <>
                        {' · '}
                        <span>
                          {[it.category, it.model, it.location].filter((v) => v !== '').join('・')}
                        </span>
                      </>
                    )}
                  </p>

                  {saved === it.code && editing !== it.code && (
                    <p role="status" className="mt-1.5 text-[13px] text-tint">
                      保存しました
                    </p>
                  )}
                </div>

                {editing === it.code ? (
                  <EditForm item={it} onSaved={replaceItem} onCancel={() => setEditing('')} />
                ) : (
                  // __削除の導線は置かない。__ 廃棄は状態で、行を消すと
                  // 貸出履歴の参照先が消える（CLAUDE.md）。
                  <div className="flex border-t border-separator">
                    <button
                      className="min-h-11 flex-1 text-[17px] text-tint active:bg-fill"
                      onClick={() => {
                        setSaved('')
                        setEditing(it.code)
                      }}
                    >
                      編集
                    </button>
                    <Link
                      className="flex min-h-11 flex-1 items-center justify-center border-l border-separator text-[17px] text-tint active:bg-fill"
                      to={`/i/${it.code}`}
                    >
                      詳細
                    </Link>
                  </div>
                )}
              </li>
            ))}
          </ul>
        </>
      )}
    </Screen>
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
    // 一覧の地の色に戻す。カードの中でさらにカードを重ねる形になるため、
    // 同じ白のままだと入力欄のまとまりが見えなくなる。
    <form className="bg-bg px-3 pt-1 pb-4" onSubmit={(e) => void handleSubmit(e)}>
      {/* 入力欄は登録フォームと同じものを使う。別々に書くと、項目を足した時に
          片方だけ直され、経路によって入る値が変わる。 */}
      <ItemFields attrs={attrs} onChange={setAttrs} idPrefix={item.code} />

      {error !== '' && <Alert>{error}</Alert>}

      <div className="mt-6 space-y-3">
        <Button type="submit" full disabled={saving}>
          {saving ? '保存しています…' : '保存'}
        </Button>
        <Button type="button" tone="plain" full onClick={onCancel}>
          キャンセル
        </Button>
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
