import { useState } from 'react'
import type { ChangeEvent } from 'react'
import { Link } from 'react-router'

import { errorMessage } from '../api/client'
import { importItems, previewImport } from '../api/items'
import type { ImportPreview, ImportResult } from '../api/items'

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
      <main className="mx-auto max-w-screen-sm p-4">
        <h1 className="text-xl font-bold">取り込みました</h1>

        <p className="mt-4">{result.record_count}件を登録しました。</p>

        <p className="mt-4 text-sm text-gray-600">採番された備品コード</p>
        {/* 予定ではなく確定した値。この範囲がそのままラベルの印刷範囲になる。 */}
        <p className="font-mono text-2xl">
          {result.code_from} 〜 {result.code_to}
        </p>

        <div className="mt-6 flex flex-wrap gap-3">
          <Link className="text-blue-700 underline" to="/admin/items">
            マスタ管理へ
          </Link>
        </div>
      </main>
    )
  }

  return (
    <main className="mx-auto max-w-screen-sm p-4">
      <h1 className="text-xl font-bold">CSVで一括登録</h1>
      <p className="mt-1 text-sm text-gray-600">
        棚卸しシートのCSVを取り込みます。備品コードは取り込み時に採番されます。
      </p>

      <div className="mt-4">
        <label className="block text-sm font-medium" htmlFor="file">
          CSVファイル
        </label>
        <input
          id="file"
          type="file"
          accept=".csv,text/csv"
          className="mt-1 w-full text-sm"
          onChange={chooseFile}
        />
        <p className="mt-1 text-xs text-gray-600">
          文字コードは UTF-8 でも Shift_JIS でも構いません。
        </p>
      </div>

      <button
        className="mt-3 rounded bg-blue-700 px-4 py-2 text-white disabled:bg-gray-400"
        disabled={file === null || previewing || importing}
        onClick={() => void runPreview()}
      >
        {previewing ? '確認しています…' : '内容を確認する'}
      </button>

      {error !== '' && (
        <p role="alert" className="mt-4 text-sm text-red-700">
          {error}
        </p>
      )}

      {preview !== null && <Preview preview={preview} excluded={excluded} onToggle={toggle} />}

      {preview !== null && preview.can_import && (
        <button
          className="mt-4 w-full rounded bg-blue-700 px-4 py-3 text-white disabled:bg-gray-400"
          disabled={importing}
          onClick={() => void runImport()}
        >
          {importing ? '取り込んでいます…' : 'この内容で取り込む'}
        </button>
      )}
    </main>
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
    <section className="mt-6">
      <h2 className="text-lg font-bold">確認</h2>

      {preview.errors.length > 0 ? (
        <div className="mt-2 rounded bg-red-50 p-3">
          {/* 誤りが1件でもあれば取り込めない。全件成功か全件失敗で、
              誤った行だけを飛ばして入れることはしない（m1-spec §5）。 */}
          <p role="alert" className="text-sm text-red-800">
            取り込めない行が{preview.errors.length}件あります。CSVを直してから、
            もう一度確認してください。
          </p>
          <ul className="mt-2 space-y-1">
            {preview.errors.map((e) => (
              <li key={e.line} className="text-sm text-red-800">
                {e.line}行目: {e.message}
              </li>
            ))}
          </ul>
        </div>
      ) : (
        <>
          <p className="mt-2">
            {preview.row_count}行から <strong>{records}件</strong> の備品を登録します。
          </p>

          {/* 採番の予定は、除外を選んでいない時だけ出す。行を除くと範囲がずれるのに
              決まった値のように見せると、この範囲でラベルを刷る人が出る。 */}
          {excluded.size === 0 && preview.code_from !== '' && (
            <p className="mt-1 text-sm text-gray-600">
              採番の予定: <span className="font-mono">{preview.code_from} 〜 {preview.code_to}</span>
              （予定です。実際の番号は取り込み後に表示します）
            </p>
          )}

          <p className="mt-3 text-sm text-gray-600">
            テンプレートの記入例が残っている場合は、その行の「取り込まない」に印を付けてください。
            CSVを直す必要はありません。
          </p>
        </>
      )}

      {preview.rows.length > 0 && (
        <ul className="mt-3 divide-y divide-gray-200 border-y border-gray-200">
          {preview.rows.map((row) => {
            const skip = excluded.has(row.line)
            return (
              <li key={row.line} className={`py-2 ${skip ? 'opacity-50' : ''}`}>
                <div className="flex items-baseline gap-2">
                  <span className="font-mono text-xs text-gray-600">{row.line}行目</span>
                  <span className="font-medium">{row.name}</span>
                  {row.quantity > 1 && <span className="text-sm">×{row.quantity}</span>}
                </div>

                <p className="text-sm text-gray-600">
                  {[row.category, row.model, row.location, row.condition]
                    .filter((v) => v !== '')
                    .join('・')}
                  {row.is_free_use && '・自由利用品'}
                </p>

                <label className="mt-1 flex items-center gap-2 text-sm">
                  <input
                    type="checkbox"
                    className="size-4"
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
      )}
    </section>
  )
}
