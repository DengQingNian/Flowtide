<script setup lang="ts">
// 部署详情：基本信息、资源清单与预览下载、删除部署、关联流程定义。
import { computed, h, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NButton, NDataTable } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import ErrorBlock from '@/components/ErrorBlock.vue'
import EmptyState from '@/components/EmptyState.vue'
import StatusTag from '@/components/StatusTag.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import CopyText from '@/components/CopyText.vue'
import ResourcePreviewPanel from './components/ResourcePreviewPanel.vue'
import {
  deleteDeployment,
  getDeployment,
  getDeploymentResourceBlob,
  listAllProcessDefinitions,
  listDeploymentResources,
} from '@/api/repository'
import type { Deployment, DeploymentResource, ProcessDefinition } from '@/api/repository'
import { formatBytes, formatDateTime, textOrDash } from '@/lib/format'
import { downloadBlob } from '@/lib/download'
import { markAuthError } from '@/stores/ui'

/** 资源表格行 */
type ResourceRow = {
  id: string
  mediaType: string
  type: string
  size: number | null
}

const route = useRoute()
const router = useRouter()
const deploymentId = computed(() => String(route.params.id ?? ''))

const deployment = ref<Deployment | null>(null)
const resources = ref<DeploymentResource[]>([])
const definitions = ref<ProcessDefinition[]>([])
const resourceBlobs = ref<Record<string, Blob>>({})
const selectedId = ref('')
const loading = ref(false)
const error = ref<unknown>(null)
const resourceError = ref<unknown>(null)
const definitionsFailed = ref(false)
const actionError = ref<unknown>(null)
const deleting = ref(false)
const retryAction = ref<() => void>(() => {})

const selectedResource = computed(() => resources.value.find((item) => item.id === selectedId.value) ?? null)
const selectedBlob = computed(() => (selectedId.value ? resourceBlobs.value[selectedId.value] ?? null : null))
const relatedDefinitions = computed(() => definitions.value.filter((item) => item.deploymentId === deploymentId.value))

const resourceRows = computed<ResourceRow[]>(() =>
  resources.value.map((resource) => ({
    id: resource.id,
    mediaType: resource.mediaType ?? '',
    type: resource.type ?? '',
    size: resourceBlobs.value[resource.id]?.size ?? null,
  })),
)

// 返回资源类型的中文名。
// @param type 资源类型
// @return 中文名
function resourceTypeLabel(type: string): string {
  if (type === 'processDefinition') return '流程定义'
  if (type === 'processImage') return '流程图'
  return '资源'
}

const columns: DataTableColumns<ResourceRow> = [
  {
    title: '资源名',
    key: 'id',
    minWidth: 260,
    render: (row) =>
      h(
        NButton,
        { text: true, type: selectedId.value === row.id ? 'primary' : 'default', size: 'small', onClick: () => (selectedId.value = row.id) },
        { default: () => row.id },
      ),
  },
  { title: '大小', key: 'size', width: 110, render: (row) => (row.size === null ? '—' : formatBytes(row.size)) },
  { title: '类型', key: 'type', width: 150, render: (row) => `${resourceTypeLabel(row.type)} · ${textOrDash(row.mediaType)}` },
  {
    title: '操作',
    key: 'actions',
    width: 150,
    render: (row) =>
      h('div', { class: 'row' }, [
        h(NButton, { size: 'tiny', quaternary: true, onClick: () => (selectedId.value = row.id) }, { default: () => '预览' }),
        h(NButton, { size: 'tiny', quaternary: true, onClick: () => void downloadResource(row.id) }, { default: () => '下载' }),
      ]),
  },
]

const blobLoading = new Set<string>()

// 判断资源是否为大体积压缩包，压缩包不预加载内容。
// @param name 资源名
// @return 是否压缩包
function isArchive(name: string): boolean {
  return /\.(zip|bar)$/i.test(name)
}

// 预加载资源内容，用于计算大小与即时预览。
// @param list 资源列表
// @return 无
async function loadResourceBlobs(list: DeploymentResource[]): Promise<void> {
  const targets = list.filter((item) => !isArchive(item.id) && !(item.id in resourceBlobs.value) && !blobLoading.has(item.id))
  for (const item of targets) blobLoading.add(item.id)
  await Promise.all(
    targets.map(async (item) => {
      try {
        resourceBlobs.value[item.id] = await getDeploymentResourceBlob(deploymentId.value, item.id, item.mediaType)
      } catch {
        // 单个资源读取失败不影响清单展示，下载时可重试
      } finally {
        blobLoading.delete(item.id)
      }
    }),
  )
}

// 只加载资源清单，用于资源区重试。
// @return 无
async function loadResources(): Promise<void> {
  resourceError.value = null
  try {
    resources.value = await listDeploymentResources(deploymentId.value)
    if (!selectedId.value && resources.value.length > 0) selectedId.value = resources.value[0].id
    await loadResourceBlobs(resources.value)
  } catch (cause) {
    markAuthError(cause)
    resourceError.value = cause
  }
}

let loadToken = 0

// 加载部署、资源与关联流程定义。
// @return 无
async function load(): Promise<void> {
  const token = ++loadToken
  loading.value = true
  error.value = null
  actionError.value = null
  definitionsFailed.value = false
  try {
    deployment.value = await getDeployment(deploymentId.value)
    if (token !== loadToken) return
    resources.value = []
    resourceBlobs.value = {}
    selectedId.value = ''
    await loadResources()
    try {
      definitions.value = await listAllProcessDefinitions()
    } catch (cause) {
      markAuthError(cause)
      definitionsFailed.value = true
      definitions.value = []
    }
  } catch (cause) {
    if (token !== loadToken) return
    markAuthError(cause)
    error.value = cause
  } finally {
    if (token === loadToken) loading.value = false
  }
}

