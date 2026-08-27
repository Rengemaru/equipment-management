import { useRef, useState } from 'react'
import type { ChangeEvent, FormEvent } from 'react'

import { errorMessage } from '../api/client'
import { createItem, uploadPhoto } from '../api/items'
import type { Item, ItemAttributes } from '../api/types'
import { Button, ButtonLink } from '../ui/Button'
import { Alert, Notice } from '../ui/Feedback'
import { FieldGroup } from '../ui/Field'
import { ItemFields, emptyAttributes } from '../ui/ItemFields'
import { Card, LinkRow, List } from '../ui/List'
import { Screen } from '../ui/Screen'

/**
 * AdminItemNew は備品の登録フォーム（運営のみ）。
 *
 * **備品コードは入力させない。** システムが採番する。人手で振らせると
 * 抜け・重複が必ず起きる（CLAUDE.md）。採番された値は登録後に表示する。
 * ラベルを刷るのに要るため、__見せずに次へ進ませない。__
 */
export default function AdminItemNew() {
  const [attrs, setAttrs] = useState<ItemAttributes>(emptyAttributes())
  const [photo, setPhoto] = useState<File | null>(null)
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)

  /** created は登録できた備品。ここに値が入ると結果の表示に切り替わる。 */
  const [created, setCreated] = useState<Item | null>(null)

  /**
   * photoError は「備品は登録できたが写真だけ失敗した」場合の理由。
   *
   * 登録自体をやり直させない。もう一度送ると同じ備品が2件でき、
   * 採番が1つ無駄になる（番号は再利用しない）。
   */
  const [photoError, setPhotoError] = useState('')

  // input[type=file] は値を props で制御できない。やり直しの時に
  // 選択済みの表示を消すため、要素そのものを触る必要がある。
  const fileRef = useRef<HTMLInputElement>(null)

  const attachPhoto = async (item: Item, file: File) => {
    try {
      setCreated(await uploadPhoto(item.code, file))
      setPhotoError('')
    } catch (err) {
      // 備品は登録済み。写真だけ後から送り直せるようにする。
      setCreated(item)
      setPhotoError(errorMessage(err))
    }
  }

  const handleSubmit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    setError('')
    setSaving(true)

    try {
      const item = await createItem(attrs)

      if (photo === null) {
        setCreated(item)
      } else {
        // 写真は備品が登録された後でないと送れない。順序は入れ替えられない。
        await attachPhoto(item, photo)
      }
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setSaving(false)
    }
  }

  const startNext = () => {
    setCreated(null)
    setPhotoError('')
    setPhoto(null)
    setError('')
    // 分類・保管場所・所有は続けて登録する時にそのまま使えることが多い。
    // 品名と型番と備考だけ空にする。棚1つ分をまとめて登録する使い方に合わせる。
    setAttrs((prev) => ({ ...prev, name: '', model: '', note: '' }))
    if (fileRef.current !== null) fileRef.current.value = ''
  }

  if (created !== null) {
    return (
      <Screen title="登録しました" back={{ to: '/admin/items', label: 'マスタ管理' }}>
        {/* ラベルを刷る時に要る番号。__この応答でしか手に入らないので__
            __目立つ形で出す。__ */}
        <Card header="備品コード">
          <div className="px-4 py-5 text-center">
            <p className="font-mono text-[44px] leading-none font-semibold tabular-nums">
              {created.code}
            </p>
            <p className="mt-2 text-[15px] text-label-2">{created.name}</p>
          </div>
        </Card>

        {photoError !== '' && (
          <>
            <Notice tone="warn">
              <span role="alert">
                備品は登録されましたが、写真の添付に失敗しました: {photoError}
              </span>
            </Notice>

            {/* __登録はやり直させない。__ 送り直すと同じ備品が2件でき、
                採番が1つ無駄になる（番号は再利用しない）。 */}
            {photo !== null && (
              <div className="mt-3">
                <Button tone="tinted" full onClick={() => void attachPhoto(created, photo)}>
                  写真を送り直す
                </Button>
              </div>
            )}
          </>
        )}

        {created.photo_url !== '' && (
          <img
            className="mt-6 w-full rounded-group"
            src={created.photo_url}
            alt={`${created.name}の写真`}
          />
        )}

        <div className="mt-6">
          <Button full onClick={startNext}>
            続けて登録する
          </Button>
        </div>

        <List>
          <LinkRow to={`/i/${created.code}`}>
            <span className="text-[17px]">この備品を見る</span>
          </LinkRow>
          <LinkRow to="/admin/items">
            <span className="text-[17px]">マスタ管理へ</span>
          </LinkRow>
        </List>
      </Screen>
    )
  }

  return (
    <Screen title="備品を登録" back={{ to: '/admin/items', label: 'マスタ管理' }}>
      <p className="mt-1 px-4 text-[13px] leading-snug text-label-2">
        備品コードは登録時に自動で採番されます。入力は要りません。
      </p>

      <form onSubmit={(e) => void handleSubmit(e)}>
        <ItemFields attrs={attrs} onChange={setAttrs} />

        <FieldGroup header="写真" footer="JPEG か PNG。10MBまで。">
          <div className="px-4 py-3">
            <label className="block text-[13px] text-label-2" htmlFor="photo">
              写真
            </label>
            <input
              id="photo"
              ref={fileRef}
              type="file"
              // スマートフォンでその場で撮れるようにする。棚の前で登録する
              // 使い方を想定している。撮影に限定はしない（既存の画像も選べる）。
              accept="image/jpeg,image/png"
              capture="environment"
              className="mt-1.5 w-full text-[15px] file:mr-3 file:rounded-full file:border-0 file:bg-fill file:px-3 file:py-1.5 file:text-[15px] file:text-tint"
              onChange={(e: ChangeEvent<HTMLInputElement>) => setPhoto(e.target.files?.[0] ?? null)}
            />
          </div>
        </FieldGroup>

        {error !== '' && <Alert>{error}</Alert>}

        <div className="mt-6 space-y-3">
          <Button
            type="submit"
            full
            // 二重送信を止める。押し直すと同じ備品が2件でき、
            // 採番が1つ無駄になる（番号は再利用しない）。
            disabled={saving}
          >
            {saving ? '登録しています…' : '登録する'}
          </Button>
          <ButtonLink to="/admin/items" tone="plain" full>
            やめる
          </ButtonLink>
        </div>
      </form>
    </Screen>
  )
}
