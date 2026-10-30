import { afterEach, describe, expect, it, vi } from 'vitest'
import HistoricProcessDetailPage from '@/pages/history/HistoricProcessDetailPage.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import { authNotice } from '@/stores/ui'
import { mockFetch, pageOf, settle } from '../helpers'
import { clickButton, mountAt } from './support'

/** 历史流程详情样本 */
const DETAIL = {
  id: 'pi-1',
  processDefinitionId: 'leave:1:1',
  processDefinitionName: '请假流程',
  businessKey: 'B-1',
  startUserId: 'alice',
  startTime: '2026-10-30T03:00:00Z',
  endTime: '2026-10-30T04:00:00Z',
  durationInMillis: 3600000,
  deleteReason: null,
  state: 'completed',
  tenantId: 't1',
  startActivityId: 'start',
  endActivityId: 'end',
  superProcessInstanceId: 'pi-0',
}

/** 变量样本 */
const VARIABLES = [
  { id: 'v1', name: 'amount', type: 'long', value: 100, taskId: '', processInstanceId: 'pi-1', createTime: '2026-10-30T03:00:00Z' },
  { id: 'v2', name: 'file', type: 'binary', value: null, taskId: 't1', processInstanceId: 'pi-1', createTime: '2026-10-30T03:00:00Z' },
]

// 构造详情页常用路由。
// @return 模拟路由
function detailRoutes() {
  return [
    { path: '/service/history/historic-process-instances/pi-1/variables', body: pageOf(VARIABLES) },
    { path: '/service/history/historic-process-instances/pi-1/comments', body: [{ id: 'c1', author: 'bob', message: '审批通过', time: '2026-10-30T03:30:00Z' }] },
    { path: '/service/history/historic-process-instances/pi-1/identitylinks', body: [{ type: 'candidate', userId: 'bob', taskId: 't1' }] },
    { path: '/service/history/historic-process-instances/pi-1', body: DETAIL },
  ]
}

afterEach(() => {
  vi.unstubAllGlobals()
  authNotice.value = 'none'
  document.body.innerHTML = ''
})

