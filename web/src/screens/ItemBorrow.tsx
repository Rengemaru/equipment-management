import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router'

import { ApiError, errorMessage } from '../api/client'
import { getItem } from '../api/items'
import { borrowItem } from '../api/loans'
import type { Item, Member } from '../api/types'
import { listMembers } from '../api/users'
import { useAuth } from '../auth/AuthProvider'
import { defaultDueDate, formatDate } from '../lib/date'
import { Button } from '../ui/Button'
import { Field, SelectField, StackedField } from '../ui/Field'
import { Alert, Loading, Notice } from '../ui/Feedback'
import { Card, List, Row } from '../ui/List'
import { Screen } from '../ui/Screen'

/**
 * ItemBorrow は借用の確認画面（`/i/{code}/borrow`）。
 *
 * # タップ数
 *
 * QRを読む（操作ゼロ）→ 詳細で「借りる」（1）→ ここで「この内容で借りる」（2）。
 * __3タップは上限であって目標ではない。__ 返却予定日の変更と事後登録は
 * 「変更する」の内側に置き、既定のまま進む人のタップ数を増やさない。
 *
 * # 現在の状態と備考を必ず見せる（m2-spec §3）
 *
 * 破損報告は任意ボタンなので、報告されないまま次の人が借りるケースが必ず出る。
 * そのとき現状表示がないと、__前の人の破損が次の人の責任になる。__
 * 借りる前に見せておけば、食い違いに気づいた人がその場で報告できる。
 */
export default function ItemBorrow() {
  const { code = '' } = useParams()
  const navigate = useNavigate()
  const auth = useAuth()
  const meID = auth.status === 'authenticated' ? auth.user.id : 0

  const [item, setItem] = useState<Item | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  // 変更する人だけが開く。既定のまま進む人には見せない。
  const [detailed, setDetailed] = useState(false)
  const [dueDate, setDueDate] = useState(defaultDueDate())
  const [borrowedAt, setBorrowedAt] = useState('')
  const [note, setNote] = useState('')

  // 借用者。空文字は自分。__既定を自分から動かさない。__
  const [userID, setUserID] = useState('')
  const [members, setMembers] = useState<Member[]>([])

  useEffect(() => {
    let alive = true

    void getItem(code).then(
      (it) => {
        if (alive) setItem(it)
      },
      (err: unknown) => {
        if (alive) setError(errorMessage(err))
      },
    )

    return () => {
      alive = false
    }
  }, [code])

  useEffect(() => {
    // 「変更する」を開いた人にだけ要る。開くまで取りに行かない。
    if (!detailed || members.length > 0) return

    let alive = true
    // 取れなくても借用そのものは成立する。ここで画面を止めない。
    void listMembers().then(
      (list) => {
        if (alive) setMembers(list)
      },
      () => {},
    )

    return () => {
      alive = false
    }
  }, [detailed, members.length])

  const back = { to: `/i/${encodeURIComponent(code)}`, label: '備品' }

  async function run() {
    setBusy(true)
    setError('')
    try {
      await borrowItem(code, {
        // 触っていない項目は送らない。サーバの既定に委ねる。
        userId: detailed && userID !== '' ? Number(userID) : undefined,
        dueDate: detailed ? dueDate : undefined,
        borrowedAt: detailed && borrowedAt !== '' ? toRFC3339(borrowedAt) : undefined,
        note: detailed ? note : undefined,
      })
      void navigate(`/i/${encodeURIComponent(code)}`, { replace: true })
    } catch (err) {
      // 409 は「他の人が先に借りた」。異常ではないので、そう伝えて詳細へ戻す。
      if (err instanceof ApiError && err.status === 409) {
        setError('他の人が先に借りました。備品の画面で最新の状態を確認してください。')
      } else {
        setError(errorMessage(err))
      }
      setBusy(false)
    }
  }

  if (error !== '' && item === null) {
    return (
      <Screen title="借りる" subtitle={code} back={back}>
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

  return (
    <Screen title="借りる" subtitle={`${item.code} ${item.name}`} back={back} narrow>
      {error !== '' && <Alert>{error}</Alert>}

      {item.location_status !== '在庫' && (
        <Notice tone="warn">
          この備品は「{item.location_status}」として記録されています。
          <span className="mt-1 block text-[13px]">
            借りると在庫に戻ります。報告は要りません。
          </span>
        </Notice>
      )}

      {/* 借りる前に現状を見せる。責任の誤帰属を防ぐ。 */}
      <List header="今の状態">
        <Row label="状態" value={item.condition} />
        <Row label="保管場所" value={item.location} />
      </List>

      <Card header="備考">
        <p className="px-4 py-3 text-[17px] leading-relaxed whitespace-pre-wrap">
          {item.note === '' ? (
            <span className="text-label-3">—</span>
          ) : (
            item.note
          )}
        </p>
        <p className="px-4 pb-3 text-[13px] text-label-2">
          書かれていない破損に気づいたら、借りた後に備品の画面から報告してください。
        </p>
      </Card>

      {!detailed ? (
        <List footer={`返却予定日は${formatDate(dueDate)}です。`}>
          <button
            type="button"
            className="flex min-h-11 w-full items-center px-4 text-left text-[17px] text-tint active:opacity-60"
            onClick={() => setDetailed(true)}
          >
            返却予定日や借用日時を変更する
          </button>
        </List>
      ) : (
        <List footer="持ち出した後に登録する場合は、借用日時を実際の時刻にしてください。">
          {/* 代理登録。「〇〇さんが持っていくのを見た」を第三者が登録できる。
              __本人にはメールで通知され、本人が取り消せる__（m2-spec §4, §5）。 */}
          <SelectField
            id="borrower"
            label="借りる人"
            value={userID}
            onChange={(e) => setUserID(e.target.value)}
          >
            <option value="">自分</option>
            {members
              .filter((m) => m.id !== meID)
              .map((m) => (
                <option key={m.id} value={String(m.id)}>
                  {m.name}
                </option>
              ))}
          </SelectField>
          <Field
            id="due-date"
            label="返却予定日"
            type="date"
            value={dueDate}
            onChange={(e) => setDueDate(e.target.value)}
          />
          <Field
            id="borrowed-at"
            label="借用日時"
            type="datetime-local"
            value={borrowedAt}
            onChange={(e) => setBorrowedAt(e.target.value)}
            hint="空欄なら今の時刻"
          />
          <StackedField
            id="note"
            label="メモ（任意）"
            value={note}
            onChange={(e) => setNote(e.target.value)}
          />
        </List>
      )}

      {detailed && userID !== '' && (
        <Notice>
          {members.find((m) => String(m.id) === userID)?.name ?? 'この人'}
          さんの借用として記録し、本人にメールで知らせます。
        </Notice>
      )}

      <div className="mt-6 px-4">
        <Button full disabled={busy} onClick={() => void run()}>
          {busy ? '記録しています…' : userID === '' ? 'この内容で借りる' : 'この内容で登録する'}
        </Button>
      </div>
    </Screen>
  )
}

/**
 * toRFC3339 は `datetime-local` の値をサーバへ送れる形にする。
 *
 * 入力欄はタイムゾーンを持たない文字列（`2026-09-10T19:30`）を返す。
 * そのまま送るとサーバが解釈できない。端末の時計の地域差を付けて送る。
 */
function toRFC3339(local: string): string {
  const at = new Date(local)
  if (Number.isNaN(at.getTime())) return local
  return at.toISOString()
}
