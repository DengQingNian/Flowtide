<script setup lang="ts">
// 流程模型列表：筛选、分页、新建、编辑元信息与删除，条件写入路由 query。
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NButton, NInput, NSelect } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import FilterBar from '@/components/FilterBar.vue'
import DataTable from '@/components/DataTable.vue'
import ErrorBlock from '@/components/ErrorBlock.vue'
import ModelFormDialog from './components/model/ModelFormDialog.vue'
import { buildModelColumns } from './components/model/modelTable'
import {
  createModel,
  deleteModel,
  listModels,
  saveModelSource,
  sourceFileName,
  updateModel,
  type Model,
} from '@/api/models'
import { errorMessage } from '@/api/client'
import { buildMinimalBpmn } from '@/design/minimalBpmn'
import { markAuthError } from '@/stores/ui'

/** 表单提交值 */
interface FormPayload { name: string; key?: string; category?: string; metaInfo?: string }

/** 筛选与分页状态 */
interface FilterState { nameLike: string; key: string; category: string; deployed: string; start: number; size: number }

const route = useRoute()
const router = useRouter()

/** 筛选条件 */
const filters = reactive<FilterState>({ nameLike: '', key: '', category: '', deployed: '', start: 0, size: 50 })
/** 模型数据 */
const rows = ref<Model[]>([])
/** 总条数 */
const total = ref(0)
/** 列表加载中 */
const loading = ref(false)
/** 列表错误 */
const error = ref<unknown>(null)
/** 操作错误 */
const actionError = ref<unknown>(null)
/** 操作结果提示 */
const notice = ref('')
/** 新建弹窗 */
const createOpen = ref(false)
const createLoading = ref(false)
const createError = ref<unknown>(null)
/** 编辑弹窗 */
const editOpen = ref(false)
const editTarget = ref<Model | null>(null)
const editLoading = ref(false)
const editError = ref<unknown>(null)
/** 正在删除的模型 ID */
const deletingId = ref('')

/** 部署状态选项 */
const deployedOptions = [
  { label: '全部', value: '' },
  { label: '已部署', value: 'true' },
  { label: '未部署', value: 'false' },
]

/** 当前页码 */
const page = computed(() => Math.floor(filters.start / filters.size) + 1)

// 读取路由 query 中的单个字符串。
// @param value 查询值
// @return 字符串
function queryString(value: unknown): string {
  if (Array.isArray(value)) return value.length > 0 ? String(value[0]) : ''
  return value === null || value === undefined ? '' : String(value)
}

// 读取路由 query 中的非负数字。
// @param value 查询值
// @param fallback 缺省值
// @return 数字
function queryNumber(value: unknown, fallback: number): number {
  const raw = queryString(value)
  if (!raw) return fallback
  const parsed = Number(raw)
  return Number.isFinite(parsed) && parsed >= 0 ? parsed : fallback
}

// 读取筛选文本并去除首尾空白，兼容输入框清空后的 null。
// @param value 筛选值
// @return 去空白文本
function trimValue(value: string | null | undefined): string {
  return (value ?? '').trim()
}

// 从路由 query 还原筛选条件。
// @return 无
function readFilters(): void {
  filters.nameLike = queryString(route.query.nameLike)
  filters.key = queryString(route.query.key)
  filters.category = queryString(route.query.category)
  const deployed = queryString(route.query.deployed)
  filters.deployed = deployed === 'true' || deployed === 'false' ? deployed : ''
  filters.size = Math.min(1000, Math.max(1, queryNumber(route.query.size, 50)))
  filters.start = queryNumber(route.query.start, 0)
}

// 将筛选条件编码为路由 query，空条件不写入。
// @return 查询对象
function buildRouteQuery(): Record<string, string> {
  const query: Record<string, string> = {}
  if (trimValue(filters.nameLike)) query.nameLike = trimValue(filters.nameLike)
  if (trimValue(filters.key)) query.key = trimValue(filters.key)
  if (trimValue(filters.category)) query.category = trimValue(filters.category)
  if (filters.deployed) query.deployed = filters.deployed
  if (filters.start > 0) query.start = String(filters.start)
  if (filters.size !== 50) query.size = String(filters.size)
  return query
}

