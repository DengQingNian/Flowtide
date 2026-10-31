<script setup lang="ts">
// 流程配置页（三 Tab）：步骤管理 / 流程模板（flow_xml 只读预览）/ 流程规则占位。
import { computed, h, onMounted, reactive, ref, watch } from 'vue'
import { NButton, NFormItem, NInput, NInputNumber, NModal, NSelect, NSwitch, NTabs, NTabPane, NTag } from 'naive-ui'
import type { DataTableColumns, SelectOption } from 'naive-ui'
import {
  addWorkflowStep,
  addWorkflowTemplate,
  deleteWorkflowSteps,
  deleteWorkflowTemplates,
  editWorkflowStep,
  editWorkflowTemplate,
  extractRows,
  listWorkflowModels,
  listWorkflowSteps,
  listWorkflowTemplates,
} from '@/api/workflow'
import type { KeyedRow, WorkflowModel, WorkflowStep, WorkflowTemplate } from '@/api/workflow'
import { formatDateTime, textOrDash } from '@/lib/format'
import PageHeader from '@/components/PageHeader.vue'
import DataTable from '@/components/DataTable.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import EmptyState from '@/components/EmptyState.vue'
import XmlViewer from '@/components/XmlViewer.vue'
import FormModal from './components/FormModal.vue'
import { actionErrorOf, boolField, pickField, useWorkflowListSource } from './helpers'

/** 当前激活 Tab */
const activeTab = ref('step')

/** 已选模型（步骤管理 Tab 数据范围） */
const selectedModelId = ref<string | null>(null)
/** 模型下拉选项 */
const modelOptions = ref<SelectOption[]>([])
/** 模型选项加载中 */
const modelLoading = ref(false)

// ── Tab1：步骤管理 ──

/** 步骤表单字段 */
const stepForm = reactive({
  id: '',
  stepName: '',
  stepCode: '',
  sort: 0,
  allowReturn: false,
  backStepCodes: '',
  returnApprovalStep: false,
  url: '',
})
const showStepForm = ref(false)
const savingStep = ref(false)
const stepFormError = ref('')
const deletingStepId = ref('')

/** 步骤数据源（随所选模型刷新） */
const stepSource = useWorkflowListSource<WorkflowStep>(
  () =>
    listWorkflowSteps({
      modelId: selectedModelId.value ?? undefined,
      param: { modelId: selectedModelId.value ?? undefined },
    }),
  '步骤列表接口接线中（后端 501），暂无数据',
)

/** 步骤表格列（核心字段子集，全字段集待设计器接入后开放） */
const stepColumns = computed<DataTableColumns<WorkflowStep & KeyedRow>>(() => [
  {
    title: '步骤名称',
    key: 'stepName',
    minWidth: 150,
    render: (row) => pickField<string>(row, 'stepName', 'step_name', 'name') ?? row._rid,
  },
  {
    title: '步骤代号',
    key: 'stepCode',
    width: 130,
    render: (row) => h('span', { class: 'mono' }, pickField<string>(row, 'stepCode', 'step_code', 'code') ?? '—'),
  },
  { title: '排序', key: 'sort', width: 70, render: (row) => String(pickField<number>(row, 'sort', 'sortNo') ?? '—') },
  {
    title: '允许退回',
    key: 'allowReturn',
    width: 90,
    render: (row) => (boolField(row, 'allowReturn', 'allow_return') ? '是' : '否'),
  },
  {
    title: '可退步骤',
    key: 'backStepCodes',
    minWidth: 140,
    render: (row) => {
      const codes = pickField<string | string[]>(row, 'backStepCodes', 'back_step_codes')
      if (!codes) return '—'
      const text = Array.isArray(codes) ? codes.join(', ') : codes
      return h('span', { class: 'mono' }, text)
    },
  },
  {
    title: '退回审批',
    key: 'returnApprovalStep',
    width: 90,
    render: (row) => (boolField(row, 'returnApprovalStep', 'return_approval_step') ? '是' : '否'),
  },
  {
    title: 'URL',
    key: 'url',
    minWidth: 140,
    render: (row) => h('span', { class: 'mono' }, textOrDash(pickField<string>(row, 'url'))),
  },
  {
    title: '操作',
    key: 'actions',
    width: 130,
    render: (row) =>
      h('div', { class: 'row' }, [
        h(NButton, { size: 'tiny', quaternary: true, onClick: () => openStepEdit(row) }, { default: () => '编辑' }),
        h(ConfirmButton, {
          label: '删除',
          title: '确认删除该步骤？',
          description: '删除会影响该模型流程流转配置，操作不可恢复。',
          loading: deletingStepId.value === row._rid,
          onConfirm: () => removeStep(row),
        }),
      ]),
  },
])

