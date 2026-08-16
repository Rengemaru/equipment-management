/**
 * 認証まわりのエンドポイント。
 *
 * 経路の文字列はここにだけ書く。画面から直接 fetch を呼ばせない。
 * 呼ぶ場所が散ると、Cookie の指定や 401 の扱いが経路ごとにずれる。
 */

import { request, requestJSON } from './client'
import type { AuthResponse } from './types'

/**
 * login はログインする。
 *
 * next は `/login?next=/i/0042` の値をそのまま渡す。検証はサーバが行い、
 * 安全な行き先が redirect_to として返る。__フロントで next を解釈しない。__
 * QRから来た未ログインの利用者を元の備品ページへ戻せないと、
 * もう一度QRを読み直させることになり、その一手間が記録漏れに直結する。
 */
export function login(loginID: string, password: string, next: string): Promise<AuthResponse> {
  return requestJSON<AuthResponse>('/api/login', 'POST', {
    login_id: loginID,
    password,
    next,
  })
}

/**
 * logout はセッションを消す。
 *
 * ログイン中でなくても成功する（サーバが 204 を返す）。
 */
export function logout(): Promise<void> {
  return request<void>('/api/logout', { method: 'POST' })
}

/**
 * me はログイン中の利用者を返す。未ログインなら 401 の ApiError を投げる。
 *
 * 起動時にログイン状態を復元するために使う。セッションは Cookie に入っていて
 * JavaScript から読めない（HttpOnly）ため、サーバに聞く以外に方法がない。
 */
export function me(): Promise<AuthResponse> {
  return request<AuthResponse>('/api/me')
}

/**
 * changePassword はパスワードを変更する。
 *
 * 変更すると他の端末のセッションは全て切れる。今の端末だけは
 * サーバが繋ぎ直すため、ログインし直す必要はない。
 */
export function changePassword(
  currentPassword: string,
  newPassword: string,
): Promise<AuthResponse> {
  return requestJSON<AuthResponse>('/api/password', 'POST', {
    current_password: currentPassword,
    new_password: newPassword,
  })
}
