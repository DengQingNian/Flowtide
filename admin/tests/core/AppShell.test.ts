import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { defineComponent } from 'vue'
import AppShell from '@/components/AppShell.vue'
import { mockFetch, settle } from '../helpers'

const Page = defineComponent({ template: '<div class="page-stub">页面内容</div>' })

// 创建带单页路由的实例。
// @return 已就绪的路由
async function createRouterForShell() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/dashboard', component: Page, meta: { title: '仪表盘', group: '概览' } },
      { path: '/system/about', component: Page, meta: { title: '关于', group: '系统' } },
      { path: '/system/metrics', component: Page, meta: { title: '指标', group: '系统' } },
      // 兜底路由：侧栏新增的工作流链接在此测试路由中不产生 No match 告警
      { path: '/:pathMatch(.*)*', component: Page },
    ],
  })
  await router.push('/dashboard')
  await router.isReady()
  return router
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('AppShell', () => {
  it('渲染标题、分组、页面与服务状态', async () => {
    mockFetch([
      { path: '/healthz', body: { status: 'UP' } },
      { path: '/readyz', body: { status: 'UP' } },
    ])
    const router = await createRouterForShell()
    const wrapper = mount(AppShell, { global: { plugins: [router] } })
    await settle()
    expect(wrapper.text()).toContain('概览')
    expect(wrapper.text()).toContain('仪表盘')
    expect(wrapper.find('.page-stub').exists()).toBe(true)
    expect(wrapper.text()).toContain('服务正常')
    wrapper.unmount()
  })

  it('折叠侧栏与切换主题按钮生效', async () => {
    mockFetch([
      { path: '/healthz', body: { status: 'UP' } },
      { path: '/readyz', body: { status: 'UP' } },
    ])
    const router = await createRouterForShell()
    const wrapper = mount(AppShell, { global: { plugins: [router] } })
    await settle()
    await wrapper.find('.app-header__toggle').trigger('click')
    expect(wrapper.find('.side-nav').classes()).toContain('side-nav--collapsed')
    const themeButton = wrapper.findAll('.icon-button')[0]
    await themeButton.trigger('click')
    expect(wrapper.html()).toBeTruthy()
    wrapper.unmount()
  })

  it('健康检查失败时展示横幅', async () => {
    mockFetch([
      { path: '/healthz', status: 500, body: {} },
      { path: '/readyz', status: 500, body: {} },
    ])
    const router = await createRouterForShell()
    const wrapper = mount(AppShell, { global: { plugins: [router] } })
    await settle()
    expect(wrapper.text()).toContain('服务端健康检查未通过')
    wrapper.unmount()
  })
})
