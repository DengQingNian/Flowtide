<script setup lang="ts">
// 实例流程图：定义 XML + 历史/运行活动高亮，并列出活动明细。
import { computed, h, onMounted, ref } from 'vue'
import { NDataTable, NSpin } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import {
  getProcessDefinitionXml,
  listDeadletterJobs,
  listHistoricActivities,
  listRuntimeActivities,
  type ActivityRow,
  type DeadletterJob,
} from '@/api/runtime'
import type { ActivityHighlight } from '@/design/types'
import { formatDateTime, textOrDash } from '@/lib/format'
import { markAuthError } from '@/stores/ui'
import DiagramPanel from '@/components/DiagramPanel.vue'
import ErrorBlock from '@/components/ErrorBlock.vue'
import StatusTag from '@/components/StatusTag.vue'

/** 活动展示状态 */
type ActivityStatus = 'completed' | 'active' | 'failed'

/** 带状态的活动 */
type DisplayActivity = ActivityRow & { status: ActivityStatus }

const props = defineProps<{
  /** 实例 ID */
  instanceId: string
  /** 流程定义 ID */
  definitionId: string
}>()

const xml = ref('')
const activities = ref<DisplayActivity[]>([])
const loading = ref(false)
const error = ref<unknown>(null)

/** 高亮项，供 DiagramPanel 使用 */
const highlights = computed<ActivityHighlight[]>(() =>
  activities.value.map((item) => ({ id: item.activityId, type: item.status })),
)

/** 状态文案 */
const statusLabel: Record<ActivityStatus, string> = {
  completed: '已完成',
  active: '当前',
  failed: '异常',
}

/** 活动表格列 */
const columns: DataTableColumns<DisplayActivity> = [
  { title: '活动 ID', key: 'activityId', minWidth: 150, render: (row) => h('span', { class: 'mono' }, row.activityId) },
  { title: '名称', key: 'activityName', minWidth: 140, render: (row) => textOrDash(row.activityName) },
  { title: '类型', key: 'activityType', width: 120, render: (row) => textOrDash(row.activityType) },
  {
    title: '状态',
    key: 'status',
    width: 90,
    render: (row) => h(StatusTag, { value: row.status, label: statusLabel[row.status], tone: row.status === 'failed' ? 'danger' : row.status === 'active' ? 'info' : 'default' }),
  },
  { title: '开始时间', key: 'startTime', width: 170, render: (row) => formatDateTime(row.startTime) },
  { title: '结束时间', key: 'endTime', width: 170, render: (row) => formatDateTime(row.endTime) },
  { title: '办理人', key: 'assignee', width: 110, render: (row) => textOrDash(row.assignee) },
]

// 返回活动行主键。
// @param row 活动
// @return 活动 ID
function rowKey(row: DisplayActivity): string {
  return row.activityId
}

// 合并活动记录并计算结果状态，同一活动保留最高优先级状态。
// @param historic 历史活动
// @param runtime 运行态活动
// @param jobs 死信作业
// @return 去重后的活动列表
function mergeActivities(historic: ActivityRow[], runtime: ActivityRow[], jobs: DeadletterJob[]): DisplayActivity[] {
  const failed = new Set(
    jobs.filter((job) => job.processInstanceId === props.instanceId && job.activityId).map((job) => job.activityId as string),
  )
  const rank: Record<ActivityStatus, number> = { completed: 1, active: 2, failed: 3 }
  const merged = new Map<string, DisplayActivity>()
  const push = (row: ActivityRow, status: ActivityStatus): void => {
    if (!row.activityId) return
    const next: DisplayActivity = { ...row, status: failed.has(row.activityId) ? 'failed' : status }
    const existing = merged.get(row.activityId)
    // 同一活动可能多次执行，保留状态更高或时间更新的一条。
    if (!existing || rank[next.status] >= rank[existing.status]) merged.set(row.activityId, next)
  }
  historic.forEach((row) => push(row, row.endTime ? 'completed' : 'active'))
  runtime.forEach((row) => push(row, 'active'))
  return [...merged.values()].sort((left, right) => {
    const leftTime = left.startTime ?? ''
    const rightTime = right.startTime ?? ''
    return leftTime.localeCompare(rightTime)
  })
}

// 加载 XML、活动与死信作业。
// @return 无
async function load(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    const [xmlText, historic, runtime, jobs] = await Promise.all([
      getProcessDefinitionXml(props.definitionId),
      listHistoricActivities(props.instanceId),
      listRuntimeActivities(props.instanceId),
      listDeadletterJobs().catch(() => ({ data: [] })),
    ])
    xml.value = xmlText ?? ''
    activities.value = mergeActivities(historic.data ?? [], runtime.data ?? [], jobs.data ?? [])
  } catch (cause) {
    error.value = cause
    markAuthError(cause)
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="stack">
    <ErrorBlock v-if="error" :error="error" @retry="load" />
    <div v-else class="panel">
      <div class="panel__header">
        <span class="panel__title">流程图</span>
        <span class="muted" style="margin-left: auto; font-size: 12px">灰色＝已完成，蓝色＝当前，红色＝异常</span>
      </div>
      <div class="panel__body">
        <n-spin :show="loading">
          <DiagramPanel :xml="xml" :loading="loading" :highlights="highlights" :height="420" />
        </n-spin>
        <p class="muted" style="margin: 8px 0 0; font-size: 12px">
          高亮来源：历史活动与运行态活动；服务端另有图片端点，但本页优先复用定义 XML 以支持节点级高亮。
        </p>
      </div>
    </div>

    <div class="panel">
      <div class="panel__header">
        <span class="panel__title">活动明细</span>
        <span class="muted" style="margin-left: auto; font-size: 12px">共 {{ activities.length }} 个活动</span>
      </div>
      <div class="panel__body">
        <n-data-table
          :columns="columns"
          :data="activities"
          :loading="loading"
          :bordered="false"
          :single-line="false"
          :pagination="false"
          :row-key="rowKey"
        >
          <template #empty>
            <div class="empty-state">
              <div class="empty-state__title">暂无活动记录</div>
              <div>实例尚未进入任何节点，或引擎未开启历史记录。</div>
            </div>
          </template>
        </n-data-table>
      </div>
    </div>
  </div>
</template>