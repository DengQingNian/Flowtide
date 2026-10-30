<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { ArrowRight, BookOpen, Compass, FileText, Gauge, Search, ShieldCheck } from '@lucide/vue'
import { content, pages, pagesInSection, sectionMeta } from '@/lib/content'
import StatusBadge from '@/components/StatusBadge.vue'
import { openPalette } from '@/lib/palette'

const router = useRouter()
const total = computed(() => pages.length)
const sections = computed(() =>
  sectionMeta.map((section) => ({
    ...section,
    pages: pagesInSection(section.id),
  })),
)

const paths = [
  {
    icon: Compass,
    title: '第一次使用',
    description: '启动服务，部署流程，完成任务并查询历史。',
    route: '/docs/getting-started/quickstart-rest',
  },
  {
    icon: Gauge,
    title: '判断当前进度',
    description: '查看各阶段的实现情况和待验收工作。',
    route: '/progress',
  },
  {
    icon: ShieldCheck,
    title: '核对能力边界',
    description: '查看单测、数据库和 Java 对照的验收结果。',
    route: '/compatibility',
  },
  {
    icon: FileText,
    title: '评估 Java 迁移',
    description: '了解迁移工具的用法和切换前的准备。',
    route: '/docs/migration',
  },
]

// goSearch 打开搜索面板。
// @return 无
function goSearch(): void {
  openPalette()
}
</script>

<template>
  <div class="docs-index">
    <header class="index-hero">
      <div class="index-hero__badge"><BookOpen :size="14" /> Flowtide 文档</div>
      <h1>文档中心</h1>
      <p>共 {{ total }} 篇文档，包含安装示例、Go SDK、REST API 和迁移说明。第一次使用可以从快速开始读起，评估接入前请先查阅兼容性与已知问题。</p>
      <button type="button" class="index-search" @click="goSearch">
        <Search :size="17" />
        <span>搜索文档、端点与术语</span>
        <kbd>Ctrl K</kbd>
      </button>
      <div class="index-stats">
        <span><strong>{{ total }}</strong> 篇文档</span>
        <span><strong>{{ sectionMeta.length }}</strong> 个分区</span>
        <span>数据截至 <code>{{ content.commit }}</code></span>
      </div>
    </header>

    <section class="index-paths" aria-label="推荐路径">
      <button v-for="path in paths" :key="path.route" type="button" class="path-card group" @click="router.push(path.route)">
        <span class="block-handle" aria-hidden="true">⋮⋮</span>
        <component :is="path.icon" :size="20" />
        <span class="path-card__title">{{ path.title }}</span>
        <span class="path-card__desc">{{ path.description }}</span>
        <ArrowRight :size="16" class="path-card__arrow" />
      </button>
    </section>

    <div class="index-sections">
      <section v-for="section in sections" :id="section.id" :key="section.id" class="index-section">
        <div class="index-section__head">
          <div>
            <h2>{{ section.label }}</h2>
            <p>{{ section.description }}</p>
          </div>
          <span>{{ section.pages.length }} 篇</span>
        </div>
        <div class="index-links">
          <router-link v-for="page in section.pages" :key="page.route" :to="page.route" class="index-link">
            <span class="index-link__title">{{ page.title }}</span>
            <span class="index-link__desc">{{ page.description }}</span>
            <StatusBadge :status="page.status" compact />
          </router-link>
        </div>
      </section>
    </div>
  </div>
</template>
