import { describe, expect, it } from 'vitest'
import { content, neighbors, pageByRoute, pages, pagesInSection, sectionMeta, statusLabel } from '@/lib/content'

describe('生成的文档内容', () => {
  it('包含足够的页面与路由', () => {
    expect(content.pageCount).toBeGreaterThanOrEqual(45)
    expect(pages.length).toBe(content.pageCount)
    expect(new Set(pages.map((page) => page.route)).size).toBe(pages.length)
    expect(pages.every((page) => page.route.startsWith('/'))).toBe(true)
  })

  it('每个页面都有必要的元数据与目录', () => {
    for (const page of pages) {
      expect(page.title.length).toBeGreaterThan(0)
      expect(page.description.length).toBeGreaterThan(0)
      expect(page.html.length).toBeGreaterThan(200)
      expect(page.headings.length).toBeGreaterThan(0)
      expect(['verified', 'prototype', 'planned']).toContain(page.status)
      expect(page.readingMinutes).toBeGreaterThan(0)
      expect(page.sourcePath.startsWith('docs-site/src/content/')).toBe(true)
    }
  })

  it('标题锚点在页面内唯一', () => {
    for (const page of pages) {
      const ids = page.headings.map((heading) => heading.id)
      expect(new Set(ids).size).toBe(ids.length)
    }
  })

  it('侧栏分组都能找到内容', () => {
    for (const section of sectionMeta) {
      expect(pagesInSection(section.id).length).toBeGreaterThan(0)
    }
  })

  it('上一篇/下一篇按分组顺序连接', () => {
    const page = pageByRoute.get('/docs/architecture/runtime')
    expect(page).toBeDefined()
    const { prev, next } = neighbors('/docs/architecture/runtime')
    expect(prev?.route).toBe('/docs/architecture/overview')
    expect(next?.route).toBe('/docs/architecture/persistence')
  })

  it('状态标签为中文', () => {
    expect(statusLabel('verified')).toBe('已验证')
    expect(statusLabel('prototype')).toBe('原型可用')
    expect(statusLabel('planned')).toBe('规划中')
  })
})
