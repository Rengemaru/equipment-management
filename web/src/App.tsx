import { Link, Route, Routes } from 'react-router'

import { useAuth } from './auth/AuthProvider'
import { RequireAdmin } from './auth/RequireAdmin'
import { RequireAuth } from './auth/RequireAuth'
import AdminItems from './screens/AdminItems'
import ItemDetail from './screens/ItemDetail'
import Items from './screens/Items'
import Login from './screens/Login'
import PasswordChange from './screens/PasswordChange'

/**
 * App は画面の割り当てだけを持つ。
 *
 * 画面はここに1つずつ足していく。押せないボタンやダミー画面は置かない
 * （CLAUDE.md）。未実装の画面へのリンクを先に置くと、触った人には
 * 「壊れている」としか見えない。
 *
 * ログインが要る画面は RequireAuth で包む。包み忘れた画面は、APIが401を
 * 返すまで中身を出し続ける。__新しい画面を足す時は必ず判断すること。__
 */
export default function App() {
  return (
    <Routes>
      {/* ログイン画面だけは包まない。包むと自分自身へ送り続ける。 */}
      <Route path="/login" element={<Login />} />

      <Route
        path="/password"
        element={
          <RequireAuth>
            <PasswordChange />
          </RequireAuth>
        }
      />

      <Route
        path="/items"
        element={
          <RequireAuth>
            <Items />
          </RequireAuth>
        }
      />

      {/* QRの遷移先。__この経路は変えられない。__ ラベルは貼り替えられない
          ため、URLを変えると印刷済みのQRが全て読めなくなる（url-design.md §1）。 */}
      <Route
        path="/i/:code"
        element={
          <RequireAuth>
            <ItemDetail />
          </RequireAuth>
        }
      />

      <Route
        path="/"
        element={
          <RequireAuth>
            <Home />
          </RequireAuth>
        }
      />

      {/* 運営のみ。RequireAdmin は中で RequireAuth を通す。 */}
      <Route
        path="/admin/items"
        element={
          <RequireAdmin>
            <AdminItems />
          </RequireAdmin>
        }
      />

      {/* 知らないURLで白い画面を出さない。 */}
      <Route path="*" element={<NotFound />} />
    </Routes>
  )
}

/**
 * Home はトップ。M1では備品一覧へのリンクだけを置く（m1-spec §7）。
 *
 * 貸出中一覧や自分の貸出はM2で作る。先に置くと、押せない項目が並ぶ。
 */
function Home() {
  const auth = useAuth()
  const isAdmin = auth.status === 'authenticated' && auth.user.role === 'admin'

  return (
    <main className="mx-auto max-w-screen-sm p-4">
      <h1 className="text-xl font-bold">備品管理</h1>

      <ul className="mt-4 space-y-2">
        <li>
          <Link className="text-blue-700 underline" to="/items">
            備品一覧
          </Link>
        </li>

        {/* 運営の画面は運営にだけ出す。member に出すと、押した先で
            「権限がありません」に当たるだけになる。 */}
        {isAdmin && (
          <li>
            <Link className="text-blue-700 underline" to="/admin/items">
              備品マスタ管理
            </Link>
          </li>
        )}
      </ul>
    </main>
  )
}

function NotFound() {
  return (
    <main className="mx-auto max-w-screen-sm p-4">
      <h1 className="text-xl font-bold">ページが見つかりません</h1>
      <p className="mt-2 text-sm text-gray-600">
        QRの読み取りに失敗したか、URLが変わった可能性があります。
      </p>
      <Link className="mt-4 inline-block text-blue-700 underline" to="/">
        トップへ
      </Link>
    </main>
  )
}
