<script setup lang="ts">
// 历史变量明细表格：变量名、类型、值、记录时间与原始数据、删除入口。
import { computed, h } from 'vue'
import { NButton, NDataTable, NTag } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { RouterLink } from 'vue-router'
import type { HistoricDetail } from '@/api/history'
import { formatDateTime, formatValue, shortId } from '@/lib/format'
import EmptyState from '@/components/EmptyState.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'

const props = withDefaults(
  defineProps<{
    /** 明细集合 */
    details: HistoricDetail[]
    /** 加载态 */
    loading?: boolean
    /** 正在删除的明细 ID */
    deletingId?: string
    /** 总条数 */
    total?: number
    /** 当前页，从 1 开始 */
    page?: number
    /** 每页条数 */
    pageSize?: number
  }>(),
  { loading: false, deletingId: '', total: 0, page: 1, pageSize: 50 },
)

const emit = defineEmits<{
  raw: [HistoricDetail]
  remove: [HistoricDetail]
  'update:page': [number]
  'update:pageSize': [number]
}>()

/** 分页配置 */
const pagination = computed(() => ({
  page: props.page,
  pageSize: props.pageSize,
  itemCount: props.total,
  showSizePicker: true,
  pageSizes: [10, 20, 50, 100, 200, 500, 1000],
  prefix: (info: { itemCount?: number }) => `共 ${info.itemCount ?? 0} 条`,
}))

// 处理页码变化。
// @param value 新页码
// @return 无
function changePage(value: number): void {
  emit('update:page', value)
}

// 处理每页条数变化。
// @param value 新条数
// @return 无
function changePageSize(value: number): void {
  emit('update:pageSize', value)
  emit('update:page', 1)
}

/** 表格列 */
const columns = computed<DataTableColumns<HistoricDetail>>(() => [
  { title: '明细 ID', key: 'id', width: 180, render: (row) => h('span', { class: 'mono', title: row.id }, shortId(row.id, 9, 5)) },
  { title: '变量名', key: 'name', minWidth: 150, render: (row) => h('span', { class: 'mono' }, row.variable?.name ?? '—') },
  {
    title: '类型',
    key: 'type',
    width: 110,
    render: (row) => h(NTag, { size: 'small', bordered: false }, { default: () => row.variable?.type || row.detailType || '推断' }),
  },
  {
    title: '值',
    key: 'value',
    minWidth: 230,
    render: (row) => h('span', { class: 'mono ellipsis', title: formatValue(row.variable?.value, 500) }, formatValue(row.variable?.value)),
  },
  { title: '记录时间', key: 'time', width: 165, render: (row) => formatDateTime(row.time) },
  {
    title: '流程实例',
    key: 'processInstanceId',
    width: 175,
    render: (row) =>
      row.processInstanceId
        ? h(
            RouterLink,
            { to: { name: 'historic-process-detail', params: { id: row.processInstanceId } }, class: 'mono', title: row.processInstanceId },
            { default: () => shortId(row.processInstanceId, 8, 4) },
          )
        : '—',
  },
  { title: '任务 ID', key: 'taskId', width: 160, render: (row) => (row.taskId ? h('span', { class: 'mono' }, shortId(row.taskId, 8, 4)) : '—') },
  { title: '修订', key: 'revision', width: 80, render: (row) => String(row.revision ?? 0) },
  {
    title: '操作',
    key: 'actions',
    width: 130,
    render: (row) =>
      h('div', { class: 'row' }, [
        h(NButton, { size: 'tiny', quaternary: true, onClick: () => emit('raw', row) }, { default: () => '原始' }),
        h(ConfirmButton, {
          label: '删除',
          title: '确认删除该历史明细？',
          description: '仅删除该条变量明细记录，不影响变量实例本身与流程运行数据，操作不可撤销。',
          loading: props.deletingId === row.id,
          onConfirm: () => emit('remove', row),
        }),
      ]),
  },
])
</script>

<template>
  <n-data-table
    remote
    :columns="columns"
    :data="details"
    :loading="loading"
    :row-key="(row: HistoricDetail) => row.id"
    :pagination="pagination"
    :bordered="false"
    :single-line="false"
    :scroll-x="1450"
    @update:page="changePage"
    @update:page-size="changePageSize"
  >
    <template #empty>
      <EmptyState title="暂无历史明细" description="按流程实例、执行、任务或变量名筛选后重试。" />
    </template>
  </n-data-table>
</template>