// 判断目标 query 与当前路由是否一致。
// @param next 目标 query
// @return 是否一致
function sameQuery(next: Record<string, string>): boolean {
  const current = Object.keys(route.query)
    .map((key) => [key, queryString(route.query[key])] as const)
    .sort(([a], [b]) => a.localeCompare(b))
  const target = Object.entries(next).sort(([a], [b]) => a.localeCompare(b))
  return JSON.stringify(current) === JSON.stringify(target)
}

// 加载模型列表。
// @return 无
async function load(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    const result = await listModels({
      nameLike: trimValue(filters.nameLike) || undefined,
      key: trimValue(filters.key) || undefined,
      category: trimValue(filters.category) || undefined,
      deployed: filters.deployed === '' ? undefined : filters.deployed === 'true',
      start: filters.start,
      size: filters.size,
      sort: 'lastUpdateTime',
      order: 'desc',
    })
    rows.value = result.data
    total.value = result.total
  } catch (loadError) {
    markAuthError(loadError)
    error.value = loadError
  } finally {
    loading.value = false
  }
}

// 把筛选条件写回路由并加载；条件未变化时直接刷新。
// @return 无
async function commit(): Promise<void> {
  const query = buildRouteQuery()
  if (sameQuery(query)) {
    await load()
    return
  }
  await router.replace({ query })
}

// 执行查询，重置到第一页。
// @return 无
async function search(): Promise<void> {
  filters.start = 0
  await commit()
}

// 重置全部筛选条件。
// @return 无
async function reset(): Promise<void> {
  filters.nameLike = ''
  filters.key = ''
  filters.category = ''
  filters.deployed = ''
  filters.start = 0
  filters.size = 50
  await commit()
}

// 切换页码。
// @param next 页码，从 1 开始
// @return 无
async function changePage(next: number): Promise<void> {
  filters.start = Math.max(0, (next - 1) * filters.size)
  await commit()
}

// 切换每页条数并回到第一页。
// @param size 每页条数
// @return 无
async function changePageSize(size: number): Promise<void> {
  filters.size = Math.min(1000, Math.max(1, size))
  filters.start = 0
  await commit()
}

// 打开新建弹窗。
// @return 无
function openCreate(): void {
  createError.value = null
  createOpen.value = true
}

// 新建模型：先创建元信息，再写入最小 BPMN，最后进入设计器。
// @param payload 表单值
// @return 无
async function submitCreate(payload: FormPayload): Promise<void> {
  createLoading.value = true
  createError.value = null
  try {
    const created = await createModel({
      name: payload.name,
      key: payload.key ?? '',
      category: payload.category,
      metaInfo: payload.metaInfo,
    })
    try {
      await saveModelSource(created.id, buildMinimalBpmn(created.key, created.name), sourceFileName(created))
    } catch (sourceError) {
      // 模型已创建但源码写入失败时不再重复创建，提示从列表进入设计器
      markAuthError(sourceError)
      createOpen.value = false
      notice.value = `模型「${created.name}」已创建，但最小源码写入失败：${errorMessage(sourceError)}；可从列表进入设计器后保存。`
      await load()
      return
    }
    createOpen.value = false
    notice.value = `已创建模型「${created.name}」，正在打开设计器。`
    await router.push({ name: 'model-designer', params: { id: created.id } })
  } catch (submitError) {
    markAuthError(submitError)
    createError.value = submitError
  } finally {
    createLoading.value = false
  }
}

// 打开编辑弹窗。
// @param model 目标模型
// @return 无
function openEdit(model: Model): void {
  editTarget.value = model
  editError.value = null
  editOpen.value = true
}

