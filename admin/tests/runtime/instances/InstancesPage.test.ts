import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import InstancesPage from '@/pages/runtime/InstancesPage.vue'
import StartInstanceDialog from '@/components/StartInstanceDialog.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import { mockFetch, pageOf, settle } from '../../helpers'

/** 构造测试路由 */
function makeRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/runtime/instances', component: InstancesPage },
      { path: '/runtime/instances/:id', component: { template: '<div>detail</div>' } },
    ],
  })
}

/** 实例样例 */
const instance = {
  id: 'pi-1',
  processDefinitionId: 'leave:2:def-1',
  processDefinitionName: '请假流程',
  businessKey: 'B-1',
  startUserId: 'alice',
  startTime: '2026-10-30T03:00:00Z',
  suspended: false,
  activityId: 'usertask1',
  tenantId: 't1',
}

/** 挂载页面并等待路由就绪 */
async function mountPage(path = '/runtime/instances') {
  const router = makeRouter()
  await router.push(path)
  await router.isReady()
  const wrapper = mount(InstancesPage, { global: { plugins: [router] } })
  await settle()
  return { wrapper, router }
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('InstancesPage', () => {
  it('加载并渲染实例列表', async () => {
    mockFetch([{ path: '/service/runtime/process-instances?', body: pageOf([instance]) }])
    const { wrapper } = await mountPage()
    expect(wrapper.text()).toContain('流程实例')
    expect(wrapper.text()).toContain('B-1')
    expect(wrapper.text()).toContain('请假流程')
    expect(wrapper.text()).toContain('运行中')
    expect(wrapper.text()).toContain('usertask1')
  })

  it('空数据显示空态说明', async () => {
    mockFetch([{ path: '/service/runtime/process-instances?', body: pageOf([]) }])
    const { wrapper } = await mountPage()
    expect(wrapper.text()).toContain('没有符合条件的运行中实例')
  })

  it('接口失败展示 ErrorBlock 并支持重试', async () => {
    const calls = mockFetch([{ path: '/service/runtime/process-instances?', status: 500, body: { message: '引擎失败', exception: 'boom' } }])
    const { wrapper } = await mountPage()
    expect(wrapper.text()).toContain('引擎失败')
    await wrapper.find('button[title="重试"]').trigger('click')
    await settle()
    expect(calls.length).toBeGreaterThanOrEqual(2)
  })

  it('筛选写入路由 Query 并带上真实参数', async () => {
    const calls = mockFetch([{ path: '/service/runtime/process-instances?', body: pageOf([instance]) }])
    const { wrapper, router } = await mountPage()
    await wrapper.find('input[placeholder="业务单据号"]').setValue('B-9')
    const buttons = wrapper.findAll('button').filter((item) => item.text() === '查询')
    await buttons[0].trigger('click')
    await settle()
    const last = calls[calls.length - 1].url
    expect(last).toContain('businessKey=B-9')
    expect(router.currentRoute.value.query.businessKey).toBe('B-9')
  })

  it('同一筛选条件下仅未挂起映射为 suspended=false', async () => {
    const calls = mockFetch([{ path: '/service/runtime/process-instances?', body: pageOf([instance]) }])
    const { wrapper } = await mountPage()
    const checkbox = wrapper.find('.n-checkbox')
    await checkbox.trigger('click')
    await settle()
    const last = calls[calls.length - 1].url
    expect(last).toContain('suspended=false')
  })

  it('启动实例成功后跳转详情', async () => {
    mockFetch([
      { path: '/service/runtime/process-instances', method: 'POST', status: 201, body: { id: 'pi-2' } },
      { path: '/service/repository/process-definitions?', body: pageOf([{ id: 'def-1', key: 'leave', name: '请假', version: 2 }]) },
      { path: '/service/runtime/process-instances?', body: pageOf([instance]) },
    ])
    const { wrapper, router } = await mountPage()
    const start = wrapper.findAll('button').find((item) => item.text() === '启动实例')
    await start?.trigger('click')
    await settle()
    const dialog = wrapper.findComponent(StartInstanceDialog)
    expect(dialog.exists()).toBe(true)
    dialog.vm.$emit('submit', { processDefinitionId: 'def-1', businessKey: 'B-2', variables: { amount: 1 } })
    await settle()
    expect(router.currentRoute.value.path).toBe('/runtime/instances/pi-2')
  })

  it('启动实例失败保留弹窗与错误原文', async () => {
    mockFetch([
      { path: '/service/runtime/process-instances', method: 'POST', status: 400, body: { message: '变量类型非法' } },
      { path: '/service/repository/process-definitions?', body: pageOf([{ id: 'def-1', key: 'leave', version: 1 }]) },
      { path: '/service/runtime/process-instances?', body: pageOf([instance]) },
    ])
    const { wrapper } = await mountPage()
    const start = wrapper.findAll('button').find((item) => item.text() === '启动实例')
    await start?.trigger('click')
    await settle()
    wrapper.findComponent(StartInstanceDialog).vm.$emit('submit', { processDefinitionId: 'def-1', variables: {} })
    await settle()
    expect(wrapper.text()).toContain('启动失败')
    expect(wrapper.text()).toContain('变量类型非法')
    expect(wrapper.findComponent(StartInstanceDialog).props('show')).toBe(true)
  })

  it('删除实例成功与失败都有反馈', async () => {
    const calls = mockFetch([
      { path: '/service/runtime/process-instances/pi-1', method: 'DELETE', status: 204 },
      { path: '/service/runtime/process-instances?', body: pageOf([instance]) },
    ])
    const { wrapper } = await mountPage()
    const confirm = wrapper.findAllComponents(ConfirmButton)[0]
    confirm.vm.$emit('confirm')
    await settle()
    expect(calls.some((call) => call.method === 'DELETE')).toBe(true)
    expect(wrapper.text()).toContain('已删除实例')
  })
})