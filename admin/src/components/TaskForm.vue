<script setup lang="ts">
// 任务/启动表单渲染：按表单定义生成字段、校验必填并提交变量。
import { computed, reactive, ref, watch } from 'vue'
import { NButton, NForm, NFormItem, NInput, NInputNumber, NSelect, NSwitch } from 'naive-ui'
import type { FormData, FormProperty } from '@/api/types'

const props = withDefaults(
  defineProps<{
    /** 表单定义 */
    form: FormData | null
    /** 提交中 */
    submitting?: boolean
    /** 提交按钮文本 */
    submitText?: string
  }>(),
  { submitting: false, submitText: '提交' },
)

const emit = defineEmits<{ submit: [Record<string, unknown>] }>()

const values = reactive<Record<string, unknown>>({})
const errors = reactive<Record<string, string>>({})
const localError = ref('')

/** 可读字段 */
const properties = computed(() => (props.form?.formProperties ?? []).filter((item) => item.readable !== false))

// 根据表单定义重置字段值。
// @return 无
function reset(): void {
  for (const key of Object.keys(values)) delete values[key]
  for (const key of Object.keys(errors)) delete errors[key]
  for (const property of properties.value) {
    values[property.id] = property.value ?? defaultFor(property)
  }
  localError.value = ''
}

// 返回字段类型的默认值。
// @param property 表单字段
// @return 默认值
function defaultFor(property: FormProperty): unknown {
  const type = (property.type ?? 'string').toLowerCase()
  if (type === 'boolean') return false
  if (type === 'long' || type === 'double') return null
  return ''
}

watch(
  () => props.form,
  () => reset(),
  { immediate: true, deep: false },
)

// 判断字段是否为数字类型。
// @param property 表单字段
// @return 是否数字
function isNumber(property: FormProperty): boolean {
  const type = (property.type ?? '').toLowerCase()
  return type === 'long' || type === 'double' || type === 'integer'
}

// 判断字段是否为布尔类型。
// @param property 表单字段
// @return 是否布尔
function isBoolean(property: FormProperty): boolean {
  return (property.type ?? '').toLowerCase() === 'boolean'
}

// 判断字段是否为枚举类型。
// @param property 表单字段
// @return 是否枚举
function isEnum(property: FormProperty): boolean {
  return Boolean(property.enumValues && property.enumValues.length > 0)
}

// 判断字段是否只读。
// @param property 表单字段
// @return 是否只读
function isReadonly(property: FormProperty): boolean {
  return property.writable === false
}

// 校验并提交表单值。
// @return 无
function submit(): void {
  localError.value = ''
  for (const key of Object.keys(errors)) delete errors[key]
  for (const property of properties.value) {
    if (property.required && (values[property.id] === '' || values[property.id] === null || values[property.id] === undefined)) {
      errors[property.id] = `${property.name || property.id} 为必填项`
    }
  }
  if (Object.keys(errors).length > 0) {
    localError.value = '请补全表单必填项'
    return
  }
  emit('submit', { ...values })
}
</script>

<template>
  <div>
    <n-form v-if="properties.length > 0" label-placement="top">
      <n-form-item v-for="property in properties" :key="property.id" :label="property.name || property.id" :required="property.required">
        <n-select
          v-if="isEnum(property)"
          v-model:value="values[property.id] as string | number | null"
          :options="(property.enumValues ?? []).map((item) => ({ label: item.name || item.id, value: item.id }))"
          :disabled="isReadonly(property)"
          :placeholder="property.required ? '请选择' : '可选'"
        />
        <n-switch
          v-else-if="isBoolean(property)"
          v-model:value="values[property.id] as boolean"
          :disabled="isReadonly(property)"
        />
        <n-input-number
          v-else-if="isNumber(property)"
          v-model:value="values[property.id] as number | null"
          style="width: 100%"
          :disabled="isReadonly(property)"
        />
        <n-input
          v-else
          v-model:value="values[property.id] as string"
          :type="(property.type ?? '').toLowerCase() === 'json' ? 'textarea' : 'text'"
          :rows="4"
          :disabled="isReadonly(property)"
          :placeholder="property.type === 'date' ? 'YYYY-MM-DD' : ''"
        />
        <div v-if="errors[property.id]" class="text-danger" role="alert">{{ errors[property.id] }}</div>
      </n-form-item>
    </n-form>
    <div v-else class="muted">该流程未定义表单字段，可直接提交或通过变量编辑器补充。</div>
    <div v-if="localError" class="text-danger" role="alert" style="margin-top: 8px">{{ localError }}</div>
    <div class="row" style="justify-content: flex-end; margin-top: 14px">
      <n-button size="small" type="primary" :loading="submitting" @click="submit">{{ submitText }}</n-button>
    </div>
  </div>
</template>