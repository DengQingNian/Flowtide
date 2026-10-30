// 任务动作栏测试：按钮可见性、弹窗校验与动作载荷。
import { afterEach, describe, expect, it } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'
import { NButton, NCheckbox, NInput, NInputNumber } from 'naive-ui'
import TaskActionBar from '@/pages/runtime/components/task/TaskActionBar.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import type { Task } from '@/api/task'

afterEach(() => {
  document.body.innerHTML = ''
})

/** 任务样例 */
const baseTask: Task = {
  id: 't-1',
  name: '审批',
  assignee: 'alice',
  owner: 'alice',
  priority: 50,
  createTime: '2026-10-30T03:00:00Z',
}

// 查找指定文本的按钮。
// @param wrapper 包装器
// @param text 文本
// @return 按钮
function button(wrapper: VueWrapper, text: string) {
  return wrapper.findAllComponents(NButton).find((item) => item.text().includes(text))!
}

// 点击弹窗确认按钮。
// @param wrapper 包装器
// @return 无
async function confirmModal(wrapper: VueWrapper): Promise<void> {
  await button(wrapper, '确认').trigger('click')
}

describe('TaskActionBar', () => {
  it('未签收时展示签收且不展示取消签收与解决委派', () => {
    const wrapper = mount(TaskActionBar, { props: { task: { ...baseTask, assignee: null } } })
    expect(wrapper.text()).toContain('签收')
    expect(wrapper.text()).not.toContain('取消签收')
    expect(wrapper.text()).not.toContain('解决委派')
  })

  it('已签收时展示取消签收，委派待处理时展示解决委派', () => {
    const wrapper = mount(TaskActionBar, { props: { task: { ...baseTask, delegationState: 'pending' } } })
    expect(wrapper.text()).toContain('取消签收')
    expect(wrapper.text()).toContain('解决委派')
    expect(wrapper.text()).toContain('所有者 alice')
  })

  it('签收弹窗校验签收人并提交', async () => {
    const wrapper = mount(TaskActionBar, { props: { task: { ...baseTask, assignee: null } }, attachTo: document.body })
    await button(wrapper, '签收').trigger('click')
    await confirmModal(wrapper)
    expect(document.body.textContent).toContain('请输入签收人')
    await wrapper.findComponent(NInput).vm.$emit('update:value', 'bob')
    await confirmModal(wrapper)
    expect(wrapper.emitted('action')?.[0]).toEqual(['claim', { assignee: 'bob' }])
  })

  it('委派弹窗校验被委托人并提交', async () => {
    const wrapper = mount(TaskActionBar, { props: { task: baseTask }, attachTo: document.body })
    await button(wrapper, '委派').trigger('click')
    await confirmModal(wrapper)
    expect(document.body.textContent).toContain('请输入被委托人')
    await wrapper.findComponent(NInput).vm.$emit('update:value', 'carol')
    await confirmModal(wrapper)
    expect(wrapper.emitted('action')?.[0]).toEqual(['delegate', { assignee: 'carol' }])
  })

  it('改派支持清空办理人', async () => {
    const wrapper = mount(TaskActionBar, { props: { task: baseTask }, attachTo: document.body })
    await button(wrapper, '改派').trigger('click')
    await wrapper.findComponent(NInput).vm.$emit('update:value', '')
    await confirmModal(wrapper)
    expect(wrapper.emitted('action')?.[0]).toEqual(['assign', { assignee: '' }])
  })

  it('到期日校验非法格式并提交合法值与清空', async () => {
    const wrapper = mount(TaskActionBar, { props: { task: baseTask }, attachTo: document.body })
    await button(wrapper, '到期日').trigger('click')
    await wrapper.findComponent(NInput).vm.$emit('update:value', 'not-a-date')
    await confirmModal(wrapper)
    expect(document.body.textContent).toContain('到期时间格式非法')
    await wrapper.findComponent(NInput).vm.$emit('update:value', '2026-12-01T00:00:00Z')
    await confirmModal(wrapper)
    await button(wrapper, '到期日').trigger('click')
    await wrapper.findComponent(NInput).vm.$emit('update:value', '')
    await confirmModal(wrapper)
    const actions = wrapper.emitted('action')!
    expect(actions[0]).toEqual(['due', { value: '2026-12-01T00:00:00Z' }])
    expect(actions[1]).toEqual(['due', { value: null }])
  })

  it('优先级校验数字并提交', async () => {
    const wrapper = mount(TaskActionBar, { props: { task: baseTask }, attachTo: document.body })
    await button(wrapper, '优先级').trigger('click')
    await wrapper.findComponent(NInputNumber).vm.$emit('update:value', null)
    await confirmModal(wrapper)
    expect(document.body.textContent).toContain('请输入合法优先级')
    await wrapper.findComponent(NInputNumber).vm.$emit('update:value', 80.6)
    await confirmModal(wrapper)
    expect(wrapper.emitted('action')?.[0]).toEqual(['priority', { value: 80 }])
  })

  it('删除弹窗提交原因与级联选项', async () => {
    const wrapper = mount(TaskActionBar, { props: { task: baseTask }, attachTo: document.body })
    const remove = wrapper.findAllComponents(ConfirmButton).find((item) => item.props('label') === '删除')!
    remove.vm.$emit('confirm')
    await nextTick()
    await wrapper.findComponent(NInput).vm.$emit('update:value', '重复任务')
    await wrapper.findComponent(NCheckbox).vm.$emit('update:checked', true)
    await confirmModal(wrapper)
    expect(wrapper.emitted('action')?.[0]).toEqual(['delete', { reason: '重复任务', cascadeHistory: true }])
  })

  it('取消签收、解决委派与完成分别触发事件', async () => {
    const wrapper = mount(TaskActionBar, { props: { task: { ...baseTask, delegationState: 'pending' } } })
    const confirmButtons = wrapper.findAllComponents(ConfirmButton)
    confirmButtons.find((item) => item.props('label') === '取消签收')!.vm.$emit('confirm')
    confirmButtons.find((item) => item.props('label') === '解决委派')!.vm.$emit('confirm')
    await button(wrapper, '完成').trigger('click')
    expect(wrapper.emitted('action')).toEqual([['unclaim', {}], ['resolve', {}]])
    expect(wrapper.emitted('complete')).toHaveLength(1)
  })

  it('关闭弹窗与任务切换会清理状态', async () => {
    const wrapper = mount(TaskActionBar, { props: { task: { ...baseTask, assignee: null } }, attachTo: document.body })
    await button(wrapper, '签收').trigger('click')
    await nextTick()
    expect(document.body.textContent).toContain('签收任务')
    const cancel = wrapper.findAllComponents(NButton).find((item) => item.text() === '取消')!
    await cancel.trigger('click')
    await wrapper.setProps({ task: { ...baseTask, id: 't-2' } })
    expect(document.body.textContent).not.toContain('签收人')
  })
})
