import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import HistoricRawValueDialog from '@/pages/history/components/HistoricRawValueDialog.vue'
import { settle } from '../helpers'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

describe('历史原始数据弹窗', () => {
  it('展示文本内容并支持下载', async () => {
    const createObjectURL = vi.fn(() => 'blob:mock')
    vi.stubGlobal('URL', { ...URL, createObjectURL, revokeObjectURL: vi.fn() })
    const loader = vi.fn(async () => new Blob(['hello raw'], { type: 'text/plain' }))
    const wrapper = mount(HistoricRawValueDialog, {
      props: { show: true, title: '变量 amount', filename: 'amount.bin', loader },
      attachTo: document.body,
    })
    await settle()
    expect(loader).toHaveBeenCalledTimes(1)
    expect(document.body.textContent).toContain('hello raw')
    expect(document.body.textContent).toContain('大小：9 B')
    const download = Array.from(document.body.querySelectorAll('button')).find((button) => button.textContent?.includes('下载原始数据'))
    expect(download).toBeTruthy()
    download?.click()
    expect(createObjectURL).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('二进制内容只提示下载', async () => {
    const bytes = new Uint8Array([0, 1, 2, 3])
    const loader = async () => new Blob([bytes], { type: 'application/octet-stream' })
    const wrapper = mount(HistoricRawValueDialog, { props: { show: true, loader }, attachTo: document.body })
    await settle()
    expect(document.body.textContent).toContain('内容为二进制')
    wrapper.unmount()
  })

  it('空内容给出提示且无加载函数时说明缺失', async () => {
    const empty = mount(HistoricRawValueDialog, { props: { show: true, loader: async () => new Blob([]) }, attachTo: document.body })
    await settle()
    expect(document.body.textContent).toContain('（空内容）')
    empty.unmount()
    document.body.innerHTML = ''
    const missing = mount(HistoricRawValueDialog, { props: { show: true }, attachTo: document.body })
    await settle()
    expect(document.body.textContent).toContain('没有可读取的原始数据地址')
    missing.unmount()
  })

  it('加载失败展示 ErrorBlock 并可重试', async () => {
    const loader = vi.fn().mockRejectedValue(new Error('读取失败'))
    const wrapper = mount(HistoricRawValueDialog, { props: { show: true, loader }, attachTo: document.body })
    await settle()
    expect(document.body.textContent).toContain('读取失败')
    const retry = document.body.querySelector('button[title="重试"]') as HTMLButtonElement | null
    expect(retry).toBeTruthy()
    retry?.click()
    await nextTick()
    await settle()
    expect(loader).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })
})