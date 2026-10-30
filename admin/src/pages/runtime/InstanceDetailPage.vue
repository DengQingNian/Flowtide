<script setup lang="ts">
// 流程实例详情：概览、流程图、变量、任务、评论、附件、身份链接与迁移。
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NButton, NInput, NModal, NSpin, NTabPane, NTabs } from 'naive-ui'
import {
  deleteProcessInstance,
  getProcessInstance,
  updateProcessInstanceState,
  type ProcessInstance,
} from '@/api/runtime'
import { errorMessage } from '@/api/client'
import { markAuthError } from '@/stores/ui'
import { formatDateTime, shortId } from '@/lib/format'
import PageHeader from '@/components/PageHeader.vue'
import ErrorBlock from '@/components/ErrorBlock.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import OverviewTab from './components/instance/OverviewTab.vue'
import DiagramTab from './components/instance/DiagramTab.vue'
import VariablesTab from './components/instance/VariablesTab.vue'
import TasksTab from './components/instance/TasksTab.vue'
import CommentsTab from './components/instance/CommentsTab.vue'
import AttachmentsTab from './components/instance/AttachmentsTab.vue'
import IdentityLinksTab from './components/instance/IdentityLinksTab.vue'
import MigrationTab from './components/instance/MigrationTab.vue'

const route = useRoute()
const router = useRouter()

/** 实例 ID */
const instanceId = computed(() => String(route.params.id ?? ''))
const instance = ref<ProcessInstance | null>(null)
const loading = ref(false)
const error = ref<unknown>(null)
const activeTab = ref('overview')
const notice = ref('')
const actionError = ref('')
const busy = ref(false)

const showDelete = ref(false)
const deleteReason = ref('管理台删除')
const deleting = ref(false)

/** 页面标题 */
const title = computed(() => instance.value?.name || `流程实例 ${shortId(instanceId.value, 10, 6)}`)

// 加载实例详情。
// @return 无
async function load(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    instance.value = await getProcessInstance(instanceId.value)
  } catch (cause) {
    instance.value = null
    error.value = cause
    markAuthError(cause)
  } finally {
    loading.value = false
  }
}

// 挂起或激活实例，成功后重新加载完整 DTO。
// @return 无
async function toggleState(): Promise<void> {
  if (!instance.value) return
  const action = instance.value.suspended ? 'activate' : 'suspend'
  busy.value = true
  actionError.value = ''
  notice.value = ''
  try {
    await updateProcessInstanceState(instanceId.value, action)
    notice.value = action === 'suspend' ? '实例已挂起' : '实例已激活'
    await load()
  } catch (cause) {
    actionError.value = errorMessage(cause)
    markAuthError(cause)
  } finally {
    busy.value = false
  }
}

// 删除实例并返回列表。
// @return 无
async function confirmDelete(): Promise<void> {
  deleting.value = true
  actionError.value = ''
  try {
    await deleteProcessInstance(instanceId.value, deleteReason.value.trim())
    showDelete.value = false
    await router.push('/runtime/instances')
  } catch (cause) {
    actionError.value = errorMessage(cause)
    markAuthError(cause)
  } finally {
    deleting.value = false
  }
}

// 迁移完成后刷新详情并回到概览。
// @return 无
function onMigrated(): void {
  activeTab.value = 'overview'
  notice.value = '实例迁移完成，已刷新详情'
  void load()
}

// 路由参数变化时重新加载。
watch(instanceId, () => {
  activeTab.value = 'overview'
  void load()
}, { immediate: true })
</script>

<template>
  <div>
    <PageHeader :title="title" description="查看实例执行位置、变量、任务与协作记录。">
      <template #actions>
        <n-button size="small" quaternary @click="router.push('/runtime/instances')">返回列表</n-button>
        <n-button size="small" quaternary :loading="loading" @click="load">刷新</n-button>
        <ConfirmButton
          v-if="instance && !instance.ended"
          :label="instance.suspended ? '激活实例' : '挂起实例'"
          :title="instance.suspended ? '确认激活该实例？' : '确认挂起该实例？'"
          :description="instance.suspended ? '激活后实例可继续推进，待办任务恢复可办理。' : '挂起后实例停止推进，所有待办任务不可办理。'"
          :danger="!instance.suspended"
          :loading="busy"
          @confirm="toggleState"
        />
        <n-button v-if="instance" size="small" type="error" secondary @click="showDelete = true">删除实例</n-button>
      </template>
    </PageHeader>

    <div v-if="notice" class="text-success" style="margin-bottom: 8px">{{ notice }}</div>
    <div v-if="actionError" class="banner banner--error" role="alert" style="margin: 0 0 10px">
      <span class="grow">{{ actionError }}</span>
    </div>

    <n-spin :show="loading && !instance">
      <ErrorBlock v-if="error" :error="error" @retry="load" />
      <template v-else-if="instance">
        <div class="muted" style="margin-bottom: 8px; font-size: 12px">
          开始于 {{ formatDateTime(instance.startTime) }} · 定义 {{ instance.processDefinitionId }}
        </div>
        <n-tabs v-model:value="activeTab" type="line" display-directive="if">
          <n-tab-pane name="overview" tab="概览">
            <OverviewTab v-if="activeTab === 'overview'" :instance="instance" />
          </n-tab-pane>
          <n-tab-pane name="diagram" tab="流程图">
            <DiagramTab
              v-if="activeTab === 'diagram'"
              :instance-id="instanceId"
              :definition-id="instance.processDefinitionId"
            />
          </n-tab-pane>
          <n-tab-pane name="variables" tab="变量">
            <VariablesTab v-if="activeTab === 'variables'" :instance-id="instanceId" />
          </n-tab-pane>
          <n-tab-pane name="tasks" tab="任务">
            <TasksTab v-if="activeTab === 'tasks'" :instance-id="instanceId" />
          </n-tab-pane>
          <n-tab-pane name="comments" tab="评论">
            <CommentsTab v-if="activeTab === 'comments'" :instance-id="instanceId" />
          </n-tab-pane>
          <n-tab-pane name="attachments" tab="附件">
            <AttachmentsTab v-if="activeTab === 'attachments'" :instance-id="instanceId" />
          </n-tab-pane>
          <n-tab-pane name="identitylinks" tab="身份链接">
            <IdentityLinksTab v-if="activeTab === 'identitylinks'" :instance-id="instanceId" />
          </n-tab-pane>
          <n-tab-pane name="migration" tab="迁移">
            <MigrationTab
              v-if="activeTab === 'migration'"
              :instance-id="instanceId"
              :definition-id="instance.processDefinitionId"
              @migrated="onMigrated"
            />
          </n-tab-pane>
        </n-tabs>
      </template>
    </n-spin>

    <n-modal v-model:show="showDelete" preset="card" title="删除流程实例" style="width: 480px; max-width: 92vw">
      <p style="margin-top: 0">
        删除后实例的运行执行、待办任务与变量会被清理，且不可恢复。已生成的历史数据是否保留取决于引擎历史级别。
      </p>
      <div class="filter-bar__label">删除原因</div>
      <n-input v-model:value="deleteReason" type="textarea" :rows="3" placeholder="填写删除原因，写入历史记录" />
      <template #footer>
        <div class="row" style="justify-content: flex-end">
          <n-button size="small" quaternary @click="showDelete = false">取消</n-button>
          <ConfirmButton label="确认删除" :loading="deleting" @confirm="confirmDelete" />
        </div>
      </template>
    </n-modal>
  </div>
</template>
