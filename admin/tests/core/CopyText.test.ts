import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import CopyText from '@/components/CopyText.vue'

afterEach(() => {
  vi.restoreAllMocks()
  vi.useRealTimers()
})

// 覆盖剪贴板实现。
// @param writeText 写入实现
// @return 无
function installClipboard(writeText: (text: string) => Promise<void>): void {
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: { writeText: vi.fn(writeText) },
  })
}

describe('CopyText', () => {
  it('优先使用 Clipboard API 并给出反馈', async () => {
    vi.useFakeTimers()
    installClipboard(async () => undefined)
    const wrapper = mount(CopyText, { props: { value: 'abc', display: '复制我' } })
    await wrapper.find('button').trigger('click')
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith('abc')
    expect(wrapper.emitted('copied')?.[0]).toEqual(['abc'])
    expect(wrapper.find('button').attributes('title')).toBe('已复制')
    vi.advanceTimersByTime(2000)
    await wrapper.vm.$nextTick()
    expect(wrapper.find('button').attributes('title')).toBe('复制')
  })

  it('Clipboard 失败时回退 execCommand', async () => {
    installClipboard(async () => {
      throw new Error('denied')
    })
    const execCommand = vi.fn(() => true)
    Object.defineProperty(document, 'execCommand', { configurable: true, value: execCommand })
    const wrapper = mount(CopyText, { props: { value: 'xyz' }, attachTo: document.body })
    await wrapper.find('button').trigger('click')
    expect(execCommand).toHaveBeenCalledWith('copy')
    expect(wrapper.emitted('copied')).toBeTruthy()
    wrapper.unmount()
  })

  it('两种方式都失败时不反馈', async () => {
    installClipboard(async () => {
      throw new Error('denied')
    })
    Object.defineProperty(document, 'execCommand', { configurable: true, value: vi.fn(() => false) })
    const wrapper = mount(CopyText, { props: { value: 'xyz' } })
    await wrapper.find('button').trigger('click')
    expect(wrapper.emitted('copied')).toBeFalsy()
  })
})
