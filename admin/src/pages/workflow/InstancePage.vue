<script setup lang="ts">
// 流程实例页：实例分页（501 容错）+ 业务键状态查询 + 终止/挂起/激活/删除 + 流程图高亮抽屉。
import { computed, h, onMounted, ref } from 'vue'
import { NButton, NDrawer, NDrawerContent, NInput } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import {
  activateWorkflowProcessInstance,
  deleteWorkflowInstance,
  getProcessStatusByBusinessKey,
  getWorkflowHighlight,
  getWorkflowHisModelJson,
  listWorkflowInstances,
  stopWorkflowProcessInstance,
  suspendWorkflowProcessInstance,
} from '@/api/workflow'
import type { HighlightInfo, KeyedRow, ProcessStatusInfo, WorkflowInstanceRow } from '@/api/workflow'
import { formatDateTime, textOrDash } from '@/lib/format'
import PageHeader from '@/components/PageHeader.vue'
import FilterBar from '@/components/FilterBar.vue'
import DataTable from '@/components/DataTable.vue'
import StatusTag from '@/components/StatusTag.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import EmptyState from '@/components/EmptyState.vue'
import DiagramPanel from '@/components/DiagramPanel.vue'
import JsonViewer from '@/components/JsonViewer.vue'
import type { ActivityHighlight } from '@/design/types'
import { actionErrorOf, pickField, useWorkflowListSource } from './helpers'

/** 当前页 */
const page = ref(1)
/** 每页条数 */
const pageSize = ref(20)
/** 业务键筛选 */
const businessKey = ref('')

const notice = ref('')
const actionError = ref('')
/** 操作中的实例 ID */
const actingId = ref('')

/** 业务键查询输入 */
const queryBusinessKey = ref('')
/** 业务键查询中 */
const statusQuerying = ref(false)
/** 业务键查询结果（原始 JSON 展示） */
const statusResult = ref<ProcessStatusInfo | null>(null)
/** 业务键查询错误/降级文案 */
const statusMessage = ref('')
/** 业务键查询是否因 501 降级 */
const statusDegraded = ref(false)

/** 流程图抽屉 */
const showDiagram = ref(false)
/** 抽屉对应的实例行 */
const diagramRow = ref<(WorkflowInstanceRow & KeyedRow) | null>(null)
/** BPMN XML */
const diagramXml = ref('')
/** 高亮活动 */
const diagramHighlights = ref<ActivityHighlight[]>([])
/** 图形加载态 */
const diagramLoading = ref(false)
/** 图形错误/降级文案 */
const diagramMessage = ref('')

/** 实例数据源（501 → 空态提示"实例分页接口接线中"） */
const source = useWorkflowListSource<WorkflowInstanceRow>(
  () =>
    listWorkflowInstances({
      pageNum: page.value,
      pageSize: pageSize.value,
      businessKey: businessKey.value.trim() || undefined,
    }),
  '实例分页接口接线中（后端 501），暂无数据',
)

// statusMeta 把状态码映射为标签文案与色调（0 已结束 / 1 激活 / 2 挂起）。
// @param row 实例行
// @return [文本, 色调值]
function statusMeta(row: Record<string, unknown>): [string, string] {
  const status = pickField<number | string>(row, 'status', 'processStatus', 'state')
  if (status === 0 || status === '0' || status === 'finished' || status === '已结束') return ['已结束', 'default']
  if (status === 2 || status === '2' || status === 'suspended') return ['已挂起', 'warning']
  return ['运行中', 'success']
}

