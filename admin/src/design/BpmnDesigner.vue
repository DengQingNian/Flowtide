<script setup lang="ts">
// BPMN 设计器：bpmn-js Modeler + 属性面板 + 中文翻译，暴露保存/导入/缩放等命令。
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { translateModule } from './translate'
import type { ModelerWarningLike } from './validation'

const props = withDefaults(
  defineProps<{
    /** 初始 BPMN XML */
    xml?: string
    /** 用于新模型的流程 Key */
    modelKey?: string
    /** 是否只读 */
    readonly?: boolean
  }>(),
  { xml: '', modelKey: '', readonly: false },
)

const emit = defineEmits<{
  ready: []
  dirty: []
  error: [string]
  warnings: [ModelerWarningLike[]]
}>()

const canvasRef = ref<HTMLDivElement | null>(null)
const panelRef = ref<HTMLDivElement | null>(null)
const loading = ref(true)

/** 画布能力，宽松声明以隔离 bpmn-js 内部类型 */
interface CanvasLike {
  zoom?: (value?: number | string) => number
  scrollToElement?: (element: unknown) => void
}

/** 元素注册表能力 */
interface ElementRegistryLike {
  get: (id: string) => unknown
}

/** 选择能力 */
interface SelectionLike {
  select: (element: unknown) => void
}

/** 命令栈能力 */
interface CommandStackLike {
  undo?: () => void
  redo?: () => void
}

/** 设计器实例使用宽松接口，避免把 bpmn-js 内部类型泄漏到页面 */
interface ModelerLike {
  importXML: (xml: string) => Promise<{ warnings?: ModelerWarningLike[] }>
  saveXML: (options?: { format?: boolean }) => Promise<{ xml?: string }>
  on: (event: string, callback: () => void) => void
  get: (name: string) => unknown
  destroy: () => void
}

let modeler: ModelerLike | null = null
let lastImported = ''
/** 导入期间抑制 dirty 事件，避免初始加载被误判为未保存 */
let suppressDirty = false

// 读取画布能力对象。
// @return 画布对象或 null
function getCanvas(): CanvasLike | null {
  return (modeler?.get('canvas') as CanvasLike | undefined) ?? null
}

// 读取元素注册表能力对象。
// @return 注册表或 null
function getElementRegistry(): ElementRegistryLike | null {
  return (modeler?.get('elementRegistry') as ElementRegistryLike | undefined) ?? null
}

// 读取选择能力对象。
// @return 选择对象或 null
function getSelection(): SelectionLike | null {
  return (modeler?.get('selection') as SelectionLike | undefined) ?? null
}

// 读取命令栈能力对象。
// @return 命令栈或 null
function getCommandStack(): CommandStackLike | null {
  return (modeler?.get('commandStack') as CommandStackLike | undefined) ?? null
}

// 处理命令栈变化，导入过程不广播 dirty。
// @return 无
function handleCommandStackChanged(): void {
  if (!suppressDirty) emit('dirty')
}

// 创建设计器并导入初始 XML。
// @return 无
async function createModeler(): Promise<void> {
  if (!canvasRef.value || !panelRef.value) return
  loading.value = true
  try {
    const [{ default: Modeler }, { BpmnPropertiesPanelModule, BpmnPropertiesProviderModule }] = await Promise.all([
      import('bpmn-js/lib/Modeler'),
      import('bpmn-js-properties-panel'),
    ])
    if (!canvasRef.value || !panelRef.value) return
    modeler = new Modeler({
      container: canvasRef.value,
      propertiesPanel: { parent: panelRef.value },
      additionalModules: [translateModule, BpmnPropertiesPanelModule, BpmnPropertiesProviderModule],
    }) as unknown as ModelerLike
    if (props.xml) await importXml(props.xml)
    // 初次导入完成后再监听，避免加载即产生未保存状态
    modeler.on('commandStack.changed', handleCommandStackChanged)
    emit('ready')
  } catch (error) {
    emit('error', error instanceof Error ? error.message : String(error))
  } finally {
    loading.value = false
  }
}

