<script setup lang="ts">
// 批处理详情：摘要、分片列表/详情、批处理文档与删除。
import { h, ref, watch } from 'vue'
import { useRoute, useRouter, RouterLink } from 'vue-router'
import { NButton, NSpin } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { AlertTriangle, ArrowLeft, X } from '@lucide/vue'
import PageHeader from '@/components/PageHeader.vue'
import DataTable from '@/components/DataTable.vue'
import ErrorBlock from '@/components/ErrorBlock.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import CopyText from '@/components/CopyText.vue'
import StatusTag from '@/components/StatusTag.vue'
import JsonViewer from '@/components/JsonViewer.vue'
import EmptyState from '@/components/EmptyState.vue'
import { errorMessage } from '@/api/client'
import { markAuthError } from '@/stores/ui'
import {
  deleteBatch,
  getBatch,
  getBatchDocument,
  getBatchPart,
  getBatchPartDocument,
  getBatchSummary,
  listBatchParts,
} from '@/api/management'
import type { Batch, BatchPart, BatchSummary } from '@/api/management'
import { formatDateTime, formatNumber, shortId, textOrDash } from '@/lib/format'

const route = useRoute()
const router = useRouter()

/** 批次 ID */
const batchId = ref(String(route.params.id ?? ''))
/** 批次 */
const batch = ref<Batch | null>(null)
/** 摘要 */
const summary = ref<BatchSummary | null>(null)
/** 批次文档 */
const document = ref<unknown>(null)
/** 文档加载错误 */
const documentError = ref<unknown>(null)
/** 分片 */
const parts = ref<BatchPart[]>([])
/** 分片总数 */
const partsTotal = ref(0)
/** 分片页码 */
const partsPage = ref(1)
/** 分片每页条数 */
const partsPageSize = ref(50)
/** 分片加载态 */
const partsLoading = ref(false)
/** 分片错误 */
const partsError = ref<unknown>(null)
/** 初始加载态 */
const loading = ref(false)
/** 页面错误 */
const error = ref<unknown>(null)
/** 摘要错误 */
const summaryError = ref<unknown>(null)
/** 删除错误 */
const deleteError = ref<unknown>(null)
/** 删除加载态 */
const deleting = ref(false)
/** 选中的分片 ID */
const selectedPartId = ref('')
/** 分片详情 */
const selectedPart = ref<BatchPart | null>(null)
/** 分片文档 */
const partDocument = ref<unknown>(null)
/** 分片详情加载态 */
const partLoading = ref(false)
/** 分片详情错误 */
const partError = ref<unknown>(null)

// 加载批次详情。
// @return 无
async function load(): Promise<void> {
  loading.value = true
  error.value = null
  summaryError.value = null
  documentError.value = null
  try {
    batch.value = await getBatch(batchId.value)
  } catch (cause) {
    error.value = cause
    markAuthError(cause)
    loading.value = false
    return
  }
  await Promise.all([loadSummary(), loadDocument(), loadParts()])
  loading.value = false
}

// 加载批次摘要。
// @return 无
async function loadSummary(): Promise<void> {
  try {
    summary.value = await getBatchSummary(batchId.value)
  } catch (cause) {
    summaryError.value = cause
    markAuthError(cause)
  }
}

// 加载批次文档。
// @return 无
async function loadDocument(): Promise<void> {
  try {
    document.value = (await getBatchDocument(batchId.value)) ?? null
  } catch (cause) {
    documentError.value = cause
    markAuthError(cause)
  }
}

// 加载分片列表。
// @return 无
async function loadParts(): Promise<void> {
  partsLoading.value = true
  partsError.value = null
  try {
    const result = await listBatchParts(batchId.value, { start: (partsPage.value - 1) * partsPageSize.value, size: partsPageSize.value })
    parts.value = result.data
    partsTotal.value = result.total
  } catch (cause) {
    partsError.value = cause
    markAuthError(cause)
  } finally {
    partsLoading.value = false
  }
}

// 查看分片详情与文档。
// @param id 分片 ID
// @return 无
async function selectPart(id: string): Promise<void> {
  selectedPartId.value = id
  selectedPart.value = null
  partDocument.value = null
  partError.value = null
  partLoading.value = true
  try {
    selectedPart.value = await getBatchPart(id)
    try {
      partDocument.value = (await getBatchPartDocument(id)) ?? null
    } catch (cause) {
      // 旧快照可能没有分片文档，不影响分片字段展示
      if (!partDocument.value) partDocument.value = null
      markAuthError(cause)
    }
  } catch (cause) {
    partError.value = cause
    markAuthError(cause)
  } finally {
    partLoading.value = false
  }
}

