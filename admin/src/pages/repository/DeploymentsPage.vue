<script setup lang="ts">
// 部署列表：上传、筛选、排序、分页与删除部署。
import { computed, h, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NButton, NInput, NSelect } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import FilterBar from '@/components/FilterBar.vue'
import DataTable from '@/components/DataTable.vue'
import ErrorBlock from '@/components/ErrorBlock.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import DeployUploadDialog from './components/DeployUploadDialog.vue'
import type { DeployUploadPayload } from './components/DeployUploadDialog.vue'
import {
  REPOSITORY_MAX_SIZE,
  createDeployment,
  deleteDeployment,
  listAllProcessDefinitions,
  listDeploymentResources,
  listDeployments,
} from '@/api/repository'
import type { Deployment, ProcessDefinition } from '@/api/repository'
import { formatDateTime, textOrDash, toDate } from '@/lib/format'
import { markAuthError } from '@/stores/ui'

type DeploymentRow = {
  id: string
  name: string
  tenantId: string
  deployTime: string
}
const route = useRoute()
const router = useRouter()
const deployments = ref<Deployment[]>([])
const definitions = ref<ProcessDefinition[]>([])
const resourceCounts = ref<Record<string, number | null>>({})
const loading = ref(false)
const error = ref<unknown>(null)
const actionError = ref<unknown>(null)
const retryDeployment = ref<Deployment | null>(null)
const deletingId = ref('')
const showUpload = ref(false)
const uploading = ref(false)
const uploadError = ref<unknown>(null)
const nameLike = ref('')
const tenantId = ref('')
const sort = ref('deployTime')
const order = ref<'asc' | 'desc'>('desc')
const page = ref(1)
const pageSize = ref(50)
const pageSizes = [20, 50, 100, 200, 500, 1000]
const sortOptions = [{ label: '部署时间', value: 'deployTime' }, { label: '名称', value: 'name' }, { label: 'ID', value: 'id' }]
const orderOptions = [{ label: '倒序', value: 'desc' }, { label: '正序', value: 'asc' }]

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
  nameLike.value = queryValue(route.query.nameLike)
  tenantId.value = queryValue(route.query.tenantId)
  const sortValue = queryValue(route.query.sort)
  sort.value = ['deployTime', 'name', 'id'].includes(sortValue) ? sortValue : 'deployTime'
  order.value = queryValue(route.query.order) === 'asc' ? 'asc' : 'desc'
  const pageNumber = Number(queryValue(route.query.page))
  page.value = Number.isFinite(pageNumber) && pageNumber > 0 ? Math.floor(pageNumber) : 1
  const sizeNumber = Number(queryValue(route.query.size))
  pageSize.value = Number.isFinite(sizeNumber) && sizeNumber > 0 ? Math.min(sizeNumber, REPOSITORY_MAX_SIZE) : 50
}

// 将当前筛选、排序与分页写入路由。
// @return 无
async function commitQuery(): Promise<void> {
  const query: Record<string, string> = {}
  if (nameLike.value.trim()) query.nameLike = nameLike.value.trim()
  if (tenantId.value.trim()) query.tenantId = tenantId.value.trim()
  if (sort.value !== 'deployTime') query.sort = sort.value
  if (order.value !== 'desc') query.order = order.value
  if (page.value > 1) query.page = String(page.value)
  if (pageSize.value !== 50) query.size = String(pageSize.value)
  await router.replace({ query })
}

const filtered = computed(() => {
  const keyword = nameLike.value.trim().toLowerCase()
  const tenant = tenantId.value.trim()
  const list = deployments.value.filter((item) => {
    // 租户与名称均为本地过滤，服务端部署列表只支持租户参数
    if (tenant && item.tenantId !== tenant) return false
    if (keyword && !(item.name ?? '').toLowerCase().includes(keyword)) return false
    return true
  })
  const direction = order.value === 'asc' ? 1 : -1
  return [...list].sort((left, right) => {
    let result = 0
    if (sort.value === 'name') result = (left.name ?? '').localeCompare(right.name ?? '', 'zh-CN')
    else if (sort.value === 'id') result = left.id.localeCompare(right.id)
    else result = (toDate(left.deployTime)?.getTime() ?? 0) - (toDate(right.deployTime)?.getTime() ?? 0)
    return result * direction
  })
})

