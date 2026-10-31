<script setup lang="ts">
// 待办任务页：身份选择器 + todoList 分页表格 + 办理（complete/task：意见 + 变量 JSON）。
import { h, onMounted, ref } from 'vue'
import { NButton, NInput } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { completeWorkflowTask, listTodoTasks } from '@/api/workflow'
import type { KeyedRow, WorkflowTaskRow } from '@/api/workflow'
import { formatDateTime, textOrDash } from '@/lib/format'
import PageHeader from '@/components/PageHeader.vue'
import FilterBar from '@/components/FilterBar.vue'
import DataTable from '@/components/DataTable.vue'
import ApprovalDialog from './components/ApprovalDialog.vue'
import type { ApprovalSubmitPayload } from './components/ApprovalDialog.vue'
import { pickField, useWorkflowListSource } from './helpers'

/** 当前身份（默认 u1，输入后查询） */
const identityId = ref('u1')
/** 当前页 */
const page = ref(1)
/** 每页条数 */
const pageSize = ref(20)

/** 办理弹窗状态 */
const showApproval = ref(false)
const submitting = ref(false)
const approvalError = ref('')
/** 办理中的任务行 */
const actingTask = ref<(WorkflowTaskRow & KeyedRow) | null>(null)
const notice = ref('')

/** 待办数据源 */
const source = useWorkflowListSource<WorkflowTaskRow>(
  () =>
    listTodoTasks({
      identityId: identityId.value.trim() || 'u1',
      pageNum: page.value,
      pageSize: pageSize.value,
    }),
  '待办列表接口接线中（后端 501），暂无数据',
)

/** 表格列定义 */
const todoColumns: DataTableColumns<WorkflowTaskRow & KeyedRow> = [
  {
    title: '任务标题',
    key: 'title',
    minWidth: 180,
    render: (row) => textOrDash(pickField<string>(row, 'title', 'taskName', 'task_name')),
  },
  {
    title: '流程名称',
    key: 'processName',
    minWidth: 140,
    render: (row) => textOrDash(pickField<string>(row, 'processName', 'process_name', 'processDefinitionName')),
  },
  {
    title: '步骤名称',
    key: 'stepName',
    width: 130,
    render: (row) => textOrDash(pickField<string>(row, 'stepName', 'step_name', 'activityName')),
  },
  {
    title: '发送人',
    key: 'senderName',
    width: 110,
    render: (row) => textOrDash(pickField<string>(row, 'senderName', 'sender_name', 'sendUserName')),
  },
  {
    title: '业务单据',
    key: 'businessKey',
    width: 130,
    render: (row) => h('span', { class: 'mono' }, textOrDash(pickField<string>(row, 'businessKey', 'business_key'))),
  },
  {
    title: '下发时间',
    key: 'createTime',
    width: 170,
    render: (row) => formatDateTime(pickField<string>(row, 'createTime', 'create_time', 'startTime')),
  },
  {
    title: '操作',
    key: 'actions',
    width: 90,
    render: (row) =>
      h(NButton, { size: 'tiny', type: 'primary', secondary: true, onClick: () => openApproval(row) }, { default: () => '办理' }),
  },
]

// openApproval 打开办理弹窗。
// @param row 待办行
// @return 无
function openApproval(row: WorkflowTaskRow & KeyedRow): void {
  actingTask.value = row
  approvalError.value = ''
  showApproval.value = true
}

// search 执行查询并回到第一页。
// @return 无
function search(): void {
  page.value = 1
  void source.load()
}

// reset 重置身份为默认值。
// @return 无
function reset(): void {
  identityId.value = 'u1'
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

// submitApproval 提交办结任务（POST /workflow/process/complete/task）。
// @param payload 弹窗载荷（意见 + 变量）
// @return 无
async function submitApproval(payload: ApprovalSubmitPayload): Promise<void> {
  const task = actingTask.value
  if (!task) return
  submitting.value = true
  approvalError.value = ''
  try {
    const result = await completeWorkflowTask({
      taskId: task._rid,
      userCode: identityId.value.trim() || 'u1',
      message: payload.message,
      processInstanceId: pickField<string>(task, 'processInstanceId', 'process_instance_id') ?? '',
      type: 'SP',
      variables: payload.variables,
    })
    if (result.notImplemented) {
      approvalError.value = '办理接口接线中（后端 501），请稍后再试'
      return
    }
    if (!result.success) {
      approvalError.value = result.errorMessage
      return
    }
    showApproval.value = false
    notice.value = `任务 ${textOrDash(pickField<string>(task, 'title', 'taskName'))} 已办结`
    await source.load()
  } finally {
    submitting.value = false
  }
}

onMounted(() => {
  void source.load()
})
</script>

<template>
  <div>
    <PageHeader title="待办任务" description="按身份查询待办任务（/workflow/process/task/all/todoList），并可填写意见与变量办理。">
      <template #actions>
        <n-button size="small" quaternary :loading="source.loading.value" @click="() => source.load()">刷新</n-button>
      </template>
    </PageHeader>

    <div v-if="notice" class="banner" role="status" style="margin: 0 0 10px">{{ notice }}</div>
    <div v-if="source.errorMessage.value" class="banner banner--error" role="alert" style="margin: 0 0 10px">
      {{ source.errorMessage.value }}
    </div>

    <FilterBar :loading="source.loading.value" @search="search" @reset="reset">
      <div class="filter-bar__item">
        <span class="filter-bar__label">身份 ID</span>
        <n-input v-model:value="identityId" size="small" placeholder="处理人身份 Id，默认 u1" clearable @keyup.enter="search" />
      </div>
    </FilterBar>

    <DataTable
      :columns="todoColumns"
      :data="source.rows.value"
      :loading="source.loading.value"
      :total="source.total.value"
      :page="page"
      :page-size="pageSize"
      row-key="_rid"
      :empty-description="source.degraded.value || '该身份暂无待办任务。'"
      @update:page="changePage"
      @update:page-size="changePageSize"
    />

    <ApprovalDialog
      v-model:show="showApproval"
      :title="`办理 · ${textOrDash(actingTask ? pickField<string>(actingTask, 'title', 'taskName') : '')}`"
      :submitting="submitting"
      :error="approvalError"
      @submit="submitApproval"
    />
  </div>
</template>
