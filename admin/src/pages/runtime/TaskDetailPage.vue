<script setup lang="ts">
// 任务详情：基本信息、动作、表单、变量、评论、附件、身份链接、子任务与事件。
import { computed, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { NAlert, NButton, NSpin } from 'naive-ui'
import { errorMessage } from '@/api/client'
import { deleteTask, getTask, taskAction, updateTask, type Task } from '@/api/task'
import { markAuthError } from '@/stores/ui'
import { formatDateTime, textOrDash } from '@/lib/format'
import PageHeader from '@/components/PageHeader.vue'
import ErrorBlock from '@/components/ErrorBlock.vue'
import StatusTag from '@/components/StatusTag.vue'
import CopyText from '@/components/CopyText.vue'
import TaskActionBar from './components/task/TaskActionBar.vue'
import TaskFormPanel from './components/task/TaskFormPanel.vue'
import TaskVariablesPanel from './components/task/TaskVariablesPanel.vue'
import TaskCommentsPanel from './components/task/TaskCommentsPanel.vue'
import TaskAttachmentsPanel from './components/task/TaskAttachmentsPanel.vue'
import TaskIdentityLinksPanel from './components/task/TaskIdentityLinksPanel.vue'
import TaskSubTasksPanel from './components/task/TaskSubTasksPanel.vue'
import TaskEventsPanel from './components/task/TaskEventsPanel.vue'

/** 动作类型 */
type ActionKind = 'claim' | 'unclaim' | 'delegate' | 'resolve' | 'assign' | 'due' | 'priority' | 'delete'

/** 动作附加数据 */
interface ActionPayload {
  assignee?: string
  value?: string | number | null
  reason?: string
  cascadeHistory?: boolean
}

/** 页签定义 */
const TABS = [
  { key: 'form', label: '表单' },
  { key: 'variables', label: '变量' },
  { key: 'comments', label: '评论' },
  { key: 'attachments', label: '附件' },
  { key: 'identitylinks', label: '身份链接' },
  { key: 'subtasks', label: '子任务' },
  { key: 'events', label: '事件' },
]

const route = useRoute()
const router = useRouter()
const task = ref<Task | null>(null)
const loading = ref(false)
const error = ref<unknown>(null)
const activeTab = ref('form')
const actionBusy = ref('')
const actionError = ref('')
const actionFeedback = ref('')

/** 任务 ID */
const taskId = computed(() => String(route.params.id ?? ''))

/** 状态文本 */
const statusText = computed(() => {
  const current = task.value
  if (!current) return '—'
  if (current.suspended) return '已挂起'
  if ((current.delegationState ?? '').toLowerCase() === 'pending') return '委派待处理'
  return current.assignee ? (current.claimTime ? '已签收' : '待办理') : '未签收'
})

/** 状态色调 */
const statusTone = computed<'success' | 'danger' | 'warning' | 'info' | 'default'>(() => {
  const current = task.value
  if (!current) return 'default'
  if (current.suspended) return 'danger'
  if ((current.delegationState ?? '').toLowerCase() === 'pending') return 'warning'
  return current.assignee ? 'info' : 'default'
})

// 加载任务详情。
// @return 无
async function load(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    task.value = await getTask(taskId.value)
  } catch (err) {
    markAuthError(err)
    error.value = err
    task.value = null
  } finally {
    loading.value = false
  }
}

// 执行动作并刷新详情。
// @param kind 动作
// @param payload 附加数据
// @return 无
async function handleAction(kind: ActionKind, payload: ActionPayload): Promise<void> {
  const id = taskId.value
  actionError.value = ''
  actionFeedback.value = ''
  if (kind === 'delete') {
    actionBusy.value = 'delete'
    try {
      await deleteTask(id, { cascadeHistory: payload.cascadeHistory, deleteReason: payload.reason })
      await router.push('/runtime/tasks')
    } catch (err) {
      markAuthError(err)
      actionError.value = errorMessage(err)
    } finally {
      actionBusy.value = ''
    }
    return
  }
  actionBusy.value = kind
  try {
    if (kind === 'claim' || kind === 'delegate') await taskAction(id, { action: kind, assignee: payload.assignee })
    else if (kind === 'unclaim' || kind === 'resolve') await taskAction(id, { action: kind })
    else if (kind === 'assign') await updateTask(id, { assignee: payload.assignee || null })
    else if (kind === 'due') await updateTask(id, { dueDate: (payload.value as string | null) || null })
    else if (kind === 'priority') await updateTask(id, { priority: Number(payload.value) })
    actionFeedback.value = '任务已更新'
    await load()
  } catch (err) {
    markAuthError(err)
    actionError.value = errorMessage(err)
  } finally {
    actionBusy.value = ''
  }
}

// 任务完成后返回列表。
// @return 无
async function onCompleted(): Promise<void> {
  await router.push('/runtime/tasks')
}

watch(taskId, () => void load(), { immediate: true })
</script>

