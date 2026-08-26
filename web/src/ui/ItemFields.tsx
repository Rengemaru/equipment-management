import { CONDITIONS, LOCATION_STATUSES, OWNERS } from '../api/types'
import type { ItemAttributes } from '../api/types'
import { Field, FieldGroup, SelectField, StackedField, SwitchField } from './Field'

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
      <FieldGroup footer="分類が空欄なら「未分類」になります。">
        <Field
          id={id('name')}
          label="品名"
          required
          value={attrs.name}
          onChange={(e) => set('name', e.target.value)}
        />
        <Field
          id={id('category')}
          label="分類"
          value={attrs.category}
          onChange={(e) => set('category', e.target.value)}
        />
        <Field
          id={id('model')}
          label="型番"
          value={attrs.model}
          onChange={(e) => set('model', e.target.value)}
        />
        <Field
          id={id('location')}
          label="保管場所"
          value={attrs.location}
          onChange={(e) => set('location', e.target.value)}
        />
      </FieldGroup>

      <FieldGroup>
        <SelectField
          id={id('owner')}
          label="所有"
          value={attrs.owner}
          onChange={(e) => set('owner', e.target.value as ItemAttributes['owner'])}
        >
          <Options options={OWNERS} />
        </SelectField>

        {/* 廃棄も状態の1つ。削除ではないため、ここで指定する。 */}
        <SelectField
          id={id('condition')}
          label="状態"
          value={attrs.condition}
          onChange={(e) => set('condition', e.target.value as ItemAttributes['condition'])}
        >
          <Options options={CONDITIONS} />
        </SelectField>

        <SelectField
          id={id('location_status')}
          label="所在"
          value={attrs.location_status}
          onChange={(e) =>
            set('location_status', e.target.value as ItemAttributes['location_status'])
          }
        >
          <Options options={LOCATION_STATUSES} />
        </SelectField>
      </FieldGroup>

      {/* 自由利用品は貸出フローから完全に除外される。追跡対象を減らすことが
          遵守率を上げる最短経路（CLAUDE.md）。何が起きるかを書き添える。 */}
      <FieldGroup footer="自由利用品にすると、貸出の記録を求めなくなります。">
        <SwitchField
          id={id('is_free_use')}
          label="自由利用品にする（貸出の記録を求めない）"
          checked={attrs.is_free_use}
          onChange={(v) => set('is_free_use', v)}
        />
      </FieldGroup>

      <FieldGroup>
        <StackedField
          id={id('note')}
          label="備考"
          multiline
          value={attrs.note}
          onChange={(e) => set('note', e.target.value)}
        />
      </FieldGroup>
    </>
  )
}

/** Options は選択肢を並べる。 */
function Options({ options }: { options: readonly string[] }) {
  return (
    <>
      {options.map((opt) => (
        <option key={opt} value={opt}>
          {opt}
        </option>
      ))}
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
