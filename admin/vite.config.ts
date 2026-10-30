import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// 解析构建期注入的管理台版本信息。
// @return 版本、提交号与构建时间
function buildMetadata(): { version: string; commit: string; buildTime: string } {
  const version = process.env.npm_package_version || '0.1.0'
  const commit = process.env.GIT_COMMIT || process.env.COMMIT || 'unknown'
  return { version, commit, buildTime: new Date().toISOString() }
}

const metadata = buildMetadata()

// 管理台构建配置：同源 /adminpage/、设计器独立分包。
// @return Vite 配置
export default defineConfig({
  base: '/adminpage/',
  plugins: [vue()],
  resolve: {
    alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
  },
  define: {
    __ADMIN_VERSION__: JSON.stringify(metadata.version),
    __ADMIN_COMMIT__: JSON.stringify(metadata.commit),
    __ADMIN_BUILD_TIME__: JSON.stringify(metadata.buildTime),
  },
  build: {
    target: 'es2022',
    outDir: 'dist',
    sourcemap: false,
    chunkSizeWarningLimit: 1300,
    rollupOptions: {
      output: {
        manualChunks: {
          naive: ['naive-ui'],
          vue: ['vue', 'vue-router'],
        },
      },
    },
  },
  server: {
    port: 5174,
    proxy: {
      '/service': { target: 'http://localhost:8080', changeOrigin: false },
      '/healthz': { target: 'http://localhost:8080', changeOrigin: false },
      '/readyz': { target: 'http://localhost:8080', changeOrigin: false },
      '/metrics': { target: 'http://localhost:8080', changeOrigin: false },
    },
  },
  preview: { port: 4174 },
})
