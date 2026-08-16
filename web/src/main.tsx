import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router'

import App from './App'
import { AuthProvider } from './auth/AuthProvider'
import './index.css'

// ルータとログイン状態はここで1回だけ被せる。App の中に置くとテストから
// 差し替えられず、画面ごとのテストで毎回ブラウザのURLと通信に依存する。
const root = document.getElementById('root')
if (!root) {
  throw new Error('#root が無い。index.html を確認すること')
}

createRoot(root).render(
  <StrictMode>
    <BrowserRouter>
      <AuthProvider>
        <App />
      </AuthProvider>
    </BrowserRouter>
  </StrictMode>,
)
