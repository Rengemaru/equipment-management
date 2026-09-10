import { Route, Routes } from 'react-router'

import { useAuth } from './auth/AuthProvider'
import { RequireAdmin } from './auth/RequireAdmin'
import { RequireAuth } from './auth/RequireAuth'
import { useLogout } from './auth/useLogout'
import AdminDamages from './screens/AdminDamages'
import AdminItemNew from './screens/AdminItemNew'
import AdminItems from './screens/AdminItems'
import AdminItemsImport from './screens/AdminItemsImport'
import AdminLabels from './screens/AdminLabels'
import AdminUsers from './screens/AdminUsers'
import ItemBorrow from './screens/ItemBorrow'
import ItemDetail from './screens/ItemDetail'
import Items from './screens/Items'
import Loans from './screens/Loans'
import Login from './screens/Login'
import MyLoans from './screens/MyLoans'
import PasswordChange from './screens/PasswordChange'
import { Empty } from './ui/Feedback'
import { ButtonRow, LinkRow, List } from './ui/List'
import { Screen } from './ui/Screen'

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

      {/* 貸出中一覧は全メンバーが見られる。誰が何を持っているかが
          全員に見える状態を作ることが、罰則より強く働く（CLAUDE.md）。 */}
      <Route
        path="/loans"
        element={
          <RequireAuth>
            <Loans />
          </RequireAuth>
        }
      />

      {/* 代理登録の通知メールが指す先。身に覚えのない借用をここで取り消す。 */}
      <Route
        path="/loans/mine"
        element={
          <RequireAuth>
            <MyLoans />
          </RequireAuth>
        }
      />

      {/* 借用の確認画面。詳細の「借りる」から来る。
          __直接開いても成立させる。__ 借りようとしてブラウザを再読み込みした
          人が、そこで止まらないようにする。 */}
      <Route
        path="/i/:code/borrow"
        element={
          <RequireAuth>
            <ItemBorrow />
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

      <Route
        path="/admin/items/new"
        element={
          <RequireAdmin>
            <AdminItemNew />
          </RequireAdmin>
        }
      />

      <Route
        path="/admin/items/import"
        element={
          <RequireAdmin>
            <AdminItemsImport />
          </RequireAdmin>
        }
      />

      <Route
        path="/admin/labels"
        element={
          <RequireAdmin>
            <AdminLabels />
          </RequireAdmin>
        }
      />

      <Route
        path="/admin/users"
        element={
          <RequireAdmin>
            <AdminUsers />
          </RequireAdmin>
        }
      />

      {/* 破損報告の追認。__承認ではない。__ 報告は既に反映済みで、
          ここでの操作は事後の確認と処理の記録。 */}
      <Route
        path="/admin/damages"
        element={
          <RequireAdmin>
            <AdminDamages />
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
  const name = auth.status === 'authenticated' ? auth.user.name : ''

  return (
    <Screen title="備品管理" subtitle={name === '' ? undefined : `${name} さん`}>
      {/*
        広い画面では同じ行き先がサイドバーに出ている。__ここにも並べると、__
        __同じリンクが2箇所に出て、どちらを押せばよいのか分からなくなる。__
        狭い画面ではサイドバーが無いので、こちらが唯一の入口になる。
      */}
      <div className="lg:hidden">
        <List>
          {/* 自分の貸出を先頭に置く。__返すのは借りた人だけができる操作__で、
              この画面に来る動機として一番多い。 */}
          <LinkRow to="/loans/mine">
            <span className="text-[17px]">自分が借りているもの</span>
          </LinkRow>
          <LinkRow to="/loans">
            <span className="text-[17px]">貸出中の一覧</span>
          </LinkRow>
          <LinkRow to="/items">
            <span className="text-[17px]">備品一覧</span>
          </LinkRow>
        </List>

        {/* 事後登録の入口。__「もう持ち出しちゃったから今さら」を潰す__のが目的で、
            深い階層に埋めると意味が無くなる（m2-spec §4）。備品を選べば、
            借用画面で借用日時を過去にできる。 */}
        <List footer="持ち出した後でも、備品を選べば借用日時を遡って記録できます。">
          <LinkRow to="/items">
            <span className="text-[17px]">持ち出したものを後から記録する</span>
          </LinkRow>
        </List>

        {/* 運営の画面は運営にだけ出す。member に出すと、押した先で
            「権限がありません」に当たるだけになる。 */}
        {isAdmin && (
          <List header="運営">
            <LinkRow to="/admin/items">
              <span className="text-[17px]">備品マスタ管理</span>
            </LinkRow>
            <LinkRow to="/admin/users">
              <span className="text-[17px]">ユーザー管理</span>
            </LinkRow>
            <LinkRow to="/admin/damages">
              <span className="text-[17px]">破損報告</span>
            </LinkRow>
          </List>
        )}

        <List>
          <LinkRow to="/password">
            <span className="text-[17px]">パスワードの変更</span>
          </LinkRow>
        </List>

        <LogoutList />
      </div>

      <p className="mt-6 hidden px-4 text-[15px] leading-relaxed text-label-2 lg:block">
        左の一覧から選んでください。
        <span className="mt-1 block text-[13px]">
          棚に貼ったQRを読むと、その備品の画面が直接開きます。
        </span>
      </p>
    </Screen>
  )
}

/**
 * LogoutList はトップの一番下に置くログアウト。
 *
 * 一番下に置くのは iOS の設定画面と同じ。__よく押すものの隣に置かない。__
 * セッションは1年もつので、押し間違えるとその場では戻れない。
 */
function LogoutList() {
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
    <List>
      <ButtonRow tone="danger" onClick={logout.ask}>
        ログアウト
      </ButtonRow>
    </List>
  )
}

function NotFound() {
  return (
    <Screen title="ページが見つかりません" back={{ to: '/', label: 'トップ' }}>
      <Empty
        title="そのURLの画面はありません"
        hint="QRの読み取りに失敗したか、URLが変わった可能性があります。"
      />
    </Screen>
  )
}
