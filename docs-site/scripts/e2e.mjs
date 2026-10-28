import fs from 'node:fs'
import path from 'node:path'
import { chromium } from 'playwright'

const base = process.env.BASE_URL || 'http://127.0.0.1:4173'
const outDir = path.resolve('screenshots')
const failures = []

// check 记录断言失败。
// @param condition 条件
// @param message 说明
// @return 无
function check(condition, message) {
  if (!condition) failures.push(message)
}

// visit 打开页面并等待网络空闲，返回页面与收集到的错误。
// @param context 浏览器上下文
// @param route 路径
// @param viewport 视口
// @return 页面、错误数组
async function visit(context, route, viewport = { width: 1440, height: 960 }) {
  const page = await context.newPage()
  await page.setViewportSize(viewport)
  const errors = []
  page.on('console', (message) => {
    if (message.type() === 'error') errors.push('console: ' + message.text())
  })
  page.on('pageerror', (error) => errors.push('pageerror: ' + error.message))
  const response = await page.goto(base + route, { waitUntil: 'networkidle' })
  check(response !== null, route + ': 无响应')
  check(response === null || response.status() < 400, route + ': HTTP ' + (response?.status() ?? 'unknown'))
  return { page, errors }
}

fs.mkdirSync(outDir, { recursive: true })
const browser = await chromium.launch()
const context = await browser.newContext({ viewport: { width: 1440, height: 960 }, reducedMotion: 'reduce', colorScheme: 'light' })

const routes = [
  { route: '/', heading: '用纯 Go' },
  { route: '/docs', heading: '从一个可验证的事实开始' },
  { route: '/progress', heading: '重构进度看板' },
  { route: '/compatibility', heading: '一项能力，四种证据' },
  { route: '/rest-explorer', heading: '找到端点，也看到差异' },
  { route: '/docs/getting-started/quickstart-rest', heading: '10 分钟跑通第一个流程' },
  { route: '/docs/migration', heading: '迁移总览与决策树' },
  { route: '/docs/sdk/config', heading: '引擎配置' },
  { route: '/releases', heading: '版本与变更' },
]

for (const item of routes) {
  const { page, errors } = await visit(context, item.route)
  const h1 = await page.locator('h1').first().textContent()
  check(h1?.includes(item.heading), item.route + ': h1 不匹配，实际 ' + h1)
  const title = await page.title()
  check(title.length > 0, item.route + ': 标题为空')
  check(errors.length === 0, item.route + ': ' + errors.join('; '))
  await page.close()
}

// 文档页正文与右侧目录
{
  const { page, errors } = await visit(context, '/docs/getting-started/quickstart-rest')
  check((await page.locator('.markdown-body').count()) === 1, '文档页缺少正文')
  check((await page.locator('.markdown-body .code-block').count()) > 3, '文档页代码块不足')
  check((await page.locator('.toc__item').count()) > 3, '文档页目录不足')
  check((await page.locator('.doc-header .status-badge').count()) === 1, '文档页缺少状态徽标')
  check(errors.length === 0, '文档页错误: ' + errors.join('; '))
  await page.screenshot({ path: path.join(outDir, 'doc-quickstart.png'), fullPage: false })
  await page.close()
}

// 搜索面板
{
  const { page, errors } = await visit(context, '/')
  await page.getByRole('button', { name: '搜索文档' }).first().click()
  await page.locator('.palette-input input').fill('迁移')
  await page.waitForTimeout(200)
  check((await page.locator('.palette-item').count()) > 0, '搜索“迁移”没有结果')
  await page.screenshot({ path: path.join(outDir, 'search.png'), fullPage: false })
  await page.keyboard.press('Enter')
  await page.waitForURL(/migration|docs/, { timeout: 5000 })
  check(page.url().includes('migration'), '搜索跳转 URL 不正确: ' + page.url())
  check(errors.length === 0, '搜索错误: ' + errors.join('; '))
  await page.close()
}

