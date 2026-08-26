import { useEffect, useState } from 'react'
import { useParams } from 'react-router'

import { ApiError, errorMessage } from '../api/client'
import { getItem } from '../api/items'
import type { Item } from '../api/types'
import { Alert, Empty, Loading, Notice } from '../ui/Feedback'
import { Card, List, Row } from '../ui/List'
import { Screen } from '../ui/Screen'

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


  // 戻り先は常に備品一覧にする。QRから直接来た人には履歴が無く、
  // ブラウザの戻るが効かない。行き先を示さないと、そこで止まってしまう。
  const back = { to: '/items', label: '備品一覧' }

  if (notFound) {
    return (
      <Screen title="登録されていない備品コードです" subtitle={code} back={back}>
        <Empty
          title="この備品は登録されていません"
          hint="ラベルの読み取りに失敗したか、まだ登録されていない可能性があります。"
        />
      </Screen>
    )
  }

  if (error !== '') {
    return (
      <Screen title="備品" subtitle={code} back={back}>
        <Alert>{error}</Alert>
      </Screen>
    )
  }

  if (item === null) {
    return (
      <Screen title="読み込み中…" subtitle={code} back={back}>
        <Loading />
      </Screen>
    )
  }

  return (
    <Screen title={item.name} subtitle={item.code} back={back}>
      {/* 廃棄は物理削除の代わり。ラベルは貼られたままなので、
          読んだ人に「これはもう使わないもの」と分かる形で出す。 */}
      {item.condition === '廃棄' && <Notice>この備品は廃棄されています。</Notice>}

      {item.location_status !== '在庫' && (
        <Notice tone="warn">所在: {item.location_status}</Notice>
      )}

      {/* 自由利用品は貸出フローの対象外。追跡対象を減らすことが
          遵守率を上げる最短経路（CLAUDE.md）。 */}
      {item.is_free_use && <Notice>自由利用品です。借用の記録は要りません。</Notice>}

      {item.photo_url !== '' && (
        <img
          className="mt-6 w-full rounded-group"
          src={item.photo_url}
          alt={`${item.name}の写真`}
          // 写真は備品を見分けるためのもの。読み込めなくても他の情報は要る。
          loading="lazy"
        />
      )}

      <List>
        <Row label="分類" value={item.category} />
        <Row label="型番" value={item.model} />
        <Row label="所有" value={item.owner} />
        <Row label="保管場所" value={item.location} />
        <Row label="状態" value={item.condition} />
      </List>

      {/* 備考は長くなる。右寄せの Row に入れると折り返しが読みにくい。 */}
      <Card header="備考">
        <p className="px-4 py-3 text-[17px] leading-relaxed whitespace-pre-wrap">
          {item.note === '' ? <span className="text-label-3">—</span> : item.note}
        </p>
      </Card>

      <p className="mt-6 px-4 text-[13px] text-label-2">最終更新: {item.updated_at}</p>
    </Screen>
  )
}
