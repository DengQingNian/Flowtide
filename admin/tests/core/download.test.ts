import { afterEach, describe, expect, it, vi } from 'vitest'
import { downloadBlob, downloadJson, downloadText, extensionOf, sanitizeFileName } from '@/lib/download'

afterEach(() => {
  vi.restoreAllMocks()
  document.body.innerHTML = ''
})

describe('下载工具', () => {
  it('清理文件名', () => {
    expect(sanitizeFileName('a/b:c*?.bpmn')).toBe('a_b_c__.bpmn')
    expect(sanitizeFileName('   ')).toBe('download')
  })

  it('下载文本并触发链接', () => {
    const click = vi.fn()
    const createElement = vi.spyOn(document, 'createElement')
    createElement.mockImplementation(((tag: string) => {
      const element = document.createElementNS('http://www.w3.org/1999/xhtml', tag) as HTMLElement
      if (tag === 'a') element.click = click
      return element
    }) as typeof document.createElement)
    downloadText('test.txt', 'hello')
    expect(click).toHaveBeenCalled()
    createElement.mockRestore()
  })

  it('下载 JSON 与 Blob', () => {
    const click = vi.fn()
    const original = document.createElement.bind(document)
    vi.spyOn(document, 'createElement').mockImplementation(((tag: string) => {
      const element = original(tag)
      if (tag === 'a') element.click = click
      return element
    }) as typeof document.createElement)
    downloadJson('x.json', { a: 1 })
    downloadBlob('x.bin', new Blob(['x']))
    expect(click).toHaveBeenCalledTimes(2)
  })

  it('提取扩展名', () => {
    expect(extensionOf('a.BPMN')).toBe('bpmn')
    expect(extensionOf('noext')).toBe('')
  })
})
