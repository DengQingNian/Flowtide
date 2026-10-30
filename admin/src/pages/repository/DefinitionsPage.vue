<script setup lang="ts">
// 流程定义列表：按 Key 分组展示版本，支持 Key、名称、状态、部署与最新版本筛选。
import { computed, h, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NButton, NInput, NSelect, NTag } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import FilterBar from '@/components/FilterBar.vue'
import DataTable from '@/components/DataTable.vue'
import ErrorBlock from '@/components/ErrorBlock.vue'
import StatusTag from '@/components/StatusTag.vue'
import { REPOSITORY_MAX_SIZE, listAllProcessDefinitions } from '@/api/repository'
import type { ProcessDefinition } from '@/api/repository'
import { textOrDash } from '@/lib/format'
import { markAuthError } from '@/stores/ui'

/** 表格行 */
type DefinitionRow = {
  id: string
  key: string
  name: string
  version: number
  deploymentId: string
  tenantId: string
  suspended: boolean
  latest: boolean
  versionCount: number
}

const route = useRoute()
const router = useRouter()

const definitions = ref<ProcessDefinition[]>([])
const loading = ref(false)
const error = ref<unknown>(null)
const keyword = ref('')
const nameLike = ref('')
const tenantId = ref('')
const deploymentId = ref('')
const suspended = ref<'all' | 'active' | 'suspended'>('all')
const versionMode = ref<'latest' | 'all'>('latest')
const latestVersion = computed(() => versionMode.value === 'latest')
const page = ref(1)
const pageSize = ref(50)

const pageSizes = [20, 50, 100, 200, 500, 1000]
const statusOptions = [
  { label: '全部状态', value: 'all' },
  { label: '激活', value: 'active' },
  { label: '挂起', value: 'suspended' },
]
const versionOptions = [
  { label: '仅最新版本', value: 'latest' },
  { label: '全部版本', value: 'all' },
]

// 读取路由查询值。
// @param value 查询值
// @return 字符串
function queryValue(value: unknown): string {
  if (Array.isArray(value)) return typeof value[0] === 'string' ? value[0] : ''
  return typeof value === 'string' ? value : ''
}

// 从路由查询还原筛选条件。
// @return 无
function syncFromRoute(): void {
  keyword.value = queryValue(route.query.key)
  nameLike.value = queryValue(route.query.nameLike)
  tenantId.value = queryValue(route.query.tenantId)
  deploymentId.value = queryValue(route.query.deploymentId)
  const status = queryValue(route.query.suspended)
  suspended.value = status === 'active' || status === 'suspended' ? status : 'all'
  versionMode.value = queryValue(route.query.latestVersion) === 'false' ? 'all' : 'latest'
  const pageNumber = Number(queryValue(route.query.page))
  page.value = Number.isFinite(pageNumber) && pageNumber > 0 ? Math.floor(pageNumber) : 1
  const sizeNumber = Number(queryValue(route.query.size))
  pageSize.value = Number.isFinite(sizeNumber) && sizeNumber > 0 ? Math.min(sizeNumber, REPOSITORY_MAX_SIZE) : 50
}

// 将筛选与分页写入路由。
// @return 无
async function commitQuery(): Promise<void> {
  const query: Record<string, string> = {}
  if (keyword.value.trim()) query.key = keyword.value.trim()
  if (nameLike.value.trim()) query.nameLike = nameLike.value.trim()
  if (tenantId.value.trim()) query.tenantId = tenantId.value.trim()
  if (deploymentId.value.trim()) query.deploymentId = deploymentId.value.trim()
  if (suspended.value !== 'all') query.suspended = suspended.value
  if (versionMode.value === 'all') query.latestVersion = 'false'
  if (page.value > 1) query.page = String(page.value)
  if (pageSize.value !== 50) query.size = String(pageSize.value)
  await router.replace({ query })
}

