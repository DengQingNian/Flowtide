// 作业详情页测试：字段与异常栈、重试/恢复/调度/删除及失败提示。
import { afterEach, describe, expect, it, vi } from 'vitest'
import { mockFetch, settle } from '../helpers'
import JobDetailPage from '@/pages/management/JobDetailPage.vue'
import { buttonByText, confirmByLabel, mountPage } from './pageUtils'

afterEach(() => {
  vi.unstubAllGlobals()
})

// 构造作业。
// @param overrides 覆盖字段
// @return 作业对象
function job(overrides: Record<string, unknown> = {}) {
  return {
    id: 'job-1',
    type: 'async-continuation',
    processInstanceId: 'pi-1',
    executionId: 'ex-1',
    activityId: 'task-1',
    retries: 2,
    exception: 'Error: boom',
    dueDate: '2026-10-30T12:00:00Z',
    executable: true,
    ...overrides,
  }
}

describe('JobDetailPage', () => {
  it('渲染作业字段、异常栈与实例/定义跳转', async () => {
    mockFetch([
      { method: 'GET', path: '/service/management/jobs/job-1/exception-stacktrace', text: 'Error: stack line\n  at flow' },
      { method: 'GET', path: '/service/management/jobs/job-1', body: job() },
      { method: 'GET', path: '/service/runtime/process-instances/pi-1', body: { id: 'pi-1', processDefinitionId: 'def-1', businessKey: 'bk-1' } },
    ])
    const { wrapper } = await mountPage(JobDetailPage, '/management/jobs/executable/job-1')
    await settle()
    expect(wrapper.text()).toContain('async-continuation')
    expect(wrapper.text()).toContain('Error: stack line')
    expect(wrapper.text()).toContain('bk-1')
    expect(wrapper.findAll('a').some((link) => link.attributes('href') === '/runtime/instances/pi-1')).toBe(true)
    expect(wrapper.findAll('a').some((link) => link.attributes('href') === '/repository/definitions/def-1')).toBe(true)
  })

  it('没有异常栈时展示提示', async () => {
    mockFetch([
      { method: 'GET', path: '/service/management/jobs/job-1/exception-stacktrace', status: 404, body: { code: 'ENGINE-1505', message: '作业没有异常栈' } },
      { method: 'GET', path: '/service/management/jobs/job-1', body: job({ exception: '' }) },
      { method: 'GET', path: '/service/runtime/process-instances/pi-1', status: 404, body: { message: 'Not Found' } },
    ])
    const { wrapper } = await mountPage(JobDetailPage, '/management/jobs/executable/job-1')
    await settle()
    expect(wrapper.text()).toContain('没有记录异常栈')
    expect(wrapper.text()).toContain('关联实例已结束或不存在')
  })

  it('设置可执行作业重试次数', async () => {
    const calls = mockFetch([
      { method: 'GET', path: '/service/management/jobs/job-1/exception-stacktrace', status: 404, body: {} },
      { method: 'GET', path: '/service/management/jobs/job-1', body: job() },
      { method: 'GET', path: '/service/runtime/process-instances/pi-1', body: { id: 'pi-1' } },
      { method: 'POST', path: '/service/management/jobs/job-1', status: 204 },
    ])
    const { wrapper } = await mountPage(JobDetailPage, '/management/jobs/executable/job-1')
    await settle()
    await wrapper.find('input[type="number"]').setValue('5')
    await buttonByText(wrapper, '设置重试次数')!.trigger('click')
    await settle()
    const action = calls.find((call) => call.method === 'POST')
    expect(action?.body).toEqual({ action: 'setRetries', retries: 5 })
    expect(wrapper.text()).toContain('重试次数已更新')
  })

  it('非法重试次数被前端拒绝', async () => {
    const calls = mockFetch([
      { method: 'GET', path: '/service/management/jobs/job-1/exception-stacktrace', status: 404, body: {} },
      { method: 'GET', path: '/service/management/jobs/job-1', body: job() },
      { method: 'GET', path: '/service/runtime/process-instances/pi-1', body: { id: 'pi-1' } },
    ])
    const { wrapper } = await mountPage(JobDetailPage, '/management/jobs/executable/job-1')
    await settle()
    await wrapper.find('input[type="number"]').setValue('-1')
    await buttonByText(wrapper, '设置重试次数')!.trigger('click')
    await settle()
    expect(wrapper.text()).toContain('重试次数必须是不小于 0 的整数')
    expect(calls.some((call) => call.method === 'POST')).toBe(false)
  })

  it('死信恢复到可执行并跳转列表', async () => {
    const calls = mockFetch([
      { method: 'GET', path: '/service/management/deadletter-jobs/job-1/exception-stacktrace', status: 404, body: {} },
      { method: 'GET', path: '/service/management/deadletter-jobs/job-1', body: job({ type: 'async-continuation', executable: false }) },
      { method: 'GET', path: '/service/runtime/process-instances/pi-1', body: { id: 'pi-1' } },
      { method: 'POST', path: '/service/management/deadletter-jobs/job-1', status: 204 },
      { method: 'GET', path: '/service/management/jobs/job-1', body: job() },
    ])
    const { wrapper, router } = await mountPage(JobDetailPage, '/management/jobs/deadletter/job-1')
    await settle()
    confirmByLabel(wrapper, '恢复到可执行')!.vm.$emit('confirm')
    await settle()
    const action = calls.find((call) => call.method === 'POST')
    expect(action?.body).toEqual({ action: 'move', retries: 2 })
    expect(router.currentRoute.value.path).toBe('/management/jobs/executable')
  })

  it('定时器转为可执行并跳转', async () => {
    const calls = mockFetch([
      { method: 'GET', path: '/service/management/timer-jobs/job-1/exception-stacktrace', status: 404, body: {} },
      { method: 'GET', path: '/service/management/timer-jobs/job-1', body: job({ type: 'timer-transition', executable: false }) },
      { method: 'GET', path: '/service/runtime/process-instances/pi-1', body: { id: 'pi-1' } },
      { method: 'POST', path: '/service/management/timer-jobs/job-1', status: 204 },
      { method: 'GET', path: '/service/management/jobs/job-1', body: job() },
    ])
    const { wrapper, router } = await mountPage(JobDetailPage, '/management/jobs/timer/job-1')
    await settle()
    confirmByLabel(wrapper, '转为可执行')!.vm.$emit('confirm')
    await settle()
    expect(calls.find((call) => call.method === 'POST')?.body).toEqual({ action: 'move' })
    expect(router.currentRoute.value.path).toBe('/management/jobs/executable')
  })

  it('定时器重新调度发送 RFC3339 时间', async () => {
    const calls = mockFetch([
      { method: 'GET', path: '/service/management/timer-jobs/job-1/exception-stacktrace', status: 404, body: {} },
      { method: 'GET', path: '/service/management/timer-jobs/job-1', body: job({ type: 'timer-transition', executable: false }) },
      { method: 'GET', path: '/service/runtime/process-instances/pi-1', body: { id: 'pi-1' } },
      { method: 'POST', path: '/service/management/timer-jobs/job-1', status: 204 },
    ])
    const { wrapper } = await mountPage(JobDetailPage, '/management/jobs/timer/job-1')
    await settle()
    await wrapper.find('input[type="datetime-local"]').setValue('2026-11-01T10:00')
    await buttonByText(wrapper, '重新调度')!.trigger('click')
    await settle()
    const body = calls.find((call) => call.method === 'POST')?.body as { action: string; dueDate: string }
    expect(body.action).toBe('reschedule')
    expect(body.dueDate).toBe(new Date('2026-11-01T10:00').toISOString())
    expect(wrapper.text()).toContain('到期时间已更新')
  })

  it('缺少到期时间时不发请求', async () => {
    const calls = mockFetch([
      { method: 'GET', path: '/service/management/timer-jobs/job-1/exception-stacktrace', status: 404, body: {} },
      { method: 'GET', path: '/service/management/timer-jobs/job-1', body: job({ type: 'timer-transition' }) },
      { method: 'GET', path: '/service/runtime/process-instances/pi-1', body: { id: 'pi-1' } },
    ])
    const { wrapper } = await mountPage(JobDetailPage, '/management/jobs/timer/job-1')
    await settle()
    await buttonByText(wrapper, '重新调度')!.trigger('click')
    await settle()
    expect(wrapper.text()).toContain('请选择新的到期时间')
    expect(calls.some((call) => call.method === 'POST')).toBe(false)
  })

  it('动作失败展示服务端原文', async () => {
    mockFetch([
      { method: 'GET', path: '/service/management/jobs/job-1/exception-stacktrace', status: 404, body: {} },
      { method: 'GET', path: '/service/management/jobs/job-1', body: job() },
      { method: 'GET', path: '/service/runtime/process-instances/pi-1', body: { id: 'pi-1' } },
      { method: 'POST', path: '/service/management/jobs/job-1', status: 403, body: { code: 'REST-0403', message: '无权限' } },
    ])
    const { wrapper } = await mountPage(JobDetailPage, '/management/jobs/executable/job-1')
    await settle()
    await buttonByText(wrapper, '设置重试次数')!.trigger('click')
    await settle()
    expect(wrapper.text()).toContain('无权限')
  })

  it('删除作业后回到桶列表', async () => {
    const calls = mockFetch([
      { method: 'GET', path: '/service/management/suspended-jobs/job-1/exception-stacktrace', status: 404, body: {} },
      { method: 'GET', path: '/service/management/suspended-jobs/job-1', body: job({ type: 'async-continuation' }) },
      { method: 'GET', path: '/service/runtime/process-instances/pi-1', body: { id: 'pi-1' } },
      { method: 'DELETE', path: '/service/management/suspended-jobs/job-1', status: 204 },
    ])
    const { wrapper, router } = await mountPage(JobDetailPage, '/management/jobs/suspended/job-1')
    await settle()
    expect(wrapper.text()).toContain('服务端未提供恢复动作')
    confirmByLabel(wrapper, '删除作业')!.vm.$emit('confirm')
    await settle()
    expect(calls.some((call) => call.method === 'DELETE')).toBe(true)
    expect(router.currentRoute.value.path).toBe('/management/jobs/suspended')
  })

  it('详情接口错误展示 ErrorBlock', async () => {
    mockFetch([
      { method: 'GET', path: '/service/management/jobs/job-1/exception-stacktrace', status: 404, body: {} },
      { method: 'GET', path: '/service/management/jobs/job-1', status: 500, body: { code: 'ENGINE-1500', message: '作业服务不可用' } },
    ])
    const { wrapper } = await mountPage(JobDetailPage, '/management/jobs/executable/job-1')
    await settle()
    expect(wrapper.text()).toContain('作业服务不可用')
    expect(wrapper.text()).toContain('错误码：ENGINE-1500')
  })
})
