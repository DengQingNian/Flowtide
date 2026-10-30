<script setup lang="ts">
// 历史活动列表：按实例、定义、活动与类型筛选，弹窗查看单条活动详情。
import { computed, h, onMounted, reactive, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { NButton, NInput, NModal, NSelect, NSpin } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { getHistoricActivity, queryHistoricActivities } from '@/api/history'
import type { HistoricActivityInstance, HistoricActivityQuery } from '@/api/history'
import { isAuthError } from '@/api/client'
import { markAuthError } from '@/stores/ui'
import { formatDateTime, formatDurationMs, shortId, textOrDash } from '@/lib/format'
import PageHeader from '@/components/PageHeader.vue'
import FilterBar from '@/components/FilterBar.vue'
import DataTable from '@/components/DataTable.vue'
import StatusTag from '@/components/StatusTag.vue'
import ErrorBlock from '@/components/ErrorBlock.vue'
import CopyText from '@/components/CopyText.vue'

/** 状态选项 */
const FINISHED_OPTIONS = [
  { label: '全部', value: 'all' },
  { label: '已结束', value: 'true' },
  { label: '进行中', value: 'false' },
]

const route = useRoute()
const router = useRouter()

/** 筛选条件 */
const filters = reactive({
  /** 流程实例 ID */
  processInstanceId: '',
  /** 流程定义 Key */
  definitionKey: '',
  /** 活动 ID */
  activityId: '',
  /** 活动类型 */
  activityType: '',
  /** 结束状态，null 表示全部 */
  finished: null as boolean | null,
})
/** 当前页 */
const page = ref(1)
/** 每页条数 */
const pageSize = ref(50)
/** 列表数据 */
const rows = ref<HistoricActivityInstance[]>([])
/** 总条数 */
const total = ref(0)
/** 加载态 */
const loading = ref(false)
/** 请求错误 */
const error = ref<unknown>(null)
/** 详情弹窗显隐 */
const detailShow = ref(false)
/** 当前详情 */
const detail = ref<HistoricActivityInstance | null>(null)
/** 当前详情 ID，供重试使用 */
const detailId = ref('')
/** 详情加载态 */
const detailLoading = ref(false)
/** 详情错误 */
const detailError = ref<unknown>(null)

/** 最近一次请求条件签名，避免路由同步导致重复加载 */
let lastSignature = ''

// 读取字符串型查询参数。
// @param value 路由参数
// @return 字符串或空串
function queryText(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

// 读取数值型查询参数。
// @param value 路由参数
// @return 数值或 null
function queryNumber(value: unknown): number | null {
  const text = queryText(value)
  if (!text) return null
  const parsed = Number(text)
  return Number.isFinite(parsed) ? parsed : null
}

// 从路由恢复筛选与分页。
// @return 无
function readRoute(): void {
  filters.processInstanceId = queryText(route.query.processInstanceId)
  filters.definitionKey = queryText(route.query.definitionKey)
  filters.activityId = queryText(route.query.activityId)
  filters.activityType = queryText(route.query.activityType)
  const finished = queryText(route.query.finished)
  filters.finished = finished === 'true' ? true : finished === 'false' ? false : null
  page.value = queryNumber(route.query.page) ?? 1
  pageSize.value = queryNumber(route.query.size) ?? 50
}

// 计算当前筛选条件签名。
// @return 条件签名
function signature(): string {
  return JSON.stringify({ ...filters, page: page.value, size: pageSize.value })
}

// 组装查询请求体。
// @return 历史活动查询参数
function buildQuery(): HistoricActivityQuery {
  return {
    processInstanceId: filters.processInstanceId || undefined,
    processDefinitionKey: filters.definitionKey || undefined,
    activityId: filters.activityId || undefined,
    activityType: filters.activityType || undefined,
    finished: filters.finished === null ? undefined : filters.finished,
    start: (page.value - 1) * pageSize.value,
    size: pageSize.value,
    sort: 'startTime',
    order: 'desc',
  }
}

// 加载历史活动。
// @return 无
async function load(): Promise<void> {
  lastSignature = signature()
  loading.value = true
  error.value = null
  try {
    const result = await queryHistoricActivities(buildQuery())
    rows.value = result.data
    total.value = result.total
  } catch (cause) {
    error.value = cause
    if (isAuthError(cause)) markAuthError(cause)
  } finally {
    loading.value = false
  }
}

// 把当前筛选写入路由 query。
// @return 无
function writeRoute(): void {
  const query: Record<string, string> = {}
  if (filters.processInstanceId) query.processInstanceId = filters.processInstanceId
  if (filters.definitionKey) query.definitionKey = filters.definitionKey
  if (filters.activityId) query.activityId = filters.activityId
  if (filters.activityType) query.activityType = filters.activityType
  if (filters.finished !== null) query.finished = String(filters.finished)
  query.page = String(page.value)
  query.size = String(pageSize.value)
  lastSignature = signature()
  void router.replace({ query })
}

// 应用筛选并回到第一页。
// @return 无
function search(): void {
  page.value = 1
  writeRoute()
  void load()
}

// 重置全部筛选。
// @return 无
function reset(): void {
  filters.processInstanceId = ''
  filters.definitionKey = ''
  filters.activityId = ''
  filters.activityType = ''
  filters.finished = null
  page.value = 1
  pageSize.value = 50
  writeRoute()
  void load()
}

// 处理页码变化。
// @param value 新页码
// @return 无
function changePage(value: number): void {
  page.value = value
  writeRoute()
  void load()
}

// 处理每页条数变化。
// @param value 新条数
// @return 无
function changePageSize(value: number): void {
  pageSize.value = value
  page.value = 1
  writeRoute()
  void load()
}

// 切换结束状态筛选。
// @param value all/true/false
// @return 无
function changeFinished(value: string): void {
  filters.finished = value === 'all' ? null : value === 'true'
  search()
}

/** 状态显示值 */
const finishedValue = computed(() => (filters.finished === null ? 'all' : String(filters.finished)))

// 加载单条活动详情。
// @param id 历史活动 ID
// @return 无
async function openDetail(id: string): Promise<void> {
  detailShow.value = true
  detailId.value = id
  detail.value = null
  detailError.value = null
  detailLoading.value = true
  try {
    detail.value = await getHistoricActivity(id)
  } catch (cause) {
    detailError.value = cause
    if (isAuthError(cause)) markAuthError(cause)
  } finally {
    detailLoading.value = false
  }
}

/** 列表列定义 */
const columns = computed<DataTableColumns<HistoricActivityInstance>>(() => [
  { title: '活动 ID', key: 'activityId', width: 190, render: (row) => h('span', { class: 'mono' }, row.activityId) },
  { title: '名称', key: 'activityName', minWidth: 150, ellipsis: { tooltip: true }, render: (row) => textOrDash(row.activityName) },
  { title: '类型', key: 'activityType', width: 140, render: (row) => textOrDash(row.activityType) },
  {
    title: '流程实例',
    key: 'processInstanceId',
    width: 180,
    render: (row) =>
      h(
        RouterLink,
        { to: { name: 'historic-process-detail', params: { id: row.processInstanceId } }, class: 'mono', title: row.processInstanceId },
        { default: () => shortId(row.processInstanceId, 8, 4) },
      ),
  },
  { title: '执行 ID', key: 'executionId', width: 170, render: (row) => (row.executionId ? h('span', { class: 'mono' }, shortId(row.executionId, 8, 4)) : '—') },
  {
    title: '任务',
    key: 'taskId',
    width: 150,
    render: (row) =>
      row.taskId
        ? h(
            RouterLink,
            { to: { name: 'historic-task-detail', params: { id: row.taskId } }, class: 'mono', title: row.taskId },
            { default: () => shortId(row.taskId, 8, 4) },
          )
        : '—',
  },
  { title: '办理人', key: 'assignee', width: 110, render: (row) => textOrDash(row.assignee) },
  {
    title: '状态',
    key: 'state',
    width: 90,
    render: (row) => h(StatusTag, { value: row.endTime ? '已完成' : '进行中', tone: row.endTime ? 'success' : 'info' }),
  },
  { title: '开始时间', key: 'startTime', width: 165, render: (row) => formatDateTime(row.startTime) },
  { title: '结束时间', key: 'endTime', width: 165, render: (row) => formatDateTime(row.endTime) },
  { title: '时长', key: 'durationInMillis', width: 120, render: (row) => formatDurationMs(row.durationInMillis) },
  {
    title: '操作',
    key: 'actions',
    width: 80,
    render: (row) =>
      h(NButton, { size: 'tiny', quaternary: true, onClick: () => void openDetail(row.id) }, { default: () => '查看' }),
  },
])

onMounted(() => {
  readRoute()
  void load()
})

watch(
  () => route.query,
  () => {
    readRoute()
    // 与最近一次请求条件一致时不再重复查询
    if (signature() !== lastSignature) void load()
  },
)
</script>

<template>
  <div>
    <PageHeader
      title="历史活动"
      description="活动执行记录、时长与关联任务；历史活动不支持单独删除，删除流程实例历史时会一并清理。"
    >
      <template #actions>
        <n-button size="small" quaternary @click="router.push('/history/processes')">历史流程</n-button>
      </template>
    </PageHeader>

    <FilterBar :loading="loading" @search="search" @reset="reset">
      <div class="filter-bar__item" style="min-width: 220px">
        <span class="filter-bar__label">流程实例 ID</span>
        <n-input v-model:value="filters.processInstanceId" size="small" placeholder="精确匹配" clearable @keyup.enter="search" />
      </div>
      <div class="filter-bar__item">
        <span class="filter-bar__label">流程定义 Key</span>
        <n-input v-model:value="filters.definitionKey" size="small" placeholder="如 leave" clearable @keyup.enter="search" />
      </div>
      <div class="filter-bar__item">
        <span class="filter-bar__label">活动 ID</span>
        <n-input v-model:value="filters.activityId" size="small" placeholder="BPMN 元素 ID" clearable @keyup.enter="search" />
      </div>
      <div class="filter-bar__item">
        <span class="filter-bar__label">活动类型</span>
        <n-input v-model:value="filters.activityType" size="small" placeholder="如 userTask" clearable @keyup.enter="search" />
      </div>
      <div class="filter-bar__item" style="min-width: 130px">
        <span class="filter-bar__label">状态</span>
        <n-select :value="finishedValue" size="small" :options="FINISHED_OPTIONS" @update:value="changeFinished" />
      </div>
    </FilterBar>

    <DataTable
      :columns="columns"
      :data="rows"
      :loading="loading"
      :error="error"
      :total="total"
      :page="page"
      :page-size="pageSize"
      :page-sizes="[10, 20, 50, 100, 200, 500, 1000]"
      empty-description="没有符合条件的历史活动；可按流程实例缩小范围后重试。"
      @update:page="changePage"
      @update:page-size="changePageSize"
      @retry="load"
    />

    <n-modal v-model:show="detailShow" preset="card" title="历史活动详情" :style="{ width: '720px', maxWidth: '94vw' }">
      <n-spin :show="detailLoading">
        <ErrorBlock v-if="detailError" :error="detailError" @retry="openDetail(detailId)" />
        <div v-else-if="detail" class="detail-grid">
          <div class="detail-item">
            <span class="detail-item__label">历史活动 ID</span>
            <span class="detail-item__value"><CopyText :value="detail.id" /></span>
          </div>
          <div class="detail-item">
            <span class="detail-item__label">活动 ID</span>
            <span class="detail-item__value mono">{{ detail.activityId }}</span>
          </div>
          <div class="detail-item">
            <span class="detail-item__label">活动名称</span>
            <span class="detail-item__value">{{ textOrDash(detail.activityName) }}</span>
          </div>
          <div class="detail-item">
            <span class="detail-item__label">活动类型</span>
            <span class="detail-item__value">{{ textOrDash(detail.activityType) }}</span>
          </div>
          <div class="detail-item">
            <span class="detail-item__label">流程实例</span>
            <span class="detail-item__value mono">{{ detail.processInstanceId }}</span>
          </div>
          <div class="detail-item">
            <span class="detail-item__label">执行 ID</span>
            <span class="detail-item__value mono">{{ textOrDash(detail.executionId) }}</span>
          </div>
          <div class="detail-item">
            <span class="detail-item__label">任务 ID</span>
            <span class="detail-item__value mono">{{ textOrDash(detail.taskId) }}</span>
          </div>
          <div class="detail-item">
            <span class="detail-item__label">办理人</span>
            <span class="detail-item__value">{{ textOrDash(detail.assignee) }}</span>
          </div>
          <div class="detail-item">
            <span class="detail-item__label">开始时间</span>
            <span class="detail-item__value">{{ formatDateTime(detail.startTime) }}</span>
          </div>
          <div class="detail-item">
            <span class="detail-item__label">结束时间</span>
            <span class="detail-item__value">{{ formatDateTime(detail.endTime) }}</span>
          </div>
          <div class="detail-item">
            <span class="detail-item__label">时长</span>
            <span class="detail-item__value">{{ formatDurationMs(detail.durationInMillis) }}</span>
          </div>
          <div class="detail-item">
            <span class="detail-item__label">租户</span>
            <span class="detail-item__value">{{ textOrDash(detail.tenantId) }}</span>
          </div>
        </div>
      </n-spin>
    </n-modal>
  </div>
</template>
