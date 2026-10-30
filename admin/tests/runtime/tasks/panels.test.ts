// 任务详情子面板测试：变量、评论、附件、身份链接、子任务与事件的关键操作。
import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { NButton, NInput, NInputNumber, NSelect } from 'naive-ui'
import TaskVariablesPanel from '@/pages/runtime/components/task/TaskVariablesPanel.vue'
import TaskCommentsPanel from '@/pages/runtime/components/task/TaskCommentsPanel.vue'
import TaskAttachmentsPanel from '@/pages/runtime/components/task/TaskAttachmentsPanel.vue'
import TaskIdentityLinksPanel from '@/pages/runtime/components/task/TaskIdentityLinksPanel.vue'
import TaskSubTasksPanel from '@/pages/runtime/components/task/TaskSubTasksPanel.vue'
import TaskEventsPanel from '@/pages/runtime/components/task/TaskEventsPanel.vue'
import VariableEditor from '@/components/VariableEditor.vue'
import VariableTable from '@/components/VariableTable.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import { mockFetch, pageOf, settle } from '../../helpers'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

// 按文本查找按钮。
// @param wrapper 包装器
// @param text 文本
// @return 按钮
function button(wrapper: VueWrapper, text: string) {
  return wrapper.findAllComponents(NButton).find((item) => item.text().includes(text))!
}

describe('TaskVariablesPanel', () => {
  it('加载、新增、编辑、删除与查看原始数据', async () => {
    const calls = mockFetch([
      { path: '/service/runtime/tasks/t-1/variables/amount/data', contentType: 'application/octet-stream', text: '10' },
      { path: '/service/runtime/tasks/t-1/variables', method: 'GET', body: [{ name: 'amount', value: 10 }] },
      { path: '/service/runtime/tasks/t-1/variables', method: 'POST', status: 201, body: { name: 'x', value: 'y' } },
      { path: '/service/runtime/tasks/t-1/variables/amount', method: 'PUT', status: 204 },
      { path: '/service/runtime/tasks/t-1/variables/amount', method: 'DELETE', status: 204 },
    ])
    const wrapper = mount(TaskVariablesPanel, { props: { taskId: 't-1' }, attachTo: document.body })
    await settle()
    expect(wrapper.text()).toContain('amount')
    expect(wrapper.text()).toContain('long')

    await button(wrapper, '新增变量').trigger('click')
    await settle()
    const editor = wrapper.findComponent(VariableEditor)
    await editor.findAllComponents(NInput)[0].vm.$emit('update:value', 'x')
    await editor.findAllComponents(NInput)[1].vm.$emit('update:value', 'y')
    await button(editor, '保存').trigger('click')
    await settle()
    expect(calls.find((call) => call.method === 'POST')?.body).toMatchObject({ name: 'x', type: 'string', value: 'y' })

    wrapper.findComponent(VariableTable).vm.$emit('edit', { name: 'amount', value: 10, type: 'long' })
    await settle()
    await wrapper.findComponent(VariableEditor).findComponent(NInputNumber).vm.$emit('update:value', 99)
    await button(wrapper.findComponent(VariableEditor), '保存').trigger('click')
    await settle()
    expect(calls.find((call) => call.method === 'PUT')?.body).toEqual({ value: 99 })

    wrapper.findComponent(VariableTable).vm.$emit('remove', { name: 'amount', value: 10 })
    await settle()
    await button(wrapper, '确认删除').trigger('click')
    await settle()
    expect(calls.some((call) => call.method === 'DELETE')).toBe(true)

    wrapper.findComponent(VariableTable).vm.$emit('raw', { name: 'amount', value: 10 })
    await settle()
    expect(document.body.textContent).toContain('原始数据：amount')
    expect(document.body.textContent).toContain('10')
  })

  it('保存与删除失败展示错误', async () => {
    mockFetch([
      { path: '/service/runtime/tasks/t-1/variables', method: 'GET', body: [{ name: 'a', value: 1 }] },
      { path: '/service/runtime/tasks/t-1/variables', method: 'POST', status: 409, body: { message: '变量冲突' } },
      { path: '/service/runtime/tasks/t-1/variables/a', method: 'DELETE', status: 500, body: { message: '删除失败' } },
    ])
    const wrapper = mount(TaskVariablesPanel, { props: { taskId: 't-1' }, attachTo: document.body })
    await settle()
    await button(wrapper, '新增变量').trigger('click')
    await settle()
    await wrapper.findComponent(VariableEditor).findAllComponents(NInput)[0].vm.$emit('update:value', 'a')
    await button(wrapper.findComponent(VariableEditor), '保存').trigger('click')
    await settle()
    expect(wrapper.text()).toContain('变量冲突')
    wrapper.findComponent(VariableTable).vm.$emit('remove', { name: 'a', value: 1 })
    await settle()
    await button(wrapper, '确认删除').trigger('click')
    await settle()
    expect(wrapper.text()).toContain('删除失败')
  })
})

