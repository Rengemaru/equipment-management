import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router'

import App from './App'
import './index.css'

// ルータはここで1回だけ被せる。App の中に置くとテストから差し替えられず、
// 画面ごとのテストで毎回ブラウザのURLに依存することになる。
const root = document.getElementById('root')
if (!root) {
  throw new Error('#root が無い。index.html を確認すること')
}

createRoot(root).render(
  <StrictMode>
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </StrictMode>,
)