// 记录操作错误与重试入口。
// @param cause 错误
// @param retry 重试函数
// @return 无
function failAction(cause: unknown, retry: () => void): void {
  markAuthError(cause)
  actionError.value = cause
  retryAction.value = retry
}

// 重试上一次失败的操作。
// @return 无
function retryFailedAction(): void {
  retryAction.value()
}

// 下载部署资源。
// @param resourceId 资源名
// @return 无
async function downloadResource(resourceId: string): Promise<void> {
  try {
    const blob = resourceBlobs.value[resourceId] ?? (await getDeploymentResourceBlob(deploymentId.value, resourceId))
    downloadBlob(resourceId, blob)
  } catch (cause) {
    failAction(cause, () => void downloadResource(resourceId))
  }
}

// 删除当前部署并返回列表。
// @return 无
async function remove(): Promise<void> {
  deleting.value = true
  actionError.value = null
  try {
    await deleteDeployment(deploymentId.value, true)
    await router.push('/repository/deployments')
  } catch (cause) {
    failAction(cause, () => void remove())
  } finally {
    deleting.value = false
  }
}

onMounted(() => void load())
</script>

<template>
  <div>
    <PageHeader title="部署详情" description="查看部署资源、预览内容并管理关联流程定义。">
      <template #actions>
        <n-button size="small" quaternary @click="router.push('/repository/deployments')">返回列表</n-button>
        <n-button size="small" quaternary :loading="loading" @click="load">刷新</n-button>
        <ConfirmButton
          label="删除部署"
          title="删除该部署？"
          description="关联流程实例仍可运行；部署下的流程定义与资源会从仓库移除，且无法恢复。"
          :loading="deleting"
          @confirm="remove"
        />
      </template>
    </PageHeader>

    <ErrorBlock v-if="error" :error="error" context="GET /service/repository/deployments/{id}" @retry="load" />
    <ErrorBlock v-if="actionError" :error="actionError" context="部署操作" @retry="retryFailedAction" />

    <div v-if="deployment" class="stack">
      <div class="panel">
        <div class="panel__header"><span class="panel__title">基本信息</span></div>
        <div class="panel__body detail-grid">
          <div class="detail-item">
            <span class="detail-item__label">部署 ID</span>
            <span class="detail-item__value"><CopyText :value="deployment.id" /></span>
          </div>
          <div class="detail-item">
            <span class="detail-item__label">名称</span>
            <span class="detail-item__value">{{ textOrDash(deployment.name) }}</span>
          </div>
          <div class="detail-item">
            <span class="detail-item__label">租户</span>
            <span class="detail-item__value">{{ textOrDash(deployment.tenantId) }}</span>
          </div>
          <div class="detail-item">
            <span class="detail-item__label">部署时间</span>
            <span class="detail-item__value">{{ formatDateTime(deployment.deployTime) }}</span>
          </div>
          <div class="detail-item">
            <span class="detail-item__label">资源数</span>
            <span class="detail-item__value">{{ resources.length }}</span>
          </div>
          <div class="detail-item">
            <span class="detail-item__label">关联流程定义</span>
            <span class="detail-item__value">{{ relatedDefinitions.length }}</span>
          </div>
        </div>
      </div>

      <div class="panel">
        <div class="panel__header">
          <span class="panel__title">资源清单</span>
          <span class="muted" style="margin-left: auto; font-size: 12px">共 {{ resources.length }} 个资源</span>
        </div>
        <div class="panel__body">
          <ErrorBlock v-if="resourceError" :error="resourceError" context="GET /service/repository/deployments/{id}/resources" @retry="loadResources" />
          <n-data-table
            :columns="columns"
            :data="resourceRows"
            :loading="loading"
            :bordered="false"
            :single-line="false"
            :pagination="false"
            size="small"
          >
            <template #empty>
              <EmptyState title="部署没有资源" description="该部署未包含可展示的资源文件。" />
            </template>
          </n-data-table>
        </div>
      </div>

      <ResourcePreviewPanel
        :deployment-id="deploymentId"
        :resource="selectedResource"
        :blob="selectedBlob"
        @downloaded="() => undefined"
      />

      <div class="panel">
        <div class="panel__header">
          <span class="panel__title">关联流程定义</span>
          <span class="muted" style="margin-left: auto; font-size: 12px">按部署 ID 从定义列表筛选</span>
        </div>
        <div class="panel__body">
          <div v-if="definitionsFailed" class="muted">关联流程定义读取失败，可点击“刷新”重试。</div>
          <EmptyState v-else-if="relatedDefinitions.length === 0" title="暂无关联定义" description="该部署未生成流程定义，或定义已被删除。" />
          <div v-else>
            <div v-for="definition in relatedDefinitions" :key="definition.id" class="row row--between" style="padding: 6px 0; border-bottom: 1px solid var(--border)">
              <n-button text type="primary" size="small" @click="router.push(`/repository/definitions/${definition.id}`)">
                {{ definition.key }} · {{ textOrDash(definition.name) }}
              </n-button>
              <span class="row">
                <span class="muted">v{{ definition.version }}</span>
                <StatusTag :value="definition.suspended ? 'suspended' : 'active'" :label="definition.suspended ? '挂起' : '激活'" />
              </span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
