<script setup lang="ts">
// 历史流程实例详情：基本信息、变量、评论、身份链接与级联删除。
import { computed, h, onMounted, ref } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { NButton, NButtonGroup, NDataTable, NSpin } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import {
  deleteHistoricProcessInstance,
  getHistoricProcessInstance,
  getHistoricProcessVariableData,
  listHistoricProcessComments,
  listHistoricProcessIdentityLinks,
  listHistoricProcessVariables,
} from '@/api/history'
import type {
  HistoricComment,
  HistoricIdentityLink,
  HistoricProcessInstance,
  HistoricVariableRecord,
  HistoricVariableValue,
} from '@/api/history'
import { isAuthError } from '@/api/client'
import { markAuthError } from '@/stores/ui'
import { formatDateTime, formatDurationMs, shortId, textOrDash } from '@/lib/format'
import PageHeader from '@/components/PageHeader.vue'
import ErrorBlock from '@/components/ErrorBlock.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import StatusTag from '@/components/StatusTag.vue'
import CopyText from '@/components/CopyText.vue'
import HistoricVariableList from './components/HistoricVariableList.vue'

/** 详情页签 */
type TabKey = 'variables' | 'comments' | 'links'

/** 页签定义 */
const TABS: { value: TabKey; label: string }[] = [
  { value: 'variables', label: '变量' },
  { value: 'comments', label: '评论' },
  { value: 'links', label: '身份链接' },
]

const route = useRoute()
const router = useRouter()

/** 实例 ID */
const id = computed(() => String(route.params.id ?? ''))
/** 实例详情 */
const detail = ref<HistoricProcessInstance | null>(null)
/** 详情加载态 */
const detailLoading = ref(false)
/** 详情错误 */
const detailError = ref<unknown>(null)
/** 当前页签 */
const activeTab = ref<TabKey>('variables')
/** 变量集合 */
const variables = ref<HistoricVariableValue[]>([])
/** 变量加载态 */
const variableLoading = ref(false)
/** 变量错误 */
const variableError = ref<unknown>(null)
/** 变量是否已加载 */
const variablesLoaded = ref(false)
/** 评论集合 */
const comments = ref<HistoricComment[]>([])
/** 评论加载态 */
const commentLoading = ref(false)
/** 评论错误 */
const commentError = ref<unknown>(null)
/** 评论是否已加载 */
const commentsLoaded = ref(false)
/** 身份链接集合 */
const links = ref<HistoricIdentityLink[]>([])
/** 链接加载态 */
const linkLoading = ref(false)
/** 链接错误 */
const linkError = ref<unknown>(null)
/** 链接是否已加载 */
const linksLoaded = ref(false)
/** 删除加载态 */
const deleting = ref(false)
/** 删除错误 */
const actionError = ref<unknown>(null)

/** 是否仍在运行 */
const running = computed(() => Boolean(detail.value) && !detail.value?.endTime)

// 加载实例详情。
// @return 无
async function loadDetail(): Promise<void> {
  detailLoading.value = true
  detailError.value = null
  try {
    detail.value = await getHistoricProcessInstance(id.value)
  } catch (cause) {
    detailError.value = cause
    if (isAuthError(cause)) markAuthError(cause)
  } finally {
    detailLoading.value = false
  }
}

// 把裸变量实体转换为统一的变量值结构。
// @param record 历史变量实体
// @return 变量值
function toVariableValue(record: HistoricVariableRecord): HistoricVariableValue {
  return {
    name: record.name,
    type: record.type ?? null,
    value: record.value,
    scope: record.taskId ? 'local' : 'global',
  }
}

// 加载历史变量。
// @return 无
async function loadVariables(): Promise<void> {
  variableLoading.value = true
  variableError.value = null
  try {
    const result = await listHistoricProcessVariables(id.value, { start: 0, size: 1000 })
    variables.value = result.data.map(toVariableValue)
    variablesLoaded.value = true
  } catch (cause) {
    variableError.value = cause
    if (isAuthError(cause)) markAuthError(cause)
  } finally {
    variableLoading.value = false
  }
}

