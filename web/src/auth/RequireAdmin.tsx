import type { ReactNode } from 'react'
import { Link } from 'react-router'

import { useAuth } from './AuthProvider'
import { RequireAuth } from './RequireAuth'

/**
 * RequireAdmin は運営（admin）以外に画面を出さない。
 *
 * 中で RequireAuth を通す。並べて書く形にすると、いつか片方だけ書いた経路が
 * できる（サーバ側の `RequireAdmin` と同じ理由。CLAUDE.md）。
 *
 * __これは表示の都合でしかない。__ member が備品マスタを書き換えられないことは
 * APIが保証する。画面で隠すだけにしない（CLAUDE.md）。
 */
export function RequireAdmin({ children }: { children: ReactNode }) {
  return (
    <RequireAuth>
      <AdminOnly>{children}</AdminOnly>
    </RequireAuth>
  )
}

function AdminOnly({ children }: { children: ReactNode }) {
  const auth = useAuth()

  // RequireAuth を通っているので、ここに来る時点でログイン済み。
  if (auth.status !== 'authenticated') {
    return null
  }

  if (auth.user.role !== 'admin') {
    return (
      <main className="mx-auto max-w-screen-sm p-4">
        <h1 className="text-xl font-bold">この画面は運営のみが使えます</h1>
        <p className="mt-2 text-sm text-gray-600">
          備品の登録や修正が必要な場合は、運営に依頼してください。
        </p>
        <Link className="mt-4 inline-block text-blue-700 underline" to="/items">
          備品一覧へ
        </Link>
      </main>
    )
  }

  return <>{children}</>
}
