<script setup lang="ts">
// 历史任务列表：按办理人、实例、定义、任务名与状态筛选，分页查询并跳转详情。
import { computed, h, onMounted, reactive, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { NButton, NInput, NSelect } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { queryHistoricTaskInstances } from '@/api/history'
import type { HistoricTaskInstance, HistoricTaskQuery } from '@/api/history'
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
  { label: '已完成', value: 'true' },
  { label: '进行中', value: 'false' },
]

const route = useRoute()
const router = useRouter()

/** 筛选条件 */
const filters = reactive({
  /** 办理人 */
  assignee: '',
  /** 任务名称 */
  taskName: '',
  /** 流程实例 ID */
  processInstanceId: '',
  /** 流程定义 Key */
  definitionKey: '',
  /** 结束状态，null 表示全部 */
  finished: null as boolean | null,
})
/** 当前页 */
const page = ref(1)
/** 每页条数 */
const pageSize = ref(50)
/** 列表数据 */
const rows = ref<HistoricTaskInstance[]>([])
/** 总条数 */
const total = ref(0)
/** 加载态 */
const loading = ref(false)
/** 请求错误 */
const error = ref<unknown>(null)

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
  filters.assignee = queryText(route.query.assignee)
  filters.taskName = queryText(route.query.taskName)
  filters.processInstanceId = queryText(route.query.processInstanceId)
  filters.definitionKey = queryText(route.query.definitionKey)
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
// @return 历史任务查询参数
function buildQuery(): HistoricTaskQuery {
  return {
    assignee: filters.assignee || undefined,
    name: filters.taskName || undefined,
    processInstanceId: filters.processInstanceId || undefined,
    processDefinitionKey: filters.definitionKey || undefined,
    finished: filters.finished === null ? undefined : filters.finished,
    start: (page.value - 1) * pageSize.value,
    size: pageSize.value,
    sort: 'startTime',
    order: 'desc',
  }
}

// 加载历史任务。
// @return 无
async function load(): Promise<void> {
  lastSignature = signature()
  loading.value = true
  error.value = null
  try {
    const result = await queryHistoricTaskInstances(buildQuery())
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
  if (filters.assignee) query.assignee = filters.assignee
  if (filters.taskName) query.taskName = filters.taskName
  if (filters.processInstanceId) query.processInstanceId = filters.processInstanceId
  if (filters.definitionKey) query.definitionKey = filters.definitionKey
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
  filters.assignee = ''
  filters.taskName = ''
  filters.processInstanceId = ''
  filters.definitionKey = ''
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

/** 列表列定义 */
const columns = computed<DataTableColumns<HistoricTaskInstance>>(() => [
  {
    title: '任务 ID',
    key: 'id',
    width: 190,
    render: (row) =>
      h(
        RouterLink,
        { to: { name: 'historic-task-detail', params: { id: row.id } }, class: 'mono', title: row.id },
        { default: () => shortId(row.id, 10, 6) },
      ),
  },
  { title: '任务名称', key: 'name', minWidth: 160, ellipsis: { tooltip: true }, render: (row) => textOrDash(row.name) },
  { title: '办理人', key: 'assignee', width: 120, render: (row) => textOrDash(row.assignee) },
  {
    title: '流程实例',
    key: 'processInstanceId',
    width: 180,
    render: (row) =>
      row.processInstanceId
        ? h(
            RouterLink,
            { to: { name: 'historic-process-detail', params: { id: row.processInstanceId } }, class: 'mono', title: row.processInstanceId },
            { default: () => shortId(row.processInstanceId, 8, 4) },
          )
        : '—',
  },
  { title: '任务 Key', key: 'taskDefinitionKey', width: 150, render: (row) => textOrDash(row.taskDefinitionKey) },
  {
    title: '状态',
    key: 'state',
    width: 90,
    render: (row) => h(StatusTag, { value: row.endTime ? '已完成' : '进行中', tone: row.endTime ? 'success' : 'info' }),
  },
  { title: '开始时间', key: 'startTime', width: 165, render: (row) => formatDateTime(row.startTime) },
  { title: '结束时间', key: 'endTime', width: 165, render: (row) => formatDateTime(row.endTime) },
  { title: '时长', key: 'durationInMillis', width: 120, render: (row) => formatDurationMs(row.durationInMillis) },
  { title: '表单 Key', key: 'formKey', width: 140, render: (row) => textOrDash(row.formKey) },
  {
    title: '操作',
    key: 'actions',
    width: 80,
    render: (row) =>
      h(
        NButton,
        { size: 'tiny', quaternary: true, onClick: () => void router.push({ name: 'historic-task-detail', params: { id: row.id } }) },
        { default: () => '详情' },
      ),
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
    <PageHeader title="历史任务" description="已归档与在途任务历史，支持办理人、实例、定义与任务名筛选。">
      <template #actions>
        <n-button size="small" quaternary @click="router.push('/history/processes')">历史流程</n-button>
      </template>
    </PageHeader>

    <FilterBar :loading="loading" @search="search" @reset="reset">
      <div class="filter-bar__item">
        <span class="filter-bar__label">办理人</span>
        <n-input v-model:value="filters.assignee" size="small" placeholder="如 alice" clearable @keyup.enter="search" />
      </div>
      <div class="filter-bar__item">
        <span class="filter-bar__label">任务名称</span>
        <n-input v-model:value="filters.taskName" size="small" placeholder="精确匹配" clearable @keyup.enter="search" />
      </div>
      <div class="filter-bar__item" style="min-width: 220px">
        <span class="filter-bar__label">流程实例 ID</span>
        <n-input v-model:value="filters.processInstanceId" size="small" placeholder="精确匹配" clearable @keyup.enter="search" />
      </div>
      <div class="filter-bar__item">
        <span class="filter-bar__label">流程定义 Key</span>
        <n-input v-model:value="filters.definitionKey" size="small" placeholder="如 leave" clearable @keyup.enter="search" />
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
      empty-description="没有符合条件的历史任务；可调整办理人、实例或状态后重试。"
      @update:page="changePage"
      @update:page-size="changePageSize"
      @retry="load"
    />
  </div>
</template>
