<script setup lang="ts">
// 独立任务创建弹窗：填写任务属性并调用 POST /runtime/tasks。
import { reactive, ref, watch } from 'vue'
import { NAlert, NButton, NForm, NFormItem, NInput, NInputNumber, NModal } from 'naive-ui'
import { createTask } from '@/api/task'
import { errorMessage, isAuthError } from '@/api/client'
import { markAuthError } from '@/stores/ui'

const props = defineProps<{
  /** 是否显示 */
  show: boolean
}>()

const emit = defineEmits<{
  'update:show': [boolean]
  /** 创建成功 */
  created: []
}>()

/** 表单状态 */
const form = reactive({
  name: '',
  assignee: '',
  owner: '',
  description: '',
  priority: 50 as number | null,
  dueDate: '',
  category: '',
  formKey: '',
  parentTaskId: '',
})

const submitting = ref(false)
const error = ref('')

// 重置表单与错误。
// @return 无
function reset(): void {
  form.name = ''
  form.assignee = ''
  form.owner = ''
  form.description = ''
  form.priority = 50
  form.dueDate = ''
  form.category = ''
  form.formKey = ''
  form.parentTaskId = ''
  error.value = ''
}

watch(
  () => props.show,
  (visible) => {
    if (visible) reset()
  },
)

// 关闭弹窗。
// @return 无
function close(): void {
  emit('update:show', false)
}

// 校验并提交创建请求。
// @return 无
async function submit(): Promise<void> {
  error.value = ''
  if (!form.name.trim()) {
    error.value = '任务名称不能为空'
    return
  }
  let dueDate: string | undefined
  if (form.dueDate.trim()) {
    // 到期日必须能被浏览器解析，避免提交非法时间被服务端拒绝。
    if (Number.isNaN(Date.parse(form.dueDate.trim()))) {
      error.value = '到期时间格式非法，请使用 ISO-8601 或 YYYY-MM-DDTHH:mm:ss'
      return
    }
    dueDate = form.dueDate.trim()
  }
  submitting.value = true
  try {
    await createTask({
      name: form.name.trim(),
      assignee: form.assignee.trim() || undefined,
      owner: form.owner.trim() || undefined,
      description: form.description.trim() || undefined,
      priority: form.priority ?? undefined,
      dueDate,
      category: form.category.trim() || undefined,
      formKey: form.formKey.trim() || undefined,
      parentTaskId: form.parentTaskId.trim() || undefined,
    })
    emit('created')
    close()
  } catch (err) {
    markAuthError(err)
    error.value = errorMessage(err)
    if (isAuthError(err)) error.value = `${error.value}（服务端已启用认证或权限限制，本期管理台未提供登录与权限适配）`
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <n-modal
    :show="show"
    preset="card"
    title="新建独立任务"
    style="width: 620px; max-width: 94vw"
    @update:show="emit('update:show', $event)"
  >
    <n-alert v-if="error" type="error" :bordered="false" style="margin-bottom: 12px">{{ error }}</n-alert>
    <n-form label-placement="top">
      <div class="detail-grid">
        <n-form-item label="任务名称" required>
          <n-input v-model:value="form.name" placeholder="例如 人工复核" />
        </n-form-item>
        <n-form-item label="办理人">
          <n-input v-model:value="form.assignee" placeholder="留空为未签收" />
        </n-form-item>
        <n-form-item label="所有者">
          <n-input v-model:value="form.owner" placeholder="可选" />
        </n-form-item>
        <n-form-item label="优先级">
          <n-input-number v-model:value="form.priority" :min="0" :max="1000" style="width: 100%" />
        </n-form-item>
        <n-form-item label="到期时间">
          <n-input v-model:value="form.dueDate" placeholder="2026-11-01T00:00:00Z" />
        </n-form-item>
        <n-form-item label="分类">
          <n-input v-model:value="form.category" placeholder="可选" />
        </n-form-item>
        <n-form-item label="表单 Key">
          <n-input v-model:value="form.formKey" placeholder="可选" />
        </n-form-item>
        <n-form-item label="父任务 ID">
          <n-input v-model:value="form.parentTaskId" placeholder="可选，用于创建子任务" />
        </n-form-item>
      </div>
      <n-form-item label="描述">
        <n-input v-model:value="form.description" type="textarea" :rows="3" placeholder="可选" />
      </n-form-item>
      <div class="muted">独立任务不关联流程定义；流程任务请从流程定义启动实例后产生。</div>
    </n-form>
    <template #footer>
      <div class="row" style="justify-content: flex-end">
        <n-button size="small" quaternary @click="close">取消</n-button>
        <n-button size="small" type="primary" :loading="submitting" @click="submit">创建</n-button>
      </div>
    </template>
  </n-modal>
</template>