// 保存模型元信息。
// @param payload 表单值
// @return 无
async function submitEdit(payload: FormPayload): Promise<void> {
  const target = editTarget.value
  if (!target) return
  editLoading.value = true
  editError.value = null
  try {
    await updateModel(target.id, {
      name: payload.name,
      category: payload.category ?? '',
      metaInfo: payload.metaInfo ?? '',
    })
    editOpen.value = false
    notice.value = `已更新模型「${payload.name}」的元信息。`
    await load()
  } catch (submitError) {
    markAuthError(submitError)
    editError.value = submitError
  } finally {
    editLoading.value = false
  }
}

// 删除模型；删除当前页最后一条时回退一页。
// @param model 目标模型
// @return 无
async function remove(model: Model): Promise<void> {
  deletingId.value = model.id
  actionError.value = null
  try {
    await deleteModel(model.id)
    notice.value = `已删除模型「${model.name || model.key}」。`
    if (rows.value.length === 1 && filters.start > 0) filters.start = Math.max(0, filters.start - filters.size)
    await commit()
  } catch (removeError) {
    markAuthError(removeError)
    actionError.value = removeError
  } finally {
    deletingId.value = ''
  }
}

// 跳转模型设计器。
// @param model 目标模型
// @return 无
async function openDesigner(model: Model): Promise<void> {
  await router.push({ name: 'model-designer', params: { id: model.id } })
}

/** 表格列 */
const columns = computed(() =>
  buildModelColumns({
    onDesign: (row) => void openDesigner(row),
    onEdit: openEdit,
    onDelete: (row) => void remove(row),
    isDeleting: (row) => deletingId.value === row.id,
  }),
)

watch(
  () => route.query,
  () => {
    readFilters()
    void load()
  },
  { deep: true },
)

onMounted(() => {
  readFilters()
  void load()
})
</script>

<template>
  <div>
    <PageHeader title="流程模型" description="维护 BPMN 模型元信息，进入设计器编辑、保存并部署为流程定义。">
      <template #actions>
        <n-button size="small" type="primary" data-testid="create-model" @click="openCreate">新建模型</n-button>
      </template>
    </PageHeader>

    <ErrorBlock v-if="actionError" :error="actionError" context="删除模型" @retry="load" />
    <div v-if="notice" class="banner" role="status">{{ notice }}</div>

    <FilterBar :loading="loading" @search="search" @reset="reset">
      <div class="filter-bar__item">
        <span class="filter-bar__label">名称</span>
        <n-input v-model:value="filters.nameLike" clearable placeholder="按名称模糊搜索" @keyup.enter="search" />
      </div>
      <div class="filter-bar__item">
        <span class="filter-bar__label">Key</span>
        <n-input v-model:value="filters.key" clearable placeholder="按 Key 精确匹配" @keyup.enter="search" />
      </div>
      <div class="filter-bar__item">
        <span class="filter-bar__label">分类</span>
        <n-input v-model:value="filters.category" clearable placeholder="按分类精确匹配" @keyup.enter="search" />
      </div>
      <div class="filter-bar__item">
        <span class="filter-bar__label">部署状态</span>
        <n-select v-model:value="filters.deployed" :options="deployedOptions" style="width: 150px" />
      </div>
    </FilterBar>

    <DataTable
      :columns="columns"
      :data="rows"
      :loading="loading"
      :total="total"
      :page="page"
      :page-size="filters.size"
      :page-sizes="[10, 20, 50, 100, 200, 500, 1000]"
      :error="error"
      :scroll-x="1050"
      empty-description="暂无流程模型，点击“新建模型”开始设计。"
      @update:page="changePage"
      @update:page-size="changePageSize"
      @retry="load"
    />

    <ModelFormDialog
      v-model:open="createOpen"
      mode="create"
      :loading="createLoading"
      :error="createError"
      @submit="submitCreate"
    />
    <ModelFormDialog
      v-model:open="editOpen"
      mode="edit"
      :model="editTarget"
      :loading="editLoading"
      :error="editError"
      @submit="submitEdit"
    />
  </div>
</template>
