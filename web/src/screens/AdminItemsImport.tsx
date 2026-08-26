import { useState } from 'react'
import type { ChangeEvent } from 'react'

import { errorMessage } from '../api/client'
import { importItems, previewImport } from '../api/items'
import type { ImportPreview, ImportResult } from '../api/items'
import { Button } from '../ui/Button'
import { Alert } from '../ui/Feedback'
import { FieldGroup } from '../ui/Field'
import { Card, LinkRow, List } from '../ui/List'
import { Screen } from '../ui/Screen'

/**
 * AdminItemsImport は棚卸しCSVの取り込み画面（運営のみ）。
 *
 * 確定の前に必ずプレビューを挟む。テンプレートの2行目には記入例が入っていて、
 * 「作業前に削除する」と書かれていても消し忘れは必ず起きる（m1-spec §5）。
 * __備品コードは再利用しないため、余計な行を取り込むと番号を戻せない。__
 *
 * プレビューと確定で同じファイルを2度送る。サーバは解析結果を覚えていない。
 */
export default function AdminItemsImport() {
  const [file, setFile] = useState<File | null>(null)
  const [preview, setPreview] = useState<ImportPreview | null>(null)
  const [result, setResult] = useState<ImportResult | null>(null)
  const [error, setError] = useState('')
  const [previewing, setPreviewing] = useState(false)
  const [importing, setImporting] = useState(false)

  /** excluded は取り込まない行番号。テンプレートの記入例を除くために使う。 */
  const [excluded, setExcluded] = useState<Set<number>>(new Set())

  const chooseFile = (e: ChangeEvent<HTMLInputElement>) => {
    // 別のファイルを選んだら、前のプレビューは無効。残すと、見ている内容と
    // 送るファイルが食い違ったまま確定できてしまう。
    setFile(e.target.files?.[0] ?? null)
    setPreview(null)
    setResult(null)
    setExcluded(new Set())
    setError('')
  }

  const runPreview = async () => {
    if (file === null) return

    setError('')
    setPreviewing(true)
    try {
      setPreview(await previewImport(file))
      setExcluded(new Set())
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setPreviewing(false)
    }
  }

  const runImport = async () => {
    if (file === null) return

    setError('')
    setImporting(true)
    try {
      setResult(await importItems(file, [...excluded]))
      setPreview(null)
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setImporting(false)
    }
  }

  const toggle = (line: number) => {
    setExcluded((prev) => {
      const next = new Set(prev)
      if (next.has(line)) next.delete(line)
      else next.add(line)
      return next
    })
  }

  if (result !== null) {
    return (
      <Screen title="取り込みました" back={{ to: '/admin/items', label: 'マスタ管理' }}>
        {/* 予定ではなく確定した値。__この範囲がそのままラベルの印刷範囲になる。__ */}
        <Card header="採番された備品コード">
          <div className="px-4 py-5 text-center">
            <p className="font-mono text-[32px] leading-none font-semibold tabular-nums">
              {result.code_from} 〜 {result.code_to}
            </p>
            <p className="mt-2 text-[15px] text-label-2">
              {result.record_count}件を登録しました。
            </p>
          </div>
        </Card>

        <List>
          <LinkRow to={`/admin/labels?from=${result.code_from}&to=${result.code_to}`}>
            <span className="text-[17px]">この範囲のQRラベルを刷る</span>
          </LinkRow>
          <LinkRow to="/admin/items">
            <span className="text-[17px]">マスタ管理へ</span>
          </LinkRow>
        </List>
      </Screen>
    )
  }

  return (
    <Screen title="CSVで一括登録" back={{ to: '/admin/items', label: 'マスタ管理' }}>
      <p className="mt-1 px-4 text-[13px] leading-snug text-label-2">
        棚卸しシートのCSVを取り込みます。備品コードは取り込み時に採番されます。
      </p>

      <FieldGroup header="CSVファイル" footer="文字コードは UTF-8 でも Shift_JIS でも構いません。">
        <div className="px-4 py-3">
          <label className="block text-[13px] text-label-2" htmlFor="file">
            CSVファイル
          </label>
          <input
            id="file"
            type="file"
            accept=".csv,text/csv"
            className="mt-1.5 w-full text-[15px] file:mr-3 file:rounded-full file:border-0 file:bg-fill file:px-3 file:py-1.5 file:text-[15px] file:text-tint"
            onChange={chooseFile}
          />
        </div>
      </FieldGroup>

      {/* __確定の前に必ずプレビューを挟む。__ 同じファイルを2度送る
          （サーバは解析結果を持たない）。 */}
      <div className="mt-6">
        <Button
          full
          tone="tinted"
          disabled={file === null || previewing || importing}
          onClick={() => void runPreview()}
        >
          {previewing ? '確認しています…' : '内容を確認する'}
        </Button>
      </div>

      {error !== '' && <Alert>{error}</Alert>}

      {preview !== null && <Preview preview={preview} excluded={excluded} onToggle={toggle} />}

      {preview !== null && preview.can_import && (
        <div className="mt-6">
          <Button full disabled={importing} onClick={() => void runImport()}>
            {importing ? '取り込んでいます…' : 'この内容で取り込む'}
          </Button>
        </div>
      )}
    </Screen>
  )
}

