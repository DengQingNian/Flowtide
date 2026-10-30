import { afterEach, describe, expect, it, vi } from 'vitest'
import { NInput, NSelect } from 'naive-ui'
import HistoricTasksPage from '@/pages/history/HistoricTasksPage.vue'
import HistoricTaskDetailPage from '@/pages/history/HistoricTaskDetailPage.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import DataTable from '@/components/DataTable.vue'
import { authNotice } from '@/stores/ui'
import { mockFetch, pageOf, settle } from '../helpers'
import { clickButton, mountAt } from './support'

/** 历史任务样本 */
const TASK = {
  id: 't1',
  name: '主管审批',
  assignee: 'alice',
  owner: 'bob',
  processInstanceId: 'pi-1',
  processDefinitionId: 'leave:1:1',
  taskDefinitionKey: 'approve',
  startTime: '2026-10-30T03:00:00Z',
  endTime: '2026-10-30T03:30:00Z',
  durationInMillis: 1800000,
  dueDate: '2026-10-31T03:00:00Z',
  formKey: 'leaveForm',
  deleteReason: null,
  tenantId: 't1',
}

afterEach(() => {
  vi.unstubAllGlobals()
  authNotice.value = 'none'
  document.body.innerHTML = ''
})

describe('历史任务列表', () => {
  it('加载并渲染任务', async () => {
    const calls = mockFetch([{ path: '/service/query/historic-task-instances', method: 'POST', body: pageOf([TASK]) }])
    const { wrapper } = await mountAt(HistoricTasksPage, '/history/tasks')
    expect(wrapper.text()).toContain('主管审批')
    expect(wrapper.text()).toContain('alice')
    expect(wrapper.text()).toContain('approve')
    expect(wrapper.text()).toContain('已完成')
    expect(wrapper.text()).toContain('30分')
    expect(calls[0].body).toMatchObject({ start: 0, size: 50, sort: 'startTime', order: 'desc' })
    wrapper.unmount()
  })

  it('从路由恢复筛选与分页', async () => {
    const calls = mockFetch([{ path: '/service/query/historic-task-instances', method: 'POST', body: pageOf([TASK]) }])
    await mountAt(
      HistoricTasksPage,
      '/history/tasks?assignee=alice&taskName=%E5%AE%A1%E6%89%B9&processInstanceId=pi-1&definitionKey=leave&finished=false&page=2&size=20',
    )
    expect(calls[0].body).toEqual({
      assignee: 'alice',
      name: '审批',
      processInstanceId: 'pi-1',
      processDefinitionKey: 'leave',
      finished: false,
      start: 20,
      size: 20,
      sort: 'startTime',
      order: 'desc',
    })
  })

  it('空数据与接口错误', async () => {
    mockFetch([{ path: '/service/query/historic-task-instances', method: 'POST', body: pageOf([]) }])
    const empty = await mountAt(HistoricTasksPage, '/history/tasks')
    expect(empty.wrapper.text()).toContain('没有符合条件的历史任务')
    empty.wrapper.unmount()

    mockFetch([
      { path: '/service/query/historic-task-instances', method: 'POST', status: 403, body: { code: 'REST-0403', message: '无权限' } },
    ])
    const failed = await mountAt(HistoricTasksPage, '/history/tasks')
    expect(failed.wrapper.text()).toContain('无权限')
    expect(authNotice.value).toBe('forbidden')
    await failed.wrapper.find('button[title="重试"]').trigger('click')
    await settle()
    failed.wrapper.unmount()
  })

  it('查询、状态切换与分页会重新请求', async () => {
    const calls = mockFetch([{ path: '/service/query/historic-task-instances', method: 'POST', body: pageOf([TASK]) }])
    const { wrapper } = await mountAt(HistoricTasksPage, '/history/tasks')
    const inputs = wrapper.findAllComponents(NInput)
    inputs[0].vm.$emit('update:value', 'alice')
    await clickButton(wrapper, '查询')
    expect(calls[1].body).toMatchObject({ assignee: 'alice', start: 0 })
    wrapper.findComponent(NSelect).vm.$emit('update:value', 'false')
    await settle()
    expect(calls[2].body).toMatchObject({ finished: false })
    wrapper.findComponent(DataTable).vm.$emit('update:page', 2)
    await settle()
    expect(calls[3].body).toMatchObject({ start: 50 })
    await clickButton(wrapper, '重置')
    expect(calls[4].body).not.toHaveProperty('assignee')
    wrapper.unmount()
  })
})

