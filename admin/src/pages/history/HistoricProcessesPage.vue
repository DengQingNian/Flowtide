<script setup lang="ts">
// 历史流程实例列表：按定义、业务 Key、租户、状态与时间筛选，分页查询并跳转详情。
import { computed, h, onMounted, reactive, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { NButton, NDatePicker, NInput, NSelect } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { queryHistoricProcessInstances } from '@/api/history'
import type { HistoricProcessInstance, HistoricProcessQuery } from '@/api/history'
import { isAuthError } from '@/api/client'
import { markAuthError } from '@/stores/ui'
import { formatDateTime, formatDurationMs, shortId, textOrDash } from '@/lib/format'
import PageHeader from '@/components/PageHeader.vue'
import FilterBar from '@/components/FilterBar.vue'
import DataTable from '@/components/DataTable.vue'
import StatusTag from '@/components/StatusTag.vue'

/** 状态选项 */
const FINISHED_OPTIONS = [
  { label: '全部', value: 'all' },
  { label: '已结束', value: 'true' },
  { label: '未结束', value: 'false' },
]

const route = useRoute()
const router = useRouter()

/** 筛选条件 */
const filters = reactive({
  /** 流程定义 Key */
  definitionKey: '',
  /** 业务 Key */
  businessKey: '',
  /** 租户 */
  tenantId: '',
  /** 结束状态，null 表示全部 */
  finished: null as boolean | null,
  /** 开始时间下界（毫秒） */
  startedAfter: null as number | null,
  /** 结束时间下界（毫秒） */
  endedAfter: null as number | null,
})
/** 当前页 */
const page = ref(1)
/** 每页条数 */
const pageSize = ref(50)
/** 列表数据 */
const rows = ref<HistoricProcessInstance[]>([])
/** 总条数 */
const total = ref(0)
/** 加载态 */
const loading = ref(false)
/** 请求错误 */
const error = ref<unknown>(null)

/** 最近一次发起请求时的条件签名，用于避免路由同步导致重复加载 */
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
  filters.definitionKey = queryText(route.query.definitionKey)
  filters.businessKey = queryText(route.query.businessKey)
  filters.tenantId = queryText(route.query.tenantId)
  const finished = queryText(route.query.finished)
  filters.finished = finished === 'true' ? true : finished === 'false' ? false : null
  filters.startedAfter = queryNumber(route.query.startedAfter)
  filters.endedAfter = queryNumber(route.query.endedAfter)
  page.value = queryNumber(route.query.page) ?? 1
  pageSize.value = queryNumber(route.query.size) ?? 50
}

// 计算当前筛选条件签名。
// @return 条件签名
function signature(): string {
  return JSON.stringify({ ...filters, page: page.value, size: pageSize.value })
}

// 将毫秒时间转换为查询所需的 RFC3339 文本。
// @param value 毫秒时间
// @return ISO 文本或 undefined
function toIso(value: number | null): string | undefined {
  return value === null ? undefined : new Date(value).toISOString()
}

// 组装查询请求体。
// @return 历史流程查询参数
function buildQuery(): HistoricProcessQuery {
  return {
    processDefinitionKey: filters.definitionKey || undefined,
    businessKey: filters.businessKey || undefined,
    tenantId: filters.tenantId || undefined,
    finished: filters.finished === null ? undefined : filters.finished,
    startedAfter: toIso(filters.startedAfter),
    endedAfter: toIso(filters.endedAfter),
    start: (page.value - 1) * pageSize.value,
    size: pageSize.value,
    sort: 'startTime',
    order: 'desc',
  }
}

