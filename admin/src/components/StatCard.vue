<script setup lang="ts">
// 指标卡片：数字可点击跳转到对应列表。
import { RouterLink } from 'vue-router'

withDefaults(
  defineProps<{
    /** 指标名称 */
    label: string
    /** 指标值 */
    value: string | number
    /** 补充说明 */
    hint?: string
    /** 跳转路径 */
    to?: string
  }>(),
  { hint: '', to: '' },
)
</script>

<template>
  <component :is="to ? RouterLink : 'div'" :to="to || undefined" class="stat-card">
    <span class="stat-card__label">{{ label }}</span>
    <span class="stat-card__value">{{ value }}</span>
    <span v-if="hint" class="stat-card__hint">{{ hint }}</span>
  </component>
</template>
