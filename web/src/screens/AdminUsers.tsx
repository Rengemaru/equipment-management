import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'

import { errorMessage } from '../api/client'
import type { AdminUser, Role, UserWithPassword } from '../api/types'
import { createUser, listUsers, resetPassword, setUserActive } from '../api/users'
import { useAuth } from '../auth/AuthProvider'
import { Button } from '../ui/Button'
import { Alert, Badge, Loading } from '../ui/Feedback'
import { Field, FieldGroup, SelectField } from '../ui/Field'
import { Screen } from '../ui/Screen'

/**
 * AdminUsers は利用者の管理画面（運営のみ）。
 *
 * 利用者は消せない。卒業者は無効化する。消すと貸出履歴が壊れる（CLAUDE.md）。
 * 最後の admin は無効化できない（サーバが拒む）。
 */
export default function AdminUsers() {
  const auth = useAuth()
  const me = auth.status === 'authenticated' ? auth.user.id : 0

  const [users, setUsers] = useState<AdminUser[] | null>(null)
  const [error, setError] = useState('')
  const [adding, setAdding] = useState(false)

  /**
   * issued は発行された初期パスワード。
   *
   * __この画面から消すと二度と表示できない。__ DBにはハッシュしか無い。
   * 控えたと運営が明示するまで消さない。
   */
  const [issued, setIssued] = useState<UserWithPassword | null>(null)

  useEffect(() => {
    let alive = true
    void listUsers().then(
      (list) => {
        if (alive) setUsers(list)
      },
      (err: unknown) => {
        if (alive) setError(errorMessage(err))
      },
    )
    return () => {
      alive = false
    }
  }, [])

  /** replaceUser は1人だけ差し替える。一覧を取り直さない。 */
  const replaceUser = (updated: AdminUser) => {
    setUsers((prev) => prev?.map((u) => (u.id === updated.id ? updated : u)) ?? null)
  }

  const toggleActive = async (user: AdminUser) => {
    setError('')
    try {
      replaceUser(await setUserActive(user.id, !user.is_active))
    } catch (err) {
      // 最後の admin を無効化しようとした場合もここに来る（409）。
      setError(errorMessage(err))
    }
  }

  const reissue = async (user: AdminUser) => {
    setError('')
    try {
      const res = await resetPassword(user.id)
      replaceUser(res.user)
      setIssued(res)
    } catch (err) {
      setError(errorMessage(err))
    }
  }

  return (
    <Screen title="ユーザー管理" back={{ to: '/', label: 'トップ' }}>
      {issued !== null && <IssuedPassword issued={issued} onDismiss={() => setIssued(null)} />}

      {adding ? (
        <NewUserForm
          onCreated={(res) => {
            setUsers((prev) => (prev === null ? [res.user] : [...prev, res.user]))
            setIssued(res)
            setAdding(false)
          }}
          onCancel={() => setAdding(false)}
        />
      ) : (
        <div className="mt-6">
          <Button
            full
            tone="tinted"
            onClick={() => {
              setError('')
              setAdding(true)
            }}
          >
            ユーザーを追加
          </Button>
        </div>
      )}

      {error !== '' && <Alert>{error}</Alert>}

      {users === null && error === '' && <Loading />}

      {users !== null && (
        <ul className="mt-6 space-y-3">
          {users.map((u) => (
            <li
              key={u.id}
              className={`overflow-hidden rounded-group bg-card ${u.is_active ? '' : 'opacity-60'}`}
            >
              <div className="px-4 py-3">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-[17px]">{u.name}</span>
                  {u.id === me && <span className="text-[13px] text-label-3">（自分）</span>}
                </div>

                <p className="mt-0.5 font-mono text-[13px] text-label-2">{u.login_id}</p>

                <div className="mt-1.5 flex flex-wrap gap-1">
                  {u.role === 'admin' && <Badge tone="info">運営</Badge>}
                  {!u.is_active && <Badge tone="warn">無効</Badge>}
                  {/* まだ一度も使っていない目安になる。渡し忘れに気付ける。 */}
                  {u.must_change_password && <Badge tone="warn">初期パスワードのまま</Badge>}
                </div>
              </div>

              {/* __削除の導線は置かない。__ 卒業者は無効化する。
                  削除すると貸出履歴の参照先が消える（CLAUDE.md）。 */}
              <div className="flex border-t border-separator">
                <button
                  className={`min-h-11 flex-1 text-[17px] active:bg-fill ${
                    u.is_active ? 'text-danger' : 'text-tint'
                  }`}
                  onClick={() => void toggleActive(u)}
                >
                  {u.is_active ? '無効にする' : '有効にする'}
                </button>
                <button
                  className="min-h-11 flex-1 border-l border-separator text-[17px] text-tint active:bg-fill"
                  onClick={() => void reissue(u)}
                >
                  パスワードを再発行
                </button>
              </div>
            </li>
          ))}
        </ul>
      )}
    </Screen>
  )
}

