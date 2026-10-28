import { describe, expect, it } from 'vitest'
import { buildSidebar, findGroupForRoute, navLinks } from '@/lib/navigation'

describe('导航结构', () => {
  it('顶部导航覆盖主要入口', () => {
    expect(navLinks.map((link) => link.to)).toEqual(['/docs', '/progress', '/compatibility', '/rest-explorer', '/releases'])
  })

  it('每个侧栏分组都有条目', () => {
    const groups = buildSidebar()
    expect(groups.length).toBeGreaterThanOrEqual(10)
    expect(groups.every((group) => group.entries.length > 0)).toBe(true)
  })

  it('置顶页面出现在对应分组', () => {
    const groups = buildSidebar()
    expect(groups.find((group) => group.id === 'progress')?.entries[0].route).toBe('/progress')
    expect(groups.find((group) => group.id === 'compatibility')?.entries[0].route).toBe('/compatibility')
    expect(groups.find((group) => group.id === 'rest')?.entries[0].route).toBe('/rest-explorer')
  })

  it('可以按路由找到分组', () => {
    expect(findGroupForRoute('/progress')).toBe('progress')
    expect(findGroupForRoute('/docs/sdk/config')).toBe('sdk')
    expect(findGroupForRoute('/docs/migration/preflight')).toBe('migration')
  })
})
