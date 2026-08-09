import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

// The production build is embedded by the Go server via //go:embed webui,
// so dist output is redirected into server/webui (checked into the repo,
// mirroring the existing dashboard.html pattern).
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: '../server/webui',
    emptyOutDir: true,
  },
  server: {
    proxy: {
      '/v1': 'http://localhost:8080',
      '/health': 'http://localhost:8080',
    },
  },
  test: {
    environment: 'node',
  },
})
