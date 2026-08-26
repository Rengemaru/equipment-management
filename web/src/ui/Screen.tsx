import type { ReactNode } from 'react'
import { Link } from 'react-router'

/**
 * Screen は全画面に共通の枠。
 *
 * ナビゲーションバー（戻る）と大タイトルを持つ。__画面ごとに枠を書かない。__
 * 書かせると、余白や戻る導線が画面ごとにずれ、同じアプリに見えなくなる。
 *
 * iOS と同じく、__バーには戻り先の名前だけを出し、今いる画面の名前は__
 * __その下に大きく置く。__ 両方に同じ文字を出すと、狭い画面で二重に場所を取る。
 */
export function Screen({
  title,
  back,
  subtitle,
  actions,
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
  children: ReactNode
}) {
  return (
    <div className="min-h-dvh">
      <NavBar back={back} />

      {/*
        pb は下に余裕を持たせる。iPhone のホームバーの上で操作すると
        誤って画面を閉じることがあり、最後の項目が指の下に来ないようにする。
      */}
      <main className="mx-auto max-w-screen-sm px-4 pb-[max(3rem,env(safe-area-inset-bottom))]">
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
  )
}

/**
 * NavBar は上端に貼り付く帯。
 *
 * 半透明にして下の内容を透かす。iOS の標準がそうであるだけでなく、
 * スクロールしていることが分かる手がかりになる。
 */
function NavBar({ back }: { back?: { to: string; label: string } }) {
  return (
    <div className="sticky top-0 z-10 bg-nav backdrop-blur-xl">
      {/* ノッチの下に潜り込ませない。 */}
      <div className="h-[env(safe-area-inset-top)]" />

      <div className="mx-auto flex h-11 max-w-screen-sm items-center px-2">
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
