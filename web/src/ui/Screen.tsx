import { useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import { Link, NavLink } from 'react-router'

import { useAuth } from '../auth/AuthProvider'
import { useLogout } from '../auth/useLogout'

/**
 * Screen は全画面に共通の枠。
 *
 * ナビゲーションバー（戻る）と大タイトルを持つ。__画面ごとに枠を書かない。__
 * 書かせると、余白や戻る導線が画面ごとにずれ、同じアプリに見えなくなる。
 *
 * iOS と同じく、__バーには戻り先の名前だけを出し、今いる画面の名前は__
 * __その下に大きく置く。__ 両方に同じ文字を出すと、狭い画面で二重に場所を取る。
 *
 * # 広い画面
 *
 * 1024px 以上では iPadOS と同じ二段組にする。左に行き先の一覧を出し、
 * 右に中身を置く。__スマートフォンの1列をそのまま引き伸ばさない。__
 * 引き伸ばすと1行が長くなりすぎて読みにくく、真ん中に細い列が浮くだけの
 * 画面は「作りかけ」に見える。
 */
export function Screen({
  title,
  back,
  subtitle,
  actions,
  narrow = false,
  children,
}: {
  /** title は大タイトル。h1 になる。 */
  title: string
  /** back は戻り先。省略するとバーの左は空になる（トップとログイン）。 */
  back?: { to: string; label: string }
  /** subtitle は大タイトルの上に置く小さな行（備品コードなど）。 */
  subtitle?: ReactNode
  /** actions は大タイトルの右に並べるもの。 */
  actions?: ReactNode
  /**
   * narrow は広い画面でも列を広げない。
   *
   * ログインとパスワード変更に使う。__入力欄が数個しかない画面を__
   * __画面幅いっぱいに広げると、押すべきボタンが遠くなるだけで何も良くならない。__
   */
  narrow?: boolean
  children: ReactNode
}) {
  const auth = useAuth()
  const wide = useWideScreen()

  // 初期パスワードのままの人にはサイドバーを出さない。その人はこの画面を
  // 出ることを許されておらず、押せる行き先を並べると押した先で追い返される。
  const showSidebar =
    wide && auth.status === 'authenticated' && !auth.user.must_change_password

  // 列の広げ方。narrow は端末の大きさによらず読みやすい幅に留める。
  const columnWidth = narrow
    ? 'max-w-screen-sm'
    : 'max-w-screen-sm md:max-w-2xl lg:max-w-3xl xl:max-w-4xl'

  // サイドバーがある時は左寄せにする。中央に寄せると、サイドバーとの間に
  // 意味のない隙間ができ、二段組が2つの島に見える。
  const columnAlign = showSidebar ? 'lg:mx-0' : 'mx-auto'

  return (
    <div className="min-h-dvh lg:flex">
      {showSidebar && <Sidebar isAdmin={auth.user.role === 'admin'} name={auth.user.name} />}

      {/* min-w-0 が要る。付けないと、中身の長い行が伸びて二段組が崩れる。 */}
      <div className="min-w-0 flex-1">
        <NavBar back={back} width={columnWidth} align={columnAlign} />

        {/*
          pb は下に余裕を持たせる。iPhone のホームバーの上で操作すると
          誤って画面を閉じることがあり、最後の項目が指の下に来ないようにする。

          幅は段階的に広げる。読める1行の長さには上限があるので、
          画面いっぱいには広げない（macOS の設定画面と同じ考え方）。
        */}
        <main
          className={`mx-auto px-4 pb-[max(3rem,env(safe-area-inset-bottom))] lg:px-8 ${columnWidth} ${columnAlign}`}
        >
          <header className="pt-2 pb-1">
            {subtitle !== undefined && (
              <div className="text-[15px] text-label-2 tabular-nums">{subtitle}</div>
            )}

            <div className="flex items-end justify-between gap-3">
              {/* iOS の Large Title は 34px / bold / 詰め気味。 */}
              <h1 className="text-[34px] leading-tight font-bold tracking-[-0.02em]">{title}</h1>
              {actions !== undefined && <div className="shrink-0 pb-1.5">{actions}</div>}
            </div>
          </header>

          {children}
        </main>
      </div>
    </div>
  )
}

/**
 * Sidebar は広い画面の行き先の一覧。
 *
 * 出すのは __トップに並ぶものだけ。__ 画面の中にある下位の導線
 * （備品を登録・CSVで一括登録など）まで並べると、同じリンクが
 * 2箇所に出て、どちらを押せばよいのか分からなくなる。
 */
function Sidebar({ isAdmin, name }: { isAdmin: boolean; name: string }) {
  return (
    <aside className="sticky top-0 hidden h-dvh w-64 shrink-0 flex-col border-r border-separator bg-card lg:flex">
      <div className="px-4 pt-[max(1.25rem,env(safe-area-inset-top))] pb-3">
        <Link to="/" className="text-[20px] font-bold tracking-[-0.01em]">
          備品管理
        </Link>
      </div>

      <nav className="flex-1 overflow-y-auto px-2 pb-4">
        <SidebarLink to="/items">備品一覧</SidebarLink>
        <SidebarLink to="/loans">貸出中</SidebarLink>
        <SidebarLink to="/loans/mine">自分の貸出</SidebarLink>

        {/* 運営の画面は運営にだけ出す。member に出すと、押した先で
            「権限がありません」に当たるだけになる。 */}
        {isAdmin && (
          <>
            <SidebarHeading>運営</SidebarHeading>
            <SidebarLink to="/admin/items">備品マスタ管理</SidebarLink>
            <SidebarLink to="/admin/users">ユーザー管理</SidebarLink>
          </>
        )}

        <SidebarHeading>アカウント</SidebarHeading>
        <SidebarLink to="/password">パスワードの変更</SidebarLink>
      </nav>

      <SidebarFooter name={name} />
    </aside>
  )
}

/**
 * SidebarFooter は誰として使っているかと、ログアウト。
 *
 * 名前を出すのは、部室の共用PCで前の人のまま操作しないため。
 * __誰の記録として残るかが分からない状態で借用を記録させない。__
 */
function SidebarFooter({ name }: { name: string }) {
  const logout = useLogout()

  return (
    <div className="border-t border-separator px-3 py-3">
      <p className="px-1 text-[13px] text-label-2">{name} さん</p>

      {logout.confirming ? (
        <div className="mt-1.5">
          <button
            type="button"
            disabled={logout.busy}
            onClick={() => void logout.run()}
            className="flex min-h-9 w-full items-center rounded-lg px-3 text-[15px] text-danger active:bg-fill disabled:text-label-3"
          >
            {logout.busy ? 'ログアウトしています…' : '本当にログアウトする'}
          </button>
          <button
            type="button"
            disabled={logout.busy}
            onClick={logout.cancel}
            className="flex min-h-9 w-full items-center rounded-lg px-3 text-[15px] text-tint active:bg-fill disabled:text-label-3"
          >
            やめる
          </button>
        </div>
      ) : (
        <button
          type="button"
          onClick={logout.ask}
          className="mt-1.5 flex min-h-9 w-full items-center rounded-lg px-3 text-[15px] text-danger active:bg-fill"
        >
          ログアウト
        </button>
      )}
    </div>
  )
}

function SidebarHeading({ children }: { children: ReactNode }) {
  return <h2 className="px-3 pt-5 pb-1 text-[12px] font-medium text-label-2">{children}</h2>
}

/**
 * SidebarLink は行き先1つ。今いる場所は色を変えて示す。
 *
 * NavLink を使うのは、開いている画面が自分で分かるようにするため。
 * 二段組では「どれを選んでいるか」が見えないと、右側の中身が
 * どこから来たのか分からなくなる。
 */
function SidebarLink({ to, children }: { to: string; children: ReactNode }) {
  return (
    <NavLink
      to={to}
      className={({ isActive }) =>
        `flex min-h-9 items-center rounded-lg px-3 text-[15px] ${
          isActive ? 'bg-fill font-medium text-tint' : 'text-label active:bg-fill'
        }`
      }
    >
      {children}
    </NavLink>
  )
}

/**
 * NavBar は上端に貼り付く帯。
 *
 * 半透明にして下の内容を透かす。iOS の標準がそうであるだけでなく、
 * スクロールしていることが分かる手がかりになる。
 */
function NavBar({
  back,
  width,
  align,
}: {
  back?: { to: string; label: string }
  /** width と align は本文の列に合わせる。ずれると戻るが本文の左端に揃わない。 */
  width: string
  align: string
}) {
  return (
    <div className="sticky top-0 z-10 bg-nav backdrop-blur-xl">
      {/* ノッチの下に潜り込ませない。 */}
      <div className="h-[env(safe-area-inset-top)]" />

      <div className={`mx-auto flex h-11 items-center px-2 lg:px-6 ${width} ${align}`}>
        {back !== undefined && (
          // 指で押す的を44pxに近づける。文字だけを的にすると押し損ねる。
          <Link
            to={back.to}
            className="-ml-1 flex h-11 items-center gap-0.5 px-1 text-[17px] text-tint active:opacity-40"
          >
            <Chevron />
            {back.label}
          </Link>
        )}
      </div>
    </div>
  )
}

/** wideQuery は二段組にする幅。Tailwind の lg と同じ値にする。 */
const wideQuery = '(min-width: 1024px)'

/**
 * useWideScreen は二段組にしてよい幅かを返す。
 *
 * CSS だけで隠さないのは、__隠しても DOM には残るため。__ 同じ行き先が
 * サイドバーと画面の中の両方に置かれると、読み上げでは2回読まれ、
 * 「どちらを押したのか」が分からなくなる。出さない時は作らない。
 *
 * matchMedia が無い環境（テストの jsdom）では false を返す。
 * 狭い画面と同じ形になるだけで、判定の欠如が画面を壊すことはない。
 */
function useWideScreen(): boolean {
  const [wide, setWide] = useState(matchesWide)

  useEffect(() => {
    if (typeof window.matchMedia !== 'function') return

    const mq = window.matchMedia(wideQuery)
    const handle = () => setWide(mq.matches)

    handle()
    mq.addEventListener('change', handle)
    return () => mq.removeEventListener('change', handle)
  }, [])

  return wide
}

function matchesWide(): boolean {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return false
  return window.matchMedia(wideQuery).matches
}

/**
 * Chevron は戻るの山形。
 *
 * 文字の「‹」を使わない。フォントによって太さと位置が変わり、
 * Android と iOS で別物に見える。
 */
function Chevron() {
  return (
    <svg
      viewBox="0 0 12 20"
      className="h-[19px] w-[11px]"
      fill="none"
      stroke="currentColor"
      strokeWidth="2.4"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M10 1 2 10l8 9" />
    </svg>
  )
}
