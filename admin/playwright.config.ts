import { defineConfig } from 'playwright/test'

// 管理台端到端配置：默认启动已构建的单二进制，也可用 ADMIN_E2E_BASE_URL 指向外部服务。
export default defineConfig({
  testDir: './e2e',
  timeout: 60000,
  expect: { timeout: 10000 },
  fullyParallel: false,
  workers: 1,
  reporter: [['list']],
  use: {
    baseURL: process.env.ADMIN_E2E_BASE_URL || 'http://127.0.0.1:18080',
    headless: true,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  webServer: process.env.ADMIN_E2E_BASE_URL
    ? undefined
    : {
        command: process.env.ADMIN_E2E_SERVER
          ? `${process.env.ADMIN_E2E_SERVER} -addr 127.0.0.1:18080 -state /tmp/flowtide-admin-e2e.json -jobs-interval 0`
          : "bash -c 'cd .. && ./bin/flowtide-server -addr 127.0.0.1:18080 -state /tmp/flowtide-admin-e2e.json -jobs-interval 0'",
        url: 'http://127.0.0.1:18080/healthz',
        reuseExistingServer: false,
        timeout: 120000,
      },
})
