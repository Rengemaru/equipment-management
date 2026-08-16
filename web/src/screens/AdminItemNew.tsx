import { useRef, useState } from 'react'
import type { ChangeEvent, FormEvent } from 'react'
import { Link } from 'react-router'

import { errorMessage } from '../api/client'
import { createItem, uploadPhoto } from '../api/items'
import type { Item, ItemAttributes } from '../api/types'
import { ItemFields, emptyAttributes } from '../ui/ItemFields'

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
      <main className="mx-auto max-w-screen-sm p-4">
        <h1 className="text-xl font-bold">登録しました</h1>

        <p className="mt-4 text-sm text-gray-600">備品コード</p>
        {/* ラベルを刷る時に要る番号。目立つ形で出す。 */}
        <p className="font-mono text-3xl">{created.code}</p>
        <p className="mt-1">{created.name}</p>

        {photoError !== '' && (
          <div className="mt-4 rounded bg-amber-50 p-3">
            <p role="alert" className="text-sm text-amber-900">
              備品は登録されましたが、写真の添付に失敗しました: {photoError}
            </p>
            {photo !== null && (
              <button
                className="mt-2 rounded border border-amber-300 px-3 py-1 text-sm"
                onClick={() => void attachPhoto(created, photo)}
              >
                写真を送り直す
              </button>
            )}
          </div>
        )}

        {created.photo_url !== '' && (
          <img className="mt-4 w-full rounded" src={created.photo_url} alt={`${created.name}の写真`} />
        )}

        <div className="mt-6 flex flex-wrap gap-3">
          <button className="rounded bg-blue-700 px-4 py-2 text-white" onClick={startNext}>
            続けて登録する
          </button>
          <Link className="self-center text-blue-700 underline" to={`/i/${created.code}`}>
            この備品を見る
          </Link>
          <Link className="self-center text-blue-700 underline" to="/admin/items">
            マスタ管理へ
          </Link>
        </div>
      </main>
    )
  }

  return (
    <main className="mx-auto max-w-screen-sm p-4">
      <h1 className="text-xl font-bold">備品を登録</h1>
      <p className="mt-1 text-sm text-gray-600">
        備品コードは登録時に自動で採番されます。入力は要りません。
      </p>

      <form className="mt-4 space-y-3" onSubmit={(e) => void handleSubmit(e)}>
        <ItemFields attrs={attrs} onChange={setAttrs} />

        <div>
          <label className="block text-sm font-medium" htmlFor="photo">
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
            className="mt-1 w-full text-sm"
            onChange={(e: ChangeEvent<HTMLInputElement>) =>
              setPhoto(e.target.files?.[0] ?? null)
            }
          />
          <p className="mt-1 text-xs text-gray-600">JPEG か PNG。10MBまで。</p>
        </div>

        {error !== '' && (
          <p role="alert" className="text-sm text-red-700">
            {error}
          </p>
        )}

        <div className="flex gap-2">
          <button
            type="submit"
            // 二重送信を止める。押し直すと同じ備品が2件でき、
            // 採番が1つ無駄になる（番号は再利用しない）。
            disabled={saving}
            className="rounded bg-blue-700 px-4 py-3 text-white disabled:bg-gray-400"
          >
            {saving ? '登録しています…' : '登録する'}
          </button>
          <Link className="self-center px-2 text-blue-700 underline" to="/admin/items">
            やめる
          </Link>
        </div>
      </form>
    </main>
  )
}
