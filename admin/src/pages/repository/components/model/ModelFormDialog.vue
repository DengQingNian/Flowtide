<script setup lang="ts">
// 模型元信息表单弹窗：新建与编辑共用，校验必填项并展示服务端错误。
import { computed, reactive, ref, watch } from 'vue'
import { NButton, NForm, NFormItem, NInput, NModal } from 'naive-ui'
import ErrorBlock from '@/components/ErrorBlock.vue'
import type { Model } from '@/api/models'

/** 表单提交值 */
interface FormPayload {
  /** 名称 */
  name: string
  /** Key，编辑时服务端允许修改，但页面不提交 */
  key?: string
  /** 分类 */
  category?: string
  /** 元信息 */
  metaInfo?: string
}

const props = withDefaults(
  defineProps<{
    /** 是否打开 */
    open: boolean
    /** 模式 */
    mode: 'create' | 'edit'
    /** 编辑目标模型 */
    model?: Model | null
    /** 提交中 */
    loading?: boolean
    /** 服务端错误 */
    error?: unknown
  }>(),
  { model: null, loading: false },
)

const emit = defineEmits<{
  'update:open': [boolean]
  submit: [FormPayload]
}>()

/** 表单字段 */
const form = reactive({ name: '', key: '', category: '', metaInfo: '' })
/** 名称校验错误 */
const nameError = ref('')
/** Key 校验错误 */
const keyError = ref('')

/** 弹窗标题 */
const title = computed(() => (props.mode === 'create' ? '新建模型' : '编辑模型'))

// 用当前模型填充表单。
// @return 无
function fillForm(): void {
  form.name = props.mode === 'edit' ? props.model?.name ?? '' : ''
  form.key = props.mode === 'edit' ? props.model?.key ?? '' : ''
  form.category = props.model?.category ?? ''
  form.metaInfo = props.model?.metaInfo ?? ''
  nameError.value = ''
  keyError.value = ''
}

// 校验必填字段并提交。
// @return 无
function submit(): void {
  nameError.value = form.name.trim() ? '' : '请输入模型名称'
  // 编辑时 Key 不可改，仅在新建时校验
  keyError.value = props.mode === 'create' && !form.key.trim() ? '请输入模型 Key' : ''
  if (nameError.value || keyError.value) return
  emit('submit', {
    name: form.name.trim(),
    key: form.key.trim(),
    category: form.category.trim(),
    metaInfo: form.metaInfo,
  })
}

// 关闭弹窗。
// @param value 是否打开
// @return 无
function updateOpen(value: boolean): void {
  emit('update:open', value)
}

watch(
  () => props.open,
  (open) => {
    if (open) fillForm()
  },
)
</script>

<template>
  <n-modal
    :show="open"
    preset="card"
    :title="title"
    style="width: 520px; max-width: calc(100vw - 32px)"
    :mask-closable="!loading"
    @update:show="updateOpen"
  >
    <n-form label-placement="top" @submit.prevent>
      <n-form-item label="名称" required :validation-status="nameError ? 'error' : undefined" :feedback="nameError">
        <n-input v-model:value="form.name" placeholder="例如：请假流程" :disabled="loading" />
      </n-form-item>
      <n-form-item
        label="Key"
        :required="mode === 'create'"
        :validation-status="keyError ? 'error' : undefined"
        :feedback="keyError || (mode === 'edit' ? 'Key 与 BPMN 流程 ID 绑定，创建后不支持在此修改' : '建议使用英文、数字与下划线，例如 leave')"
      >
        <n-input v-model:value="form.key" placeholder="例如：leave" :disabled="loading || mode === 'edit'" />
      </n-form-item>
      <n-form-item label="分类">
        <n-input v-model:value="form.category" placeholder="例如：hr" :disabled="loading" />
      </n-form-item>
      <n-form-item label="元信息">
        <n-input
          v-model:value="form.metaInfo"
          type="textarea"
          :autosize="{ minRows: 3, maxRows: 6 }"
          placeholder="可选，JSON 文本或说明"
          :disabled="loading"
        />
      </n-form-item>
    </n-form>
    <ErrorBlock v-if="error" :error="error" />
    <template #footer>
      <div class="row" style="justify-content: flex-end">
        <n-button size="small" quaternary :disabled="loading" @click="updateOpen(false)">取消</n-button>
        <n-button size="small" type="primary" :loading="loading" @click="submit">
          {{ mode === 'create' ? '创建' : '保存' }}
        </n-button>
      </div>
    </template>
  </n-modal>
</template>