<template>
  <div>
    <PageHeader :title="task?.name || '任务详情'" :description="task ? `任务 ${task.id}` : '表单、变量、评论、附件与任务动作。'">
      <template #actions>
        <n-button size="small" quaternary @click="router.push('/runtime/tasks')">返回列表</n-button>
        <router-link v-if="task" :to="`/history/tasks/${task.id}`" class="row">
          <n-button size="small" quaternary>历史任务</n-button>
        </router-link>
        <n-button size="small" :loading="loading" @click="load">刷新</n-button>
      </template>
    </PageHeader>

    <n-spin :show="loading">
      <error-block v-if="error" :error="error" :context="`GET /service/runtime/tasks/${taskId}`" @retry="load">
      </error-block>
      <template v-else-if="task">
        <n-alert v-if="actionError" type="error" :bordered="false" style="margin-bottom: 12px">
          {{ actionError }}
          <div class="muted" style="margin-top: 4px">401/403 时页面与表单内容会保留；可复制诊断信息联系服务端管理员。</div>
        </n-alert>
        <n-alert v-if="actionFeedback" type="success" :bordered="false" style="margin-bottom: 12px">{{ actionFeedback }}</n-alert>

        <div class="panel">
          <div class="panel__header">
            <span class="panel__title">基本信息</span>
            <status-tag :value="statusText" :tone="statusTone" />
            <span v-if="task.suspended" class="muted">流程实例已挂起，动作会被服务端拒绝</span>
          </div>
          <div class="panel__body">
            <div class="detail-grid">
              <div class="detail-item">
                <span class="detail-item__label">任务 ID</span>
                <span class="detail-item__value"><copy-text :value="task.id" /></span>
              </div>
              <div class="detail-item">
                <span class="detail-item__label">办理人</span>
                <span class="detail-item__value">{{ textOrDash(task.assignee) }}</span>
              </div>
              <div class="detail-item">
                <span class="detail-item__label">所有者</span>
                <span class="detail-item__value">{{ textOrDash(task.owner) }}</span>
              </div>
              <div class="detail-item">
                <span class="detail-item__label">委派状态</span>
                <span class="detail-item__value">{{ textOrDash(task.delegationState) }}</span>
              </div>
              <div class="detail-item">
                <span class="detail-item__label">优先级</span>
                <span class="detail-item__value">{{ task.priority }}</span>
              </div>
              <div class="detail-item">
                <span class="detail-item__label">创建时间</span>
                <span class="detail-item__value">{{ formatDateTime(task.createTime) }}</span>
              </div>
              <div class="detail-item">
                <span class="detail-item__label">到期时间</span>
                <span class="detail-item__value">{{ formatDateTime(task.dueDate) }}</span>
              </div>
              <div class="detail-item">
                <span class="detail-item__label">签收时间</span>
                <span class="detail-item__value">{{ formatDateTime(task.claimTime) }}</span>
              </div>
              <div class="detail-item">
                <span class="detail-item__label">流程实例</span>
                <span class="detail-item__value">
                  <router-link v-if="task.processInstanceId" :to="`/runtime/instances/${task.processInstanceId}`">
                    <span class="mono">{{ task.processInstanceId }}</span>
                  </router-link>
                  <span v-else>—</span>
                </span>
              </div>
              <div class="detail-item">
                <span class="detail-item__label">流程定义</span>
                <span class="detail-item__value">
                  <router-link v-if="task.processDefinitionId" :to="`/repository/definitions/${task.processDefinitionId}`">
                    <span class="mono">{{ task.processDefinitionId }}</span>
                  </router-link>
                  <span v-else>—</span>
                </span>
              </div>
              <div class="detail-item">
                <span class="detail-item__label">任务定义 Key</span>
                <span class="detail-item__value mono">{{ textOrDash(task.taskDefinitionKey) }}</span>
              </div>
              <div class="detail-item">
                <span class="detail-item__label">表单 Key</span>
                <span class="detail-item__value mono">{{ textOrDash(task.formKey) }}</span>
              </div>
              <div class="detail-item">
                <span class="detail-item__label">分类</span>
                <span class="detail-item__value">{{ textOrDash(task.category) }}</span>
              </div>
              <div class="detail-item">
                <span class="detail-item__label">租户</span>
                <span class="detail-item__value">{{ textOrDash(task.tenantId) }}</span>
              </div>
              <div class="detail-item">
                <span class="detail-item__label">父任务</span>
                <span class="detail-item__value">
                  <router-link v-if="task.parentTaskId" :to="`/runtime/tasks/${task.parentTaskId}`">
                    <span class="mono">{{ task.parentTaskId }}</span>
                  </router-link>
                  <span v-else>—</span>
                </span>
              </div>
            </div>
          </div>
        </div>

        <task-action-bar :task="task" :busy="actionBusy" style="margin-top: 14px" @action="handleAction" @complete="activeTab = 'form'" />

        <div class="row" style="margin: 16px 0 10px">
          <n-button
            v-for="tab in TABS"
            :key="tab.key"
            size="small"
            :type="activeTab === tab.key ? 'primary' : 'default'"
            :secondary="activeTab === tab.key"
            @click="activeTab = tab.key"
          >
            {{ tab.label }}
          </n-button>
        </div>

        <div class="panel">
          <div class="panel__body">
            <task-form-panel v-if="activeTab === 'form'" :task-id="task.id" @completed="onCompleted" />
            <task-variables-panel v-else-if="activeTab === 'variables'" :task-id="task.id" />
            <task-comments-panel v-else-if="activeTab === 'comments'" :task-id="task.id" />
            <task-attachments-panel v-else-if="activeTab === 'attachments'" :task-id="task.id" />
            <task-identity-links-panel v-else-if="activeTab === 'identitylinks'" :task-id="task.id" />
            <task-sub-tasks-panel v-else-if="activeTab === 'subtasks'" :task-id="task.id" />
            <task-events-panel v-else-if="activeTab === 'events'" :task-id="task.id" />
          </div>
        </div>
      </template>
    </n-spin>
  </div>
</template>