import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import StatusBadge from '@/components/StatusBadge.vue'

describe('StatusBadge', () => {
  it('渲染中文状态', () => {
    const wrapper = mount(StatusBadge, { props: { status: 'verified' } })
    expect(wrapper.text()).toContain('已验证')
    expect(wrapper.classes()).toContain('status-badge--verified')
  })

  it('未知状态回退为原值', () => {
    const wrapper = mount(StatusBadge, { props: { status: 'custom' } })
    expect(wrapper.text()).toContain('custom')
  })
})
