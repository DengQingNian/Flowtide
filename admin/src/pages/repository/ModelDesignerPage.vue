<script setup lang="ts">
// BPMN 设计器页面：加载模型与源码，提供保存、部署、导入导出、校验与离开提示。
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { onBeforeRouteLeave, useRoute } from 'vue-router'
import { NSpin } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import ErrorBlock from '@/components/ErrorBlock.vue'
import BpmnDesigner from '@/design/BpmnDesigner.vue'
import ModelDesignerToolbar from './components/model/ModelDesignerToolbar.vue'
import ValidationList from './components/model/ValidationList.vue'
import {
  deployModel,
  getModel,
  getModelSource,
  listProcessDefinitionsByKey,
  pickLatestDefinition,
  saveModelSource,
  sourceFileName,
  type Model,
  type ModelDeployment,
  type ProcessDefinitionSummary,
} from '@/api/models'
import { ApiError } from '@/api/client'
import { formatBytes, formatDateTime } from '@/lib/format'
import { createBeforeUnloadGuard, exportBpmnXml, readFileText } from './components/model/designerFiles'
import { buildMinimalBpmn } from '@/design/minimalBpmn'
import {
  isFileTooLarge,
  issuesFromModelerWarnings,
  validateBpmnXml,
  type ModelerWarningLike,
  type ValidationIssue,
} from '@/design/validation'
import { markAuthError } from '@/stores/ui'

const route = useRoute()
/** 当前模型 ID */
const modelId = computed(() => String(route.params.id ?? ''))

/** 模型元信息 */
const model = ref<Model | null>(null)
/** 画布初始 XML，只在加载时赋值，保存后不重新导入以保留撤销栈 */
const canvasXml = ref('')
/** 最后一次成功保存的 XML，用于导入失败回退 */
const lastSavedXml = ref('')
/** 页面加载中 */
const loading = ref(true)
/** 加载错误 */
const loadError = ref<unknown>(null)
/** 操作错误 */
const actionError = ref<unknown>(null)
/** 操作提示 */
const notice = ref('')
/** 是否有未保存修改 */
const dirty = ref(false)
/** 保存中 */
const saving = ref(false)
/** 部署中 */
const deploying = ref(false)
/** 校验错误与警告 */
const validationIssues = ref<ValidationIssue[]>([])
/** 导入产生的警告 */
const importWarnings = ref<ValidationIssue[]>([])
/** 最近一次部署结果 */
const deployment = ref<ModelDeployment | null>(null)
/** 部署生成的最新流程定义 */
const deployedDefinition = ref<ProcessDefinitionSummary | null>(null)
/** 模型暂无源码 */
const sourceMissing = ref(false)
/** 本地导入文件输入 */
const fileInput = ref<HTMLInputElement | null>(null)
/** 设计器暴露的命令 */
interface DesignerHandle {
  getXml: () => Promise<string>
  importXml: (xml: string) => Promise<boolean>
  fitViewport: () => void
  zoomIn: () => void
  zoomOut: () => void
  undo: () => void
  redo: () => void
  selectElement: (id: string) => boolean
}

/** 设计器实例 */
const designerRef = ref<DesignerHandle | null>(null)

/** 合并后的校验问题 */
const allIssues = computed<ValidationIssue[]>(() => [...importWarnings.value, ...validationIssues.value])

/** 页面标题 */
const pageTitle = computed(() => (model.value ? `设计：${model.value.name || model.value.key}` : 'BPMN 设计器'))
/** 页面说明 */
const pageDescription = computed(() => {
  if (!model.value) return '在线建模、属性编辑、校验、保存与部署。'
  return `Key：${model.value.key || '—'} · 版本 v${model.value.version} · 最后更新 ${formatDateTime(model.value.lastUpdateTime)}`
})

// 读取当前画布 XML；设计器未就绪时退回最后保存的内容。
// @return XML 文本
async function currentXml(): Promise<string> {
  if (!designerRef.value) return lastSavedXml.value
  try {
    return await designerRef.value.getXml()
  } catch (readError) {
    markAuthError(readError)
    actionError.value = readError
    return ''
  }
}