// 关闭分片详情。
// @return 无
function closePart(): void {
  selectedPartId.value = ''
  selectedPart.value = null
  partDocument.value = null
  partError.value = null
}

// 删除批次并返回列表。
// @return 无
async function remove(): Promise<void> {
  deleting.value = true
  deleteError.value = null
  try {
    await deleteBatch(batchId.value)
    await router.push({ name: 'batches' })
  } catch (cause) {
    deleteError.value = cause
    markAuthError(cause)
  } finally {
    deleting.value = false
  }
}

// 处理分片页码变化。
// @param value 页码
// @return 无
function changePartsPage(value: number): void {
  partsPage.value = value
  void loadParts()
}

// 处理分片每页条数变化。
// @param value 条数
// @return 无
function changePartsPageSize(value: number): void {
  partsPageSize.value = value
  partsPage.value = 1
  void loadParts()
}

/** 分片表格列 */
const partColumns: DataTableColumns<BatchPart> = [
  {
    key: 'id',
    title: '分片 ID',
    width: 170,
    render: (row) => h('span', { class: 'mono' }, shortId(row.id)),
  },
  { key: 'status', title: '状态', width: 110, render: (row) => h(StatusTag, { value: row.status }) },
  {
    key: 'processInstanceId',
    title: '流程实例',
    width: 160,
    render: (row) =>
      row.processInstanceId
        ? h(RouterLink, { to: `/runtime/instances/${row.processInstanceId}`, class: 'mono' }, { default: () => shortId(row.processInstanceId) })
        : '—',
  },
  { key: 'startTime', title: '开始时间', width: 170, render: (row) => formatDateTime(row.startTime) },
  { key: 'endTime', title: '结束时间', width: 170, render: (row) => formatDateTime(row.endTime) },
  { key: 'errorMessage', title: '失败原因', minWidth: 220, ellipsis: { tooltip: true }, render: (row) => textOrDash(row.errorMessage) },
  {
    key: 'action',
    title: '操作',
    width: 90,
    render: (row) => h(NButton, { size: 'tiny', quaternary: true, onClick: () => void selectPart(row.id) }, { default: () => '查看' }),
  },
]

// 监听路由参数变化。
watch(
  () => route.params.id,
  (value) => {
    batchId.value = String(value ?? '')
    if (batchId.value) void load()
  },
  { immediate: true },
)
</script>