const latestByKey = computed<Record<string, ProcessDefinition>>(() => {
  const map: Record<string, ProcessDefinition> = {}
  for (const definition of definitions.value) {
    const current = map[definition.key]
    if (!current || definition.version > current.version) map[definition.key] = definition
  }
  return map
})

const versionCountByKey = computed<Record<string, number>>(() => {
  const map: Record<string, number> = {}
  for (const definition of definitions.value) map[definition.key] = (map[definition.key] ?? 0) + 1
  return map
})

const filtered = computed(() => {
  const keyText = keyword.value.trim()
  const nameText = nameLike.value.trim().toLowerCase()
  const tenantText = tenantId.value.trim()
  const deploymentText = deploymentId.value.trim()
  let list = definitions.value.filter((definition) => {
    // 逐项判断五个筛选条件，空条件不参与过滤
    if (keyText && definition.key !== keyText) return false
    if (nameText && !(definition.name ?? '').toLowerCase().includes(nameText)) return false
    if (tenantText && definition.tenantId !== tenantText) return false
    if (deploymentText && definition.deploymentId !== deploymentText) return false
    if (suspended.value === 'active' && definition.suspended) return false
    if (suspended.value === 'suspended' && !definition.suspended) return false
    return true
  })
  if (latestVersion.value) {
    list = list.filter((definition) => latestByKey.value[definition.key]?.id === definition.id)
  }
  return [...list].sort((left, right) => {
    const keyOrder = left.key.localeCompare(right.key, 'zh-CN')
    return keyOrder !== 0 ? keyOrder : right.version - left.version
  })
})

const total = computed(() => filtered.value.length)

const rows = computed<DefinitionRow[]>(() =>
  filtered.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value).map((definition) => ({
    id: definition.id,
    key: definition.key,
    name: definition.name ?? '',
    version: definition.version,
    deploymentId: definition.deploymentId ?? '',
    tenantId: definition.tenantId ?? '',
    suspended: Boolean(definition.suspended),
    latest: latestByKey.value[definition.key]?.id === definition.id,
    versionCount: versionCountByKey.value[definition.key] ?? 1,
  })),
)

const columns: DataTableColumns<DefinitionRow> = [
  {
    title: 'Key',
    key: 'key',
    minWidth: 220,
    render: (row) =>
      h('div', { class: 'row' }, [
        h('span', { class: 'mono' }, row.key),
        row.latest ? h(NTag, { size: 'tiny', type: 'success', bordered: false }, { default: () => `最新 v${row.version}` }) : null,
        row.versionCount > 1 ? h('span', { class: 'muted', style: 'font-size: 12px' }, `共 ${row.versionCount} 个版本`) : null,
      ]),
  },
  {
    title: '名称',
    key: 'name',
    minWidth: 180,
    render: (row) =>
      h(
        NButton,
        { text: true, type: 'primary', size: 'small', onClick: () => void router.push(`/repository/definitions/${row.id}`) },
        { default: () => textOrDash(row.name) },
      ),
  },
  { title: '版本', key: 'version', width: 80, render: (row) => `v${row.version}` },
  {
    title: '状态',
    key: 'suspended',
    width: 100,
    render: (row) => h(StatusTag, { value: row.suspended ? 'suspended' : 'active', label: row.suspended ? '挂起' : '激活' }),
  },
  {
    title: '部署',
    key: 'deploymentId',
    width: 140,
    render: (row) =>
      row.deploymentId
        ? h(
            NButton,
            { text: true, size: 'small', onClick: () => void router.push(`/repository/deployments/${row.deploymentId}`) },
            { default: () => '查看部署' },
          )
        : '—',
  },
  { title: '租户', key: 'tenantId', width: 120, render: (row) => textOrDash(row.tenantId) },
  {
    title: '操作',
    key: 'actions',
    width: 90,
    render: (row) =>
      h(NButton, { size: 'tiny', quaternary: true, onClick: () => void router.push(`/repository/definitions/${row.id}`) }, { default: () => '详情' }),
  },
]

