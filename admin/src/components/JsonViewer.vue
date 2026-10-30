<script setup lang="ts">
// JSON 查看器：格式化、复制与大文本折叠。
import { computed, ref } from 'vue'
import { ChevronDown, ChevronRight } from '@lucide/vue'
import { formatJson } from '@/lib/format'
import CopyText from './CopyText.vue'

const props = withDefaults(
  defineProps<{
    /** 任意 JSON 值 */
    value: unknown
    /** 展示标题 */
    title?: string
    /** 超过该字节数默认折叠 */
    collapseThreshold?: number
  }>(),
  { title: '', collapseThreshold: 65536 },
)

const expanded = ref(false)

/** 格式化文本 */
const text = computed(() => formatJson(props.value))
/** 是否默认折叠 */
const collapsible = computed(() => text.value.length > props.collapseThreshold)
/** 实际展示文本 */
const visibleText = computed(() => (collapsible.value && !expanded.value ? `${text.value.slice(0, 2000)}\n…（已折叠，共 ${text.value.length} 字符）` : text.value))
</script>

<template>
  <div>
    <div class="row row--between" style="margin-bottom: 6px">
      <span v-if="title" class="section-title" style="margin: 0">{{ title }}</span>
      <span class="row" style="margin-left: auto">
        <button v-if="collapsible" class="icon-button" type="button" :title="expanded ? '折叠' : '展开'" @click="expanded = !expanded">
          <ChevronDown v-if="expanded" :size="14" />
          <ChevronRight v-else :size="14" />
        </button>
        <CopyText :value="text" display="复制" />
      </span>
    </div>
    <pre class="code-block">{{ visibleText }}</pre>
  </div>
</template>
