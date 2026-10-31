<script setup lang="ts">
// 催办配置页：remind/getPageList 查询 + 新增/编辑/删除（预警提醒 + 延时提醒核心字段）。
import { h, onMounted, reactive, ref } from 'vue'
import { NButton, NFormItem, NInput, NInputNumber, NSelect, NSwitch } from 'naive-ui'
import type { DataTableColumns, SelectOption } from 'naive-ui'
import {
  addWorkflowRemind,
  deleteWorkflowRemind,
  editWorkflowRemind,
  listWorkflowReminds,
} from '@/api/workflow'
import type { KeyedRow, WorkflowRemind } from '@/api/workflow'
import { formatDateTime, textOrDash } from '@/lib/format'
import PageHeader from '@/components/PageHeader.vue'
import FilterBar from '@/components/FilterBar.vue'
import DataTable from '@/components/DataTable.vue'
import StatusTag from '@/components/StatusTag.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import FormModal from './components/FormModal.vue'
import { actionErrorOf, boolField, pickField, useWorkflowListSource } from './helpers'

/** 系统代号筛选 */
const systemSymbol = ref('')
/** 当前页 */
const page = ref(1)
/** 每页条数 */
const pageSize = ref(20)

/** 间隔单位选项 */
const unitOptions: SelectOption[] = [
  { label: '分钟', value: 'minute' },
  { label: '小时', value: 'hour' },
  { label: '天', value: 'day' },
]

/** 表单字段（预警块 + 延时块） */
const form = reactive({
  id: '',
  systemSymbol: '',
  processDefinitionKey: '',
  stepCodes: '',
  aheadEnabled: true,
  ahead: 30,
  retry: 3,
  interval: 30,
  intervalUnit: 'minute',
  delayEnabled: false,
  delay: 60,
})
const showForm = ref(false)
const saving = ref(false)
const formError = ref('')
const notice = ref('')
const actionError = ref('')
/** 操作中的配置 ID */
const actingId = ref('')

/** 催办数据源 */
const source = useWorkflowListSource<WorkflowRemind>(
  () =>
    listWorkflowReminds({
      systemSymbol: systemSymbol.value.trim() || undefined,
      pageNum: page.value,
      pageSize: pageSize.value,
    }),
  '催办列表接口接线中（后端 501），暂无数据',
)

/** 表格列定义 */
const columns: DataTableColumns<WorkflowRemind & KeyedRow> = [
  {
    title: '所属系统',
    key: 'systemSymbol',
    width: 110,
    render: (row) => textOrDash(pickField<string>(row, 'systemSymbol', 'system_symbol')),
  },
  {
    title: '流程名称',
    key: 'processName',
    minWidth: 150,
    render: (row) => textOrDash(pickField<string>(row, 'processName', 'process_name') ?? pickField<string>(row, 'processDefinitionKey', 'process_definition_key')),
  },
  {
    title: '步骤名称',
    key: 'stepName',
    minWidth: 130,
    render: (row) => textOrDash(pickField<string>(row, 'stepName', 'step_name') ?? pickField<string>(row, 'stepCodes', 'step_codes')),
  },
  {
    title: '预警提醒',
    key: 'aheadEnabled',
    width: 90,
    render: (row) =>
      h(StatusTag, {
        value: boolField(row, 'aheadEnabled', 'ahead_enabled') ? 'active' : 'false',
        label: boolField(row, 'aheadEnabled', 'ahead_enabled') ? `提前 ${pickField<number>(row, 'ahead') ?? '—'}` : '未启用',
        tone: boolField(row, 'aheadEnabled', 'ahead_enabled') ? 'success' : 'default',
      }),
  },
  {
    title: '延时提醒',
    key: 'delayEnabled',
    width: 90,
    render: (row) =>
      h(StatusTag, {
        value: boolField(row, 'delayEnabled', 'delay_enabled') ? 'active' : 'false',
        label: boolField(row, 'delayEnabled', 'delay_enabled') ? `延时 ${pickField<number>(row, 'delay') ?? '—'}` : '未启用',
        tone: boolField(row, 'delayEnabled', 'delay_enabled') ? 'info' : 'default',
      }),
  },
  {
    title: '创建时间',
    key: 'createTime',
    width: 165,
    render: (row) => formatDateTime(pickField<string>(row, 'createTime', 'create_time')),
  },
  {
    title: '操作',
    key: 'actions',
    width: 130,
    render: (row) =>
      h('div', { class: 'row' }, [
        h(NButton, { size: 'tiny', quaternary: true, onClick: () => openEdit(row) }, { default: () => '编辑' }),
        h(ConfirmButton, {
          label: '删除',
          title: '确认删除该催办配置？',
          description: '删除后该流程步骤不再触发自动催办。',
          loading: actingId.value === row._rid,
          onConfirm: () => remove(row),
        }),
      ]),
  },
]

