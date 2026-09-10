/**
 * 日付の扱い。
 *
 * サーバはJSTの日付で返却予定日を決める（UTCで計算すると JST 00:00〜09:00 の分が
 * 1日ずれる）。画面側も同じ基準で計算しないと、既定として見せた日付と
 * 実際に登録される日付が食い違う。
 */

/** JST_OFFSET_MS は UTC からの差（+9時間）。夏時間が無いため固定でよい。 */
const JST_OFFSET_MS = 9 * 60 * 60 * 1000

/** DEFAULT_LOAN_DAYS は返却予定日の既定（借用日+14日）。Go 側の defaultLoanDays と対。 */
export const DEFAULT_LOAN_DAYS = 14

/** toJST は端末の時計に関係なくJSTの時刻に直す。 */
function toJST(at: Date): Date {
  return new Date(at.getTime() + JST_OFFSET_MS)
}

/**
 * jstDate はJSTの日付を 'YYYY-MM-DD' で返す。
 *
 * getFullYear ではなく UTC 系を使う。ずらした後の値を端末のタイムゾーンで
 * 読み直すと、二重にずれる。
 */
export function jstDate(at: Date = new Date()): string {
  const d = toJST(at)
  const mm = String(d.getUTCMonth() + 1).padStart(2, '0')
  const dd = String(d.getUTCDate()).padStart(2, '0')
  return `${d.getUTCFullYear()}-${mm}-${dd}`
}

/** defaultDueDate は借用日から数えた返却予定日の既定。 */
export function defaultDueDate(borrowedAt: Date = new Date()): string {
  return jstDate(new Date(borrowedAt.getTime() + DEFAULT_LOAN_DAYS * 24 * 60 * 60 * 1000))
}

/**
 * formatDate は 'YYYY-MM-DD' を「9月24日」の形にする。
 *
 * 年は出さない。貸出は数週間で終わるもので、年まで出すと読む量が増える。
 */
export function formatDate(date: string): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(date)
  if (m === null) return date
  return `${Number(m[2])}月${Number(m[3])}日`
}

/**
 * formatDateTime は RFC3339 を「9月10日 19:30」の形にする。
 *
 * 読めない値はそのまま返す。表示のために例外を投げない。
 */
export function formatDateTime(at: string): string {
  const t = Date.parse(at)
  if (Number.isNaN(t)) return at

  const d = toJST(new Date(t))
  const hh = String(d.getUTCHours()).padStart(2, '0')
  const mi = String(d.getUTCMinutes()).padStart(2, '0')
  return `${d.getUTCMonth() + 1}月${d.getUTCDate()}日 ${hh}:${mi}`
}
