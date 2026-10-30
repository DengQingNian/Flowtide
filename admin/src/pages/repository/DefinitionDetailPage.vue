<script setup lang="ts">
// 流程定义详情：基本信息、BPMN 图与 XML、启动表单、身份链接、挂起激活与启动实例。
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NAlert, NButton, NCheckbox, NModal, NTabPane, NTabs } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import ErrorBlock from '@/components/ErrorBlock.vue'
import StatusTag from '@/components/StatusTag.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import CopyText from '@/components/CopyText.vue'
import XmlViewer from '@/components/XmlViewer.vue'
import StartInstanceDialog from '@/components/StartInstanceDialog.vue'
import type { StartInstancePayload } from '@/components/StartInstanceDialog.vue'
import DefinitionStartForm from './components/DefinitionStartForm.vue'
import DefinitionIdentityLinks from './components/DefinitionIdentityLinks.vue'
import {
  getDefinitionResourceBlob,
  getProcessDefinition,
  getProcessDefinitionImage,
  getProcessDefinitionXML,
  startProcessInstance,
  updateProcessDefinition,
} from '@/api/repository'
import type { ProcessDefinition, StartProcessInstancePayload } from '@/api/repository'
import { downloadBlob } from '@/lib/download'
import { textOrDash } from '@/lib/format'
import { markAuthError } from '@/stores/ui'

const route = useRoute()
const router = useRouter()
const definitionId = computed(() => String(route.params.id ?? ''))

const definition = ref<ProcessDefinition | null>(null)
const xml = ref('')
const xmlLoading = ref(false)
const xmlError = ref<unknown>(null)
const imageUrl = ref('')
const imageLoading = ref(false)
const imageError = ref(false)
const loading = ref(false)
const error = ref<unknown>(null)
const actionError = ref<unknown>(null)
const actionMessage = ref('')
const retryAction = ref<() => void>(() => {})
const toggling = ref(false)
const includeInstances = ref(false)
const showStart = ref(false)
const showMigrate = ref(false)
const starting = ref(false)

const suspended = computed(() => Boolean(definition.value?.suspended))
const startOptions = computed(() =>
  definition.value
    ? [{ id: definition.value.id, key: definition.value.key, name: definition.value.name, version: definition.value.version }]
    : [],
)
const toggleLabel = computed(() => (suspended.value ? '激活' : '挂起'))
const toggleDescription = computed(() =>
  suspended.value
    ? '激活后可重新启动实例；是否同时激活运行中的实例由下方选项决定。'
    : '挂起后不能启动新实例；是否同时挂起运行中的实例由下方选项决定。',
)

// 释放图片预览地址。
// @return 无
function releaseImageUrl(): void {
  if (imageUrl.value) {
    URL.revokeObjectURL(imageUrl.value)
    imageUrl.value = ''
  }
}

// 读取 BPMN XML。
// @return 无
async function loadXml(): Promise<void> {
  xmlLoading.value = true
  xmlError.value = null
  try {
    xml.value = await getProcessDefinitionXML(definitionId.value)
  } catch (cause) {
    markAuthError(cause)
    xmlError.value = cause
  } finally {
    xmlLoading.value = false
  }
}

// 读取流程图图片，404 或缺图时由页面降级到 XML。
// @return 无
async function loadImage(): Promise<void> {
  imageLoading.value = true
  imageError.value = false
  releaseImageUrl()
  try {
    const blob = await getProcessDefinitionImage(definitionId.value)
    imageUrl.value = URL.createObjectURL(blob)
  } catch (cause) {
    markAuthError(cause)
    imageError.value = true
  } finally {
    imageLoading.value = false
  }
}

let loadToken = 0

// 加载定义基本信息、XML 与图片。
// @return 无
async function load(): Promise<void> {
  const token = ++loadToken
  loading.value = true
  error.value = null
  actionError.value = null
  try {
    definition.value = await getProcessDefinition(definitionId.value)
    if (token !== loadToken) return
    await Promise.all([loadXml(), loadImage()])
  } catch (cause) {
    if (token !== loadToken) return
    markAuthError(cause)
    error.value = cause
  } finally {
    if (token === loadToken) loading.value = false
  }
}

// 记录操作错误与重试入口。
// @param cause 错误
// @param retry 重试函数
// @return 无
function failAction(cause: unknown, retry: () => void): void {
  markAuthError(cause)
  actionError.value = cause
  actionMessage.value = ''
  retryAction.value = retry
}