/** 表格列定义 */
const columns = computed<DataTableColumns<WorkflowInstanceRow & KeyedRow>>(() => [
  {
    title: '实例标题',
    key: 'title',
    minWidth: 170,
    render: (row) => textOrDash(pickField<string>(row, 'title', 'processTitle')),
  },
  {
    title: '流程名称',
    key: 'processName',
    minWidth: 140,
    render: (row) => textOrDash(pickField<string>(row, 'processName', 'process_name', 'processDefinitionName')),
  },
  {
    title: '实例 ID',
    key: 'processInstanceId',
    width: 150,
    render: (row) => h('span', { class: 'mono', title: row._rid }, row._rid),
  },
  {
    title: '业务键',
    key: 'businessKey',
    width: 130,
    render: (row) => h('span', { class: 'mono' }, textOrDash(pickField<string>(row, 'businessKey', 'business_key'))),
  },
  {
    title: '发起人',
    key: 'initiatorName',
    width: 100,
    render: (row) => textOrDash(pickField<string>(row, 'initiatorName', 'initiator_name', 'startUserName')),
  },
  {
    title: '状态',
    key: 'status',
    width: 90,
    render: (row) => {
      const [label, value] = statusMeta(row)
      return h(StatusTag, { value, label })
    },
  },
  {
    title: '当前步骤',
    key: 'stepName',
    width: 120,
    render: (row) => textOrDash(pickField<string>(row, 'stepName', 'step_name', 'currentActivityName')),
  },
  {
    title: '发起时间',
    key: 'createTime',
    width: 165,
    render: (row) => formatDateTime(pickField<string>(row, 'createTime', 'create_time', 'startTime')),
  },
  {
    title: '操作',
    key: 'actions',
    width: 300,
    render: (row) => {
      // statusMeta 返回 [文案, 色调]，此处仅需文案判断挂起态
      const [statusLabel] = statusMeta(row)
      return h('div', { class: 'row' }, [
        h(NButton, { size: 'tiny', quaternary: true, onClick: () => openDiagram(row) }, { default: () => '流程图' }),
        h(
          ConfirmButton,
          {
            label: '终止',
            danger: false,
            title: '确认终止该流程实例？',
            description: '终止后实例立即结束，未完成的任务一并取消。',
            loading: actingId.value === row._rid,
            onConfirm: () => act('stop', row),
          },
        ),
        statusLabel === '已挂起'
          ? h(
              ConfirmButton,
              {
                label: '激活',
                danger: false,
                title: '确认激活该流程实例？',
                loading: actingId.value === row._rid,
                onConfirm: () => act('activate', row),
              },
            )
          : h(
              ConfirmButton,
              {
                label: '挂起',
                danger: false,
                title: '确认挂起该流程实例？',
                description: '挂起后实例下的任务暂停流转，可再次激活。',
                loading: actingId.value === row._rid,
                onConfirm: () => act('suspend', row),
              },
            ),
        h(
          ConfirmButton,
          {
            label: '删除',
            title: '确认删除该流程实例？',
            description: '删除后运行中的执行、任务与变量一并清理，且不可恢复。',
            loading: actingId.value === row._rid,
            onConfirm: () => act('delete', row),
          },
        ),
      ])
    },
  },
])

// search 执行查询并回到第一页。
// @return 无
function search(): void {
  page.value = 1
  void source.load()
}

