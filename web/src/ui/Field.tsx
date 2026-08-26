import type { InputHTMLAttributes, ReactNode, SelectHTMLAttributes } from 'react'

/**
 * 入力欄も iOS のグループ化リストに合わせる。
 *
 * 枠線で囲った箱を並べない。__白いカードの中に「項目名 ─ 入力欄」の行を__
 * __積む形にすると、読み取り画面（List の Row）と見た目が揃う。__
 * 同じ情報が同じ位置に出るため、編集と閲覧を行き来しても目が迷わない。
 *
 * 使う側は FieldGroup で包み、中に Field / SelectField / SwitchField を並べる。
 */
export function FieldGroup({
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
        <h2 className="px-4 pb-1.5 text-[13px] text-label-2 uppercase">{header}</h2>
      )}
      <div className="overflow-hidden rounded-group bg-card">{children}</div>
      {footer !== undefined && (
        <div className="px-4 pt-1.5 text-[13px] leading-snug text-label-2">{footer}</div>
      )}
    </section>
  )
}

const separator =
  "relative after:absolute after:inset-x-0 after:bottom-0 after:ml-4 after:h-px after:bg-separator after:content-[''] last:after:hidden"

/**
 * Field は1行の入力欄。
 *
 * label は必ず受け取る。__placeholder で代用しない。__ 入力を始めた瞬間に
 * 消えるため、何を書く欄だったか分からなくなる。
 */
export function Field({
  id,
  label,
  hint,
  ...rest
}: {
  id: string
  label: string
  /** hint は欄の下に出す短い補足。 */
  hint?: string
} & InputHTMLAttributes<HTMLInputElement>) {
  return (
    <div className={separator}>
      <div className="flex min-h-11 items-center gap-3 px-4 py-1.5">
        <label className="w-24 shrink-0 text-[17px]" htmlFor={id}>
          {label}
        </label>
        <input
          id={id}
          className="min-w-0 flex-1 bg-transparent py-1.5 text-right text-[17px] placeholder:text-label-3 focus:outline-none"
          {...rest}
        />
      </div>
      {hint !== undefined && <p className="px-4 pb-2 text-[13px] text-label-2">{hint}</p>}
    </div>
  )
}

/**
 * StackedField は項目名を上に置く入力欄。
 *
 * 値が長いもの（パスワード・備考・検索語）に使う。Field は右寄せのため、
 * 長い値だと項目名に食い込む。
 */
export function StackedField({
  id,
  label,
  hint,
  multiline,
  ...rest
}: {
  id: string
  label: string
  hint?: string
  multiline?: boolean
} & InputHTMLAttributes<HTMLInputElement>) {
  const inputClass =
    'w-full bg-transparent text-[17px] placeholder:text-label-3 focus:outline-none'

  return (
    <div className={separator}>
      <div className="px-4 py-2">
        <label className="block text-[13px] text-label-2" htmlFor={id}>
          {label}
        </label>
        {multiline === true ? (
          <textarea
            id={id}
            rows={3}
            className={`${inputClass} mt-0.5 resize-none`}
            value={rest.value}
            onChange={rest.onChange as never}
            placeholder={rest.placeholder}
          />
        ) : (
          <input id={id} className={`${inputClass} mt-0.5 min-h-8`} {...rest} />
        )}
      </div>
      {hint !== undefined && <p className="px-4 pb-2 text-[13px] text-label-2">{hint}</p>}
    </div>
  )
}

/** SelectField は1行の選択欄。 */
export function SelectField({
  id,
  label,
  children,
  ...rest
}: {
  id: string
  label: string
  children: ReactNode
} & SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <div className={`flex min-h-11 items-center gap-3 px-4 py-1.5 ${separator}`}>
      <label className="w-24 shrink-0 text-[17px]" htmlFor={id}>
        {label}
      </label>

      {/* 端末標準の選択UIを使う。iOS ならホイール、Android ならリストが出る。
          自前で作り直すと、その端末の人が知っている操作から外れる。 */}
      <select
        id={id}
        className="min-w-0 flex-1 appearance-none bg-transparent py-1.5 text-right text-[17px] text-label-2 focus:outline-none"
        {...rest}
      >
        {children}
      </select>
    </div>
  )
}

/**
 * SwitchField は on/off の1行。
 *
 * checkbox のまま見た目だけ iOS のスイッチにする。__役割は checkbox のまま__
 * __保つ。__ div で作り直すと、読み上げとキーボード操作が消える。
 */
export function SwitchField({
  id,
  label,
  hint,
  checked,
  onChange,
}: {
  id: string
  label: string
  hint?: string
  checked: boolean
  onChange: (checked: boolean) => void
}) {
  return (
    <div className={separator}>
      <div className="flex min-h-11 items-center gap-3 px-4 py-2">
        <label className="flex-1 text-[17px]" htmlFor={id}>
          {label}
        </label>

        <span className="relative inline-flex shrink-0">
          <input
            id={id}
            type="checkbox"
            checked={checked}
            onChange={(e) => onChange(e.target.checked)}
            className="peer h-[31px] w-[51px] cursor-pointer appearance-none rounded-full bg-fill transition-colors checked:bg-[#34c759] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-tint"
          />
          {/* つまみ。入力そのものは描けないので上に重ねる。 */}
          <span className="pointer-events-none absolute top-0.5 left-0.5 h-[27px] w-[27px] rounded-full bg-white shadow-sm transition-transform peer-checked:translate-x-5" />
        </span>
      </div>
      {hint !== undefined && <p className="px-4 pb-2 text-[13px] text-label-2">{hint}</p>}
    </div>
  )
}
