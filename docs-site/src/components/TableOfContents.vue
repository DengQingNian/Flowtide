<script setup lang="ts">
import { computed } from 'vue'
import type { TocItem } from '@/lib/content'

const props = defineProps<{ headings: TocItem[]; active?: string }>()
const emit = defineEmits<{ navigate: [] }>()
const items = computed(() => props.headings.filter((heading) => heading.level === 2 || heading.level === 3))

// scrollTo 平滑滚动到标题。
// @param id 标题 ID
// @return 无
function scrollTo(id: string): void {
  const element = document.getElementById(id)
  if (!element) return
  const top = element.getBoundingClientRect().top + window.scrollY - 88
  window.scrollTo({ top, behavior: 'smooth' })
  history.replaceState(null, '', `#${id}`)
  emit('navigate')
}

// scrollTop 回到页面顶部。
// @return 无
function scrollTop(): void {
  window.scrollTo({ top: 0, behavior: 'smooth' })
}
</script>

<template>
  <div v-if="items.length" class="toc">
    <div class="toc__title">本页目录</div>
    <ul class="toc__list">
      <li v-for="heading in items" :key="heading.id" :class="['toc__item', `toc__item--h${heading.level}`, { active: active === heading.id }]">
        <a :href="`#${heading.id}`" @click.prevent="scrollTo(heading.id)">{{ heading.text }}</a>
      </li>
    </ul>
    <button class="toc__top" type="button" @click="scrollTop">回到顶部</button>
  </div>
</template>
