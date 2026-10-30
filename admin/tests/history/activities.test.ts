import { afterEach, describe, expect, it, vi } from 'vitest'
import HistoricActivitiesPage from '@/pages/history/HistoricActivitiesPage.vue'
import { authNotice } from '@/stores/ui'
import { mockFetch, pageOf, settle } from '../helpers'
import { clickButton, mountAt } from './support'

/** 历史活动样本 */
const ACTIVITY = {
  id: 'a1',
  activityId: 'approve',
  activityName: '主管审批',
  activityType: 'userTask',
  processInstanceId: 'pi-1',
  executionId: 'e1',
  taskId: 't1',
  assignee: 'alice',
  startTime: '2026-10-30T03:00:00Z',
  endTime: '2026-10-30T03:30:00Z',
  durationInMillis: 1800000,
  tenantId: 't1',
}

afterEach(() => {
  vi.unstubAllGlobals()
  authNotice.value = 'none'
  document.body.innerHTML = ''
})

describe('历史活动列表', () => {
  it('加载并渲染活动', async () => {
    const calls = mockFetch([{ path: '/service/query/historic-activity-instances', method: 'POST', body: pageOf([ACTIVITY]) }])
    const { wrapper } = await mountAt(HistoricActivitiesPage, '/history/activities')
    expect(wrapper.text()).toContain('approve')
    expect(wrapper.text()).toContain('主管审批')
    expect(wrapper.text()).toContain('userTask')
    expect(wrapper.text()).toContain('alice')
    expect(wrapper.text()).toContain('已完成')
    expect(wrapper.text()).toContain('30分')
    expect(calls[0].body).toMatchObject({ start: 0, size: 50, sort: 'startTime', order: 'desc' })
    wrapper.unmount()
  })

  it('从路由恢复筛选并展示空态', async () => {
    const calls = mockFetch([{ path: '/service/query/historic-activity-instances', method: 'POST', body: pageOf([]) }])
    const { wrapper } = await mountAt(
      HistoricActivitiesPage,
      '/history/activities?processInstanceId=pi-1&definitionKey=leave&activityId=approve&activityType=userTask&finished=true&page=2&size=20',
    )
    expect(wrapper.text()).toContain('没有符合条件的历史活动')
    expect(calls[0].body).toEqual({
      processInstanceId: 'pi-1',
      processDefinitionKey: 'leave',
      activityId: 'approve',
      activityType: 'userTask',
      finished: true,
      start: 20,
      size: 20,
      sort: 'startTime',
      order: 'desc',
    })
  })

  it('接口错误展示 ErrorBlock 并可重试', async () => {
    mockFetch([
      { path: '/service/query/historic-activity-instances', method: 'POST', status: 500, body: { message: '活动查询失败' } },
    ])
    const { wrapper } = await mountAt(HistoricActivitiesPage, '/history/activities')
    expect(wrapper.text()).toContain('活动查询失败')
    await wrapper.find('button[title="重试"]').trigger('click')
    await settle()
    wrapper.unmount()
  })

  it('查看活动详情', async () => {
    const calls = mockFetch([
      { path: '/service/history/historic-activity-instances/a1', body: ACTIVITY },
      { path: '/service/query/historic-activity-instances', method: 'POST', body: pageOf([ACTIVITY]) },
    ])
    const { wrapper } = await mountAt(HistoricActivitiesPage, '/history/activities')
    await clickButton(wrapper, '查看')
    await settle()
    expect(calls.some((call) => call.url.endsWith('/history/historic-activity-instances/a1'))).toBe(true)
    expect(document.body.textContent).toContain('历史活动详情')
    expect(document.body.textContent).toContain('主管审批')
    expect(document.body.textContent).toContain('pi-1')
    wrapper.unmount()
  })

  it('活动详情失败可重试', async () => {
    const calls = mockFetch([
      { path: '/service/history/historic-activity-instances/a1', status: 404, body: { message: '活动不存在' } },
      { path: '/service/query/historic-activity-instances', method: 'POST', body: pageOf([ACTIVITY]) },
    ])
    const { wrapper } = await mountAt(HistoricActivitiesPage, '/history/activities')
    await clickButton(wrapper, '查看')
    await settle()
    expect(document.body.textContent).toContain('活动不存在')
    const retry = document.body.querySelector('button[title="重试"]') as HTMLButtonElement | null
    expect(retry).toBeTruthy()
    retry?.click()
    await settle()
    expect(calls.filter((call) => call.url.endsWith('/historic-activity-instances/a1'))).toHaveLength(2)
    wrapper.unmount()
  })
})
