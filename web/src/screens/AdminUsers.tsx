import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { Link } from 'react-router'

import { errorMessage } from '../api/client'
import type { AdminUser, Role, UserWithPassword } from '../api/types'
import { createUser, listUsers, resetPassword, setUserActive } from '../api/users'
import { useAuth } from '../auth/AuthProvider'

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
    <main className="mx-auto max-w-screen-sm p-4">
      <h1 className="text-xl font-bold">ユーザー管理</h1>

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
        <button
          className="mt-4 rounded bg-blue-700 px-4 py-2 text-white"
          onClick={() => {
            setError('')
            setAdding(true)
          }}
        >
          ユーザーを追加
        </button>
      )}

      {error !== '' && (
        <p role="alert" className="mt-4 text-sm text-red-700">
          {error}
        </p>
      )}

      {users === null && error === '' && <p className="mt-4 text-sm text-gray-600">読み込み中…</p>}

      {users !== null && (
        <ul className="mt-4 divide-y divide-gray-200 border-y border-gray-200">
          {users.map((u) => (
            <li key={u.id} className={`py-3 ${u.is_active ? '' : 'opacity-60'}`}>
              <div className="flex flex-wrap items-baseline gap-2">
                <span className="font-medium">{u.name}</span>
                <span className="font-mono text-sm text-gray-600">{u.login_id}</span>
                {u.id === me && <span className="text-xs text-gray-600">（自分）</span>}
              </div>

              <div className="mt-1 flex flex-wrap gap-1">
                {u.role === 'admin' && <Badge tone="info">運営</Badge>}
                {!u.is_active && <Badge tone="warn">無効</Badge>}
                {/* まだ一度も使っていない目安になる。渡し忘れに気付ける。 */}
                {u.must_change_password && <Badge tone="warn">初期パスワードのまま</Badge>}
              </div>

              <div className="mt-2 flex flex-wrap gap-2">
                <button
                  className="rounded border border-gray-300 px-3 py-1 text-sm"
                  onClick={() => void toggleActive(u)}
                >
                  {u.is_active ? '無効にする' : '有効にする'}
                </button>
                <button
                  className="rounded border border-gray-300 px-3 py-1 text-sm"
                  onClick={() => void reissue(u)}
                >
                  パスワードを再発行
                </button>
              </div>
            </li>
          ))}
        </ul>
      )}

      <div className="mt-6">
        <Link className="text-blue-700 underline" to="/admin/items">
          マスタ管理へ
        </Link>
      </div>
    </main>
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
    <section role="status" className="mt-4 rounded border-2 border-blue-700 p-3">
      <p className="text-sm">
        {issued.user.name}（<span className="font-mono">{issued.user.login_id}</span>）の初期パスワード
      </p>

      {/*
        select-all にしておくと、1回触れば全体が選ばれる。
        クリップボードAPIは使わない。HTTP運用だと動かず、押しても何も
        起きないボタンになる（COOKIE_SECURE を落として使う想定がある）。
      */}
      <p className="mt-1 select-all break-all font-mono text-2xl">{issued.initial_password}</p>

      <p className="mt-2 text-sm text-red-700">
        この表示を閉じると二度と確認できません。本人に渡してください。
      </p>

      <button className="mt-2 rounded bg-blue-700 px-4 py-2 text-white" onClick={onDismiss}>
        控えました
      </button>
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
    <form
      className="mt-4 space-y-3 rounded border border-gray-300 p-3"
      onSubmit={(e) => void handleSubmit(e)}
    >
      <h2 className="font-bold">ユーザーを追加</h2>

      <div>
        <label className="block text-sm font-medium" htmlFor="name">
          名前
        </label>
        <input
          id="name"
          required
          // text-base（16px）未満だと iOS が焦点を当てた瞬間に拡大する。
          className="mt-1 w-full rounded border border-gray-300 px-3 py-2 text-base"
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
      </div>

      <div>
        <label className="block text-sm font-medium" htmlFor="login-id">
          ログインID
        </label>
        <input
          id="login-id"
          required
          // 大文字小文字は区別されない（サーバが小文字に寄せる）が、
          // 自動大文字化と自動修正は切る。英数字の入力に邪魔でしかない。
          autoCapitalize="none"
          autoCorrect="off"
          spellCheck={false}
          className="mt-1 w-full rounded border border-gray-300 px-3 py-2 text-base"
          value={loginID}
          onChange={(e) => setLoginID(e.target.value)}
        />
        <p className="mt-1 text-xs text-gray-600">
          英数字・ハイフン・アンダースコア。本人が打ちやすいものにしてください。
        </p>
      </div>

      <div>
        <label className="block text-sm font-medium" htmlFor="email">
          メールアドレス（任意）
        </label>
        <input
          id="email"
          type="email"
          autoCapitalize="none"
          autoCorrect="off"
          spellCheck={false}
          className="mt-1 w-full rounded border border-gray-300 px-3 py-2 text-base"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
        />
        {/* 認証には使わない。入れなくても運用できることを書いておかないと、
            全員分を集める作業が発生する。 */}
        <p className="mt-1 text-xs text-gray-600">
          通知にだけ使います。ログインには使いません。空欄で構いません。
        </p>
      </div>

      <div>
        <label className="block text-sm font-medium" htmlFor="role">
          権限
        </label>
        <select
          id="role"
          className="mt-1 w-full rounded border border-gray-300 px-2 py-2 text-base"
          value={role}
          onChange={(e) => setRole(e.target.value as Role)}
        >
          <option value="member">メンバー</option>
          <option value="admin">運営</option>
        </select>
      </div>

      <p className="text-xs text-gray-600">
        パスワードは自動で発行され、この後に一度だけ表示されます。
      </p>

      {error !== '' && (
        <p role="alert" className="text-sm text-red-700">
          {error}
        </p>
      )}

      <div className="flex gap-2">
        <button
          type="submit"
          disabled={saving}
          className="rounded bg-blue-700 px-4 py-2 text-white disabled:bg-gray-400"
        >
          {saving ? '追加しています…' : '追加する'}
        </button>
        <button type="button" className="rounded border border-gray-300 px-4 py-2" onClick={onCancel}>
          キャンセル
        </button>
      </div>
    </form>
  )
}

function Badge({ tone, children }: { tone: 'warn' | 'info'; children: string }) {
  const color = tone === 'warn' ? 'bg-amber-100 text-amber-900' : 'bg-gray-100 text-gray-700'
  return <span className={`rounded px-2 py-0.5 text-xs ${color}`}>{children}</span>
}
