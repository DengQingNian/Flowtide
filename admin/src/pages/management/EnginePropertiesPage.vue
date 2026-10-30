<script setup lang="ts">
// 引擎属性：系统属性只读展示，自定义属性支持新增、编辑与删除。
import { computed, h, onMounted, reactive, ref } from 'vue'
import { NButton } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { AlertTriangle, RefreshCw } from '@lucide/vue'
import PageHeader from '@/components/PageHeader.vue'
import DataTable from '@/components/DataTable.vue'
import ErrorBlock from '@/components/ErrorBlock.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import StatusTag from '@/components/StatusTag.vue'
import { errorMessage } from '@/api/client'
import { markAuthError } from '@/stores/ui'
import { createEngineProperty, deleteEngineProperty, listEngineProperties, updateEngineProperty } from '@/api/management'
import { isSystemProperty } from './components/managementUtils'
import { formatNumber } from '@/lib/format'

/** 属性行 */
interface PropertyRow extends Record<string, unknown> {
  name: string
  value: string
  system: boolean
}

/** 表单 */
const form = reactive({ name: '', value: '' })
/** 编辑模式 */
const mode = ref<'create' | 'edit'>('create')
/** 正在编辑的属性名 */
const editingName = ref('')
/** 属性列表 */
const properties = ref<Record<string, string>>({})
/** 加载态 */
const loading = ref(false)
/** 页面错误 */
const error = ref<unknown>(null)
/** 保存加载态 */
const saving = ref(false)
/** 表单错误 */
const formError = ref<unknown>(null)
/** 操作提示 */
const notice = ref('')
/** 删除中的属性名 */
const deletingName = ref('')

/** 表格数据 */
const rows = computed<PropertyRow[]>(() =>
  Object.entries(properties.value)
    .map(([name, value]) => ({ name, value, system: isSystemProperty(name) }))
    .sort((left, right) => {
      // 系统属性排在前面，同组按名称排序
      if (left.system !== right.system) return left.system ? -1 : 1
      return left.name.localeCompare(right.name)
    }),
)
/** 自定义属性数量 */
const customCount = computed(() => rows.value.filter((row) => !row.system).length)

// 加载属性。
// @return 无
async function load(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    properties.value = await listEngineProperties()
  } catch (cause) {
    error.value = cause
    markAuthError(cause)
  } finally {
    loading.value = false
  }
}

// 重置表单。
// @return 无
function resetForm(): void {
  form.name = ''
  form.value = ''
  mode.value = 'create'
  editingName.value = ''
  formError.value = null
}

// 进入编辑模式。
// @param row 属性行
// @return 无
function startEdit(row: PropertyRow): void {
  // 系统属性由引擎维护，界面上不提供编辑入口
  if (row.system) return
  mode.value = 'edit'
  editingName.value = row.name
  form.name = row.name
  form.value = row.value
  formError.value = null
  notice.value = ''
}

// 提交新增或更新。
// @return 无
async function submit(): Promise<void> {
  const name = form.name.trim()
  if (!name) {
    formError.value = new Error('属性名不能为空')
    return
  }
  saving.value = true
  formError.value = null
  notice.value = ''
  try {
    if (mode.value === 'edit') await updateEngineProperty(editingName.value, form.value)
    else await createEngineProperty(name, form.value)
    notice.value = mode.value === 'edit' ? `属性 ${editingName.value} 已更新。` : `属性 ${name} 已创建。`
    resetForm()
    await load()
  } catch (cause) {
    formError.value = cause
    markAuthError(cause)
  } finally {
    saving.value = false
  }
}

// 删除自定义属性。
// @param name 属性名
// @return 无
async function remove(name: string): Promise<void> {
  deletingName.value = name
  formError.value = null
  notice.value = ''
  try {
    await deleteEngineProperty(name)
    if (editingName.value === name) resetForm()
    notice.value = `属性 ${name} 已删除。`
    await load()
  } catch (cause) {
    formError.value = cause
    markAuthError(cause)
  } finally {
    deletingName.value = ''
  }
}