describe('历史任务详情', () => {
  // 构造任务详情路由。
  // @return 模拟路由
  function taskRoutes() {
    return [
      { path: '/service/history/historic-task-instances/t1/form', body: { formKey: 'f1', formProperties: [{ id: 'reason', name: '原因', type: 'string', required: true, writable: true, value: 'ok' }] } },
      { path: '/service/history/historic-task-instances/t1/identitylinks', body: [{ type: 'candidate', userId: 'carol', groupId: null }] },
      { path: '/service/query/historic-task-instances', method: 'POST', body: pageOf([{ ...TASK, variables: [{ name: 'amount', type: 'long', value: 10, scope: 'global' }] }]) },
      { path: '/service/history/historic-task-instances/t1', body: TASK },
    ]
  }

  it('加载任务详情与变量', async () => {
    const calls = mockFetch(taskRoutes())
    const { wrapper } = await mountAt(HistoricTaskDetailPage, '/history/tasks/t1')
    expect(wrapper.text()).toContain('主管审批')
    expect(wrapper.text()).toContain('alice')
    expect(wrapper.text()).toContain('bob')
    expect(wrapper.text()).toContain('已完成')
    expect(wrapper.text()).toContain('30分')
    expect(wrapper.text()).toContain('amount')
    expect(calls.some((call) => call.method === 'POST' && call.url === '/service/query/historic-task-instances')).toBe(true)
    wrapper.unmount()
  })

  it('表单页签展示字段或能力受限提示', async () => {
    mockFetch(taskRoutes())
    const success = await mountAt(HistoricTaskDetailPage, '/history/tasks/t1')
    await clickButton(success.wrapper, '表单')
    expect(success.wrapper.text()).toContain('字段 ID')
    expect(success.wrapper.text()).toContain('reason')
    success.wrapper.unmount()

    mockFetch([
      { path: '/service/history/historic-task-instances/t1/form', status: 400, body: { message: 'Form engine is not initialized' } },
      ...taskRoutes().filter((route) => !route.path.endsWith('/form')),
    ])
    const limited = await mountAt(HistoricTaskDetailPage, '/history/tasks/t1')
    await clickButton(limited.wrapper, '表单')
    expect(limited.wrapper.text()).toContain('Form engine is not initialized')
    expect(limited.wrapper.text()).toContain('引擎未内置表单服务')
    await clickButton(limited.wrapper, '重试当前页签')
    limited.wrapper.unmount()
  })

  it('评论与身份链接错误、变量错误可重试', async () => {
    mockFetch([
      { path: '/service/history/historic-task-instances/t1/form', status: 401, body: { message: '未认证' } },
      { path: '/service/history/historic-task-instances/t1/identitylinks', status: 500, body: { message: '链接失败' } },
      { path: '/service/query/historic-task-instances', method: 'POST', status: 500, body: { message: '变量失败' } },
      { path: '/service/history/historic-task-instances/t1', body: TASK },
    ])
    const { wrapper } = await mountAt(HistoricTaskDetailPage, '/history/tasks/t1')
    expect(wrapper.text()).toContain('变量失败')
    await clickButton(wrapper, '重试当前页签')
    await clickButton(wrapper, '表单')
    expect(wrapper.text()).toContain('未认证')
    expect(authNotice.value).toBe('unauthorized')
    await clickButton(wrapper, '身份链接')
    expect(wrapper.text()).toContain('链接失败')
    await clickButton(wrapper, '重试当前页签')
    wrapper.unmount()
  })

  it('删除成功返回列表，失败保留页面', async () => {
    const calls = mockFetch([
      ...taskRoutes().filter((route) => route.body !== TASK),
      { path: '/service/history/historic-task-instances/t1', method: 'DELETE', status: 204 },
      { path: '/service/history/historic-task-instances/t1', body: TASK },
    ])
    const ok = await mountAt(HistoricTaskDetailPage, '/history/tasks/t1')
    ok.wrapper.findComponent(ConfirmButton).vm.$emit('confirm')
    await settle()
    expect(calls.some((call) => call.method === 'DELETE')).toBe(true)
    expect(ok.router.currentRoute.value.name).toBe('historic-tasks')
    ok.wrapper.unmount()

    mockFetch([
      ...taskRoutes().filter((route) => route.body !== TASK),
      { path: '/service/history/historic-task-instances/t1', method: 'DELETE', status: 404, body: { message: '任务不存在' } },
      { path: '/service/history/historic-task-instances/t1', body: TASK },
    ])
    const failed = await mountAt(HistoricTaskDetailPage, '/history/tasks/t1')
    failed.wrapper.findComponent(ConfirmButton).vm.$emit('confirm')
    await settle()
    expect(failed.wrapper.text()).toContain('任务不存在')
    expect(failed.router.currentRoute.value.name).toBe('historic-task-detail')
    failed.wrapper.unmount()
  })

  it('查看任务变量原始数据', async () => {
    mockFetch([
      { path: '/service/history/historic-task-instances/t1/variables/amount/data', text: 'ten', contentType: 'application/octet-stream' },
      ...taskRoutes(),
    ])
    const { wrapper } = await mountAt(HistoricTaskDetailPage, '/history/tasks/t1')
    await clickButton(wrapper, '原始')
    await settle()
    expect(document.body.textContent).toContain('ten')
    wrapper.unmount()
  })
})