// search 执行查询并回到第一页。
// @return 无
function search(): void {
  page.value = 1
  void source.load()
}

// reset 重置筛选。
// @return 无
function reset(): void {
  systemSymbol.value = ''
  page.value = 1
  void source.load()
}

// changePage 翻页。
// @param value 新页码
// @return 无
function changePage(value: number): void {
  page.value = value
  void source.load()
}

// changePageSize 调整每页条数。
// @param value 新条数
// @return 无
function changePageSize(value: number): void {
  pageSize.value = value
  page.value = 1
  void source.load()
}

// openCreate 打开新增弹窗。
// @return 无
function openCreate(): void {
  form.id = ''
  form.systemSymbol = systemSymbol.value.trim()
  form.processDefinitionKey = ''
  form.stepCodes = ''
  form.aheadEnabled = true
  form.ahead = 30
  form.retry = 3
  form.interval = 30
  form.intervalUnit = 'minute'
  form.delayEnabled = false
  form.delay = 60
  formError.value = ''
  showForm.value = true
}

// openEdit 打开编辑弹窗并回填行数据。
// @param row 目标配置行
// @return 无
function openEdit(row: WorkflowRemind & KeyedRow): void {
  form.id = row._rid
  form.systemSymbol = pickField<string>(row, 'systemSymbol', 'system_symbol') ?? ''
  form.processDefinitionKey = pickField<string>(row, 'processDefinitionKey', 'process_definition_key') ?? ''
  const codes = pickField<string | string[]>(row, 'stepCodes', 'step_codes')
  form.stepCodes = Array.isArray(codes) ? codes.join(',') : codes ?? ''
  form.aheadEnabled = boolField(row, 'aheadEnabled', 'ahead_enabled')
  form.ahead = pickField<number>(row, 'ahead') ?? 30
  form.retry = pickField<number>(row, 'retry') ?? 3
  form.interval = pickField<number>(row, 'interval') ?? 30
  form.intervalUnit = pickField<string>(row, 'intervalUnit', 'interval_unit') ?? 'minute'
  form.delayEnabled = boolField(row, 'delayEnabled', 'delay_enabled')
  form.delay = pickField<number>(row, 'delay') ?? 60
  formError.value = ''
  showForm.value = true
}

// save 提交新增或编辑。
// @return 无
async function save(): Promise<void> {
  if (!form.systemSymbol.trim()) {
    formError.value = '所属系统代号为必填项'
    return
  }
  saving.value = true
  formError.value = ''
  try {
    const payload = {
      id: form.id || undefined,
      systemSymbol: form.systemSymbol.trim(),
      processDefinitionKey: form.processDefinitionKey.trim(),
      stepCodes: form.stepCodes.trim(),
      aheadEnabled: form.aheadEnabled,
      ahead: form.ahead,
      retry: form.retry,
      interval: form.interval,
      intervalUnit: form.intervalUnit,
      delayEnabled: form.delayEnabled,
      delay: form.delay,
    }
    const result = form.id ? await editWorkflowRemind(payload) : await addWorkflowRemind(payload)
    const failure = actionErrorOf(result, '催办保存接口接线中（后端 501）')
    if (failure) {
      formError.value = failure
      return
    }
    showForm.value = false
    notice.value = form.id ? '催办配置已更新' : '催办配置已新增'
    await source.load()
  } finally {
    saving.value = false
  }
}