// 加载实例评论。
// @return 无
async function loadComments(): Promise<void> {
  commentLoading.value = true
  commentError.value = null
  try {
    comments.value = await listHistoricProcessComments(id.value)
    commentsLoaded.value = true
  } catch (cause) {
    commentError.value = cause
    if (isAuthError(cause)) markAuthError(cause)
  } finally {
    commentLoading.value = false
  }
}

// 加载身份链接。
// @return 无
async function loadLinks(): Promise<void> {
  linkLoading.value = true
  linkError.value = null
  try {
    links.value = await listHistoricProcessIdentityLinks(id.value)
    linksLoaded.value = true
  } catch (cause) {
    linkError.value = cause
    if (isAuthError(cause)) markAuthError(cause)
  } finally {
    linkLoading.value = false
  }
}

// 切换页签，首次进入时按需加载数据。
// @param key 页签
// @return 无
function switchTab(key: TabKey): void {
  activeTab.value = key
  // 已加载过的页签不再重复请求
  if (key === 'variables' && !variablesLoaded.value) void loadVariables()
  if (key === 'comments' && !commentsLoaded.value) void loadComments()
  if (key === 'links' && !linksLoaded.value) void loadLinks()
}

// 重新加载当前页签的数据。
// @return 无
function loadActive(): void {
  if (activeTab.value === 'variables') void loadVariables()
  else if (activeTab.value === 'comments') void loadComments()
  else void loadLinks()
}

// 删除历史实例，成功后回到列表。
// @return 无
async function remove(): Promise<void> {
  deleting.value = true
  actionError.value = null
  try {
    await deleteHistoricProcessInstance(id.value)
    await router.push({ name: 'historic-processes' })
  } catch (cause) {
    actionError.value = cause
    if (isAuthError(cause)) markAuthError(cause)
  } finally {
    deleting.value = false
  }
}

// 加载实例变量的原始数据。
// @param variable 变量
// @return 原始数据
function loadRaw(variable: HistoricVariableValue): Promise<Blob> {
  return getHistoricProcessVariableData(id.value, variable.name)
}

/** 评论列 */
const commentColumns = computed<DataTableColumns<HistoricComment>>(() => [
  { title: '时间', key: 'time', width: 170, render: (row) => formatDateTime(row.time) },
  { title: '评论人', key: 'author', width: 140, render: (row) => textOrDash(row.author) },
  { title: '内容', key: 'message', minWidth: 260, render: (row) => textOrDash(row.message) },
  {
    title: '任务 ID',
    key: 'taskId',
    width: 180,
    render: (row) => (row.taskId ? h('span', { class: 'mono' }, shortId(row.taskId, 8, 4)) : '—'),
  },
])

/** 身份链接列 */
const linkColumns = computed<DataTableColumns<HistoricIdentityLink>>(() => [
  { title: '类型', key: 'type', width: 130, render: (row) => textOrDash(row.type) },
  { title: '用户', key: 'userId', width: 150, render: (row) => textOrDash(row.userId) },
  { title: '组', key: 'groupId', width: 150, render: (row) => textOrDash(row.groupId) },
  { title: '任务 ID', key: 'taskId', width: 200, render: (row) => (row.taskId ? h('span', { class: 'mono' }, row.taskId) : '—') },
])

onMounted(() => {
  void loadDetail()
  void loadVariables()
})
</script>

