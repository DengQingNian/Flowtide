<script setup lang="ts">
// 变量列表：名称、类型、值预览与操作入口，二进制值提示不可读。
import { computed, h } from 'vue'
import { NButton, NDataTable, NTag } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import type { VariableInstance } from '@/api/types'
import { formatValue, shortId } from '@/lib/format'
import EmptyState from './EmptyState.vue'

const props = withDefaults(
  defineProps<{
    /** 变量列表 */
    variables: VariableInstance[]
    /** 加载态 */
    loading?: boolean
    /** 是否显示操作列 */
    actions?: boolean
    /** 是否显示作用域列 */
    showScope?: boolean
  }>(),
  { loading: false, actions: true, showScope: false },
)

const emit = defineEmits<{
  edit: [VariableInstance]
  remove: [VariableInstance]
  raw: [VariableInstance]
}>()

// 判断变量是否为二进制类型。
// @param variable 变量
// @return 是否二进制
function isBinary(variable: VariableInstance): boolean {
  const type = (variable.type ?? '').toLowerCase()
  return type.includes('binary') || type.includes('bytes') || type === 'serializable'
}

/** 表格列 */
const columns = computed<DataTableColumns<VariableInstance>>(() => {
  const base: DataTableColumns<VariableInstance> = [
    {
      title: '名称',
      key: 'name',
      minWidth: 160,
      render: (row) => h('span', { class: 'mono' }, row.name),
    },
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
      render: (row) => (isBinary(row) ? h('span', { class: 'muted' }, '二进制内容，不可直接阅读，可查看原始数据') : h('span', { class: 'mono ellipsis', title: formatValue(row.value, 500) }, formatValue(row.value))),
    },
  ]
  if (props.showScope) {
    base.push({ title: '作用域', key: 'scope', width: 100, render: (row) => row.scope || 'global' })
  }
  if (props.actions) {
    base.push({
      title: '操作',
      key: 'actions',
      width: 160,
      render: (row) =>
        h('div', { class: 'row' }, [
          h(NButton, { size: 'tiny', quaternary: true, onClick: () => emit('edit', row) }, { default: () => '编辑' }),
          h(NButton, { size: 'tiny', quaternary: true, disabled: !isBinary(row), onClick: () => emit('raw', row) }, { default: () => '原始' }),
          h(NButton, { size: 'tiny', quaternary: true, type: 'error', onClick: () => emit('remove', row) }, { default: () => '删除' }),
        ]),
    })
  }
  return base
})

/** 行主键：同名变量可能出现在不同作用域 */
function rowKey(row: VariableInstance): string {
  return `${row.name}:${row.scope ?? 'global'}:${shortId(row.id ?? '', 6, 4)}`
}
</script>

<template>
  <n-data-table
    :columns="columns"
    :data="variables"
    :loading="loading"
    :row-key="rowKey"
    :bordered="false"
    :single-line="false"
  >
    <template #empty>
      <EmptyState title="暂无变量" description="启动实例或办理任务时写入的变量会显示在这里。" />
    </template>
  </n-data-table>
</template>
