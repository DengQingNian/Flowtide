<script setup lang="ts">
// 表数据浏览：列元数据、分页、排序、横向滚动与单元格查看。
import { computed, h, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NButton } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { AlertTriangle, RefreshCw } from '@lucide/vue'
import PageHeader from '@/components/PageHeader.vue'
import FilterBar from '@/components/FilterBar.vue'
import DataTable from '@/components/DataTable.vue'
import EmptyState from '@/components/EmptyState.vue'
import TableCellViewer from './components/TableCellViewer.vue'
import { markAuthError } from '@/stores/ui'
import { getTableColumns, getTableData } from '@/api/management'
import { isBinaryColumn } from './components/managementUtils'
import type { TableMetaData, TableRow } from '@/api/management'
import { formatNumber, formatValue } from '@/lib/format'

const route = useRoute()
const router = useRouter()

/** 表名 */
const tableName = ref(String(route.params.name ?? ''))
/** 列元数据 */
const meta = ref<TableMetaData | null>(null)
/** 行数据 */
const rows = ref<TableRow[]>([])
/** 总行数 */
const total = ref(0)
/** 当前页 */
const page = ref(1)
/** 每页条数 */
const pageSize = ref(50)
/** 排序列 */
const sortColumn = ref('')
/** 排序方向 */
const order = ref<'asc' | 'desc'>('asc')
/** 加载态 */
const loading = ref(false)
/** 错误 */
const error = ref<unknown>(null)
/** 选中的单元格 */
const selected = ref<{ column: string; type?: string; value: unknown } | null>(null)

/** 二进制列 */
const binaryColumns = computed(() => {
  const data = meta.value
  if (!data) return []
  return data.columnNames.filter((_, index) => isBinaryColumn(data.columnTypes[index]))
})

/** 横向滚动宽度 */
const scrollX = computed(() => Math.max(900, (meta.value?.columnNames.length ?? 0) * 180))

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

// 从 URL 还原分页与排序。
// @return 无
function readQuery(): void {
  page.value = queryInt(route.query.page, 1)
  pageSize.value = Math.min(queryInt(route.query.size, 50), 1000)
  sortColumn.value = queryText(route.query.sort)
  order.value = queryText(route.query.order) === 'desc' ? 'desc' : 'asc'
}

// 把分页与排序写回 URL。
// @return 无
function syncQuery(): void {
  const query: Record<string, string> = { page: String(page.value), size: String(pageSize.value) }
  if (sortColumn.value) {
    query.sort = sortColumn.value
    query.order = order.value
  }
  void router.replace({ name: 'table-data', params: { name: tableName.value }, query })
}

// 加载列元数据与数据。
// @return 无
async function load(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    const columns = await getTableColumns(tableName.value)
    meta.value = columns
    // 路由里的排序列可能来自旧表，非法时回退为默认排序
    if (sortColumn.value && !columns.columnNames.includes(sortColumn.value)) sortColumn.value = ''
    const result = await getTableData(tableName.value, {
      start: (page.value - 1) * pageSize.value,
      size: pageSize.value,
      sortColumn: sortColumn.value || undefined,
      order: order.value,
    })
    rows.value = result.data
    total.value = result.total
  } catch (cause) {
    error.value = cause
    markAuthError(cause)
  } finally {
    loading.value = false
  }
}

// 应用排序。
// @return 无
function applySort(): void {
  page.value = 1
  syncQuery()
  void load()
}

// 重置排序。
// @return 无
function resetSort(): void {
  sortColumn.value = ''
  order.value = 'asc'
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

// 查询列类型。
// @param column 列名
// @return 列类型
function typeOf(column: string): string | undefined {
  const index = meta.value?.columnNames.indexOf(column) ?? -1
  return index >= 0 ? meta.value?.columnTypes[index] : undefined
}

// 打开单元格查看器。
// @param column 列名
// @param value 原始值
// @return 无
function selectCell(column: string, value: unknown): void {
  selected.value = { column, type: typeOf(column), value }
}

/** 表格列 */
const columns = computed<DataTableColumns<TableRow>>(() => {
  const names = meta.value?.columnNames ?? []
  return names.map((name) => ({
    key: name,
    title: name,
    width: 180,
    ellipsis: { tooltip: true },
    render: (row: TableRow) => {
      const value = row[name]
      if (value === null || value === undefined) return '—'
      const binary = isBinaryColumn(typeOf(name))
      const label = binary ? '二进制数据' : formatValue(value, 60)
      return h(
        'button',
        { class: 'cell-button', type: 'button', title: '查看完整值', onClick: () => selectCell(name, value) },
        label,
      )
    },
  }))
})

// 监听表名变化。
watch(
  () => route.params.name,
  (value) => {
    tableName.value = String(value ?? '')
    selected.value = null
    if (!tableName.value) return
    readQuery()
    void load()
  },
  { immediate: true },
)
</script>

<template>
  <div>
    <PageHeader :title="`表数据 · ${tableName}`" :description="`共 ${formatNumber(meta?.columnNames.length ?? 0)} 列、${formatNumber(total)} 行，默认每页 50 行，最大 1000 行。`">
      <template #actions>
        <router-link to="/management/tables" custom v-slot="{ navigate }">
          <n-button size="small" @click="navigate">返回表清单</n-button>
        </router-link>
        <n-button size="small" :loading="loading" @click="load">
          <template #icon><RefreshCw :size="14" /></template>
          刷新
        </n-button>
      </template>
    </PageHeader>

    <FilterBar :loading="loading" @search="applySort" @reset="resetSort">
      <label class="filter-bar__item">
        <span class="filter-bar__label">排序列</span>
        <select v-model="sortColumn" class="native-input">
          <option value="">默认（ID_）</option>
          <option v-for="name in meta?.columnNames ?? []" :key="name" :value="name">{{ name }}</option>
        </select>
      </label>
      <label class="filter-bar__item">
        <span class="filter-bar__label">方向</span>
        <select v-model="order" class="native-input">
          <option value="asc">升序</option>
          <option value="desc">降序</option>
        </select>
      </label>
      <div v-if="binaryColumns.length" class="banner banner--warning" role="note" style="margin: 0">
        <AlertTriangle :size="16" aria-hidden="true" />
        <div class="grow">{{ binaryColumns.length }} 个二进制列（{{ binaryColumns.join('、') }}）仅展示提示文本。</div>
      </div>
    </FilterBar>

    <DataTable
      :columns="columns"
      :data="rows"
      :loading="loading"
      :total="total"
      :page="page"
      :page-size="pageSize"
      :page-sizes="[10, 20, 50, 100, 200, 500, 1000]"
      row-key="ID_"
      :error="error"
      :empty-description="`表 ${tableName} 暂无数据。`"
      :scroll-x="scrollX"
      @update:page="changePage"
      @update:page-size="changePageSize"
      @retry="load"
    />

    <TableCellViewer
      v-if="selected"
      style="margin-top: 14px"
      :column="selected.column"
      :type="selected.type"
      :value="selected.value"
      @close="selected = null"
    />
    <EmptyState v-if="meta && !meta.columnNames.length" title="没有列元数据" description="该表未返回任何列定义。" />
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
  min-width: 160px;
}
:deep(.cell-button) {
  border: 0;
  background: transparent;
  padding: 0;
  color: var(--text-2);
  font: inherit;
  cursor: pointer;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
:deep(.cell-button:hover) { color: var(--text); text-decoration: underline; }
</style>
