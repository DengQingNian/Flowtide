<script setup lang="ts">
// 启动实例弹窗：选择定义、业务 Key、名称与变量 JSON。
import { computed, ref, watch } from 'vue'
import { NButton, NForm, NFormItem, NInput, NModal, NSelect } from 'naive-ui'

/** 可选流程定义 */
export interface ProcessDefinitionOption {
  id: string
  name?: string
  key: string
  version?: number
}

/** 启动实例提交载荷 */
export interface StartInstancePayload {
  processDefinitionId?: string
  processDefinitionKey?: string
  businessKey?: string
  name?: string
  variables: Record<string, unknown>
}

const props = withDefaults(
  defineProps<{
    /** 是否显示 */
    show: boolean
    /** 可选流程定义 */
    definitions?: ProcessDefinitionOption[]
    /** 预选定义 ID */
    initialDefinitionId?: string
    /** 预选定义 Key，定义列表为空时使用 */
    initialDefinitionKey?: string
    /** 提交中 */
    submitting?: boolean
  }>(),
  { definitions: () => [], initialDefinitionId: '', initialDefinitionKey: '', submitting: false },
)

const emit = defineEmits<{
  'update:show': [boolean]
  submit: [StartInstancePayload]
}>()

const definitionId = ref('')
const businessKey = ref('')
const name = ref('')
const variablesText = ref('{}')
const error = ref('')

const options = computed(() =>
  props.definitions.map((definition) => ({
    label: `${definition.name || definition.key} · ${definition.key} v${definition.version ?? '—'}`,
    value: definition.id,
  })),
)

// 重置表单。
// @return 无
function reset(): void {
  definitionId.value = props.initialDefinitionId || props.definitions[0]?.id || ''
  businessKey.value = ''
  name.value = ''
  variablesText.value = '{}'
  error.value = ''
}

watch(
  () => props.show,
  (visible) => {
    if (visible) reset()
  },
  { immediate: true },
)

// 解析变量 JSON。
// @return 变量对象或 null
function parseVariables(): Record<string, unknown> | null {
  error.value = ''
  const text = variablesText.value.trim()
  if (!text) return {}
  try {
    const parsed = JSON.parse(text) as unknown
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      error.value = '变量必须是 JSON 对象，例如 {"amount": 100}'
      return null
    }
    return parsed as Record<string, unknown>
  } catch {
    error.value = '变量 JSON 格式非法，请修正后再提交'
    return null
  }
}

// 提交启动请求。
// @return 无
function submit(): void {
  const variables = parseVariables()
  if (!variables) return
  if (!definitionId.value && !props.initialDefinitionKey) {
    error.value = '请选择流程定义'
    return
  }
  emit('submit', {
    processDefinitionId: definitionId.value || undefined,
    processDefinitionKey: definitionId.value ? undefined : props.initialDefinitionKey,
    businessKey: businessKey.value.trim() || undefined,
    name: name.value.trim() || undefined,
    variables,
  })
}

// 关闭弹窗。
// @return 无
function close(): void {
  emit('update:show', false)
}
</script>

<template>
  <n-modal
    :show="show"
    preset="card"
    title="启动流程实例"
    style="width: 620px; max-width: 94vw"
    @update:show="emit('update:show', $event)"
  >
    <n-form label-placement="top">
      <n-form-item label="流程定义">
        <n-select v-model:value="definitionId" :options="options" filterable placeholder="选择流程定义" />
      </n-form-item>
      <n-form-item label="业务 Key">
        <n-input v-model:value="businessKey" placeholder="可选，用于关联业务单据" />
      </n-form-item>
      <n-form-item label="实例名称">
        <n-input v-model:value="name" placeholder="可选" />
      </n-form-item>
      <n-form-item label="变量（JSON 对象）">
        <n-input v-model:value="variablesText" type="textarea" :rows="6" placeholder="{&quot;amount&quot;: 100}" />
      </n-form-item>
      <div v-if="error" class="text-danger" role="alert">{{ error }}</div>
    </n-form>
    <template #footer>
      <div class="row" style="justify-content: flex-end">
        <n-button size="small" quaternary :disabled="submitting" @click="close">取消</n-button>
        <n-button size="small" type="primary" :loading="submitting" @click="submit">启动</n-button>
      </div>
    </template>
  </n-modal>
</template>