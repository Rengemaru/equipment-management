import type { ReactNode } from 'react'

/**
 * 画面が「今どうなっているか」を伝える部品をまとめる。
 *
 * __読み込み中・失敗・0件は、どの画面でも同じ形で出す。__ 画面ごとに
 * 書き方が違うと、利用者は毎回それが何なのかを読み取り直すことになる。
 */

/**
 * Alert は操作が失敗したことを伝える。
 *
 * role="alert" を必ず付ける。読み上げ環境では、これが無いと画面の下に
 * 出た文字に気付けない。
 */
export function Alert({ children }: { children: ReactNode }) {
  return (
    <p
      role="alert"
      className="mt-6 rounded-group bg-card px-4 py-3 text-[15px] leading-snug text-danger"
    >
      {children}
    </p>
  )
}

/** Notice は失敗ではない知らせ（廃棄済み・自由利用品など）。 */
export function Notice({
  tone = 'plain',
  children,
}: {
  tone?: 'plain' | 'warn'
  children: ReactNode
}) {
  const color = tone === 'warn' ? 'bg-warn-bg text-warn' : 'bg-card text-label-2'

  return (
    <p className={`mt-6 rounded-group px-4 py-3 text-[15px] leading-snug ${color}`}>{children}</p>
  )
}

/**
 * Loading は読み込み中。
 *
 * 骨組みだけの偽の行を並べない。件数が分からないうちに並べると、
 * 実際より多く（少なく）見え、届いた瞬間に画面が飛ぶ。
 */
export function Loading({ children = '読み込み中…' }: { children?: string }) {
  return <p className="mt-8 text-center text-[15px] text-label-2">{children}</p>
}

/**
 * Empty は該当が無いこと。
 *
 * 「ありません」だけで終えない。__次に何をすればよいかまで書く。__
 * 絞り込みの条件を疑えばよいのか、そもそも登録が無いのかで手が変わる。
 */
export function Empty({ title, hint }: { title: string; hint?: ReactNode }) {
  return (
    <div className="mt-12 px-6 text-center">
      <p className="text-[17px] text-label-2">{title}</p>
      {hint !== undefined && (
        <p className="mt-1.5 text-[13px] leading-snug text-label-3">{hint}</p>
      )}
    </div>
  )
}

/**
 * Badge は行に付ける短い印。
 *
 * __良好・在庫のような「普通の状態」には付けない。__ 全ての行に付くと、
 * 注意すべき行が埋もれて、印としての意味を失う。
 */
export function Badge({ tone, children }: { tone: 'warn' | 'info'; children: ReactNode }) {
  const color = tone === 'warn' ? 'bg-warn-bg text-warn' : 'bg-fill text-label-2'

  return (
    <span className={`rounded-md px-1.5 py-0.5 text-[12px] font-medium ${color}`}>{children}</span>
  )
}