/**
 * IssuedPassword は発行された初期パスワードを見せる。
 *
 * __この応答でしか手に入らない。__ 閉じる操作を明示的にして、
 * 画面を切り替えた拍子に消えないようにする。
 */
function IssuedPassword({
  issued,
  onDismiss,
}: {
  issued: UserWithPassword
  onDismiss: () => void
}) {
  return (
    <section
      role="status"
      className="mt-6 overflow-hidden rounded-group border-2 border-tint bg-card"
    >
      <div className="px-4 py-4">
        <p className="text-[13px] text-label-2">
          {issued.user.name}（<span className="font-mono">{issued.user.login_id}</span>
          ）の初期パスワード
        </p>

        {/*
          select-all にしておくと、1回触れば全体が選ばれる。
          __クリップボードAPIは使わない。__ HTTP運用だと動かず、押しても何も
          起きないボタンになる（COOKIE_SECURE を落として使う想定がある）。
        */}
        <p className="mt-1.5 font-mono text-[28px] leading-tight break-all select-all">
          {issued.initial_password}
        </p>

        <p className="mt-2 text-[13px] leading-snug text-danger">
          この表示を閉じると二度と確認できません。本人に渡してください。
        </p>
      </div>

      <div className="border-t border-separator">
        <button
          className="min-h-11 w-full text-[17px] font-semibold text-tint active:bg-fill"
          onClick={onDismiss}
        >
          控えました
        </button>
      </div>
    </section>
  )
}

function NewUserForm({
  onCreated,
  onCancel,
}: {
  onCreated: (res: UserWithPassword) => void
  onCancel: () => void
}) {
  const [name, setName] = useState('')
  const [loginID, setLoginID] = useState('')
  const [email, setEmail] = useState('')
  const [role, setRole] = useState<Role>('member')
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)

  const handleSubmit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    setError('')
    setSaving(true)

    try {
      onCreated(await createUser({ name, loginID, email, role }))
    } catch (err) {
      setError(errorMessage(err))
      setSaving(false)
    }
  }

  return (
    <form onSubmit={(e) => void handleSubmit(e)}>
      <FieldGroup
        header="ユーザーを追加"
        footer="ログインIDは英数字・ハイフン・アンダースコア。本人が打ちやすいものにしてください。"
      >
        <Field
          id="name"
          label="名前"
          required
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
        <Field
          id="login-id"
          label="ログインID"
          required
          // 大文字小文字は区別されない（サーバが小文字に寄せる）が、
          // 自動大文字化と自動修正は切る。英数字の入力に邪魔でしかない。
          autoCapitalize="none"
          autoCorrect="off"
          spellCheck={false}
          value={loginID}
          onChange={(e) => setLoginID(e.target.value)}
        />
      </FieldGroup>

      {/* 認証には使わない。入れなくても運用できることを書いておかないと、
          全員分を集める作業が発生する。 */}
      <FieldGroup footer="メールは通知にだけ使います。ログインには使いません。空欄で構いません。">
        <Field
          id="email"
          label="メール"
          type="email"
          placeholder="任意"
          autoCapitalize="none"
          autoCorrect="off"
          spellCheck={false}
          value={email}
          onChange={(e) => setEmail(e.target.value)}
        />
      </FieldGroup>

      <FieldGroup footer="パスワードは自動で発行され、この後に一度だけ表示されます。">
        <SelectField
          id="role"
          label="権限"
          value={role}
          onChange={(e) => setRole(e.target.value as Role)}
        >
          <option value="member">メンバー</option>
          <option value="admin">運営</option>
        </SelectField>
      </FieldGroup>

      {error !== '' && <Alert>{error}</Alert>}

      <div className="mt-6 space-y-3">
        <Button type="submit" full disabled={saving}>
          {saving ? '追加しています…' : '追加する'}
        </Button>
        <Button type="button" tone="plain" full onClick={onCancel}>
          キャンセル
        </Button>
      </div>
    </form>
  )
}
