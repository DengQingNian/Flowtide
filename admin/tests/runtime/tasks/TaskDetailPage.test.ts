// 任务详情页面测试：加载、错误、动作分发、页签懒加载与完成跳转。
import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { createMemoryHistory, createRouter, type Router } from 'vue-router'
import { NButton } from 'naive-ui'
import TaskDetailPage from '@/pages/runtime/TaskDetailPage.vue'
import TaskActionBar from '@/pages/runtime/components/task/TaskActionBar.vue'
import TaskFormPanel from '@/pages/runtime/components/task/TaskFormPanel.vue'
import { authNotice, clearAuthNotice } from '@/stores/ui'
import { mockFetch, settle } from '../../helpers'

afterEach(() => {
  vi.unstubAllGlobals()
  clearAuthNotice()
  document.body.innerHTML = ''
})

/** 任务样例 */
const task = {
  id: 't-1',
  name: '审批',
  assignee: 'alice',
  owner: 'alice',
  priority: 50,
  createTime: '2026-10-30T03:00:00Z',
  dueDate: '2026-11-01T00:00:00Z',
  processInstanceId: 'pi-1',
  processDefinitionId: 'leave:1:1',
  delegationState: null,
  suspended: false,
  tenantId: '',
}

/** 默认模拟路由 */
function defaultRoutes(): Parameters<typeof mockFetch>[0] {
  return [
    { path: /^\/service\/runtime\/tasks\/t-1\/form$/, status: 400, body: { code: 'REST-0406', message: '任务未定义表单' } },
    { path: /^\/service\/runtime\/tasks\/t-1\/variables$/, body: [{ name: 'amount', value: 10 }] },
    { path: /^\/service\/runtime\/tasks\/t-1\/comments\?/, body: { data: [{ id: 'c1', message: '已阅', userId: 'alice' }], total: 1, start: 0, size: 1 } },
    { path: /^\/service\/runtime\/tasks\/t-1\/attachments\?/, body: { data: [{ id: 'a1', name: '材料.txt' }], total: 1, start: 0, size: 1 } },
    { path: /^\/service\/runtime\/tasks\/t-1\/comments\/c1$/, method: 'DELETE', status: 204 },
    { path: /^\/service\/runtime\/tasks\/t-1\/attachments\/a1$/, method: 'DELETE', status: 204 },
    { path: /^\/service\/runtime\/tasks\/t-1\/identitylinks$/, body: [{ type: 'candidate', userId: 'bob' }] },
    { path: /^\/service\/runtime\/tasks\/t-1\/identitylinks\/users\/bob$/, method: 'DELETE', status: 204 },
    { path: /^\/service\/runtime\/tasks\/t-1\/subtasks$/, body: [{ ...task, id: 't-2', name: '子任务' }] },
    { path: /^\/service\/runtime\/tasks\/t-1\/events$/, body: [{ id: 1, action: 'create', timestamp: '2026-10-30T03:00:00Z', userId: 'alice' }] },
    { path: '/service/runtime/tasks/t-1?', method: 'DELETE', status: 204 },
    { path: /^\/service\/runtime\/tasks\/t-1$/, method: 'GET', body: task },
    { path: /^\/service\/runtime\/tasks\/t-1$/, method: 'POST', body: {} },
    { path: /^\/service\/runtime\/tasks\/t-1$/, method: 'PUT', body: task },
    { path: /^\/service\/runtime\/tasks\/t-1$/, method: 'DELETE', status: 204 },
  ]
}

// 挂载任务详情页。
// @param routes 模拟路由
// @param path 初始路由
// @return 包装器与路由
async function mountPage(
  routes: Parameters<typeof mockFetch>[0] = defaultRoutes(),
  path = '/runtime/tasks/t-1',
): Promise<{ wrapper: VueWrapper; router: Router; calls: ReturnType<typeof mockFetch> }> {
  const calls = mockFetch(routes)
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/runtime/tasks', name: 'tasks', component: { template: '<div>列表</div>' } },
      { path: '/runtime/tasks/:id', name: 'task-detail', component: TaskDetailPage },
      { path: '/history/tasks/:id', component: { template: '<div>历史</div>' } },
      { path: '/runtime/instances/:id', component: { template: '<div>实例</div>' } },
      { path: '/repository/definitions/:id', component: { template: '<div>定义</div>' } },
    ],
  })
  await router.push(path)
  await router.isReady()
  const wrapper = mount(TaskDetailPage, { global: { plugins: [router] }, attachTo: document.body })
  await settle()
  return { wrapper, router, calls }
}

// 按文本查找按钮。
// @param wrapper 包装器
// @param text 文本
// @return 按钮包装器
function buttonByText(wrapper: VueWrapper, text: string) {
  return wrapper.findAllComponents(NButton).find((item) => item.text().includes(text))
}

