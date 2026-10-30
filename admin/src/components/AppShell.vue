<script setup lang="ts">
// 应用外壳：侧栏、顶栏、服务健康横幅与页面内容区。
import { computed, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { Info, Monitor, Moon, PanelLeftClose, PanelLeftOpen, Sun } from '@lucide/vue'
import SideNav from './SideNav.vue'
import ServiceBanner from './ServiceBanner.vue'
import AuthNotice from './AuthNotice.vue'
import { checkService, serviceStatus, sidebarCollapsed, toggleSidebar } from '@/stores/ui'
import { cycleTheme, isDark, themeMode, themeModeLabel } from '@/lib/theme'

const route = useRoute()
const pageTitle = computed(() => (route.meta.title as string) || '管理台')
const pageGroup = computed(() => (route.meta.group as string) || '')
const themeIcon = computed(() => (themeMode.value === 'system' ? Monitor : isDark.value ? Moon : Sun))
const statusLabel = computed(() =>
  serviceStatus.value === 'ok' ? '服务正常' : serviceStatus.value === 'degraded' ? '服务异常' : '状态未知',
)

let timer: number | undefined

// 启动健康检查并定时刷新。
// @return 无
function startHealthWatch(): void {
  void checkService()
  timer = window.setInterval(() => void checkService(), 30000)
}

onMounted(startHealthWatch)
onUnmounted(() => {
  if (timer !== undefined) window.clearInterval(timer)
})
</script>

<template>
  <div class="app-layout">
    <SideNav :collapsed="sidebarCollapsed" />
    <div class="app-main">
      <header class="app-header">
        <button
          class="app-header__toggle"
          type="button"
          :title="sidebarCollapsed ? '展开侧栏' : '折叠侧栏'"
          :aria-label="sidebarCollapsed ? '展开侧栏' : '折叠侧栏'"
          @click="toggleSidebar"
        >
          <PanelLeftOpen v-if="sidebarCollapsed" :size="16" />
          <PanelLeftClose v-else :size="16" />
        </button>
        <div class="app-header__crumb">
          <span v-if="pageGroup" class="app-header__group">{{ pageGroup }} /</span>
          <span class="app-header__title">{{ pageTitle }}</span>
        </div>
        <div class="app-header__spacer" />
        <span class="app-header__meta" :title="statusLabel">
          <span class="dot" :class="`dot--${serviceStatus === 'ok' ? 'ok' : serviceStatus === 'degraded' ? 'bad' : 'unknown'}`" />
          {{ statusLabel }}
        </span>
        <button
          class="icon-button"
          type="button"
          :title="`主题：${themeModeLabel(themeMode)}（点击切换）`"
          :aria-label="`主题：${themeModeLabel(themeMode)}`"
          @click="cycleTheme"
        >
          <component :is="themeIcon" :size="16" />
        </button>
        <router-link class="icon-button" to="/system/about" title="关于" aria-label="关于">
          <Info :size="16" />
        </router-link>
      </header>
      <AuthNotice />
      <ServiceBanner />
      <main class="app-content">
        <router-view v-slot="{ Component }">
          <component :is="Component" />
        </router-view>
      </main>
    </div>
  </div>
</template>