<template>
  <div>
    <PageHeader title="历史流程详情" :description="detail ? `实例 ${detail.id}` : '加载历史流程实例的完整记录。'">
      <template #actions>
        <n-button size="small" quaternary @click="router.push('/history/processes')">返回列表</n-button>
        <n-button
          v-if="running"
          size="small"
          quaternary
          @click="router.push(`/runtime/instances/${encodeURIComponent(id)}`)"
        >
          运行实例
        </n-button>
        <ConfirmButton
          label="删除历史"
          title="确认删除该历史流程实例？"
          description="将级联删除该实例的历史变量、任务、活动、评论与身份链接，操作不可撤销；运行中的实例不允许删除。"
          :loading="deleting"
          @confirm="remove"
        />
      </template>
    </PageHeader>

    <ErrorBlock v-if="actionError" :error="actionError" @retry="remove" />
    <ErrorBlock v-if="detailError" :error="detailError" @retry="loadDetail" />
    <div v-else-if="detailLoading && !detail" class="panel">
      <div class="panel__body"><n-spin size="small" /> 加载中…</div>
    </div>

    <template v-if="detail">
      <section class="panel">
        <div class="panel__header">
          <span class="panel__title">基本信息</span>
          <StatusTag :value="detail.endTime ? '已完成' : '运行中'" :tone="detail.endTime ? 'success' : 'info'" />
        </div>
        <div class="panel__body">
          <div class="detail-grid">
            <div class="detail-item">
              <span class="detail-item__label">实例 ID</span>
              <span class="detail-item__value"><CopyText :value="detail.id" /></span>
            </div>
            <div class="detail-item">
              <span class="detail-item__label">流程定义</span>
              <span class="detail-item__value">
                <RouterLink :to="`/repository/definitions/${encodeURIComponent(detail.processDefinitionId)}`">
                  {{ detail.processDefinitionName || detail.processDefinitionId }}
                </RouterLink>
                <span class="mono muted"> {{ detail.processDefinitionId }}</span>
              </span>
            </div>
            <div class="detail-item">
              <span class="detail-item__label">业务 Key</span>
              <span class="detail-item__value">{{ textOrDash(detail.businessKey) }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-item__label">发起人</span>
              <span class="detail-item__value">{{ textOrDash(detail.startUserId) }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-item__label">开始时间</span>
              <span class="detail-item__value">{{ formatDateTime(detail.startTime) }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-item__label">结束时间</span>
              <span class="detail-item__value">{{ formatDateTime(detail.endTime) }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-item__label">时长</span>
              <span class="detail-item__value">{{ formatDurationMs(detail.durationInMillis) }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-item__label">删除原因</span>
              <span class="detail-item__value">{{ textOrDash(detail.deleteReason) }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-item__label">租户</span>
              <span class="detail-item__value">{{ textOrDash(detail.tenantId) }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-item__label">开始活动</span>
              <span class="detail-item__value">{{ textOrDash(detail.startActivityId) }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-item__label">结束活动</span>
              <span class="detail-item__value">{{ textOrDash(detail.endActivityId) }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-item__label">父流程实例</span>
              <span class="detail-item__value">{{ textOrDash(detail.superProcessInstanceId) }}</span>
            </div>
          </div>
        </div>
      </section>

      <section class="panel">
        <div class="panel__header">
          <span class="panel__title">关联数据</span>
          <n-button-group size="small" style="margin-left: auto">
            <n-button
              v-for="tab in TABS"
              :key="tab.value"
              size="small"
              :type="activeTab === tab.value ? 'primary' : 'default'"
              :secondary="activeTab !== tab.value"
              @click="switchTab(tab.value)"
            >
              {{ tab.label }}
            </n-button>
          </n-button-group>
        </div>
        <div class="panel__body">
          <template v-if="activeTab === 'variables'">
            <ErrorBlock v-if="variableError" :error="variableError" @retry="loadVariables" />
            <HistoricVariableList
              v-else
              :variables="variables"
              :loading="variableLoading"
              :raw-loader="loadRaw"
              empty-text="该实例没有历史变量，可能启动时未传入变量或历史级别较低。"
            />
          </template>
          <template v-else-if="activeTab === 'comments'">
            <ErrorBlock v-if="commentError" :error="commentError" @retry="loadComments" />
            <n-data-table
              v-else
              :columns="commentColumns"
              :data="comments"
              :loading="commentLoading"
              :bordered="false"
              :single-line="false"
            >
              <template #empty>
                <span class="muted">该实例没有评论记录。</span>
              </template>
            </n-data-table>
          </template>
          <template v-else>
            <ErrorBlock v-if="linkError" :error="linkError" @retry="loadLinks" />
            <n-data-table
              v-else
              :columns="linkColumns"
              :data="links"
              :loading="linkLoading"
              :bordered="false"
              :single-line="false"
            >
              <template #empty>
                <span class="muted">该实例没有身份链接记录。</span>
              </template>
            </n-data-table>
          </template>
          <div v-if="variableError || commentError || linkError" class="row" style="margin-top: 8px">
            <n-button size="tiny" quaternary @click="loadActive">重试当前页签</n-button>
          </div>
        </div>
      </section>
    </template>
  </div>
</template>
