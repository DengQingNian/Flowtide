<script setup lang="ts">
// 危险操作按钮：二次确认、加载态与禁用态。
import { NButton, NPopconfirm } from 'naive-ui'

withDefaults(
  defineProps<{
    /** 按钮文本 */
    label: string
    /** 确认框标题 */
    title?: string
    /** 影响范围说明 */
    description?: string
    /** 是否危险操作 */
    danger?: boolean
    /** 是否加载中 */
    loading?: boolean
    /** 是否禁用 */
    disabled?: boolean
    /** 按钮尺寸 */
    size?: 'tiny' | 'small' | 'medium' | 'large'
  }>(),
  { title: '确认执行该操作？', description: '', danger: true, loading: false, disabled: false, size: 'small' },
)

const emit = defineEmits<{ confirm: [] }>()
</script>

<template>
  <n-popconfirm @positive-click="emit('confirm')">
    <template #trigger>
      <n-button
        size="small"
        :type="danger ? 'error' : 'default'"
        :secondary="danger"
        :loading="loading"
        :disabled="disabled"
      >
        {{ label }}
      </n-button>
    </template>
    <div>{{ title }}</div>
    <div v-if="description" class="muted" style="margin-top: 4px; max-width: 280px">{{ description }}</div>
  </n-popconfirm>
</template>
