// 任务表单面板测试：表单完成、变量 JSON 完成、无表单与错误分支。
import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { NButton, NInput } from 'naive-ui'
import TaskFormPanel from '@/pages/runtime/components/task/TaskFormPanel.vue'
import TaskForm from '@/components/TaskForm.vue'
import { clearAuthNotice, authNotice } from '@/stores/ui'
import { mockFetch, settle } from '../../helpers'

afterEach(() => {
  vi.unstubAllGlobals()
  clearAuthNotice()
  document.body.innerHTML = ''
})

// 按文本查找按钮。
// @param wrapper 包装器
// @param text 文本
// @return 按钮
function button(wrapper: VueWrapper, text: string) {
  return wrapper.findAllComponents(NButton).find((item) => item.text().includes(text))!
}

describe('TaskFormPanel', () => {
  it('无表单时校验变量 JSON 并完成任务', async () => {
    const calls = mockFetch([
      { path: '/service/runtime/tasks/t-1/form', status: 400, body: { code: 'REST-0406', message: '任务未定义表单' } },
      { path: '/service/runtime/tasks/t-1', method: 'POST', body: {} },
    ])
    const wrapper = mount(TaskFormPanel, { props: { taskId: 't-1' } })
    await settle()
    expect(wrapper.text()).toContain('变量 JSON 完成')
    await wrapper.findComponent(NInput).vm.$emit('update:value', '{bad')
    await button(wrapper, '完成任务').trigger('click')
    await settle()
    expect(wrapper.text()).toContain('变量 JSON 格式非法')
    await wrapper.findComponent(NInput).vm.$emit('update:value', '[1,2]')
    await button(wrapper, '完成任务').trigger('click')
    await settle()
    expect(wrapper.text()).toContain('变量 JSON 必须是对象')
    await wrapper.findComponent(NInput).vm.$emit('update:value', '{"approved":true}')
    await button(wrapper, '完成任务').trigger('click')
    await settle()
    expect(calls[1].body).toEqual({ action: 'complete', variables: [{ name: 'approved', value: true }] })
    expect(wrapper.emitted('completed')).toHaveLength(1)
  })

  it('渲染表单并提交变量', async () => {
    const calls = mockFetch([
      { path: '/service/runtime/tasks/t-1/form', body: { formKey: 'f', formProperties: [{ id: 'approved', name: '同意', type: 'boolean', required: true, readable: true, writable: true }] } },
      { path: '/service/runtime/tasks/t-1', method: 'POST', body: {} },
    ])
    const wrapper = mount(TaskFormPanel, { props: { taskId: 't-1' } })
    await settle()
    expect(wrapper.text()).toContain('表单完成')
    expect(wrapper.text()).toContain('同意')
    wrapper.findComponent(TaskForm).vm.$emit('submit', { approved: true })
    await settle()
    expect(calls[1].body).toEqual({ action: 'complete', variables: [{ name: 'approved', value: true }] })
  })

  it('提交失败展示服务端原文并保留内容', async () => {
    mockFetch([
      { path: '/service/runtime/tasks/t-1/form', status: 400, body: { code: 'REST-0406', message: '任务未定义表单' } },
      { path: '/service/runtime/tasks/t-1', method: 'POST', status: 400, body: { message: '变量类型错误' } },
    ])
    const wrapper = mount(TaskFormPanel, { props: { taskId: 't-1' } })
    await settle()
    await wrapper.findComponent(NInput).vm.$emit('update:value', '{"a":1}')
    await button(wrapper, '完成任务').trigger('click')
    await settle()
    expect(wrapper.text()).toContain('变量类型错误')
    expect(wrapper.findComponent(NInput).props('value')).toBe('{"a":1}')
  })

  it('401 提交标记认证提示并展示说明', async () => {
    mockFetch([
      { path: '/service/runtime/tasks/t-1/form', status: 400, body: { code: 'REST-0406', message: '任务未定义表单' } },
      { path: '/service/runtime/tasks/t-1', method: 'POST', status: 401, body: { message: '未认证' } },
    ])
    const wrapper = mount(TaskFormPanel, { props: { taskId: 't-1' } })
    await settle()
    await button(wrapper, '完成任务').trigger('click')
    await settle()
    expect(wrapper.text()).toContain('未认证')
    expect(wrapper.text()).toContain('服务端已启用认证或权限限制')
    expect(authNotice.value).toBe('unauthorized')
  })

  it('表单加载失败展示 ErrorBlock 并可重新加载', async () => {
    const calls = mockFetch([{ path: '/service/runtime/tasks/t-1/form', status: 500, body: { message: '表单服务不可用' } }])
    const wrapper = mount(TaskFormPanel, { props: { taskId: 't-1' } })
    await settle()
    expect(wrapper.text()).toContain('表单服务不可用')
    await wrapper.find('.icon-button').trigger('click')
    await settle()
    expect(calls.length).toBeGreaterThanOrEqual(2)
  })
})