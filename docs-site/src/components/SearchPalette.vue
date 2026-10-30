<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ArrowRight, CornerDownLeft, Search, X } from '@lucide/vue'
import { closePalette, openPalette, paletteOpen } from '@/lib/palette'
import { searchDocs } from '@/lib/search'
import { sectionLabel } from '@/lib/content'
import type { SearchHit } from '@/lib/search'

const router = useRouter()
const query = ref('')
const active = ref(0)
const input = ref<HTMLInputElement | null>(null)
const hits = computed<SearchHit[]>(() => searchDocs(query.value))
const quickLinks = [
  { title: '当前重构进度', description: '规模、阶段、迁移状态与下一步', route: '/progress', section: 'progress' },
  { title: '能力矩阵', description: '查看实现情况、验收结果与使用限制', route: '/compatibility', section: 'compatibility' },
  { title: 'REST 浏览器', description: '搜索 242 个方法端点', route: '/rest-explorer', section: 'rest' },
  { title: 'REST 快速开始', description: '10 分钟跑通部署到历史', route: '/docs/getting-started/quickstart-rest', section: 'getting-started' },
  { title: '已知问题', description: '使用前必须确认的限制', route: '/docs/compatibility/known-issues', section: 'compatibility' },
]

const visible = computed(() => (query.value.trim() ? hits.value : quickLinks))

// go 跳转到搜索结果。
// @param route 目标路由
// @return 无
function go(route: string): void {
  closePalette()
  router.push(route)
}

// onKeydown 处理快捷键与方向键。
// @param event 键盘事件
// @return 无
function onKeydown(event: KeyboardEvent): void {
  if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
    event.preventDefault()
    paletteOpen.value ? closePalette() : openPalette()
    return
  }
  if (!paletteOpen.value) return
  if (event.key === 'Escape') closePalette()
  if (event.key === 'ArrowDown') {
    event.preventDefault()
    active.value = Math.min(active.value + 1, Math.max(visible.value.length - 1, 0))
  }
  if (event.key === 'ArrowUp') {
    event.preventDefault()
    active.value = Math.max(active.value - 1, 0)
  }
  if (event.key === 'Enter' && visible.value[active.value]) {
    event.preventDefault()
    go(visible.value[active.value].route)
  }
}

watch(paletteOpen, async (open) => {
  if (!open) return
  query.value = ''
  active.value = 0
  document.body.style.overflow = 'hidden'
  await nextTick()
  input.value?.focus()
})
watch(query, () => {
  active.value = 0
})
watch(paletteOpen, (open) => {
  if (!open) document.body.style.overflow = ''
})

onMounted(() => window.addEventListener('keydown', onKeydown))
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown)
  document.body.style.overflow = ''
})
</script>

<template>
  <Teleport to="body">
    <Transition name="palette">
      <div v-if="paletteOpen" class="palette-overlay" @click.self="closePalette">
        <div class="palette" role="dialog" aria-modal="true" aria-label="搜索文档">
          <div class="palette-input">
            <Search :size="18" />
            <input ref="input" v-model="query" type="text" placeholder="搜索文档、端点、术语…" aria-label="搜索关键词" />
            <button type="button" class="palette-close" @click="closePalette"><X :size="16" /></button>
          </div>
          <div class="palette-results">
            <div v-if="visible.length === 0" class="palette-empty">
              没有找到「{{ query }}」。试试迁移、mTLS、任务或覆盖率。
            </div>
            <button
              v-for="(item, index) in visible"
              :key="item.route"
              type="button"
              class="palette-item"
              :class="{ active: index === active }"
              @mouseenter="active = index"
              @click="go(item.route)"
            >
              <div class="palette-item__main">
                <div class="palette-item__title">{{ item.title }}</div>
                <div class="palette-item__desc">{{ 'description' in item ? item.description : '' }}</div>
              </div>
              <span class="palette-item__section">{{ sectionLabel(item.section) }}</span>
              <ArrowRight v-if="index === active" :size="15" />
            </button>
          </div>
          <div class="palette-footer">
            <span><kbd>↑</kbd><kbd>↓</kbd> 选择</span>
            <span><CornerDownLeft :size="12" /> 打开</span>
            <span><kbd>Esc</kbd> 关闭</span>
            <span class="palette-footer__brand">Flowtide 文档</span>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>
