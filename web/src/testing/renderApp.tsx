import { render } from '@testing-library/react'
import { MemoryRouter } from 'react-router'

import App from '../App'
import { AuthProvider } from '../auth/AuthProvider'

/**
 * renderApp は指定のURLでアプリ全体を描画する。
 *
 * 画面だけを単体で描画しない。経路の割り当てと RequireAuth を通した形で
 * 確かめる。包み忘れた画面は、単体で描画するテストでは見つからない。
 *
 * MemoryRouter を使うのは、テストがブラウザのURLに依存しないようにするため。
 */
export function renderApp(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <AuthProvider>
        <App />
      </AuthProvider>
    </MemoryRouter>,
  )
}
