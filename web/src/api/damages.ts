/**
 * 破損報告のエンドポイント。
 *
 * **報告は全員できる。** 報告のハードルを上げると、壊れたまま次の人が借りる。
 * 承認を待たず、報告した時点で備品が要修理になる。
 */

import { request, requestJSON } from './client'
import type { DamageReport, DamageStatus, Item } from './types'

/** DamageResult は報告・追認の結果。更新後の備品も返る。 */
export type DamageResult = {
  report: DamageReport
  /** item は更新後の備品。報告した時点で condition が要修理に変わる。 */
  item: Item
}

/**
 * reportDamage は破損を報告する。説明は必須。
 *
 * 「壊れた」とだけ記録されても、運営が現物を見るまで何も判断できない。
 */
export async function reportDamage(code: string, description: string): Promise<DamageResult> {
  return requestJSON<DamageResult>(`/api/items/${encodeURIComponent(code)}/damages`, 'POST', {
    description,
  })
}

/** listDamages は破損報告の一覧を返す。**admin のみ。** */
export async function listDamages(status?: DamageStatus): Promise<DamageReport[]> {
  const query = status === undefined ? '' : `?status=${encodeURIComponent(status)}`
  const res = await request<{ reports: DamageReport[] }>(`/api/damages${query}`)
  return res.reports
}

/**
 * setDamageStatus は運営の追認を記録する。**admin のみ。**
 *
 * 承認ではなく追認。備品の状態も連動する（修理済み→良好、廃棄→廃棄）。
 */
export async function setDamageStatus(
  id: number,
  status: DamageStatus,
  note = '',
): Promise<DamageResult> {
  return requestJSON<DamageResult>(`/api/damages/${id}/status`, 'POST', { status, note })
}
