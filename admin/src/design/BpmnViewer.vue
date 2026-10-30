<script setup lang="ts">
// BPMN 只读查看器：懒加载 bpmn-js，按活动状态添加高亮标记。
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { ActivityHighlight } from './types'

const props = withDefaults(
  defineProps<{
    /** BPMN XML */
    xml: string
    /** 已完成活动 ID */
    completedIds?: string[]
    /** 当前活动 ID */
    activeIds?: string[]
    /** 异常活动 ID */
    failedIds?: string[]
    /** 画布高度 */
    height?: number
  }>(),
  { completedIds: () => [], activeIds: () => [], failedIds: () => [], height: 420 },
)

const emit = defineEmits<{ error: [string]; ready: [] }>()

const container = ref<HTMLDivElement | null>(null)
const loading = ref(false)

// 查看器实例与当前标记，使用宽松类型隔离 bpmn-js 的动态 API。
let viewer: {
  importXML: (xml: string) => Promise<unknown>
  getDefinitions: () => { diagrams?: unknown[] } | undefined
  get: (name: string) => {
    zoom: (value: string) => void
    addMarker: (id: string, className: string) => void
    removeMarker: (id: string, className: string) => void
  }
  destroy: () => void
} | null = null
let markers: Array<{ id: string; className: string }> = []
let renderToken = 0

const CLASS_BY_TYPE: Record<ActivityHighlight['type'], string> = {
  completed: 'highlight-completed',
  active: 'highlight-active',
  failed: 'highlight-failed',
}

// 清除已有标记。
// @return 无
function clearMarkers(): void {
  if (!viewer) return
  const canvas = viewer.get('canvas')
  for (const marker of markers) {
    try {
      canvas.removeMarker(marker.id, marker.className)
    } catch {
      // 元素可能已不在当前图中，忽略即可
    }
  }
  markers = []
}

// 应用活动高亮。
// @return 无
function applyMarkers(): void {
  if (!viewer) return
  clearMarkers()
  const canvas = viewer.get('canvas')
  const groups: Array<[string[], string]> = [
    [props.completedIds, CLASS_BY_TYPE.completed],
    [props.activeIds, CLASS_BY_TYPE.active],
    [props.failedIds, CLASS_BY_TYPE.failed],
  ]
  for (const [ids, className] of groups) {
    for (const id of ids) {
      if (!id) continue
      try {
        canvas.addMarker(id, className)
        markers.push({ id, className })
      } catch {
        // 活动 ID 与图形元素不匹配时跳过
      }
    }
  }
}

// 加载并渲染 BPMN。
// @return 无
async function render(): Promise<void> {
  if (!props.xml) return
  const token = ++renderToken
  loading.value = true
  try {
    if (!viewer) {
      const { default: NavigatedViewer } = await import('bpmn-js/lib/NavigatedViewer')
      if (!container.value) return
      viewer = new NavigatedViewer({ container: container.value }) as unknown as typeof viewer
    }
    await viewer!.importXML(props.xml)
    if (token !== renderToken) return
    const definitions = viewer!.getDefinitions()
    const hasDiagram = Array.isArray(definitions?.diagrams) && definitions.diagrams.length > 0
    if (!hasDiagram) {
      emit('error', '缺少 BPMN DI 图形信息')
      return
    }
    viewer!.get('canvas').zoom('fit-viewport')
    applyMarkers()
    emit('ready')
  } catch (error) {
    if (token !== renderToken) return
    emit('error', error instanceof Error ? error.message : String(error))
  } finally {
    if (token === renderToken) loading.value = false
  }
}

watch(() => props.xml, () => void render())
watch(() => [props.completedIds, props.activeIds, props.failedIds], applyMarkers, { deep: true })

onMounted(() => void render())
onBeforeUnmount(() => {
  renderToken += 1
  if (viewer) {
    viewer.destroy()
    viewer = null
  }
})
</script>

<template>
  <div class="bpmn-viewer" :style="{ height: `${height}px` }">
    <div ref="container" class="bpmn-viewer__canvas" />
    <div v-if="loading" class="bpmn-viewer__loading">正在渲染流程图…</div>
  </div>
</template>

<style>
@import 'bpmn-js/dist/assets/diagram-js.css';
@import 'bpmn-js/dist/assets/bpmn-js.css';
@import 'bpmn-js/dist/assets/bpmn-font/css/bpmn.css';
</style>

<style scoped>
.bpmn-viewer { position: relative; width: 100%; background: var(--surface); }
.bpmn-viewer__canvas { width: 100%; height: 100%; }
.bpmn-viewer__loading { position: absolute; inset: 0; display: flex; align-items: center; justify-content: center; color: var(--muted); background: var(--surface); }

:deep(.djs-element.highlight-completed .djs-visual > *) { fill: #e3e1db !important; stroke: #78746c !important; }
:deep(.djs-element.highlight-active .djs-visual > *) { fill: #edf3f8 !important; stroke: #3675a9 !important; stroke-width: 2px !important; }
:deep(.djs-element.highlight-failed .djs-visual > *) { fill: #faecea !important; stroke: #a44841 !important; }
:deep(.djs-container) { outline: none; }
</style>
