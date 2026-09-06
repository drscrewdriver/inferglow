import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { resolve } from 'node:path'

// Integrated into the InferGlow repo: build output goes to server/webui-dsh/ so
// `go build` embeds it (go:embed in server/handlers_webui_dsh.go), mirroring the
// server/webui and server/webui2 conventions. Dev server proxies API calls to
// the Go backend on :8080 so the app runs same-origin without CORS setup.
export default defineConfig({
  plugins: [react()],
  base: '/webui-dsh/',
  resolve: {
    alias: {
      '@': resolve(__dirname, 'src'),
    },
  },
  build: {
    outDir: '../server/webui-dsh',
    emptyOutDir: true,
    sourcemap: false,
    rollupOptions: {
      output: {
        // Separate vendor chunk for React to allow caching
        manualChunks: {
          vendor: ['react', 'react-dom'],
        },
      },
    },
  },
  server: {
    port: 5176,
    strictPort: true,
    proxy: {
      '/v1': 'http://localhost:8080',
      '/health': 'http://localhost:8080',
    },
  },
})
