import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import InstanceDetailPage from '@/pages/runtime/InstanceDetailPage.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import { mockFetch, pageOf, settle } from '../../helpers'

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

/** 构造测试路由 */
function makeRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/runtime/instances', component: { template: '<div>list</div>' } },
      { path: '/runtime/instances/:id', component: InstanceDetailPage },
    ],
  })
}

/** 挂载详情页 */
async function mountPage() {
  const router = makeRouter()
  await router.push('/runtime/instances/pi-1')
  await router.isReady()
  const wrapper = mount(InstanceDetailPage, { global: { plugins: [router] } })
  await settle()
  return { wrapper, router }
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('InstanceDetailPage', () => {
  it('加载并展示概览字段', async () => {
    mockFetch([{ path: '/service/runtime/process-instances/pi-1', body: instance }])
    const { wrapper } = await mountPage()
    expect(wrapper.text()).toContain('请假流程')
    expect(wrapper.text()).toContain('alice')
    expect(wrapper.text()).toContain('B-1')
    expect(wrapper.text()).toContain('运行中')
    expect(wrapper.text()).toContain('leave / v2')
  })

  it('详情不存在时展示 ErrorBlock 与错误原文', async () => {
    mockFetch([{ path: '/service/runtime/process-instances/pi-1', status: 404, body: { message: 'Not found', exception: 'gone' } }])
    const { wrapper } = await mountPage()
    expect(wrapper.text()).toContain('Not found')
    expect(wrapper.text()).toContain('gone')
    expect(wrapper.find('button[title="重试"]').exists()).toBe(true)
  })

  it('挂起实例成功后刷新状态', async () => {
    const calls = mockFetch([
      { path: '/service/runtime/process-instances/pi-1', method: 'PUT', body: { ...instance, suspended: true } },
      { path: '/service/runtime/process-instances/pi-1', body: instance },
    ])
    const { wrapper } = await mountPage()
    const confirm = wrapper.findAllComponents(ConfirmButton)[0]
    confirm.vm.$emit('confirm')
    await settle()
    expect(calls.some((call) => call.method === 'PUT')).toBe(true)
    const put = calls.find((call) => call.method === 'PUT')
    expect(put?.body).toEqual({ action: 'suspend' })
    expect(wrapper.text()).toContain('实例已挂起')
  })

  it('挂起失败保留页面并展示服务端错误', async () => {
    mockFetch([
      { path: '/service/runtime/process-instances/pi-1', method: 'PUT', status: 409, body: { message: 'Process instance is already suspended.' } },
      { path: '/service/runtime/process-instances/pi-1', body: instance },
    ])
    const { wrapper } = await mountPage()
    wrapper.findAllComponents(ConfirmButton)[0].vm.$emit('confirm')
    await settle()
    expect(wrapper.text()).toContain('already suspended')
    expect(wrapper.text()).toContain('请假流程')
  })

  it('删除实例需要填写原因并返回列表', async () => {
    const calls = mockFetch([
      { path: '/service/runtime/process-instances/pi-1', method: 'DELETE', status: 204 },
      { path: '/service/runtime/process-instances/pi-1', body: instance },
    ])
    const { wrapper, router } = await mountPage()
    const deleteButton = wrapper.findAll('button').find((item) => item.text() === '删除实例')
    await deleteButton?.trigger('click')
    await settle()
    const textarea = document.querySelector('textarea') as HTMLTextAreaElement
    textarea.value = '业务取消'
    textarea.dispatchEvent(new Event('input'))
    const confirms = wrapper.findAllComponents(ConfirmButton)
    confirms[confirms.length - 1].vm.$emit('confirm')
    await settle()
    expect(calls.some((call) => call.method === 'DELETE')).toBe(true)
    expect(router.currentRoute.value.path).toBe('/runtime/instances')
  })

  it('切换任务 Tab 时懒加载任务列表', async () => {
    mockFetch([
      { path: '/service/runtime/process-instances/pi-1/tasks', body: pageOf([{ id: 't-1', name: '审批任务', assignee: 'bob', taskDefinitionKey: 'usertask1', priority: 50 }]) },
      { path: '/service/runtime/process-instances/pi-1', body: instance },
    ])
    const { wrapper } = await mountPage()
    const tab = wrapper.findAll('.n-tabs-tab').find((item) => item.text().includes('任务'))
    expect(tab).toBeTruthy()
    await tab?.trigger('click')
    await settle()
    expect(wrapper.text()).toContain('运行中任务')
    expect(wrapper.text()).toContain('审批任务')
    expect(wrapper.text()).toContain('bob')
  })

  it('切换变量 Tab 时加载变量', async () => {
    mockFetch([
      { path: '/service/runtime/process-instances/pi-1/variables', body: [{ name: 'amount', value: 100, type: 'long' }] },
      { path: '/service/runtime/process-instances/pi-1', body: instance },
    ])
    const { wrapper } = await mountPage()
    const tab = wrapper.findAll('.n-tabs-tab').find((item) => item.text().includes('变量'))
    await tab?.trigger('click')
    await settle()
    expect(wrapper.text()).toContain('amount')
    expect(wrapper.text()).toContain('100')
  })
})