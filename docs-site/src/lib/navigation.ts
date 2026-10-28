import type { DocPage } from './content'
import { pagesInSection, sectionMeta } from './content'

// NavLink 是顶部导航项。
export interface NavLink {
  label: string
  to: string
}

export const navLinks: NavLink[] = [
  { label: '文档', to: '/docs' },
  { label: '进度', to: '/progress' },
  { label: '兼容性', to: '/compatibility' },
  { label: 'REST', to: '/rest-explorer' },
  { label: '发布', to: '/releases' },
]

// SidebarEntry 是侧栏中的一个链接。
export interface SidebarEntry {
  route: string
  title: string
  status?: DocPage['status']
  pinned?: boolean
}

// SidebarGroup 是侧栏分组。
export interface SidebarGroup {
  id: string
  label: string
  entries: SidebarEntry[]
}

const pinned: Record<string, SidebarEntry[]> = {
  progress: [{ route: '/progress', title: '进度看板', pinned: true }],
  compatibility: [{ route: '/compatibility', title: '能力矩阵', pinned: true }],
  rest: [{ route: '/rest-explorer', title: 'REST 浏览器', pinned: true }],
}

// buildSidebar 生成侧栏分组与条目。
// @return 侧栏分组
export function buildSidebar(): SidebarGroup[] {
  return sectionMeta.map((section) => {
    const entries: SidebarEntry[] = [
      ...(pinned[section.id] ?? []),
      ...pagesInSection(section.id).map((page) => ({
        route: page.route,
        title: page.title,
        status: page.status,
      })),
    ]
    return { id: section.id, label: section.label, entries }
  })
}

// findGroupForRoute 返回路由所属的侧栏分组。
// @param route 路由
// @return 分组 ID
export function findGroupForRoute(route: string): string {
  if (route === '/progress') return 'progress'
  if (route === '/compatibility') return 'compatibility'
  if (route === '/rest-explorer') return 'rest'
  const section = sectionMeta.find((item) => route.startsWith(`/docs/${item.id}`))
  return section?.id ?? ''
}
