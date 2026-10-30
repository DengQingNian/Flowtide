import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { readFileSync } from 'node:fs'

const feedback = vi.hoisted(() => {
  window.matchMedia = vi.fn().mockReturnValue({ matches: false, addListener: vi.fn(), removeListener: vi.fn() })
  const stored = new Map<string, string>()
  const storage = {
    getItem: vi.fn((key: string) => stored.get(key) ?? null),
    setItem: vi.fn((key: string, value: string) => stored.set(key, value)),
  }
  Object.defineProperty(window, 'localStorage', { configurable: true, value: storage })
  Object.defineProperty(globalThis, 'localStorage', { configurable: true, value: storage })
  return { success: vi.fn(), error: vi.fn() }
})

vi.mock('naive-ui', () => ({
  NConfigProvider: { template: '<div><slot /></div>' },
  NMessageProvider: { template: '<div><slot /></div>' },
  useMessage: () => feedback,
  darkTheme: {},
  dateZhCN: {},
  zhCN: {},
}))

import App from '@/App.vue'
import router from '@/router'
import { closePalette, openPalette } from '@/lib/palette'
import { initTheme } from '@/lib/theme'
import { milestones } from '@/lib/milestones'
import { pages } from '@/lib/content'

let wrapper: VueWrapper
const writeText = vi.fn().mockResolvedValue(undefined)

// openSite 在真实路由和布局中挂载站点，便于验证跨组件交互。
// @param path 需要打开的站内路径
// @return 完成异步渲染的根组件
async function openSite(path: string): Promise<VueWrapper> {
  await router.push(path)
  await router.isReady()
  wrapper = mount(App, { attachTo: document.body, global: { plugins: [router] } })
  await flushPromises()
  return wrapper
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.stubGlobal('scrollTo', vi.fn())
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
  initTheme()
})

afterEach(() => {
  closePalette()
  wrapper?.unmount()
  document.body.innerHTML = ''
  document.body.style.overflow = ''
  vi.unstubAllGlobals()
})

