import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount } from '@vue/test-utils'
import DefinitionsPage from '@/pages/repository/DefinitionsPage.vue'
import { mockFetch, pageOf, settle } from '../helpers'
import { findButton, mountPage } from './testUtils'

vi.setConfig({ testTimeout: 20000 })
enableAutoUnmount(afterEach)
afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

// 构造流程定义列表。
// @return 定义数组
function definitions() {
  return [
    { id: 'def1', key: 'leave', name: '请假', version: 1, deploymentId: 'd1', tenantId: 't1', suspended: true },
    { id: 'def2', key: 'leave', name: '请假', version: 2, deploymentId: 'd1', tenantId: 't1', suspended: true },
    { id: 'def3', key: 'leave', name: '请假', version: 3, deploymentId: 'd2', tenantId: 't1', suspended: false },
    { id: 'def4', key: 'expense', name: '报销', version: 1, deploymentId: 'd3', tenantId: 't2', suspended: true },
  ]
}

// 构造定义列表路由。
// @param overrides 覆盖项
// @return 模拟路由
function routes(overrides: { list?: unknown; status?: number } = {}) {
  return [{ path: '/service/repository/process-definitions', status: overrides.status ?? 200, body: overrides.status ? { message: '定义读取失败' } : (overrides.list ?? pageOf(definitions())) }]
}

describe('DefinitionsPage', () => {
  it('默认按 Key 只展示最新版本并标记版本数', async () => {
    mockFetch(routes())
    const { wrapper } = await mountPage(DefinitionsPage, '/repository/definitions')
    await settle()
    expect(wrapper.text()).toContain('leave')
    expect(wrapper.text()).toContain('最新 v3')
    expect(wrapper.text()).toContain('共 3 个版本')
    expect(wrapper.text()).toContain('expense')
    expect(wrapper.text()).not.toContain('请假v1')
  })

  it('全部版本模式展示历史版本与挂起状态', async () => {
    mockFetch(routes())
    const { wrapper } = await mountPage(DefinitionsPage, '/repository/definitions?latestVersion=false')
    await settle()
    expect(wrapper.text()).toContain('v2')
    expect(wrapper.text()).toContain('挂起')
    expect(wrapper.text()).toContain('最新 v3')
  })

  it('按 Key、名称、状态、部署与租户筛选', async () => {
    mockFetch(routes())
    const first = await mountPage(DefinitionsPage, '/repository/definitions?key=leave&nameLike=请&deploymentId=d2&tenantId=t1&suspended=active')
    await settle()
    expect(first.wrapper.text()).toContain('leave')
    expect(first.wrapper.text()).not.toContain('expense')
    first.wrapper.unmount()
    mockFetch(routes())
    const second = await mountPage(DefinitionsPage, '/repository/definitions?suspended=suspended')
    await settle()
    expect(second.wrapper.text()).toContain('expense')
    expect(second.wrapper.text()).not.toContain('leave')
  })

  it('查询按钮写入路由', async () => {
    mockFetch(routes())
    const { wrapper, router } = await mountPage(DefinitionsPage, '/repository/definitions?page=2')
    await settle()
    const inputs = wrapper.findAll('input')
    await inputs[0].setValue('expense')
    findButton('查询', wrapper.element).click()
    await settle()
    expect(router.currentRoute.value.query.key).toBe('expense')
    expect(router.currentRoute.value.query.page).toBeUndefined()
    expect(wrapper.text()).toContain('expense')
    expect(wrapper.text()).not.toContain('leave')
  })

  it('空数据展示空态', async () => {
    mockFetch(routes({ list: pageOf([]) }))
    const { wrapper } = await mountPage(DefinitionsPage, '/repository/definitions')
    await settle()
    expect(wrapper.text()).toContain('没有匹配的流程定义')
  })

  it('接口错误展示 ErrorBlock', async () => {
    mockFetch(routes({ status: 500 }))
    const { wrapper } = await mountPage(DefinitionsPage, '/repository/definitions')
    await settle()
    expect(wrapper.text()).toContain('定义读取失败')
    expect(wrapper.find('button[title="重试"]').exists()).toBe(true)
  })

  it('点击名称进入定义详情', async () => {
    mockFetch(routes())
    const { wrapper, router } = await mountPage(DefinitionsPage, '/repository/definitions')
    await settle()
    findButton('请假', wrapper.element).click()
    await settle()
    expect(router.currentRoute.value.fullPath).toBe('/repository/definitions/def3')
  })
})
