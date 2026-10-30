import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import MetricsPage from '@/pages/system/MetricsPage.vue'
import { mockFetch, settle } from '../helpers'

const text = [
  '# HELP flowable_deployments 部署数',
  'flowable_deployments 4',
  'flowable_tasks_active 7',
  'flowable_http_requests_total{method="GET"} 12',
  'flowable_http_requests_total{method="POST"} 3',
].join('\n')

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('MetricsPage', () => {
  it('解析指标并展示摘要与分组表格', async () => {
    mockFetch([{ path: '/metrics', text }])
    const wrapper = mount(MetricsPage)
    await settle()
    expect(wrapper.text()).toContain('flowable_deployments')
    expect(wrapper.text()).toContain('method=GET')
    expect(wrapper.text()).toContain('4')
    expect(wrapper.text()).toContain('15')
  })

  it('支持过滤与原始文本切换', async () => {
    mockFetch([{ path: '/metrics', text }])
    const wrapper = mount(MetricsPage)
    await settle()
    await wrapper.find('input').setValue('POST')
    await nextTick()
    expect(wrapper.text()).toContain('POST')
    expect(wrapper.text()).not.toContain('method=GET')
    const toggle = wrapper.findAll('button').find((button) => button.text().includes('原始文本'))
    await toggle!.trigger('click')
    expect(wrapper.find('pre').text()).toContain('# HELP')
  })

  it('解析失败时回退原文', async () => {
    mockFetch([{ path: '/metrics', text: 'not a metric at all' }])
    const wrapper = mount(MetricsPage)
    await settle()
    expect(wrapper.text()).toContain('无法解析')
    expect(wrapper.find('pre').text()).toContain('not a metric at all')
  })

  it('接口失败展示错误块', async () => {
    mockFetch([{ path: '/metrics', status: 500, body: { message: '指标失败' } }])
    const wrapper = mount(MetricsPage)
    await settle()
    expect(wrapper.text()).toContain('指标失败')
  })
})