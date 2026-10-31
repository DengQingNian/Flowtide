<script setup lang="ts">
// 委托配置页：delegate/list 查询 + 新增/编辑/删除 + 启用/停用（时间重叠校验由后端兜底）。
import { h, onMounted, reactive, ref } from 'vue'
import { NButton, NFormItem, NInput, NSwitch } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import {
  addWorkflowDelegate,
  deleteWorkflowDelegates,
  editWorkflowDelegate,
  listWorkflowDelegates,
} from '@/api/workflow'
import type { KeyedRow, WorkflowDelegate } from '@/api/workflow'
import { formatDateTime, textOrDash } from '@/lib/format'
import PageHeader from '@/components/PageHeader.vue'
import FilterBar from '@/components/FilterBar.vue'
import DataTable from '@/components/DataTable.vue'
import StatusTag from '@/components/StatusTag.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import FormModal from './components/FormModal.vue'
import { actionErrorOf, boolField, pickField, useWorkflowListSource } from './helpers'

/** 委托人筛选（输入身份 Id，可空查全部） */
const ownerId = ref('')
/** 当前页 */
const page = ref(1)
/** 每页条数 */
const pageSize = ref(20)

/** 表单字段 */
const form = reactive({
  id: '',
  ownerId: '',
  trusteeId: '',
  condition: '',
  startTime: '',
  endTime: '',
  systemSymbol: '',
  enabled: true,
  remark: '',
})
const showForm = ref(false)
const saving = ref(false)
const formError = ref('')
const notice = ref('')
const actionError = ref('')
/** 操作中的配置 ID */
const actingId = ref('')

/** 委托数据源 */
const source = useWorkflowListSource<WorkflowDelegate>(
  () =>
    listWorkflowDelegates({
      ownerId: ownerId.value.trim() || undefined,
      pageNum: page.value,
      pageSize: pageSize.value,
    }),
  '委托列表接口接线中（后端 501），暂无数据',
)