// loadModels 拉取模型下拉选项；失败时下拉为空并在 Tab 内提示列表错误。
// @return 无
async function loadModels(): Promise<void> {
  modelLoading.value = true
  try {
    const result = await listWorkflowModels({ pageNum: 1, pageSize: 200 })
    if (result.success) {
      const models = extractRows<WorkflowModel>(result.data)
      modelOptions.value = models.map((row) => ({
        label: `${pickField<string>(row, 'modelName', 'model_name', 'name') ?? row._rid}（${pickField<string>(row, 'modelCode', 'model_code', 'code') ?? row._rid}）`,
        value: row._rid,
      }))
      // 默认选中第一个模型，便于直接查看步骤
      if (!selectedModelId.value && models.length) selectedModelId.value = models[0]._rid
    }
  } finally {
    modelLoading.value = false
  }
}

// openStepCreate 打开新增步骤弹窗。
// @return 无
function openStepCreate(): void {
  stepForm.id = ''
  stepForm.stepName = ''
  stepForm.stepCode = ''
  stepForm.sort = 0
  stepForm.allowReturn = false
  stepForm.backStepCodes = ''
  stepForm.returnApprovalStep = false
  stepForm.url = ''
  stepFormError.value = ''
  showStepForm.value = true
}

// openStepEdit 打开编辑步骤弹窗并回填。
// @param row 目标步骤行
// @return 无
function openStepEdit(row: WorkflowStep & KeyedRow): void {
  stepForm.id = row._rid
  stepForm.stepName = pickField<string>(row, 'stepName', 'step_name', 'name') ?? ''
  stepForm.stepCode = pickField<string>(row, 'stepCode', 'step_code', 'code') ?? ''
  stepForm.sort = pickField<number>(row, 'sort', 'sortNo') ?? 0
  stepForm.allowReturn = boolField(row, 'allowReturn', 'allow_return')
  const codes = pickField<string | string[]>(row, 'backStepCodes', 'back_step_codes')
  stepForm.backStepCodes = Array.isArray(codes) ? codes.join(',') : codes ?? ''
  stepForm.returnApprovalStep = boolField(row, 'returnApprovalStep', 'return_approval_step')
  stepForm.url = pickField<string>(row, 'url') ?? ''
  stepFormError.value = ''
  showStepForm.value = true
}

// saveStep 提交新增或编辑步骤。
// @return 无
async function saveStep(): Promise<void> {
  if (!selectedModelId.value) {
    stepFormError.value = '请先在上方选择流程模型'
    return
  }
  if (!stepForm.stepName.trim() || !stepForm.stepCode.trim()) {
    stepFormError.value = '步骤名称与步骤代号为必填项'
    return
  }
  savingStep.value = true
  stepFormError.value = ''
  try {
    const payload = {
      id: stepForm.id || undefined,
      modelId: selectedModelId.value,
      stepName: stepForm.stepName.trim(),
      stepCode: stepForm.stepCode.trim(),
      sort: stepForm.sort,
      allowReturn: stepForm.allowReturn,
      backStepCodes: stepForm.backStepCodes.trim(),
      returnApprovalStep: stepForm.returnApprovalStep,
      url: stepForm.url.trim(),
    }
    const result = stepForm.id ? await editWorkflowStep(payload) : await addWorkflowStep(payload)
    const failure = actionErrorOf(result, '步骤保存接口接线中（后端 501）')
    if (failure) {
      stepFormError.value = failure
      return
    }
    showStepForm.value = false
    await stepSource.load()
  } finally {
    savingStep.value = false
  }
}

