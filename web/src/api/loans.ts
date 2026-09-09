/**
 * 貸出まわりのエンドポイント。
 *
 * 貸出中一覧は member も見られる。誰が何を持っているかが全員に見える状態を
 * 作ることが、罰則より強く働く（CLAUDE.md）。
 */

import { request, requestJSON } from './client'
import type { Item, Loan } from './types'

/** LoanResult は貸出の操作の結果。更新後の備品も返る。 */
export type LoanResult = {
  loan: Loan
  /** item は更新後の備品。所在不明から在庫に戻った結果がここに出る。 */
  item: Item
}

/**
 * BorrowRequest は借用の入力。**全項目が省略できる。**
 *
 * 何も指定しなければ「自分が今借りる、返却予定日は14日後」になる。
 * これが3タップで終わらせる前提になっている。
 */
export type BorrowRequest = {
  /** userId は借用者。省略すると自分（指定すると代理登録）。 */
  userId?: number

  /** dueDate は 'YYYY-MM-DD'。省略すると借用日+14日。 */
  dueDate?: string

  /** borrowedAt は RFC3339。省略すると現在時刻（過去を指定すると事後登録）。 */
  borrowedAt?: string

  note?: string
}

/**
 * borrowItem は借用を記録する。
 *
 * 既に誰かが借りていれば 409（`already_borrowed`）。自由利用品は 400
 * （`free_use`）。**どちらも異常ではない。** 画面は code で分岐して、
 * 読み直せばよい。
 */
export async function borrowItem(code: string, req: BorrowRequest = {}): Promise<LoanResult> {
  const body: Record<string, unknown> = {}
  if (req.userId !== undefined) body.user_id = req.userId
  if (req.dueDate !== undefined && req.dueDate !== '') body.due_date = req.dueDate
  if (req.borrowedAt !== undefined && req.borrowedAt !== '') body.borrowed_at = req.borrowedAt
  if (req.note !== undefined && req.note !== '') body.note = req.note

  return requestJSON<LoanResult>(`/api/items/${encodeURIComponent(code)}/loans`, 'POST', body)
}

/**
 * returnItem は返却を記録する。
 *
 * **借用者本人でなくてもよい。** 棚に戻っているのを見つけた人が処理できないと、
 * 記録が永久にズレたままになる。貸出中でなければ 409（`not_borrowed`）。
 */
export async function returnItem(code: string): Promise<LoanResult> {
  return requestJSON<LoanResult>(`/api/items/${encodeURIComponent(code)}/return`, 'POST', {})
}

/**
 * cancelLoan は誤登録の貸出を取り消す。借用者本人・登録者・admin のみ。
 *
 * 返却ではない。**「返した」と「そもそも借りていない」は別の事実。**
 */
export async function cancelLoan(id: number, reason = ''): Promise<LoanResult> {
  return requestJSON<LoanResult>(`/api/loans/${id}/cancel`, 'POST', { reason })
}

/** LoanFilter は貸出中一覧の絞り込み。 */
export type LoanFilter = {
  /** query は備品コード・品名・借用者名の部分一致。 */
  query?: string
  /** overdueOnly は返却予定日を過ぎたものだけ。 */
  overdueOnly?: boolean
  userId?: number
}

/** listLoans は貸出中の一覧を返す。全メンバーが見られる。 */
export async function listLoans(filter: LoanFilter = {}): Promise<Loan[]> {
  const params = new URLSearchParams()

  const q = filter.query?.trim() ?? ''
  if (q !== '') params.set('q', q)
  if (filter.overdueOnly === true) params.set('overdue', '1')
  if (filter.userId !== undefined) params.set('user_id', String(filter.userId))

  const query = params.toString()
  const res = await request<{ loans: Loan[] }>(`/api/loans${query === '' ? '' : `?${query}`}`)
  return res.loans
}

/** MyLoans は自分の貸出中と履歴。1リクエストで両方返る。 */
export type MyLoans = {
  active: Loan[]
  returned: Loan[]
}

/**
 * myLoans は自分の貸出中と履歴を返す。
 *
 * 2回に分けて取らない。片方だけ更新された表示が出る。
 */
export async function myLoans(): Promise<MyLoans> {
  return request<MyLoans>('/api/loans/mine')
}