// 加载模型元信息与源码；源码缺失时载入最小 BPMN 模板。
// @return 无
async function load(): Promise<void> {
  loading.value = true
  loadError.value = null
  actionError.value = null
  notice.value = ''
  deployment.value = null
  deployedDefinition.value = null
  try {
    const loaded = await getModel(modelId.value)
    let source = ''
    try {
      source = await getModelSource(loaded.id)
    } catch (sourceError) {
      // 模型存在但没有源码时服务端返回 404，视为空源码
      if (!(sourceError instanceof ApiError && sourceError.status === 404)) throw sourceError
    }
    model.value = loaded
    sourceMissing.value = !source.trim()
    if (sourceMissing.value) source = buildMinimalBpmn(loaded.key, loaded.name)
    canvasXml.value = source
    lastSavedXml.value = source
    dirty.value = false
    validationIssues.value = []
    importWarnings.value = []
  } catch (error) {
    markAuthError(error)
    loadError.value = error
  } finally {
    loading.value = false
  }
}

// 标记画布存在未保存修改。
// @return 无
function markDirty(): void {
  dirty.value = true
}

// 保存画布；校验不通过时阻断保存。
// @return 是否保存成功
async function save(): Promise<boolean> {
  if (!model.value) return false
  const xml = await currentXml()
  if (!xml.trim()) {
    actionError.value = new Error('无法读取画布 XML，未执行保存。')
    return false
  }
  const result = validateBpmnXml(xml, model.value.key)
  validationIssues.value = result.issues
  if (!result.valid) {
    notice.value = `校验发现 ${result.errors.length} 个错误，已取消保存。`
    return false
  }
  saving.value = true
  actionError.value = null
  try {
    await saveModelSource(model.value.id, xml, sourceFileName(model.value))
    lastSavedXml.value = xml
    dirty.value = false
    sourceMissing.value = false
    notice.value = '已保存模型源码。'
    const refreshed = await getModel(model.value.id).catch(() => null)
    if (refreshed) model.value = refreshed
    return true
  } catch (error) {
    markAuthError(error)
    actionError.value = error
    return false
  } finally {
    saving.value = false
  }
}

// 部署模型；存在未保存修改时先保存，保存失败则中止部署。
// @return 无
async function deploy(): Promise<void> {
  if (!model.value) return
  if (dirty.value) {
    const saved = await save()
    if (!saved) return
  }
  deploying.value = true
  actionError.value = null
  deployment.value = null
  deployedDefinition.value = null
  try {
    const result = await deployModel(model.value.id)
    deployment.value = result
    try {
      const definitions = await listProcessDefinitionsByKey(model.value.key)
      deployedDefinition.value = pickLatestDefinition(definitions.data)
    } catch {
      // 定义查询失败不影响部署结果展示
      deployedDefinition.value = null
    }
    notice.value = `部署成功：${result.name || result.id}`
    const refreshed = await getModel(model.value.id).catch(() => null)
    if (refreshed) model.value = refreshed
  } catch (error) {
    markAuthError(error)
    actionError.value = error
  } finally {
    deploying.value = false
  }
}

// 打开本地文件选择框。
// @return 无
function chooseFile(): void {
  fileInput.value?.click()
}

// 导入本地 BPMN；失败时回退到导入前画布，不破坏当前内容。
// @param event 文件输入事件
// @return 无
async function onFileChange(event: Event): Promise<void> {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file || !designerRef.value) return
  if (isFileTooLarge(file.size)) {
    actionError.value = new Error(`文件大小 ${formatBytes(file.size)} 超过 5MB 限制，已拒绝导入。`)
    return
  }
  actionError.value = null
  const previous = await currentXml()
  let text = ''
  try {
    text = await readFileText(file)
  } catch (error) {
    markAuthError(error)
    actionError.value = error
    return
  }
  const imported = await designerRef.value.importXml(text)
  if (!imported) {
    if (previous) await designerRef.value.importXml(previous)
    if (!actionError.value) actionError.value = new Error('BPMN 解析失败，已保留导入前的画布。')
    return
  }
  dirty.value = true
  notice.value = `已导入 ${file.name}，请校验并保存。`
  const xml = await currentXml()
  if (xml) validationIssues.value = validateBpmnXml(xml, model.value?.key ?? '').issues
}

// 执行校验并把结果显示在列表。
// @return 无
async function runValidation(): Promise<void> {
  const xml = await currentXml()
  if (!xml.trim()) return
  const result = validateBpmnXml(xml, model.value?.key ?? '')
  validationIssues.value = result.issues
  if (!result.valid) notice.value = `校验发现 ${result.errors.length} 个错误、${result.warnings.length} 个警告。`
  else notice.value = result.warnings.length > 0 ? `校验通过，存在 ${result.warnings.length} 个警告。` : '校验通过。'
}

