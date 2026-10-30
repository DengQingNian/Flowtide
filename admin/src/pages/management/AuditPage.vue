<script setup lang="ts">
// 审计日志：加载最近审计记录，按方法/路径/状态/主体本地筛选并分页。
import { computed, h, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NButton } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { RefreshCw } from '@lucide/vue'
import PageHeader from '@/components/PageHeader.vue'
import FilterBar from '@/components/FilterBar.vue'
import DataTable from '@/components/DataTable.vue'
import StatusTag from '@/components/StatusTag.vue'
import { markAuthError } from '@/stores/ui'
import { listAuditLogs } from '@/api/management'
import type { AuditEvent } from '@/api/management'
import { filterAuditEvents } from './components/managementUtils'
import { formatDateTime, formatDurationMs, formatNumber, textOrDash } from '@/lib/format'

const route = useRoute()
const router = useRouter()

/** 单次加载上限，服务端审计环单页最大 1000 条 */
const LOAD_SIZE = 1000

/** 筛选条件 */
const filters = reactive({ method: '', path: '', status: '', userId: '' })
/** 已加载事件 */
const events = ref<AuditEvent[]>([])
/** 服务端总数 */
const remoteTotal = ref(0)
/** 当前页 */
const page = ref(1)
/** 每页条数 */
const pageSize = ref(50)
/** 加载态 */
const loading = ref(false)
/** 错误 */
const error = ref<unknown>(null)

/** 过滤后的事件 */
const filtered = computed(() => filterAuditEvents(events.value, filters))
/** 当前页事件 */
const pageEvents = computed(() => filtered.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value))

/** 方法选项 */
const METHOD_OPTIONS = ['', 'GET', 'POST', 'PUT', 'PATCH', 'DELETE']

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
  filters.method = queryText(route.query.method)
  filters.path = queryText(route.query.path)
  filters.status = queryText(route.query.status)
  filters.userId = queryText(route.query.userId)
  page.value = queryInt(route.query.page, 1)
  pageSize.value = Math.min(queryInt(route.query.size, 50), 1000)
}

// 把筛选与分页写回 URL。
// @return 无
function syncQuery(): void {
  const query: Record<string, string> = { page: String(page.value), size: String(pageSize.value) }
  if (filters.method) query.method = filters.method
  if (filters.path.trim()) query.path = filters.path.trim()
  if (filters.status.trim()) query.status = filters.status.trim()
  if (filters.userId.trim()) query.userId = filters.userId.trim()
  void router.replace({ name: 'audit', query })
}

// 加载审计日志。
// @return 无
async function load(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    const result = await listAuditLogs({ start: 0, size: LOAD_SIZE })
    events.value = result.data
    remoteTotal.value = result.total
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
  filters.method = ''
  filters.path = ''
  filters.status = ''
  filters.userId = ''
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
}

// 处理每页条数变化。
// @param value 条数
// @return 无
function changePageSize(value: number): void {
  pageSize.value = value
  page.value = 1
  syncQuery()
}

// 状态码到色调。
// @param status HTTP 状态码
// @return 标签色调
function statusTone(status: number): 'success' | 'warning' | 'danger' | 'info' | 'default' {
  if (status >= 500) return 'danger'
  if (status >= 400) return 'warning'
  if (status >= 300) return 'info'
  if (status >= 200) return 'success'
  return 'default'
}

/** 表格列 */
const columns: DataTableColumns<AuditEvent> = [
  { key: 'time', title: '时间', width: 170, render: (row) => formatDateTime(row.time) },
  { key: 'userId', title: '主体', width: 140, ellipsis: { tooltip: true }, render: (row) => textOrDash(row.userId) },
  { key: 'method', title: '方法', width: 90, render: (row) => h('span', { class: 'mono' }, row.method) },
  { key: 'path', title: '路径', minWidth: 260, ellipsis: { tooltip: true }, render: (row) => h('span', { class: 'mono' }, row.path) },
  { key: 'status', title: '状态', width: 90, render: (row) => h(StatusTag, { value: String(row.status), tone: statusTone(row.status) }) },
  { key: 'durationMillis', title: '耗时', width: 110, render: (row) => formatDurationMs(row.durationMillis) },
  { key: 'tenantId', title: '租户', width: 130, ellipsis: { tooltip: true }, render: (row) => textOrDash(row.tenantId) },
]

onMounted(() => {
  readQuery()
  void load()
})
</script>

<template>
  <div>
    <PageHeader title="审计日志" description="查看最近请求的时间、主体、方法、路径、状态码与耗时。">
      <template #actions>
        <n-button size="small" :loading="loading" @click="load">
          <template #icon><RefreshCw :size="14" /></template>
          刷新
        </n-button>
      </template>
    </PageHeader>

    <FilterBar :loading="loading" @search="search" @reset="reset">
      <label class="filter-bar__item">
        <span class="filter-bar__label">方法</span>
        <select v-model="filters.method" class="native-input">
          <option v-for="item in METHOD_OPTIONS" :key="item" :value="item">{{ item || '全部方法' }}</option>
        </select>
      </label>
      <label class="filter-bar__item">
        <span class="filter-bar__label">路径</span>
        <input v-model="filters.path" class="native-input" type="text" placeholder="包含关键字" />
      </label>
      <label class="filter-bar__item">
        <span class="filter-bar__label">状态码</span>
        <input v-model="filters.status" class="native-input" type="text" inputmode="numeric" placeholder="如 500" />
      </label>
      <label class="filter-bar__item">
        <span class="filter-bar__label">主体</span>
        <input v-model="filters.userId" class="native-input" type="text" placeholder="用户 ID 关键字" />
      </label>
    </FilterBar>

    <p class="muted" style="margin: 0 0 8px; font-size: 12px">
      已加载最近 {{ formatNumber(events.length) }} 条，服务端共 {{ formatNumber(remoteTotal) }} 条；筛选作用于已加载记录。
    </p>

    <DataTable
      :columns="columns"
      :data="pageEvents"
      :loading="loading"
      :total="filtered.length"
      :page="page"
      :page-size="pageSize"
      :page-sizes="[10, 20, 50, 100, 200, 500, 1000]"
      row-key="id"
      :error="error"
      empty-description="没有匹配的审计记录；未启用审计时该列表恒为空。"
      :scroll-x="1100"
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
  min-width: 150px;
}
</style>
