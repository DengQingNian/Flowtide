import { describe, expect, it } from 'vitest'
import { searchDocs } from '@/lib/search'

describe('全文搜索', () => {
  it('可以搜索中文主题', () => {
    const hits = searchDocs('迁移')
    expect(hits.length).toBeGreaterThan(0)
    expect(hits.some((hit) => hit.route.includes('migration'))).toBe(true)
  })

  it('可以搜索英文术语', () => {
    const hits = searchDocs('mTLS')
    expect(hits.length).toBeGreaterThan(0)
  })

  it('空查询返回空结果', () => {
    expect(searchDocs('   ')).toEqual([])
  })

  it('限制返回数量', () => {
    expect(searchDocs('流程', 3).length).toBeLessThanOrEqual(3)
  })
})
