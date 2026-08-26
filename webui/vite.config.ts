import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// 浏览器 Web UI 构建产物嵌入 server（//go:embed webbrowser）。
// base 匹配 /web/ 挂载点，outDir 输出到 server/webbrowser/。
export default defineConfig({
  plugins: [react()],
  base: '/web/',
  build: {
    outDir: '../server/webbrowser',
    emptyOutDir: true,
  },
  server: {
    port: 5174,
    proxy: {
      '/v1': 'http://localhost:8080',
      '/health': 'http://localhost:8080',
    },
  },
})
