<script setup lang="ts">
// 左侧分组导航：七个业务域、当前项高亮、分组折叠与整体折叠。
import { computed, reactive } from 'vue'
import { useRoute } from 'vue-router'
import type { Component } from 'vue'
import {
  Activity,
  BellRing,
  Boxes,
  CircleCheck,
  CirclePlay,
  Database,
  FileClock,
  FolderTree,
  GitBranch,
  History,
  Inbox,
  LayoutDashboard,
  ListChecks,
  Package,
  PlayCircle,
  Settings,
  SlidersHorizontal,
  TriangleAlert,
  UserCheck,
  Users,
  Wrench,
} from '@lucide/vue'

/** 导航项 */
interface NavItem {
  label: string
  to: string
  icon: Component
}

/** 导航分组 */
interface NavGroup {
  label: string
  items: NavItem[]
}

const props = defineProps<{ collapsed: boolean }>()

const route = useRoute()
/** 站点图标地址 */
const logoUrl = `${import.meta.env.BASE_URL}favicon.svg`

/** 导航分组定义 */
const groups: NavGroup[] = [
  { label: '概览', items: [{ label: '仪表盘', to: '/dashboard', icon: LayoutDashboard }] },
  {
    label: '工作流',
    items: [
      { label: '流程分类', to: '/workflow/category', icon: FolderTree },
      { label: '流程模型', to: '/workflow/model', icon: Boxes },
      { label: '流程配置', to: '/workflow/template', icon: SlidersHorizontal },
      { label: '待办任务', to: '/workflow/message/todo', icon: Inbox },
      { label: '已办任务', to: '/workflow/message/done', icon: CircleCheck },
      { label: '流程实例', to: '/workflow/instance', icon: CirclePlay },
      { label: '委托配置', to: '/workflow/entrust', icon: UserCheck },
      { label: '催办配置', to: '/workflow/remind', icon: BellRing },
      { label: '流程异常', to: '/workflow/exception', icon: TriangleAlert },
    ],
  },
  {
    label: '仓库',
    items: [
      { label: '部署', to: '/repository/deployments', icon: Package },
      { label: '流程定义', to: '/repository/definitions', icon: GitBranch },
      { label: '流程模型', to: '/repository/models', icon: Boxes },
    ],
  },
  {
    label: '运行态',
    items: [
      { label: '流程实例', to: '/runtime/instances', icon: PlayCircle },
      { label: '任务', to: '/runtime/tasks', icon: ListChecks },
    ],
  },
  {
    label: '运维',
    items: [
      { label: '作业', to: '/management/jobs/executable', icon: Wrench },
      { label: '批处理', to: '/management/batches', icon: Activity },
      { label: '表数据', to: '/management/tables', icon: Database },
      { label: '引擎属性', to: '/management/engine', icon: Settings },
      { label: '审计日志', to: '/management/audit', icon: FileClock },
    ],
  },
  {
    label: '历史',
    items: [
      { label: '历史流程', to: '/history/processes', icon: History },
      { label: '历史任务', to: '/history/tasks', icon: ListChecks },
      { label: '历史活动', to: '/history/activities', icon: Activity },
      { label: '历史变量', to: '/history/variables', icon: Database },
    ],
  },
  {
    label: '身份',
    items: [
      { label: '用户', to: '/identity/users', icon: Users },
      { label: '组', to: '/identity/groups', icon: Users },
      { label: '角色', to: '/identity/roles', icon: Users },
    ],
  },
  {
    label: '系统',
    items: [
      { label: '指标', to: '/system/metrics', icon: Activity },
      { label: '关于', to: '/system/about', icon: Settings },
    ],
  },
]

/** 折叠分组状态 */
const closedGroups = reactive<Record<string, boolean>>({})

// 判断导航项是否处于激活状态。
// @param to 目标路径
// @return 是否激活
function isActive(to: string): boolean {
  return route.path === to || route.path.startsWith(`${to}/`)
}

// 切换分组折叠。
// @param label 分组名称
// @return 无
function toggleGroup(label: string): void {
  closedGroups[label] = !closedGroups[label]
}

/** 是否处于折叠模式 */
const isCollapsed = computed(() => props.collapsed)
</script>

<template>
  <aside class="side-nav" :class="{ 'side-nav--collapsed': isCollapsed }">
    <router-link class="side-nav__brand" to="/dashboard" title="Flowtide 管理台">
      <img class="side-nav__logo" :src="logoUrl" alt="Flowtide" />
      <span class="side-nav__label">Flowtide 管理台</span>
    </router-link>
    <nav class="side-nav__groups" aria-label="主导航">
      <div v-for="group in groups" :key="group.label" class="side-nav__group">
        <button
          v-if="!isCollapsed"
          class="side-nav__group-title"
          type="button"
          :aria-expanded="!closedGroups[group.label]"
          @click="toggleGroup(group.label)"
        >
          <span>{{ group.label }}</span>
          <span aria-hidden="true">{{ closedGroups[group.label] ? '＋' : '－' }}</span>
        </button>
        <template v-if="!closedGroups[group.label] || isCollapsed">
          <router-link
            v-for="item in group.items"
            :key="item.to"
            class="side-nav__link"
            :class="{ 'side-nav__link--active': isActive(item.to) }"
            :to="item.to"
            :title="item.label"
          >
            <component :is="item.icon" class="side-nav__icon" :size="16" />
            <span class="side-nav__label">{{ item.label }}</span>
          </router-link>
        </template>
      </div>
    </nav>
    <div class="side-nav__footer">
      <span v-if="!isCollapsed">Flowtide · 单二进制管理台</span>
      <span v-else>FT</span>
    </div>
  </aside>
</template>