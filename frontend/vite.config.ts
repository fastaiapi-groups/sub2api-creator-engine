import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath, URL } from 'node:url'

export default defineConfig({
  base: '/fastaiui/',
  plugins: [vue()],
  resolve: { alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) } },
  server: {
    proxy: {
      '/fastaiapi': {
        target: process.env.SUB_ADDS_DEV_API_URL || 'http://127.0.0.1:4455',
        changeOrigin: true,
        rewrite: path => path.replace(/^\/fastaiapi/, ''),
      },
    },
  },
})
