import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { useSearchParams } from 'react-router'

import { errorMessage } from '../api/client'
import { listLoans } from '../api/loans'
import type { Loan } from '../api/types'
import { formatDate } from '../lib/date'
import { Alert, Badge, Empty, Loading } from '../ui/Feedback'
import { LinkRow, List } from '../ui/List'
import { Screen } from '../ui/Screen'

/**
 * Loans は貸出中の一覧。**全メンバーが見られる。**
 *
 * 誰が何を持っているかが全員に見える状態を作ることが目的で、
 * 可視性は罰則より強く働く（CLAUDE.md）。
 *
 * 絞り込みの状態はURLのクエリに持つ。画面の中だけに持つと、戻る操作で
 * 条件が消え、探し直しになる。
 */
export default function Loans() {
  const [params, setParams] = useSearchParams()

  const query = params.get('q') ?? ''
  const overdue = params.get('overdue') === '1'

  // 検索語だけは入力中の値を画面に持つ。1文字ごとに問い合わせると、
  // 打っている間ずっと通信が走り、部室の回線では入力が詰まる。
  const [queryInput, setQueryInput] = useState(query)

  const [loans, setLoans] = useState<Loan[] | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    // 条件を変えた直後に古い応答が届くと、新しい条件の結果を上書きする。
    let alive = true

    setLoans(null)
    setError('')

    void listLoans({ query, overdueOnly: overdue }).then(
      (list) => {
        if (alive) setLoans(list)
      },
      (err: unknown) => {
        if (alive) setError(errorMessage(err))
      },
    )

    return () => {
      alive = false
    }
  }, [query, overdue])

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

  return (
    <Screen title="貸出中" back={{ to: '/', label: 'トップ' }}>
      <form className="mt-4" onSubmit={handleSearch} role="search">
        <label className="sr-only" htmlFor="q">
          品名・備品コード・借用者で検索
        </label>
        <div className="flex items-center gap-2 rounded-[10px] bg-fill px-2.5">
          <input
            id="q"
            type="search"
            placeholder="品名・備品コード・借用者"
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

      {/* 期限超過だけに絞る。__「正常」を絞る条件は置かない。__
          全件に当てはまる条件は、注意すべき行を探す助けにならない。 */}
      <div className="mt-3 px-1">
        <button
          type="button"
          className={`min-h-11 rounded-[10px] px-3 text-[15px] ${
            overdue ? 'bg-tint text-white' : 'bg-fill text-tint'
          }`}
          aria-pressed={overdue}
          onClick={() => update('overdue', overdue ? '' : '1')}
        >
          期限を過ぎたものだけ
        </button>
      </div>

      {error !== '' && <Alert>{error}</Alert>}

      {loans === null ? (
        <Loading />
      ) : loans.length === 0 ? (
        <Empty
          title={overdue || query !== '' ? '条件に合う貸出はありません' : '貸出中の備品はありません'}
          hint={
            overdue || query !== ''
              ? '条件を外すと全ての貸出中が出ます。'
              : '棚のQRを読むと、その場で借りられます。'
          }
        />
      ) : (
        <List header={`${loans.length}件`}>
          {loans.map((loan) => (
            <LoanRow key={loan.id} loan={loan} />
          ))}
        </List>
      )}
    </Screen>
  )
}

/**
 * LoanRow は1件の貸出。
 *
 * 行から備品の画面へ行けるようにする。__一覧で見つけた人が、そこから__
 * __返却まで辿り着けないと意味がない。__
 */
function LoanRow({ loan }: { loan: Loan }) {
  return (
    <LinkRow to={`/i/${encodeURIComponent(loan.item.code)}`}>
      <div className="flex items-center gap-2">
        <span className="truncate text-[17px]">{loan.item.name}</span>
        {/* __期限内であることの印は出さない。__ 全行に付く印は、
            注意すべき行を埋もれさせる。 */}
        {loan.overdue_days > 0 && <Badge tone="warn">{loan.overdue_days}日超過</Badge>}
      </div>
      <p className="mt-0.5 truncate text-[15px] text-label-2">
        {loan.item.code} / {loan.user.name} さん / 返却予定 {formatDate(loan.due_date)}
      </p>
    </LinkRow>
  )
}