// removeStep 删除步骤并刷新列表。
// @param row 目标步骤行
// @return 无
async function removeStep(row: WorkflowStep & KeyedRow): Promise<void> {
  deletingStepId.value = row._rid
  try {
    const result = await deleteWorkflowSteps([row._rid])
    const failure = actionErrorOf(result, '步骤删除接口接线中（后端 501）')
    if (failure) stepSource.errorMessage.value = failure
    else await stepSource.load()
  } finally {
    deletingStepId.value = ''
  }
}

// ── Tab2：流程模板 ──

/** 模板 XML 预览弹窗 */
const xmlPreview = ref('')
/** XML 预览弹窗标题 */
const xmlPreviewTitle = ref('')
const showXmlPreview = ref(false)

/** 模板表单字段 */
const templateForm = reactive({
  id: '',
  templateName: '',
  templateCode: '',
  remark: '',
})
const showTemplateForm = ref(false)
const savingTemplate = ref(false)
const templateFormError = ref('')
const deletingTemplateId = ref('')

/** 模板数据源 */
const templateSource = useWorkflowListSource<WorkflowTemplate>(
  () => listWorkflowTemplates({ pageNum: 1, pageSize: 100 }),
  '模板列表接口接线中（后端 501），暂无数据',
)

/** 模板表格列 */
const templateColumns = computed<DataTableColumns<WorkflowTemplate & KeyedRow>>(() => [
  {
    title: '模板名称',
    key: 'templateName',
    minWidth: 160,
    render: (row) => pickField<string>(row, 'templateName', 'template_name', 'name') ?? row._rid,
  },
  {
    title: '模板代号',
    key: 'templateCode',
    width: 140,
    render: (row) => h('span', { class: 'mono' }, pickField<string>(row, 'templateCode', 'template_code', 'code') ?? '—'),
  },
  {
    title: '备注',
    key: 'remark',
    minWidth: 120,
    render: (row) => textOrDash(pickField<string>(row, 'remark')),
  },
  {
    title: '更新时间',
    key: 'updateTime',
    width: 170,
    render: (row) => formatDateTime(pickField<string>(row, 'updateTime', 'update_time', 'lastUpdateTime')),
  },
  {
    title: '操作',
    key: 'actions',
    width: 190,
    render: (row) =>
      h('div', { class: 'row' }, [
        h(
          NButton,
          {
            size: 'tiny',
            quaternary: true,
            disabled: !pickField<string>(row, 'flowXml', 'flow_xml', 'xml'),
            onClick: () => openXmlPreview(row),
          },
          { default: () => 'XML 预览' },
        ),
        h(NButton, { size: 'tiny', quaternary: true, onClick: () => openTemplateEdit(row) }, { default: () => '编辑' }),
        h(ConfirmButton, {
          label: '删除',
          title: '确认删除该模板？',
          loading: deletingTemplateId.value === row._rid,
          onConfirm: () => removeTemplate(row),
        }),
      ]),
  },
])

// openXmlPreview 打开 flow_xml 只读预览弹窗。
// @param row 目标模板行
// @return 无
function openXmlPreview(row: WorkflowTemplate & KeyedRow): void {
  xmlPreview.value = pickField<string>(row, 'flowXml', 'flow_xml', 'xml') ?? ''
  xmlPreviewTitle.value = `流程模板 XML · ${pickField<string>(row, 'templateName', 'name') ?? row._rid}`
  showXmlPreview.value = true
}

// openTemplateCreate 打开新增模板弹窗。
// @return 无
function openTemplateCreate(): void {
  templateForm.id = ''
  templateForm.templateName = ''
  templateForm.templateCode = ''
  templateForm.remark = ''
  templateFormError.value = ''
  showTemplateForm.value = true
}