describe('TaskCommentsPanel', () => {
  it('加载、校验、新增与删除评论', async () => {
    const calls = mockFetch([
      { path: /^\/service\/runtime\/tasks\/t-1\/comments\//, method: 'DELETE', status: 204 },
      { path: '/service/runtime/tasks/t-1/comments?', method: 'GET', body: pageOf([{ id: 'c1', message: '已阅', userId: 'alice', time: '2026-10-30T03:00:00Z' }], 1) },
      { path: /^\/service\/runtime\/tasks\/t-1\/comments$/, method: 'POST', status: 201, body: { id: 'c2', message: 'ok' } },
    ])
    const wrapper = mount(TaskCommentsPanel, { props: { taskId: 't-1' }, attachTo: document.body })
    await settle()
    expect(wrapper.text()).toContain('已阅')
    await button(wrapper, '发布').trigger('click')
    await settle()
    expect(wrapper.text()).toContain('评论内容不能为空')
    await wrapper.findAllComponents(NInput)[0].vm.$emit('update:value', 'ok')
    await wrapper.findAllComponents(NInput)[1].vm.$emit('update:value', 'bob')
    await button(wrapper, '发布').trigger('click')
    await settle()
    expect(calls.find((call) => call.method === 'POST')?.body).toEqual({ message: 'ok', userId: 'bob' })
    wrapper.findComponent(ConfirmButton).vm.$emit('confirm')
    await settle()
    expect(calls.some((call) => call.method === 'DELETE')).toBe(true)
  })

  it('新增失败展示错误', async () => {
    mockFetch([
      { path: '/service/runtime/tasks/t-1/comments?', method: 'GET', body: pageOf([]) },
      { path: /^\/service\/runtime\/tasks\/t-1\/comments$/, method: 'POST', status: 500, body: { message: '发布失败' } },
    ])
    const wrapper = mount(TaskCommentsPanel, { props: { taskId: 't-1' } })
    await settle()
    await wrapper.findAllComponents(NInput)[0].vm.$emit('update:value', 'ok')
    await button(wrapper, '发布').trigger('click')
    await settle()
    expect(wrapper.text()).toContain('发布失败')
  })
})

describe('TaskAttachmentsPanel', () => {
  it('校验文件、上传、下载与删除', async () => {
    const calls = mockFetch([
      { path: '/service/runtime/tasks/t-1/attachments?', method: 'GET', body: pageOf([{ id: 'a1', name: '材料.txt', type: 'text/plain', time: '2026-10-30T03:00:00Z' }], 1) },
      { path: '/service/runtime/tasks/t-1/attachments', method: 'POST', status: 201, body: { id: 'a2', name: 'y.txt' } },
      { path: '/service/runtime/tasks/t-1/attachments/a1/content', contentType: 'application/octet-stream', text: 'abc' },
      { path: '/service/runtime/tasks/t-1/attachments/a1', method: 'DELETE', status: 204 },
    ])
    const wrapper = mount(TaskAttachmentsPanel, { props: { taskId: 't-1' }, attachTo: document.body })
    await settle()
    expect(wrapper.text()).toContain('材料.txt')
    await button(wrapper, '上传').trigger('click')
    await settle()
    expect(wrapper.text()).toContain('请先选择文件')
    const file = new File(['hello'], 'y.txt', { type: 'text/plain' })
    const input = wrapper.find('input[type="file"]')
    Object.defineProperty(input.element, 'files', { value: [file], configurable: true })
    await input.trigger('change')
    await button(wrapper, '上传').trigger('click')
    await settle()
    expect(calls.find((call) => call.method === 'POST')?.body).toBeInstanceOf(FormData)
    await button(wrapper, '下载').trigger('click')
    await settle()
    expect(wrapper.text()).toContain('已开始下载')
    wrapper.findComponent(ConfirmButton).vm.$emit('confirm')
    await settle()
    expect(calls.some((call) => call.method === 'DELETE')).toBe(true)
  })

  it('上传与下载失败展示错误', async () => {
    mockFetch([
      { path: '/service/runtime/tasks/t-1/attachments?', method: 'GET', body: pageOf([{ id: 'a1', name: 'x' }], 1) },
      { path: '/service/runtime/tasks/t-1/attachments', method: 'POST', status: 500, body: { message: '上传失败' } },
      { path: '/service/runtime/tasks/t-1/attachments/a1/content', status: 500, body: { message: '下载失败' } },
    ])
    const wrapper = mount(TaskAttachmentsPanel, { props: { taskId: 't-1' }, attachTo: document.body })
    await settle()
    const file = new File(['x'], 'x.txt', { type: 'text/plain' })
    const input = wrapper.find('input[type="file"]')
    Object.defineProperty(input.element, 'files', { value: [file], configurable: true })
    await input.trigger('change')
    await button(wrapper, '上传').trigger('click')
    await settle()
    expect(wrapper.text()).toContain('上传失败')
    await button(wrapper, '下载').trigger('click')
    await settle()
    expect(wrapper.text()).toContain('下载失败')
  })
})

describe('TaskIdentityLinksPanel', () => {
  it('加载、校验、添加用户/组与移除', async () => {
    const calls = mockFetch([
      { path: '/service/runtime/tasks/t-1/identitylinks/users/alice', method: 'DELETE', status: 204 },
      { path: '/service/runtime/tasks/t-1/identitylinks', method: 'GET', body: [{ type: 'candidate', userId: 'alice' }, { type: 'candidate', groupId: 'g1' }] },
      { path: '/service/runtime/tasks/t-1/identitylinks', method: 'POST', status: 201 },
    ])
    const wrapper = mount(TaskIdentityLinksPanel, { props: { taskId: 't-1' }, attachTo: document.body })
    await settle()
    expect(wrapper.text()).toContain('alice')
    expect(wrapper.text()).toContain('g1')
    await button(wrapper, '添加').trigger('click')
    await settle()
    expect(wrapper.text()).toContain('请输入用户 ID')
    await wrapper.findComponent(NInput).vm.$emit('update:value', 'bob')
    await button(wrapper, '添加').trigger('click')
    await settle()
    expect(calls.filter((call) => call.method === 'POST')[0]?.body).toEqual({ type: 'candidate', userId: 'bob' })
    await wrapper.findComponent(NSelect).vm.$emit('update:value', 'group')
    await wrapper.findComponent(NInput).vm.$emit('update:value', 'g2')
    await button(wrapper, '添加').trigger('click')
    await settle()
    expect(calls.filter((call) => call.method === 'POST')[1]?.body).toEqual({ type: 'candidate', groupId: 'g2' })
    wrapper.findAllComponents(ConfirmButton)[0].vm.$emit('confirm')
    await settle()
    expect(calls.some((call) => call.method === 'DELETE')).toBe(true)
  })

  it('添加失败展示错误', async () => {
    mockFetch([
      { path: '/service/runtime/tasks/t-1/identitylinks', method: 'GET', body: [] },
      { path: '/service/runtime/tasks/t-1/identitylinks', method: 'POST', status: 400, body: { message: '用户不存在' } },
    ])
    const wrapper = mount(TaskIdentityLinksPanel, { props: { taskId: 't-1' } })
    await settle()
    await wrapper.findComponent(NInput).vm.$emit('update:value', 'bob')
    await button(wrapper, '添加').trigger('click')
    await settle()
    expect(wrapper.text()).toContain('用户不存在')
  })
})

describe('任务子面板渲染', () => {
  it('子任务与事件表格', async () => {
    mockFetch([
      { path: '/service/runtime/tasks/t-1/subtasks', body: [{ id: 't-2', name: '复核', priority: 10, createTime: '2026-10-30T03:00:00Z' }] },
      { path: '/service/runtime/tasks/t-1/events', body: [{ id: 1, action: 'create', timestamp: '2026-10-30T03:00:00Z', userId: 'alice', data: '{}' }] },
    ])
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/runtime/tasks/:id', component: { template: '<div />' } }],
    })
    await router.push('/runtime/tasks/t-1')
    await router.isReady()
    const subtasks = mount(TaskSubTasksPanel, { props: { taskId: 't-1' }, global: { plugins: [router] } })
    const events = mount(TaskEventsPanel, { props: { taskId: 't-1' } })
    await settle()
    expect(subtasks.text()).toContain('复核')
    expect(events.text()).toContain('创建')
    expect(events.text()).toContain('alice')

    mockFetch([
      { path: '/service/runtime/tasks/t-1/subtasks', status: 500, body: { message: '子任务失败' } },
      { path: '/service/runtime/tasks/t-1/events', status: 500, body: { message: '事件失败' } },
    ])
    const badSubtasks = mount(TaskSubTasksPanel, { props: { taskId: 't-1' }, global: { plugins: [router] } })
    const badEvents = mount(TaskEventsPanel, { props: { taskId: 't-1' } })
    await settle()
    expect(badSubtasks.text()).toContain('子任务失败')
    expect(badEvents.text()).toContain('事件失败')
  })
})
