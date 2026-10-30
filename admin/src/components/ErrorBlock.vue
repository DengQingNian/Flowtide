<script setup lang="ts">
// 错误块：展示服务端原文、可复制的诊断信息与重试入口。
import { computed } from 'vue'
import { AlertTriangle, RefreshCw } from '@lucide/vue'
import { ApiError, errorMessage } from '@/api/client'
import CopyText from './CopyText.vue'

const props = defineProps<{
  /** 错误对象 */
  error: unknown
  /** 额外上下文，如请求路径 */
  context?: string
}>()

const emit = defineEmits<{ retry: [] }>()

/** 可读信息 */
const message = computed(() => errorMessage(props.error))
/** 业务错误码 */
const code = computed(() => (props.error instanceof ApiError ? props.error.code : ''))
/** 异常详情 */
const exception = computed(() => (props.error instanceof ApiError ? props.error.exception : ''))
/** 诊断文本 */
const diagnostic = computed(() => {
  const parts = [props.context, code.value, message.value, exception.value].filter(Boolean)
  return parts.join('\n')
})
</script>

<template>
  <div class="banner banner--error" role="alert" style="margin: 0 0 12px">
    <AlertTriangle :size="16" aria-hidden="true" />
    <div class="grow">
      <div>{{ message }}</div>
      <div v-if="code" class="mono muted">错误码：{{ code }}</div>
      <details v-if="exception">
        <summary class="muted">异常详情</summary>
        <pre class="code-block">{{ exception }}</pre>
      </details>
    </div>
    <span class="banner__actions">
      <CopyText v-if="diagnostic" :value="diagnostic" display="复制诊断" />
      <button class="icon-button" type="button" title="重试" @click="emit('retry')">
        <RefreshCw :size="14" />
      </button>
    </span>
  </div>
</template>
