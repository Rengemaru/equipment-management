import { useState } from 'react'

import { useAuth } from './AuthProvider'

/**
 * useLogout はログアウトの操作をまとめる。
 *
 * 見た目は置き場所ごとに違う（狭い画面ではリストの行、広い画面では
 * サイドバーの下）が、__挙動は1箇所に持つ。__ 2つ書くと、片方だけに
 * 確認が付いた状態がいつか生まれる。
 *
 * # なぜ2段階にするのか
 *
 * 確認を安易に増やさないのがこのプロジェクトの方針だが、ログアウトは例外。
 * セッションは1年もつので、__押し間違えた人はその場では戻れない。__
 * パスワードを覚えていなければ運営に再発行を頼むことになり、
 * その間その人は借用を記録できない。1タップの確認はその代償より軽い。
 *
 * ダイアログ（confirm）は使わない。画面を覆って操作を止めるほどのことではなく、
 * 押した場所のすぐそこで聞けば足りる。
 */
export function useLogout() {
  const auth = useAuth()

  const [confirming, setConfirming] = useState(false)
  const [busy, setBusy] = useState(false)

  return {
    /** confirming が true の間は、確認の文言と2つの選択肢を出す。 */
    confirming,
    busy,

    /** ask は確認を出す。 */
    ask: () => setConfirming(true),

    /** cancel は確認をやめる。 */
    cancel: () => setConfirming(false),

    /**
     * run は実際にログアウトする。
     *
     * 画面遷移はしない。ログアウトすると auth の状態が anonymous になり、
     * RequireAuth がログイン画面へ送る。__ここで navigate すると、__
     * __その判断が2箇所になる。__
     */
    run: async () => {
      setBusy(true)
      await auth.logout()
    },
  }
}
