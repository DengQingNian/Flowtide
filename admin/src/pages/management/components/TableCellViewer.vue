<script setup lang="ts">
// 单元格查看器：展示完整值、二进制提示与复制入口。
import { computed } from 'vue'
import { Binary, X } from '@lucide/vue'
import JsonViewer from '@/components/JsonViewer.vue'
import CopyText from '@/components/CopyText.vue'
import { isBinaryColumn } from './managementUtils'
import { formatJson, formatValue } from '@/lib/format'

const props = defineProps<{
  /** 列名 */
  column: string
  /** 列类型 */
  type?: string
  /** 单元格原始值 */
  value: unknown
}>()

const emit = defineEmits<{ close: [] }>()

/** 是否二进制列 */
const binary = computed(() => isBinaryColumn(props.type))
/** 是否对象值 */
const complex = computed(() => props.value !== null && typeof props.value === 'object')
/** 展示文本 */
const text = computed(() => {
  if (props.value === null || props.value === undefined) return '—'
  if (complex.value) return formatJson(props.value)
  return formatValue(props.value, 20000)
})
</script>

<template>
  <section class="panel">
    <div class="panel__header">
      <span class="panel__title">单元格 {{ column }}</span>
      <span v-if="type" class="muted mono">{{ type }}</span>
      <span class="row" style="margin-left: auto">
        <CopyText :value="text" display="复制值" />
        <button class="icon-button" type="button" title="关闭单元格查看" @click="emit('close')">
          <X :size="14" />
        </button>
      </span>
    </div>
    <div class="panel__body stack" style="gap: 10px">
      <div v-if="binary" class="banner banner--warning" role="note">
        <Binary :size="16" aria-hidden="true" />
        <div class="grow">二进制列不可直接预览，以下为服务端返回的 Base64/字节文本。</div>
      </div>
      <JsonViewer v-if="complex && !binary" :value="value" />
      <pre v-else class="code-block cell-viewer__value">{{ text }}</pre>
    </div>
  </section>
</template>

<style scoped>
.cell-viewer__value { white-space: pre-wrap; overflow-wrap: anywhere; }
</style>
