<script setup lang="ts">
// 实例任务：分页列出运行中任务并跳转任务详情。
import { computed, h, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { NButton } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { listInstanceTasks, type RuntimeTask } from '@/api/runtime'
import { formatDateTime, textOrDash } from '@/lib/format'
import { markAuthError } from '@/stores/ui'
import DataTable from '@/components/DataTable.vue'
import StatusTag from '@/components/StatusTag.vue'

const props = defineProps<{
  /** 实例 ID */
  instanceId: string
}>()

const router = useRouter()
const rows = ref<RuntimeTask[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(50)
const loading = ref(false)
const error = ref<unknown>(null)

/** 表格列 */
const columns = computed<DataTableColumns<RuntimeTask>>(() => [
  { title: '任务名称', key: 'name', minWidth: 180, render: (row) => textOrDash(row.name) },
  { title: '任务定义 Key', key: 'taskDefinitionKey', minWidth: 140, render: (row) => h('span', { class: 'mono' }, textOrDash(row.taskDefinitionKey)) },
  { title: '办理人', key: 'assignee', width: 120, render: (row) => textOrDash(row.assignee) },
  { title: '优先级', key: 'priority', width: 80 },
  {
    title: '状态',
    key: 'delegationState',
    width: 100,
    render: (row) => h(StatusTag, { value: row.delegationState ? 'delegated' : 'active', label: row.delegationState ? '已委派' : '待办理', tone: row.delegationState ? 'info' : 'success' }),
  },
  { title: '创建时间', key: 'createTime', width: 170, render: (row) => formatDateTime(row.createTime) },
  { title: '到期时间', key: 'dueDate', width: 170, render: (row) => formatDateTime(row.dueDate) },
  {
    title: '操作',
    key: 'actions',
    width: 100,
    render: (row) =>
      h(NButton, { size: 'tiny', quaternary: true, onClick: () => void router.push(`/runtime/tasks/${row.id}`) }, { default: () => '任务详情' }),
  },
])

// 加载任务分页。
// @return 无
async function load(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    const result = await listInstanceTasks(props.instanceId, { start: (page.value - 1) * pageSize.value, size: pageSize.value })
    rows.value = result.data ?? []
    total.value = result.total ?? 0
  } catch (cause) {
    error.value = cause
    markAuthError(cause)
  } finally {
    loading.value = false
  }
}

// 切换页码。
// @param value 页码
// @return 无
function changePage(value: number): void {
  page.value = value
  void load()
}

// 切换每页条数。
// @param value 条数
// @return 无
function changePageSize(value: number): void {
  pageSize.value = Math.min(value, 1000)
  page.value = 1
  void load()
}

onMounted(load)
</script>

<template>
  <div class="panel">
    <div class="panel__header">
      <span class="panel__title">运行中任务</span>
      <span class="muted" style="margin-left: auto; font-size: 12px">共 {{ total }} 条</span>
    </div>
    <div class="panel__body">
      <DataTable
        :columns="columns"
        :data="rows"
        :loading="loading"
        :total="total"
        :page="page"
        :page-size="pageSize"
        :error="error"
        row-key="id"
        empty-description="该实例当前没有待办任务，可能已全部完成或停在非用户任务节点。"
        @update:page="changePage"
        @update:page-size="changePageSize"
        @retry="load"
      />
    </div>
  </div>
</template>
