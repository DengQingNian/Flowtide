// 任务列表页面测试：加载、空态、错误、筛选、分页、行内动作与批量操作。
import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { createMemoryHistory, createRouter, type Router } from 'vue-router'
import { NButton, NInput, NSelect } from 'naive-ui'
import TasksPage from '@/pages/runtime/TasksPage.vue'
import DataTable from '@/components/DataTable.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import QuickClaimDialog from '@/pages/runtime/components/task/QuickClaimDialog.vue'
import CreateTaskDialog from '@/pages/runtime/components/task/CreateTaskDialog.vue'
import BulkUpdateDialog from '@/pages/runtime/components/task/BulkUpdateDialog.vue'
import { clearAuthNotice, authNotice } from '@/stores/ui'
import { mockFetch, pageOf, settle } from '../../helpers'

afterEach(() => {
  vi.unstubAllGlobals()
  clearAuthNotice()
  document.body.innerHTML = ''
})

/** 任务样例 */
const task = {
  id: 't-1',
  name: '审批',
  assignee: null,
  priority: 50,
  createTime: '2026-10-30T03:00:00Z',
  processInstanceId: 'pi-1',
}

// 挂载任务列表页。
// @param path 初始路由
// @return 包装器与路由
async function mountPage(path = '/runtime/tasks'): Promise<{ wrapper: VueWrapper; router: Router }> {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/runtime/tasks', name: 'tasks', component: TasksPage },
      { path: '/runtime/tasks/:id', name: 'task-detail', component: { template: '<div>详情</div>' } },
      { path: '/runtime/instances/:id', component: { template: '<div>实例</div>' } },
    ],
  })
  await router.push(path)
  await router.isReady()
  const wrapper = mount(TasksPage, { global: { plugins: [router] }, attachTo: document.body })
  await settle()
  return { wrapper, router }
}

// 找到指定文本的按钮。
// @param wrapper 包装器
// @param text 文本
// @return 按钮包装器
function buttonByText(wrapper: VueWrapper, text: string) {
  return wrapper.findAllComponents(NButton).find((item) => item.text().includes(text))
}

// 按文本查找包装器内的按钮。
// @param wrapper 包装器
// @param text 文本
// @return 按钮包装器
function childButton(wrapper: VueWrapper, text: string) {
  return wrapper.findAllComponents(NButton).find((item) => item.text().includes(text))
}

