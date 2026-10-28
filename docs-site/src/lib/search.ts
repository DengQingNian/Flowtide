import MiniSearch from 'minisearch'
import { content, sectionLabel } from './content'
import type { SearchDocument } from './content'

let index: MiniSearch<SearchDocument> | null = null

// tokenize 把中英文拆成可检索词元：拉丁词小写、中文按二元组切分。
// @param text 原始文本
// @return 词元数组
function tokenize(text: string): string[] {
  const tokens: string[] = []
  const matches = text.toLowerCase().match(/[a-z0-9_]+|[\u3400-\u9fff]+/g) ?? []
  for (const match of matches) {
    if (/^[\u3400-\u9fff]+$/.test(match)) {
      if (match.length === 1) {
        tokens.push(match)
      } else {
        for (let index = 0; index < match.length - 1; index += 1) {
          tokens.push(match.slice(index, index + 2))
        }
      }
    } else {
      tokens.push(match)
    }
  }
  return tokens
}

// getIndex 惰性构建全文搜索索引。
// @return MiniSearch 实例
export function getIndex(): MiniSearch<SearchDocument> {
  if (index) return index
  index = new MiniSearch<SearchDocument>({
    fields: ['title', 'description', 'headings', 'text'],
    storeFields: ['title', 'route', 'section', 'description'],
    tokenize,
    searchOptions: {
      boost: { title: 4, description: 2, headings: 2 },
      fuzzy: 0.2,
      prefix: true,
      combineWith: 'AND',
    },
  })
  index.addAll(content.search)
  return index
}

// SearchHit 是搜索结果。
export interface SearchHit {
  id: string
  title: string
  route: string
  section: string
  description: string
  score: number
}

// searchDocs 执行搜索并返回前若干条。
// @param query 查询词
// @param limit 最大条数
// @return 搜索结果
export function searchDocs(query: string, limit = 8): SearchHit[] {
  const trimmed = query.trim()
  if (!trimmed) return []
  const hits = getIndex()
    .search(trimmed, { tokenize })
    .map((result) => {
      const section = String(result.section)
      // 查询命中分区名时优先展示该分区的落地页，例如输入“迁移”优先返回迁移总览
      const sectionBoost = sectionLabel(section).includes(trimmed) ? 8 : 0
      return {
        id: String(result.id),
        title: String(result.title),
        route: String(result.route),
        section,
        description: String(result.description),
        score: result.score + sectionBoost,
      }
    })
  return hits.sort((a, b) => b.score - a.score).slice(0, limit)
}
