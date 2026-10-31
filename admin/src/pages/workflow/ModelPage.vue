<script setup lang="ts">
// 流程模型页：模型列表（/workflow/model 门面）+ 新增/编辑/部署/删除。
import { computed, h, onMounted, reactive, ref } from 'vue'
import { NButton, NFormItem, NInput, NInputNumber, NSwitch, NTreeSelect } from 'naive-ui'
import type { DataTableColumns, TreeSelectOption } from 'naive-ui'
import {
  addWorkflowModel,
  deleteWorkflowModels,
  deployWorkflowProcess,
  editWorkflowModel,
  extractRows,
  listWorkflowCategories,
  listWorkflowModels,
} from '@/api/workflow'
import type { KeyedRow, WorkflowCategory, WorkflowModel } from '@/api/workflow'
import { formatDateTime, textOrDash } from '@/lib/format'
import PageHeader from '@/components/PageHeader.vue'
import FilterBar from '@/components/FilterBar.vue'
import DataTable from '@/components/DataTable.vue'
import StatusTag from '@/components/StatusTag.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import FormModal from './components/FormModal.vue'
import { actionErrorOf, boolField, pickField, useWorkflowListSource } from './helpers'

/** 当前页 */
const page = ref(1)
/** 每页条数 */
const pageSize = ref(20)
/** 名称关键字 */
const keyword = ref('')

/** 表单字段 */
const form = reactive({
  id: '',
  modelName: '',
  modelCode: '',
  categoryId: null as string | null,
  enabled: true,
  sort: 0,
  remark: '',
})
const showForm = ref(false)
const saving = ref(false)
const formError = ref('')
const notice = ref('')
const actionError = ref('')
const actingId = ref('')
/** 部署中的模型 ID */
const deployingId = ref('')

/** 分类缓存（弹窗树选择用） */
const categories = ref<(WorkflowCategory & KeyedRow)[]>([])

/** 模型数据源 */
const source = useWorkflowListSource<WorkflowModel>(
  () =>
    listWorkflowModels({
      pageNum: page.value,
      pageSize: pageSize.value,
      modelName: keyword.value.trim() || undefined,
      name: keyword.value.trim() || undefined,
    }),
  '模型列表接口接线中（后端 501），暂无数据',
)

/** 分类树选择选项 */
const categoryOptions = computed<TreeSelectOption[]>(() =>
  categories.value.map((row) => ({
    key: row._rid,
    label: `${pickField<string>(row, 'categoryName', 'category_name', 'name') ?? row._rid}`,
  })),
)

/** 表格列 */
const columns = computed<DataTableColumns<WorkflowModel & KeyedRow>>(() => [
  {
    title: '模型名称',
    key: 'modelName',
    minWidth: 180,
    render: (row) => pickField<string>(row, 'modelName', 'model_name', 'name') ?? row._rid,
  },
  {
    title: '模型代号',
    key: 'modelCode',
    width: 150,
    render: (row) => h('span', { class: 'mono' }, pickField<string>(row, 'modelCode', 'model_code', 'code') ?? '—'),
  },
  {
    title: '所属分类',
    key: 'categoryName',
    width: 140,
    render: (row) => textOrDash(pickField<string>(row, 'categoryName', 'category_name')),
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
    title: '最近部署时间',
    key: 'lastDeployTime',
    width: 170,
    render: (row) => formatDateTime(pickField<string>(row, 'lastDeployTime', 'last_deploy_time', 'deployTime')),
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
        h(ConfirmButton, {
          label: '部署',
          danger: false,
          title: '确认部署该模型？',
          description: '部署会把模型 BPMN 发布为新的流程定义版本。',
          loading: deployingId.value === row._rid,
          onConfirm: () => deploy(row),
        }),
        h(ConfirmButton, {
          label: '删除',
          title: '确认删除该模型？',
          description: '删除后模型配置与步骤定义一并移除，操作不可恢复。',
          loading: actingId.value === row._rid,
          onConfirm: () => remove(row),
        }),
      ]),
  },
])

// loadCategories 拉取分类供弹窗选择；失败静默（树选择退化为无选项）。
// @return 无
async function loadCategories(): Promise<void> {
  const result = await listWorkflowCategories()
  if (result.success) {
    categories.value = extractRows<WorkflowCategory>(result.data)
  }
}

// search 执行查询并回到第一页。
// @return 无
function search(): void {
  page.value = 1
  void source.load()
}

// reset 重置筛选。
// @return 无
function reset(): void {
  keyword.value = ''
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
  form.modelName = ''
  form.modelCode = ''
  form.categoryId = null
  form.enabled = true
  form.sort = 0
  form.remark = ''
  formError.value = ''
  showForm.value = true
}

