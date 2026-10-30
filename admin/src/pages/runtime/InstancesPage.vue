<script setup lang="ts">
// 流程实例列表：筛选、分页、排序、启动与删除。
import { computed, h, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NButton, NCheckbox, NInput, NSelect } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import {
  definitionKeyOf,
  definitionVersionOf,
  deleteProcessInstance,
  listLatestDefinitions,
  listProcessInstances,
  startProcessInstance,
  type ProcessDefinitionSummary,
  type ProcessInstance,
  type StartInstancePayload,
} from '@/api/runtime'
import { errorMessage } from '@/api/client'
import { markAuthError } from '@/stores/ui'
import { formatDateTime, shortId, textOrDash } from '@/lib/format'
import PageHeader from '@/components/PageHeader.vue'
import FilterBar from '@/components/FilterBar.vue'
import DataTable from '@/components/DataTable.vue'
import StatusTag from '@/components/StatusTag.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import StartInstanceDialog from '@/components/StartInstanceDialog.vue'

const route = useRoute()
const router = useRouter()

/** 筛选条件 */
const filters = reactive({
  processDefinitionKey: '',
  businessKey: '',
  tenantId: '',
  onlyActive: false,
  sort: 'startTime',
  order: 'desc' as 'asc' | 'desc',
})

const page = ref(1)
const pageSize = ref(50)
const rows = ref<ProcessInstance[]>([])
const total = ref(0)
const loading = ref(false)
const error = ref<unknown>(null)
const actionError = ref('')
const notice = ref('')
const deletingId = ref('')

const showStart = ref(false)
const startLoading = ref(false)
const submitting = ref(false)
const definitions = ref<ProcessDefinitionSummary[]>([])
const startError = ref('')

const startDefinitions = computed(() => definitions.value.map((item) => ({
  id: item.id,
  key: item.key,
  name: item.name ?? undefined,
  version: item.version,
})))

const sortOptions = [
  { label: '开始时间', value: 'startTime' },
  { label: '定义 ID', value: 'processDefinitionId' },
  { label: '实例 ID', value: 'id' },
  { label: '业务 Key', value: 'businessKey' },
]
const orderOptions = [
  { label: '降序', value: 'desc' },
  { label: '升序', value: 'asc' },
]

let controller: AbortController | null = null

/** 表格列 */
const columns = computed<DataTableColumns<ProcessInstance>>(() => [
  {
    title: '业务 Key',
    key: 'businessKey',
    width: 150,
    render: (row) => h('span', { class: 'mono' }, textOrDash(row.businessKey)),
  },
  {
    title: '流程定义',
    key: 'processDefinitionName',
    minWidth: 220,
    render: (row) =>
      h('div', [
        h('div', row.processDefinitionName || definitionKeyOf(row.processDefinitionId) || '—'),
        h('div', { class: 'muted mono' }, `${definitionKeyOf(row.processDefinitionId)} · v${definitionVersionOf(row.processDefinitionId) ?? '—'}`),
      ]),
  },
  {
    title: '状态',
    key: 'suspended',
    width: 100,
    render: (row) =>
      h(StatusTag, {
        value: row.suspended ? 'suspended' : 'active',
        label: row.suspended ? '已挂起' : '运行中',
        tone: row.suspended ? 'warning' : 'success',
      }),
  },
  { title: '开始时间', key: 'startTime', width: 170, render: (row) => formatDateTime(row.startTime) },
  {
    title: '当前活动',
    key: 'activityId',
    minWidth: 140,
    render: (row) => h('span', { class: 'mono', title: row.activityId ?? '' }, textOrDash(row.activityId)),
  },
  { title: '租户', key: 'tenantId', width: 110, render: (row) => textOrDash(row.tenantId) },
  {
    title: '操作',
    key: 'actions',
    width: 150,
    render: (row) =>
      h('div', { class: 'row' }, [
        h(NButton, { size: 'tiny', quaternary: true, onClick: () => void router.push(`/runtime/instances/${row.id}`) }, { default: () => '详情' }),
        h(ConfirmButton, {
          label: '删除',
          title: '确认删除该流程实例？',
          description: '删除后运行中的执行、任务与变量会被清理，且不可恢复。',
          loading: deletingId.value === row.id,
          onConfirm: () => void remove(row),
        }),
      ]),
  },
])