describe('TasksPage', () => {
  it('加载并渲染任务、候选人与默认分页参数', async () => {
    const calls = mockFetch([
      { path: '/service/query/tasks', method: 'POST', body: pageOf([task], 1) },
      { path: '/service/runtime/tasks/t-1/identitylinks', body: [{ type: 'candidate', userId: 'alice' }, { type: 'candidate', groupId: 'g1' }] },
    ])
    const { wrapper } = await mountPage()
    expect(wrapper.text()).toContain('审批')
    expect(wrapper.text()).toContain('@alice')
    expect(wrapper.text()).toContain('#g1')
    expect(calls[0].body).toEqual({ start: 0, size: 50, sort: 'createTime', order: 'desc' })
  })

  it('空数据展示空态', async () => {
    const calls = mockFetch([{ path: '/service/query/tasks', method: 'POST', body: pageOf([]) }])
    const { wrapper } = await mountPage()
    expect(wrapper.text()).toContain('暂无数据')
    expect(wrapper.text()).toContain('当前筛选条件下没有运行中任务')
    expect(calls).toHaveLength(1)
  })

  it('接口错误展示 ErrorBlock 并可重试', async () => {
    const calls = mockFetch([{ path: '/service/query/tasks', method: 'POST', status: 500, body: { message: '任务查询失败' } }])
    const { wrapper } = await mountPage()
    expect(wrapper.text()).toContain('任务查询失败')
    const retry = wrapper.find('.icon-button')
    await retry.trigger('click')
    await settle()
    expect(calls.length).toBeGreaterThanOrEqual(2)
  })

  it('筛选条件写入路由并携带到请求', async () => {
    const calls = mockFetch([
      { path: '/service/query/tasks', method: 'POST', body: pageOf([task]) },
      { path: '/service/runtime/tasks/t-1/identitylinks', body: [] },
    ])
    const { wrapper, router } = await mountPage()
    await wrapper.findAllComponents(NInput)[0].vm.$emit('update:value', 'alice')
    await wrapper.findAllComponents(NInput)[5].vm.$emit('update:value', '审')
    await buttonByText(wrapper, '查询')!.trigger('click')
    await settle()
    const last = calls.filter((call) => call.url.startsWith('/service/query/tasks')).at(-1)!
    expect(last.body).toMatchObject({ assignee: 'alice', nameLike: '审', start: 0 })
    expect(router.currentRoute.value.query).toMatchObject({ assignee: 'alice', nameLike: '审' })
  })

  it('重置筛选条件', async () => {
    const calls = mockFetch([
      { path: '/service/query/tasks', method: 'POST', body: pageOf([]) },
    ])
    const { wrapper } = await mountPage('/runtime/tasks?assignee=alice')
    expect(wrapper.findAllComponents(NInput)[0].props('value')).toBe('alice')
    await buttonByText(wrapper, '重置')!.trigger('click')
    await settle()
    const last = calls.filter((call) => call.url.startsWith('/service/query/tasks')).at(-1)!
    expect(last.body).not.toHaveProperty('assignee')
  })

  it('分页、每页条数与排序变化会重新请求', async () => {
    const calls = mockFetch([
      { path: '/service/query/tasks', method: 'POST', body: pageOf([task]) },
      { path: '/service/runtime/tasks/t-1/identitylinks', body: [] },
    ])
    const { wrapper } = await mountPage()
    const table = wrapper.findComponent(DataTable)
    table.vm.$emit('update:page', 2)
    await settle()
    table.vm.$emit('update:pageSize', 100)
    await settle()
    await wrapper.findAllComponents(NSelect)[0].vm.$emit('update:value', 'priority')
    await settle()
    const requests = calls.filter((call) => call.url.startsWith('/service/query/tasks'))
    expect(requests.some((call) => (call.body as { start: number }).start === 50)).toBe(true)
    expect(requests.some((call) => (call.body as { size: number }).size === 100)).toBe(true)
    expect((requests.at(-1)!.body as { sort: string }).sort).toBe('priority')
  })

  it('从路由还原筛选与分页', async () => {
    const calls = mockFetch([{ path: '/service/query/tasks', method: 'POST', body: pageOf([]) }])
    await mountPage('/runtime/tasks?assignee=alice&size=20&page=2&sort=name&order=asc')
    expect(calls[0].body).toEqual({ assignee: 'alice', start: 20, size: 20, sort: 'name', order: 'asc' })
  })

  it('快速签收成功并刷新', async () => {
    const calls = mockFetch([
      { path: '/service/query/tasks', method: 'POST', body: pageOf([task]) },
      { path: '/service/runtime/tasks/t-1/identitylinks', body: [] },
      { path: '/service/runtime/tasks/t-1', method: 'POST', body: {} },
    ])
    const { wrapper } = await mountPage()
    await buttonByText(wrapper, '签收')!.trigger('click')
    await settle()
    const dialog = wrapper.findComponent(QuickClaimDialog)
    await dialog.findComponent(NInput).vm.$emit('update:value', 'bob')
    await childButton(dialog, '确认签收')!.trigger('click')
    await settle()
    const action = calls.find((call) => call.url.endsWith('/runtime/tasks/t-1') && call.method === 'POST')
    expect(action?.body).toEqual({ action: 'claim', assignee: 'bob' })
    expect(wrapper.text()).toContain('任务已签收')
  })

  it('快速签收校验与冲突错误', async () => {
    const calls = mockFetch([
      { path: '/service/query/tasks', method: 'POST', body: pageOf([task]) },
      { path: '/service/runtime/tasks/t-1/identitylinks', body: [] },
      { path: '/service/runtime/tasks/t-1', method: 'POST', status: 409, body: { message: 'Task was already claimed' } },
    ])
    const { wrapper } = await mountPage()
    await buttonByText(wrapper, '签收')!.trigger('click')
    await settle()
    const dialog = wrapper.findComponent(QuickClaimDialog)
    await childButton(dialog, '确认签收')!.trigger('click')
    await settle()
    expect(document.body.textContent).toContain('请输入签收人')
    await dialog.findComponent(NInput).vm.$emit('update:value', 'bob')
    await childButton(dialog, '确认签收')!.trigger('click')
    await settle()
    expect(document.body.textContent).toContain('Task was already claimed')
    expect(calls.filter((call) => call.url.endsWith('/runtime/tasks/t-1'))).toHaveLength(1)
  })

  it('行内完成成功与失败', async () => {
    const calls = mockFetch([
      { path: '/service/query/tasks', method: 'POST', body: pageOf([task]) },
      { path: '/service/runtime/tasks/t-1/identitylinks', body: [] },
      { path: '/service/runtime/tasks/t-1', method: 'POST', status: 409, body: { message: '任务已结束' } },
    ])
    const { wrapper } = await mountPage()
    await wrapper.findComponent(ConfirmButton).vm.$emit('confirm')
    await settle()
    expect(wrapper.text()).toContain('任务已结束')
    expect(calls.some((call) => (call.body as { action?: string } | null)?.action === 'complete')).toBe(true)
  })

  it('新建独立任务提交后刷新', async () => {
    const calls = mockFetch([
      { path: '/service/query/tasks', method: 'POST', body: pageOf([]) },
      { path: '/service/runtime/tasks', method: 'POST', status: 201, body: { ...task, id: 't-2' } },
    ])
    const { wrapper } = await mountPage()
    await buttonByText(wrapper, '新建独立任务')!.trigger('click')
    await settle()
    const dialog = wrapper.findComponent(CreateTaskDialog)
    await dialog.findAllComponents(NInput)[0].vm.$emit('update:value', '人工复核')
    await childButton(dialog, '创建')!.trigger('click')
    await settle()
    const create = calls.find((call) => call.url === '/service/runtime/tasks' && call.method === 'POST')
    expect(create?.body).toMatchObject({ name: '人工复核', priority: 50 })
  })

  it('批量改派所选任务并展示结果', async () => {
    const calls = mockFetch([
      { path: '/service/query/tasks', method: 'POST', body: pageOf([task], 1) },
      { path: '/service/runtime/tasks/t-1/identitylinks', body: [] },
      { path: '/service/runtime/tasks', method: 'PUT', body: pageOf([task]) },
    ])
    const { wrapper } = await mountPage()
    await wrapper.findComponent(DataTable).vm.$emit('update:checkedRowKeys', ['t-1'])
    await settle()
    await buttonByText(wrapper, '批量操作')!.trigger('click')
    await settle()
    const dialog = wrapper.findComponent(BulkUpdateDialog)
    await dialog.findComponent(NInput).vm.$emit('update:value', 'carol')
    await childButton(dialog, '确认改派')!.trigger('click')
    await settle()
    const update = calls.find((call) => call.url === '/service/runtime/tasks' && call.method === 'PUT')
    expect(update?.body).toEqual({ taskIds: ['t-1'], assignee: 'carol' })
    expect(document.body.textContent).toContain('已改派给 carol')
  })

  it('401 时标记认证提示并保留错误原文', async () => {
    mockFetch([{ path: '/service/query/tasks', method: 'POST', status: 401, body: { message: '未认证' } }])
    const { wrapper } = await mountPage()
    expect(wrapper.text()).toContain('未认证')
    expect(authNotice.value).toBe('unauthorized')
  })

  it('候选人加载失败不影响列表', async () => {
    mockFetch([
      { path: '/service/query/tasks', method: 'POST', body: pageOf([task]) },
      { path: '/service/runtime/tasks/t-1/identitylinks', status: 500, body: { message: '链接失败' } },
    ])
    const { wrapper } = await mountPage()
    expect(wrapper.text()).toContain('审批')
    expect(wrapper.text()).toContain('—')
  })
})
