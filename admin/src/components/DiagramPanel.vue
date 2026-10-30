<script setup lang="ts">
// 只读 BPMN 图：懒加载查看器，按活动状态高亮并给出图例。
import { computed, defineAsyncComponent, ref } from 'vue'
import { NSpin } from 'naive-ui'
import type { ActivityHighlight } from '@/design/types'

const BpmnViewer = defineAsyncComponent(() => import('@/design/BpmnViewer.vue'))

const props = withDefaults(
  defineProps<{
    /** BPMN XML */
    xml?: string | null
    /** 加载态 */
    loading?: boolean
    /** 活动高亮 */
    highlights?: ActivityHighlight[]
    /** 图形高度 */
    height?: number
  }>(),
  { xml: '', loading: false, highlights: () => [], height: 420 },
)

/** 已完成活动 */
const completedIds = computed(() => props.highlights.filter((item) => item.type === 'completed').map((item) => item.id))
/** 当前活动 */
const activeIds = computed(() => props.highlights.filter((item) => item.type === 'active').map((item) => item.id))
/** 异常活动 */
const failedIds = computed(() => props.highlights.filter((item) => item.type === 'failed').map((item) => item.id))

const error = ref('')

// 接收查看器导入错误。
// @param message 错误信息
// @return 无
function onError(message: string): void {
  error.value = message
}

/** 是否缺少图形信息 */
const missing = computed(() => Boolean(error.value))

</script>

<template>
  <div class="diagram-panel">
    <div v-if="loading" class="diagram-panel__placeholder">
      <n-spin size="small" />
    </div>
    <div v-else-if="!xml" class="diagram-panel__placeholder">该流程定义缺少 BPMN XML，无法渲染图形。</div>
    <div v-else-if="missing" class="diagram-panel__placeholder">
      该定义缺少 BPMN DI 图形信息，或 XML 无法解析：{{ error }}
    </div>
    <template v-else>
      <BpmnViewer
        :xml="xml"
        :completed-ids="completedIds"
        :active-ids="activeIds"
        :failed-ids="failedIds"
        :height="height"
        @error="onError"
      />
      <div class="diagram-legend">
        <span>图例：</span>
        <span>灰色＝已完成</span>
        <span>蓝色＝当前</span>
        <span>红色＝异常</span>
      </div>
    </template>
  </div>
</template>