<script setup lang="ts">
// 页面标题组件：标题、描述与右侧操作区。
defineProps<{
  /** 页面标题 */
  title: string
  /** 页面说明 */
  description?: string
}>()
</script>

<template>
  <header class="page-header">
    <div class="page-header__main">
      <h1 class="page-header__title">{{ title }}</h1>
      <p v-if="description" class="page-header__description">{{ description }}</p>
    </div>
    <div v-if="$slots.actions" class="page-header__actions">
      <slot name="actions" />
    </div>
  </header>
</template>