const total = computed(() => filtered.value.length)
const rows = computed<DeploymentRow[]>(() =>
  filtered.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value).map((item) => ({
    id: item.id,
    name: item.name ?? '',
    tenantId: item.tenantId ?? '',
    deployTime: item.deployTime ?? '',
  })),
)

const deploymentKeys = computed<Record<string, string[]>>(() => {
  const map: Record<string, string[]> = {}
  for (const definition of definitions.value) {
    if (!definition.deploymentId) continue
    const keys = map[definition.deploymentId] ?? (map[definition.deploymentId] = [])
    if (!keys.includes(definition.key)) keys.push(definition.key)
  }
  return map
})

// 返回部署关联的流程定义 Key。
// @param deploymentId 部署 ID
// @return Key 文本
function keysText(deploymentId: string): string {
  return textOrDash((deploymentKeys.value[deploymentId] ?? []).join('、'))
}

// 返回部署资源数展示值。
// @param deploymentId 部署 ID
// @return 资源数或占位
function resourceCountText(deploymentId: string): string {
  const count = resourceCounts.value[deploymentId]
  if (count === undefined) return '…'
  if (count === null) return '—'
  return String(count)
}

const countLoading = new Set<string>()

// 加载当前页部署的资源数量。
// @return 无
async function loadResourceCounts(): Promise<void> {
  const targets = rows.value
    .map((row) => row.id)
    .filter((id) => resourceCounts.value[id] === undefined && !countLoading.has(id))
  for (const id of targets) countLoading.add(id)
  await Promise.all(
    targets.map(async (id) => {
      try {
        const resources = await listDeploymentResources(id)
        resourceCounts.value[id] = resources.length
      } catch {
        resourceCounts.value[id] = null
      } finally {
        countLoading.delete(id)
      }
    }),
  )
}

const columns: DataTableColumns<DeploymentRow> = [
  {
    title: '名称',
    key: 'name',
    minWidth: 180,
    render: (row) =>
      h(
        NButton,
        { text: true, type: 'primary', size: 'small', onClick: () => void router.push(`/repository/deployments/${row.id}`) },
        { default: () => row.name || '未命名部署' },
      ),
  },
  { title: 'Key', key: 'key', minWidth: 180, render: (row) => keysText(row.id) },
  { title: '租户', key: 'tenantId', width: 140, render: (row) => textOrDash(row.tenantId) },
  { title: '部署时间', key: 'deployTime', width: 180, render: (row) => formatDateTime(row.deployTime) },
  { title: '资源数', key: 'resourceCount', width: 90, render: (row) => resourceCountText(row.id) },
  {
    title: '操作',
    key: 'actions',
    width: 170,
    render: (row) =>
      h('div', { class: 'row' }, [
        h(NButton, { size: 'tiny', quaternary: true, onClick: () => void router.push(`/repository/deployments/${row.id}`) }, { default: () => '详情' }),
        h(ConfirmButton, {
          label: '删除',
          title: '删除该部署？',
          description: '关联流程实例仍可运行；部署下的流程定义会从仓库移除，且无法恢复。',
          loading: deletingId.value === row.id,
          onConfirm: () => {
            const target = deployments.value.find((item) => item.id === row.id)
            if (target) void removeDeployment(target)
          },
        }),
      ]),
  },
]

let loadToken = 0

