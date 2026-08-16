import { CONDITIONS, LOCATION_STATUSES, OWNERS } from '../api/types'
import type { ItemAttributes } from '../api/types'

/**
 * ItemFields は備品の入力欄一式。
 *
 * 登録（`/admin/items/new`）と編集（`/admin/items`）で同じものを使う。
 * 別々に書くと、項目を足した時に片方だけ直され、__経路によって入る値が__
 * __変わる__ ことになる。サーバは全項目を受け取る形なので、送り漏れは
 * 空欄として保存されてしまい、気付きにくい。
 *
 * idPrefix は同じ画面に複数出す場合に渡す。label と input の対応が
 * 壊れると、読み上げでも指での操作でも狙った欄に入らない。
 */
export function ItemFields({
  attrs,
  onChange,
  idPrefix = '',
}: {
  attrs: ItemAttributes
  onChange: (attrs: ItemAttributes) => void
  idPrefix?: string
}) {
  const set = <K extends keyof ItemAttributes>(key: K, value: ItemAttributes[K]) => {
    onChange({ ...attrs, [key]: value })
  }

  const id = (name: string) => (idPrefix === '' ? name : `${idPrefix}-${name}`)

  return (
    <>
      <TextField
        id={id('name')}
        label="品名"
        value={attrs.name}
        onChange={(v) => set('name', v)}
        required
      />
      <TextField
        id={id('category')}
        label="分類"
        value={attrs.category}
        onChange={(v) => set('category', v)}
        hint="空欄なら「未分類」になります"
      />
      <TextField
        id={id('model')}
        label="型番"
        value={attrs.model}
        onChange={(v) => set('model', v)}
      />
      <TextField
        id={id('location')}
        label="保管場所"
        value={attrs.location}
        onChange={(v) => set('location', v)}
      />

      <SelectField
        id={id('owner')}
        label="所有"
        value={attrs.owner}
        options={OWNERS}
        onChange={(v) => set('owner', v as ItemAttributes['owner'])}
      />
      {/* 廃棄も状態の1つ。削除ではないため、ここで指定する。 */}
      <SelectField
        id={id('condition')}
        label="状態"
        value={attrs.condition}
        options={CONDITIONS}
        onChange={(v) => set('condition', v as ItemAttributes['condition'])}
      />
      <SelectField
        id={id('location_status')}
        label="所在"
        value={attrs.location_status}
        options={LOCATION_STATUSES}
        onChange={(v) => set('location_status', v as ItemAttributes['location_status'])}
      />

      {/* 自由利用品は貸出フローから完全に除外される。追跡対象を減らすことが
          遵守率を上げる最短経路（CLAUDE.md）。何が起きるかを書き添える。 */}
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
        <label className="block text-sm font-medium" htmlFor={id('note')}>
          備考
        </label>
        <textarea
          id={id('note')}
          rows={2}
          className="mt-1 w-full rounded border border-gray-300 px-3 py-2 text-base"
          value={attrs.note}
          onChange={(e) => set('note', e.target.value)}
        />
      </div>
    </>
  )
}

/** emptyAttributes は登録フォームの初期値。既定値はサーバと揃える。 */
export function emptyAttributes(): ItemAttributes {
  return {
    name: '',
    category: '',
    model: '',
    owner: 'サークル',
    is_free_use: false,
    location: '',
    condition: '良好',
    location_status: '在庫',
    note: '',
  }
}

export function TextField({
  id,
  label,
  value,
  onChange,
  required,
  hint,
}: {
  id: string
  label: string
  value: string
  onChange: (value: string) => void
  required?: boolean
  hint?: string
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
      {hint !== undefined && <p className="mt-1 text-xs text-gray-600">{hint}</p>}
    </div>
  )
}

export function SelectField({
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