describe('文档站阅读与导航', () => {
  it('首页提供直接可用的阅读入口并保留原型限制', async () => {
    const site = await openSite('/')
    expect(site.get('h1').text()).toBe('Flowtide 文档')
    expect(site.get('nav[aria-label="开始阅读"]').findAll('.reading-link')).toHaveLength(3)
    expect(site.text()).toContain('单执行者')
    expect(site.text()).toContain('尚未')
    expect(site.findAll('.phase-card')).toHaveLength(milestones.length)
    expect(site.find('.hero__glow').exists()).toBe(false)
    expect(site.get('.block-handle').attributes('aria-hidden')).toBe('true')
  })

  it('切换示例后复制对应代码，并处理剪贴板拒绝', async () => {
    const site = await openSite('/')
    const buttons = site.findAll('.code-tabs__buttons button')
    for (const [index, text] of ['curl', 'flowtide.New', 'mysql-rows'].entries()) {
      await buttons[index].trigger('click')
      expect(site.get('.code-tabs__code').text()).toContain(text)
      await site.get('.copy-inline').trigger('click')
      expect(writeText).toHaveBeenLastCalledWith(expect.stringContaining(text))
    }
    expect(feedback.success).toHaveBeenCalledWith('示例已复制')
    writeText.mockRejectedValueOnce(new Error('拒绝访问'))
    await site.get('.copy-inline').trigger('click')
    expect(feedback.error).toHaveBeenCalledWith('浏览器未允许剪贴板访问')
  })

  it('文档中心从实际内容生成数量，并能打开推荐文档', async () => {
    const site = await openSite('/docs')
    expect(site.get('h1').text()).toBe('文档中心')
    expect(site.get('.index-hero > p').text()).toContain(String(pages.length))
    await site.get('.index-search').trigger('click')
    expect(document.querySelector('[role="dialog"]')).not.toBeNull()
    closePalette()
    await site.get('.path-card').trigger('click')
    await vi.waitFor(() => expect(router.currentRoute.value.path).toBe('/docs/getting-started/quickstart-rest'))
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/docs/getting-started/quickstart-rest')
    expect(site.get('.markdown-body').text()).toContain('alice')
  })

  it('进度页按实际里程碑统计阶段状态', async () => {
    const site = await openSite('/progress')
    expect(site.findAll('.stage-card')).toHaveLength(milestones.length)
    expect(site.get('#stages .muted').text()).toBe('10 个阶段 · 1 个部分完成 · 2 个未完成')
    expect(site.get('.dash-hero__badge').text()).toContain('构建时更新')
  })

  it('移动导航随路由关闭，主题切换写入本地存储', async () => {
    const site = await openSite('/')
    await site.get('[aria-label="打开导航"]').trigger('click')
    expect(site.find('.mobile-nav').exists()).toBe(true)
    await site.get('.mobile-nav a').trigger('click')
    await flushPromises()
    expect(site.find('.mobile-nav').exists()).toBe(false)
    const previous = document.documentElement.dataset.theme
    await site.get('button[aria-label*="主题"]').trigger('click')
    expect(document.documentElement.dataset.theme).not.toBe(previous)
    expect(localStorage.getItem('flowtide-docs-theme')).toBe(document.documentElement.dataset.theme)
  })

  it('文档侧栏可折叠，移动目录和页内目录支持关闭', async () => {
    const site = await openSite('/docs/getting-started/quickstart-rest')
    const head = site.get('.sidebar-group__head')
    await head.trigger('click')
    await head.trigger('click')
    const toolbar = site.findAll('.mobile-toolbar button')
    await toolbar[0].trigger('click')
    expect(document.querySelector('.drawer--left')).not.toBeNull()
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
    expect(document.querySelector('.drawer--left')).toBeNull()
    await toolbar[1].trigger('click')
    expect(document.querySelector('.drawer--right')).not.toBeNull()
    const heading = site.get('.markdown-body h2').element as HTMLElement
    vi.spyOn(heading, 'getBoundingClientRect').mockReturnValue({ top: 100 } as DOMRect)
    await site.get('.toc__item a').trigger('click')
    expect(window.scrollTo).toHaveBeenCalled()
    await site.get('.toc__top').trigger('click')
    expect(window.scrollTo).toHaveBeenLastCalledWith({ top: 0, behavior: 'smooth' })
    window.dispatchEvent(new Event('scroll'))
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
  })

  it('能力矩阵支持关键词、状态筛选和详情展开', async () => {
    const site = await openSite('/compatibility')
    expect(site.get('h1').text()).toBe('能力矩阵')
    await site.get('input[aria-label="搜索能力"]').setValue('MySQL')
    expect(site.findAll('.matrix-row').length).toBeGreaterThan(0)
    await site.get('.matrix-row').trigger('click')
    expect(site.find('.matrix-detail').exists()).toBe(true)
    await site.get('.matrix-row').trigger('click')
    expect(site.find('.matrix-detail').exists()).toBe(false)
    await site.get('select').setValue('restricted')
    await site.get('input').setValue('不存在的能力xyz')
    expect(site.findAll('.matrix-row')).toHaveLength(0)
    await site.get('.field-clear').trigger('click')
    expect(site.findAll('.matrix-row').length).toBeGreaterThan(0)
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    URL.createObjectURL = vi.fn().mockReturnValue('blob:matrix')
    URL.revokeObjectURL = vi.fn()
    await site.get('.matrix-download').trigger('click')
    expect(click).toHaveBeenCalled()
    expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:matrix')
    click.mockRestore()
  })

  it('REST 筛选、分页与复制保留正确的请求路径', async () => {
    const site = await openSite('/rest-explorer')
    expect(site.get('h1').text()).toBe('REST 端点查询')
    await site.get('[aria-label="下一页"]').trigger('click')
    expect(site.get('.pagination').text()).toContain('第 2 /')
    await site.get('[aria-label="上一页"]').trigger('click')
    await site.get('[aria-label="搜索端点"]').setValue('migration')
    expect(site.get('.pagination').text()).toContain('第 1 /')
    await site.get('.copy-icon').trigger('click')
    expect(writeText).toHaveBeenLastCalledWith(expect.stringMatching(/^[A-Z]+ \/service\//))
    writeText.mockRejectedValueOnce(new Error('拒绝访问'))
    await site.get('.copy-icon').trigger('click')
    expect(feedback.error).toHaveBeenCalled()
    await site.get('.field-clear').trigger('click')
    await site.get('[aria-label="按方法筛选"]').setValue('POST')
    await site.get('[aria-label="按业务域筛选"]').setValue('runtime')
    for (const button of site.findAll('.segmented button')) await button.trigger('click')
    await site.get('[aria-label="搜索端点"]').setValue('不存在xyz')
    expect(site.findAll('.endpoint-table tbody tr')).toHaveLength(1)
    expect(site.get('.matrix-empty').text()).toContain('没有匹配')
    await site.get('.rest-toolbar > .button').trigger('click')
    expect(site.findAll('.endpoint-table tbody tr')).toHaveLength(20)
  })

  it('搜索支持快捷键、结果选择、空结果和路由跳转', async () => {
    await openSite('/docs')
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'k', ctrlKey: true }))
    await flushPromises()
    expect(document.querySelector('[role="dialog"]')).not.toBeNull()
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown' }))
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowUp' }))
    await flushPromises()
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter' }))
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/progress')
    openPalette()
    await flushPromises()
    const input = document.querySelector('.palette-input input') as HTMLInputElement
    input.value = 'xyz不存在'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()
    expect(document.querySelector('.palette-empty')?.textContent).toContain('没有找到')
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
    expect(document.querySelector('[role="dialog"]')).toBeNull()
  })

  it('文档代码复制提供反馈，未知页面给出返回入口', async () => {
    const site = await openSite('/docs/getting-started/quickstart-rest')
    await site.get('.copy-btn').trigger('click')
    expect(feedback.success).toHaveBeenCalledWith('代码已复制')
    writeText.mockRejectedValueOnce(new Error('拒绝访问'))
    await site.get('.copy-btn').trigger('click')
    expect(feedback.error).toHaveBeenCalled()
    await router.push('/releases')
    await flushPromises()
    expect(site.get('h1').text()).toBe('版本与变更')
    await router.push('/docs/does-not-exist')
    await flushPromises()
    expect(site.find('.not-found').exists()).toBe(true)
  })
})

describe('Notion 风格约束', () => {
  it('统一米色背景和中性色文字，移除渐变及位移动画', () => {
    const css = readFileSync('src/styles/main.css', 'utf8')
    expect(css).toContain('--bg-soft: #f7f6f3')
    expect(css).toContain('--text: #37352f')
    expect(css).not.toMatch(/(?:linear|radial)-gradient\(/)
    expect(css).not.toMatch(/\b(?:translate[XY]?|scale)\(/)
    expect(css).not.toMatch(/border-radius:\s*(?:1[2-9]|[2-9]\d|100)px/)
  })
})