describe('TaskDetailPage', () => {
  it('加载任务并展示基本信息与链接', async () => {
    const { wrapper } = await mountPage()
    expect(wrapper.text()).toContain('审批')
    expect(wrapper.text()).toContain('alice')
    expect(wrapper.text()).toContain('50')
    expect(wrapper.text()).toContain('2026-11-01')
    expect(wrapper.text()).toContain('leave:1:1')
    expect(wrapper.text()).toContain('变量 JSON 完成')
  })

  it('加载失败展示错误并可重试', async () => {
    const routes = defaultRoutes().filter((route) => !(route.method === 'GET' && route.path instanceof RegExp && route.path.test('/service/runtime/tasks/t-1') && !route.path.test('/form')))
    routes.push({ path: /^\/service\/runtime\/tasks\/t-1$/, method: 'GET', status: 404, body: { message: '任务不存在' } })
    const { wrapper } = await mountPage(routes)
    expect(wrapper.text()).toContain('任务不存在')
    expect(wrapper.text()).not.toContain('变量 JSON 完成')
  })

  it('所有动作通过真实端点提交', async () => {
    const { wrapper, calls } = await mountPage()
    const bar = wrapper.findComponent(TaskActionBar)
    bar.vm.$emit('action', 'claim', { assignee: 'bob' })
    await settle()
    bar.vm.$emit('action', 'delegate', { assignee: 'carol' })
    await settle()
    bar.vm.$emit('action', 'unclaim', {})
    await settle()
    bar.vm.$emit('action', 'resolve', {})
    await settle()
    bar.vm.$emit('action', 'assign', { assignee: 'dave' })
    await settle()
    bar.vm.$emit('action', 'due', { value: '2026-12-01T00:00:00Z' })
    await settle()
    bar.vm.$emit('action', 'due', { value: null })
    await settle()
    bar.vm.$emit('action', 'priority', { value: 80 })
    await settle()
    const posts = calls.filter((call) => call.method === 'POST').map((call) => call.body)
    expect(posts).toContainEqual({ action: 'claim', assignee: 'bob' })
    expect(posts).toContainEqual({ action: 'delegate', assignee: 'carol' })
    expect(posts).toContainEqual({ action: 'unclaim' })
    expect(posts).toContainEqual({ action: 'resolve' })
    const puts = calls.filter((call) => call.method === 'PUT').map((call) => call.body)
    expect(puts).toContainEqual({ assignee: 'dave' })
    expect(puts).toContainEqual({ dueDate: '2026-12-01T00:00:00Z' })
    expect(puts).toContainEqual({ dueDate: null })
    expect(puts).toContainEqual({ priority: 80 })
  })

  it('动作失败展示服务端原文且保留页面', async () => {
    const routes = defaultRoutes().map((route) =>
      route.method === 'POST' ? { ...route, status: 409, body: { message: '任务已被签收' } } : route,
    )
    const { wrapper } = await mountPage(routes)
    wrapper.findComponent(TaskActionBar).vm.$emit('action', 'claim', { assignee: 'bob' })
    await settle()
    expect(wrapper.text()).toContain('任务已被签收')
    expect(wrapper.text()).toContain('审批')
  })

  it('401 动作标记认证提示', async () => {
    const routes = defaultRoutes().map((route) =>
      route.method === 'POST' ? { ...route, status: 401, body: { message: '未认证' } } : route,
    )
    const { wrapper } = await mountPage(routes)
    wrapper.findComponent(TaskActionBar).vm.$emit('action', 'claim', { assignee: 'bob' })
    await settle()
    expect(wrapper.text()).toContain('未认证')
    expect(authNotice.value).toBe('unauthorized')
  })

  it('删除后返回任务列表', async () => {
    const { wrapper, router, calls } = await mountPage()
    wrapper.findComponent(TaskActionBar).vm.$emit('action', 'delete', { reason: '重复', cascadeHistory: true })
    await settle()
    const del = calls.find((call) => call.method === 'DELETE')
    expect(del?.url).toContain('cascadeHistory=true')
    expect(del?.url).toContain('deleteReason=%E9%87%8D%E5%A4%8D')
    await new Promise((resolve) => setTimeout(resolve, 100))
    await settle()
    expect(router.currentRoute.value.path).toBe('/runtime/tasks')
  })

  it('删除失败保留页面并提示', async () => {
    const routes = defaultRoutes().map((route) => (route.method === 'DELETE' ? { ...route, status: 403, body: { message: '无权限' } } : route))
    const { wrapper, router } = await mountPage(routes)
    wrapper.findComponent(TaskActionBar).vm.$emit('action', 'delete', {})
    await settle()
    expect(wrapper.text()).toContain('无权限')
    expect(router.currentRoute.value.path).toBe('/runtime/tasks/t-1')
    expect(authNotice.value).toBe('forbidden')
  })

  it('页签按需加载变量、评论、附件、身份链接、子任务与事件', async () => {
    const { wrapper, calls } = await mountPage()
    await buttonByText(wrapper, '变量')!.trigger('click')
    await settle()
    expect(wrapper.text()).toContain('任务局部变量')
    expect(wrapper.text()).toContain('amount')
    await buttonByText(wrapper, '评论')!.trigger('click')
    await settle()
    expect(wrapper.text()).toContain('已阅')
    await buttonByText(wrapper, '附件')!.trigger('click')
    await settle()
    expect(wrapper.text()).toContain('材料.txt')
    await buttonByText(wrapper, '身份链接')!.trigger('click')
    await settle()
    expect(wrapper.text()).toContain('bob')
    await buttonByText(wrapper, '子任务')!.trigger('click')
    await settle()
    expect(wrapper.text()).toContain('子任务（1）')
    await buttonByText(wrapper, '事件')!.trigger('click')
    await settle()
    expect(wrapper.text()).toContain('创建')
    expect(calls.some((call) => call.url.includes('/variables'))).toBe(true)
    expect(calls.some((call) => call.url.includes('/comments?'))).toBe(true)
    expect(calls.some((call) => call.url.includes('/subtasks'))).toBe(true)
    expect(calls.some((call) => call.url.includes('/events'))).toBe(true)
  }, 20000)

  it('完成表单后跳回列表', async () => {
    const { wrapper, router } = await mountPage()
    wrapper.findComponent(TaskFormPanel).vm.$emit('completed')
    await settle()
    expect(router.currentRoute.value.path).toBe('/runtime/tasks')
  })
})
