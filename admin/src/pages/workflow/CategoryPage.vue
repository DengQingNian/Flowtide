<script setup lang="ts">
// 流程分类页：分类树形列表（category/list 投影）+ 新增/编辑/删除/添加子级。
import { computed, h, onMounted, reactive, ref } from 'vue'
import { NButton, NFormItem, NInput, NInputNumber, NSwitch, NTreeSelect } from 'naive-ui'
import type { DataTableColumns, TreeSelectOption } from 'naive-ui'
import {
  addWorkflowCategory,
  deleteWorkflowCategories,
  editWorkflowCategory,
  listWorkflowCategories,
} from '@/api/workflow'
import type { KeyedRow, WorkflowCategory } from '@/api/workflow'
import PageHeader from '@/components/PageHeader.vue'
import DataTable from '@/components/DataTable.vue'
import StatusTag from '@/components/StatusTag.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import FormModal from './components/FormModal.vue'
import { actionErrorOf, boolField, pickField, useWorkflowListSource } from './helpers'

/** 树节点：分类行 + children 子树 */
interface CategoryNode extends WorkflowCategory, KeyedRow {
  /** 子分类 */
  children?: CategoryNode[]
}

/** 表单字段 */
const form = reactive({
  id: '',
  parentId: null as string | null,
  categoryName: '',
  categoryCode: '',
  sort: 0,
  visible: true,
  enabled: true,
  remark: '',
})
const showForm = ref(false)
const saving = ref(false)
const formError = ref('')
const notice = ref('')
const actionError = ref('')
const deletingId = ref('')

/** 分类数据源 */
const source = useWorkflowListSource<WorkflowCategory>(() => listWorkflowCategories(), '分类列表接口接线中（后端 501），暂无数据')

/** 树形表格数据（按 parentId 组树） */
const treeRows = computed<CategoryNode[]>(() => buildTree(source.rows.value))

/** 树选择选项（新增/编辑弹窗的所属分类） */
const parentOptions = computed<TreeSelectOption[]>(() => buildOptions(treeRows.value, 0))

// buildTree 把平铺分类行组装为树；parentId 缺失或找不到父节点时视为根节点。
// @param rows 平铺分类行
// @return 树形行
function buildTree(rows: (WorkflowCategory & KeyedRow)[]): CategoryNode[] {
  const nodes = new Map<string, CategoryNode>()
  for (const row of rows) nodes.set(row._rid, { ...row, children: [] })
  const roots: CategoryNode[] = []
  for (const node of nodes.values()) {
    // 父 ID 归一化为字符串后查父节点；找不到或指向自身都按根处理
    const pid = node.parentId === null || node.parentId === undefined ? '' : String(node.parentId)
    const parent = pid && pid !== node._rid ? nodes.get(pid) : undefined
    if (parent) parent.children!.push(node)
    else roots.push(node)
  }
  for (const node of nodes.values()) {
    if (!node.children!.length) delete node.children
  }
  return roots
}

// buildOptions 递归构造树选择选项。
// @param nodes 分类树
// @param depth 递归深度（防环保护）
// @return 树选择选项
function buildOptions(nodes: CategoryNode[], depth: number): TreeSelectOption[] {
  // 深度保护：异常数据（成环）时不无限展开
  if (depth > 20) return []
  return nodes.map((node) => ({
    key: node._rid,
    label: `${pickField<string>(node, 'categoryName', 'category_name', 'name') ?? node._rid}`,
    children: node.children?.length ? buildOptions(node.children, depth + 1) : undefined,
  }))
}

/** 表格列 */
const columns = computed<DataTableColumns<CategoryNode>>(() => [
  {
    title: '分类名称',
    key: 'categoryName',
    minWidth: 200,
    render: (row) => pickField<string>(row, 'categoryName', 'category_name', 'name') ?? row._rid,
  },
  {
    title: '代号',
    key: 'categoryCode',
    width: 140,
    render: (row) => pickField<string>(row, 'categoryCode', 'category_code', 'code') ?? '—',
  },
  { title: '排序', key: 'sort', width: 80, render: (row) => String(pickField<number>(row, 'sort', 'sortNo') ?? '—') },
  {
    title: '可见',
    key: 'visible',
    width: 80,
    render: (row) => h(StatusTag, { value: boolField(row, 'visible') ? 'true' : 'false', label: boolField(row, 'visible') ? '可见' : '隐藏', tone: boolField(row, 'visible') ? 'success' : 'default' }),
  },
  {
    title: '启用',
    key: 'enabled',
    width: 80,
    render: (row) => h(StatusTag, { value: boolField(row, 'enabled') ? 'active' : 'false', label: boolField(row, 'enabled') ? '启用' : '停用', tone: boolField(row, 'enabled') ? 'success' : 'warning' }),
  },
  {
    title: '备注',
    key: 'remark',
    minWidth: 140,
    render: (row) => pickField<string>(row, 'remark') ?? '—',
  },
  {
    title: '操作',
    key: 'actions',
    width: 200,
    render: (row) =>
      h('div', { class: 'row' }, [
        h(NButton, { size: 'tiny', quaternary: true, onClick: () => openCreate(row._rid) }, { default: () => '添加子级' }),
        h(NButton, { size: 'tiny', quaternary: true, onClick: () => openEdit(row) }, { default: () => '编辑' }),
        h(ConfirmButton, {
          label: '删除',
          title: '确认删除该分类？',
          description: '删除后其子分类与关联模型需自行处理归属，操作不可恢复。',
          loading: deletingId.value === row._rid,
          onConfirm: () => remove(row),
        }),
      ]),
  },
])

