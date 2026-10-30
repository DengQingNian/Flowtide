<script setup lang="ts">
// 部署资源预览：按资源类型展示 XML、JSON、文本、图片，二进制仅提供下载提示。
import { computed, onUnmounted, ref, watch } from 'vue'
import { NButton, NSpin } from 'naive-ui'
import ErrorBlock from '@/components/ErrorBlock.vue'
import EmptyState from '@/components/EmptyState.vue'
import JsonViewer from '@/components/JsonViewer.vue'
import XmlViewer from '@/components/XmlViewer.vue'
import {
  classifyResource,
  getDeploymentResourceBlob,
} from '@/api/repository'
import type { DeploymentResource } from '@/api/repository'
import { formatBytes, textOrDash } from '@/lib/format'
import { downloadBlob } from '@/lib/download'
import { markAuthError } from '@/stores/ui'

const props = withDefaults(
  defineProps<{
    /** 部署 ID */
    deploymentId: string
    /** 当前资源 */
    resource?: DeploymentResource | null
    /** 已加载的资源二进制 */
    blob?: Blob | null
  }>(),
  { resource: null, blob: null },
)

const emit = defineEmits<{ downloaded: [string] }>()

const loading = ref(false)
const error = ref<unknown>(null)
const downloadError = ref<unknown>(null)
const text = ref('')
const objectUrl = ref('')
let previewToken = 0

/** 预览类型 */
const kind = computed(() => (props.resource ? classifyResource(props.resource.id, props.resource.mediaType) : 'binary'))
/** JSON 解析结果，解析失败时退回原始文本 */
const jsonValue = computed(() => {
  try {
    return JSON.parse(text.value) as unknown
  } catch {
    return text.value
  }
})
/** 资源大小 */
const sizeText = computed(() => (props.blob ? formatBytes(props.blob.size) : '—'))

// 释放图片预览地址。
// @return 无
function releaseObjectUrl(): void {
  if (objectUrl.value) {
    URL.revokeObjectURL(objectUrl.value)
    objectUrl.value = ''
  }
}

// 读取 Blob 文本，兼容未实现 Blob.text 的测试与旧浏览器。
// @param blob 二进制内容
// @return 文本
function blobToText(blob: Blob): Promise<string> {
  if (typeof blob.text === 'function') return blob.text()
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result ?? ''))
    reader.onerror = () => reject(reader.error ?? new Error('读取资源内容失败'))
    reader.readAsText(blob)
  })
}

// 加载文本或图片预览。
// @return 无
async function loadPreview(): Promise<void> {
  releaseObjectUrl()
  text.value = ''
  error.value = null
  downloadError.value = null
  const resource = props.resource
  if (!resource) return
  if (kind.value === 'binary') return
  const token = ++previewToken
  loading.value = true
  try {
    // 图片必须使用二进制，其余预览优先复用已加载内容，避免重复请求
    const blob = props.blob ?? (await getDeploymentResourceBlob(props.deploymentId, resource.id, resource.mediaType))
    if (token !== previewToken) return
    if (kind.value === 'image') {
      objectUrl.value = URL.createObjectURL(blob)
      return
    }
    text.value = await blobToText(blob)
  } catch (cause) {
    if (token !== previewToken) return
    markAuthError(cause)
    error.value = cause
  } finally {
    if (token === previewToken) loading.value = false
  }
}

// 下载当前资源，按需重新读取原始内容。
// @return 无
async function download(): Promise<void> {
  if (!props.resource) return
  downloadError.value = null
  try {
    const blob = props.blob ?? (await getDeploymentResourceBlob(props.deploymentId, props.resource.id, props.resource.mediaType))
    downloadBlob(props.resource.id, blob)
    emit('downloaded', props.resource.id)
  } catch (cause) {
    markAuthError(cause)
    downloadError.value = cause
  }
}

watch(() => [props.resource?.id, props.blob], () => void loadPreview(), { immediate: true })

onUnmounted(releaseObjectUrl)
</script>

<template>
  <div class="panel">
    <div class="panel__header">
      <span class="panel__title">资源预览</span>
      <span v-if="resource" class="muted mono" style="margin-left: auto">{{ textOrDash(resource.mediaType) }}</span>
    </div>
    <div class="panel__body">
      <EmptyState v-if="!resource" title="未选择资源" description="在资源清单中点击“预览”查看内容。" />
      <template v-else>
        <div class="row row--between" style="margin-bottom: 10px">
          <div class="mono" style="overflow-wrap: anywhere">{{ resource.id }}</div>
          <div class="row">
            <span class="muted" style="font-size: 12px">{{ sizeText }}</span>
            <n-button size="tiny" quaternary :disabled="!resource" @click="download">下载</n-button>
          </div>
        </div>
        <ErrorBlock v-if="downloadError" :error="downloadError" context="下载部署资源" @retry="download" />
        <ErrorBlock v-if="error" :error="error" context="读取部署资源内容" @retry="loadPreview" />
        <div v-else-if="loading" class="row" style="justify-content: center; padding: 24px">
          <n-spin size="small" />
        </div>
        <img
          v-else-if="kind === 'image' && objectUrl"
          :src="objectUrl"
          :alt="resource.id"
          style="max-width: 100%; max-height: 520px; border: 1px solid var(--border); border-radius: 6px"
        />
        <XmlViewer v-else-if="kind === 'xml'" :xml="text" :filename="resource.id" />
        <JsonViewer v-else-if="kind === 'json'" :value="jsonValue" :title="resource.id" />
        <pre v-else-if="kind === 'text'" class="code-block">{{ text }}</pre>
        <div v-else class="banner banner--warning" style="margin: 0">
          该资源为二进制内容，无法在页面内预览，请下载后使用本地工具查看。
        </div>
      </template>
    </div>
  </div>
</template>