// reset 重置筛选。
// @return 无
function reset(): void {
  businessKey.value = ''
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

// act 执行行操作：终止 / 挂起 / 激活 / 删除。
// @param kind 操作类型
// @param row 实例行
// @return 无
async function act(kind: 'stop' | 'suspend' | 'activate' | 'delete', row: WorkflowInstanceRow & KeyedRow): Promise<void> {
  actingId.value = row._rid
  actionError.value = ''
  notice.value = ''
  try {
    const result =
      kind === 'stop'
        ? await stopWorkflowProcessInstance(row._rid)
        : kind === 'suspend'
          ? await suspendWorkflowProcessInstance(row._rid)
          : kind === 'activate'
            ? await activateWorkflowProcessInstance(row._rid)
            : await deleteWorkflowInstance(row._rid)
    const labelMap = { stop: '终止', suspend: '挂起', activate: '激活', delete: '删除' } as const
    const failure = actionErrorOf(result, `${labelMap[kind]}接口接线中（后端 501）`)
    if (failure) actionError.value = failure
    else {
      notice.value = `实例 ${row._rid} 已${labelMap[kind]}`
      await source.load()
    }
  } finally {
    actingId.value = ''
  }
}

// queryStatus 按业务键查询流程状态（POST /workflow/process/public/processes/id）。
// 501 时展示降级提示，不阻塞页面其他区域。
// @return 无
async function queryStatus(): Promise<void> {
  const key = queryBusinessKey.value.trim()
  if (!key) {
    statusMessage.value = '请输入业务键（业务单据 ID）'
    statusDegraded.value = false
    statusResult.value = null
    return
  }
  statusQuerying.value = true
  statusMessage.value = ''
  try {
    const result = await getProcessStatusByBusinessKey(key)
    if (result.notImplemented) {
      statusResult.value = null
      statusDegraded.value = true
      statusMessage.value = '按业务键查询状态接口接线中（后端 501）'
      return
    }
    statusDegraded.value = false
    if (!result.success) {
      statusResult.value = null
      statusMessage.value = result.errorMessage
      return
    }
    statusResult.value = result.data
    statusMessage.value = ''
  } finally {
    statusQuerying.value = false
  }
}

// openDiagram 打开流程图抽屉并加载高亮数据。
// @param row 实例行
// @return 无
function openDiagram(row: WorkflowInstanceRow & KeyedRow): void {
  diagramRow.value = row
  diagramXml.value = ''
  diagramHighlights.value = []
  diagramMessage.value = ''
  diagramLoading.value = true
  showDiagram.value = true
  void loadDiagram(row._rid)
}

// loadDiagram 拉取高亮信息与实例态 BPMN XML；全部 501 时降级空态。
// @param pid 流程实例 ID
// @return 无
async function loadDiagram(pid: string): Promise<void> {
  diagramLoading.value = true
  diagramMessage.value = ''
  try {
    const highlight = await getWorkflowHighlight(pid)
    if (highlight.notImplemented) {
      diagramMessage.value = '流程图高亮接口接线中（后端 501）'
      return
    }
    if (!highlight.success) {
      diagramMessage.value = highlight.errorMessage
      return
    }
    const info = (highlight.data ?? {}) as HighlightInfo
    diagramHighlights.value = mergeHighlights(info)
    // 高亮数据未携带 XML 时再尝试实例态 XML 门面
    const xml = pickField<string>(info, 'bpmnXml', 'xml', 'modelJson', 'svgXml')
    if (xml) {
      diagramXml.value = xml
      return
    }
    const hisXml = await getWorkflowHisModelJson({ processInstanceId: pid })
    if (hisXml.notImplemented) {
      diagramMessage.value = '实例图形 XML 接口接线中（后端 501），无法渲染流程图'
      return
    }
    if (!hisXml.success) {
      diagramMessage.value = hisXml.errorMessage
      return
    }
    diagramXml.value = typeof hisXml.data === 'string' ? hisXml.data : pickField<string>(hisXml.data, 'bpmnXml', 'xml') ?? ''
    if (!diagramXml.value) diagramMessage.value = '该实例缺少 BPMN XML，无法渲染流程图'
  } finally {
    diagramLoading.value = false
  }
}

// mergeHighlights 把高亮数据归一为 DiagramPanel 需要的活动高亮数组。
// 兼容 completed/completedActivityIds 与 active/activeActivityIds/currentActivityIds 等字段。
// @param info 高亮数据
// @return 活动高亮数组
function mergeHighlights(info: HighlightInfo): ActivityHighlight[] {
  const highlights: ActivityHighlight[] = []
  const completed = pickField<string[]>(info, 'completed', 'completedActivityIds', 'completedNodes') ?? []
  const active = pickField<string[]>(info, 'active', 'activeActivityIds', 'currentActivityIds', 'currentNodes') ?? []
  for (const id of completed) highlights.push({ id, type: 'completed' })
  for (const id of active) highlights.push({ id, type: 'active' })
  return highlights
}

/** 抽屉标题 */
const diagramTitle = computed(() => `流程图 · ${diagramRow.value?._rid ?? ''}`)

onMounted(() => {
  void source.load()
})
</script>

<template>
  <div>
    <PageHeader
      title="流程实例"
      description="查询二次封装门面下的流程实例；列表分页与图形高亮依赖的端点接线中时自动降级为空态提示。"
    >
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
        <span class="filter-bar__label">业务键</span>
        <n-input v-model:value="businessKey" size="small" placeholder="业务单据 ID" clearable @keyup.enter="search" />
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
      :empty-description="source.degraded.value || '暂无流程实例。'"
      @update:page="changePage"
      @update:page-size="changePageSize"
    />

    <div class="panel" style="margin-top: 16px">
      <div class="panel__body">
        <h2 class="section-title">按业务键查询状态</h2>
      <p class="muted" style="margin: 0 0 10px">
        调用 POST /workflow/process/public/processes/id 查询业务单据对应的实例状态（0 已结束 / 1 激活 / 2 挂起）。
      </p>
      <div class="row" style="margin-bottom: 10px">
        <n-input v-model:value="queryBusinessKey" size="small" placeholder="业务单据 ID，如 ORDER-2026-001" style="max-width: 320px" @keyup.enter="queryStatus" />
        <n-button size="small" type="primary" :loading="statusQuerying" @click="queryStatus">查询状态</n-button>
      </div>
      <div v-if="statusMessage" class="banner" :class="{ 'banner--error': !statusDegraded }" role="status" style="margin: 0 0 10px">
        {{ statusMessage }}
      </div>
      <JsonViewer v-if="statusResult" :value="statusResult" title="查询结果" />
      </div>
    </div>

    <n-drawer v-model:show="showDiagram" :width="720">
      <n-drawer-content :title="diagramTitle" closable>
        <DiagramPanel
          v-if="!diagramMessage"
          :xml="diagramXml"
          :loading="diagramLoading"
          :highlights="diagramHighlights"
          :height="480"
        />
        <div v-else style="padding: 24px 0">
          <EmptyState title="流程图暂不可用" :description="diagramMessage" />
        </div>
      </n-drawer-content>
    </n-drawer>
  </div>
</template>
