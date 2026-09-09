import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router'

import { errorMessage } from '../api/client'
import { listDamages, setDamageStatus } from '../api/damages'
import { DAMAGE_STATUSES } from '../api/types'
import type { DamageReport, DamageStatus } from '../api/types'
import { formatDateTime } from '../lib/date'
import { Button } from '../ui/Button'
import { Alert, Badge, Empty, Loading } from '../ui/Feedback'
import { Card } from '../ui/List'
import { Screen } from '../ui/Screen'

/**
 * AdminDamages は破損報告の**追認**画面（`/admin/damages`）。
 *
 * __承認ではない。__ 報告は既に反映済み（報告した時点で備品は要修理になっている）で、
 * ここでの操作は事後の確認と処理の記録。承認待ちの間システムが「良好」と
 * 表示し続ける状態は、紙の台帳より悪い（CLAUDE.md）。
 *
 * __削除の導線を置かない。__ 報告は履歴で、誤報告も「そう報告された」という事実。
 */
export default function AdminDamages() {
  // 既定は未確認だけ。運営が見たいのは「まだ手を付けていないもの」で、
  // 全件を既定にすると処理済みが積み上がって埋もれる。
  const [status, setStatus] = useState<DamageStatus | ''>('未確認')

  const [reports, setReports] = useState<DamageReport[] | null>(null)
  const [error, setError] = useState('')

  const load = useCallback(async () => {
    setError('')
    try {
      setReports(await listDamages(status === '' ? undefined : status))
    } catch (err) {
      setError(errorMessage(err))
      setReports([])
    }
  }, [status])

  useEffect(() => {
    setReports(null)
    void load()
  }, [load])

  return (
    <Screen title="破損報告" back={{ to: '/', label: 'トップ' }}>
      <div className="mt-4 flex flex-wrap gap-2 px-1">
        <FilterButton current={status} value="未確認" onSelect={setStatus} />
        <FilterButton current={status} value="" onSelect={setStatus} label="すべて" />
      </div>

      {error !== '' && <Alert>{error}</Alert>}

      {reports === null ? (
        <Loading />
      ) : reports.length === 0 ? (
        <Empty
          title={status === '未確認' ? '未確認の報告はありません' : '報告はありません'}
          hint="メンバーが報告すると、その時点で備品は要修理になります。"
        />
      ) : (
        reports.map((report) => (
          <ReportCard key={report.id} report={report} onChanged={load} onError={setError} />
        ))
      )}
    </Screen>
  )
}

function FilterButton({
  current,
  value,
  label,
  onSelect,
}: {
  current: DamageStatus | ''
  value: DamageStatus | ''
  label?: string
  onSelect: (v: DamageStatus | '') => void
}) {
  const active = current === value
  return (
    <button
      type="button"
      className={`min-h-11 rounded-[10px] px-3 text-[15px] ${
        active ? 'bg-tint text-white' : 'bg-fill text-tint'
      }`}
      aria-pressed={active}
      onClick={() => onSelect(value)}
    >
      {label ?? value}
    </button>
  )
}

/**
 * ReportCard は報告1件。
 *
 * 状態を変えると備品の状態が連動する（m2-spec §7）。
 * __押す前にその結果を書く。__ 書かないと、直したつもりで廃棄にする人が出る。
 */
function ReportCard({
  report,
  onChanged,
  onError,
}: {
  report: DamageReport
  onChanged: () => Promise<void>
  onError: (msg: string) => void
}) {
  const [busy, setBusy] = useState(false)

  async function run(next: DamageStatus) {
    setBusy(true)
    onError('')
    try {
      await setDamageStatus(report.id, next)
      await onChanged()
    } catch (err) {
      onError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Card>
      <div className="px-4 py-3">
        <div className="flex items-center gap-2">
          <Link className="text-[17px]" to={`/i/${encodeURIComponent(report.item.code)}`}>
            {report.item.code} {report.item.name}
          </Link>
          {report.status === '未確認' && <Badge tone="warn">未確認</Badge>}
          {report.status !== '未確認' && <Badge tone="info">{report.status}</Badge>}
        </div>

        <p className="mt-2 text-[17px] leading-relaxed whitespace-pre-wrap">
          {report.description}
        </p>

        <p className="mt-1 text-[13px] text-label-2">
          {report.reporter.name} さん / {formatDateTime(report.reported_at)}
          {/* 貸出中の報告なら、誰が借りている間に壊れたのかを辿れるようにする。 */}
          {report.loan_id !== null && ' / 貸出中に報告'}
        </p>

        {report.confirmed_by !== null && (
          <p className="mt-1 text-[13px] text-label-2">
            {report.confirmed_by.name} さんが確認
            {report.confirmed_at !== null && ` / ${formatDateTime(report.confirmed_at)}`}
          </p>
        )}
      </div>

      <div className="flex flex-wrap gap-2 px-4 pb-3">
        {DAMAGE_STATUSES.filter((s) => s !== '未確認' && s !== report.status).map((s) => (
          <Button
            key={s}
            tone={s === '廃棄' ? 'danger' : 'tinted'}
            disabled={busy}
            onClick={() => void run(s)}
          >
            {s}にする
          </Button>
        ))}
      </div>

      {/* 状態と備品の連動を書く。押した後に気づく形にしない。 */}
      <p className="px-4 pb-3 text-[13px] leading-snug text-label-2">
        「修理済み」にすると備品の状態が<strong>良好</strong>に戻ります。
        「廃棄」にすると<strong>廃棄</strong>になり、借りられなくなります。
        「確認済み」は要修理のままです。
      </p>
    </Card>
  )
}