function Preview({
  preview,
  excluded,
  onToggle,
}: {
  preview: ImportPreview
  excluded: Set<number>
  onToggle: (line: number) => void
}) {
  // 除外した行のぶんを引く。1行が数量の数だけのレコードになるため、
  // 行数ではなく数量で数える。
  const records = preview.rows
    .filter((row) => !excluded.has(row.line))
    .reduce((sum, row) => sum + row.quantity, 0)

  return (
    <>
      {preview.errors.length > 0 ? (
        <>
          {/* 誤りが1件でもあれば取り込めない。全件成功か全件失敗で、
              誤った行だけを飛ばして入れることはしない（m1-spec §5）。 */}
          <Alert>
            取り込めない行が{preview.errors.length}件あります。CSVを直してから、
            もう一度確認してください。
          </Alert>

          <Card header="取り込めない行">
            <ul className="divide-y divide-separator">
              {preview.errors.map((e) => (
                <li key={e.line} className="px-4 py-2.5 text-[15px] text-danger">
                  {e.line}行目: {e.message}
                </li>
              ))}
            </ul>
          </Card>
        </>
      ) : (
        <Card header="確認">
          <div className="px-4 py-3">
            <p className="text-[17px]">
              {preview.row_count}行から <strong>{records}件</strong> の備品を登録します。
            </p>

            {/* 採番の予定は、除外を選んでいない時だけ出す。行を除くと範囲がずれるのに
                決まった値のように見せると、この範囲でラベルを刷る人が出る。 */}
            {excluded.size === 0 && preview.code_from !== '' && (
              <p className="mt-1.5 text-[13px] text-label-2">
                採番の予定:{' '}
                <span className="font-mono tabular-nums">
                  {preview.code_from} 〜 {preview.code_to}
                </span>
                （予定です。実際の番号は取り込み後に表示します）
              </p>
            )}

            <p className="mt-3 text-[13px] leading-snug text-label-2">
              テンプレートの記入例が残っている場合は、その行の「取り込まない」に印を付けてください。
              CSVを直す必要はありません。
            </p>
          </div>
        </Card>
      )}

      {preview.rows.length > 0 && (
        <Card header="内容">
          <ul className="divide-y divide-separator">
            {preview.rows.map((row) => {
              const skip = excluded.has(row.line)
              const detail = [row.category, row.model, row.location, row.condition]
                .filter((v) => v !== '')
                .join('・')

              return (
                <li key={row.line} className={`px-4 py-2.5 ${skip ? 'opacity-40' : ''}`}>
                  <div className="flex items-center gap-2">
                    <span className="font-mono text-[12px] tabular-nums text-label-2">
                      {row.line}行目
                    </span>
                    <span className="truncate text-[17px]">{row.name}</span>
                    {row.quantity > 1 && (
                      <span className="text-[15px] text-label-2">×{row.quantity}</span>
                    )}
                  </div>

                  <p className="mt-0.5 truncate text-[13px] text-label-2">
                    {detail}
                    {row.is_free_use && '・自由利用品'}
                  </p>

                  <label className="mt-1.5 flex min-h-9 items-center gap-2 text-[15px] text-label-2">
                    <input
                      type="checkbox"
                      className="size-5 accent-[color:var(--c-tint)]"
                      // 同じ文言の印が並ぶため、どの行のものか名前に含める。
                      // 読み上げでも「どれを外したのか」が分かるようにする。
                      aria-label={`${row.line}行目を取り込まない`}
                      checked={skip}
                      onChange={() => onToggle(row.line)}
                    />
                    取り込まない
                  </label>
                </li>
              )
            })}
          </ul>
        </Card>
      )}
    </>
  )
}