let loadToken = 0

// 加载流程定义。
// @return 无
async function load(): Promise<void> {
  const token = ++loadToken
  loading.value = true
  error.value = null
  try {
    const list = await listAllProcessDefinitions()
    if (token !== loadToken) return
    definitions.value = list
    clampPage()
  } catch (cause) {
    if (token !== loadToken) return
    markAuthError(cause)
    error.value = cause
  } finally {
    if (token === loadToken) loading.value = false
  }
}

// 将页码限制在有效范围。
// @return 无
function clampPage(): void {
  const maxPage = Math.max(1, Math.ceil(total.value / pageSize.value))
  if (page.value > maxPage) page.value = maxPage
}

// 执行查询并刷新。
// @return 无
async function search(): Promise<void> {
  page.value = 1
  await commitQuery()
  await load()
}

// 重置全部筛选条件。
// @return 无
async function reset(): Promise<void> {
  keyword.value = ''
  nameLike.value = ''
  tenantId.value = ''
  deploymentId.value = ''
  suspended.value = 'all'
  versionMode.value = 'latest'
  page.value = 1
  pageSize.value = 50
  await commitQuery()
  await load()
}

// 更新页码。
// @param value 页码
// @return 无
function changePage(value: number): void {
  page.value = value
  void commitQuery()
}

// 更新每页条数。
// @param value 每页条数
// @return 无
function changePageSize(value: number): void {
  pageSize.value = value
  page.value = 1
  void commitQuery()
}

watch([total, pageSize], () => {
  const before = page.value
  clampPage()
  if (before !== page.value) void commitQuery()
})
watch(
  () => route.query,
  () => syncFromRoute(),
)

onMounted(() => {
  syncFromRoute()
  void load()
})
</script>

<template>
  <div>
    <PageHeader title="流程定义" description="按 Key 分组查看版本、状态与部署；默认只显示最新版本。">
      <template #actions>
        <n-button size="small" quaternary :loading="loading" @click="load">刷新</n-button>
      </template>
    </PageHeader>

    <ErrorBlock v-if="error" :error="error" context="GET /service/repository/process-definitions" @retry="load" />

    <FilterBar :loading="loading" @search="search" @reset="reset">
      <div class="filter-bar__item">
        <span class="filter-bar__label">Key</span>
        <n-input v-model:value="keyword" size="small" clearable placeholder="精确匹配 Key" @keyup.enter="search" />
      </div>
      <div class="filter-bar__item">
        <span class="filter-bar__label">名称包含</span>
        <n-input v-model:value="nameLike" size="small" clearable placeholder="按名称过滤" @keyup.enter="search" />
      </div>
      <div class="filter-bar__item">
        <span class="filter-bar__label">状态</span>
        <n-select v-model:value="suspended" size="small" :options="statusOptions" style="width: 120px" />
      </div>
      <div class="filter-bar__item">
        <span class="filter-bar__label">部署 ID</span>
        <n-input v-model:value="deploymentId" size="small" clearable placeholder="精确匹配部署" @keyup.enter="search" />
      </div>
      <div class="filter-bar__item">
        <span class="filter-bar__label">版本</span>
        <n-select v-model:value="versionMode" size="small" :options="versionOptions" style="width: 130px" />
      </div>
      <div class="filter-bar__item">
        <span class="filter-bar__label">租户</span>
        <n-input v-model:value="tenantId" size="small" clearable placeholder="精确匹配租户" @keyup.enter="search" />
      </div>
    </FilterBar>

    <DataTable
      :columns="columns"
      :data="rows"
      :loading="loading"
      :total="total"
      :page="page"
      :page-size="pageSize"
      :page-sizes="pageSizes"
      row-key="id"
      empty-description="没有匹配的流程定义。调整筛选条件，或先在部署页上传 BPMN 文件。"
      @update:page="changePage"
      @update:page-size="changePageSize"
    />
  </div>
</template>
