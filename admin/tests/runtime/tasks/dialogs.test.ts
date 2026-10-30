// 任务弹窗测试：独立任务创建、批量操作与快速签收。
import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { NButton, NCheckbox, NInput, NInputNumber, NSelect } from 'naive-ui'
import CreateTaskDialog from '@/pages/runtime/components/task/CreateTaskDialog.vue'
import BulkUpdateDialog from '@/pages/runtime/components/task/BulkUpdateDialog.vue'
import QuickClaimDialog from '@/pages/runtime/components/task/QuickClaimDialog.vue'
import { mockFetch, pageOf, settle } from '../../helpers'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

/** 任务样例 */
const task = { id: 't-1', name: '审批', priority: 50, createTime: '2026-10-30T03:00:00Z' }

// 按文本查找按钮。
// @param wrapper 包装器
// @param text 文本
// @return 按钮
function button(wrapper: VueWrapper, text: string) {
  return wrapper.findAllComponents(NButton).find((item) => item.text().includes(text))!
}

describe('CreateTaskDialog', () => {
  it('校验名称、到期时间并创建任务', async () => {
    const calls = mockFetch([{ path: '/service/runtime/tasks', method: 'POST', status: 201, body: task }])
    const wrapper = mount(CreateTaskDialog, { props: { show: false }, attachTo: document.body })
    await wrapper.setProps({ show: true })
    await settle()
    await button(wrapper, '创建').trigger('click')
    expect(document.body.textContent).toContain('任务名称不能为空')
    await wrapper.findAllComponents(NInput).find((item) => item.props('placeholder') === '例如 人工复核')!.vm.$emit('update:value', '人工复核')
    const dueInput = wrapper.findAllComponents(NInput).find((item) => item.props('placeholder') === '2026-11-01T00:00:00Z')!
    await dueInput.vm.$emit('update:value', 'bad-date')
    await button(wrapper, '创建').trigger('click')
    expect(document.body.textContent).toContain('到期时间格式非法')
    await dueInput.vm.$emit('update:value', '2026-12-01T00:00:00Z')
    await wrapper.findComponent(NInputNumber).vm.$emit('update:value', 70)
    await button(wrapper, '创建').trigger('click')
    await settle()
    expect(calls[0].body).toMatchObject({ name: '人工复核', priority: 70, dueDate: '2026-12-01T00:00:00Z' })
    expect(wrapper.emitted('created')).toHaveLength(1)
    expect(wrapper.emitted('update:show')?.at(-1)).toEqual([false])
  })

  it('创建失败展示服务端原文', async () => {
    mockFetch([{ path: '/service/runtime/tasks', method: 'POST', status: 409, body: { message: '任务冲突' } }])
    const wrapper = mount(CreateTaskDialog, { props: { show: false }, attachTo: document.body })
    await wrapper.setProps({ show: true })
    await wrapper.findAllComponents(NInput)[0].vm.$emit('update:value', '任务')
    await button(wrapper, '创建').trigger('click')
    await settle()
    expect(document.body.textContent).toContain('任务冲突')
  })

  it('取消关闭弹窗', async () => {
    const wrapper = mount(CreateTaskDialog, { props: { show: true } })
    await button(wrapper, '取消').trigger('click')
    expect(wrapper.emitted('update:show')?.[0]).toEqual([false])
  })
})

describe('BulkUpdateDialog', () => {
  it('校验改派目标并批量提交', async () => {
    const calls = mockFetch([{ path: '/service/runtime/tasks', method: 'PUT', body: pageOf([task]) }])
    const wrapper = mount(BulkUpdateDialog, { props: { show: true, tasks: [task] }, attachTo: document.body })
    await settle()
    await button(wrapper, '确认改派').trigger('click')
    await settle()
    expect(document.body.textContent).toContain('请输入改派后的办理人')
    await wrapper.findComponent(NInput).vm.$emit('update:value', 'carol')
    await button(wrapper, '确认改派').trigger('click')
    await settle()
    expect(calls[0].body).toEqual({ taskIds: ['t-1'], assignee: 'carol' })
    expect(document.body.textContent).toContain('已改派给 carol')
    expect(wrapper.emitted('done')!.length).toBeGreaterThan(0)
  })

  it('删除模式逐条反馈成功与失败', async () => {
    const calls = mockFetch([
      { path: '/service/runtime/tasks/t-1', method: 'DELETE', status: 204 },
      { path: '/service/runtime/tasks/t-2', method: 'DELETE', status: 500, body: { message: '删除被拒绝' } },
    ])
    const wrapper = mount(BulkUpdateDialog, { props: { show: true, tasks: [task, { ...task, id: 't-2', name: '复核' }] }, attachTo: document.body })
    await settle()
    await wrapper.findComponent(NSelect).vm.$emit('update:value', 'delete')
    await wrapper.findComponent(NInput).vm.$emit('update:value', '清理')
    await wrapper.findComponent(NCheckbox).vm.$emit('update:checked', true)
    await button(wrapper, '确认删除').trigger('click')
    await settle()
    expect(calls).toHaveLength(2)
    expect(calls[0].url).toContain('deleteReason=%E6%B8%85%E7%90%86')
    expect(document.body.textContent).toContain('已删除')
    expect(document.body.textContent).toContain('删除被拒绝')
    expect(document.body.textContent).toContain('成功 1 条，失败 1 条')
  })

  it('未选择任务与批量接口失败提示', async () => {
    mockFetch([{ path: '/service/runtime/tasks', method: 'PUT', status: 500, body: { message: '批量失败' } }])
    const wrapper = mount(BulkUpdateDialog, { props: { show: true, tasks: [] }, attachTo: document.body })
    await settle()
    await button(wrapper, '确认改派').trigger('click')
    await settle()
    expect(document.body.textContent).toContain('请先选择任务')
    await wrapper.setProps({ tasks: [task] })
    await wrapper.findComponent(NInput).vm.$emit('update:value', 'carol')
    await button(wrapper, '确认改派').trigger('click')
    await settle()
    expect(document.body.textContent).toContain('批量失败')
  })

  it('关闭弹窗', async () => {
    const wrapper = mount(BulkUpdateDialog, { props: { show: true, tasks: [task] } })
    await button(wrapper, '关闭').trigger('click')
    expect(wrapper.emitted('update:show')?.[0]).toEqual([false])
  })
})

describe('QuickClaimDialog', () => {
  it('校验并提交签收', async () => {
    const calls = mockFetch([{ path: '/service/runtime/tasks/t-1', method: 'POST', body: {} }])
    const wrapper = mount(QuickClaimDialog, { props: { show: false, task }, attachTo: document.body })
    await wrapper.setProps({ show: true })
    await settle()
    await button(wrapper, '确认签收').trigger('click')
    expect(document.body.textContent).toContain('请输入签收人')
    await wrapper.findComponent(NInput).vm.$emit('update:value', 'bob')
    await button(wrapper, '确认签收').trigger('click')
    await settle()
    expect(calls[0].body).toEqual({ action: 'claim', assignee: 'bob' })
    expect(wrapper.emitted('done')).toHaveLength(1)
  })

  it('签收冲突展示错误', async () => {
    mockFetch([{ path: '/service/runtime/tasks/t-1', method: 'POST', status: 409, body: { message: 'Task was already claimed' } }])
    const wrapper = mount(QuickClaimDialog, { props: { show: true, task }, attachTo: document.body })
    await settle()
    await wrapper.findComponent(NInput).vm.$emit('update:value', 'bob')
    await button(wrapper, '确认签收').trigger('click')
    await settle()
    expect(document.body.textContent).toContain('Task was already claimed')
  })
})