/** 表格列定义 */
const columns: DataTableColumns<WorkflowDelegate & KeyedRow> = [
  {
    title: '委托人',
    key: 'ownerId',
    width: 130,
    render: (row) => textOrDash(pickField<string>(row, 'ownerName', 'owner_name') ?? pickField<string>(row, 'ownerId', 'owner_id')),
  },
  {
    title: '受托人',
    key: 'trusteeId',
    width: 130,
    render: (row) => textOrDash(pickField<string>(row, 'trusteeName', 'trustee_name') ?? pickField<string>(row, 'trusteeId', 'trustee_id')),
  },
  {
    title: '委托条件',
    key: 'condition',
    width: 120,
    render: (row) => textOrDash(pickField<string>(row, 'condition')),
  },
  {
    title: '委托时限',
    key: 'startTime',
    minWidth: 240,
    render: (row) => `${formatDateTime(pickField<string>(row, 'startTime', 'start_time'))} ~ ${formatDateTime(pickField<string>(row, 'endTime', 'end_time'))}`,
  },
  {
    title: '系统代号',
    key: 'systemSymbol',
    width: 110,
    render: (row) => textOrDash(pickField<string>(row, 'systemSymbol', 'system_symbol', 'tenantId')),
  },
  {
    title: '启用',
    key: 'enabled',
    width: 80,
    render: (row) =>
      h(StatusTag, {
        value: boolField(row, 'enabled') ? 'active' : 'false',
        label: boolField(row, 'enabled') ? '启用' : '停用',
        tone: boolField(row, 'enabled') ? 'success' : 'warning',
      }),
  },
  {
    title: '备注',
    key: 'remark',
    minWidth: 120,
    render: (row) => textOrDash(pickField<string>(row, 'remark')),
  },
  {
    title: '操作',
    key: 'actions',
    width: 210,
    render: (row) =>
      h('div', { class: 'row' }, [
        h(NButton, { size: 'tiny', quaternary: true, onClick: () => openEdit(row) }, { default: () => '编辑' }),
        h(
          NButton,
          {
            size: 'tiny',
            quaternary: true,
            loading: actingId.value === row._rid,
            onClick: () => toggleEnabled(row),
          },
          { default: () => (boolField(row, 'enabled') ? '停用' : '启用') },
        ),
        h(ConfirmButton, {
          label: '删除',
          title: '确认删除该委托配置？',
          description: '删除后原委托人的待办不再自动转交。',
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
  ownerId.value = ''
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
  form.ownerId = ownerId.value.trim()
  form.trusteeId = ''
  form.condition = ''
  form.startTime = ''
  form.endTime = ''
  form.systemSymbol = ''
  form.enabled = true
  form.remark = ''
  formError.value = ''
  showForm.value = true
}

// openEdit 打开编辑弹窗并回填行数据。
// @param row 目标配置行
// @return 无
function openEdit(row: WorkflowDelegate & KeyedRow): void {
  form.id = row._rid
  form.ownerId = pickField<string>(row, 'ownerId', 'owner_id') ?? ''
  form.trusteeId = pickField<string>(row, 'trusteeId', 'trustee_id') ?? ''
  form.condition = pickField<string>(row, 'condition') ?? ''
  form.startTime = pickField<string>(row, 'startTime', 'start_time') ?? ''
  form.endTime = pickField<string>(row, 'endTime', 'end_time') ?? ''
  form.systemSymbol = pickField<string>(row, 'systemSymbol', 'system_symbol', 'tenantId') ?? ''
  form.enabled = boolField(row, 'enabled')
  form.remark = pickField<string>(row, 'remark') ?? ''
  formError.value = ''
  showForm.value = true
}

// buildPayload 把表单组装为提交载荷。
// @return 委托载荷
function buildPayload() {
  return {
    id: form.id || undefined,
    ownerId: form.ownerId.trim(),
    trusteeId: form.trusteeId.trim(),
    condition: form.condition.trim(),
    startTime: form.startTime,
    endTime: form.endTime,
    systemSymbol: form.systemSymbol.trim(),
    enabled: form.enabled,
    remark: form.remark.trim(),
  }
}

// save 提交新增或编辑。
// @return 无
async function save(): Promise<void> {
  if (!form.ownerId.trim() || !form.trusteeId.trim()) {
    formError.value = '委托人与受托人为必填项'
    return
  }
  if (form.ownerId.trim() === form.trusteeId.trim()) {
    formError.value = '受托人不能与委托人相同'
    return
  }
  saving.value = true
  formError.value = ''
  try {
    const result = form.id ? await editWorkflowDelegate(buildPayload()) : await addWorkflowDelegate(buildPayload())
    const failure = actionErrorOf(result, '委托保存接口接线中（后端 501）；时间重叠/传递委托等规则由后端校验')
    if (failure) {
      formError.value = failure
      return
    }
    showForm.value = false
    notice.value = form.id ? '委托配置已更新' : '委托配置已新增'
    await source.load()
  } finally {
    saving.value = false
  }
}

// toggleEnabled 启用/停用（走 edit 门面，携带完整行字段）。
// @param row 目标配置行
// @return 无
async function toggleEnabled(row: WorkflowDelegate & KeyedRow): Promise<void> {
  actingId.value = row._rid
  actionError.value = ''
  notice.value = ''
  try {
    const result = await editWorkflowDelegate({
      id: row._rid,
      ownerId: pickField<string>(row, 'ownerId', 'owner_id') ?? '',
      trusteeId: pickField<string>(row, 'trusteeId', 'trustee_id') ?? '',
      condition: pickField<string>(row, 'condition'),
      startTime: pickField<string>(row, 'startTime', 'start_time'),
      endTime: pickField<string>(row, 'endTime', 'end_time'),
      systemSymbol: pickField<string>(row, 'systemSymbol', 'system_symbol', 'tenantId'),
      enabled: !boolField(row, 'enabled'),
      remark: pickField<string>(row, 'remark'),
    })
    const failure = actionErrorOf(result, '委托启停接口接线中（后端 501）')
    if (failure) actionError.value = failure
    else {
      notice.value = boolField(row, 'enabled') ? '委托配置已停用' : '委托配置已启用'
      await source.load()
    }
  } finally {
    actingId.value = ''
  }
}

// remove 删除委托配置并刷新列表。
// @param row 目标配置行
// @return 无
async function remove(row: WorkflowDelegate & KeyedRow): Promise<void> {
  actingId.value = row._rid
  actionError.value = ''
  notice.value = ''
  try {
    const result = await deleteWorkflowDelegates([row._rid])
    const failure = actionErrorOf(result, '委托删除接口接线中（后端 501）')
    if (failure) actionError.value = failure
    else {
      notice.value = '委托配置已删除'
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
    <PageHeader title="委托配置" description="配置委托人外出期间的待办自动转交（/workflow/delegate 门面）；时间重叠与传递委托由后端校验并明示规则。">
      <template #actions>
        <n-button size="small" type="primary" @click="openCreate">新增委托</n-button>
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
        <span class="filter-bar__label">委托人</span>
        <n-input v-model:value="ownerId" size="small" placeholder="委托人身份 Id（可空）" clearable @keyup.enter="search" />
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
      :empty-description="source.degraded.value || '暂无委托配置，点击右上角新增委托。'"
      @update:page="changePage"
      @update:page-size="changePageSize"
    />

    <FormModal
      v-model:show="showForm"
      :title="form.id ? '编辑委托' : '新增委托'"
      :submitting="saving"
      :error="formError"
      @submit="save"
    >
      <n-form-item label="委托人" required>
        <n-input v-model:value="form.ownerId" placeholder="委托人身份 Id（必填）" />
      </n-form-item>
      <n-form-item label="受托人" required>
        <n-input v-model:value="form.trusteeId" placeholder="受托人身份 Id（必填）" />
      </n-form-item>
      <n-form-item label="委托条件">
        <n-input v-model:value="form.condition" placeholder="委托条件（字典代码，可选）" />
      </n-form-item>
      <n-form-item label="开始时间">
        <n-input v-model:value="form.startTime" placeholder="YYYY-MM-DD HH:mm:ss（可选）" />
      </n-form-item>
      <n-form-item label="结束时间">
        <n-input v-model:value="form.endTime" placeholder="YYYY-MM-DD HH:mm:ss（可选）" />
      </n-form-item>
      <n-form-item label="系统代号" required>
        <n-input v-model:value="form.systemSymbol" placeholder="系统代号（必填，如 crm）" />
      </n-form-item>
      <n-form-item label="启用">
        <n-switch v-model:value="form.enabled" />
      </n-form-item>
      <n-form-item label="备注" required>
        <n-input v-model:value="form.remark" type="textarea" :rows="2" placeholder="备注（必填）" />
      </n-form-item>
    </FormModal>
  </div>
</template>
