<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ChevronRight, List, X } from '@lucide/vue'
import SiteFooter from '@/components/SiteFooter.vue'
import SiteHeader from '@/components/SiteHeader.vue'
import DocsSidebar from '@/components/DocsSidebar.vue'
import TableOfContents from '@/components/TableOfContents.vue'
import { pageByRoute, sectionLabel } from '@/lib/content'
import type { TocItem } from '@/lib/content'

const route = useRoute()
const sidebarOpen = ref(false)
const tocOpen = ref(false)
const activeHeading = ref('')
const scrollProgress = ref(0)
const headings = computed<TocItem[]>(() => (route.meta.toc as TocItem[] | undefined) ?? [])
const page = computed(() => pageByRoute.get(route.path))
const section = computed(() => (page.value ? sectionLabel(page.value.section) : ''))
const pageTitle = computed(() => page.value?.title ?? (route.meta.title as string) ?? '')

// updateScroll 计算阅读进度与当前标题。
// @return 无
function updateScroll(): void {
  const scrollable = document.documentElement.scrollHeight - window.innerHeight
  scrollProgress.value = scrollable > 0 ? Math.min(100, (window.scrollY / scrollable) * 100) : 0
  let current = ''
  for (const heading of headings.value) {
    const element = document.getElementById(heading.id)
    if (element && element.getBoundingClientRect().top <= 140) current = heading.id
  }
  activeHeading.value = current
}

// onKeydown 处理 Esc 关闭移动端抽屉。
// @param event 键盘事件
// @return 无
function onKeydown(event: KeyboardEvent): void {
  if (event.key === 'Escape') {
    sidebarOpen.value = false
    tocOpen.value = false
  }
}

watch(
  () => route.fullPath,
  async () => {
    sidebarOpen.value = false
    tocOpen.value = false
    await nextTick()
    updateScroll()
  },
)

watch(sidebarOpen, (open) => {
  document.body.style.overflow = open ? 'hidden' : ''
})

onMounted(() => {
  window.addEventListener('scroll', updateScroll, { passive: true })
  window.addEventListener('keydown', onKeydown)
  updateScroll()
})
onBeforeUnmount(() => {
  window.removeEventListener('scroll', updateScroll)
  window.removeEventListener('keydown', onKeydown)
  document.body.style.overflow = ''
})
</script>

<template>
  <div class="app-shell docs-shell">
    <SiteHeader />
    <div class="scroll-progress" :style="{ width: scrollProgress + '%' }" />
    <div class="docs-frame" :class="{ 'docs-frame--wide': headings.length === 0 }">
      <aside class="sidebar-desktop" aria-label="文档侧栏">
        <DocsSidebar />
      </aside>

      <div class="docs-main">
        <div class="mobile-toolbar">
          <button type="button" class="toolbar-button" @click="sidebarOpen = true">
            <List :size="16" /> 目录
          </button>
          <button v-if="headings.length" type="button" class="toolbar-button" @click="tocOpen = true">
            本页目录
          </button>
        </div>

        <div class="breadcrumb" aria-label="面包屑">
          <router-link to="/docs">文档</router-link>
          <template v-if="section">
            <ChevronRight :size="13" />
            <span>{{ section }}</span>
          </template>
          <template v-if="pageTitle">
            <ChevronRight :size="13" />
            <span class="breadcrumb__current">{{ pageTitle }}</span>
          </template>
        </div>

        <slot />

        <div class="docs-footer-actions">
          <router-link to="/compatibility" class="footer-action">查看能力矩阵</router-link>
          <a href="https://github.com/DengQingNian/Flowtide" target="_blank" rel="noopener" class="footer-action">报告问题</a>
        </div>
      </div>

      <aside class="toc-desktop" aria-label="本页目录">
        <TableOfContents :headings="headings" :active="activeHeading" />
      </aside>
    </div>

    <Teleport to="body">
      <div v-if="sidebarOpen" class="drawer-overlay" @click.self="sidebarOpen = false">
        <div class="drawer drawer--left">
          <div class="drawer-head">
            <span>文档目录</span>
            <button type="button" class="icon-button" aria-label="关闭目录" @click="sidebarOpen = false"><X :size="18" /></button>
          </div>
          <DocsSidebar mobile @navigate="sidebarOpen = false" />
        </div>
      </div>
      <div v-if="tocOpen" class="drawer-overlay" @click.self="tocOpen = false">
        <div class="drawer drawer--right">
          <div class="drawer-head">
            <span>本页目录</span>
            <button type="button" class="icon-button" aria-label="关闭目录" @click="tocOpen = false"><X :size="18" /></button>
          </div>
          <TableOfContents :headings="headings" :active="activeHeading" @navigate="tocOpen = false" />
        </div>
      </div>
    </Teleport>

    <SiteFooter />
  </div>
</template>