// 主题切换
{
  const { page } = await visit(context, '/docs')
  const before = await page.evaluate(() => document.documentElement.dataset.theme)
  await page.getByRole('button', { name: /切换.*主题/ }).click()
  const after = await page.evaluate(() => document.documentElement.dataset.theme)
  check(before !== after, '主题未切换')
  await page.screenshot({ path: path.join(outDir, 'docs-dark.png'), fullPage: false })
  await page.getByRole('button', { name: /切换.*主题/ }).click()
  const restored = await page.evaluate(() => document.documentElement.dataset.theme)
  check(restored === before, '主题未恢复为亮色')
  await page.close()
}

// REST 浏览器筛选
{
  const { page, errors } = await visit(context, '/rest-explorer')
  await page.locator('input[aria-label="搜索端点"]').fill('migration')
  await page.waitForTimeout(150)
  const rows = await page.locator('.endpoint-table tbody tr').count()
  check(rows > 0 && rows < 20, 'REST 筛选结果数量异常: ' + rows)
  check(errors.length === 0, 'REST 错误: ' + errors.join('; '))
  await page.screenshot({ path: path.join(outDir, 'rest-explorer.png'), fullPage: false })
  await page.close()
}

// 能力矩阵筛选
{
  const { page, errors } = await visit(context, '/compatibility')
  await page.locator('input[aria-label="搜索能力"]').fill('MySQL')
  await page.waitForTimeout(150)
  check((await page.locator('.matrix-row').count()) > 0, '能力矩阵筛选无结果')
  check(errors.length === 0, '能力矩阵错误: ' + errors.join('; '))
  await page.screenshot({ path: path.join(outDir, 'compatibility.png'), fullPage: false })
  await page.close()
}

// 移动端
{
  const { page, errors } = await visit(context, '/', { width: 390, height: 844 })
  check(await page.locator('.mobile-only').isVisible(), '移动端菜单按钮不可见')
  await page.getByRole('button', { name: '打开导航' }).click()
  check(await page.locator('.mobile-nav').isVisible(), '移动端导航未展开')
  check(errors.length === 0, '移动端错误: ' + errors.join('; '))
  await page.screenshot({ path: path.join(outDir, 'mobile-landing.png'), fullPage: false })
  await page.close()
}

// 移动端文档
{
  const { page, errors } = await visit(context, '/docs/progress/summary', { width: 390, height: 844 })
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
  check(overflow <= 1, '移动端文档横向溢出 ' + overflow + 'px')
  await page.locator('.mobile-toolbar .toolbar-button').first().click()
  check(await page.locator('.drawer--left').isVisible(), '移动端文档目录未展开')
  check(errors.length === 0, '移动端文档错误: ' + errors.join('; '))
  await page.screenshot({ path: path.join(outDir, 'mobile-docs.png'), fullPage: false })
  await page.close()
}

// 桌面截图
for (const item of [
  { route: '/', name: 'landing' },
  { route: '/progress', name: 'progress' },
  { route: '/docs', name: 'docs-index' },
]) {
  const { page } = await visit(context, item.route)
  await page.screenshot({ path: path.join(outDir, item.name + '.png'), fullPage: true })
  await page.close()
}

// 暗色落地页
{
  const darkContext = await browser.newContext({ viewport: { width: 1440, height: 960 }, colorScheme: 'dark', reducedMotion: 'reduce' })
  const page = await darkContext.newPage()
  await page.goto(base + '/', { waitUntil: 'networkidle' })
  await page.screenshot({ path: path.join(outDir, 'landing-dark.png'), fullPage: false })
  await darkContext.close()
}

// 404
{
  const { page } = await visit(context, '/docs/does-not-exist')
  check((await page.locator('.not-found').count()) === 1, '404 页面未渲染')
  await page.close()
}

await browser.close()

if (failures.length > 0) {
  console.error('E2E 失败：')
  for (const failure of failures) console.error(' - ' + failure)
  process.exit(1)
}
console.log('E2E 通过：' + routes.length + ' 个路由、搜索、主题、筛选、移动端与 404')