// openCreate 打开新增弹窗；指定父分类时预填所属分类。
// @param parentId 父分类 ID，可空
// @return 无
function openCreate(parentId?: string): void {
  form.id = ''
  form.parentId = parentId ?? null
  form.categoryName = ''
  form.categoryCode = ''
  form.sort = 0
  form.visible = true
  form.enabled = true
  form.remark = ''
  formError.value = ''
  showForm.value = true
}

// openEdit 打开编辑弹窗并回填行数据。
// @param row 目标分类行
// @return 无
function openEdit(row: CategoryNode): void {
  form.id = row._rid
  form.parentId = row.parentId ? String(row.parentId) : null
  form.categoryName = pickField<string>(row, 'categoryName', 'category_name', 'name') ?? ''
  form.categoryCode = pickField<string>(row, 'categoryCode', 'category_code', 'code') ?? ''
  form.sort = pickField<number>(row, 'sort', 'sortNo') ?? 0
  form.visible = boolField(row, 'visible')
  form.enabled = boolField(row, 'enabled')
  form.remark = pickField<string>(row, 'remark') ?? ''
  formError.value = ''
  showForm.value = true
}

// save 提交新增或编辑；编辑时携带 id 走 edit 接口。
// @return 无
async function save(): Promise<void> {
  if (!form.categoryName.trim() || !form.categoryCode.trim()) {
    formError.value = '分类名称与分类代号为必填项'
    return
  }
  saving.value = true
  formError.value = ''
  try {
    const payload = {
      id: form.id || undefined,
      parentId: form.parentId || null,
      categoryName: form.categoryName.trim(),
      categoryCode: form.categoryCode.trim(),
      sort: form.sort,
      visible: form.visible,
      enabled: form.enabled,
      remark: form.remark.trim(),
    }
    const result = form.id ? await editWorkflowCategory(payload) : await addWorkflowCategory(payload)
    const failure = actionErrorOf(result, '分类保存接口接线中（后端 501）')
    if (failure) {
      formError.value = failure
      return
    }
    showForm.value = false
    notice.value = form.id ? '分类已更新' : '分类已新增'
    await source.load()
  } finally {
    saving.value = false
  }
}

// remove 删除分类并刷新树。
// @param row 目标分类行
// @return 无
async function remove(row: CategoryNode): Promise<void> {
  deletingId.value = row._rid
  actionError.value = ''
  notice.value = ''
  try {
    const result = await deleteWorkflowCategories([row._rid])
    const failure = actionErrorOf(result, '分类删除接口接线中（后端 501）')
    if (failure) actionError.value = failure
    else {
      notice.value = '分类已删除'
      await source.load()
    }
  } finally {
    deletingId.value = ''
  }
}

onMounted(() => {
  void source.load()
})
</script>

<template>
  <div>
    <PageHeader title="流程分类" description="维护流程分类树（/workflow/category 门面）；分类与模型同表存储，仅投影分类字段。">
      <template #actions>
        <n-button size="small" type="primary" @click="openCreate()">新增分类</n-button>
        <n-button size="small" quaternary :loading="source.loading.value" @click="() => source.load()">刷新</n-button>
      </template>
    </PageHeader>

    <div v-if="notice" class="banner" role="status" style="margin: 0 0 10px">{{ notice }}</div>
    <div v-if="actionError" class="banner banner--error" role="alert" style="margin: 0 0 10px">{{ actionError }}</div>
    <div v-if="source.errorMessage.value" class="banner banner--error" role="alert" style="margin: 0 0 10px">
      {{ source.errorMessage.value }}
    </div>

    <DataTable
      :columns="columns"
      :data="treeRows"
      :loading="source.loading.value"
      :total="source.total.value"
      row-key="_rid"
      :empty-description="source.degraded.value || '暂无分类，点击右上角新增分类。'"
      :page-size="source.total.value || 50"
      @update:page-size="() => {}"
    />

    <FormModal
      v-model:show="showForm"
      :title="form.id ? '编辑分类' : '新增分类'"
      :submitting="saving"
      :error="formError"
      @submit="save"
    >
      <n-form-item label="所属分类">
        <n-tree-select
          v-model:value="form.parentId"
          :options="parentOptions"
          clearable
          placeholder="不选则为根分类"
          :disabled="Boolean(form.id)"
        />
      </n-form-item>
      <n-form-item label="分类名称" required>
        <n-input v-model:value="form.categoryName" placeholder="分类名称（必填）" />
      </n-form-item>
      <n-form-item label="分类代号" required>
        <n-input v-model:value="form.categoryCode" placeholder="分类代号（必填，如 leave）" :disabled="Boolean(form.id)" />
      </n-form-item>
      <n-form-item label="排序">
        <n-input-number v-model:value="form.sort" :min="0" style="width: 100%" />
      </n-form-item>
      <n-form-item label="可见">
        <n-switch v-model:value="form.visible" />
      </n-form-item>
      <n-form-item label="启用">
        <n-switch v-model:value="form.enabled" />
      </n-form-item>
      <n-form-item label="备注">
        <n-input v-model:value="form.remark" type="textarea" :rows="2" placeholder="备注（可选）" />
      </n-form-item>
    </FormModal>
  </div>
</template>
