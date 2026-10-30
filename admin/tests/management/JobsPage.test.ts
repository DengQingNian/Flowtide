// 作业列表页测试：分类计数、桶切换、立即执行与错误/空态。
import { afterEach, describe, expect, it, vi } from 'vitest'
import { mockFetch, pageOf, settle } from '../helpers'
import JobsPage from '@/pages/management/JobsPage.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import { buttonByText, mountPage } from './pageUtils'

afterEach(() => {
  vi.unstubAllGlobals()
})

// 构造作业。
// @param id 作业 ID
// @return 作业对象
function job(id: string) {
  return {
    id,
    type: 'async-continuation',
    processInstanceId: 'pi-1234567890',
    executionId: 'ex-1234567890',
    activityId: 'task-1',
    retries: 2,
    exception: 'boom',
    dueDate: '2026-10-30T12:00:00Z',
    executable: true,
  }
}

describe('JobsPage', () => {
  it('加载四类计数并渲染作业', async () => {
    mockFetch([
      { method: 'GET', path: /^\/service\/management\/jobs\?start=0&size=1$/, body: pageOf([], 4) },
      { method: 'GET', path: /^\/service\/management\/timer-jobs\?start=0&size=1$/, body: pageOf([], 2) },
      { method: 'GET', path: /^\/service\/management\/suspended-jobs\?start=0&size=1$/, body: pageOf([], 1) },
      { method: 'GET', path: /^\/service\/management\/deadletter-jobs\?start=0&size=1$/, body: pageOf([], 3) },
      { method: 'GET', path: /^\/service\/management\/jobs\?start=0&size=50$/, body: pageOf([job('job-1')]) },
    ])
    const { wrapper } = await mountPage(JobsPage, '/management/jobs/executable')
    await settle()
    expect(wrapper.text()).toContain('async-continuation')
    expect(wrapper.text()).toContain('boom')
    expect(wrapper.text()).toContain('4')
    expect(wrapper.findAll('a').some((link) => link.attributes('href') === '/management/jobs/executable/job-1')).toBe(true)
    expect(wrapper.findAll('a').some((link) => link.attributes('href') === '/runtime/instances/pi-1234567890')).toBe(true)
  })

  it('切换作业桶后请求对应端点', async () => {
    const calls = mockFetch([
      { method: 'GET', path: /^\/service\/management\/jobs\?start=0&size=1$/, body: pageOf([], 0) },
      { method: 'GET', path: /^\/service\/management\/timer-jobs\?start=0&size=1$/, body: pageOf([], 0) },
      { method: 'GET', path: /^\/service\/management\/suspended-jobs\?start=0&size=1$/, body: pageOf([], 0) },
      { method: 'GET', path: /^\/service\/management\/deadletter-jobs\?start=0&size=1$/, body: pageOf([], 0) },
      { method: 'GET', path: /^\/service\/management\/jobs\?start=0&size=50$/, body: pageOf([job('job-1')]) },
      { method: 'GET', path: /^\/service\/management\/deadletter-jobs\?start=0&size=50$/, body: pageOf([job('dead-1')]) },
    ])
    const { wrapper, router } = await mountPage(JobsPage, '/management/jobs/executable')
    await settle()
    await buttonByText(wrapper, '死信')!.trigger('click')
    await settle()
    expect(router.currentRoute.value.params.bucket).toBe('deadletter')
    expect(calls.some((call) => call.url.includes('/deadletter-jobs?start=0&size=50'))).toBe(true)
    expect(wrapper.text()).toContain('dead-1')
  })

  it('立即执行到期作业并刷新', async () => {
    const calls = mockFetch([
      { method: 'GET', path: /^\/service\/management\/jobs\?start=0&size=1$/, body: pageOf([], 0) },
      { method: 'GET', path: /^\/service\/management\/timer-jobs\?start=0&size=1$/, body: pageOf([], 0) },
      { method: 'GET', path: /^\/service\/management\/suspended-jobs\?start=0&size=1$/, body: pageOf([], 0) },
      { method: 'GET', path: /^\/service\/management\/deadletter-jobs\?start=0&size=1$/, body: pageOf([], 0) },
      { method: 'GET', path: /^\/service\/management\/jobs\?start=0&size=50$/, body: pageOf([]) },
      { method: 'POST', path: '/service/management/jobs/run', body: { executed: 2 } },
    ])
    const { wrapper } = await mountPage(JobsPage, '/management/jobs/executable')
    await settle()
    wrapper.findComponent(ConfirmButton).vm.$emit('confirm')
    await settle()
    expect(wrapper.text()).toContain('已执行 2 个到期作业')
    expect(calls.some((call) => call.method === 'POST' && call.url === '/service/management/jobs/run')).toBe(true)
  })

  it('接口错误展示 ErrorBlock', async () => {
    mockFetch([
      { method: 'GET', path: '/service/management/jobs', status: 500, body: { code: 'ENGINE-5000', message: '引擎内部错误' } },
      { method: 'GET', path: '/service/management/timer-jobs', body: pageOf([], 0) },
      { method: 'GET', path: '/service/management/suspended-jobs', body: pageOf([], 0) },
      { method: 'GET', path: '/service/management/deadletter-jobs', body: pageOf([], 0) },
    ])
    const { wrapper } = await mountPage(JobsPage, '/management/jobs/executable')
    await settle()
    expect(wrapper.text()).toContain('引擎内部错误')
    expect(wrapper.text()).toContain('错误码：ENGINE-5000')
  })

  it('空数据展示空态说明', async () => {
    mockFetch([
      { method: 'GET', path: '/service/management/jobs', body: pageOf([]) },
      { method: 'GET', path: '/service/management/timer-jobs', body: pageOf([], 0) },
      { method: 'GET', path: '/service/management/suspended-jobs', body: pageOf([], 0) },
      { method: 'GET', path: '/service/management/deadletter-jobs', body: pageOf([], 0) },
    ])
    const { wrapper } = await mountPage(JobsPage, '/management/jobs/executable')
    await settle()
    expect(wrapper.text()).toContain('当前没有可执行作业')
  })
})
