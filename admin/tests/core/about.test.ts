import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import AboutPage from '@/pages/system/AboutPage.vue'
import { mockFetch, settle } from '../helpers'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('AboutPage', () => {
  it('展示构建信息与服务端引擎信息', async () => {
    mockFetch([{ path: '/service/management/engine', body: { name: 'Flowtide', version: '9.9.9', historyLevel: 'full', closed: false } }])
    const wrapper = mount(AboutPage)
    await settle()
    expect(wrapper.text()).toContain('0.1.0')
    expect(wrapper.text()).toContain('test-commit')
    expect(wrapper.text()).toContain('Flowtide')
    expect(wrapper.text()).toContain('9.9.9')
    expect(wrapper.text()).toContain('运行中')
  })

  it('接口失败时展示错误', async () => {
    mockFetch([{ path: '/service/management/engine', status: 500, body: { message: '引擎不可用' } }])
    const wrapper = mount(AboutPage)
    await settle()
    expect(wrapper.text()).toContain('引擎不可用')
  })
})
