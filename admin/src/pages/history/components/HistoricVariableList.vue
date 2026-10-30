<script setup lang="ts">
// 历史变量列表：名称、类型、作用域、值预览，并按需加载原始数据。
import { computed, h, ref } from 'vue'
import { NButton, NDataTable, NTag } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import type { HistoricVariableValue } from '@/api/history'
import { formatValue } from '@/lib/format'
import EmptyState from '@/components/EmptyState.vue'
import HistoricRawValueDialog from './HistoricRawValueDialog.vue'

const props = withDefaults(
  defineProps<{
    /** 变量集合 */
    variables: HistoricVariableValue[]
    /** 加载态 */
    loading?: boolean
    /** 空态说明 */
    emptyText?: string
    /** 原始数据加载函数，为空时不展示原始入口 */
    rawLoader?: ((variable: HistoricVariableValue) => Promise<Blob>) | null
    /** 是否展示作用域列 */
    showScope?: boolean
  }>(),
  { loading: false, emptyText: '该记录没有历史变量。', rawLoader: null, showScope: true },
)

/** 当前查看原始数据的变量 */
const rawTarget = ref<HistoricVariableValue | null>(null)

// 判断变量是否为二进制类型。
// @param variable 变量
// @return 是否二进制
function isBinary(variable: HistoricVariableValue): boolean {
  const type = (variable.type ?? '').toLowerCase()
  return type.includes('binary') || type.includes('bytes') || type === 'serializable'
}

// 打开展示原始数据的弹窗。
// @param variable 变量
// @return 无
function openRaw(variable: HistoricVariableValue): void {
  rawTarget.value = variable
}

// 加载当前变量的原始数据。
// @return 原始数据
function loadRaw(): Promise<Blob> {
  if (!props.rawLoader || !rawTarget.value) return Promise.reject(new Error('缺少原始数据地址'))
  return props.rawLoader(rawTarget.value)
}

/** 表格列 */
const columns = computed<DataTableColumns<HistoricVariableValue>>(() => {
  const base: DataTableColumns<HistoricVariableValue> = [
    { title: '名称', key: 'name', minWidth: 160, render: (row) => h('span', { class: 'mono' }, row.name) },
    {
      title: '类型',
      key: 'type',
      width: 110,
      render: (row) => h(NTag, { size: 'small', bordered: false }, { default: () => row.type || '推断' }),
    },
    {
      title: '值',
      key: 'value',
      minWidth: 260,
      render: (row) =>
        isBinary(row)
          ? h('span', { class: 'muted' }, '二进制内容，不可直接阅读，可查看原始数据')
          : h('span', { class: 'mono ellipsis', title: formatValue(row.value, 500) }, formatValue(row.value)),
    },
  ]
  if (props.showScope) {
    base.push({
      title: '作用域',
      key: 'scope',
      width: 90,
      render: (row) => (row.scope === 'local' ? '任务局部' : '流程全局'),
    })
  }
  if (props.rawLoader) {
    base.push({
      title: '操作',
      key: 'actions',
      width: 90,
      render: (row) => h(NButton, { size: 'tiny', quaternary: true, onClick: () => openRaw(row) }, { default: () => '原始' }),
    })
  }
  return base
})

/** 行主键：同名变量可能位于不同作用域 */
function rowKey(row: HistoricVariableValue): string {
  return `${row.name}:${row.scope ?? 'global'}`
}
</script>

<template>
  <n-data-table :columns="columns" :data="variables" :loading="loading" :row-key="rowKey" :bordered="false" :single-line="false">
    <template #empty>
      <EmptyState title="暂无变量" :description="emptyText" />
    </template>
  </n-data-table>
  <HistoricRawValueDialog
    :show="rawTarget !== null"
    :title="`变量 ${rawTarget?.name ?? ''} 原始数据`"
    :filename="`${rawTarget?.name ?? 'variable'}.bin`"
    :loader="rawTarget ? loadRaw : null"
    @update:show="(value: boolean) => { if (!value) rawTarget = null }"
  />
</template>
