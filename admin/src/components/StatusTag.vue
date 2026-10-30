<script setup lang="ts">
// 状态标签：把引擎状态映射为低饱和底色，支持显式 tone。
import { computed } from 'vue'

/** 标签色调 */
export type StatusTone = 'auto' | 'default' | 'success' | 'danger' | 'warning' | 'info'

const props = withDefaults(
  defineProps<{
    /** 状态值 */
    value?: string | number | boolean | null
    /** 显示文本，默认使用 value */
    label?: string
    /** 色调，auto 时按状态值推断 */
    tone?: StatusTone
  }>(),
  { value: '', tone: 'auto' },
)

const SUCCESS = ['active', 'completed', 'complete', 'running', 'success', 'ok', 'resolved', 'deployed', 'finished', 'true', '已完成', '运行中', '激活', '成功']
const DANGER = ['failed', 'failure', 'deadletter', 'dead', 'suspended', 'error', 'deleted', 'invalid', 'false', '失败', '挂起', '已删除', '死信']
const WARNING = ['pending', 'created', 'scheduled', 'timer', 'waiting', 'locked', 'draft', 'pending', '待办', '待处理', '已创建']
const INFO = ['claimed', 'assigned', 'inprogress', 'in_progress', 'processing', '已签收', '处理中']

/** 显示文本 */
const text = computed(() => (props.label ?? (props.value === null || props.value === undefined || props.value === '' ? '—' : String(props.value))))

/** 推断色调 */
const resolvedTone = computed<StatusTone>(() => {
  if (props.tone !== 'auto') return props.tone
  const lower = text.value.toLowerCase()
  if (SUCCESS.includes(lower)) return 'success'
  if (DANGER.includes(lower)) return 'danger'
  if (WARNING.includes(lower)) return 'warning'
  if (INFO.includes(lower)) return 'info'
  return 'default'
})
</script>

<template>
  <span class="status-tag" :class="`status-tag--${resolvedTone}`">{{ text }}</span>
</template>
