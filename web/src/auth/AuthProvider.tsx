import { createContext, useCallback, useContext, useEffect, useState } from 'react'
import type { ReactNode } from 'react'

import * as api from '../api/auth'
import { ApiError, onSessionExpired } from '../api/client'
import type { User } from '../api/types'

/**
 * AuthState はログイン状態。
 *
 * 4つを1つの型に潰さない。`user: User | null` と `loading: boolean` の
 * 組み合わせにすると、起動直後の「まだ分からない」と「未ログイン」が
 * 同じ形になり、確認が終わる前にログイン画面を一瞬出してしまう。
 */
export type AuthState =
  | { status: 'loading' }
  | { status: 'authenticated'; user: User }
  | { status: 'anonymous' }
  /**
   * unavailable はサーバに聞けなかったこと。
   *
   * 未ログインとして扱わない。繋がらないだけなのにログイン画面を出すと、
   * 利用者はIDとパスワードを打ち込んで、また同じ失敗を見ることになる。
   */
  | { status: 'unavailable'; message: string }

/** AuthContextValue は状態と操作。 */
export type AuthContextValue = AuthState & {
  /**
   * login はログインし、進むべきパスを返す。
   *
   * 返るのはサーバが検証した redirect_to。呼び出し側はこの値へ進むだけでよい。
   * 失敗は ApiError として投げる。画面はその message をそのまま出せる。
   */
  login(loginID: string, password: string, next: string): Promise<string>

  /**
   * logout はログアウトする。__失敗しても投げない。__
   *
   * 押した人にできることが無い操作で、投げると呼び出し側全てに
   * 意味のない catch を書かせることになる（書き忘れた画面では
   * 未処理の例外になる）。
   */
  logout(): Promise<void>

  /** changePassword はパスワードを変更し、進むべきパスを返す。 */
  changePassword(currentPassword: string, newPassword: string): Promise<string>

  /** reload はサーバに現在のログイン状態を聞き直す。 */
  reload(): Promise<void>
}

const AuthContext = createContext<AuthContextValue | null>(null)

/**
 * AuthProvider はログイン状態を保持する。
 *
 * 状態はサーバにしか無い。セッションは HttpOnly Cookie に入っていて
 * JavaScript から読めないため、起動のたびに `/api/me` で確認する。
 * localStorage に写しを置かないこと。ログアウトや無効化の後も
 * 「ログイン中」と表示し続ける原因になる。
 */
export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AuthState>({ status: 'loading' })

  const reload = useCallback(async () => {
    try {
      const res = await api.me()
      setState({ status: 'authenticated', user: res.user })
    } catch (err) {
      setState(toAnonymousOrUnavailable(err))
    }
  }, [])

  useEffect(() => {
    // 応答が返る前に外された場合に備える。外れた後に setState すると、
    // React が「消えた要素を更新した」と警告する。
    let alive = true

    void (async () => {
      try {
        const res = await api.me()
        if (alive) setState({ status: 'authenticated', user: res.user })
      } catch (err) {
        if (alive) setState(toAnonymousOrUnavailable(err))
      }
    })()

    return () => {
      alive = false
    }
  }, [])

  useEffect(() => {
    // どの画面の呼び出しで 401 が返っても、ここで未ログインに戻す。
    // 画面ごとに拾わせると、必ず拾い忘れた画面ができる。
    return onSessionExpired(() => {
      setState((prev) => {
        // ログイン画面でパスワードを間違えた時も 401 が返る。
        // 元から未ログインなら何もしない。
        if (prev.status !== 'authenticated') return prev
        return { status: 'anonymous' }
      })
    })
  }, [])

  const login = useCallback(async (loginID: string, password: string, next: string) => {
    const res = await api.login(loginID, password, next)
    setState({ status: 'authenticated', user: res.user })
    return res.redirect_to
  }, [])

  const logout = useCallback(async () => {
    try {
      await api.logout()
    } catch (err) {
      // サーバ側を消せなくても、この端末は未ログインとして扱う。
      // 「ログアウトを押したのに何も起きない」が一番困る。
      //
      // ただし握り潰しはしない。サーバ側のセッションが残っている場合、
      // 再読み込みするとログイン状態に戻る。原因を追える形で残す。
      console.warn('ログアウトに失敗した（この端末では未ログインとして扱う）', err)
    } finally {
      setState({ status: 'anonymous' })
    }
  }, [])

  const changePassword = useCallback(async (currentPassword: string, newPassword: string) => {
    const res = await api.changePassword(currentPassword, newPassword)
    setState({ status: 'authenticated', user: res.user })
    return res.redirect_to
  }, [])

  return (
    <AuthContext.Provider value={{ ...state, login, logout, changePassword, reload }}>
      {children}
    </AuthContext.Provider>
  )
}

/**
 * useAuth はログイン状態と操作を返す。
 *
 * Provider の外で呼ぶと投げる。null を返す形にすると、被せ忘れた画面が
 * 「常に未ログイン」として静かに動いてしまう。
 */
export function useAuth(): AuthContextValue {
  const value = useContext(AuthContext)
  if (value === null) {
    throw new Error('useAuth は AuthProvider の中で呼ぶこと')
  }
  return value
}

/**
 * toAnonymousOrUnavailable は `/api/me` の失敗を状態に変える。
 *
 * 401 だけが「未ログイン」。それ以外（サーバが落ちている、DBに触れない）を
 * 未ログインに混ぜると、障害がログイン画面として見えることになる。
 */
function toAnonymousOrUnavailable(err: unknown): AuthState {
  if (err instanceof ApiError && err.status === 401) {
    return { status: 'anonymous' }
  }
  if (err instanceof ApiError) {
    return { status: 'unavailable', message: err.message }
  }
  return { status: 'unavailable', message: '予期しない問題が起きました' }
}
