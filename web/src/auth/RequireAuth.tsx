import type { ReactNode } from 'react'
import { Navigate, useLocation } from 'react-router'

import { Button } from '../ui/Button'
import { Loading, Notice } from '../ui/Feedback'
import { Screen } from '../ui/Screen'
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
    return (
      <Screen title="備品管理">
        <Loading>確認しています…</Loading>
      </Screen>
    )
  }

  if (auth.status === 'unavailable') {
    return (
      <Screen title="接続できません">
        <Notice>
          サーバに接続できません。
          <span className="mt-1 block text-[13px] text-label-3">{auth.message}</span>
        </Notice>

        <div className="mt-6">
          <Button full tone="tinted" onClick={() => void auth.reload()}>
            再試行
          </Button>
        </div>
      </Screen>
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
  //
  // ここでも元いた場所を next で持たせる。持たせないと、初期パスワードの
  // ままの人がQRを読んだ時に、変更を終えた瞬間トップへ出ることになる。
  if (auth.user.must_change_password && location.pathname !== passwordPath) {
    const next = encodeURIComponent(location.pathname + location.search)
    return <Navigate to={`${passwordPath}?next=${next}`} replace />
  }

  return <>{children}</>
}

/** passwordPath はパスワード変更画面。サーバの redirect_to と同じ値。 */
const passwordPath = '/password'
