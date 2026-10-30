<script setup lang="ts">
// 快速签收弹窗：为未签收任务指定办理人并调用 claim 动作。
import { ref, watch } from 'vue'
import { NButton, NInput, NModal } from 'naive-ui'
import { errorMessage } from '@/api/client'
import { taskAction, type Task } from '@/api/task'
import { markAuthError } from '@/stores/ui'

const props = defineProps<{
  /** 是否显示 */
  show: boolean
  /** 目标任务 */
  task: Task | null
}>()

const emit = defineEmits<{
  'update:show': [boolean]
  /** 签收成功 */
  done: []
}>()

const assignee = ref('')
const error = ref('')
const submitting = ref(false)

watch(
  () => props.show,
  (visible) => {
    if (!visible) return
    assignee.value = ''
    error.value = ''
    submitting.value = false
  },
)

// 提交签收。
// @return 无
async function submit(): Promise<void> {
  const task = props.task
  if (!task) return
  error.value = ''
  if (!assignee.value.trim()) {
    error.value = '请输入签收人'
    return
  }
  submitting.value = true
  try {
    await taskAction(task.id, { action: 'claim', assignee: assignee.value.trim() })
    emit('done')
    emit('update:show', false)
  } catch (err) {
    markAuthError(err)
    error.value = errorMessage(err)
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <n-modal :show="show" preset="card" title="快速签收" style="width: 440px; max-width: 92vw" @update:show="emit('update:show', $event)">
    <div class="muted" style="margin-bottom: 10px">任务：{{ task?.name || '—' }}</div>
    <div class="filter-bar__item">
      <span class="filter-bar__label">签收人</span>
      <n-input v-model:value="assignee" placeholder="用户 ID" @keyup.enter="submit" />
    </div>
    <div v-if="error" class="text-danger" role="alert" style="margin-top: 8px">{{ error }}</div>
    <template #footer>
      <div class="row" style="justify-content: flex-end">
        <n-button size="small" quaternary @click="emit('update:show', false)">取消</n-button>
        <n-button size="small" type="primary" :loading="submitting" @click="submit">确认签收</n-button>
      </div>
    </template>
  </n-modal>
</template>