/** 表格列 */
const columns = computed<DataTableColumns<PropertyRow>>(() => [
  { key: 'name', title: '属性名', minWidth: 220, ellipsis: { tooltip: true }, render: (row) => h('span', { class: 'mono' }, row.name) },
  { key: 'value', title: '属性值', minWidth: 260, ellipsis: { tooltip: true }, render: (row) => row.value || '—' },
  {
    key: 'system',
    title: '类型',
    width: 110,
    render: (row) => h(StatusTag, { value: row.system ? '系统属性' : '自定义', tone: row.system ? 'default' : 'info' }),
  },
  {
    key: 'action',
    title: '操作',
    width: 170,
    render: (row) =>
      row.system
        ? h('span', { class: 'muted' }, '只读')
        : h('span', { class: 'row' }, [
            h(NButton, { size: 'tiny', quaternary: true, onClick: () => startEdit(row) }, { default: () => '编辑' }),
            h(ConfirmButton, {
              label: '删除',
              title: `删除属性 ${row.name}？`,
              description: '删除后引擎不再保留该自定义属性，依赖它的配置可能失效。',
              loading: deletingName.value === row.name,
              onConfirm: () => void remove(row.name),
            }),
          ]),
  },
])

onMounted(() => void load())
</script>

<template>
  <div>
    <PageHeader title="引擎属性" :description="`共 ${formatNumber(rows.length)} 个属性，其中 ${formatNumber(customCount)} 个自定义属性可编辑。`">
      <template #actions>
        <n-button size="small" :loading="loading" @click="load">
          <template #icon><RefreshCw :size="14" /></template>
          刷新
        </n-button>
      </template>
    </PageHeader>

    <ErrorBlock v-if="error" :error="error" context="GET /service/management/engine-properties" @retry="load" />

    <div v-if="notice" class="banner" role="status" style="margin-bottom: 12px">{{ notice }}</div>
    <div v-if="formError" class="banner banner--error" role="alert" style="margin-bottom: 12px">
      <AlertTriangle :size="16" aria-hidden="true" />
      <div class="grow">{{ errorMessage(formError) }}</div>
    </div>

    <section class="panel" style="margin-bottom: 14px">
      <div class="panel__header">
        <span class="panel__title">{{ mode === 'edit' ? `编辑属性 · ${editingName}` : '新增自定义属性' }}</span>
        <span class="muted" style="margin-left: auto; font-size: 12px">系统属性由引擎维护，只读</span>
      </div>
      <div class="panel__body">
        <div class="property-form">
          <label class="field">
            <span class="field__label">属性名</span>
            <input v-model="form.name" class="native-input" type="text" :disabled="mode === 'edit' || saving" placeholder="如 feature.flag" />
          </label>
          <label class="field grow">
            <span class="field__label">属性值</span>
            <input v-model="form.value" class="native-input" type="text" :disabled="saving" placeholder="属性值" />
          </label>
          <n-button size="small" type="primary" :loading="saving" @click="submit">{{ mode === 'edit' ? '保存修改' : '新增属性' }}</n-button>
          <n-button v-if="mode === 'edit'" size="small" quaternary :disabled="saving" @click="resetForm">取消编辑</n-button>
        </div>
      </div>
    </section>

    <DataTable
      :columns="columns"
      :data="rows"
      :loading="loading"
      :total="rows.length"
      :page="1"
      :page-size="1000"
      :page-sizes="[1000]"
      row-key="name"
      :error="null"
      empty-description="暂无引擎属性，可在上方新增自定义属性。"
      :scroll-x="1000"
      @retry="load"
    />
  </div>
</template>

<style scoped>
.property-form { display: flex; align-items: flex-end; gap: 10px; flex-wrap: wrap; }
.field { display: flex; flex-direction: column; gap: 4px; min-width: 200px; }
.field__label { font-size: 12px; color: var(--muted); }
.native-input {
  height: 30px;
  padding: 0 8px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: var(--surface);
  color: var(--text);
  font-size: 13px;
}
.native-input:disabled { color: var(--muted); background: var(--surface-2); }
</style>
