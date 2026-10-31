<script setup lang="ts">
// 工作流页面通用表单弹窗外壳：标题、表单插槽、错误条与取消/保存按钮。
import { NButton, NForm, NModal } from 'naive-ui'

withDefaults(
  defineProps<{
    /** 是否展示 */
    show: boolean
    /** 弹窗标题 */
    title: string
    /** 宽度像素 */
    width?: number
    /** 提交中 */
    submitting?: boolean
    /** 提交错误文案 */
    error?: string
    /** 确认按钮文本 */
    submitLabel?: string
  }>(),
  { width: 620, submitting: false, error: '', submitLabel: '保存' },
)

const emit = defineEmits<{
  'update:show': [boolean]
  submit: []
}>()
</script>

<template>
  <n-modal
    :show="show"
    preset="card"
    :title="title"
    :style="{ width: `${width}px` }"
    :mask-closable="false"
    @update:show="emit('update:show', $event)"
  >
    <n-form label-placement="left" label-width="104">
      <slot />
    </n-form>
    <div v-if="error" class="banner banner--error" role="alert" style="margin: 4px 0 0">{{ error }}</div>
    <template #footer>
      <div class="row" style="justify-content: flex-end">
        <n-button size="small" quaternary :disabled="submitting" @click="emit('update:show', false)">取消</n-button>
        <n-button size="small" type="primary" :loading="submitting" @click="emit('submit')">{{ submitLabel }}</n-button>
      </div>
    </template>
  </n-modal>
</template>
