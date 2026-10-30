<script setup lang="ts">
// 流程定义启动表单：读取 /start-form 并按字段类型渲染，提交时把字段值交给页面启动实例。
import { onMounted, ref } from 'vue'
import { NAlert, NButton, NSpin } from 'naive-ui'
import ErrorBlock from '@/components/ErrorBlock.vue'
import EmptyState from '@/components/EmptyState.vue'
import TaskForm from '@/components/TaskForm.vue'
import { ApiError } from '@/api/client'
import { getProcessDefinitionStartForm } from '@/api/repository'
import type { FormData as FormDataType } from '@/api/types'
import { markAuthError } from '@/stores/ui'

const props = withDefaults(
  defineProps<{
    /** 流程定义 ID */
    definitionId: string
    /** 提交中 */
    submitting?: boolean
  }>(),
  { submitting: false },
)

const emit = defineEmits<{ submit: [Record<string, unknown>] }>()

const form = ref<FormDataType | null>(null)
const loading = ref(false)
const error = ref<unknown>(null)
const noFields = ref(false)

// 读取启动表单。
// @return 无
async function load(): Promise<void> {
  loading.value = true
  error.value = null
  noFields.value = false
  try {
    const data = await getProcessDefinitionStartForm(props.definitionId)
    form.value = data
    noFields.value = data.formProperties.length === 0
  } catch (cause) {
    markAuthError(cause)
    // 定义没有开始事件或不存在时按无表单展示，其余错误保留原始信息
    if (cause instanceof ApiError && (cause.status === 400 || cause.status === 404)) {
      form.value = null
      noFields.value = true
    } else {
      error.value = cause
    }
  } finally {
    loading.value = false
  }
}

onMounted(() => void load())
</script>

<template>
  <div>
    <ErrorBlock v-if="error" :error="error" context="GET /service/repository/process-definitions/{id}/start-form" @retry="load" />
    <div v-else-if="loading" class="row" style="justify-content: center; padding: 24px">
      <n-spin size="small" />
    </div>
    <template v-else-if="form && !noFields">
      <n-alert v-if="form.formKey" type="info" :bordered="false" style="margin-bottom: 10px">
        表单 Key：{{ form.formKey }}
      </n-alert>
      <TaskForm :form="form" :submitting="submitting" submit-text="按表单启动实例" @submit="emit('submit', $event)" />
    </template>
    <template v-else>
      <EmptyState title="该定义没有可渲染的启动表单" description="可直接使用顶部“启动实例”填写业务 Key 与变量。" />
      <div class="row" style="justify-content: flex-end">
        <n-button size="small" quaternary :loading="loading" @click="load">重新读取</n-button>
      </div>
    </template>
  </div>
</template>
