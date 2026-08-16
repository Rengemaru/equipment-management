import { useState } from 'react'
import type { FormEvent } from 'react'
import { Navigate, useSearchParams } from 'react-router'

import { errorMessage } from '../api/client'
import { useAuth } from '../auth/AuthProvider'

/**
 * Login はログイン画面。
 *
 * `?next=` は読むだけで解釈しない。安全かどうかはサーバが判断し、
 * 進む先は応答の redirect_to で返る。判断を2箇所に分けると、
 * 片方だけ直した時にオープンリダイレクトが開く。
 */
export default function Login() {
  const auth = useAuth()
  const [params] = useSearchParams()

  const [loginID, setLoginID] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)

  /**
   * redirectTo はログインが成功した後に進む先。
   *
   * navigate() を呼ぶのではなく状態に持つ。ログインが成功すると
   * auth.status も authenticated になるため、命令的に呼ぶと
   * __下の「ログイン済みならトップへ」と競り、先に描画された方が勝つ。__
   * QRから来た人がトップへ放り出されるのはこれが原因になる。
   */
  const [redirectTo, setRedirectTo] = useState('')

  // 自分のログインで得た行き先を先に見る。順序を逆にすると常にトップへ飛ぶ。
  if (redirectTo !== '') {
    return <Navigate to={redirectTo} replace />
  }

  // 確認が終わる前にフォームを出さない。出すと、ログイン済みの人に
  // 一瞬フォームが見えてから消えることになる。
  if (auth.status === 'loading') {
    return <main className="mx-auto max-w-screen-sm p-4">確認しています…</main>
  }

  // 元からログイン済みなら用がない。next は使わない。ログイン済みの利用者が
  // QRを読んだ場合は /login を通らず目的のページへ直接来る。
  if (auth.status === 'authenticated') {
    return <Navigate to="/" replace />
  }

  const handleSubmit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    setError('')
    setSubmitting(true)

    try {
      setRedirectTo(await auth.login(loginID, password, params.get('next') ?? ''))
    } catch (err) {
      setError(errorMessage(err))
      // 成功した時は画面ごと入れ替わるため、ここでだけ戻す。
      setSubmitting(false)
    }
  }

  return (
    <main className="mx-auto max-w-screen-sm p-4">
      <h1 className="text-xl font-bold">ログイン</h1>

      <form className="mt-4 space-y-4" onSubmit={(e) => void handleSubmit(e)}>
        <div>
          <label className="block text-sm font-medium" htmlFor="login-id">
            ログインID
          </label>
          <input
            id="login-id"
            name="login_id"
            // ブラウザとパスワード管理に「これは利用者名」と伝える。
            // 無いと、スマートフォンで毎回手入力になる。
            autoComplete="username"
            // スマートフォンの既定では先頭が大文字になり、自動修正もかかる。
            // ログインIDは英数字なので、どちらも邪魔にしかならない。
            autoCapitalize="none"
            autoCorrect="off"
            spellCheck={false}
            required
            // text-base（16px）未満だと iOS が焦点を当てた瞬間に拡大する。
            className="mt-1 w-full rounded border border-gray-300 px-3 py-2 text-base"
            value={loginID}
            onChange={(e) => setLoginID(e.target.value)}
          />
        </div>

        <div>
          <label className="block text-sm font-medium" htmlFor="password">
            パスワード
          </label>
          <input
            id="password"
            name="password"
            type="password"
            autoComplete="current-password"
            required
            className="mt-1 w-full rounded border border-gray-300 px-3 py-2 text-base"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </div>

        {error !== '' && (
          // role="alert" を付ける。読み上げでも、送信後に何が起きたかが分かる。
          <p role="alert" className="text-sm text-red-700">
            {error}
          </p>
        )}

        <button
          type="submit"
          // 二重送信を止める。失敗回数で待たされる仕組みがあるため、
          // 連打すると自分で自分を待たせることになる。
          disabled={submitting}
          className="w-full rounded bg-blue-700 px-4 py-3 text-base text-white disabled:bg-gray-400"
        >
          {submitting ? 'ログインしています…' : 'ログイン'}
        </button>
      </form>

      {/* 自己リセットは作らない（メールに依存するため。m1-spec §3）。
          導線が無いこと自体を伝えないと、無い機能を探させることになる。 */}
      <p className="mt-6 text-sm text-gray-600">
        パスワードが分からない場合は、運営に再発行を依頼してください。
      </p>
    </main>
  )
}
