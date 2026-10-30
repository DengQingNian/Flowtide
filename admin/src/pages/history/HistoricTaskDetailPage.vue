<script setup lang="ts">
// 历史任务详情：任务信息、变量、表单与身份链接，并提供历史任务删除。
import { computed, h, onMounted, ref } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { NAlert, NButton, NButtonGroup, NDataTable, NSpin } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import {
  deleteHistoricTaskInstance,
  getHistoricTaskForm,
  getHistoricTaskInstance,
  getHistoricTaskVariableData,
  listHistoricTaskIdentityLinks,
  listHistoricTaskVariables,
} from '@/api/history'
import type { HistoricIdentityLink, HistoricTaskInstance, HistoricVariableValue } from '@/api/history'
import type { FormData, FormProperty } from '@/api/types'
import { errorMessage, isAuthError } from '@/api/client'
import { markAuthError } from '@/stores/ui'
import { formatDateTime, formatDurationMs, formatValue, textOrDash } from '@/lib/format'
import PageHeader from '@/components/PageHeader.vue'
import ErrorBlock from '@/components/ErrorBlock.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import StatusTag from '@/components/StatusTag.vue'
import CopyText from '@/components/CopyText.vue'
import HistoricVariableList from './components/HistoricVariableList.vue'

/** 详情页签 */
type TabKey = 'variables' | 'form' | 'links'

/** 页签定义 */
const TABS: { value: TabKey; label: string }[] = [
  { value: 'variables', label: '变量' },
  { value: 'form', label: '表单' },
  { value: 'links', label: '身份链接' },
]

const route = useRoute()
const router = useRouter()

/** 任务 ID */
const id = computed(() => String(route.params.id ?? ''))
/** 任务详情 */
const detail = ref<HistoricTaskInstance | null>(null)
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
/** 表单数据 */
const form = ref<FormData | null>(null)
/** 表单加载态 */
const formLoading = ref(false)
/** 表单错误 */
const formError = ref<unknown>(null)
/** 表单是否已尝试加载 */
const formLoaded = ref(false)
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

/** 表单错误是否为认证问题 */
const formAuthError = computed(() => isAuthError(formError.value))
/** 表单错误说明 */
const formErrorMessage = computed(() => (formError.value ? errorMessage(formError.value) : ''))

// 加载任务详情。
// @return 无
async function loadDetail(): Promise<void> {
  detailLoading.value = true
  detailError.value = null
  try {
    detail.value = await getHistoricTaskInstance(id.value)
  } catch (cause) {
    detailError.value = cause
    if (isAuthError(cause)) markAuthError(cause)
  } finally {
    detailLoading.value = false
  }
}

// 加载任务变量（通过历史任务查询端点填充）。
// @return 无
async function loadVariables(): Promise<void> {
  variableLoading.value = true
  variableError.value = null
  try {
    variables.value = await listHistoricTaskVariables(id.value)
    variablesLoaded.value = true
  } catch (cause) {
    variableError.value = cause
    if (isAuthError(cause)) markAuthError(cause)
  } finally {
    variableLoading.value = false
  }
}

// 加载历史任务表单。
// @return 无
async function loadForm(): Promise<void> {
  formLoading.value = true
  formError.value = null
  form.value = null
  try {
    form.value = await getHistoricTaskForm(id.value)
    formLoaded.value = true
  } catch (cause) {
    formError.value = cause
    formLoaded.value = true
    if (isAuthError(cause)) markAuthError(cause)
  } finally {
    formLoading.value = false
  }
}