// remove 删除催办配置并刷新列表。
// @param row 目标配置行
// @return 无
async function remove(row: WorkflowRemind & KeyedRow): Promise<void> {
  actingId.value = row._rid
  actionError.value = ''
  notice.value = ''
  try {
    const result = await deleteWorkflowRemind(row._rid)
    const failure = actionErrorOf(result, '催办删除接口接线中（后端 501）')
    if (failure) actionError.value = failure
    else {
      notice.value = '催办配置已删除'
      await source.load()
    }
  } finally {
    actingId.value = ''
  }
}

onMounted(() => {
  void source.load()
})
</script>

<template>
  <div>
    <PageHeader title="催办配置" description="按系统/流程/步骤配置预警提醒与延时提醒（/workflow/remind 门面）；手动催办见待办任务页。">
      <template #actions>
        <n-button size="small" type="primary" @click="openCreate">新增催办</n-button>
        <n-button size="small" quaternary :loading="source.loading.value" @click="() => source.load()">刷新</n-button>
      </template>
    </PageHeader>

    <div v-if="notice" class="banner" role="status" style="margin: 0 0 10px">{{ notice }}</div>
    <div v-if="actionError" class="banner banner--error" role="alert" style="margin: 0 0 10px">{{ actionError }}</div>
    <div v-if="source.errorMessage.value" class="banner banner--error" role="alert" style="margin: 0 0 10px">
      {{ source.errorMessage.value }}
    </div>

    <FilterBar :loading="source.loading.value" @search="search" @reset="reset">
      <div class="filter-bar__item">
        <span class="filter-bar__label">系统代号</span>
        <n-input v-model:value="systemSymbol" size="small" placeholder="所属系统代号（可空）" clearable @keyup.enter="search" />
      </div>
    </FilterBar>

    <DataTable
      :columns="columns"
      :data="source.rows.value"
      :loading="source.loading.value"
      :total="source.total.value"
      :page="page"
      :page-size="pageSize"
      row-key="_rid"
      :empty-description="source.degraded.value || '暂无催办配置，点击右上角新增催办。'"
      @update:page="changePage"
      @update:page-size="changePageSize"
    />

    <FormModal
      v-model:show="showForm"
      :title="form.id ? '编辑催办' : '新增催办'"
      :submitting="saving"
      :error="formError"
      :width="680"
      @submit="save"
    >
      <n-form-item label="所属系统" required>
        <n-input v-model:value="form.systemSymbol" placeholder="系统代号（必填，如 crm）" />
      </n-form-item>
      <n-form-item label="流程 Key">
        <n-input v-model:value="form.processDefinitionKey" placeholder="流程定义 Key（可空表示全部流程）" class="mono" />
      </n-form-item>
      <n-form-item label="步骤代号">
        <n-input v-model:value="form.stepCodes" placeholder="步骤代号，逗号分隔（可空表示全部步骤）" class="mono" />
      </n-form-item>
      <n-form-item label="预警提醒">
        <n-switch v-model:value="form.aheadEnabled" />
      </n-form-item>
      <n-form-item label="提前量">
        <n-input-number v-model:value="form.ahead" :min="0" style="width: 160px" :disabled="!form.aheadEnabled" />
        <span class="muted" style="margin-left: 8px">按计划完成时间提前的量（分钟）</span>
      </n-form-item>
      <n-form-item label="再次提醒">
        <n-input-number v-model:value="form.retry" :min="0" style="width: 160px" :disabled="!form.aheadEnabled" />
        <span class="muted" style="margin-left: 8px">次数</span>
      </n-form-item>
      <n-form-item label="提醒间隔">
        <n-input-number v-model:value="form.interval" :min="0" style="width: 140px" :disabled="!form.aheadEnabled" />
        <n-select v-model:value="form.intervalUnit" :options="unitOptions" style="width: 110px" :disabled="!form.aheadEnabled" />
      </n-form-item>
      <n-form-item label="延时提醒">
        <n-switch v-model:value="form.delayEnabled" />
      </n-form-item>
      <n-form-item label="延时时长">
        <n-input-number v-model:value="form.delay" :min="0" style="width: 160px" :disabled="!form.delayEnabled" />
        <span class="muted" style="margin-left: 8px">超时后延时（分钟）</span>
      </n-form-item>
    </FormModal>
  </div>
</template>