describe('历史流程详情', () => {
  it('加载详情与变量并渲染字段', async () => {
    const calls = mockFetch(detailRoutes())
    const { wrapper } = await mountAt(HistoricProcessDetailPage, '/history/processes/pi-1')
    expect(wrapper.text()).toContain('请假流程')
    expect(wrapper.text()).toContain('B-1')
    expect(wrapper.text()).toContain('alice')
    expect(wrapper.text()).toContain('已完成')
    expect(wrapper.text()).toContain('1小时')
    expect(wrapper.text()).toContain('amount')
    expect(wrapper.text()).toContain('流程全局')
    expect(wrapper.text()).toContain('任务局部')
    expect(wrapper.text()).toContain('二进制内容，不可直接阅读')
    expect(calls[1].url).toBe('/service/history/historic-process-instances/pi-1/variables?start=0&size=1000')
    expect(calls[1].method).toBe('GET')
    wrapper.unmount()
  })

  it('切换页签按需加载评论与身份链接', async () => {
    const calls = mockFetch(detailRoutes())
    const { wrapper } = await mountAt(HistoricProcessDetailPage, '/history/processes/pi-1')
    await clickButton(wrapper, '评论')
    expect(wrapper.text()).toContain('审批通过')
    expect(calls[2].url).toContain('/comments')
    await clickButton(wrapper, '身份链接')
    expect(wrapper.text()).toContain('candidate')
    expect(wrapper.text()).toContain('bob')
    expect(calls[3].url).toContain('/identitylinks')
    // 已加载过的变量页签不会重复请求
    await clickButton(wrapper, '变量')
    expect(calls).toHaveLength(4)
    wrapper.unmount()
  })

  it('详情接口失败展示 ErrorBlock 并可重试', async () => {
    const calls = mockFetch([
      { path: '/service/history/historic-process-instances/pi-1/variables', body: pageOf([]) },
      { path: '/service/history/historic-process-instances/pi-1', status: 500, body: { message: '历史服务故障' } },
    ])
    const { wrapper } = await mountAt(HistoricProcessDetailPage, '/history/processes/pi-1')
    expect(wrapper.text()).toContain('历史服务故障')
    expect(wrapper.text()).not.toContain('基本信息')
    await wrapper.find('button[title="重试"]').trigger('click')
    await settle()
    expect(calls.filter((call) => call.url.endsWith('/pi-1') && call.method === 'GET')).toHaveLength(2)
    wrapper.unmount()
  })

  it('变量加载失败可重试', async () => {
    const calls = mockFetch([
      { path: '/service/history/historic-process-instances/pi-1/variables', status: 500, body: { message: '变量服务故障' } },
      { path: '/service/history/historic-process-instances/pi-1', body: DETAIL },
    ])
    const { wrapper } = await mountAt(HistoricProcessDetailPage, '/history/processes/pi-1')
    expect(wrapper.text()).toContain('变量服务故障')
    await clickButton(wrapper, '重试当前页签')
    expect(calls.filter((call) => call.url.includes('/variables'))).toHaveLength(2)
    wrapper.unmount()
  })

  it('评论与身份链接加载失败可重试当前页签', async () => {
    const calls = mockFetch([
      { path: '/service/history/historic-process-instances/pi-1/variables', body: pageOf([]) },
      { path: '/service/history/historic-process-instances/pi-1/comments', status: 500, body: { message: '评论服务故障' } },
      { path: '/service/history/historic-process-instances/pi-1/identitylinks', status: 500, body: { message: '链接服务故障' } },
      { path: '/service/history/historic-process-instances/pi-1', body: DETAIL },
    ])
    const { wrapper } = await mountAt(HistoricProcessDetailPage, '/history/processes/pi-1')
    await clickButton(wrapper, '评论')
    expect(wrapper.text()).toContain('评论服务故障')
    await clickButton(wrapper, '重试当前页签')
    expect(calls.filter((call) => call.url.includes('/comments'))).toHaveLength(2)
    await clickButton(wrapper, '身份链接')
    expect(wrapper.text()).toContain('链接服务故障')
    await clickButton(wrapper, '重试当前页签')
    expect(calls.filter((call) => call.url.includes('/identitylinks'))).toHaveLength(2)
    wrapper.unmount()
  })

  it('删除成功后返回历史列表', async () => {
    const calls = mockFetch([
      ...detailRoutes().filter((route) => route.body !== DETAIL),
      { path: '/service/history/historic-process-instances/pi-1', method: 'DELETE', status: 204 },
      { path: '/service/history/historic-process-instances/pi-1', body: DETAIL },
    ])
    const { wrapper, router } = await mountAt(HistoricProcessDetailPage, '/history/processes/pi-1')
    wrapper.findComponent(ConfirmButton).vm.$emit('confirm')
    await settle()
    expect(calls.some((call) => call.method === 'DELETE' && call.url.endsWith('/pi-1'))).toBe(true)
    expect(router.currentRoute.value.name).toBe('historic-processes')
    wrapper.unmount()
  })

  it('删除失败保留页面并展示服务端原因', async () => {
    const calls = mockFetch([
      ...detailRoutes().filter((route) => route.body !== DETAIL),
      { path: '/service/history/historic-process-instances/pi-1', method: 'DELETE', status: 500, body: { message: 'Process instance is still running' } },
      { path: '/service/history/historic-process-instances/pi-1', body: DETAIL },
    ])
    const { wrapper, router } = await mountAt(HistoricProcessDetailPage, '/history/processes/pi-1')
    wrapper.findComponent(ConfirmButton).vm.$emit('confirm')
    await settle()
    expect(wrapper.text()).toContain('Process instance is still running')
    expect(router.currentRoute.value.name).toBe('historic-process-detail')
    await wrapper.find('button[title="重试"]').trigger('click')
    await settle()
    expect(calls.filter((call) => call.method === 'DELETE')).toHaveLength(2)
    wrapper.unmount()
  })

  it('运行中的实例提供运行态入口', async () => {
    mockFetch([
      ...detailRoutes().filter((route) => route.body !== DETAIL),
      { path: '/service/history/historic-process-instances/pi-1', body: { ...DETAIL, endTime: null, durationInMillis: null, state: 'running' } },
    ])
    const { wrapper, router } = await mountAt(HistoricProcessDetailPage, '/history/processes/pi-1')
    expect(wrapper.text()).toContain('运行中')
    await clickButton(wrapper, '运行实例')
    expect(router.currentRoute.value.path).toBe('/runtime/instances/pi-1')
    wrapper.unmount()
  })

  it('查看历史变量原始数据', async () => {
    const calls = mockFetch([
      { path: '/service/history/historic-process-instances/pi-1/variables/amount/data', text: 'remaining-100', contentType: 'application/octet-stream' },
      ...detailRoutes(),
    ])
    const { wrapper } = await mountAt(HistoricProcessDetailPage, '/history/processes/pi-1')
    await clickButton(wrapper, '原始')
    await settle()
    expect(calls.some((call) => call.url.endsWith('/variables/amount/data'))).toBe(true)
    expect(document.body.textContent).toContain('remaining-100')
    expect(document.body.textContent).toContain('变量 amount 原始数据')
    wrapper.unmount()
  })
})