<template>
  <div>
    <PageHeader :title="`批处理详情 ${batchId ? shortId(batchId) : ''}`" description="摘要、分片、文档与删除操作。">
      <template #actions>
        <router-link to="/management/batches" custom v-slot="{ navigate }">
          <n-button size="small" @click="navigate">
            <template #icon><ArrowLeft :size="14" /></template>
            返回列表
          </n-button>
        </router-link>
        <ConfirmButton
          label="删除批次"
          title="删除该批处理？"
          description="将同时删除全部分片与文档，删除后无法恢复。"
          :loading="deleting"
          :disabled="!batch"
          @confirm="remove"
        />
      </template>
    </PageHeader>

    <ErrorBlock v-if="error" :error="error" :context="`GET /service/management/batches/${batchId}`" @retry="load" />
    <div v-if="deleteError" class="banner banner--error" role="alert" style="margin-bottom: 12px">
      <AlertTriangle :size="16" aria-hidden="true" />
      <div class="grow">{{ errorMessage(deleteError) }}</div>
    </div>

    <n-spin :show="loading">
      <div v-if="batch" class="stack">
        <section class="panel">
          <div class="panel__header">
            <span class="panel__title">摘要</span>
            <StatusTag :value="batch.status" />
            <span class="muted" style="margin-left: auto; font-size: 12px">{{ textOrDash(batch.type) }}</span>
          </div>
          <div class="panel__body">
            <ErrorBlock v-if="summaryError" :error="summaryError" context="GET /summary" @retry="loadSummary" />
            <div v-else class="detail-grid">
              <div class="detail-item"><span class="detail-item__label">批次 ID</span><span class="detail-item__value"><CopyText :value="batch.id" /></span></div>
              <div class="detail-item"><span class="detail-item__label">类型</span><span class="detail-item__value">{{ textOrDash(batch.type) }}</span></div>
              <div class="detail-item"><span class="detail-item__label">状态</span><span class="detail-item__value">{{ textOrDash(batch.status) }}</span></div>
              <div class="detail-item"><span class="detail-item__label">开始时间</span><span class="detail-item__value">{{ formatDateTime(batch.startTime) }}</span></div>
              <div class="detail-item"><span class="detail-item__label">结束时间</span><span class="detail-item__value">{{ formatDateTime(batch.endTime) }}</span></div>
              <div class="detail-item"><span class="detail-item__label">分片总数</span><span class="detail-item__value">{{ formatNumber(summary?.total ?? batch.total) }}</span></div>
              <div class="detail-item"><span class="detail-item__label">完成 / 失败</span><span class="detail-item__value">{{ formatNumber(summary?.completed ?? batch.completed) }} / {{ formatNumber(summary?.failed ?? 0) }}</span></div>
              <div class="detail-item"><span class="detail-item__label">执行中 / 排队</span><span class="detail-item__value">{{ formatNumber(summary?.inProgress ?? 0) }} / {{ formatNumber(summary?.queued ?? 0) }}</span></div>
              <div class="detail-item"><span class="detail-item__label">租户</span><span class="detail-item__value">{{ textOrDash(batch.tenantId) }}</span></div>
            </div>
          </div>
        </section>

        <section class="panel">
          <div class="panel__header"><span class="panel__title">批次文档</span></div>
          <div class="panel__body">
            <ErrorBlock v-if="documentError" :error="documentError" context="GET /batch-document" @retry="loadDocument" />
            <JsonViewer v-else-if="document !== null && document !== undefined" :value="document" />
            <EmptyState v-else title="没有批次文档" description="旧快照可能没有保存批次文档。" />
          </div>
        </section>

        <section class="panel">
          <div class="panel__header">
            <span class="panel__title">分片</span>
            <span class="muted" style="margin-left: auto; font-size: 12px">共 {{ formatNumber(partsTotal) }} 个</span>
          </div>
          <div class="panel__body">
            <DataTable
              :columns="partColumns"
              :data="parts"
              :loading="partsLoading"
              :total="partsTotal"
              :page="partsPage"
              :page-size="partsPageSize"
              :page-sizes="[10, 20, 50, 100, 200, 500, 1000]"
              row-key="id"
              :error="partsError"
              empty-description="该批次没有分片记录。"
              :scroll-x="1100"
              @update:page="changePartsPage"
              @update:page-size="changePartsPageSize"
              @retry="loadParts"
            />
          </div>
        </section>

        <section v-if="selectedPartId" class="panel">
          <div class="panel__header">
            <span class="panel__title">分片详情</span>
            <button class="icon-button" type="button" title="关闭" style="margin-left: auto" @click="closePart">
              <X :size="14" />
            </button>
          </div>
          <div class="panel__body">
            <ErrorBlock v-if="partError" :error="partError" :context="`GET /service/management/batch-parts/${selectedPartId}`" @retry="selectPart(selectedPartId)" />
            <n-spin v-else :show="partLoading">
              <div v-if="selectedPart" class="stack" style="gap: 12px">
                <div class="detail-grid">
                  <div class="detail-item"><span class="detail-item__label">分片 ID</span><span class="detail-item__value"><CopyText :value="selectedPart.id" /></span></div>
                  <div class="detail-item"><span class="detail-item__label">所属批次</span><span class="detail-item__value"><CopyText :value="selectedPart.batchId" /></span></div>
                  <div class="detail-item"><span class="detail-item__label">状态</span><span class="detail-item__value">{{ textOrDash(selectedPart.status) }}</span></div>
                  <div class="detail-item"><span class="detail-item__label">流程实例</span><span class="detail-item__value">{{ textOrDash(selectedPart.processInstanceId) }}</span></div>
                  <div class="detail-item"><span class="detail-item__label">开始时间</span><span class="detail-item__value">{{ formatDateTime(selectedPart.startTime) }}</span></div>
                  <div class="detail-item"><span class="detail-item__label">结束时间</span><span class="detail-item__value">{{ formatDateTime(selectedPart.endTime) }}</span></div>
                  <div class="detail-item"><span class="detail-item__label">失败原因</span><span class="detail-item__value">{{ textOrDash(selectedPart.errorMessage) }}</span></div>
                </div>
                <JsonViewer v-if="partDocument !== null" :value="partDocument" title="分片文档" />
              </div>
            </n-spin>
          </div>
        </section>
      </div>
    </n-spin>
  </div>
</template>
