import { useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import { Link, useParams } from 'react-router'

import { ApiError, errorMessage } from '../api/client'
import { getItem } from '../api/items'
import type { Item } from '../api/types'

/**
 * ItemDetail は備品の詳細画面。QRの遷移先（`/i/{code}`）。
 *
 * __M1では表示のみ。貸出ボタンを置かない。__ 借用はM2で作る。押せない
 * ボタンを先に置くと、触った人には壊れているとしか見えない（CLAUDE.md）。
 *
 * 未ログインで来た場合は RequireAuth がログイン画面へ送り、認証後に
 * ここへ戻す。戻せないともう一度QRを読み直させることになり、
 * その一手間が記録漏れの直接原因になる。
 */
export default function ItemDetail() {
  const { code = '' } = useParams()

  const [item, setItem] = useState<Item | null>(null)
  const [error, setError] = useState('')
  const [notFound, setNotFound] = useState(false)

  useEffect(() => {
    let alive = true

    setItem(null)
    setError('')
    setNotFound(false)

    void getItem(code).then(
      (it) => {
        if (alive) setItem(it)
      },
      (err: unknown) => {
        if (!alive) return
        // 登録が無い場合は、原因の心当たりまで出す。QRを読んで来た人が
        // 最初に見る画面で、「エラー」とだけ出しても次の手が分からない。
        if (err instanceof ApiError && err.status === 404) {
          setNotFound(true)
          return
        }
        setError(errorMessage(err))
      },
    )

    return () => {
      alive = false
    }
  }, [code])

  if (notFound) {
    return (
      <Layout>
        <h1 className="text-xl font-bold">登録されていない備品コードです</h1>
        <p className="mt-2 font-mono text-sm text-gray-600">{code}</p>
        <p className="mt-2 text-sm text-gray-600">
          ラベルの読み取りに失敗したか、まだ登録されていない可能性があります。
        </p>
      </Layout>
    )
  }

  if (error !== '') {
    return (
      <Layout>
        <p role="alert" className="text-red-700">
          {error}
        </p>
      </Layout>
    )
  }

  if (item === null) {
    return (
      <Layout>
        <p className="text-sm text-gray-600">読み込み中…</p>
      </Layout>
    )
  }

  return (
    <Layout>
      <p className="font-mono text-sm text-gray-600">{item.code}</p>
      <h1 className="text-xl font-bold">{item.name}</h1>

      {/* 廃棄は物理削除の代わり。ラベルは貼られたままなので、
          読んだ人に「これはもう使わないもの」と分かる形で出す。 */}
      {item.condition === '廃棄' && (
        <p className="mt-3 rounded bg-gray-200 p-3 text-sm">この備品は廃棄されています。</p>
      )}

      {item.location_status !== '在庫' && (
        <p className="mt-3 rounded bg-amber-50 p-3 text-sm text-amber-900">
          所在: {item.location_status}
        </p>
      )}

      {/* 自由利用品は貸出フローの対象外。追跡対象を減らすことが
          遵守率を上げる最短経路（CLAUDE.md）。 */}
      {item.is_free_use && (
        <p className="mt-3 rounded bg-gray-100 p-3 text-sm">
          自由利用品です。借用の記録は要りません。
        </p>
      )}

      {item.photo_url !== '' && (
        <img
          className="mt-4 w-full rounded"
          src={item.photo_url}
          alt={`${item.name}の写真`}
          // 写真は備品を見分けるためのもの。読み込めなくても他の情報は要る。
          loading="lazy"
        />
      )}

      <dl className="mt-4 divide-y divide-gray-200 border-y border-gray-200">
        <Row label="分類" value={item.category} />
        <Row label="型番" value={item.model} />
        <Row label="所有" value={item.owner} />
        <Row label="保管場所" value={item.location} />
        <Row label="状態" value={item.condition} />
        <Row label="備考" value={item.note} />
      </dl>

      <p className="mt-4 text-sm text-gray-600">最終更新: {item.updated_at}</p>

      <Link className="mt-6 inline-block text-blue-700 underline" to="/items">
        備品一覧へ
      </Link>
    </Layout>
  )
}

/** Row は1項目。空欄は「—」で埋める。行ごと消すと項目の有無が揺れて読みにくい。 */
function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex gap-4 py-2">
      <dt className="w-24 shrink-0 text-sm text-gray-600">{label}</dt>
      <dd className="text-sm">{value === '' ? '—' : value}</dd>
    </div>
  )
}

function Layout({ children }: { children: ReactNode }) {
  return <main className="mx-auto max-w-screen-sm p-4">{children}</main>
}
