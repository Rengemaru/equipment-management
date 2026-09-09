import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router'

import { ApiError, errorMessage } from '../api/client'
import { cancelLoan, myLoans, returnItem } from '../api/loans'
import type { Loan } from '../api/types'
import { formatDate, formatDateTime } from '../lib/date'
import { Button } from '../ui/Button'
import { Alert, Badge, Empty, Loading } from '../ui/Feedback'
import { Card, List } from '../ui/List'
import { Screen } from '../ui/Screen'

/**
 * MyLoans は自分の貸出中と履歴（`/loans/mine`）。
 *
 * 代理登録の通知メールはこの画面を指す。__身に覚えのない借用を__
 * __その場で取り消せることが、代理登録を成立させる条件__になる（m2-spec §5）。
 */
export default function MyLoans() {
  const [active, setActive] = useState<Loan[] | null>(null)
  const [returned, setReturned] = useState<Loan[]>([])
  const [error, setError] = useState('')

  const load = useCallback(async () => {
    setError('')
    try {
      const res = await myLoans()
      setActive(res.active)
      setReturned(res.returned)
    } catch (err) {
      setError(errorMessage(err))
      setActive([])
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  return (
    <Screen title="自分の貸出" back={{ to: '/', label: 'トップ' }}>
      {error !== '' && <Alert>{error}</Alert>}

      {active === null ? (
        <Loading />
      ) : active.length === 0 ? (
        <Empty
          title="借りているものはありません"
          hint="棚のQRを読むと、その場で借りられます。"
        />
      ) : (
        <List header="借りているもの">
          {active.map((loan) => (
            <ActiveRow key={loan.id} loan={loan} onChanged={load} onError={setError} />
          ))}
        </List>
      )}

      {returned.length > 0 && (
        <Card header="返した履歴">
          <ul>
            {returned.map((loan) => (
              <li key={loan.id} className="px-4 py-3">
                <Link className="text-[17px]" to={`/i/${encodeURIComponent(loan.item.code)}`}>
                  {loan.item.name}
                </Link>
                <p className="mt-0.5 text-[15px] text-label-2">
                  {loan.item.code} / {formatDateTime(loan.borrowed_at)} から
                  {loan.returned_at !== null && ` ${formatDateTime(loan.returned_at)} に返却`}
                </p>
              </li>
            ))}
          </ul>
        </Card>
      )}
    </Screen>
  )
}

/**
 * ActiveRow は貸出中の1件。返却と取り消しをここから行う。
 *
 * __返却と取り消しを同じボタンにまとめない。__ 「返した」と
 * 「そもそも借りていない」は別の事実で、まとめると記録が嘘になる。
 */
function ActiveRow({
  loan,
  onChanged,
  onError,
}: {
  loan: Loan
  onChanged: () => Promise<void>
  onError: (msg: string) => void
}) {
  const [busy, setBusy] = useState(false)
  const [cancelling, setCancelling] = useState(false)

  async function run(action: () => Promise<unknown>, conflict: string) {
    setBusy(true)
    onError('')
    try {
      await action()
      await onChanged()
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        // 二重タップと「他の人が先に処理した」の両方がここに来る。
        // 読み直しが先。後にすると load がエラー表示を消す。
        await onChanged()
        onError(conflict)
      } else {
        onError(errorMessage(err))
      }
    } finally {
      setBusy(false)
    }
  }

  return (
    <li className="px-4 py-3">
      <div className="flex items-center gap-2">
        <Link className="truncate text-[17px]" to={`/i/${encodeURIComponent(loan.item.code)}`}>
          {loan.item.name}
        </Link>
        {loan.overdue_days > 0 && <Badge tone="warn">{loan.overdue_days}日超過</Badge>}
      </div>

      <p className="mt-0.5 text-[15px] text-label-2">
        {loan.item.code} / 返却予定 {formatDate(loan.due_date)}
      </p>

      {/* 代理登録は誤登録がつきまとう。誰が登録したかを本人に見せる。
          見せないと、身に覚えのない借用の出どころが分からない。 */}
      {loan.is_proxy && (
        <p className="mt-1 text-[13px] text-label-2">
          {loan.registered_by.name} さんが登録しました
        </p>
      )}

      {cancelling ? (
        <div className="mt-2 flex flex-wrap gap-2">
          <Button
            tone="danger"
            disabled={busy}
            onClick={() => void run(() => cancelLoan(loan.id), 'この貸出は既に処理されています。')}
          >
            {busy ? '取り消しています…' : '借りていないので取り消す'}
          </Button>
          <Button tone="plain" disabled={busy} onClick={() => setCancelling(false)}>
            やめる
          </Button>
        </div>
      ) : (
        <div className="mt-2 flex flex-wrap gap-2">
          {/* 返却に確認を挟まない。誤返却は再度借りれば済むが、
              確認を挟むと記録そのものが飛ぶ。 */}
          <Button
            disabled={busy}
            onClick={() =>
              void run(() => returnItem(loan.item.code), 'すでに返却されています。')
            }
          >
            {busy ? '返しています…' : '返す'}
          </Button>
          {/* 取り消しは確認を挟む。返却と違い、取り消しは記録を
              「無かったこと」にする操作で、押し間違いを戻せない。 */}
          <Button tone="plain" disabled={busy} onClick={() => setCancelling(true)}>
            借りていない
          </Button>
        </div>
      )}
    </li>
  )
}
