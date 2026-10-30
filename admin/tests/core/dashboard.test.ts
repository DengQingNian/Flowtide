import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import DashboardPage from '@/pages/dashboard/DashboardPage.vue'
import { mockFetch, pageOf, settle } from '../helpers'

const engine = { name: 'Flowtide', version: '1.2.3', historyLevel: 'audit', closed: false }
const properties = { 'schema.version': '8.0', tenantId: 't-1', 'storage.mode': 'memory' }
const deployments = pageOf([{ id: 'd-1', name: '请假流程', tenantId: 't-1', deployTime: '2026-10-30T01:00:00Z' }], 4)
const definitions = pageOf([], 9)
const instances = pageOf([{ id: 'pi-1', processDefinitionName: '请假流程', startTime: '2026-10-30T02:00:00Z' }], 3)
const tasks = pageOf([], 7)
const deadletters = pageOf([{ id: 'j-1', executionId: 'ex-1', exceptionMessage: 'boom' }], 2)

// 安装仪表盘所需路由。
// @return 无
function installRoutes(): void {
  mockFetch([
    { path: '/service/management/engine-properties', body: properties },
    { path: '/service/management/engine', body: engine },
    { path: '/service/repository/deployments', body: deployments },
    { path: '/service/repository/process-definitions', body: definitions },
    { path: '/service/runtime/process-instances', body: instances },
    { path: '/service/runtime/tasks', body: tasks },
    { path: '/service/management/deadletter-jobs', body: deadletters },
    { path: '/healthz', body: { status: 'UP' } },
    { path: '/readyz', body: { status: 'UP' } },
    { path: '/metrics', text: 'flowable_deployments 4\nflowable_tasks_active 7\n' },
  ])
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('DashboardPage', () => {
  it('展示引擎、计数、最近部署/实例/死信与指标', async () => {
    installRoutes()
    const wrapper = mount(DashboardPage, { global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } } })
    await settle()
    expect(wrapper.text()).toContain('Flowtide')
    expect(wrapper.text()).toContain('8.0')
    expect(wrapper.text()).toContain('memory')
    expect(wrapper.text()).toContain('请假流程')
    expect(wrapper.text()).toContain('boom')
    expect(wrapper.text()).toContain('活动任务')
    expect(wrapper.text()).toContain('关键指标')
  })

  it('部分接口失败时仍渲染其他卡片', async () => {
    mockFetch([
      { path: '/service/management/engine', status: 500, body: { message: '引擎失败' } },
      { path: '/service/management/engine-properties', status: 500, body: {} },
      { path: '/service/repository/deployments', status: 500, body: {} },
      { path: '/service/repository/process-definitions', status: 500, body: {} },
      { path: '/service/runtime/process-instances', status: 500, body: {} },
      { path: '/service/runtime/tasks', status: 500, body: {} },
      { path: '/service/management/deadletter-jobs', status: 500, body: {} },
      { path: '/healthz', status: 500, body: {} },
      { path: '/readyz', status: 500, body: {} },
      { path: '/metrics', status: 500, text: '' },
    ])
    const wrapper = mount(DashboardPage, { global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } } })
    await settle()
    expect(wrapper.text()).toContain('引擎信息读取失败')
    expect(wrapper.text()).toContain('健康检查未通过')
    expect(wrapper.text()).toContain('暂无部署')
  })

  it('刷新按钮重新加载', async () => {
    installRoutes()
    const wrapper = mount(DashboardPage, { global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } } })
    await settle()
    await wrapper.findAll('button')[0].trigger('click')
    await settle()
    expect(wrapper.text()).toContain('Flowtide')
  })
})