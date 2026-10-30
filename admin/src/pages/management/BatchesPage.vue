<script setup lang="ts">
// 批处理列表：按类型、状态、租户筛选，展示分片进度并远程分页。
import { h, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter, RouterLink } from 'vue-router'
import { NButton } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { RefreshCw } from '@lucide/vue'
import PageHeader from '@/components/PageHeader.vue'
import FilterBar from '@/components/FilterBar.vue'
import DataTable from '@/components/DataTable.vue'
import StatusTag from '@/components/StatusTag.vue'
import { markAuthError } from '@/stores/ui'
import { listBatches } from '@/api/management'
import type { Batch } from '@/api/management'
import { formatDateTime, formatNumber, shortId, textOrDash } from '@/lib/format'

const route = useRoute()
const router = useRouter()

/** 筛选条件 */
const filters = reactive({ type: '', status: '', tenantId: '' })
/** 批次数据 */
const batches = ref<Batch[]>([])
/** 总条数 */
const total = ref(0)
/** 当前页 */
const page = ref(1)
/** 每页条数 */
const pageSize = ref(50)
/** 加载态 */
const loading = ref(false)
/** 错误 */
const error = ref<unknown>(null)

/** 状态选项 */
const STATUS_OPTIONS = [
  { value: '', label: '全部状态' },
  { value: 'in-progress', label: '执行中' },
  { value: 'completed', label: '已完成' },
  { value: 'failed', label: '失败' },
]

// 读取路由查询中的文本值。
// @param value 查询值
// @return 文本
function queryText(value: unknown): string {
  if (typeof value === 'string') return value
  if (Array.isArray(value) && typeof value[0] === 'string') return value[0]
  return ''
}

// 读取路由查询中的正整数。
// @param value 查询值
// @param fallback 默认值
// @return 正整数
function queryInt(value: unknown, fallback: number): number {
  const parsed = Number.parseInt(queryText(value), 10)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback
}

// 从 URL 还原筛选与分页。
// @return 无
function readQuery(): void {
  filters.type = queryText(route.query.type)
  filters.status = queryText(route.query.status)
  filters.tenantId = queryText(route.query.tenantId)
  page.value = queryInt(route.query.page, 1)
  pageSize.value = Math.min(queryInt(route.query.size, 50), 1000)
}

// 把筛选与分页写回 URL。
// @return 无
function syncQuery(): void {
  const query: Record<string, string> = { page: String(page.value), size: String(pageSize.value) }
  if (filters.type.trim()) query.type = filters.type.trim()
  if (filters.status) query.status = filters.status
  if (filters.tenantId.trim()) query.tenantId = filters.tenantId.trim()
  void router.replace({ name: 'batches', query })
}

// 加载批次列表。
// @return 无
async function load(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    const result = await listBatches({
      type: filters.type.trim() || undefined,
      status: filters.status || undefined,
      tenantId: filters.tenantId.trim() || undefined,
      start: (page.value - 1) * pageSize.value,
      size: pageSize.value,
    })
    batches.value = result.data
    total.value = result.total
  } catch (cause) {
    error.value = cause
    markAuthError(cause)
  } finally {
    loading.value = false
  }
}

// 提交筛选。
// @return 无
function search(): void {
  page.value = 1
  syncQuery()
  void load()
}

// 重置筛选。
// @return 无
function reset(): void {
  filters.type = ''
  filters.status = ''
  filters.tenantId = ''
  page.value = 1
  syncQuery()
  void load()
}

// 处理页码变化。
// @param value 页码
// @return 无
function changePage(value: number): void {
  page.value = value
  syncQuery()
  void load()
}

// 处理每页条数变化。
// @param value 条数
// @return 无
function changePageSize(value: number): void {
  pageSize.value = value
  page.value = 1
  syncQuery()
  void load()
}

/** 表格列 */
const columns: DataTableColumns<Batch> = [
  {
    key: 'id',
    title: '批次 ID',
    width: 180,
    render: (row) => h(RouterLink, { to: `/management/batches/${row.id}`, class: 'mono' }, { default: () => shortId(row.id) }),
  },
  { key: 'type', title: '类型', width: 220, ellipsis: { tooltip: true }, render: (row) => textOrDash(row.type) },
  { key: 'status', title: '状态', width: 110, render: (row) => h(StatusTag, { value: row.status }) },
  { key: 'startTime', title: '创建时间', width: 170, render: (row) => formatDateTime(row.startTime) },
  { key: 'total', title: '分片数', width: 90, render: (row) => formatNumber(row.total) },
  { key: 'completed', title: '完成数', width: 90, render: (row) => formatNumber(row.completed) },
  {
    key: 'progress',
    title: '进度',
    width: 110,
    render: (row) => `${Math.min(100, Math.round((row.completed / Math.max(1, row.total)) * 100))}%`,
  },
  { key: 'endTime', title: '结束时间', width: 170, render: (row) => formatDateTime(row.endTime) },
  { key: 'tenantId', title: '租户', width: 130, ellipsis: { tooltip: true }, render: (row) => textOrDash(row.tenantId) },
]

onMounted(() => {
  readQuery()
  void load()
})
</script>

<template>
  <div>
    <PageHeader title="批处理" description="查看迁移等批处理作业的类型、状态与分片完成情况。">
      <template #actions>
        <n-button size="small" :loading="loading" @click="load">
          <template #icon><RefreshCw :size="14" /></template>
          刷新
        </n-button>
      </template>
    </PageHeader>

    <FilterBar :loading="loading" @search="search" @reset="reset">
      <label class="filter-bar__item">
        <span class="filter-bar__label">类型</span>
        <input v-model="filters.type" class="native-input" type="text" placeholder="如 process-instance-migration" />
      </label>
      <label class="filter-bar__item">
        <span class="filter-bar__label">状态</span>
        <select v-model="filters.status" class="native-input">
          <option v-for="item in STATUS_OPTIONS" :key="item.value" :value="item.value">{{ item.label }}</option>
        </select>
      </label>
      <label class="filter-bar__item">
        <span class="filter-bar__label">租户</span>
        <input v-model="filters.tenantId" class="native-input" type="text" placeholder="租户 ID" />
      </label>
    </FilterBar>

    <DataTable
      :columns="columns"
      :data="batches"
      :loading="loading"
      :total="total"
      :page="page"
      :page-size="pageSize"
      :page-sizes="[10, 20, 50, 100, 200, 500, 1000]"
      row-key="id"
      :error="error"
      empty-description="没有匹配的批处理作业，调整筛选条件后重试。"
      :scroll-x="1300"
      @update:page="changePage"
      @update:page-size="changePageSize"
      @retry="load"
    />
  </div>
</template>

<style scoped>
.native-input {
  height: 30px;
  padding: 0 8px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: var(--surface);
  color: var(--text);
  font-size: 13px;
  min-width: 180px;
}
</style>
