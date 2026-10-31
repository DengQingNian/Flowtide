<script setup lang="ts">
// 流程异常页：exception/list 查询（按实例 ID 过滤）+ 标记已处理（exception/delete/{ids}）。
import { h, onMounted, ref } from 'vue'
import { NButton, NInput } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { handleWorkflowExceptions, listWorkflowExceptions } from '@/api/workflow'
import type { KeyedRow, WorkflowException } from '@/api/workflow'
import { formatDateTime, textOrDash } from '@/lib/format'
import PageHeader from '@/components/PageHeader.vue'
import FilterBar from '@/components/FilterBar.vue'
import DataTable from '@/components/DataTable.vue'
import StatusTag from '@/components/StatusTag.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import { pickField, useWorkflowListSource } from './helpers'

/** 流程实例 ID 筛选 */
const processInstanceId = ref('')
/** 当前页 */
const page = ref(1)
/** 每页条数 */
const pageSize = ref(20)
const notice = ref('')
const actionError = ref('')
/** 操作中的异常 ID */
const actingId = ref('')

/** 异常数据源 */
const source = useWorkflowListSource<WorkflowException>(
  () =>
    listWorkflowExceptions({
      processInstanceId: processInstanceId.value.trim() || undefined,
      pageNum: page.value,
      pageSize: pageSize.value,
    }),
  '异常列表接口接线中（后端 501），暂无数据',
)

// handleLabel 把处理状态映射为展示文案。
// @param row 异常行
// @return [标签, 状态值]
function handleLabel(row: Record<string, unknown>): [string, string] {
  const status = pickField<string>(row, 'handleStatus', 'handle_status')
  if (status === 'handled' || status === '1' || status === '已处理') return ['已处理', 'success']
  if (status === 'ignored' || status === '已忽略') return ['已忽略', 'default']
  return ['待处理', 'warning']
}

/** 表格列定义 */
const columns: DataTableColumns<WorkflowException & KeyedRow> = [
  {
    title: '任务标题',
    key: 'title',
    minWidth: 160,
    render: (row) => textOrDash(pickField<string>(row, 'title', 'taskTitle')),
  },
  {
    title: '流程名称',
    key: 'processName',
    minWidth: 130,
    render: (row) => textOrDash(pickField<string>(row, 'processName', 'process_name')),
  },
  {
    title: '实例 ID',
    key: 'processInstanceId',
    width: 150,
    render: (row) =>
      h('span', { class: 'mono' }, textOrDash(pickField<string>(row, 'processInstanceId', 'process_instance_id'))),
  },
  {
    title: '异常类型',
    key: 'exceptionType',
    width: 110,
    render: (row) => textOrDash(pickField<string>(row, 'exceptionType', 'exception_type', 'type')),
  },
  {
    title: '异常信息',
    key: 'errorMessage',
    minWidth: 200,
    render: (row) => h('span', { class: 'mono' }, textOrDash(pickField<string>(row, 'errorMessage', 'error_message', 'message'))),
  },
  {
    title: '发起人',
    key: 'initiatorName',
    width: 100,
    render: (row) => textOrDash(pickField<string>(row, 'initiatorName', 'initiator_name')),
  },
  {
    title: '发生时间',
    key: 'createTime',
    width: 165,
    render: (row) => formatDateTime(pickField<string>(row, 'createTime', 'create_time', 'occurTime')),
  },
  {
    title: '处理状态',
    key: 'handleStatus',
    width: 90,
    render: (row) => {
      const [label, value] = handleLabel(row)
      return h(StatusTag, { value, label })
    },
  },
  {
    title: '操作',
    key: 'actions',
    width: 110,
    render: (row) =>
      h(ConfirmButton, {
        label: '标记已处理',
        danger: false,
        title: '确认将该异常标记为已处理？',
        description: '仅更新处理状态，不影响流程实例运行。',
        loading: actingId.value === row._rid,
        onConfirm: () => markHandled(row),
      }),
  },
]

// search 执行查询并回到第一页。
// @return 无
function search(): void {
  page.value = 1
  void source.load()
}

// reset 重置筛选。
// @return 无
function reset(): void {
  processInstanceId.value = ''
  page.value = 1
  void source.load()
}

// changePage 翻页。
// @param value 新页码
// @return 无
function changePage(value: number): void {
  page.value = value
  void source.load()
}

// changePageSize 调整每页条数。
// @param value 新条数
// @return 无
function changePageSize(value: number): void {
  pageSize.value = value
  page.value = 1
  void source.load()
}

// markHandled 标记单条异常为已处理。
// @param row 异常行
// @return 无
async function markHandled(row: WorkflowException & KeyedRow): Promise<void> {
  actingId.value = row._rid
  actionError.value = ''
  notice.value = ''
  try {
    const result = await handleWorkflowExceptions([row._rid])
    if (result.notImplemented) {
      actionError.value = '异常处理接口接线中（后端 501），请稍后再试'
      return
    }
    if (!result.success) {
      actionError.value = result.errorMessage
      return
    }
    notice.value = '异常已标记为处理'
    await source.load()
  } finally {
    actingId.value = ''
  }
}

onMounted(() => {
  void source.load()
})
</script>

<template>
  <div>
    <PageHeader title="流程异常" description="查询流程运行异常（/workflow/exception 门面）并标记已处理；记录只读，不影响实例运行。">
      <template #actions>
        <n-button size="small" quaternary :loading="source.loading.value" @click="() => source.load()">刷新</n-button>
      </template>
    </PageHeader>

    <div v-if="notice" class="banner" role="status" style="margin: 0 0 10px">{{ notice }}</div>
    <div v-if="actionError" class="banner banner--error" role="alert" style="margin: 0 0 10px">{{ actionError }}</div>
    <div v-if="source.errorMessage.value" class="banner banner--error" role="alert" style="margin: 0 0 10px">
      {{ source.errorMessage.value }}
    </div>

    <FilterBar :loading="source.loading.value" @search="search" @reset="reset">
      <div class="filter-bar__item">
        <span class="filter-bar__label">流程实例 ID</span>
        <n-input v-model:value="processInstanceId" size="small" placeholder="按流程实例 ID 过滤（可空）" clearable @keyup.enter="search" />
      </div>
    </FilterBar>

    <DataTable
      :columns="columns"
      :data="source.rows.value"
      :loading="source.loading.value"
      :total="source.total.value"
      :page="page"
      :page-size="pageSize"
      row-key="_rid"
      :empty-description="source.degraded.value || '暂无流程异常记录。'"
      @update:page="changePage"
      @update:page-size="changePageSize"
    />
  </div>
</template>
