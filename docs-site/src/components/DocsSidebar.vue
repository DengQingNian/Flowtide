<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ChevronDown, FileText } from '@lucide/vue'
import { buildSidebar, findGroupForRoute } from '@/lib/navigation'

const emit = defineEmits<{ navigate: [] }>()
defineProps<{ mobile?: boolean }>()
const route = useRoute()
const groups = buildSidebar()
const activeGroup = computed(() => findGroupForRoute(route.path))
const open = ref<Set<string>>(new Set())

// syncOpen 确保当前分组展开。
// @return 无
function syncOpen(): void {
  const next = new Set(open.value)
  next.add(activeGroup.value)
  next.add('getting-started')
  if (open.value.size === 0) next.add('progress')
  open.value = next
}

// toggle 展开或收起分组。
// @param id 分组 ID
// @return 无
function toggle(id: string): void {
  const next = new Set(open.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  open.value = next
}

// isOpen 判断分组是否展开。
// @param id 分组 ID
// @return 是否展开
function isOpen(id: string): boolean {
  return open.value.has(id) || id === activeGroup.value
}

watch(() => route.path, syncOpen, { immediate: true })
</script>

<template>
  <nav class="sidebar" :class="{ 'sidebar--mobile': mobile }" aria-label="文档导航">
    <router-link to="/docs" class="sidebar-home" :class="{ active: route.path === '/docs' }" @click="emit('navigate')">
      <span class="sidebar-home__icon"><FileText :size="15" /></span>
      <span>文档首页</span>
    </router-link>

    <div v-for="group in groups" :key="group.id" class="sidebar-group" :class="{ 'is-active': group.id === activeGroup }">
      <button class="sidebar-group__head" type="button" :aria-expanded="isOpen(group.id)" @click="toggle(group.id)">
        <span>{{ group.label }}</span>
        <span class="sidebar-group__meta">{{ group.entries.length }}</span>
        <ChevronDown :size="14" :class="{ rotated: !isOpen(group.id) }" />
      </button>
      <div v-show="isOpen(group.id)" class="sidebar-links">
        <router-link
          v-for="entry in group.entries"
          :key="entry.route"
          :to="entry.route"
          class="sidebar-link"
          :class="{ active: route.path === entry.route, pinned: entry.pinned }"
          @click="emit('navigate')"
        >
          <span class="sidebar-link__text">{{ entry.title }}</span>
          <span v-if="entry.status === 'planned'" class="mini-dot mini-dot--planned" title="规划中" />
          <span v-else-if="entry.status === 'prototype'" class="mini-dot mini-dot--prototype" title="原型可用" />
        </router-link>
      </div>
    </div>
  </nav>
</template>
