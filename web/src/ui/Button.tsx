import type { ButtonHTMLAttributes, ReactNode } from 'react'
import { Link } from 'react-router'

/**
 * tone は押した先で起きることの重さ。
 *
 * filled … その画面の主目的（ログイン・登録・確定）。1画面に1つだけ置く
 * tinted … 補助的だが押してよいもの
 * plain  … 文字だけ。並べても画面が騒がしくならない
 * danger … 取り消せないこと（無効化・廃棄）
 */
type Tone = 'filled' | 'tinted' | 'plain' | 'danger'

const toneClass: Record<Tone, string> = {
  filled: 'bg-tint text-white font-semibold',
  tinted: 'bg-fill text-tint font-medium',
  plain: 'text-tint',
  danger: 'bg-fill text-danger font-medium',
}

/**
 * base は全ての tone に共通の形。
 *
 * min-h-11（44px）は指で押せる最小の大きさ。iOS の指針であると同時に、
 * __これを下回ると狭い画面で隣を押す。__ 記録する手間を増やさない。
 *
 * active:opacity-... は押した感触。iOS は押している間だけ薄くなる。
 * hover を使わないのは、指で操作する端末では hover が「押した後も残る」ため。
 */
const base =
  'inline-flex min-h-11 items-center justify-center gap-1.5 rounded-[10px] px-4 text-[17px] active:opacity-60 disabled:opacity-40'

/** Button はその場で何かを起こすボタン。 */
export function Button({
  tone = 'filled',
  full,
  children,
  className = '',
  ...rest
}: {
  tone?: Tone
  /** full は横いっぱいに広げる。フォームの送信ボタンはこれにする。 */
  full?: boolean
  children: ReactNode
} & ButtonHTMLAttributes<HTMLButtonElement>) {
  return (
    <button
      className={`${base} ${toneClass[tone]} ${full === true ? 'w-full' : ''} ${className}`}
      {...rest}
    >
      {children}
    </button>
  )
}

/**
 * ButtonLink は見た目が Button で、実体はリンク。
 *
 * __PDFのように「開く」ものは必ずこちらを使う。__ fetch して組み立て直すと、
 * ブラウザのビューアで確認してから印刷する、という経路を潰すことになる。
 */
export function ButtonLink({
  to,
  tone = 'filled',
  full,
  children,
  ...rest
}: {
  to: string
  tone?: Tone
  full?: boolean
  children: ReactNode
} & Omit<React.AnchorHTMLAttributes<HTMLAnchorElement>, 'href'>) {
  const cls = `${base} ${toneClass[tone]} ${full === true ? 'w-full' : ''}`

  // 外部・別タブで開くものは <a>。react-router の Link はアプリ内の経路専用で、
  // PDF のような「サーバがそのまま返すもの」を渡すと画面遷移として扱ってしまう。
  if (to.startsWith('/api/') || rest.target !== undefined) {
    return (
      <a href={to} className={cls} {...rest}>
        {children}
      </a>
    )
  }

  return (
    <Link to={to} className={cls} {...rest}>
      {children}
    </Link>
  )
}
