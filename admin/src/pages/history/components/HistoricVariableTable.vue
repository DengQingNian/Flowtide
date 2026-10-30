<script setup lang="ts">
// 历史变量实例表格：名称、类型、作用域、值预览，提供原始数据与删除入口。
import { computed, h } from 'vue'
import { NButton, NDataTable, NTag } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { RouterLink } from 'vue-router'
import type { HistoricVariableInstance } from '@/api/history'
import { formatValue, shortId } from '@/lib/format'
import EmptyState from '@/components/EmptyState.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'

const props = withDefaults(
  defineProps<{
    /** 变量集合 */
    variables: HistoricVariableInstance[]
    /** 加载态 */
    loading?: boolean
    /** 正在删除的变量 ID */
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
  raw: [HistoricVariableInstance]
  remove: [HistoricVariableInstance]
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

// 判断变量是否为二进制类型。
// @param type 变量类型
// @return 是否二进制
function isBinary(type?: string | null): boolean {
  const value = (type ?? '').toLowerCase()
  return value.includes('binary') || value.includes('bytes') || value === 'serializable'
}

/** 表格列 */
const columns = computed<DataTableColumns<HistoricVariableInstance>>(() => [
  { title: '变量 ID', key: 'id', width: 180, render: (row) => h('span', { class: 'mono', title: row.id }, shortId(row.id, 9, 5)) },
  { title: '名称', key: 'name', minWidth: 150, render: (row) => h('span', { class: 'mono' }, row.variable.name) },
  {
    title: '类型',
    key: 'type',
    width: 110,
    render: (row) => h(NTag, { size: 'small', bordered: false }, { default: () => row.variable.type || '推断' }),
  },
  { title: '作用域', key: 'scope', width: 100, render: (row) => (row.variable.scope === 'local' ? '任务局部' : '流程全局') },
  {
    title: '值',
    key: 'value',
    minWidth: 230,
    render: (row) =>
      isBinary(row.variable.type)
        ? h('span', { class: 'muted' }, '二进制内容，可查看原始数据')
        : h('span', { class: 'mono ellipsis', title: formatValue(row.variable.value, 500) }, formatValue(row.variable.value)),
  },
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
  {
    title: '操作',
    key: 'actions',
    width: 130,
    render: (row) =>
      h('div', { class: 'row' }, [
        h(NButton, { size: 'tiny', quaternary: true, onClick: () => emit('raw', row) }, { default: () => '原始' }),
        h(ConfirmButton, {
          label: '删除',
          title: '确认删除该历史变量？',
          description: '仅删除该条历史变量记录，不影响流程定义、运行中实例与其他历史数据，操作不可撤销。',
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
    :data="variables"
    :loading="loading"
    :row-key="(row: HistoricVariableInstance) => row.id"
    :pagination="pagination"
    :bordered="false"
    :single-line="false"
    :scroll-x="1350"
    @update:page="changePage"
    @update:page-size="changePageSize"
  >
    <template #empty>
      <EmptyState title="暂无历史变量" description="按流程实例、执行、任务或变量名筛选后重试。" />
    </template>
  </n-data-table>
</template>
