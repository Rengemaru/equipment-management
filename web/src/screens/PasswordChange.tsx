import { useState } from 'react'
import type { FormEvent } from 'react'
import { useNavigate, useSearchParams } from 'react-router'

import { errorMessage } from '../api/client'
import { useAuth } from '../auth/AuthProvider'
import { useLogout } from '../auth/useLogout'
import { Button } from '../ui/Button'
import { Alert, Notice } from '../ui/Feedback'
import { FieldGroup, StackedField } from '../ui/Field'
import { ButtonRow, List } from '../ui/List'
import { Screen } from '../ui/Screen'

/**
 * PasswordChange はパスワード変更画面。
 *
 * RequireAuth の内側でのみ使う。初期パスワードのままの利用者は、
 * ここを終えるまで他の画面へ進めない。
 *
 * 変更すると他の端末のセッションは全て切れる。この端末はサーバが
 * 繋ぎ直すため、ログインし直す必要はない。
 *
 * `?next=` は Login と同じく __読むだけで解釈しない。__ 進む先は応答の
 * redirect_to で返る。QRから来た新入部員をここで止めたまま終わらせると、
 * 変更後にトップへ出てQRを読み直すことになる。
 */
export default function PasswordChange() {
  const auth = useAuth()
  const navigate = useNavigate()
  const [params] = useSearchParams()

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
      const redirectTo = await auth.changePassword(current, next, params.get('next') ?? '')
      navigate(redirectTo, { replace: true })
    } catch (err) {
      setError(errorMessage(err))
      setSubmitting(false)
    }
  }

  return (
    <Screen
      title="パスワードの変更"
      narrow
      // 強制されて来た人には戻る先が無い。ここを出ることは許されていない。
      back={forced ? undefined : { to: '/', label: 'トップ' }}
    >
      {forced && (
        <Notice tone="warn">初期パスワードのままです。変更するまで他の画面は使えません。</Notice>
      )}

      <form onSubmit={(e) => void handleSubmit(e)}>
        <FieldGroup>
          <StackedField
            id="current-password"
            label="現在のパスワード"
            name="current_password"
            type="password"
            autoComplete="current-password"
            required
            value={current}
            onChange={(e) => setCurrent(e.target.value)}
          />
        </FieldGroup>

        <FieldGroup footer="他の端末でログインしたままの場合、変更すると全て切れます。この端末はそのまま使えます。">
          <StackedField
            id="new-password"
            label="新しいパスワード"
            name="new_password"
            type="password"
            autoComplete="new-password"
            required
            value={next}
            onChange={(e) => setNext(e.target.value)}
          />

          <StackedField
            id="confirm-password"
            label="新しいパスワード（確認）"
            name="confirm_password"
            type="password"
            autoComplete="new-password"
            required
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
          />
        </FieldGroup>

        {error !== '' && <Alert>{error}</Alert>}

        <div className="mt-6">
          <Button type="submit" full disabled={submitting}>
            {submitting ? '変更しています…' : '変更する'}
          </Button>
        </div>
      </form>

      {/* 強制されて来た人は他の画面へ進めず、サイドバーも出ない。
          ここにログアウトが無いと、__その端末は誰も抜けられなくなる。__
          部室の共用PCで、初期パスワードのまま放置された他人のセッションが
          残った場合、次の人は現在のパスワードを知らないので変更もできない。
          サーバも同じ考えで、初期パスワードのままでも /api/logout だけは
          通している（internal/auth/middleware.go の passwordChangeExempt）。 */}
      {forced && <LeaveWithoutChanging />}
    </Screen>
  )
}

/** LeaveWithoutChanging は変更せずに離脱する手段。 */
function LeaveWithoutChanging() {
  const logout = useLogout()

  if (logout.confirming) {
    return (
      <List footer="ログアウトすると、次に使う時にもう一度パスワードが要ります。">
        <ButtonRow tone="danger" disabled={logout.busy} onClick={() => void logout.run()}>
          {logout.busy ? 'ログアウトしています…' : '本当にログアウトする'}
        </ButtonRow>
        <ButtonRow disabled={logout.busy} onClick={logout.cancel}>
          やめる
        </ButtonRow>
      </List>
    )
  }

  return (
    <List footer="自分のアカウントでない場合は、ログアウトしてから使ってください。">
      <ButtonRow tone="danger" onClick={logout.ask}>
        ログアウト
      </ButtonRow>
    </List>
  )
}