// 读取路由 Query 还原筛选与分页。
// @return 无
function readQuery(): void {
  const query = route.query
  filters.processDefinitionKey = typeof query.processDefinitionKey === 'string' ? query.processDefinitionKey : ''
  filters.businessKey = typeof query.businessKey === 'string' ? query.businessKey : ''
  filters.tenantId = typeof query.tenantId === 'string' ? query.tenantId : ''
  filters.onlyActive = query.onlyActive === 'true'
  filters.sort = typeof query.sort === 'string' && query.sort ? query.sort : 'startTime'
  filters.order = query.order === 'asc' ? 'asc' : 'desc'
  page.value = Number(query.page) > 0 ? Number(query.page) : 1
  pageSize.value = Number(query.size) > 0 ? Math.min(Number(query.size), 1000) : 50
}

// 把筛选与分页写入路由 Query。
// @return 无
function writeQuery(): void {
  const query: Record<string, string> = {}
  if (filters.processDefinitionKey.trim()) query.processDefinitionKey = filters.processDefinitionKey.trim()
  if (filters.businessKey.trim()) query.businessKey = filters.businessKey.trim()
  if (filters.tenantId.trim()) query.tenantId = filters.tenantId.trim()
  if (filters.onlyActive) query.onlyActive = 'true'
  if (filters.sort !== 'startTime') query.sort = filters.sort
  if (filters.order !== 'desc') query.order = filters.order
  if (page.value > 1) query.page = String(page.value)
  if (pageSize.value !== 50) query.size = String(pageSize.value)
  void router.replace({ path: route.path, query })
}

// 加载实例列表。
// @return 无
async function load(): Promise<void> {
  controller?.abort()
  const current = new AbortController()
  controller = current
  loading.value = true
  error.value = null
  try {
    const result = await listProcessInstances({
      processDefinitionKey: filters.processDefinitionKey.trim() || undefined,
      businessKey: filters.businessKey.trim() || undefined,
      tenantId: filters.tenantId.trim() || undefined,
      onlyActive: filters.onlyActive || undefined,
      start: (page.value - 1) * pageSize.value,
      size: pageSize.value,
      sort: filters.sort,
      order: filters.order,
    }, current.signal)
    rows.value = result.data ?? []
    total.value = result.total ?? 0
  } catch (cause) {
    // 主动取消不展示错误，避免翻页时闪出旧请求的失败提示。
    if (!current.signal.aborted) {
      error.value = cause
      markAuthError(cause)
    }
  } finally {
    if (controller === current) loading.value = false
  }
}

// 执行查询并回到第一页。
// @return 无
function search(): void {
  page.value = 1
  writeQuery()
  void load()
}

// 重置筛选条件。
// @return 无
function reset(): void {
  filters.processDefinitionKey = ''
  filters.businessKey = ''
  filters.tenantId = ''
  filters.onlyActive = false
  filters.sort = 'startTime'
  filters.order = 'desc'
  page.value = 1
  pageSize.value = 50
  writeQuery()
  void load()
}

// 切换页码。
// @param value 新页码
// @return 无
function changePage(value: number): void {
  page.value = value
  writeQuery()
  void load()
}

// 切换每页条数。
// @param value 新条数
// @return 无
function changePageSize(value: number): void {
  pageSize.value = Math.min(value, 1000)
  page.value = 1
  writeQuery()
  void load()
}

// 打开启动实例弹窗并加载最新定义。
// @return 无
async function openStart(): Promise<void> {
  startError.value = ''
  notice.value = ''
  startLoading.value = true
  try {
    definitions.value = await listLatestDefinitions()
    showStart.value = true
  } catch (cause) {
    startError.value = errorMessage(cause)
    markAuthError(cause)
  } finally {
    startLoading.value = false
  }
}

