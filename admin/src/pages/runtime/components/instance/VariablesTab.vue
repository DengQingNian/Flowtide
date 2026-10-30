<script setup lang="ts">
// 实例变量：列表、按名搜索、增删改与原始数据查看/下载。
import { computed, onMounted, ref } from 'vue'
import { NButton, NInput, NModal, NSelect, NTag } from 'naive-ui'
import type { Variable } from '@/api/types'
import {
  createInstanceVariable,
  deleteInstanceVariable,
  getInstanceVariableData,
  listInstanceVariables,
  updateInstanceVariable,
  type RuntimeVariable,
} from '@/api/runtime'
import { errorMessage } from '@/api/client'
import { downloadBlob } from '@/lib/download'
import { markAuthError } from '@/stores/ui'
import ErrorBlock from '@/components/ErrorBlock.vue'
import VariableTable from '@/components/VariableTable.vue'
import VariableEditor from '@/components/VariableEditor.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'

const props = defineProps<{
  /** 实例 ID */
  instanceId: string
}>()

const variables = ref<RuntimeVariable[]>([])
const keyword = ref('')
const loading = ref(false)
const error = ref<unknown>(null)
const actionError = ref('')
const notice = ref('')

const showEditor = ref(false)
const editing = ref<RuntimeVariable | null>(null)
const saving = ref(false)

const showDelete = ref(false)
const pendingDelete = ref<RuntimeVariable | null>(null)
const deleting = ref(false)

const showRaw = ref(false)
const rawName = ref('')
const rawType = ref('')
const rawText = ref('')
const rawBinary = ref(false)
const rawSize = ref(0)
const rawLoading = ref(false)
const rawError = ref('')
let rawBlob: Blob | null = null

/** 按名称过滤后的变量 */
const filtered = computed(() => {
  const text = keyword.value.trim().toLowerCase()
  return text ? variables.value.filter((item) => item.name.toLowerCase().includes(text)) : variables.value
})

/** 接口返回 transient=true 的变量；引擎当前不会持久化瞬态变量 */
const transientVariables = computed(() => variables.value.filter((item) => item.transient))

/** 原始数据可选的变量 */
const rawOptions = computed(() => variables.value.map((item) => ({ label: item.name, value: item.name })))

// 加载变量列表。
// @return 无
async function load(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    variables.value = await listInstanceVariables(props.instanceId)
  } catch (cause) {
    error.value = cause
    markAuthError(cause)
  } finally {
    loading.value = false
  }
}

// 打开新增变量弹窗。
// @return 无
function openAdd(): void {
  editing.value = null
  showEditor.value = true
}

// 打开编辑变量弹窗。
// @param variable 变量
// @return 无
function openEdit(variable: RuntimeVariable): void {
  editing.value = variable
  showEditor.value = true
}

// 保存变量，编辑时按变量名更新，否则新增。
// @param variable 变量载荷
// @return 无
async function save(variable: Variable): Promise<void> {
  saving.value = true
  actionError.value = ''
  notice.value = ''
  try {
    if (editing.value) {
      await updateInstanceVariable(props.instanceId, editing.value.name, { value: variable.value, type: variable.type })
      notice.value = `已更新变量 ${editing.value.name}`
    } else {
      await createInstanceVariable(props.instanceId, variable)
      notice.value = `已新增变量 ${variable.name}`
    }
    showEditor.value = false
    editing.value = null
    await load()
  } catch (cause) {
    actionError.value = errorMessage(cause)
    markAuthError(cause)
  } finally {
    saving.value = false
  }
}

// 打开删除确认。
// @param variable 变量
// @return 无
function askDelete(variable: RuntimeVariable): void {
  pendingDelete.value = variable
  showDelete.value = true
}

// 确认删除变量。
// @return 无
async function confirmDelete(): Promise<void> {
  const target = pendingDelete.value
  if (!target) return
  deleting.value = true
  actionError.value = ''
  try {
    await deleteInstanceVariable(props.instanceId, target.name)
    notice.value = `已删除变量 ${target.name}`
    showDelete.value = false
    pendingDelete.value = null
    await load()
  } catch (cause) {
    actionError.value = errorMessage(cause)
    markAuthError(cause)
  } finally {
    deleting.value = false
  }
}

// 判断内容类型是否可按文本展示。
// @param type MIME 类型
// @return 是否文本
function isTextual(type: string): boolean {
  return /(^text\/)|json|xml|javascript/.test(type)
}

