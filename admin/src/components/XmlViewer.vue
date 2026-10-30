<script setup lang="ts">
// XML 查看器：等宽只读展示、复制与下载，不执行任何 XML 内容。
import { computed } from 'vue'
import { Download } from '@lucide/vue'
import { downloadText } from '@/lib/download'
import CopyText from './CopyText.vue'

const props = withDefaults(
  defineProps<{
    /** XML 文本 */
    xml: string
    /** 下载文件名 */
    filename?: string
    /** 最大高度 */
    maxHeight?: number
  }>(),
  { filename: 'source.bpmn20.xml', maxHeight: 520 },
)

/** 行数统计 */
const lineCount = computed(() => (props.xml ? props.xml.split('\n').length : 0))

// 下载 XML 文件。
// @return 无
function download(): void {
  downloadText(props.filename, props.xml, 'application/xml;charset=utf-8')
}
</script>

<template>
  <div class="xml-viewer">
    <div class="xml-viewer__toolbar">
      <span class="muted" style="margin-right: auto; font-size: 12px">{{ lineCount }} 行</span>
      <CopyText :value="xml" display="复制" />
      <button class="icon-button" type="button" title="下载 XML" aria-label="下载 XML" @click="download">
        <Download :size="14" />
      </button>
    </div>
    <pre class="code-block" :style="{ maxHeight: `${maxHeight}px` }">{{ xml }}</pre>
  </div>
</template>