// openEdit 打开编辑弹窗并回填行数据。
// @param row 目标模型行
// @return 无
function openEdit(row: WorkflowModel & KeyedRow): void {
  form.id = row._rid
  form.modelName = pickField<string>(row, 'modelName', 'model_name', 'name') ?? ''
  form.modelCode = pickField<string>(row, 'modelCode', 'model_code', 'code') ?? ''
  form.categoryId = pickField<string>(row, 'categoryId', 'category_id') ?? null
  form.enabled = boolField(row, 'enabled')
  form.sort = pickField<number>(row, 'sort', 'sortNo') ?? 0
  form.remark = pickField<string>(row, 'remark') ?? ''
  formError.value = ''
  showForm.value = true
}

// save 提交新增或编辑。
// @return 无
async function save(): Promise<void> {
  if (!form.modelName.trim() || !form.modelCode.trim()) {
    formError.value = '模型名称与模型代号为必填项'
    return
  }
  saving.value = true
  formError.value = ''
  try {
    const payload = {
      id: form.id || undefined,
      modelName: form.modelName.trim(),
      modelCode: form.modelCode.trim(),
      categoryId: form.categoryId || null,
      enabled: form.enabled,
      sort: form.sort,
      remark: form.remark.trim(),
    }
    const result = form.id ? await editWorkflowModel(payload) : await addWorkflowModel(payload)
    const failure = actionErrorOf(result, '模型保存接口接线中（后端 501）')
    if (failure) {
      formError.value = failure
      return
    }
    showForm.value = false
    notice.value = form.id ? '模型已更新' : '模型已新增'
    await source.load()
  } finally {
    saving.value = false
  }
}

// deploy 部署模型为流程定义（POST /workflow/process/deploy）。
// @param row 目标模型行
// @return 无
async function deploy(row: WorkflowModel & KeyedRow): Promise<void> {
  deployingId.value = row._rid
  actionError.value = ''
  notice.value = ''
  try {
    const result = await deployWorkflowProcess({
      modelId: row._rid,
      tenantId: pickField<string>(row, 'tenantId', 'tenant_id') ?? '',
    })
    const failure = actionErrorOf(result, '部署接口接线中（后端 501），请稍后再试')
    if (failure) actionError.value = failure
    else {
      notice.value = result.msg || `模型 ${pickField<string>(row, 'modelName', 'name') ?? row._rid} 部署成功`
      await source.load()
    }
  } finally {
    deployingId.value = ''
  }
}

// remove 删除模型并刷新列表。
// @param row 目标模型行
// @return 无
async function remove(row: WorkflowModel & KeyedRow): Promise<void> {
  actingId.value = row._rid
  actionError.value = ''
  notice.value = ''
  try {
    const result = await deleteWorkflowModels([row._rid])
    const failure = actionErrorOf(result, '模型删除接口接线中（后端 501）')
    if (failure) actionError.value = failure
    else {
      notice.value = '模型已删除'
      await source.load()
    }
  } finally {
    actingId.value = ''
  }
}

onMounted(() => {
  void source.load()
  void loadCategories()
})
</script>

<template>
  <div>
    <PageHeader title="流程模型" description="维护流程模型（/workflow/model 门面），并部署为流程定义；部署走 /workflow/process/deploy。">
      <template #actions>
        <n-button size="small" type="primary" @click="openCreate">新增模型</n-button>
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
        <span class="filter-bar__label">模型名称</span>
        <n-input v-model:value="keyword" size="small" placeholder="按名称/代号模糊匹配" clearable @keyup.enter="search" />
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
      :empty-description="source.degraded.value || '暂无模型，点击右上角新增模型。'"
      @update:page="changePage"
      @update:page-size="changePageSize"
    />

    <FormModal
      v-model:show="showForm"
      :title="form.id ? '编辑模型' : '新增模型'"
      :submitting="saving"
      :error="formError"
      @submit="save"
    >
      <n-form-item label="所属分类">
        <n-tree-select v-model:value="form.categoryId" :options="categoryOptions" clearable placeholder="选择所属分类/系统" />
      </n-form-item>
      <n-form-item label="模型名称" required>
        <n-input v-model:value="form.modelName" placeholder="模型名称（必填）" />
      </n-form-item>
      <n-form-item label="模型代号" required>
        <n-input v-model:value="form.modelCode" placeholder="模型代号（必填，如 leave_apply）" :disabled="Boolean(form.id)" />
      </n-form-item>
      <n-form-item label="启用">
        <n-switch v-model:value="form.enabled" />
      </n-form-item>
      <n-form-item label="排序">
        <n-input-number v-model:value="form.sort" :min="0" style="width: 100%" />
      </n-form-item>
      <n-form-item label="备注">
        <n-input v-model:value="form.remark" type="textarea" :rows="2" placeholder="备注（可选）" />
      </n-form-item>
    </FormModal>
  </div>
</template>
