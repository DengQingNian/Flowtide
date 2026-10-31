<script setup lang="ts">
// 审批弹窗：审批意见（必填）+ 流程变量 JSON（可选），待办办理等场景复用。
import { ref, watch } from 'vue'
import { NButton, NForm, NFormItem, NInput, NModal } from 'naive-ui'

/** 提交载荷 */
export interface ApprovalSubmitPayload {
  /** 审批意见 */
  message: string
  /** 流程变量，未填写时为 undefined */
  variables: Record<string, unknown> | undefined
}

const props = withDefaults(
  defineProps<{
    /** 是否展示 */
    show: boolean
    /** 弹窗标题 */
    title?: string
    /** 提交中 */
    submitting?: boolean
    /** 服务端错误文案 */
    error?: string
  }>(),
  { title: '办理任务', submitting: false, error: '' },
)

const emit = defineEmits<{
  'update:show': [boolean]
  submit: [payload: ApprovalSubmitPayload]
}>()

/** 审批意见 */
const message = ref('')
/** 变量 JSON 文本 */
const variablesText = ref('')
/** 本地校验错误 */
const localError = ref('')

// 弹窗打开时重置输入与校验状态。
// @param open 是否打开
// @return 无
watch(
  () => props.show,
  (open) => {
    if (open) {
      message.value = ''
      variablesText.value = ''
      localError.value = ''
    }
  },
)

// parseVariables 把变量 JSON 文本解析为对象；空串视为未填写。
// @return 变量对象或 undefined
// @throws JSON 非法或不是对象时抛错
function parseVariables(): Record<string, unknown> | undefined {
  const text = variablesText.value.trim()
  if (!text) return undefined
  let parsed: unknown
  try {
    parsed = JSON.parse(text)
  } catch (cause) {
    throw new Error(`变量 JSON 解析失败：${cause instanceof Error ? cause.message : String(cause)}`)
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new Error('变量 JSON 需为对象，例如 {"amount": 3}')
  }
  return parsed as Record<string, unknown>
}

// handleSubmit 校验输入并上抛提交事件。
// @return 无
function handleSubmit(): void {
  if (!message.value.trim()) {
    localError.value = '审批意见为必填项'
    return
  }
  let variables: Record<string, unknown> | undefined
  try {
    variables = parseVariables()
  } catch (cause) {
    localError.value = cause instanceof Error ? cause.message : String(cause)
    return
  }
  localError.value = ''
  emit('submit', { message: message.value.trim(), variables })
}
</script>

<template>
  <n-modal
    :show="show"
    preset="card"
    :title="title"
    style="width: 560px"
    :mask-closable="false"
    @update:show="emit('update:show', $event)"
  >
    <n-form label-placement="left" label-width="88">
      <n-form-item label="审批意见" required>
        <n-input v-model:value="message" type="textarea" :rows="3" placeholder="请输入审批意见（必填）" />
      </n-form-item>
      <n-form-item label="流程变量">
        <n-input
          v-model:value="variablesText"
          type="textarea"
          :rows="4"
          placeholder="可选，JSON 对象，例如 {&quot;amount&quot;: 3}"
          class="mono"
        />
      </n-form-item>
    </n-form>
    <div v-if="localError" class="banner banner--error" role="alert" style="margin: 4px 0 0">{{ localError }}</div>
    <div v-else-if="error" class="banner banner--error" role="alert" style="margin: 4px 0 0">{{ error }}</div>
    <template #footer>
      <div class="row" style="justify-content: flex-end">
        <n-button size="small" quaternary :disabled="submitting" @click="emit('update:show', false)">取消</n-button>
        <n-button size="small" type="primary" :loading="submitting" @click="handleSubmit">提交</n-button>
      </div>
    </template>
  </n-modal>
</template>
