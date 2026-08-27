import { useRef, useState } from 'react'
import type { ChangeEvent } from 'react'

import { errorMessage } from '../api/client'
import { deletePhoto, uploadPhoto } from '../api/items'
import type { Item } from '../api/types'
import { Button } from './Button'
import { FieldGroup } from './Field'

/**
 * PhotoField は登録済みの備品の写真を差し替え・削除する。
 *
 * __備品の他の項目とは送り方が違う。__ 他は「全項目をまとめて保存」だが、
 * 写真は選んだ時点で送る。multipart と JSON を1回のリクエストに混ぜず、
 * サーバの経路も分かれているため（`POST/DELETE /api/items/{code}/photo`）。
 *
 * # なぜ編集画面に要るのか
 *
 * 登録フォームでしか写真を送れないと、__CSVで一括登録した備品には__
 * __一生写真を付けられない。__ M1 の主な入り口は一括登録なので、
 * 実際にはほとんどの備品が写真を持てないことになる。
 *
 * 保存ボタンとは独立して効く。押した時点で反映されるので、
 * 「保存を押さずに閉じたら写真だけ残った」という食い違いは起きない。
 */
export function PhotoField({
  item,
  onChanged,
}: {
  item: Item
  /** onChanged は差し替え・削除の後のItemを受け取る。一覧の表示を合わせるため。 */
  onChanged: (updated: Item) => void
}) {
  const fileRef = useRef<HTMLInputElement>(null)

  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [confirmingDelete, setConfirmingDelete] = useState(false)

  const id = `${item.code}-photo`
  const hasPhoto = item.photo_url !== ''

  const run = async (action: () => Promise<Item>) => {
    setError('')
    setBusy(true)
    try {
      onChanged(await action())
      setConfirmingDelete(false)
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusy(false)
      // 同じファイルを選び直せるようにする。value を残すと、
      // 一度失敗したファイルをもう一度選んでも change が起きない。
      if (fileRef.current !== null) fileRef.current.value = ''
    }
  }

  const choose = (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (file === undefined) return
    void run(() => uploadPhoto(item.code, file))
  }

  return (
    <FieldGroup header="写真" footer="JPEG か PNG。10MBまで。選んだ時点で反映されます。">
      {hasPhoto && (
        <div className="px-4 pt-3">
          <img
            className="max-h-56 w-full rounded-lg object-contain"
            src={item.photo_url}
            alt={`${item.name}の写真`}
            loading="lazy"
          />
        </div>
      )}

      <div className="px-4 py-3">
        <label className="block text-[13px] text-label-2" htmlFor={id}>
          {hasPhoto ? '写真を差し替える' : '写真を追加する'}
        </label>
        <input
          id={id}
          ref={fileRef}
          type="file"
          // 棚の前でその場で撮れるようにする。撮影に限定はしない。
          accept="image/jpeg,image/png"
          capture="environment"
          disabled={busy}
          className="mt-1.5 w-full text-[15px] file:mr-3 file:rounded-full file:border-0 file:bg-fill file:px-3 file:py-1.5 file:text-[15px] file:text-tint"
          onChange={choose}
        />
      </div>

      {hasPhoto &&
        (confirmingDelete ? (
          <div className="flex gap-2 px-4 pb-3">
            <Button
              type="button"
              tone="danger"
              disabled={busy}
              onClick={() => void run(() => deletePhoto(item.code))}
            >
              {busy ? '外しています…' : '本当に外す'}
            </Button>
            <Button
              type="button"
              tone="plain"
              disabled={busy}
              onClick={() => setConfirmingDelete(false)}
            >
              やめる
            </Button>
          </div>
        ) : (
          <div className="px-4 pb-3">
            {/* 消したファイルは戻らない。1タップの確認を挟む。 */}
            <Button
              type="button"
              tone="plain"
              disabled={busy}
              onClick={() => setConfirmingDelete(true)}
            >
              写真を外す
            </Button>
          </div>
        ))}

      {/* カードの中なので Alert（それ自体がカード）は使わない。
          入れ子にすると、どこで起きた失敗なのかが見えなくなる。 */}
      {error !== '' && (
        <p role="alert" className="px-4 pb-3 text-[15px] leading-snug text-danger">
          {error}
        </p>
      )}
    </FieldGroup>
  )
}
