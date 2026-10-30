<script setup lang="ts">
// 任务列表：组合筛选、远程分页排序、行内签收/完成与批量操作。
import { computed, h, reactive, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { NAlert, NButton, NInput, NSelect } from 'naive-ui'
import type { DataTableColumns, DataTableRowKey } from 'naive-ui'
import { errorMessage } from '@/api/client'
import { listTaskIdentityLinks, queryTasks, taskAction, type Task, type TaskQuery } from '@/api/task'
import { markAuthError } from '@/stores/ui'
import { formatDateTime, shortId, textOrDash } from '@/lib/format'
import PageHeader from '@/components/PageHeader.vue'
import FilterBar from '@/components/FilterBar.vue'
import DataTable from '@/components/DataTable.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import StatusTag from '@/components/StatusTag.vue'
import CreateTaskDialog from './components/task/CreateTaskDialog.vue'
import BulkUpdateDialog from './components/task/BulkUpdateDialog.vue'
import QuickClaimDialog from './components/task/QuickClaimDialog.vue'

/** 筛选字段 */
const filters = reactive({
  assignee: '',
  candidateUser: '',
  candidateGroup: '',
  processInstanceId: '',
  processDefinitionKey: '',
  nameLike: '',
})

const page = ref(1)
const size = ref(50)
const sort = ref('createTime')
const order = ref<'asc' | 'desc'>('desc')
const tasks = ref<Task[]>([])
const total = ref(0)
const loading = ref(false)
const error = ref<unknown>(null)
const checkedRowKeys = ref<DataTableRowKey[]>([])
const candidateMap = ref<Record<string, string>>({})
const candidateLoading = ref(false)
const actionLoading = ref('')
const actionError = ref('')
const actionFeedback = ref('')
const claimShow = ref(false)
const claimTarget = ref<Task | null>(null)
const createShow = ref(false)
const bulkShow = ref(false)
let lastKey = ''

const route = useRoute()
const router = useRouter()

const sortOptions = [
  { label: '创建时间', value: 'createTime' },
  { label: '到期时间', value: 'dueDate' },
  { label: '优先级', value: 'priority' },
  { label: '名称', value: 'name' },
  { label: '办理人', value: 'assignee' },
  { label: 'ID', value: 'id' },
]
const orderOptions = [
  { label: '降序', value: 'desc' },
  { label: '升序', value: 'asc' },
]
const pageSizes = [10, 20, 50, 100, 200, 500, 1000]

// 读取路由 query 中的字符串值。
// @param value query 值
// @return 字符串
function queryText(value: unknown): string {
  if (Array.isArray(value)) return String(value[0] ?? '')
  return value === null || value === undefined ? '' : String(value)
}

// 构造当前筛选对应的 URL query。
// @return query 对象
function buildQuery(): Record<string, string> {
  const output: Record<string, string> = { size: String(size.value), sort: sort.value, order: order.value }
  if (page.value > 1) output.page = String(page.value)
  for (const [key, value] of Object.entries(filters)) {
    if (value.trim()) output[key] = value.trim()
  }
  return output
}

// 从路由还原筛选与分页。
// @return 无
function syncFromRoute(): void {
  for (const key of Object.keys(filters) as (keyof typeof filters)[]) filters[key] = queryText(route.query[key])
  const parsedPage = Number(queryText(route.query.page))
  const parsedSize = Number(queryText(route.query.size))
  page.value = Number.isFinite(parsedPage) && parsedPage > 0 ? Math.floor(parsedPage) : 1
  // 每页条数限制在 1 到 1000，超出范围回落到默认 50。
  size.value = Number.isFinite(parsedSize) && parsedSize > 0 ? Math.min(1000, Math.floor(parsedSize)) : 50
  sort.value = queryText(route.query.sort) || 'createTime'
  order.value = queryText(route.query.order) === 'asc' ? 'asc' : 'desc'
}

/** 当前查询标识，用于避免重复请求 */
function currentKey(): string {
  return JSON.stringify(buildQuery())
}

// 构造接口查询条件。
// @return 查询条件
function payload(): TaskQuery {
  const output: TaskQuery = { start: (page.value - 1) * size.value, size: size.value, sort: sort.value, order: order.value }
  if (filters.assignee.trim()) output.assignee = filters.assignee.trim()
  if (filters.candidateUser.trim()) output.candidateUser = filters.candidateUser.trim()
  if (filters.candidateGroup.trim()) output.candidateGroup = filters.candidateGroup.trim()
  if (filters.processInstanceId.trim()) output.processInstanceId = filters.processInstanceId.trim()
  if (filters.processDefinitionKey.trim()) output.processDefinitionKey = filters.processDefinitionKey.trim()
  if (filters.nameLike.trim()) output.nameLike = filters.nameLike.trim()
  return output
}

// 加载任务列表与当前页候选人信息。
// @return 无
async function load(): Promise<void> {
  lastKey = currentKey()
  loading.value = true
  error.value = null
  try {
    const result = await queryTasks(payload())
    tasks.value = result.data
    total.value = result.total
    checkedRowKeys.value = []
    await loadCandidates(result.data)
  } catch (err) {
    markAuthError(err)
    error.value = err
    tasks.value = []
    total.value = 0
  } finally {
    loading.value = false
  }
}

// 并行加载当前页任务的候选人/候选组，单个失败不影响列表。
// @param items 当前页任务
// @return 无
async function loadCandidates(items: Task[]): Promise<void> {
  const map: Record<string, string> = {}
  candidateLoading.value = true
  const results = await Promise.all(
    items.map(async (task) => {
      try {
        const links = await listTaskIdentityLinks(task.id)
        const users = links.filter((link) => link.userId).map((link) => `@${link.userId}`)
        const groups = links.filter((link) => link.groupId).map((link) => `#${link.groupId}`)
        return [task.id, [...users, ...groups].join(' ') || '—'] as const
      } catch (err) {
        markAuthError(err)
        return [task.id, '—'] as const
      }
    }),
  )
  for (const [id, text] of results) map[id] = text
  candidateMap.value = map
  candidateLoading.value = false
}

// 提交筛选或分页变化到路由，条件未变化时直接刷新。
// @param patch 需要合并的 query
// @return 无
function commit(patch: Record<string, string> = {}): void {
  const next = { ...buildQuery(), ...patch }
  if (next.page === undefined) delete next.page
  const changed = JSON.stringify(next) !== JSON.stringify(route.query)
  void router.replace({ query: next })
  if (!changed) void load()
}

// 重新查询并回到第一页。
// @return 无
function search(): void {
  page.value = 1
  commit()
}

// 重置筛选条件。
// @return 无
function reset(): void {
  for (const key of Object.keys(filters) as (keyof typeof filters)[]) filters[key] = ''
  page.value = 1
  commit()
}

// 处理页码变化。
// @param value 页码
// @return 无
function changePage(value: number): void {
  page.value = value
  commit({ page: value > 1 ? String(value) : '' })
}

// 处理每页条数变化，回到第一页。
// @param value 条数
// @return 无
function changePageSize(value: number): void {
  size.value = value
  page.value = 1
  commit()
}

// 切换排序字段或方向并回到第一页。
// @param field 字段
// @param direction 方向
// @return 无
function changeSort(field?: string, direction?: 'asc' | 'desc'): void {
  if (field) sort.value = field
  if (direction) order.value = direction
  page.value = 1
  commit()
}

// 打开快速签收弹窗。
// @param task 任务
// @return 无
function openClaim(task: Task): void {
  claimTarget.value = task
  claimShow.value = true
}

// 处理签收成功并刷新列表。
// @return 无
function onClaimed(): void {
  actionFeedback.value = '任务已签收'
  void load()
}

// 行内快速完成任务，不写入额外变量。
// @param task 任务
// @return 无
async function quickComplete(task: Task): Promise<void> {
  actionError.value = ''
  actionFeedback.value = ''
  actionLoading.value = `${task.id}:complete`
  try {
    await taskAction(task.id, { action: 'complete' })
    actionFeedback.value = '任务已完成'
    await load()
  } catch (err) {
    markAuthError(err)
    actionError.value = errorMessage(err)
  } finally {
    actionLoading.value = ''
  }
}

/** 已选任务 */
const selectedTasks = computed(() => tasks.value.filter((task) => checkedRowKeys.value.includes(task.id)))

/** 表格列 */
const columns = computed<DataTableColumns<Task>>(() => [
  {
    title: '名称',
    key: 'name',
    minWidth: 180,
    render: (row) => h(RouterLink, { to: `/runtime/tasks/${row.id}` }, { default: () => row.name || row.id }),
  },
  { title: '办理人', key: 'assignee', width: 130, render: (row) => textOrDash(row.assignee) },
  {
    title: '候选人/组',
    key: 'candidates',
    minWidth: 150,
    render: (row) => candidateMap.value[row.id] ?? (candidateLoading.value ? '加载中…' : '—'),
  },
  {
    title: '流程实例',
    key: 'processInstanceId',
    width: 170,
    render: (row) =>
      row.processInstanceId
        ? h(RouterLink, { to: `/runtime/instances/${row.processInstanceId}` }, { default: () => shortId(row.processInstanceId) })
        : '—',
  },
  { title: '创建时间', key: 'createTime', width: 170, render: (row) => formatDateTime(row.createTime) },
  { title: '到期时间', key: 'dueDate', width: 170, render: (row) => formatDateTime(row.dueDate) },
  { title: '优先级', key: 'priority', width: 80, render: (row) => row.priority },
  {
    title: '状态',
    key: 'status',
    width: 100,
    render: (row) =>
      h(StatusTag, {
        value: row.delegationState === 'pending' ? '委派中' : row.assignee ? (row.claimTime ? '已签收' : '待办理') : '未签收',
        tone: row.delegationState === 'pending' ? 'warning' : row.assignee ? 'info' : 'default',
      }),
  },
  {
    title: '操作',
    key: 'actions',
    width: 210,
    fixed: 'right',
    render: (row) =>
      h('div', { class: 'row' }, [
        h(NButton, { size: 'tiny', quaternary: true, onClick: () => void router.push(`/runtime/tasks/${row.id}`) }, { default: () => '详情' }),
        !row.assignee
          ? h(NButton, { size: 'tiny', quaternary: true, loading: actionLoading.value === `${row.id}:claim`, onClick: () => openClaim(row) }, { default: () => '签收' })
          : null,
        h(ConfirmButton, {
          label: '完成',
          title: '确认完成该任务？',
          description: '将直接完成且不写入表单变量；需要填写表单时请进入详情。',
          danger: false,
          loading: actionLoading.value === `${row.id}:complete`,
          onConfirm: () => void quickComplete(row),
        }),
      ]),
  },
])

watch(
  () => route.query,
  () => {
    syncFromRoute()
    if (currentKey() !== lastKey) void load()
  },
  { immediate: true, deep: true },
)
</script>

<template>
  <div>
    <PageHeader title="任务" description="按办理人、候选人、流程实例与名称筛选运行中任务，支持快速签收与完成。">
      <template #actions>
        <n-button size="small" type="primary" @click="createShow = true">新建独立任务</n-button>
        <n-button size="small" :disabled="selectedTasks.length === 0" @click="bulkShow = true">
          批量操作{{ selectedTasks.length > 0 ? `（${selectedTasks.length}）` : '' }}
        </n-button>
        <n-button size="small" quaternary :loading="loading" @click="load">刷新</n-button>
      </template>
    </PageHeader>

    <n-alert v-if="actionError" type="error" :bordered="false" style="margin-bottom: 12px">{{ actionError }}</n-alert>
    <n-alert v-if="actionFeedback" type="success" :bordered="false" style="margin-bottom: 12px">{{ actionFeedback }}</n-alert>

    <FilterBar :loading="loading" @search="search" @reset="reset">
      <div class="filter-bar__item">
        <span class="filter-bar__label">办理人</span>
        <n-input v-model:value="filters.assignee" size="small" clearable placeholder="assignee" @keyup.enter="search" />
      </div>
      <div class="filter-bar__item">
        <span class="filter-bar__label">候选人</span>
        <n-input v-model:value="filters.candidateUser" size="small" clearable placeholder="candidateUser" @keyup.enter="search" />
      </div>
      <div class="filter-bar__item">
        <span class="filter-bar__label">候选组</span>
        <n-input v-model:value="filters.candidateGroup" size="small" clearable placeholder="candidateGroup" @keyup.enter="search" />
      </div>
      <div class="filter-bar__item">
        <span class="filter-bar__label">流程实例</span>
        <n-input v-model:value="filters.processInstanceId" size="small" clearable placeholder="processInstanceId" @keyup.enter="search" />
      </div>
      <div class="filter-bar__item">
        <span class="filter-bar__label">流程定义 Key</span>
        <n-input v-model:value="filters.processDefinitionKey" size="small" clearable placeholder="processDefinitionKey" @keyup.enter="search" />
      </div>
      <div class="filter-bar__item">
        <span class="filter-bar__label">名称模糊</span>
        <n-input v-model:value="filters.nameLike" size="small" clearable placeholder="nameLike" @keyup.enter="search" />
      </div>
      <div class="filter-bar__item" style="min-width: 130px">
        <span class="filter-bar__label">排序字段</span>
        <n-select :value="sort" size="small" :options="sortOptions" @update:value="(value: string) => changeSort(value)" />
      </div>
      <div class="filter-bar__item" style="min-width: 100px">
        <span class="filter-bar__label">方向</span>
        <n-select :value="order" size="small" :options="orderOptions" @update:value="(value: 'asc' | 'desc') => changeSort(undefined, value)" />
      </div>
    </FilterBar>

    <DataTable
      :columns="columns"
      :data="tasks"
      :loading="loading"
      :total="total"
      :page="page"
      :page-size="size"
      :page-sizes="pageSizes"
      :error="error"
      selectable
      :checked-row-keys="checkedRowKeys"
      :scroll-x="1400"
      empty-description="当前筛选条件下没有运行中任务。"
      @update:page="changePage"
      @update:page-size="changePageSize"
      @update:checked-row-keys="checkedRowKeys = $event"
      @retry="load"
    />

    <QuickClaimDialog v-model:show="claimShow" :task="claimTarget" @done="onClaimed" />
    <CreateTaskDialog v-model:show="createShow" @created="load" />
    <BulkUpdateDialog v-model:show="bulkShow" :tasks="selectedTasks" @done="load" />
  </div>
</template>