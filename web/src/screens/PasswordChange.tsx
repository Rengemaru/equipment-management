import { useState } from 'react'
import type { FormEvent } from 'react'
import { useNavigate } from 'react-router'

import { errorMessage } from '../api/client'
import { useAuth } from '../auth/AuthProvider'

/**
 * PasswordChange はパスワード変更画面。
 *
 * RequireAuth の内側でのみ使う。初期パスワードのままの利用者は、
 * ここを終えるまで他の画面へ進めない。
 *
 * 変更すると他の端末のセッションは全て切れる。この端末はサーバが
 * 繋ぎ直すため、ログインし直す必要はない。
 */
export default function PasswordChange() {
  const auth = useAuth()
  const navigate = useNavigate()

  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)

  // 初期パスワードのままかどうかで文面を変える。強制されて来た人に
  // 「なぜこの画面なのか」を出さないと、操作を誤ったように見える。
  const forced = auth.status === 'authenticated' && auth.user.must_change_password

  const handleSubmit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    setError('')

    // 入力の取り違えはサーバには分からない。ここでしか確認できない。
    //
    // 長さなどの規則はサーバが持つ。こちらに書き写すと、片方だけ直した時に
    // 「画面は通すのに登録できない」状態になる。往復1回で済むため、
    // 先回りして同じ検査を書かない。
    if (next !== confirm) {
      setError('新しいパスワードが一致しません')
      return
    }

    setSubmitting(true)
    try {
      const redirectTo = await auth.changePassword(current, next)
      navigate(redirectTo, { replace: true })
    } catch (err) {
      setError(errorMessage(err))
      setSubmitting(false)
    }
  }

  return (
    <main className="mx-auto max-w-screen-sm p-4">
      <h1 className="text-xl font-bold">パスワードの変更</h1>

      {forced && (
        <p className="mt-2 rounded bg-amber-50 p-3 text-sm text-amber-900">
          初期パスワードのままです。変更するまで他の画面は使えません。
        </p>
      )}

      <form className="mt-4 space-y-4" onSubmit={(e) => void handleSubmit(e)}>
        <div>
          <label className="block text-sm font-medium" htmlFor="current-password">
            現在のパスワード
          </label>
          <input
            id="current-password"
            name="current_password"
            type="password"
            autoComplete="current-password"
            required
            // text-base（16px）未満だと iOS が焦点を当てた瞬間に拡大する。
            className="mt-1 w-full rounded border border-gray-300 px-3 py-2 text-base"
            value={current}
            onChange={(e) => setCurrent(e.target.value)}
          />
        </div>

        <div>
          <label className="block text-sm font-medium" htmlFor="new-password">
            新しいパスワード
          </label>
          <input
            id="new-password"
            name="new_password"
            type="password"
            autoComplete="new-password"
            required
            className="mt-1 w-full rounded border border-gray-300 px-3 py-2 text-base"
            value={next}
            onChange={(e) => setNext(e.target.value)}
          />
        </div>

        <div>
          <label className="block text-sm font-medium" htmlFor="confirm-password">
            新しいパスワード（確認）
          </label>
          <input
            id="confirm-password"
            name="confirm_password"
            type="password"
            autoComplete="new-password"
            required
            className="mt-1 w-full rounded border border-gray-300 px-3 py-2 text-base"
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
          />
        </div>

        {error !== '' && (
          <p role="alert" className="text-sm text-red-700">
            {error}
          </p>
        )}

        <button
          type="submit"
          disabled={submitting}
          className="w-full rounded bg-blue-700 px-4 py-3 text-base text-white disabled:bg-gray-400"
        >
          {submitting ? '変更しています…' : '変更する'}
        </button>
      </form>
    </main>
  )
}
