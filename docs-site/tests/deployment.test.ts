import { afterEach, describe, expect, it, vi } from 'vitest'
import { withBaseLinks } from '@/lib/deployment'

describe('静态站点部署路径', () => {
  afterEach(() => vi.unstubAllEnvs())

  it('路由导航和文档锚点使用仓库子目录', async () => {
    vi.stubEnv('BASE_URL', '/Flowtide/')
    const { default: router } = await import('@/router')
    expect(router.resolve('/docs/sdk/config#配置').href).toBe('/Flowtide/docs/sdk/config#配置')
    expect(router.resolve('/').href).toBe('/Flowtide/')
  })

  it('根目录部署保留站内路径', () => {
    expect(withBaseLinks('<a href="/docs/sdk/config#配置">配置</a>', '/'))
      .toBe('<a href="/docs/sdk/config#配置">配置</a>')
  })

  it('仓库子目录部署为全部站内链接添加前缀', () => {
    const html = '<a href="/docs/sdk/config?from=导航#配置">配置</a><a href="/">首页</a>'
    expect(withBaseLinks(html, '/Flowtide/')).toBe(
      '<a href="/Flowtide/docs/sdk/config?from=导航#配置">配置</a><a href="/Flowtide/">首页</a>',
    )
  })

  it('保留外链、协议相对链接、页内锚点与相对链接', () => {
    const html = '<a href="https://example.com/docs">外链</a><a href="//example.com">协议相对</a>'
      + '<a href="#配置">锚点</a><a href="mailto:docs@example.com">邮件</a><a href="guide">相对</a>'
    expect(withBaseLinks(html, '/Flowtide/')).toBe(html)
  })

  it('支持自定义部署目录、空正文和无链接正文', () => {
    expect(withBaseLinks('<a href="/progress">进度</a>', '/team/docs/'))
      .toBe('<a href="/team/docs/progress">进度</a>')
    expect(withBaseLinks('', '/Flowtide/')).toBe('')
    expect(withBaseLinks('<p>正文</p>', '/Flowtide/')).toBe('<p>正文</p>')
  })
})