// 提交启动实例请求。
// @param payload 弹窗载荷
// @return 无
async function submitStart(payload: StartInstancePayload): Promise<void> {
  submitting.value = true
  startError.value = ''
  try {
    const created = await startProcessInstance(payload)
    showStart.value = false
    notice.value = `已启动实例 ${shortId(created.id)}`
    await router.push(`/runtime/instances/${created.id}`)
  } catch (cause) {
    startError.value = errorMessage(cause)
    markAuthError(cause)
  } finally {
    submitting.value = false
  }
}

// 删除实例并刷新列表。
// @param row 实例
// @return 无
async function remove(row: ProcessInstance): Promise<void> {
  deletingId.value = row.id
  actionError.value = ''
  notice.value = ''
  try {
    await deleteProcessInstance(row.id, '管理台删除')
    notice.value = `已删除实例 ${shortId(row.id)}`
    await load()
  } catch (cause) {
    actionError.value = errorMessage(cause)
    markAuthError(cause)
  } finally {
    deletingId.value = ''
  }
}

onMounted(() => {
  readQuery()
  writeQuery()
  void load()
})

onUnmounted(() => controller?.abort())
</script>

<template>
  <div>
    <PageHeader title="流程实例" description="查询运行中的实例、查看当前活动，并启动或删除实例。">
      <template #actions>
        <n-button size="small" :loading="startLoading" type="primary" @click="openStart">启动实例</n-button>
        <n-button size="small" quaternary :loading="loading" @click="() => load()">刷新</n-button>
      </template>
    </PageHeader>

    <div v-if="notice" class="text-success" style="margin-bottom: 8px">{{ notice }}</div>
    <div v-if="startError" class="banner banner--error" role="alert" style="margin: 0 0 10px">
      <span class="grow">启动失败：{{ startError }}</span>
      <span class="muted">服务端可能已启用认证或权限不足，请检查认证配置后重试。</span>
    </div>
    <div v-if="actionError" class="banner banner--error" role="alert" style="margin: 0 0 10px">
      <span class="grow">操作失败：{{ actionError }}</span>
    </div>

    <FilterBar :loading="loading" @search="search" @reset="reset">
      <div class="filter-bar__item">
        <span class="filter-bar__label">流程定义 Key</span>
        <n-input v-model:value="filters.processDefinitionKey" size="small" placeholder="例如 leave" clearable @keyup.enter="search" />
      </div>
      <div class="filter-bar__item">
        <span class="filter-bar__label">业务 Key</span>
        <n-input v-model:value="filters.businessKey" size="small" placeholder="业务单据号" clearable @keyup.enter="search" />
      </div>
      <div class="filter-bar__item">
        <span class="filter-bar__label">租户</span>
        <n-input v-model:value="filters.tenantId" size="small" placeholder="租户 ID" clearable @keyup.enter="search" />
      </div>
      <div class="filter-bar__item" style="min-width: 130px">
        <span class="filter-bar__label">排序字段</span>
        <n-select v-model:value="filters.sort" size="small" :options="sortOptions" @update:value="search" />
      </div>
      <div class="filter-bar__item" style="min-width: 100px">
        <span class="filter-bar__label">排序方向</span>
        <n-select v-model:value="filters.order" size="small" :options="orderOptions" @update:value="search" />
      </div>
      <div class="filter-bar__item" style="min-width: 150px; justify-content: flex-end">
        <span class="filter-bar__label">&nbsp;</span>
        <n-checkbox v-model:checked="filters.onlyActive" @update:checked="search">仅未挂起</n-checkbox>
      </div>
    </FilterBar>

    <DataTable
      :columns="columns"
      :data="rows"
      :loading="loading"
      :total="total"
      :page="page"
      :page-size="pageSize"
      :error="error"
      row-key="id"
      empty-description="没有符合条件的运行中实例，可调整筛选条件或启动新实例。"
      @update:page="changePage"
      @update:page-size="changePageSize"
      @retry="() => load()"
    />

    <StartInstanceDialog
      v-model:show="showStart"
      :definitions="startDefinitions"
      :submitting="submitting"
      @submit="submitStart"
    />
  </div>
</template>