// 重试上一次失败的操作。
// @return 无
function retryFailedAction(): void {
  retryAction.value()
}

// 挂起或激活流程定义。
// @return 无
async function toggleSuspension(): Promise<void> {
  if (!definition.value) return
  toggling.value = true
  actionError.value = null
  actionMessage.value = ''
  const action = suspended.value ? 'activate' : 'suspend'
  try {
    await updateProcessDefinition(definitionId.value, { action, includeProcessInstances: includeInstances.value })
    definition.value = await getProcessDefinition(definitionId.value)
    actionMessage.value = action === 'suspend' ? '流程定义已挂起。' : '流程定义已激活。'
  } catch (cause) {
    failAction(cause, () => void toggleSuspension())
  } finally {
    toggling.value = false
  }
}

// 启动流程实例并跳转实例详情。
// @param payload 启动载荷
// @return 无
async function start(payload: StartInstancePayload | Record<string, unknown>): Promise<void> {
  starting.value = true
  actionError.value = null
  actionMessage.value = ''
  try {
    const request = payload as StartInstancePayload
    const variables = (request.variables ?? {}) as Record<string, unknown>
    const body: StartProcessInstancePayload = {
      processDefinitionId: request.processDefinitionId ?? definitionId.value,
      businessKey: request.businessKey,
      name: request.name,
      variables,
    }
    const instance = await startProcessInstance(body)
    showStart.value = false
    if (!instance.id) throw new Error('启动成功但服务端未返回实例 ID')
    await router.push(`/runtime/instances/${instance.id}`)
  } catch (cause) {
    failAction(cause, () => void start(payload))
  } finally {
    starting.value = false
  }
}

// 用启动表单字段值启动实例。
// @param values 表单值
// @return 无
function startFromForm(values: Record<string, unknown>): void {
  void start({ processDefinitionId: definitionId.value, variables: values })
}

// 下载流程定义原始资源。
// @return 无
async function downloadResource(): Promise<void> {
  try {
    const blob = await getDefinitionResourceBlob(definitionId.value)
    downloadBlob(definition.value?.resourceName || `${definition.value?.key ?? 'process'}.bpmn20.xml`, blob)
  } catch (cause) {
    failAction(cause, () => void downloadResource())
  }
}

onMounted(() => void load())
onUnmounted(releaseImageUrl)
</script>

