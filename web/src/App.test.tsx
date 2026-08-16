import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { expect, test } from 'vitest'

import App from './App'

// MemoryRouter を使うのは、テストがブラウザのURLに依存しないようにするため。
function renderAt(path: string) {
  render(
    <MemoryRouter initialEntries={[path]}>
      <App />
    </MemoryRouter>,
  )
}

test('トップを開くと表示される', () => {
  renderAt('/')

  expect(screen.getByRole('heading', { name: '備品管理' })).toBeDefined()
})

// 知らないURLで白い画面になると、QRを読み間違えた人には
// 「壊れている」としか見えない。
test('割り当てのないURLは見つからないと表示する', () => {
  renderAt('/i/0042')

  expect(screen.getByRole('heading', { name: 'ページが見つかりません' })).toBeDefined()
  expect(screen.getByRole('link', { name: 'トップへ' })).toBeDefined()
})
