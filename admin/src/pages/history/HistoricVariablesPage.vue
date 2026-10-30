<script setup lang="ts">
// 历史变量与明细：切换变量实例/变量明细，按实例、执行、任务与变量名筛选，支持原始数据与删除。
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NButton, NButtonGroup, NInput } from 'naive-ui'
import {
  deleteHistoricDetail,
  deleteHistoricVariable,
  getHistoricDetailData,
  getHistoricVariableData,
  queryHistoricDetails,
  queryHistoricVariables,
} from '@/api/history'
import type { HistoricDetail, HistoricDetailQuery, HistoricVariableInstance, HistoricVariableQuery } from '@/api/history'
import { isAuthError } from '@/api/client'
import { markAuthError } from '@/stores/ui'
import PageHeader from '@/components/PageHeader.vue'
import FilterBar from '@/components/FilterBar.vue'
import ErrorBlock from '@/components/ErrorBlock.vue'
import HistoricVariableTable from './components/HistoricVariableTable.vue'
import HistoricDetailTable from './components/HistoricDetailTable.vue'
import HistoricRawValueDialog from './components/HistoricRawValueDialog.vue'

/** 页签键 */
type TabKey = 'variables' | 'details'

/** 页签定义 */
const TABS: { value: TabKey; label: string }[] = [
  { value: 'variables', label: '变量实例' },
  { value: 'details', label: '变量明细' },
]

const route = useRoute()
const router = useRouter()

/** 当前页签 */
const tab = ref<TabKey>('variables')
/** 筛选条件 */
const filters = reactive({
  /** 流程实例 ID */
  processInstanceId: '',
  /** 执行 ID */
  executionId: '',
  /** 任务 ID */
  taskId: '',
  /** 变量名 */
  name: '',
})
/** 当前页 */
const page = ref(1)
/** 每页条数 */
const pageSize = ref(50)
/** 变量实例数据 */
const variables = ref<HistoricVariableInstance[]>([])
/** 变量实例总数 */
const variableTotal = ref(0)
/** 变量实例加载态 */
const variableLoading = ref(false)
/** 变量实例错误 */
const variableError = ref<unknown>(null)
/** 明细数据 */
const details = ref<HistoricDetail[]>([])
/** 明细总数 */
const detailTotal = ref(0)
/** 明细加载态 */
const detailLoading = ref(false)
/** 明细错误 */
const detailError = ref<unknown>(null)
/** 正在删除的记录 ID */
const deletingId = ref('')
/** 删除错误 */
const actionError = ref<unknown>(null)
/** 原始数据弹窗显隐 */
const rawShow = ref(false)
/** 原始数据标题 */
const rawTitle = ref('原始数据')
/** 原始数据文件名 */
const rawFilename = ref('value.bin')
/** 原始数据加载函数 */
const rawLoader = ref<(() => Promise<Blob>) | null>(null)

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

// 从路由恢复筛选、页签与分页。
// @return 无
function readRoute(): void {
  tab.value = queryText(route.query.tab) === 'details' ? 'details' : 'variables'
  filters.processInstanceId = queryText(route.query.processInstanceId)
  filters.executionId = queryText(route.query.executionId)
  filters.taskId = queryText(route.query.taskId)
  filters.name = queryText(route.query.name)
  page.value = queryNumber(route.query.page) ?? 1
  pageSize.value = queryNumber(route.query.size) ?? 50
}

// 计算当前筛选条件签名。
// @return 条件签名
function signature(): string {
  return JSON.stringify({ tab: tab.value, ...filters, page: page.value, size: pageSize.value })
}

// 组装变量实例查询请求体。
// @return 历史变量查询参数
function buildVariableQuery(): HistoricVariableQuery {
  return {
    processInstanceId: filters.processInstanceId || undefined,
    executionId: filters.executionId || undefined,
    taskId: filters.taskId || undefined,
    name: filters.name || undefined,
    start: (page.value - 1) * pageSize.value,
    size: pageSize.value,
    sort: 'name',
    order: 'asc',
  }
}

// 组装变量明细查询请求体。
// @return 历史明细查询参数
function buildDetailQuery(): HistoricDetailQuery {
  return {
    processInstanceId: filters.processInstanceId || undefined,
    executionId: filters.executionId || undefined,
    taskId: filters.taskId || undefined,
    name: filters.name || undefined,
    start: (page.value - 1) * pageSize.value,
    size: pageSize.value,
    sort: 'time',
    order: 'desc',
  }
}

// 加载变量实例。
// @return 无
async function loadVariables(): Promise<void> {
  variableLoading.value = true
  variableError.value = null
  try {
    const result = await queryHistoricVariables(buildVariableQuery())
    variables.value = result.data
    variableTotal.value = result.total
  } catch (cause) {
    variableError.value = cause
    if (isAuthError(cause)) markAuthError(cause)
  } finally {
    variableLoading.value = false
  }
}

// 加载变量明细。
// @return 无
async function loadDetails(): Promise<void> {
  detailLoading.value = true
  detailError.value = null
  try {
    const result = await queryHistoricDetails(buildDetailQuery())
    details.value = result.data
    detailTotal.value = result.total
  } catch (cause) {
    detailError.value = cause
    if (isAuthError(cause)) markAuthError(cause)
  } finally {
    detailLoading.value = false
  }
}

// 按当前页签加载数据。
// @return 无
function load(): Promise<void> {
  lastSignature = signature()
  return tab.value === 'variables' ? loadVariables() : loadDetails()
}