// 加载部署列表与关联流程定义。
// @return 无
async function load(): Promise<void> {
  const token = ++loadToken
  loading.value = true
  error.value = null
  actionError.value = null
  try {
    const [result, definitionList] = await Promise.all([
      listDeployments({ start: 0, size: REPOSITORY_MAX_SIZE }),
      listAllProcessDefinitions().catch(() => [] as ProcessDefinition[]),
    ])
    if (token !== loadToken) return
    deployments.value = result.data ?? []
    definitions.value = definitionList
    clampPage()
    await loadResourceCounts()
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

// 删除部署并在成功后刷新列表。
// @param deployment 部署
// @return 无
async function removeDeployment(deployment: Deployment): Promise<void> {
  deletingId.value = deployment.id
  actionError.value = null
  retryDeployment.value = deployment
  try {
    await deleteDeployment(deployment.id, true)
    resourceCounts.value = {}
    await load()
  } catch (cause) {
    markAuthError(cause)
    actionError.value = cause
  } finally {
    deletingId.value = ''
  }
}

// 重试上一次失败的删除。
// @return 无
function retryDelete(): void {
  if (retryDeployment.value) void removeDeployment(retryDeployment.value)
}

// 上传部署并在成功后跳转详情。
// @param payload 上传载荷
// @return 无
async function upload(payload: DeployUploadPayload): Promise<void> {
  uploading.value = true
  uploadError.value = null
  try {
    const deployment = await createDeployment(payload.files, {
      name: payload.name || undefined,
      tenantId: payload.tenantId || undefined,
      enableDuplicateFiltering: payload.enableDuplicateFiltering,
    })
    showUpload.value = false
    await router.push(`/repository/deployments/${deployment.id}`)
  } catch (cause) {
    markAuthError(cause)
    uploadError.value = cause
  } finally {
    uploading.value = false
  }
}

// 执行查询并刷新服务端数据。
// @return 无
async function search(): Promise<void> {
  page.value = 1
  await commitQuery()
  await load()
}

// 重置筛选条件并刷新。
// @return 无
async function reset(): Promise<void> {
  nameLike.value = ''
  tenantId.value = ''
  sort.value = 'deployTime'
  order.value = 'desc'
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

watch(
  () => rows.value.map((row) => row.id).join('|'),
  () => {
    void loadResourceCounts()
  },
)
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
    <PageHeader title="部署" description="上传 BPMN/ZIP 部署，查看资源与关联流程定义。">
      <template #actions>
        <n-button size="small" quaternary :loading="loading" @click="load">刷新</n-button>
        <n-button size="small" type="primary" @click="showUpload = true">上传部署</n-button>
      </template>
    </PageHeader>

    <ErrorBlock v-if="error" :error="error" context="GET /service/repository/deployments" @retry="load" />
    <ErrorBlock v-if="actionError" :error="actionError" context="DELETE /service/repository/deployments/{id}" @retry="retryDelete" />

    <FilterBar :loading="loading" @search="search" @reset="reset">
      <div class="filter-bar__item">
        <span class="filter-bar__label">名称包含</span>
        <n-input v-model:value="nameLike" size="small" clearable placeholder="按名称过滤" @keyup.enter="search" />
      </div>
      <div class="filter-bar__item">
        <span class="filter-bar__label">租户</span>
        <n-input v-model:value="tenantId" size="small" clearable placeholder="精确匹配租户" @keyup.enter="search" />
      </div>
      <div class="filter-bar__item">
        <span class="filter-bar__label">排序</span>
        <n-select v-model:value="sort" size="small" :options="sortOptions" style="width: 140px" @update:value="search" />
      </div>
      <div class="filter-bar__item">
        <span class="filter-bar__label">方向</span>
        <n-select v-model:value="order" size="small" :options="orderOptions" style="width: 100px" @update:value="search" />
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
      empty-description="还没有部署。点击右上角“上传部署”提交 BPMN/XML/ZIP/BAR 文件。"
      @update:page="changePage"
      @update:page-size="changePageSize"
    />

    <DeployUploadDialog
      v-model:show="showUpload"
      :submitting="uploading"
      :error="uploadError"
      @submit="upload"
    />
  </div>
</template>
