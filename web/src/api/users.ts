/**
 * ユーザー管理のエンドポイント。すべて admin のみ。
 *
 * 画面で隠すだけにしない。member が叩いても 403 になることはAPIが保証する
 * （CLAUDE.md）。
 */

import { request, requestJSON } from './client'
import type { AdminUser, Role, UserWithPassword } from './types'

/** listUsers は利用者を全件返す。無効化された人も含む。 */
export async function listUsers(): Promise<AdminUser[]> {
  const res = await request<{ users: AdminUser[] }>('/api/users')
  return res.users
}

/**
 * createUser は利用者を作り、初期パスワードを返す。
 *
 * __パスワードは送らない。__ 運営が考えた文字列を初期パスワードにすると、
 * 全員に同じものが配られる。サーバが生成し、応答で一度だけ返す。
 */
export function createUser(input: {
  name: string
  loginID: string
  email: string
  role: Role
}): Promise<UserWithPassword> {
  return requestJSON<UserWithPassword>('/api/users', 'POST', {
    name: input.name,
    login_id: input.loginID,
    email: input.email,
    role: input.role,
  })
}

/**
 * setUserActive は有効・無効を切り替える。
 *
 * 利用者は消さない。卒業者は無効化する。消すと貸出履歴が壊れる（CLAUDE.md）。
 * __最後の admin は無効化できない__（サーバが 409 を返す）。全員無効になると
 * Webから復旧できなくなる。
 */
export async function setUserActive(id: number, active: boolean): Promise<AdminUser> {
  const action = active ? 'activate' : 'deactivate'
  const res = await request<{ user: AdminUser }>(`/api/users/${id}/${action}`, { method: 'POST' })
  return res.user
}

/**
 * resetPassword は初期パスワードを再発行する。
 *
 * 自己リセットの導線は作らない（メールに依存するため。m1-spec §3）。
 * 入れなくなった人は運営に頼む。再発行すると古いセッションは全て切れる。
 */
export function resetPassword(id: number): Promise<UserWithPassword> {
  return request<UserWithPassword>(`/api/users/${id}/reset-password`, { method: 'POST' })
}
