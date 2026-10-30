import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import EmptyState from '@/components/EmptyState.vue'
import ErrorBlock from '@/components/ErrorBlock.vue'
import JsonViewer from '@/components/JsonViewer.vue'
import XmlViewer from '@/components/XmlViewer.vue'
import FilterBar from '@/components/FilterBar.vue'
import StatCard from '@/components/StatCard.vue'
import { ApiError } from '@/api/client'

describe('EmptyState', () => {
  it('渲染标题、说明与默认插槽', () => {
    const wrapper = mount(EmptyState, { props: { title: '空', description: '无数据' }, slots: { default: '<button>新建</button>' } })
    expect(wrapper.text()).toContain('空')
    expect(wrapper.text()).toContain('无数据')
    expect(wrapper.find('button').text()).toBe('新建')
  })

  it('使用默认文案', () => {
    expect(mount(EmptyState).text()).toContain('暂无数据')
  })
})

describe('ErrorBlock', () => {
  it('展示服务端错误、错误码与异常，并支持重试', async () => {
    const error = new ApiError(500, '引擎失败', 'ENGINE-1', 'stack trace')
    const wrapper = mount(ErrorBlock, { props: { error, context: 'GET /x' } })
    expect(wrapper.text()).toContain('引擎失败')
    expect(wrapper.text()).toContain('ENGINE-1')
    expect(wrapper.text()).toContain('stack trace')
    await wrapper.find('button[title="重试"]').trigger('click')
    expect(wrapper.emitted('retry')).toBeTruthy()
  })

  it('普通错误也能展示', () => {
    expect(mount(ErrorBlock, { props: { error: new Error('普通错误') } }).text()).toContain('普通错误')
  })
})

describe('JsonViewer', () => {
  it('格式化并复制 JSON', () => {
    const wrapper = mount(JsonViewer, { props: { value: { a: 1 }, title: '数据' } })
    expect(wrapper.text()).toContain('数据')
    expect(wrapper.find('pre').text()).toContain('"a": 1')
  })

  it('超过阈值可折叠与展开', async () => {
    const wrapper = mount(JsonViewer, { props: { value: { a: 'x'.repeat(50) }, collapseThreshold: 10 } })
    expect(wrapper.find('pre').text()).toContain('已折叠')
    await wrapper.find('button').trigger('click')
    expect(wrapper.find('pre').text()).toContain('xxxxxxxxxx')
  })
})

describe('XmlViewer', () => {
  it('展示行数并下载', async () => {
    const wrapper = mount(XmlViewer, { props: { xml: '<a>\n</a>', filename: 'a.bpmn' } })
    expect(wrapper.text()).toContain('2 行')
    await wrapper.find('button[title="下载 XML"]').trigger('click')
  })
})

describe('FilterBar', () => {
  it('触发查询与重置', async () => {
    const wrapper = mount(FilterBar, { slots: { default: '<input />' } })
    const buttons = wrapper.findAll('button')
    await buttons[0].trigger('click')
    await buttons[1].trigger('click')
    expect(wrapper.emitted('search')).toBeTruthy()
    expect(wrapper.emitted('reset')).toBeTruthy()
  })

  it('可隐藏重置按钮', () => {
    const wrapper = mount(FilterBar, { props: { showReset: false } })
    expect(wrapper.findAll('button')).toHaveLength(1)
  })
})

describe('StatCard', () => {
  it('渲染标签、值与提示', () => {
    const wrapper = mount(StatCard, { props: { label: '部署', value: 3, hint: '点击查看' } })
    expect(wrapper.text()).toContain('部署')
    expect(wrapper.text()).toContain('3')
    expect(wrapper.text()).toContain('点击查看')
  })

  it('有 to 时渲染为链接', async () => {
    const wrapper = mount(StatCard, {
      props: { label: '部署', value: 3, to: '/repository/deployments' },
      global: { stubs: { RouterLink: { template: '<a class="link-stub"><slot /></a>' } } },
    })
    expect(wrapper.find('.link-stub').exists()).toBe(true)
  })
})