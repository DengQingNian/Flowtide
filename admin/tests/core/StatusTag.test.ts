import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import StatusTag from '@/components/StatusTag.vue'

describe('StatusTag', () => {
  it('按状态值推断色调', () => {
    expect(mount(StatusTag, { props: { value: 'active' } }).classes()).toContain('status-tag--success')
    expect(mount(StatusTag, { props: { value: '失败' } }).classes()).toContain('status-tag--danger')
    expect(mount(StatusTag, { props: { value: 'pending' } }).classes()).toContain('status-tag--warning')
    expect(mount(StatusTag, { props: { value: 'claimed' } }).classes()).toContain('status-tag--info')
    expect(mount(StatusTag, { props: { value: 'other' } }).classes()).toContain('status-tag--default')
  })

  it('支持显式 tone、label 与空值', () => {
    const wrapper = mount(StatusTag, { props: { value: null, tone: 'danger', label: '自定义' } })
    expect(wrapper.text()).toBe('自定义')
    expect(wrapper.classes()).toContain('status-tag--danger')
    expect(mount(StatusTag, { props: { value: '' } }).text()).toBe('—')
  })
})