// 加载身份链接。
// @return 无
async function loadLinks(): Promise<void> {
  linkLoading.value = true
  linkError.value = null
  try {
    links.value = await listHistoricTaskIdentityLinks(id.value)
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
  if (key === 'form' && !formLoaded.value) void loadForm()
  if (key === 'links' && !linksLoaded.value) void loadLinks()
}

// 重新加载当前页签的数据。
// @return 无
function loadActive(): void {
  if (activeTab.value === 'variables') void loadVariables()
  else if (activeTab.value === 'form') void loadForm()
  else void loadLinks()
}

// 删除历史任务，成功后回到列表。
// @return 无
async function remove(): Promise<void> {
  deleting.value = true
  actionError.value = null
  try {
    await deleteHistoricTaskInstance(id.value)
    await router.push({ name: 'historic-tasks' })
  } catch (cause) {
    actionError.value = cause
    if (isAuthError(cause)) markAuthError(cause)
  } finally {
    deleting.value = false
  }
}

// 加载任务变量的原始数据。
// @param variable 变量
// @return 原始数据
function loadRaw(variable: HistoricVariableValue): Promise<Blob> {
  return getHistoricTaskVariableData(id.value, variable.name)
}

/** 表单字段列 */
const formColumns = computed<DataTableColumns<FormProperty>>(() => [
  { title: '字段 ID', key: 'id', minWidth: 160, render: (row) => h('span', { class: 'mono' }, row.id) },
  { title: '名称', key: 'name', minWidth: 140, render: (row) => textOrDash(row.name) },
  { title: '类型', key: 'type', width: 110, render: (row) => textOrDash(row.type) },
  { title: '必填', key: 'required', width: 80, render: (row) => (row.required ? '是' : '否') },
  { title: '可写', key: 'writable', width: 80, render: (row) => (row.writable === false ? '否' : '是') },
  { title: '值', key: 'value', minWidth: 200, render: (row) => formatValue(row.value) },
])

/** 身份链接列 */
const linkColumns = computed<DataTableColumns<HistoricIdentityLink>>(() => [
  { title: '类型', key: 'type', width: 130, render: (row) => textOrDash(row.type) },
  { title: '用户', key: 'userId', width: 150, render: (row) => textOrDash(row.userId) },
  { title: '组', key: 'groupId', width: 150, render: (row) => textOrDash(row.groupId) },
])

onMounted(() => {
  void loadDetail()
  void loadVariables()
})
</script>

<template>
  <div>
    <PageHeader title="历史任务详情" :description="detail ? `任务 ${detail.id}` : '加载历史任务的完整记录。'">
      <template #actions>
        <n-button size="small" quaternary @click="router.push('/history/tasks')">返回列表</n-button>
        <ConfirmButton
          label="删除历史任务"
          title="确认删除该历史任务？"
          description="将删除该任务及其任务日志、身份链接、评论与附件，操作不可撤销；任务局部历史变量会保留，可在历史变量页单独删除。"
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
          <StatusTag :value="detail.endTime ? '已完成' : '进行中'" :tone="detail.endTime ? 'success' : 'info'" />
        </div>
        <div class="panel__body">
          <div class="detail-grid">
            <div class="detail-item">
              <span class="detail-item__label">任务 ID</span>
              <span class="detail-item__value"><CopyText :value="detail.id" /></span>
            </div>
            <div class="detail-item">
              <span class="detail-item__label">任务名称</span>
              <span class="detail-item__value">{{ textOrDash(detail.name) }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-item__label">办理人</span>
              <span class="detail-item__value">{{ textOrDash(detail.assignee) }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-item__label">所有者</span>
              <span class="detail-item__value">{{ textOrDash(detail.owner) }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-item__label">流程实例</span>
              <span class="detail-item__value">
                <RouterLink
                  v-if="detail.processInstanceId"
                  :to="{ name: 'historic-process-detail', params: { id: detail.processInstanceId } }"
                  class="mono"
                >
                  {{ detail.processInstanceId }}
                </RouterLink>
                <span v-else>—</span>
              </span>
            </div>
            <div class="detail-item">
              <span class="detail-item__label">流程定义</span>
              <span class="detail-item__value">
                <RouterLink
                  v-if="detail.processDefinitionId"
                  :to="`/repository/definitions/${encodeURIComponent(detail.processDefinitionId)}`"
                >
                  {{ detail.processDefinitionId }}
                </RouterLink>
                <span v-else>—</span>
              </span>
            </div>
            <div class="detail-item">
              <span class="detail-item__label">任务定义 Key</span>
              <span class="detail-item__value">{{ textOrDash(detail.taskDefinitionKey) }}</span>
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
              <span class="detail-item__label">到期时间</span>
              <span class="detail-item__value">{{ formatDateTime(detail.dueDate) }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-item__label">表单 Key</span>
              <span class="detail-item__value">{{ textOrDash(detail.formKey) }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-item__label">删除原因</span>
              <span class="detail-item__value">{{ textOrDash(detail.deleteReason) }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-item__label">租户</span>
              <span class="detail-item__value">{{ textOrDash(detail.tenantId) }}</span>
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
              empty-text="该任务没有历史变量；任务局部变量与流程全局变量都会显示在这里。"
            />
          </template>
          <template v-else-if="activeTab === 'form'">
            <ErrorBlock v-if="formAuthError" :error="formError" @retry="loadForm" />
            <template v-else>
              <n-alert v-if="formError" type="warning" :show-icon="true" style="margin-bottom: 8px">
                {{ formErrorMessage }}
                <div class="muted" style="margin-top: 4px">
                  引擎未内置表单服务时，历史任务表单端点会固定返回该提示；表单结构请参考流程定义中的表单 Key。
                </div>
              </n-alert>
              <n-data-table
                v-else
                :columns="formColumns"
                :data="form?.formProperties ?? []"
                :loading="formLoading"
                :bordered="false"
                :single-line="false"
              >
                <template #empty>
                  <span class="muted">该任务没有表单字段。</span>
                </template>
              </n-data-table>
            </template>
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
                <span class="muted">该任务没有身份链接记录。</span>
              </template>
            </n-data-table>
          </template>
          <div v-if="variableError || formError || linkError" class="row" style="margin-top: 8px">
            <n-button size="tiny" quaternary @click="loadActive">重试当前页签</n-button>
          </div>
        </div>
      </section>
    </template>
  </div>
</template>
