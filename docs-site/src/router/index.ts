import { createRouter, createWebHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'
import { pages } from '@/lib/content'
import type { TocItem } from '@/lib/content'

const progressToc: TocItem[] = [
  { id: 'snapshot', text: '当前快照', level: 2 },
  { id: 'stages', text: '阶段进展', level: 2 },
  { id: 'capabilities', text: '能力分布', level: 2 },
  { id: 'migration', text: '迁移状态', level: 2 },
  { id: 'commits', text: '最近提交', level: 2 },
  { id: 'gaps', text: '下一步', level: 2 },
]

const compatibilityToc: TocItem[] = [
  { id: 'summary', text: '状态分布', level: 2 },
  { id: 'matrix', text: '能力矩阵', level: 2 },
  { id: 'legend', text: '状态说明', level: 2 },
]

const restToc: TocItem[] = [
  { id: 'summary', text: '端点统计', level: 2 },
  { id: 'filters', text: '筛选端点', level: 2 },
]

const routes: RouteRecordRaw[] = [
  {
    path: '/',
    name: 'landing',
    component: () => import('@/pages/LandingPage.vue'),
    meta: { layout: 'landing', title: 'Flowable-go · 重构进度文档站' },
  },
  {
    path: '/docs',
    name: 'docs-index',
    component: () => import('@/pages/DocsIndexPage.vue'),
    meta: { layout: 'docs', title: '文档中心', toc: [] },
  },
  {
    path: '/progress',
    name: 'progress',
    component: () => import('@/pages/ProgressPage.vue'),
    meta: { layout: 'docs', title: '重构进度看板', toc: progressToc },
  },
  {
    path: '/compatibility',
    name: 'compatibility',
    component: () => import('@/pages/CompatibilityPage.vue'),
    meta: { layout: 'docs', title: '能力矩阵', toc: compatibilityToc },
  },
  {
    path: '/rest-explorer',
    name: 'rest-explorer',
    component: () => import('@/pages/RestExplorerPage.vue'),
    meta: { layout: 'docs', title: 'REST 浏览器', toc: restToc },
  },
  ...pages.map((page) => ({
    path: page.route,
    name: `doc-${page.key.replace(/[\/]/g, '-')}`,
    component: () => import('@/pages/DocPage.vue'),
    meta: {
      layout: 'docs',
      title: page.title,
      description: page.description,
      toc: page.headings,
    },
  })),
  {
    path: '/:pathMatch(.*)*',
    name: 'not-found',
    component: () => import('@/pages/NotFoundPage.vue'),
    meta: { layout: 'docs', title: '页面不存在', toc: [] },
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
  scrollBehavior(to, _from, savedPosition) {
    if (savedPosition) return savedPosition
    if (to.hash) return { el: to.hash, top: 96, behavior: 'smooth' }
    return { top: 0 }
  },
})

// syncTitle 根据路由更新浏览器标题。
// @param to 目标路由
// @return 无
function syncTitle(to: { meta: Record<string, unknown> }): void {
  const title = typeof to.meta.title === 'string' ? to.meta.title : 'Flowable-go'
  document.title = title === 'Flowable-go · 重构进度文档站' ? title : `${title} · Flowable-go`
}

router.afterEach(syncTitle)

export default router
