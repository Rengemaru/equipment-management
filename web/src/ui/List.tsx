import type { ReactNode } from 'react'
import { Link } from 'react-router'

/**
 * List は iOS のグループ化リスト。灰色の地の上に置く白いカード。
 *
 * __区切り線はカードの内側にだけ引き、左端は文字に合わせて下げる。__
 * カードの端まで引くと、行が独立した箱に見えて、まとまりが読み取れなくなる。
 *
 * 中身は ul / li で組む。__見た目を変えても list の役割は保つ。__ div で
 * 積むと、読み上げ環境で「何件あるか」「何番目か」が失われる。
 *
 * header / footer は iOS の「グループの上の小さな見出し」と「下の注記」。
 * 注意書きはここに置く。本文に混ぜると、読み飛ばした人には無いのと同じになる。
 */
export function List({
  header,
  footer,
  children,
}: {
  header?: string
  footer?: ReactNode
  children: ReactNode
}) {
  return (
    <section className="mt-6">
      {header !== undefined && (
        <h2 className="px-4 pb-1.5 text-[13px] text-label-2">{header}</h2>
      )}

      {/* overflow-hidden で、先頭と末尾の行が角丸からはみ出さないようにする。 */}
      <ul className="overflow-hidden rounded-group bg-card">{children}</ul>

      {footer !== undefined && (
        <div className="px-4 pt-1.5 text-[13px] leading-snug text-label-2">{footer}</div>
      )}
    </section>
  )
}

/**
 * Card は List と同じ見た目で、中身がリストでないもの（長い文章など）。
 *
 * ul に段落を入れない。役割と中身が食い違うと、読み上げが「1項目のリスト」と
 * 言ってから本文を読むことになる。
 */
export function Card({ header, children }: { header?: string; children: ReactNode }) {
  return (
    <section className="mt-6">
      {header !== undefined && (
        <h2 className="px-4 pb-1.5 text-[13px] text-label-2">{header}</h2>
      )}
      <div className="overflow-hidden rounded-group bg-card">{children}</div>
    </section>
  )
}

/**
 * separator は行の間の線。
 *
 * border ではなく擬似要素で引く。border だと最後の行にも付き、
 * カードの底に二重線が出る。
 */
const separator =
  "relative after:absolute after:inset-x-0 after:bottom-0 after:ml-4 after:h-px after:bg-separator after:content-[''] last:after:hidden"

/**
 * Row は「項目名 ─ 値」の1行。押せない。
 *
 * 値が空の時は「—」で埋める。行ごと消すと項目の有無が揺れ、
 * 別の備品と見比べた時に目が滑る。
 */
export function Row({ label, value }: { label: string; value: ReactNode }) {
  const empty = value === '' || value === null || value === undefined

  return (
    <li className={`flex min-h-11 items-center gap-4 px-4 py-2.5 ${separator}`}>
      <span className="shrink-0 text-[17px]">{label}</span>
      <span className={`ml-auto text-right text-[17px] ${empty ? 'text-label-3' : 'text-label-2'}`}>
        {empty ? '—' : value}
      </span>
    </li>
  )
}

/**
 * LinkRow は押すと別の画面へ行く行。右端に山形を出す。
 *
 * __行全体を的にする。__ 文字だけをリンクにすると、スマートフォンでは
 * 隣の行を開くことになる。
 */
export function LinkRow({
  to,
  children,
  value,
}: {
  to: string
  children: ReactNode
  /** value は右端の山形の手前に出す補足。 */
  value?: ReactNode
}) {
  return (
    <li className={separator}>
      <Link to={to} className="flex min-h-11 items-center gap-3 px-4 py-2.5 active:bg-fill">
        <div className="min-w-0 flex-1">{children}</div>
        {value !== undefined && <span className="shrink-0 text-[17px] text-label-2">{value}</span>}
        <DisclosureChevron />
      </Link>
    </li>
  )
}

/**
 * ButtonRow は押すとその場で何かが起きる行（無効化・再発行など）。
 *
 * 行き先が無いので山形は出さない。__出すと「次の画面へ進む」に見え、__
 * __押した瞬間に処理が走ることが伝わらない。__
 */
export function ButtonRow({
  onClick,
  disabled,
  tone = 'tint',
  children,
}: {
  onClick: () => void
  disabled?: boolean
  tone?: 'tint' | 'danger'
  children: ReactNode
}) {
  const color = tone === 'danger' ? 'text-danger' : 'text-tint'

  return (
    <li className={separator}>
      <button
        type="button"
        onClick={onClick}
        disabled={disabled}
        className={`flex min-h-11 w-full items-center px-4 py-2.5 text-left text-[17px] ${color} active:bg-fill disabled:text-label-3`}
      >
        {children}
      </button>
    </li>
  )
}

/** DisclosureChevron は「押すと進む」ことを示す右端の山形。 */
function DisclosureChevron() {
  return (
    <svg
      viewBox="0 0 8 14"
      className="h-3.5 w-2 shrink-0 text-label-3"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M1 1l6 6-6 6" />
    </svg>
  )
}
