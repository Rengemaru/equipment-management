import { useCallback, useEffect, useState } from 'react'
import { useParams } from 'react-router'

import { ApiError, errorMessage } from '../api/client'
import { reportDamage } from '../api/damages'
import { getItem } from '../api/items'
import { returnItem } from '../api/loans'
import type { Item } from '../api/types'
import { useAuth } from '../auth/AuthProvider'
import { formatDate, formatDateTime } from '../lib/date'
import { Button, ButtonLink } from '../ui/Button'
import { StackedField } from '../ui/Field'
import { Alert, Empty, Loading, Notice } from '../ui/Feedback'
import { Card, List, Row } from '../ui/List'
import { Screen } from '../ui/Screen'

/**
 * ItemDetail は備品の詳細画面。QRの遷移先（`/i/{code}`）。
 *
 * __この経路は変えられない。__ ラベルは貼り替えられないため、URLを変えると
 * 印刷済みのQRが全て読めなくなる（url-design.md §1）。
 *
 * 未ログインで来た場合は RequireAuth がログイン画面へ送り、認証後に
 * ここへ戻す。戻せないともう一度QRを読み直させることになり、
 * その一手間が記録漏れの直接原因になる。
 *
 * # 状態による分岐（m2-spec §3）
 *
 * | 状態 | 出すもの |
 * |---|---|
 * | 貸出可能 | 「借りる」（主要導線として大きく） |
 * | 貸出中（他人） | 借用者名・返却予定日 |
 * | 貸出中（自分） | 「返す」 |
 * | 自由利用品 | 情報のみ。__貸出ボタンを出さない__ |
 * | 廃棄 | 貸出不可を明示 |
 * | 所在不明 | 「借りる」は出すが警告を添える |
 */
export default function ItemDetail() {
  const { code = '' } = useParams()
  const auth = useAuth()
  const meID = auth.status === 'authenticated' ? auth.user.id : 0

  const [item, setItem] = useState<Item | null>(null)
  const [error, setError] = useState('')
  const [notFound, setNotFound] = useState(false)

  const load = useCallback(() => {
    setError('')
    setNotFound(false)

    return getItem(code).then(
      (it) => setItem(it),
      (err: unknown) => {
        // 登録が無い場合は、原因の心当たりまで出す。QRを読んで来た人が
        // 最初に見る画面で、「エラー」とだけ出しても次の手が分からない。
        if (err instanceof ApiError && err.status === 404) {
          setNotFound(true)
          return
        }
        setError(errorMessage(err))
      },
    )
  }, [code])

  useEffect(() => {
    setItem(null)
    void load()
  }, [load])

  // 戻り先は常に備品一覧にする。QRから直接来た人には履歴が無く、
  // ブラウザの戻るが効かない。行き先を示さないと、そこで止まってしまう。
  const back = { to: '/items', label: '備品一覧' }

  if (notFound) {
    return (
      <Screen title="登録されていない備品コードです" subtitle={code} back={back}>
        <Empty
          title="この備品は登録されていません"
          hint="ラベルの読み取りに失敗したか、まだ登録されていない可能性があります。"
        />
      </Screen>
    )
  }

  if (error !== '' && item === null) {
    return (
      <Screen title="備品" subtitle={code} back={back}>
        <Alert>{error}</Alert>
      </Screen>
    )
  }

  if (item === null) {
    return (
      <Screen title="読み込み中…" subtitle={code} back={back}>
        <Loading />
      </Screen>
    )
  }

  const loan = item.loan ?? null
  const discarded = item.condition === '廃棄'
  const mine = loan !== null && loan.user.id === meID

  return (
    <Screen title={item.name} subtitle={item.code} back={back}>
      {error !== '' && <Alert>{error}</Alert>}

      {/* 廃棄は物理削除の代わり。ラベルは貼られたままなので、
          読んだ人に「これはもう使わないもの」と分かる形で出す。 */}
      {discarded && <Notice>この備品は廃棄されています。借りられません。</Notice>}

      {item.location_status !== '在庫' && (
        <Notice tone="warn">
          所在: {item.location_status}
          <span className="mt-1 block text-[13px]">
            手元にあるなら、借りる操作をすると在庫に戻ります。
          </span>
        </Notice>
      )}

      {/* 自由利用品は貸出フローの対象外。追跡対象を減らすことが
          遵守率を上げる最短経路（CLAUDE.md）。 */}
      {item.is_free_use && <Notice>自由利用品です。借用の記録は要りません。</Notice>}

      {loan !== null && (
        <Card header={mine ? 'あなたが借りています' : '貸出中'}>
          <div className="px-4 py-3 text-[17px]">
            <p>{mine ? 'あなた' : `${loan.user.name} さん`}</p>
            <p className="mt-1 text-[15px] text-label-2">
              {formatDateTime(loan.borrowed_at)} から / 返却予定 {formatDate(loan.due_date)}
              {loan.overdue_days > 0 && (
                <span className="ml-2 text-danger">{loan.overdue_days}日超過</span>
              )}
            </p>
          </div>
        </Card>
      )}

      <Actions item={item} mine={mine} onDone={setItem} onError={setError} reload={load} />

      {item.photo_url !== '' && (
        <img
          // 広い画面で高さを抑える。__備品を見分けるための写真であって、__
          // __主役ではない。__ 抑えないと、下の項目が画面外へ押し出される。
          className="mt-6 max-h-[70vh] w-full rounded-group object-contain md:max-h-96"
          src={item.photo_url}
          alt={`${item.name}の写真`}
          loading="lazy"
        />
      )}

      <List>
        <Row label="分類" value={item.category} />
        <Row label="型番" value={item.model} />
        <Row label="所有" value={item.owner} />
        <Row label="保管場所" value={item.location} />
        <Row label="状態" value={item.condition} />
      </List>

      {/* 備考は長くなる。右寄せの Row に入れると折り返しが読みにくい。 */}
      <Card header="備考">
        <p className="px-4 py-3 text-[17px] leading-relaxed whitespace-pre-wrap">
          {item.note === '' ? <span className="text-label-3">—</span> : item.note}
        </p>
      </Card>

      <p className="mt-6 px-4 text-[13px] text-label-2">最終更新: {item.updated_at}</p>
    </Screen>
  )
}

