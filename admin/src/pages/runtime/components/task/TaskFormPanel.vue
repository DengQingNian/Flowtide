<script setup lang="ts">
// 任务表单面板：加载表单定义并完成，或退化为变量 JSON 完成。
import { onMounted, ref } from 'vue'
import { NAlert, NButton, NInput, NSpin } from 'naive-ui'
import { ApiError, errorMessage, isAuthError } from '@/api/client'
import { getTaskForm, taskAction } from '@/api/task'
import { markAuthError } from '@/stores/ui'
import type { FormData, Variable } from '@/api/types'
import ErrorBlock from '@/components/ErrorBlock.vue'
import TaskForm from '@/components/TaskForm.vue'

const props = defineProps<{
  /** 任务 ID */
  taskId: string
}>()

const emit = defineEmits<{
  /** 任务已完成 */
  completed: []
}>()

const loading = ref(false)
const loadError = ref<unknown>(null)
const form = ref<FormData | null>(null)
const noForm = ref(false)
const submitting = ref(false)
const submitError = ref('')
const jsonText = ref('{}')

// 加载任务表单；无表单时进入变量 JSON 完成模式。
// @return 无
async function load(): Promise<void> {
  loading.value = true
  loadError.value = null
  noForm.value = false
  form.value = null
  try {
    form.value = await getTaskForm(props.taskId)
  } catch (err) {
    markAuthError(err)
    // 服务端用 400 + REST-0406 表示任务没有表单，此时允许使用变量 JSON 完成。
    if (err instanceof ApiError && err.code === 'REST-0406') {
      noForm.value = true
    } else {
      loadError.value = err
    }
  } finally {
    loading.value = false
  }
}

// 把表单值转换为变量数组。
// @param values 表单值映射
// @return 变量数组
function toVariables(values: Record<string, unknown>): Variable[] {
  return Object.entries(values).map(([name, value]) => ({ name, value }))
}

// 完成表单任务。
// @param values 表单值映射
// @return 无
async function completeWithValues(values: Record<string, unknown>): Promise<void> {
  submitError.value = ''
  submitting.value = true
  try {
    await taskAction(props.taskId, { action: 'complete', variables: toVariables(values) })
    emit('completed')
  } catch (err) {
    markAuthError(err)
    submitError.value = errorMessage(err)
    if (isAuthError(err)) submitError.value = `${submitError.value}（服务端已启用认证或权限限制）`
  } finally {
    submitting.value = false
  }
}

// 解析变量 JSON 并完成任务。
// @return 无
async function completeWithJSON(): Promise<void> {
  submitError.value = ''
  let parsed: unknown
  try {
    parsed = JSON.parse(jsonText.value)
  } catch {
    submitError.value = '变量 JSON 格式非法，请修正后再完成'
    return
  }
  // 顶层必须是对象，数组或标量无法映射为变量集合。
  if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed)) {
    submitError.value = '变量 JSON 必须是对象，例如 {"approved": true}'
    return
  }
  await completeWithValues(parsed as Record<string, unknown>)
}

onMounted(load)
</script>

<template>
  <n-spin :show="loading">
    <error-block v-if="loadError" :error="loadError" :context="`GET /service/runtime/tasks/${taskId}/form`" @retry="load" />
    <template v-else>
      <div class="row row--between" style="margin-bottom: 10px">
        <div>
          <span class="section-title" style="margin: 0">{{ noForm ? '变量 JSON 完成' : '表单完成' }}</span>
          <span v-if="form?.formKey" class="muted mono" style="margin-left: 8px">formKey: {{ form.formKey }}</span>
        </div>
        <n-button size="tiny" quaternary @click="load">重新加载</n-button>
      </div>
      <n-alert v-if="submitError" type="error" :bordered="false" style="margin-bottom: 12px">
        {{ submitError }}
        <div class="muted" style="margin-top: 4px">提交失败时表单内容会保留，可修正后重试；权限不足请查看服务端原文。</div>
      </n-alert>
      <TaskForm v-if="form" :form="form" :submitting="submitting" submit-text="完成任务" @submit="completeWithValues" />
      <div v-else-if="noForm">
        <div class="muted" style="margin-bottom: 8px">
          该任务未定义引擎表单，可直接提交变量完成。留空表示不写入变量。
        </div>
        <n-input v-model:value="jsonText" type="textarea" :rows="8" placeholder='{"approved": true}' />
        <div class="row" style="justify-content: flex-end; margin-top: 12px">
          <n-button size="small" type="primary" :loading="submitting" @click="completeWithJSON">完成任务</n-button>
        </div>
      </div>
    </template>
  </n-spin>
</template>
