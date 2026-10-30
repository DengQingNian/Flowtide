import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'

// 测试配置：jsdom、统一别名与覆盖率门禁。
// @return Vitest 配置
export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
  },
  define: {
    __ADMIN_VERSION__: JSON.stringify('0.1.0'),
    __ADMIN_COMMIT__: JSON.stringify('test-commit'),
    __ADMIN_BUILD_TIME__: JSON.stringify('2026-10-30T00:00:00Z'),
  },
  test: {
    environment: 'jsdom',
    environmentOptions: { jsdom: { url: 'http://localhost/adminpage/' } },
    include: ['tests/**/*.test.ts'],
    setupFiles: ['tests/setup.ts'],
    coverage: {
      provider: 'v8',
      include: ['src/**/*.{ts,vue}'],
      exclude: ['src/**/*.d.ts', 'src/main.ts', 'src/router/index.ts', 'src/design/BpmnDesigner.vue', 'src/design/BpmnViewer.vue', 'src/design/bpmn.ts'],
      reporter: ['text', 'json-summary'],
      thresholds: { lines: 90, functions: 85, branches: 80 },
    },
  },
})