// 导入 XML 并报告校验警告。
// @param xml BPMN XML
// @return 是否成功
async function importXml(xml: string): Promise<boolean> {
  if (!modeler) return false
  suppressDirty = true
  try {
    const result = await modeler.importXML(xml)
    lastImported = xml
    const warnings = (result.warnings ?? []).filter((item) => item.message || item.element?.id)
    if (warnings.length > 0) emit('warnings', warnings)
    getCanvas()?.zoom?.('fit-viewport')
    return true
  } catch (error) {
    emit('error', error instanceof Error ? error.message : String(error))
    return false
  } finally {
    suppressDirty = false
  }
}

// 导出当前画布 XML。
// @return XML 文本
async function getXml(): Promise<string> {
  if (!modeler) return ''
  const result = await modeler.saveXML({ format: true })
  return result.xml ?? ''
}

// 缩放到适应画布。
// @return 无
function fitViewport(): void {
  getCanvas()?.zoom?.('fit-viewport')
}

// 放大画布。
// @return 无
function zoomIn(): void {
  getCanvas()?.zoom?.(Math.min(4, (currentZoom() || 1) + 0.2))
}

// 缩小画布。
// @return 无
function zoomOut(): void {
  getCanvas()?.zoom?.(Math.max(0.2, (currentZoom() || 1) - 0.2))
}

// 读取当前缩放比例；bpmn-js 未暴露读取接口时退回 1。
// @return 缩放比例
function currentZoom(): number {
  try {
    return getCanvas()?.zoom?.() ?? 1
  } catch {
    return 1
  }
}

// 撤销上一步。
// @return 无
function undo(): void {
  getCommandStack()?.undo?.()
}

// 重做上一步。
// @return 无
function redo(): void {
  getCommandStack()?.redo?.()
}

// 选中并滚动到指定元素，用于校验问题定位。
// @param id BPMN 元素 ID
// @return 是否找到并选中元素
function selectElement(id: string): boolean {
  const element = getElementRegistry()?.get(id)
  if (!element) return false
  getSelection()?.select(element)
  getCanvas()?.scrollToElement?.(element)
  return true
}

watch(
  () => props.xml,
  (xml) => {
    if (modeler && xml && xml !== lastImported) void importXml(xml)
  },
)

onMounted(() => void createModeler())
onBeforeUnmount(() => {
  modeler?.destroy()
  modeler = null
})

defineExpose({ getXml, importXml, fitViewport, zoomIn, zoomOut, undo, redo, selectElement })
</script>

<template>
  <div class="bpmn-designer">
    <div ref="canvasRef" class="bpmn-designer__canvas" />
    <aside ref="panelRef" class="bpmn-designer__panel" aria-label="元素属性" />
    <div v-if="loading" class="bpmn-designer__loading">正在加载设计器…</div>
  </div>
</template>

<style>
@import 'bpmn-js/dist/assets/diagram-js.css';
@import 'bpmn-js/dist/assets/bpmn-js.css';
@import 'bpmn-js/dist/assets/bpmn-font/css/bpmn.css';
@import '@bpmn-io/properties-panel/dist/assets/properties-panel.css';
</style>

<style scoped>
.bpmn-designer { position: relative; display: flex; width: 100%; height: 100%; min-height: 520px; border: 1px solid var(--border); border-radius: var(--radius-sm); overflow: hidden; background: var(--surface); }
.bpmn-designer__canvas { flex: 1; min-width: 0; height: 100%; }
.bpmn-designer__panel { width: 320px; flex-shrink: 0; height: 100%; overflow-y: auto; border-left: 1px solid var(--border); background: var(--surface); }
.bpmn-designer__loading { position: absolute; inset: 0; display: flex; align-items: center; justify-content: center; background: var(--surface); color: var(--muted); }
:deep(.djs-container) { outline: none; }
:deep(.bio-properties-panel) { font-family: var(--font); font-size: 13px; }
</style>