// 查看变量原始数据。
// @param name 变量名
// @return 无
async function openRaw(name: string): Promise<void> {
  if (!name) {
    actionError.value = '请选择要查看的变量'
    return
  }
  showRaw.value = true
  rawName.value = name
  rawType.value = variables.value.find((item) => item.name === name)?.type ?? ''
  rawText.value = ''
  rawBinary.value = false
  rawSize.value = 0
  rawError.value = ''
  rawBlob = null
  rawLoading.value = true
  try {
    const blob = await getInstanceVariableData(props.instanceId, name)
    rawBlob = blob
    rawSize.value = blob.size
    // 文本类内容直接展示，其余按二进制提示下载。
    // 原始数据端点固定返回 octet-stream，文本变量需要按变量类型解码。
    const textualType = ['string', 'json', 'date', 'long', 'double', 'integer', 'boolean'].includes(rawType.value)
    if (isTextual(blob.type) || textualType) {
      const text = await blob.text()
      rawText.value = text.length > 200000 ? `${text.slice(0, 200000)}\n…（已截断）` : text
    } else {
      rawBinary.value = true
    }
  } catch (cause) {
    rawError.value = errorMessage(cause)
    markAuthError(cause)
  } finally {
    rawLoading.value = false
  }
}

// 下载当前原始数据。
// @return 无
function downloadRaw(): void {
  if (rawBlob) downloadBlob(`${rawName.value}.bin`, rawBlob)
}

onMounted(load)
</script>

<template>
  <div class="stack">
    <div class="row">
      <n-input v-model:value="keyword" size="small" placeholder="按变量名搜索" clearable style="max-width: 220px" />
      <n-select v-model:value="rawName" size="small" :options="rawOptions" filterable placeholder="选择变量" style="max-width: 200px" />
      <n-button size="small" :disabled="!rawName" @click="openRaw(rawName)">查看原始数据</n-button>
      <span style="margin-left: auto" />
      <n-button size="small" type="primary" @click="openAdd">新增变量</n-button>
      <n-button size="small" quaternary :loading="loading" @click="load">刷新</n-button>
    </div>

    <div v-if="notice" class="text-success">{{ notice }}</div>
    <div v-if="actionError" class="text-danger" role="alert">操作失败：{{ actionError }}</div>
    <ErrorBlock v-if="error" :error="error" @retry="load" />

    <div v-if="transientVariables.length" class="row">
      <span class="muted">瞬态变量：</span>
      <n-tag v-for="item in transientVariables" :key="item.name" size="small" type="warning" :bordered="false">{{ item.name }}</n-tag>
    </div>
    <p v-else class="muted" style="margin: 0; font-size: 12px">
      瞬态变量只在单次命令内可见，引擎不会持久化，因此通常不会出现在列表中。
    </p>

    <VariableTable
      :variables="filtered"
      :loading="loading"
      show-scope
      @edit="openEdit"
      @remove="askDelete"
      @raw="openRaw($event.name)"
    />

    <VariableEditor
      v-model:show="showEditor"
      :variable="editing"
      :allow-name-edit="!editing"
      @save="save"
    />

    <n-modal v-model:show="showDelete" preset="card" title="删除变量" style="width: 420px; max-width: 92vw">
      <p style="margin: 0">
        确认删除变量 <span class="mono">{{ pendingDelete?.name }}</span>？删除后运行中的执行将无法再读取该变量，且不可恢复。
      </p>
      <template #footer>
        <div class="row" style="justify-content: flex-end">
          <n-button size="small" quaternary @click="showDelete = false">取消</n-button>
          <ConfirmButton label="确认删除" :loading="deleting" @confirm="confirmDelete" />
        </div>
      </template>
    </n-modal>

    <n-modal v-model:show="showRaw" preset="card" :title="`原始数据：${rawName}`" style="width: 720px; max-width: 94vw">
      <div class="row" style="margin-bottom: 8px">
        <span class="muted">{{ rawSize }} 字节</span>
        <n-button size="tiny" style="margin-left: auto" :disabled="!rawBlob" @click="downloadRaw">下载</n-button>
      </div>
      <div v-if="rawLoading" class="muted">加载中…</div>
      <div v-else-if="rawError" class="text-danger">{{ rawError }}</div>
      <div v-else-if="rawBinary" class="muted">二进制内容不可直接阅读，请下载后查看。</div>
      <pre v-else class="code-block">{{ rawText || '（空内容）' }}</pre>
    </n-modal>
  </div>
</template>