// 把当前状态写入路由 query。
// @return 无
function writeRoute(): void {
  const query: Record<string, string> = { tab: tab.value }
  if (filters.processInstanceId) query.processInstanceId = filters.processInstanceId
  if (filters.executionId) query.executionId = filters.executionId
  if (filters.taskId) query.taskId = filters.taskId
  if (filters.name) query.name = filters.name
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
  filters.executionId = ''
  filters.taskId = ''
  filters.name = ''
  page.value = 1
  pageSize.value = 50
  writeRoute()
  void load()
}

// 切换页签并重新加载。
// @param value 页签
// @return 无
function switchTab(value: TabKey): void {
  tab.value = value
  page.value = 1
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

// 删除变量实例并刷新列表。
// @param row 变量
// @return 无
async function removeVariable(row: HistoricVariableInstance): Promise<void> {
  deletingId.value = row.id
  actionError.value = null
  try {
    await deleteHistoricVariable(row.id)
    await load()
  } catch (cause) {
    actionError.value = cause
    if (isAuthError(cause)) markAuthError(cause)
  } finally {
    deletingId.value = ''
  }
}

// 删除变量明细并刷新列表。
// @param row 明细
// @return 无
async function removeDetail(row: HistoricDetail): Promise<void> {
  deletingId.value = row.id
  actionError.value = null
  try {
    await deleteHistoricDetail(row.id)
    await load()
  } catch (cause) {
    actionError.value = cause
    if (isAuthError(cause)) markAuthError(cause)
  } finally {
    deletingId.value = ''
  }
}

// 打开展示变量实例原始数据。
// @param row 变量
// @return 无
function openVariableRaw(row: HistoricVariableInstance): void {
  rawTitle.value = `变量 ${row.variable.name} 原始数据`
  rawFilename.value = `${row.variable.name}.bin`
  rawLoader.value = () => getHistoricVariableData(row.id)
  rawShow.value = true
}

// 打开展示变量明细原始数据。
// @param row 明细
// @return 无
function openDetailRaw(row: HistoricDetail): void {
  const name = row.variable?.name ?? 'variable'
  rawTitle.value = `明细 ${name} 原始数据`
  rawFilename.value = `${name}.bin`
  rawLoader.value = () => getHistoricDetailData(row.id)
  rawShow.value = true
}

// 关闭原始数据弹窗并释放加载函数。
// @param value 弹窗显隐
// @return 无
function closeRaw(value: boolean): void {
  rawShow.value = value
  // 关闭后清空旧加载函数，避免下次误用上一次的数据源
  if (!value) rawLoader.value = null
}

/** 当前页签的表格总数 */
const activeTotal = computed(() => (tab.value === 'variables' ? variableTotal.value : detailTotal.value))

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
    <PageHeader title="历史变量" description="历史变量实例与变量明细查询；支持查看原始数据与删除单条记录。">
      <template #actions>
        <n-button size="small" quaternary @click="router.push('/history/processes')">历史流程</n-button>
      </template>
    </PageHeader>

    <FilterBar :loading="tab === 'variables' ? variableLoading : detailLoading" @search="search" @reset="reset">
      <div class="filter-bar__item" style="min-width: 220px">
        <span class="filter-bar__label">流程实例 ID</span>
        <n-input v-model:value="filters.processInstanceId" size="small" placeholder="精确匹配" clearable @keyup.enter="search" />
      </div>
      <div class="filter-bar__item" style="min-width: 200px">
        <span class="filter-bar__label">执行 ID</span>
        <n-input v-model:value="filters.executionId" size="small" placeholder="精确匹配" clearable @keyup.enter="search" />
      </div>
      <div class="filter-bar__item" style="min-width: 200px">
        <span class="filter-bar__label">任务 ID</span>
        <n-input v-model:value="filters.taskId" size="small" placeholder="精确匹配" clearable @keyup.enter="search" />
      </div>
      <div class="filter-bar__item">
        <span class="filter-bar__label">变量名</span>
        <n-input v-model:value="filters.name" size="small" placeholder="精确匹配" clearable @keyup.enter="search" />
      </div>
    </FilterBar>

    <div class="row" style="margin-bottom: 10px">
      <n-button-group size="small">
        <n-button
          v-for="item in TABS"
          :key="item.value"
          size="small"
          :type="tab === item.value ? 'primary' : 'default'"
          :secondary="tab !== item.value"
          @click="switchTab(item.value)"
        >
          {{ item.label }}
        </n-button>
      </n-button-group>
      <span class="muted">共 {{ activeTotal }} 条</span>
    </div>

    <ErrorBlock v-if="actionError" :error="actionError" @retry="load" />

    <template v-if="tab === 'variables'">
      <ErrorBlock v-if="variableError" :error="variableError" @retry="loadVariables" />
      <HistoricVariableTable
        v-else
        :variables="variables"
        :loading="variableLoading"
        :deleting-id="deletingId"
        :total="variableTotal"
        :page="page"
        :page-size="pageSize"
        @raw="openVariableRaw"
        @remove="removeVariable"
        @update:page="changePage"
        @update:page-size="changePageSize"
      />
    </template>
    <template v-else>
      <ErrorBlock v-if="detailError" :error="detailError" @retry="loadDetails" />
      <HistoricDetailTable
        v-else
        :details="details"
        :loading="detailLoading"
        :deleting-id="deletingId"
        :total="detailTotal"
        :page="page"
        :page-size="pageSize"
        @raw="openDetailRaw"
        @remove="removeDetail"
        @update:page="changePage"
        @update:page-size="changePageSize"
      />
    </template>

    <HistoricRawValueDialog
      :show="rawShow"
      :title="rawTitle"
      :filename="rawFilename"
      :loader="rawLoader"
      @update:show="closeRaw"
    />
  </div>
</template>
