<script setup lang="ts">
import { computed, nextTick, onMounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useMessage } from 'naive-ui'
import { ArrowLeft, ArrowRight, CalendarClock, ExternalLink, Tag } from '@lucide/vue'
import { neighbors, pageByRoute, sectionLabel } from '@/lib/content'
import StatusBadge from '@/components/StatusBadge.vue'
import NotFoundPage from './NotFoundPage.vue'

const route = useRoute()
const message = useMessage()
const page = computed(() => pageByRoute.get(route.path))
const nav = computed(() => (page.value ? neighbors(page.value.route) : {}))

// copyFromButton 复制代码块文本并给出反馈。
// @param button 复制按钮
// @return 无
async function copyFromButton(button: HTMLElement): Promise<void> {
  const block = button.closest('.code-block')
  const code = block?.querySelector('code')
  if (!code?.textContent) return
  try {
    await navigator.clipboard.writeText(code.textContent)
    const previous = button.textContent
    button.textContent = '已复制'
    message.success('代码已复制')
    window.setTimeout(() => {
      button.textContent = previous || '复制'
    }, 1600)
  } catch {
    message.error('浏览器未允许剪贴板访问')
  }
}

// onClick 处理代码复制按钮点击。
// @param event 鼠标事件
// @return 无
function onClick(event: MouseEvent): void {
  const target = event.target as HTMLElement
  const button = target.closest('.copy-btn') as HTMLElement | null
  if (button) void copyFromButton(button)
}

// scrollToHash 在页面加载后滚动到锚点。
// @return 无
function scrollToHash(): void {
  if (!window.location.hash) return
  const element = document.getElementById(window.location.hash.slice(1))
  if (element) {
    window.setTimeout(() => element.scrollIntoView({ behavior: 'smooth', block: 'start' }), 80)
  }
}

watch(
  () => route.path,
  async () => {
    await nextTick()
    scrollToHash()
  },
)

onMounted(scrollToHash)
</script>

<template>
  <NotFoundPage v-if="!page" />
  <article v-else class="doc-page" @click="onClick">
    <header class="doc-header">
      <div class="doc-eyebrow">
        <router-link to="/docs">{{ sectionLabel(page.section) }}</router-link>
        <span class="eyebrow-sep">/</span>
        <span>{{ page.readingMinutes }} 分钟阅读</span>
      </div>
      <div class="doc-title-row">
        <h1>{{ page.title }}</h1>
        <StatusBadge :status="page.status" />
      </div>
      <p class="doc-description">{{ page.description }}</p>
      <div class="doc-meta">
        <span><CalendarClock :size="14" /> 基于提交 <code>{{ page.verifiedCommit }}</code></span>
        <span v-if="page.tags.length"><Tag :size="14" /> {{ page.tags.join(' · ') }}</span>
        <a :href="`https://github.com/DengQingNian/Flowtide/src/branch/master/${page.sourcePath}`" target="_blank" rel="noopener">
          <ExternalLink :size="14" /> 在仓库查看
        </a>
      </div>
    </header>

    <div class="markdown-body" v-html="page.html" />

    <nav class="page-nav" aria-label="上一篇与下一篇">
      <router-link v-if="nav.prev" :to="nav.prev.route" class="page-nav__item page-nav__item--prev">
        <span class="page-nav__label"><ArrowLeft :size="14" /> 上一篇</span>
        <span class="page-nav__title">{{ nav.prev.title }}</span>
      </router-link>
      <span v-else />
      <router-link v-if="nav.next" :to="nav.next.route" class="page-nav__item page-nav__item--next">
        <span class="page-nav__label">下一篇 <ArrowRight :size="14" /></span>
        <span class="page-nav__title">{{ nav.next.title }}</span>
      </router-link>
    </nav>

    <div class="doc-feedback">
      <span>本页内容基于提交 <code>{{ page.verifiedCommit }}</code>，能力声明以能力矩阵为准。</span>
      <router-link to="/compatibility">核对能力矩阵 →</router-link>
    </div>
  </article>
</template>
