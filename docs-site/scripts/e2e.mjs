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
  { route: '/', heading: 'Flowtide 文档' },
  { route: '/docs', heading: '文档中心' },
  { route: '/progress', heading: '重构进度看板' },
  { route: '/compatibility', heading: '能力矩阵' },
  { route: '/rest-explorer', heading: 'REST 端点查询' },
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

// 搜索快捷键只处理一次，打开和关闭后恢复正文滚动。
{
  const { page } = await visit(context, '/docs')
  await page.keyboard.press('Control+k')
  check(await page.getByRole('dialog', { name: '搜索文档' }).isVisible(), '搜索快捷键未打开面板')
  await page.keyboard.press('Escape')
  await page.getByRole('dialog').waitFor({ state: 'hidden' })
  check((await page.getByRole('dialog').count()) === 0, 'Esc 未关闭搜索')
  check((await page.evaluate(() => document.body.style.overflow)) !== 'hidden', '搜索关闭后正文仍被锁定')
  await page.close()
}

// 阅读块悬停只改变背景和手柄透明度，位置、边框与阴影保持稳定。
{
  const { page } = await visit(context, '/')
  const card = page.locator('.feature-card').first()
  await card.scrollIntoViewIfNeeded()
  const before = await card.boundingBox()
  const initial = await card.evaluate((element) => {
    const style = getComputedStyle(element)
    return { border: style.borderColor, shadow: style.boxShadow }
  })
  await card.hover()
  await page.waitForTimeout(180)
  const after = await card.boundingBox()
  const hovered = await card.evaluate((element) => {
    const style = getComputedStyle(element)
    return {
      border: style.borderColor,
      shadow: style.boxShadow,
      background: style.backgroundColor,
      transform: style.transform,
      handle: getComputedStyle(element.querySelector('.block-handle')).opacity,
    }
  })
  check(before?.x === after?.x && before?.y === after?.y && before?.width === after?.width, '阅读块悬停时发生位移')
  check(initial.border === hovered.border && initial.shadow === hovered.shadow, '阅读块悬停时边框或阴影变化')
  check(hovered.background === 'rgb(239, 237, 234)' && hovered.transform === 'none', '阅读块悬停样式不符合约定')
  check(hovered.handle === '1', '阅读块悬停未显示手柄')
  await page.close()
}

// 不同视口下只允许表格和代码块内部横向滚动，页面本身不能溢出。
for (const width of [320, 390, 768, 1280]) {
  for (const route of ['/', '/docs', '/progress', '/compatibility', '/rest-explorer', '/docs/getting-started/quickstart-rest']) {
    const { page } = await visit(context, route, { width, height: 900 })
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
    check(overflow <= 1, route + ': ' + width + 'px 视口横向溢出 ' + overflow + 'px')
    await page.close()
  }
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
console.log('E2E 通过：' + routes.length + ' 个路由、搜索、主题、筛选、移动端、404、悬停稳定性与四种视口下的六个页面')