// openTemplateEdit 打开编辑模板弹窗并回填。
// @param row 目标模板行
// @return 无
function openTemplateEdit(row: WorkflowTemplate & KeyedRow): void {
  templateForm.id = row._rid
  templateForm.templateName = pickField<string>(row, 'templateName', 'template_name', 'name') ?? ''
  templateForm.templateCode = pickField<string>(row, 'templateCode', 'template_code', 'code') ?? ''
  templateForm.remark = pickField<string>(row, 'remark') ?? ''
  templateFormError.value = ''
  showTemplateForm.value = true
}

// saveTemplate 提交新增或编辑模板（flow_xml 本期只读，不在此编辑）。
// @return 无
async function saveTemplate(): Promise<void> {
  if (!templateForm.templateName.trim() || !templateForm.templateCode.trim()) {
    templateFormError.value = '模板名称与模板代号为必填项'
    return
  }
  savingTemplate.value = true
  templateFormError.value = ''
  try {
    const payload = {
      id: templateForm.id || undefined,
      templateName: templateForm.templateName.trim(),
      templateCode: templateForm.templateCode.trim(),
      remark: templateForm.remark.trim(),
    }
    const result = templateForm.id ? await editWorkflowTemplate(payload) : await addWorkflowTemplate(payload)
    const failure = actionErrorOf(result, '模板保存接口接线中（后端 501）')
    if (failure) {
      templateFormError.value = failure
      return
    }
    showTemplateForm.value = false
    await templateSource.load()
  } finally {
    savingTemplate.value = false
  }
}

// removeTemplate 删除模板并刷新列表。
// @param row 目标模板行
// @return 无
async function removeTemplate(row: WorkflowTemplate & KeyedRow): Promise<void> {
  deletingTemplateId.value = row._rid
  try {
    const result = await deleteWorkflowTemplates([row._rid])
    const failure = actionErrorOf(result, '模板删除接口接线中（后端 501）')
    if (failure) templateSource.errorMessage.value = failure
    else await templateSource.load()
  } finally {
    deletingTemplateId.value = ''
  }
}

// 选中模型变化时重新拉取步骤列表。
watch(selectedModelId, () => {
  void stepSource.load()
})

onMounted(() => {
  void loadModels()
  void templateSource.load()
})
</script>

