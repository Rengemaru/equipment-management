/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],

  server: {
    // 開発サーバはコンテナの中で動く。既定の localhost 束縛のままだと、
    // ホストのブラウザから 5173 を開いても届かない。
    host: '0.0.0.0',
    port: 5173,

    // API はコンテナ内の Go サーバ（同じコンテナの 8080）に流す。
    // フロントから絶対URLで叩く形にすると、開発・本番でURLが変わり、
    // Cookie の送信条件も変わる。単一オリジンのまま揃える。
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },

  test: {
    // 画面のテストは DOM を要る。jsdom はブラウザではないが、
    // 表示の分岐を確かめるには足りる。実機での確認は別途行う。
    environment: 'jsdom',
    setupFiles: ['./src/test-setup.ts'],
  },
})
