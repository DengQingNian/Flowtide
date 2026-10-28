import contentRaw from '@/generated/content.json'

// TocItem 描述页面右侧目录中的标题。
export interface TocItem {
  id: string
  text: string
  level: number
}

// DocPage 是一篇由构建脚本生成的 Markdown 页面。
export interface DocPage {
  key: string
  route: string
  section: string
  slug: string
  title: string
  description: string
  order: number
  status: 'verified' | 'prototype' | 'planned'
  since: string
  tags: string[]
  verifiedCommit: string
  sourcePath: string
  html: string
  headings: TocItem[]
  readingMinutes: number
}

// SearchDocument 是搜索索引中的文档。
export interface SearchDocument {
  id: string
  route: string
  section: string
  title: string
  description: string
  headings: string[]
  text: string
}

// SectionMeta 描述侧栏分组。
export interface SectionMeta {
  id: string
  label: string
  description: string
}

interface ContentBundle {
  generatedAt: string
  commit: string
  pageCount: number
  pages: DocPage[]
  search: SearchDocument[]
}

export const content = contentRaw as ContentBundle
export const pages = content.pages
export const pageByRoute = new Map(pages.map((page) => [page.route, page]))

// sectionMeta 定义侧栏分组顺序与展示名称。
export const sectionMeta: SectionMeta[] = [
  { id: 'getting-started', label: '开始使用', description: '安装、快速开始与核心概念' },
  { id: 'progress', label: '重构进度', description: '截至当前提交的规模、能力与差距' },
  { id: 'architecture', label: '架构', description: '分层、运行时、持久化与模块' },
  { id: 'sdk', label: 'Go SDK', description: '嵌入引擎、配置、服务与限制' },
  { id: 'rest', label: 'REST API', description: '约定、认证、分域端点与覆盖' },
  { id: 'compatibility', label: '兼容性', description: 'BPMN、REST 与已知问题' },
  { id: 'migration', label: '迁移', description: '预检、转换、停写与回退' },
  { id: 'operations', label: '运维', description: '存储、备份、TLS 与可观测性' },
  { id: 'reference', label: '参考', description: '配置、错误码、变量与术语' },
  { id: 'releases', label: '发布', description: '当前版本与路线图' },
]

// sectionLabel 返回分组名称。
// @param id 分组 ID
// @return 分组名称
export function sectionLabel(id: string): string {
  return sectionMeta.find((section) => section.id === id)?.label ?? id
}

// pagesInSection 返回分组内的页面，按 order 排序。
// @param id 分组 ID
// @return 页面数组
export function pagesInSection(id: string): DocPage[] {
  return pages.filter((page) => page.section === id).sort((a, b) => a.order - b.order)
}

// neighbors 返回上一篇与下一篇（按全站分组顺序）。
// @param route 当前路由
// @return 上一篇与下一篇
export function neighbors(route: string): { prev?: DocPage; next?: DocPage } {
  const ordered = sectionMeta.flatMap((section) => pagesInSection(section.id))
  const index = ordered.findIndex((page) => page.route === route)
  if (index < 0) return {}
  return { prev: ordered[index - 1], next: ordered[index + 1] }
}

// statusLabel 返回状态的中文标签。
// @param status 状态值
// @return 中文标签
export function statusLabel(status: DocPage['status']): string {
  return { verified: '已验证', prototype: '原型可用', planned: '规划中' }[status] ?? status
}
