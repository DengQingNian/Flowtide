<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{ status: 'verified' | 'prototype' | 'planned' | string; compact?: boolean }>()
const label = computed(
  () => ({ verified: '已验证', prototype: '原型可用', planned: '规划中' })[props.status] ?? props.status,
)
</script>

<template>
  <span class="status-badge" :class="[`status-badge--${status}`, { 'status-badge--compact': compact }]">
    <span class="status-badge__dot" />
    {{ label }}
  </span>
</template>
