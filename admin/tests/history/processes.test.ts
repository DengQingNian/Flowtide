import { afterEach, describe, expect, it, vi } from 'vitest'
import { NInput, NSelect } from 'naive-ui'
import HistoricProcessesPage from '@/pages/history/HistoricProcessesPage.vue'
import DataTable from '@/components/DataTable.vue'
import { authNotice } from '@/stores/ui'
import { mockFetch, pageOf, settle } from '../helpers'
import { clickButton, mountAt } from './support'

/** 历史流程样本 */
const PROCESS = {
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
}

afterEach(() => {
  vi.unstubAllGlobals()
  authNotice.value = 'none'
  document.body.innerHTML = ''
})

describe('历史流程列表', () => {
  it('加载并渲染历史流程实例', async () => {
    const calls = mockFetch([{ path: '/service/query/historic-process-instances', method: 'POST', body: pageOf([PROCESS]) }])
    const { wrapper } = await mountAt(HistoricProcessesPage, '/history/processes')
    expect(wrapper.text()).toContain('请假流程')
    expect(wrapper.text()).toContain('B-1')
    expect(wrapper.text()).toContain('alice')
    expect(wrapper.text()).toContain('已完成')
    expect(wrapper.text()).toContain('1小时')
    expect(calls[0].body).toMatchObject({ start: 0, size: 50, sort: 'startTime', order: 'desc' })
    wrapper.unmount()
  })

  it('从路由恢复筛选、分页与时间条件', async () => {
    const calls = mockFetch([{ path: '/service/query/historic-process-instances', method: 'POST', body: pageOf([PROCESS]) }])
    await mountAt(
      HistoricProcessesPage,
      '/history/processes?definitionKey=leave&businessKey=B-1&tenantId=t1&finished=true&startedAfter=1700000000000&endedAfter=1700000001000&page=2&size=100',
    )
    expect(calls[0].body).toEqual({
      processDefinitionKey: 'leave',
      businessKey: 'B-1',
      tenantId: 't1',
      finished: true,
      startedAfter: new Date(1700000000000).toISOString(),
      endedAfter: new Date(1700000001000).toISOString(),
      start: 100,
      size: 100,
      sort: 'startTime',
      order: 'desc',
    })
  })

  it('空数据展示空态', async () => {
    mockFetch([{ path: '/service/query/historic-process-instances', method: 'POST', body: pageOf([]) }])
    const { wrapper } = await mountAt(HistoricProcessesPage, '/history/processes')
    expect(wrapper.text()).toContain('暂无数据')
    expect(wrapper.text()).toContain('没有符合条件的历史流程实例')
    wrapper.unmount()
  })

  it('接口错误展示 ErrorBlock 并标记认证问题', async () => {
    mockFetch([
      {
        path: '/service/query/historic-process-instances',
        method: 'POST',
        status: 401,
        body: { code: 'REST-0401', message: '未认证' },
      },
    ])
    const { wrapper } = await mountAt(HistoricProcessesPage, '/history/processes')
    expect(wrapper.text()).toContain('未认证')
    expect(wrapper.text()).toContain('错误码：REST-0401')
    expect(authNotice.value).toBe('unauthorized')
    await wrapper.find('button[title="重试"]').trigger('click')
    await settle()
    wrapper.unmount()
  })

  it('查询与重置会写回路由并重新请求', async () => {
    const calls = mockFetch([{ path: '/service/query/historic-process-instances', method: 'POST', body: pageOf([PROCESS]) }])
    const { wrapper, router } = await mountAt(HistoricProcessesPage, '/history/processes')
    const inputs = wrapper.findAllComponents(NInput)
    inputs[0].vm.$emit('update:value', 'leave')
    inputs[1].vm.$emit('update:value', 'B-1')
    inputs[2].vm.$emit('update:value', 't1')
    await clickButton(wrapper, '查询')
    expect(calls[1].body).toMatchObject({ processDefinitionKey: 'leave', businessKey: 'B-1', tenantId: 't1', start: 0 })
    expect(router.currentRoute.value.query.definitionKey).toBe('leave')

    // 状态选择会立即触发查询，因此这里单独验证 finished 条件
    wrapper.findComponent(NSelect).vm.$emit('update:value', 'true')
    await settle()
    expect(calls[2].body).toMatchObject({ finished: true, start: 0 })
    expect(router.currentRoute.value.query.finished).toBe('true')

    await clickButton(wrapper, '重置')
    expect(calls[3].body).not.toHaveProperty('processDefinitionKey')
    expect(router.currentRoute.value.query.definitionKey).toBeUndefined()
    expect(router.currentRoute.value.query.page).toBe('1')
    wrapper.unmount()
  })

  it('分页变化按新条件请求', async () => {
    const calls = mockFetch([{ path: '/service/query/historic-process-instances', method: 'POST', body: pageOf([PROCESS]) }])
    const { wrapper } = await mountAt(HistoricProcessesPage, '/history/processes')
    const table = wrapper.findComponent(DataTable)
    table.vm.$emit('update:page', 3)
    await settle()
    expect(calls[1].body).toMatchObject({ start: 100, size: 50 })
    table.vm.$emit('update:pageSize', 20)
    await settle()
    expect(calls[2].body).toMatchObject({ start: 0, size: 20 })
    wrapper.unmount()
  })

  it('浏览器回退等外部路由变化会重新加载', async () => {
    const calls = mockFetch([{ path: '/service/query/historic-process-instances', method: 'POST', body: pageOf([PROCESS]) }])
    const { router } = await mountAt(HistoricProcessesPage, '/history/processes')
    await router.replace({ query: { definitionKey: 'other' } })
    await settle()
    expect(calls[1].body).toMatchObject({ processDefinitionKey: 'other', start: 0 })
  })
})