<template>
  <div>
    <PageHeader
      :title="definition ? `${definition.name || definition.key}` : '流程定义详情'"
      description="查看 BPMN 图与 XML、渲染启动表单、维护候选发起人并启动实例。"
    >
      <template #actions>
        <n-button size="small" quaternary @click="router.push('/repository/definitions')">返回列表</n-button>
        <n-button size="small" quaternary :loading="loading" @click="load">刷新</n-button>
        <n-button size="small" type="primary" :disabled="suspended" @click="showStart = true">启动实例</n-button>
        <ConfirmButton
          v-if="definition"
          :label="toggleLabel"
          :title="`${toggleLabel}该流程定义？`"
          :description="toggleDescription"
          :danger="!suspended"
          :loading="toggling"
          @confirm="toggleSuspension"
        />
        <n-button size="small" quaternary @click="showMigrate = true">迁移</n-button>
      </template>
    </PageHeader>

    <ErrorBlock v-if="error" :error="error" context="GET /service/repository/process-definitions/{id}" @retry="load" />
    <ErrorBlock v-if="actionError" :error="actionError" context="流程定义操作" @retry="retryFailedAction" />
    <n-alert v-if="actionMessage" type="success" :bordered="false" closable style="margin-bottom: 12px" @close="actionMessage = ''">
      {{ actionMessage }}
    </n-alert>
    <n-alert v-if="definition && suspended" type="warning" :bordered="false" style="margin-bottom: 12px">
      该流程定义已挂起，不能启动新实例；激活后可恢复启动。
    </n-alert>

    <div v-if="definition" class="stack">
      <div class="panel">
        <div class="panel__header"><span class="panel__title">基本信息</span></div>
        <div class="panel__body">
          <div class="detail-grid">
            <div class="detail-item">
              <span class="detail-item__label">定义 ID</span>
              <span class="detail-item__value"><CopyText :value="definition.id" /></span>
            </div>
            <div class="detail-item">
              <span class="detail-item__label">Key</span>
              <span class="detail-item__value mono">{{ textOrDash(definition.key) }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-item__label">名称</span>
              <span class="detail-item__value">{{ textOrDash(definition.name) }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-item__label">版本</span>
              <span class="detail-item__value">v{{ definition.version }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-item__label">状态</span>
              <span class="detail-item__value">
                <StatusTag :value="suspended ? 'suspended' : 'active'" :label="suspended ? '挂起' : '激活'" />
              </span>
            </div>
            <div class="detail-item">
              <span class="detail-item__label">租户</span>
              <span class="detail-item__value">{{ textOrDash(definition.tenantId) }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-item__label">部署</span>
              <span class="detail-item__value">
                <n-button
                  v-if="definition.deploymentId"
                  text
                  type="primary"
                  size="small"
                  @click="router.push(`/repository/deployments/${definition.deploymentId}`)"
                >
                  {{ definition.deploymentId }}
                </n-button>
                <template v-else>—</template>
              </span>
            </div>
            <div class="detail-item">
              <span class="detail-item__label">资源名</span>
              <span class="detail-item__value mono">{{ textOrDash(definition.resourceName) }}</span>
            </div>
          </div>
          <div class="row" style="margin-top: 12px">
            <n-button size="tiny" quaternary @click="downloadResource">下载原始资源</n-button>
            <n-checkbox v-model:checked="includeInstances">挂起/激活时同时处理运行中的实例</n-checkbox>
          </div>
        </div>
      </div>

      <div class="panel">
        <div class="panel__body">
          <n-tabs type="line" default-value="diagram">
            <n-tab-pane name="diagram" tab="流程图">
              <div v-if="imageLoading" class="row" style="justify-content: center; padding: 24px">加载图形中…</div>
              <img
                v-else-if="imageUrl"
                :src="imageUrl"
                :alt="`${definition.key} 流程图`"
                style="max-width: 100%; border: 1px solid var(--border); border-radius: 6px"
              />
              <template v-else>
                <n-alert type="warning" :bordered="false" style="margin-bottom: 10px">
                  图形不可用（可能缺少 BPMN DI 图形信息或图片接口返回 404），已降级展示 BPMN XML。
                </n-alert>
                <div v-if="xmlLoading" class="muted">加载 XML 中…</div>
                <ErrorBlock v-else-if="xmlError" :error="xmlError" context="GET /service/repository/process-definitions/{id}/xml" @retry="loadXml" />
                <XmlViewer v-else :xml="xml" :filename="definition.resourceName || `${definition.key}.bpmn20.xml`" />
              </template>
            </n-tab-pane>
            <n-tab-pane name="xml" tab="BPMN XML">
              <div v-if="xmlLoading" class="muted">加载 XML 中…</div>
              <ErrorBlock v-else-if="xmlError" :error="xmlError" context="GET /service/repository/process-definitions/{id}/xml" @retry="loadXml" />
              <XmlViewer v-else :xml="xml" :filename="definition.resourceName || `${definition.key}.bpmn20.xml`" />
            </n-tab-pane>
            <n-tab-pane name="form" tab="启动表单" lazy>
              <DefinitionStartForm :definition-id="definitionId" :submitting="starting" @submit="startFromForm" />
            </n-tab-pane>
            <n-tab-pane name="identity" tab="身份链接" lazy>
              <DefinitionIdentityLinks :definition-id="definitionId" />
            </n-tab-pane>
          </n-tabs>
        </div>
      </div>
    </div>

    <StartInstanceDialog
      v-model:show="showStart"
      :definitions="startOptions"
      :initial-definition-id="definitionId"
      :submitting="starting"
      @submit="start"
    />

    <n-modal v-model:show="showMigrate" preset="card" title="迁移流程实例" style="width: 560px; max-width: 94vw">
      <p style="margin-top: 0">
        迁移需要选择目标流程定义、校验活动映射，再逐个迁移实例。本期先提供接口入口，页面化迁移在后续里程碑交付。
      </p>
      <pre class="code-block">POST /service/repository/process-definitions/{id}/migrate
POST /service/repository/process-definitions/{id}/batch-migrate</pre>
      <p class="muted">请求体：processInstanceIds[] 与 activityMappings{}；失败即中断并返回引擎原始错误。</p>
      <template #footer>
        <div class="row" style="justify-content: flex-end">
          <n-button size="small" quaternary @click="showMigrate = false">关闭</n-button>
          <n-button size="small" type="primary" @click="router.push('/runtime/instances')">前往实例列表</n-button>
        </div>
      </template>
    </n-modal>
  </div>
</template>