// 导出当前画布。
// @param format 导出扩展名
// @return 无
async function exportXml(format: 'bpmn' | 'bpmn20.xml'): Promise<void> {
  const xml = await currentXml()
  if (!xml.trim()) return
  notice.value = `已导出 ${exportBpmnXml(xml, model.value?.key ?? '', format)}。`
}

// 展示设计器解析警告。
// @param warnings bpmn-moddle 警告
// @return 无
function onWarnings(warnings: ModelerWarningLike[]): void {
  importWarnings.value = issuesFromModelerWarnings(warnings)
  if (importWarnings.value.length > 0) notice.value = `解析产生 ${importWarnings.value.length} 条警告，请查看校验结果。`
}

// 展示设计器错误。
// @param message 错误文本
// @return 无
function onDesignerError(message: string): void {
  actionError.value = new Error(message)
}

// 点击校验问题定位元素。
// @param elementId BPMN 元素 ID
// @return 无
function focusIssue(elementId: string): void {
  const found = designerRef.value?.selectElement(elementId)
  notice.value = found ? `已定位元素「${elementId}」。` : `当前画布未找到元素「${elementId}」。`
}

/** 浏览器关闭/刷新守卫 */
const beforeUnloadGuard = createBeforeUnloadGuard(() => dirty.value)

onBeforeRouteLeave(() => {
  if (!dirty.value) return true
  return window.confirm('当前模型有未保存的修改，确定离开吗？')
})

watch(modelId, () => {
  void load()
})

onMounted(() => {
  void load()
  window.addEventListener('beforeunload', beforeUnloadGuard)
})

onBeforeUnmount(() => {
  window.removeEventListener('beforeunload', beforeUnloadGuard)
})
</script>

<template>
  <div>
    <PageHeader :title="pageTitle" :description="pageDescription">
      <template #actions>
        <ModelDesignerToolbar
          :dirty="dirty"
          :saving="saving"
          :deploying="deploying"
          @save="save"
          @deploy="deploy"
          @import="chooseFile"
          @export="exportXml"
          @undo="designerRef?.undo()"
          @redo="designerRef?.redo()"
          @zoom-in="designerRef?.zoomIn()"
          @zoom-out="designerRef?.zoomOut()"
          @fit="designerRef?.fitViewport()"
          @validate="runValidation"
        />
      </template>
    </PageHeader>

    <ErrorBlock v-if="loadError" :error="loadError" context="加载模型" @retry="load" />
    <div v-if="loading" class="designer-loading" data-testid="designer-loading">
      <n-spin size="medium" />
      <span>正在加载模型与源码…</span>
    </div>

    <template v-else-if="model && !loadError">
      <div v-if="sourceMissing" class="banner banner--warning">
        该模型暂无源码，已载入包含开始事件的最小 BPMN 模板，保存后写入服务端。
      </div>
      <div v-if="notice" class="banner" role="status">{{ notice }}</div>
      <ErrorBlock v-if="actionError" :error="actionError" context="模型设计器" />
      <div v-if="deployment" class="banner banner--success" data-testid="deployment-result">
        部署成功：<span class="mono">{{ deployment.id }}</span>
        <span v-if="deployedDefinition"> · 流程定义 {{ deployedDefinition.key }} v{{ deployedDefinition.version }}</span>
      </div>
      <ValidationList :issues="allIssues" @focus="focusIssue" />
      <div class="designer-shell">
        <BpmnDesigner
          ref="designerRef"
          :key="model.id"
          :xml="canvasXml"
          :model-key="model.key"
          @dirty="markDirty"
          @error="onDesignerError"
          @warnings="onWarnings"
        />
      </div>
      <input
        ref="fileInput"
        type="file"
        accept=".bpmn,.xml,.bpmn20.xml"
        class="designer-file-input"
        data-testid="import-file"
        @change="onFileChange"
      />
    </template>
  </div>
</template>

<style scoped>
.designer-loading { display: flex; align-items: center; justify-content: center; gap: 10px; min-height: 240px; color: var(--muted); }
.designer-shell { height: calc(100vh - 300px); min-height: 560px; }
.designer-file-input { display: none; }
.banner--success { border-color: var(--success); background: var(--success-soft); color: var(--success); }
</style>
