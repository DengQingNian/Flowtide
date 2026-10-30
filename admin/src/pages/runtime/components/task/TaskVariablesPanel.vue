<script setup lang="ts">
// 任务变量面板：列表、增改删、原始数据查看与下载。
import { onMounted, ref } from 'vue'
import { NAlert, NButton, NModal, NSpin } from 'naive-ui'
import { errorMessage } from '@/api/client'
import {
  createTaskVariable,
  deleteTaskVariable,
  getTaskVariableData,
  listTaskVariables,
  updateTaskVariable,
} from '@/api/task'
import { markAuthError } from '@/stores/ui'
import type { Variable, VariableInstance } from '@/api/types'
import { downloadBlob } from '@/lib/download'
import ErrorBlock from '@/components/ErrorBlock.vue'
import VariableEditor from '@/components/VariableEditor.vue'
import VariableTable from '@/components/VariableTable.vue'

const props = defineProps<{
  /** 任务 ID */
  taskId: string
}>()

const variables = ref<VariableInstance[]>([])
const loading = ref(false)
const error = ref<unknown>(null)
const feedback = ref('')
const editorShow = ref(false)
const editing = ref<VariableInstance | null>(null)
const pendingRemove = ref<VariableInstance | null>(null)
const removing = ref(false)
const rawShow = ref(false)
const rawName = ref('')
const rawText = ref('')
const rawLoading = ref(false)
const rawError = ref('')

// 根据变量值推断类型，任务变量端点不回显 type。
// @param value 变量值
// @return 类型名
function inferType(value: unknown): string {
  if (typeof value === 'boolean') return 'boolean'
  if (typeof value === 'number') return Number.isInteger(value) ? 'long' : 'double'
  if (value !== null && typeof value === 'object') return 'json'
  return 'string'
}

// 加载任务变量。
// @return 无
async function load(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    const items = await listTaskVariables(props.taskId)
    variables.value = items.map((item) => ({ ...item, type: item.type || inferType(item.value) }))
  } catch (err) {
    markAuthError(err)
    error.value = err
    variables.value = []
  } finally {
    loading.value = false
  }
}

// 打开新增变量弹窗。
// @return 无
function openCreate(): void {
  editing.value = null
  editorShow.value = true
}

// 打开编辑变量弹窗。
// @param variable 变量
// @return 无
function openEdit(variable: VariableInstance): void {
  editing.value = variable
  editorShow.value = true
}

// 保存变量，编辑时变量名不可改。
// @param variable 编辑器产出的变量
// @return 无
async function save(variable: Variable): Promise<void> {
  feedback.value = ''
  try {
    if (editing.value) await updateTaskVariable(props.taskId, editing.value.name, variable.value)
    else await createTaskVariable(props.taskId, variable)
    feedback.value = editing.value ? '变量已更新' : '变量已创建'
    await load()
  } catch (err) {
    markAuthError(err)
    feedback.value = `保存失败：${errorMessage(err)}`
    error.value = err
  }
}

// 确认删除变量。
// @return 无
async function confirmRemove(): Promise<void> {
  const target = pendingRemove.value
  if (!target) return
  removing.value = true
  feedback.value = ''
  try {
    await deleteTaskVariable(props.taskId, target.name)
    feedback.value = '变量已删除'
    pendingRemove.value = null
    await load()
  } catch (err) {
    markAuthError(err)
    feedback.value = `删除失败：${errorMessage(err)}`
  } finally {
    removing.value = false
  }
}

// 查看并下载变量原始内容。
// @param variable 变量
// @return 无
async function openRaw(variable: VariableInstance): Promise<void> {
  rawName.value = variable.name
  rawText.value = ''
  rawError.value = ''
  rawShow.value = true
  rawLoading.value = true
  try {
    const blob = await getTaskVariableData(props.taskId, variable.name)
    rawText.value = await blob.text()
  } catch (err) {
    markAuthError(err)
    rawError.value = errorMessage(err)
  } finally {
    rawLoading.value = false
  }
}

onMounted(load)

// 下载当前原始数据。
// @return 无
function downloadRaw(): void {
  if (!rawText.value) return
  downloadBlob(`${rawName.value}.bin`, new Blob([rawText.value]))
}
</script>

<template>
  <div>
    <div class="row row--between" style="margin-bottom: 10px">
      <span class="section-title" style="margin: 0">任务局部变量</span>
      <n-button size="small" type="primary" @click="openCreate">新增变量</n-button>
    </div>
    <n-alert v-if="feedback" :type="feedback.includes('失败') ? 'error' : 'success'" :bordered="false" style="margin-bottom: 10px">
      {{ feedback }}
    </n-alert>
    <error-block v-if="error && variables.length === 0" :error="error" :context="`GET /service/runtime/tasks/${taskId}/variables`" @retry="load" />
    <n-spin :show="loading">
      <VariableTable :variables="variables" :loading="loading" @edit="openEdit" @remove="pendingRemove = $event" @raw="openRaw" />
    </n-spin>

    <VariableEditor v-model:show="editorShow" :variable="editing" :allow-name-edit="!editing" @save="save" />

    <n-modal :show="pendingRemove !== null" preset="card" title="删除变量" style="width: 420px; max-width: 92vw" @update:show="pendingRemove = null">
      <div>确认删除变量 <span class="mono">{{ pendingRemove?.name }}</span>？删除后任务运行期间将无法再读取该变量。</div>
      <template #footer>
        <div class="row" style="justify-content: flex-end">
          <n-button size="small" quaternary @click="pendingRemove = null">取消</n-button>
          <n-button size="small" type="error" :loading="removing" @click="confirmRemove">确认删除</n-button>
        </div>
      </template>
    </n-modal>

    <n-modal v-model:show="rawShow" preset="card" :title="`原始数据：${rawName}`" style="width: 720px; max-width: 94vw">
      <n-alert v-if="rawError" type="error" :bordered="false">{{ rawError }}</n-alert>
      <n-spin v-else :show="rawLoading">
        <pre class="code-block">{{ rawText || '（空内容）' }}</pre>
      </n-spin>
      <template #footer>
        <div class="row" style="justify-content: flex-end">
          <n-button size="small" quaternary @click="rawShow = false">关闭</n-button>
          <n-button size="small" :disabled="!rawText" @click="downloadRaw">下载</n-button>
        </div>
      </template>
    </n-modal>
  </div>
</template>