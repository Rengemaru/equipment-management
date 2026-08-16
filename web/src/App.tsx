import { Link, Route, Routes } from 'react-router'

/**
 * App は画面の割り当てだけを持つ。
 *
 * 画面はここに1つずつ足していく。押せないボタンやダミー画面は置かない
 * （CLAUDE.md）。未実装の画面へのリンクを先に置くと、触った人には
 * 「壊れている」としか見えない。
 */
export default function App() {
  return (
    <Routes>
      <Route path="/" element={<Home />} />
      {/* 知らないURLで白い画面を出さない。QRの読み間違いはここに来る。 */}
      <Route path="*" element={<NotFound />} />
    </Routes>
  )
}

function Home() {
  return (
    <main className="mx-auto max-w-screen-sm p-4">
      <h1 className="text-xl font-bold">備品管理</h1>
    </main>
  )
}

function NotFound() {
  return (
    <main className="mx-auto max-w-screen-sm p-4">
      <h1 className="text-xl font-bold">ページが見つかりません</h1>
      <p className="mt-2 text-sm text-gray-600">
        QRの読み取りに失敗したか、URLが変わった可能性があります。
      </p>
      <Link className="mt-4 inline-block text-blue-700 underline" to="/">
        トップへ
      </Link>
    </main>
  )
}