// 加载历史流程实例。
// @return 无
async function load(): Promise<void> {
  lastSignature = signature()
  loading.value = true
  error.value = null
  try {
    const result = await queryHistoricProcessInstances(buildQuery())
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
  if (filters.definitionKey) query.definitionKey = filters.definitionKey
  if (filters.businessKey) query.businessKey = filters.businessKey
  if (filters.tenantId) query.tenantId = filters.tenantId
  if (filters.finished !== null) query.finished = String(filters.finished)
  if (filters.startedAfter !== null) query.startedAfter = String(filters.startedAfter)
  if (filters.endedAfter !== null) query.endedAfter = String(filters.endedAfter)
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
  filters.definitionKey = ''
  filters.businessKey = ''
  filters.tenantId = ''
  filters.finished = null
  filters.startedAfter = null
  filters.endedAfter = null
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

// 打开历史流程详情。
// @param id 实例 ID
// @return 无
function openDetail(id: string): void {
  void router.push({ name: 'historic-process-detail', params: { id } })
}

/** 状态显示值 */
const finishedValue = computed(() => (filters.finished === null ? 'all' : String(filters.finished)))

// 切换结束状态筛选。
// @param value all/true/false
// @return 无
function changeFinished(value: string): void {
  filters.finished = value === 'all' ? null : value === 'true'
  search()
}

/** 列表列定义 */
const columns = computed<DataTableColumns<HistoricProcessInstance>>(() => [
  {
    title: '实例 ID',
    key: 'id',
    width: 190,
    render: (row) =>
      h(
        RouterLink,
        { to: { name: 'historic-process-detail', params: { id: row.id } }, class: 'mono', title: row.id },
        { default: () => shortId(row.id, 10, 6) },
      ),
  },
  {
    title: '流程定义',
    key: 'processDefinitionName',
    minWidth: 180,
    render: (row) => {
      const label = row.processDefinitionName || row.processDefinitionId
      return h(
        RouterLink,
        { to: `/repository/definitions/${encodeURIComponent(row.processDefinitionId)}`, title: row.processDefinitionId },
        { default: () => label },
      )
    },
  },
  { title: '业务 Key', key: 'businessKey', width: 140, render: (row) => textOrDash(row.businessKey) },
  { title: '发起人', key: 'startUserId', width: 110, render: (row) => textOrDash(row.startUserId) },
  {
    title: '状态',
    key: 'state',
    width: 90,
    render: (row) => h(StatusTag, { value: row.endTime ? '已完成' : '运行中', tone: row.endTime ? 'success' : 'info' }),
  },
  { title: '开始时间', key: 'startTime', width: 165, render: (row) => formatDateTime(row.startTime) },
  { title: '结束时间', key: 'endTime', width: 165, render: (row) => formatDateTime(row.endTime) },
  { title: '时长', key: 'durationInMillis', width: 120, render: (row) => formatDurationMs(row.durationInMillis) },
  { title: '删除原因', key: 'deleteReason', minWidth: 140, ellipsis: { tooltip: true }, render: (row) => textOrDash(row.deleteReason) },
  {
    title: '操作',
    key: 'actions',
    width: 150,
    render: (row) =>
      h('div', { class: 'row' }, [
        h(NButton, { size: 'tiny', quaternary: true, onClick: () => openDetail(row.id) }, { default: () => '详情' }),
        // 未结束实例仍可进入运行态详情，已结束实例只保留历史入口
        row.endTime
          ? null
          : h(
              NButton,
              { size: 'tiny', quaternary: true, onClick: () => void router.push(`/runtime/instances/${encodeURIComponent(row.id)}`) },
              { default: () => '运行实例' },
            ),
      ]),
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
    <PageHeader title="历史流程" description="已结束与在途流程实例的历史记录，支持定义、业务 Key、租户与时间筛选。">
      <template #actions>
        <n-button size="small" quaternary @click="router.push('/history/tasks')">历史任务</n-button>
        <n-button size="small" quaternary @click="router.push('/history/activities')">历史活动</n-button>
      </template>
    </PageHeader>

    <FilterBar :loading="loading" @search="search" @reset="reset">
      <div class="filter-bar__item">
        <span class="filter-bar__label">流程定义 Key</span>
        <n-input v-model:value="filters.definitionKey" size="small" placeholder="如 leave" clearable @keyup.enter="search" />
      </div>
      <div class="filter-bar__item">
        <span class="filter-bar__label">业务 Key</span>
        <n-input v-model:value="filters.businessKey" size="small" placeholder="精确匹配" clearable @keyup.enter="search" />
      </div>
      <div class="filter-bar__item">
        <span class="filter-bar__label">租户</span>
        <n-input v-model:value="filters.tenantId" size="small" placeholder="精确匹配" clearable @keyup.enter="search" />
      </div>
      <div class="filter-bar__item" style="min-width: 130px">
        <span class="filter-bar__label">状态</span>
        <n-select :value="finishedValue" size="small" :options="FINISHED_OPTIONS" @update:value="changeFinished" />
      </div>
      <div class="filter-bar__item" style="min-width: 210px">
        <span class="filter-bar__label">开始时间不早于</span>
        <n-date-picker v-model:value="filters.startedAfter" type="datetime" size="small" clearable style="width: 100%" />
      </div>
      <div class="filter-bar__item" style="min-width: 210px">
        <span class="filter-bar__label">结束时间不早于</span>
        <n-date-picker v-model:value="filters.endedAfter" type="datetime" size="small" clearable style="width: 100%" />
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
      empty-description="没有符合条件的历史流程实例；可放宽时间范围或清空筛选后重试。"
      @update:page="changePage"
      @update:page-size="changePageSize"
      @retry="load"
    />
  </div>
</template>
