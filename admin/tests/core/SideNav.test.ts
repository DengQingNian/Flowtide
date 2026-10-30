import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import SideNav from '@/components/SideNav.vue'
import { routes } from '@/router'

// 创建使用内存历史的测试路由。
// @param path 初始路径
// @return 已就绪的路由实例
async function createTestRouter(path: string) {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push(path)
  await router.isReady()
  return router
}

describe('SideNav', () => {
  it('渲染七个分组与当前项高亮', async () => {
    const router = await createTestRouter('/dashboard')
    const wrapper = mount(SideNav, { props: { collapsed: false }, global: { plugins: [router] } })
    for (const group of ['概览', '仓库', '运行态', '运维', '历史', '身份', '系统']) {
      expect(wrapper.text()).toContain(group)
    }
    expect(wrapper.find('.side-nav__link--active').text()).toContain('仪表盘')
  })

  it('折叠分组后隐藏子项', async () => {
    const router = await createTestRouter('/dashboard')
    const wrapper = mount(SideNav, { props: { collapsed: false }, global: { plugins: [router] } })
    const groupButton = wrapper.findAll('.side-nav__group-title').find((button) => button.text().includes('仓库'))
    await groupButton!.trigger('click')
    expect(wrapper.text()).not.toContain('流程模型')
    await groupButton!.trigger('click')
    expect(wrapper.text()).toContain('流程模型')
  })

  it('整体折叠时隐藏文字并保留导航', async () => {
    const router = await createTestRouter('/dashboard')
    const wrapper = mount(SideNav, { props: { collapsed: true }, global: { plugins: [router] } })
    expect(wrapper.classes()).toContain('side-nav--collapsed')
    expect(wrapper.findAll('.side-nav__group-title')).toHaveLength(0)
    expect(wrapper.findAll('.side-nav__link').length).toBeGreaterThan(10)
  })
})