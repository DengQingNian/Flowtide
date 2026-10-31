<script setup lang="ts">
// 已办任务页：身份选择器 + doneList 分页表格（含完成时间），只读。
import { h, onMounted, ref } from 'vue'
import { NButton, NInput } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { listDoneTasks } from '@/api/workflow'
import type { KeyedRow, WorkflowTaskRow } from '@/api/workflow'
import { formatDateTime, textOrDash } from '@/lib/format'
import PageHeader from '@/components/PageHeader.vue'
import FilterBar from '@/components/FilterBar.vue'
import DataTable from '@/components/DataTable.vue'
import { pickField, useWorkflowListSource } from './helpers'

/** 当前身份（默认 u1） */
const identityId = ref('u1')
/** 当前页 */
const page = ref(1)
/** 每页条数 */
const pageSize = ref(20)

/** 已办数据源 */
const source = useWorkflowListSource<WorkflowTaskRow>(
  () =>
    listDoneTasks({
      identityId: identityId.value.trim() || 'u1',
      pageNum: page.value,
      pageSize: pageSize.value,
    }),
  '已办列表接口接线中（后端 501），暂无数据',
)

/** 表格列定义 */
const doneColumns: DataTableColumns<WorkflowTaskRow & KeyedRow> = [
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
    title: '发起人',
    key: 'initiatorName',
    width: 110,
    render: (row) => textOrDash(pickField<string>(row, 'initiatorName', 'initiator_name', 'startUserName')),
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
    width: 165,
    render: (row) => formatDateTime(pickField<string>(row, 'createTime', 'create_time', 'startTime')),
  },
  {
    title: '完成时间',
    key: 'endTime',
    width: 165,
    render: (row) => formatDateTime(pickField<string>(row, 'endTime', 'end_time', 'completeTime')),
  },
]

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

onMounted(() => {
  void source.load()
})
</script>

<template>
  <div>
    <PageHeader title="已办任务" description="按身份查询已办任务（/workflow/process/task/all/doneList），并展示完成时间。">
      <template #actions>
        <n-button size="small" quaternary :loading="source.loading.value" @click="() => source.load()">刷新</n-button>
      </template>
    </PageHeader>

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
      :columns="doneColumns"
      :data="source.rows.value"
      :loading="source.loading.value"
      :total="source.total.value"
      :page="page"
      :page-size="pageSize"
      row-key="_rid"
      :empty-description="source.degraded.value || '该身份暂无已办任务。'"
      @update:page="changePage"
      @update:page-size="changePageSize"
    />
  </div>
</template>
