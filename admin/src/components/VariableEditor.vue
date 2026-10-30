<script setup lang="ts">
// 变量编辑器：新增或修改变量，支持类型切换与 JSON 语法校验。
import { computed, ref, watch } from 'vue'
import { NButton, NForm, NFormItem, NInput, NInputNumber, NModal, NSelect, NSwitch } from 'naive-ui'
import type { Variable, VariableInstance } from '@/api/types'

const props = withDefaults(
  defineProps<{
    /** 是否显示弹窗 */
    show: boolean
    /** 编辑已有变量时传入 */
    variable?: VariableInstance | null
    /** 是否允许修改名称 */
    allowNameEdit?: boolean
  }>(),
  { variable: null, allowNameEdit: true },
)

const emit = defineEmits<{
  'update:show': [boolean]
  save: [Variable]
}>()

const typeOptions = [
  { label: '字符串 string', value: 'string' },
  { label: '整数 long', value: 'long' },
  { label: '小数 double', value: 'double' },
  { label: '布尔 boolean', value: 'boolean' },
  { label: '日期 date', value: 'date' },
  { label: 'JSON json', value: 'json' },
]

const name = ref('')
const type = ref('string')
const textValue = ref('')
const numberValue = ref<number | null>(null)
const boolValue = ref(false)
const error = ref('')
const scope = ref('')

/** 是否编辑模式 */
const editing = computed(() => Boolean(props.variable))

// 根据已有变量初始化表单。
// @return 无
function reset(): void {
  const source = props.variable
  name.value = source?.name ?? ''
  type.value = source?.type && typeOptions.some((item) => item.value === source.type) ? source.type : 'string'
  scope.value = source?.scope ?? ''
  error.value = ''
  const value = source?.value
  textValue.value = value === null || value === undefined ? '' : typeof value === 'string' ? value : JSON.stringify(value, null, 2)
  numberValue.value = typeof value === 'number' ? value : null
  boolValue.value = value === true
}

watch(
  () => props.show,
  (visible) => {
    if (visible) reset()
  },
  { immediate: true },
)

watch(() => props.variable, () => {
  if (props.show) reset()
})

// 校验并构造变量值。
// @return 变量对象或 null
function buildVariable(): Variable | null {
  error.value = ''
  if (!name.value.trim()) {
    error.value = '变量名不能为空'
    return null
  }
  let value: unknown
  if (type.value === 'long' || type.value === 'double') {
    if (numberValue.value === null || !Number.isFinite(numberValue.value)) {
      error.value = '请输入合法数字'
      return null
    }
    value = type.value === 'long' ? Math.trunc(numberValue.value) : numberValue.value
  } else if (type.value === 'boolean') {
    value = boolValue.value
  } else if (type.value === 'json') {
    try {
      value = textValue.value.trim() === '' ? null : JSON.parse(textValue.value)
    } catch {
      error.value = 'JSON 格式非法，请修正后再保存'
      return null
    }
  } else {
    value = textValue.value
  }
  return { name: name.value.trim(), type: type.value, value, scope: scope.value || undefined }
}

// 提交变量。
// @return 无
function submit(): void {
  const variable = buildVariable()
  if (!variable) return
  emit('save', variable)
  emit('update:show', false)
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
    :title="editing ? '修改变量' : '新增变量'"
    style="width: 520px; max-width: 92vw"
    @update:show="emit('update:show', $event)"
  >
    <n-form label-placement="top">
      <n-form-item label="变量名">
        <n-input v-model:value="name" :disabled="!allowNameEdit" placeholder="例如 orderId" />
      </n-form-item>
      <n-form-item label="类型">
        <n-select v-model:value="type" :options="typeOptions" />
      </n-form-item>
      <n-form-item label="值">
        <n-input v-if="type === 'string'" v-model:value="textValue" type="textarea" :rows="3" placeholder="字符串值" />
        <n-input v-else-if="type === 'date'" v-model:value="textValue" placeholder="YYYY-MM-DD 或 RFC3339" />
        <n-input-number v-else-if="type === 'long' || type === 'double'" v-model:value="numberValue" style="width: 100%" />
        <n-switch v-else-if="type === 'boolean'" v-model:value="boolValue" />
        <n-input v-else v-model:value="textValue" type="textarea" :rows="6" placeholder="合法 JSON，例如 {&quot;a&quot;: 1}" />
      </n-form-item>
      <div v-if="error" class="text-danger" role="alert">{{ error }}</div>
    </n-form>
    <template #footer>
      <div class="row" style="justify-content: flex-end">
        <n-button size="small" quaternary @click="close">取消</n-button>
        <n-button size="small" type="primary" @click="submit">保存</n-button>
      </div>
    </template>
  </n-modal>
</template>