<template>
  <div>
    <PageHeader
      title="流程配置"
      description="步骤管理 / 流程模板 / 流程规则三合一；BPMN 设计器接入后在此页开放画布编辑。"
    />

    <n-tabs v-model:value="activeTab" type="line" animated>
      <n-tab-pane name="step" tab="步骤管理">
        <div class="filter-bar" style="margin-bottom: 12px">
          <div class="filter-bar__item" style="min-width: 280px">
            <span class="filter-bar__label">流程模型</span>
            <n-select
              v-model:value="selectedModelId"
              :options="modelOptions"
              :loading="modelLoading"
              size="small"
              clearable
              placeholder="选择模型查看步骤"
            />
          </div>
          <div class="row" style="margin-left: auto">
            <n-button size="small" type="primary" :disabled="!selectedModelId" @click="openStepCreate">新增步骤</n-button>
            <n-button size="small" quaternary :loading="stepSource.loading.value" @click="() => stepSource.load()">刷新</n-button>
          </div>
        </div>

        <div v-if="stepSource.errorMessage.value" class="banner banner--error" role="alert" style="margin: 0 0 10px">
          {{ stepSource.errorMessage.value }}
        </div>
        <div v-if="!selectedModelId && !modelLoading" style="margin-bottom: 10px">
          <NTag size="small" type="info">请先选择流程模型</NTag>
        </div>

        <DataTable
          :columns="stepColumns"
          :data="stepSource.rows.value"
          :loading="stepSource.loading.value"
          :total="stepSource.total.value"
          row-key="_rid"
          :empty-description="stepSource.degraded.value || '该模型暂无步骤，点击右上角新增步骤。'"
          :page-size="stepSource.total.value || 50"
          @update:page-size="() => {}"
        />
      </n-tab-pane>

      <n-tab-pane name="template" tab="流程模板">
        <div class="row" style="justify-content: flex-end; margin-bottom: 12px">
          <n-button size="small" type="primary" @click="openTemplateCreate">新增模板</n-button>
          <n-button size="small" quaternary :loading="templateSource.loading.value" @click="() => templateSource.load()">刷新</n-button>
        </div>

        <div v-if="templateSource.errorMessage.value" class="banner banner--error" role="alert" style="margin: 0 0 10px">
          {{ templateSource.errorMessage.value }}
        </div>

        <DataTable
          :columns="templateColumns"
          :data="templateSource.rows.value"
          :loading="templateSource.loading.value"
          :total="templateSource.total.value"
          row-key="_rid"
          :empty-description="templateSource.degraded.value || '暂无流程模板，BPMN 设计器接入后可在此维护模板。'"
          :page-size="templateSource.total.value || 50"
          @update:page-size="() => {}"
        />
      </n-tab-pane>

      <n-tab-pane name="rule" tab="流程规则">
        <EmptyState
          title="流程规则待开放"
          description="流程规则为建模约定静态文档（_begin/_end、Draft/Sign、${initiator}、权重表达式等），待 BPMN 设计器接入后开放。"
        />
      </n-tab-pane>
    </n-tabs>

    <FormModal
      v-model:show="showStepForm"
      :title="stepForm.id ? '编辑步骤' : '新增步骤'"
      :submitting="savingStep"
      :error="stepFormError"
      @submit="saveStep"
    >
      <n-form-item label="步骤名称" required>
        <n-input v-model:value="stepForm.stepName" placeholder="步骤名称（必填）" />
      </n-form-item>
      <n-form-item label="步骤代号" required>
        <n-input v-model:value="stepForm.stepCode" placeholder="步骤代号（必填，如 step_audit）" :disabled="Boolean(stepForm.id)" />
      </n-form-item>
      <n-form-item label="排序">
        <n-input-number v-model:value="stepForm.sort" :min="0" style="width: 100%" />
      </n-form-item>
      <n-form-item label="允许退回">
        <n-switch v-model:value="stepForm.allowReturn" />
      </n-form-item>
      <n-form-item label="可退步骤">
        <n-input v-model:value="stepForm.backStepCodes" placeholder="允许退回到的步骤代号，逗号分隔（仅限排序更小的步骤）" />
      </n-form-item>
      <n-form-item label="退回审批">
        <n-switch v-model:value="stepForm.returnApprovalStep" />
      </n-form-item>
      <n-form-item label="URL">
        <n-input v-model:value="stepForm.url" placeholder="步骤表单/跳转 URL（可选）" class="mono" />
      </n-form-item>
    </FormModal>

    <FormModal
      v-model:show="showTemplateForm"
      :title="templateForm.id ? '编辑模板' : '新增模板'"
      :submitting="savingTemplate"
      :error="templateFormError"
      @submit="saveTemplate"
    >
      <n-form-item label="模板名称" required>
        <n-input v-model:value="templateForm.templateName" placeholder="模板名称（必填）" />
      </n-form-item>
      <n-form-item label="模板代号" required>
        <n-input v-model:value="templateForm.templateCode" placeholder="模板代号（必填）" :disabled="Boolean(templateForm.id)" />
      </n-form-item>
      <n-form-item label="备注">
        <n-input v-model:value="templateForm.remark" type="textarea" :rows="2" placeholder="备注（可选）" />
      </n-form-item>
    </FormModal>

    <n-modal v-model:show="showXmlPreview" preset="card" :title="xmlPreviewTitle" style="width: 820px">
      <XmlViewer :xml="xmlPreview" filename="template.bpmn20.xml" />
    </n-modal>
  </div>
</template>
