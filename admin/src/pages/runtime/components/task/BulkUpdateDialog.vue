<script setup lang="ts">
// 批量任务操作弹窗：改派走 PUT /runtime/tasks，删除逐个调用 DELETE 并逐条反馈。
import { computed, ref, watch } from 'vue'
import { NAlert, NButton, NCheckbox, NInput, NModal, NSelect } from 'naive-ui'
import { bulkUpdateTasks, deleteTask, type Task } from '@/api/task'
import { errorMessage } from '@/api/client'
import { markAuthError } from '@/stores/ui'

const props = defineProps<{
  /** 是否显示 */
  show: boolean
  /** 已选任务 */
  tasks: Task[]
}>()

const emit = defineEmits<{
  'update:show': [boolean]
  /** 操作结束，需要刷新列表 */
  done: []
}>()

/** 操作模式 */
const mode = ref<'assign' | 'delete'>('assign')
/** 改派目标 */
const assignee = ref('')
/** 删除原因 */
const reason = ref('')
/** 是否级联历史 */
const cascadeHistory = ref(false)
const busy = ref(false)
const error = ref('')

/** 单条结果 */
interface BulkResult {
  id: string
  name: string
  ok: boolean
  message: string
}

const results = ref<BulkResult[]>([])

const modeOptions = [
  { label: '改派办理人', value: 'assign' },
  { label: '删除任务', value: 'delete' },
]

/** 成功条数 */
const successCount = computed(() => results.value.filter((item) => item.ok).length)
/** 失败条数 */
const failureCount = computed(() => results.value.filter((item) => !item.ok).length)

watch(
  () => props.show,
  (visible) => {
    if (!visible) return
    mode.value = 'assign'
    assignee.value = ''
    reason.value = ''
    cascadeHistory.value = false
    results.value = []
    error.value = ''
    busy.value = false
  },
)

// 关闭弹窗。
// @return 无
function close(): void {
  emit('update:show', false)
}

// 改派所选任务，批量接口失败时整批标记失败。
// @return 无
async function assignSelected(): Promise<void> {
  const target = assignee.value.trim()
  if (!target) {
    error.value = '请输入改派后的办理人'
    return
  }
  await bulkUpdateTasks({ taskIds: props.tasks.map((task) => task.id), assignee: target })
  results.value = props.tasks.map((task) => ({ id: task.id, name: task.name, ok: true, message: `已改派给 ${target}` }))
}

// 逐个删除所选任务并记录每条结果。
// @return 无
async function deleteSelected(): Promise<void> {
  const output: BulkResult[] = []
  for (const task of props.tasks) {
    try {
      await deleteTask(task.id, { cascadeHistory: cascadeHistory.value, deleteReason: reason.value.trim() || undefined })
      output.push({ id: task.id, name: task.name, ok: true, message: '已删除' })
    } catch (err) {
      markAuthError(err)
      output.push({ id: task.id, name: task.name, ok: false, message: errorMessage(err) })
    }
  }
  results.value = output
}

// 执行批量操作并反馈结果。
// @return 无
async function submit(): Promise<void> {
  error.value = ''
  results.value = []
  if (props.tasks.length === 0) {
    error.value = '请先选择任务'
    return
  }
  busy.value = true
  try {
    if (mode.value === 'assign') await assignSelected()
    else await deleteSelected()
    emit('done')
  } catch (err) {
    markAuthError(err)
    error.value = errorMessage(err)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <n-modal
    :show="show"
    preset="card"
    title="批量操作所选任务"
    style="width: 640px; max-width: 94vw"
    @update:show="emit('update:show', $event)"
  >
    <n-alert v-if="error" type="error" :bordered="false" style="margin-bottom: 12px">{{ error }}</n-alert>
    <div class="detail-grid">
      <div class="detail-item">
        <span class="detail-item__label">已选任务</span>
        <span class="detail-item__value">{{ tasks.length }} 条</span>
      </div>
      <div class="detail-item">
        <span class="detail-item__label">操作</span>
        <n-select v-model:value="mode" :options="modeOptions" style="width: 100%" />
      </div>
      <div v-if="mode === 'assign'" class="detail-item">
        <span class="detail-item__label">改派给</span>
        <n-input v-model:value="assignee" placeholder="办理人 ID" />
      </div>
      <div v-if="mode === 'delete'" class="detail-item">
        <span class="detail-item__label">删除原因</span>
        <n-input v-model:value="reason" placeholder="可选" />
      </div>
    </div>
    <div v-if="mode === 'delete'" class="row" style="margin-top: 12px">
      <n-checkbox v-model:checked="cascadeHistory">同时删除历史记录（cascadeHistory）</n-checkbox>
    </div>
    <div class="muted" style="margin-top: 8px">
      {{ mode === 'assign' ? '批量改派调用 PUT /service/runtime/tasks，服务端逐条更新，遇到失败会中断。' : '批量删除会逐个调用删除接口，并在下方逐条列出结果；级联历史不可恢复。' }}
    </div>
    <div v-if="results.length > 0" style="margin-top: 14px">
      <div class="section-title">执行结果（成功 {{ successCount }} 条，失败 {{ failureCount }} 条）</div>
      <table class="kv-table">
        <thead>
          <tr><th>任务</th><th>结果</th></tr>
        </thead>
        <tbody>
          <tr v-for="item in results" :key="item.id">
            <td>{{ item.name }} <span class="mono muted">{{ item.id }}</span></td>
            <td :class="item.ok ? 'text-success' : 'text-danger'">{{ item.message }}</td>
          </tr>
        </tbody>
      </table>
    </div>
    <template #footer>
      <div class="row" style="justify-content: flex-end">
        <n-button size="small" quaternary @click="close">关闭</n-button>
        <n-button size="small" :type="mode === 'delete' ? 'error' : 'primary'" :loading="busy" @click="submit">
          {{ mode === 'delete' ? '确认删除' : '确认改派' }}
        </n-button>
      </div>
    </template>
  </n-modal>
</template>
