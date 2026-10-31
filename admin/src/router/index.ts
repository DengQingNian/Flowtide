// 管理台路由：hash 模式，所有页面懒加载并标注标题与导航分组。
import { createRouter, createWebHashHistory, type RouteRecordRaw } from 'vue-router'

declare module 'vue-router' {
  interface RouteMeta {
    /** 页面标题 */
    title: string
    /** 导航分组 */
    group?: string
  }
}

/** 路由表 */
export const routes: RouteRecordRaw[] = [
  { path: '/', redirect: '/dashboard' },
  { path: '/dashboard', name: 'dashboard', component: () => import('@/pages/dashboard/DashboardPage.vue'), meta: { title: '仪表盘', group: '概览' } },
  { path: '/repository/deployments', name: 'deployments', component: () => import('@/pages/repository/DeploymentsPage.vue'), meta: { title: '部署', group: '仓库' } },
  { path: '/repository/deployments/:id', name: 'deployment-detail', component: () => import('@/pages/repository/DeploymentDetailPage.vue'), meta: { title: '部署详情', group: '仓库' } },
  { path: '/repository/definitions', name: 'definitions', component: () => import('@/pages/repository/DefinitionsPage.vue'), meta: { title: '流程定义', group: '仓库' } },
  { path: '/repository/definitions/:id', name: 'definition-detail', component: () => import('@/pages/repository/DefinitionDetailPage.vue'), meta: { title: '流程定义详情', group: '仓库' } },
  { path: '/repository/models', name: 'models', component: () => import('@/pages/repository/ModelsPage.vue'), meta: { title: '流程模型', group: '仓库' } },
  { path: '/repository/models/:id/design', name: 'model-designer', component: () => import('@/pages/repository/ModelDesignerPage.vue'), meta: { title: 'BPMN 设计器', group: '仓库' } },
  { path: '/runtime/instances', name: 'instances', component: () => import('@/pages/runtime/InstancesPage.vue'), meta: { title: '流程实例', group: '运行态' } },
  { path: '/runtime/instances/:id', name: 'instance-detail', component: () => import('@/pages/runtime/InstanceDetailPage.vue'), meta: { title: '实例详情', group: '运行态' } },
  { path: '/runtime/tasks', name: 'tasks', component: () => import('@/pages/runtime/TasksPage.vue'), meta: { title: '任务', group: '运行态' } },
  { path: '/runtime/tasks/:id', name: 'task-detail', component: () => import('@/pages/runtime/TaskDetailPage.vue'), meta: { title: '任务详情', group: '运行态' } },
  { path: '/management/jobs/:bucket', name: 'jobs', component: () => import('@/pages/management/JobsPage.vue'), meta: { title: '作业', group: '运维' } },
  { path: '/management/jobs/:bucket/:id', name: 'job-detail', component: () => import('@/pages/management/JobDetailPage.vue'), meta: { title: '作业详情', group: '运维' } },
  { path: '/management/batches', name: 'batches', component: () => import('@/pages/management/BatchesPage.vue'), meta: { title: '批处理', group: '运维' } },
  { path: '/management/batches/:id', name: 'batch-detail', component: () => import('@/pages/management/BatchDetailPage.vue'), meta: { title: '批处理详情', group: '运维' } },
  { path: '/management/tables', name: 'tables', component: () => import('@/pages/management/TablesPage.vue'), meta: { title: '表数据', group: '运维' } },
  { path: '/management/tables/:name', name: 'table-data', component: () => import('@/pages/management/TableDataPage.vue'), meta: { title: '表数据浏览', group: '运维' } },
  { path: '/management/engine', name: 'engine', component: () => import('@/pages/management/EnginePropertiesPage.vue'), meta: { title: '引擎属性', group: '运维' } },
  { path: '/management/audit', name: 'audit', component: () => import('@/pages/management/AuditPage.vue'), meta: { title: '审计日志', group: '运维' } },
  { path: '/history/processes', name: 'historic-processes', component: () => import('@/pages/history/HistoricProcessesPage.vue'), meta: { title: '历史流程', group: '历史' } },
  { path: '/history/processes/:id', name: 'historic-process-detail', component: () => import('@/pages/history/HistoricProcessDetailPage.vue'), meta: { title: '历史流程详情', group: '历史' } },
  { path: '/history/tasks', name: 'historic-tasks', component: () => import('@/pages/history/HistoricTasksPage.vue'), meta: { title: '历史任务', group: '历史' } },
  { path: '/history/tasks/:id', name: 'historic-task-detail', component: () => import('@/pages/history/HistoricTaskDetailPage.vue'), meta: { title: '历史任务详情', group: '历史' } },
  { path: '/history/activities', name: 'historic-activities', component: () => import('@/pages/history/HistoricActivitiesPage.vue'), meta: { title: '历史活动', group: '历史' } },
  { path: '/history/variables', name: 'historic-variables', component: () => import('@/pages/history/HistoricVariablesPage.vue'), meta: { title: '历史变量', group: '历史' } },
  { path: '/identity/users', name: 'users', component: () => import('@/pages/identity/UsersPage.vue'), meta: { title: '用户', group: '身份' } },
  { path: '/identity/users/:id', name: 'user-detail', component: () => import('@/pages/identity/UserDetailPage.vue'), meta: { title: '用户详情', group: '身份' } },
  { path: '/identity/groups', name: 'groups', component: () => import('@/pages/identity/GroupsPage.vue'), meta: { title: '组', group: '身份' } },
  { path: '/identity/roles', name: 'roles', component: () => import('@/pages/identity/RolesPage.vue'), meta: { title: '角色', group: '身份' } },
  { path: '/system/metrics', name: 'metrics', component: () => import('@/pages/system/MetricsPage.vue'), meta: { title: '指标', group: '系统' } },
  // ── 工作流（二次封装门面，006 系列九大页面）──
  { path: '/workflow/category', name: 'workflow-category', component: () => import('@/pages/workflow/CategoryPage.vue'), meta: { title: '流程分类', group: '工作流' } },
  { path: '/workflow/model', name: 'workflow-model', component: () => import('@/pages/workflow/ModelPage.vue'), meta: { title: '流程模型', group: '工作流' } },
  { path: '/workflow/template', name: 'workflow-template', component: () => import('@/pages/workflow/TemplateConfigPage.vue'), meta: { title: '流程配置', group: '工作流' } },
  { path: '/workflow/message/todo', name: 'workflow-todo', component: () => import('@/pages/workflow/TodoPage.vue'), meta: { title: '待办任务', group: '工作流' } },
  { path: '/workflow/message/done', name: 'workflow-done', component: () => import('@/pages/workflow/DonePage.vue'), meta: { title: '已办任务', group: '工作流' } },
  { path: '/workflow/instance', name: 'workflow-instance', component: () => import('@/pages/workflow/InstancePage.vue'), meta: { title: '流程实例', group: '工作流' } },
  { path: '/workflow/entrust', name: 'workflow-entrust', component: () => import('@/pages/workflow/EntrustPage.vue'), meta: { title: '委托配置', group: '工作流' } },
  { path: '/workflow/remind', name: 'workflow-remind', component: () => import('@/pages/workflow/RemindPage.vue'), meta: { title: '催办配置', group: '工作流' } },
  { path: '/workflow/exception', name: 'workflow-exception', component: () => import('@/pages/workflow/ExceptionPage.vue'), meta: { title: '流程异常', group: '工作流' } },
  { path: '/system/about', name: 'about', component: () => import('@/pages/system/AboutPage.vue'), meta: { title: '关于', group: '系统' } },
  { path: '/:pathMatch(.*)*', name: 'not-found', component: () => import('@/pages/NotFoundPage.vue'), meta: { title: '页面不存在' } },
]

/** 管理台路由实例 */
const router = createRouter({
  history: createWebHashHistory(),
  routes,
  scrollBehavior: () => ({ top: 0 }),
})

// 同步页面标题。
// @param to 目标路由
// @return 无
router.afterEach((to) => {
  const title = to.meta.title || '管理台'
  if (typeof document !== 'undefined') document.title = `${title} · Flowtide 管理台`
})

export default router
