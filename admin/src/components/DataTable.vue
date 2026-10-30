<script setup lang="ts" generic="T extends Record<string, unknown>">
// 通用数据表：远程分页、排序、选择、加载态与空态。
import { computed } from 'vue'
import { NDataTable } from 'naive-ui'
import type { DataTableColumns, DataTableRowKey } from 'naive-ui'
import ErrorBlock from './ErrorBlock.vue'
import EmptyState from './EmptyState.vue'

const props = withDefaults(
  defineProps<{
    /** 列定义 */
    columns: DataTableColumns<T>
    /** 数据 */
    data: T[]
    /** 加载态 */
    loading?: boolean
    /** 总条数 */
    total?: number
    /** 当前页，从 1 开始 */
    page?: number
    /** 每页条数 */
    pageSize?: number
    /** 行主键字段 */
    rowKey?: string
    /** 错误对象 */
    error?: unknown
    /** 空态说明 */
    emptyDescription?: string
    /** 是否可多选 */
    selectable?: boolean
    /** 已选行 */
    checkedRowKeys?: DataTableRowKey[]
    /** 横向滚动宽度 */
    scrollX?: number
    /** 单页条数选项 */
    pageSizes?: number[]
  }>(),
  {
    loading: false,
    total: 0,
    page: 1,
    pageSize: 50,
    rowKey: 'id',
    emptyDescription: '',
    selectable: false,
    checkedRowKeys: () => [],
    scrollX: 0,
    pageSizes: () => [10, 20, 50, 100],
  },
)

const emit = defineEmits<{
  'update:page': [number]
  'update:pageSize': [number]
  'update:checkedRowKeys': [DataTableRowKey[]]
  retry: []
}>()

/** 分页配置 */
const pagination = computed(() => ({
  page: props.page,
  pageSize: props.pageSize,
  itemCount: props.total,
  showSizePicker: true,
  pageSizes: props.pageSizes,
  prefix: (info: { itemCount?: number }) => `共 ${info.itemCount ?? 0} 条`,
}))

// 处理页码变化。
// @param page 页码
// @return 无
function updatePage(page: number): void {
  emit('update:page', page)
}

// 处理每页条数变化。
// @param size 每页条数
// @return 无
function updatePageSize(size: number): void {
  emit('update:pageSize', size)
  emit('update:page', 1)
}

// 生成行主键。
// @param row 行数据
// @return 行主键
function resolveRowKey(row: T): string {
  const value = (row as Record<string, unknown>)[props.rowKey]
  return value === null || value === undefined ? '' : String(value)
}

defineExpose({ updatePage, updatePageSize, updateChecked })

// 处理选择变化。
// @param keys 行主键
// @return 无
function updateChecked(keys: DataTableRowKey[]): void {
  emit('update:checkedRowKeys', keys)
}
</script>

<template>
  <div class="data-table">
    <ErrorBlock v-if="error" :error="error" @retry="emit('retry')" />
    <n-data-table
      remote
      :columns="columns"
      :data="data"
      :loading="loading"
      :row-key="resolveRowKey"
      :pagination="pagination"
      :checked-row-keys="checkedRowKeys"
      :scroll-x="scrollX || undefined"
      :bordered="false"
      :single-line="false"
      @update:page="updatePage"
      @update:page-size="updatePageSize"
      @update:checked-row-keys="updateChecked"
    >
      <template #empty>
        <EmptyState :description="emptyDescription || '调整筛选条件后重试，或稍后再来查看。'" />
      </template>
    </n-data-table>
  </div>
</template>