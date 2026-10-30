<script setup lang="ts">
// 筛选栏：统一条件布局、查询与重置入口。
import { NButton } from 'naive-ui'

withDefaults(
  defineProps<{
    /** 是否展示重置按钮 */
    showReset?: boolean
    /** 查询按钮加载态 */
    loading?: boolean
  }>(),
  { showReset: true, loading: false },
)

const emit = defineEmits<{ search: []; reset: [] }>()
</script>

<template>
  <div class="filter-bar">
    <slot />
    <div class="row" style="margin-left: auto">
      <slot name="actions" />
      <n-button size="small" type="primary" :loading="loading" @click="emit('search')">查询</n-button>
      <n-button v-if="showReset" size="small" quaternary @click="emit('reset')">重置</n-button>
    </div>
  </div>
</template>
