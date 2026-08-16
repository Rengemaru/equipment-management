import type { ReactNode } from 'react'
import { Navigate, useLocation } from 'react-router'

import { useAuth } from './AuthProvider'

/**
 * RequireAuth はログインしていなければログイン画面へ送る。
 *
 * 画面ごとに「未ログインなら…」を書かせない。書き忘れた画面は、
 * APIが401を返すまで中身を出し続けることになる。
 *
 * サーバ側の権限確認の代わりにはならない。こちらは表示の都合だけで、
 * 「member が備品マスタを書き換えられない」ことはAPIが保証する（CLAUDE.md）。
 */
export function RequireAuth({ children }: { children: ReactNode }) {
  const auth = useAuth()
  const location = useLocation()

  if (auth.status === 'loading') {
    return <Notice>確認しています…</Notice>
  }

  if (auth.status === 'unavailable') {
    return (
      <Notice>
        <p>サーバに接続できません。</p>
        <p className="mt-1 text-sm text-gray-600">{auth.message}</p>
        <button
          className="mt-4 rounded bg-blue-700 px-4 py-2 text-white"
          onClick={() => void auth.reload()}
        >
          再試行
        </button>
      </Notice>
    )
  }

  if (auth.status === 'anonymous') {
    // 元いた場所を next で持たせる。QRから来た人をログイン後にトップへ
    // 放り出すと、もう一度QRを読み直させることになり、その一手間が
    // 記録漏れの直接原因になる（CLAUDE.md）。
    //
    // 値の安全性はサーバが判断する。ここでは組み立てるだけ。
    const next = encodeURIComponent(location.pathname + location.search)
    return <Navigate to={`/login?next=${next}`} replace />
  }

  // 初期パスワードのままなら変更画面から出さない。
  //
  // サーバも同じことをしている（`/api/me` 等以外を403で止める）。
  // こちらは、画面が「権限がありません」だけを出して行き先を示さない
  // 状態にしないため。__UIだけで縛らない。__
  if (auth.user.must_change_password && location.pathname !== passwordPath) {
    return <Navigate to={passwordPath} replace />
  }

  return <>{children}</>
}

/** passwordPath はパスワード変更画面。サーバの redirect_to と同じ値。 */
const passwordPath = '/password'

/** Notice は画面の中身の代わりに出す短い知らせ。 */
function Notice({ children }: { children: ReactNode }) {
  return <main className="mx-auto max-w-screen-sm p-4">{children}</main>
}