/**
 * Actions は状態に応じた操作。
 *
 * __押せないボタンを出さない。__ 借りられない備品に「借りる」を出して
 * 押した先で断ると、記録する気を削ぐだけになる。
 */
function Actions({
  item,
  mine,
  onDone,
  onError,
  reload,
}: {
  item: Item
  mine: boolean
  onDone: (it: Item) => void
  onError: (msg: string) => void
  reload: () => Promise<void>
}) {
  const [busy, setBusy] = useState(false)

  if (item.condition === '廃棄') {
    // 廃棄済みでも返却はできる（持ち出したまま廃棄になった分を戻せないと、
    // 貸出中の行が永久に残る）。
    return mine ? (
      <ReturnButton code={item.code} busy={busy} setBusy={setBusy} onDone={onDone} onError={onError} reload={reload} />
    ) : null
  }

  if (item.is_free_use) {
    // 自由利用品には貸出ボタンを出さない。APIも400で拒否する。
    return <DamageReport code={item.code} onDone={onDone} onError={onError} />
  }

  const loan = item.loan ?? null

  return (
    <>
      <div className="mt-6 px-4">
        {loan === null ? (
          // 主要導線。QRを読んだ人がここを押して、次の画面で確定する（2タップ）。
          <ButtonLink to={`/i/${encodeURIComponent(item.code)}/borrow`} full>
            借りる
          </ButtonLink>
        ) : mine ? (
          <ReturnButton
            code={item.code}
            busy={busy}
            setBusy={setBusy}
            onDone={onDone}
            onError={onError}
            reload={reload}
          />
        ) : null}
      </div>

      <DamageReport code={item.code} onDone={onDone} onError={onError} />
    </>
  )
}

/** ReturnButton は返却。確認を挟まない（誤返却は再度借りれば済む）。 */
function ReturnButton({
  code,
  busy,
  setBusy,
  onDone,
  onError,
  reload,
}: {
  code: string
  busy: boolean
  setBusy: (v: boolean) => void
  onDone: (it: Item) => void
  onError: (msg: string) => void
  reload: () => Promise<void>
}) {
  async function run() {
    setBusy(true)
    onError('')
    try {
      const res = await returnItem(code)
      onDone(res.item)
    } catch (err) {
      // 409 は「他の人が先に返した」か二重タップ。異常ではないので、
      // 文言を出したうえで最新の状態に読み直す。
      if (err instanceof ApiError && err.status === 409) {
        // 読み直しが先。load が error を消すため、逆にすると文言が出ない。
        await reload()
        onError('すでに返却されています。')
      } else {
        onError(errorMessage(err))
      }
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="mt-6 px-4">
      <Button full disabled={busy} onClick={() => void run()}>
        {busy ? '返しています…' : '返す'}
      </Button>
    </div>
  )
}

/**
 * DamageReport は破損報告。
 *
 * __承認を待たない。__ 報告した時点で備品が要修理になる。
 * 押した結果が画面に出ないボタンは、効かないボタンでしかない。
 */
function DamageReport({
  code,
  onDone,
  onError,
}: {
  code: string
  onDone: (it: Item) => void
  onError: (msg: string) => void
}) {
  const [open, setOpen] = useState(false)
  const [text, setText] = useState('')
  const [busy, setBusy] = useState(false)
  const [done, setDone] = useState(false)

  if (done) {
    return <Notice tone="warn">破損を報告しました。状態を「要修理」にしました。</Notice>
  }

  if (!open) {
    return (
      <div className="mt-6 px-4">
        <Button tone="plain" full onClick={() => setOpen(true)}>
          破損を報告
        </Button>
      </div>
    )
  }

  async function run() {
    setBusy(true)
    onError('')
    try {
      const res = await reportDamage(code, text)
      onDone(res.item)
      setDone(true)
    } catch (err) {
      onError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Card header="破損の報告">
      <StackedField
        id="damage-description"
        label="どこが壊れているか"
        multiline
        value={text}
        onChange={(e) => setText(e.target.value)}
        placeholder="例: 脚のロックが割れている"
        hint="運営が後から確認します。直っているかどうかは判断しなくて構いません。"
      />
      <div className="flex gap-2 px-4 py-3">
        <Button disabled={busy || text.trim() === ''} onClick={() => void run()}>
          {busy ? '送っています…' : '報告する'}
        </Button>
        <Button tone="plain" disabled={busy} onClick={() => setOpen(false)}>
          やめる
        </Button>
      </div>
    </Card>
  )
}
