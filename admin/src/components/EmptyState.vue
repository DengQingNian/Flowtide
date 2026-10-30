<script setup lang="ts">
// 空态：一行说明 + 一个主操作。
import { Inbox } from '@lucide/vue'

withDefaults(
  defineProps<{
    /** 空态标题 */
    title?: string
    /** 补充说明 */
    description?: string
  }>(),
  { title: '暂无数据', description: '' },
)
</script>

<template>
  <div class="empty-state">
    <Inbox :size="22" aria-hidden="true" />
    <div class="empty-state__title">{{ title }}</div>
    <div v-if="description">{{ description }}</div>
    <slot />
  </div>
</template>
