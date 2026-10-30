<script setup lang="ts">
// 组列表：筛选、分页、新建、编辑、删除与组成员维护；筛选写入路由 query。
import { computed, h, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NButton, NInput } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import FilterBar from '@/components/FilterBar.vue'
import DataTable from '@/components/DataTable.vue'
import ErrorBlock from '@/components/ErrorBlock.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import GroupForm from './components/GroupForm.vue'
import GroupMembersPanel from './components/GroupMembersPanel.vue'
import { createGroup, deleteGroup, listGroups, updateGroup } from '@/api/identity'
import type { Group, GroupCreatePayload, GroupUpdatePayload } from '@/api/identity'
import { isAuthError } from '@/api/client'
import { markAuthError } from '@/stores/ui'
import { shortId, textOrDash } from '@/lib/format'

const route = useRoute()
const router = useRouter()

/** 筛选条件 */
const filters = ref({ id: '', name: '', type: '' })
/** 当前页 */
const page = ref(1)
/** 每页条数 */
const pageSize = ref(50)
/** 组数据 */
const rows = ref<Group[]>([])
/** 总数 */
const total = ref(0)
/** 加载态 */
const loading = ref(false)
/** 列表错误 */
const error = ref<unknown>(null)
/** 是否展示表单 */
const showForm = ref(false)
/** 编辑中的组，null 表示新建 */
const editing = ref<Group | null>(null)
/** 提交中 */
const saving = ref(false)
/** 表单错误 */
const formError = ref<unknown>(null)
/** 删除中的组 ID */
const deletingId = ref('')
/** 操作错误 */
const actionError = ref<unknown>(null)
/** 成功提示 */
const success = ref('')
/** 当前展开成员的组 */
const selectedGroup = ref<Group | null>(null)

// 读取路由 query 中的筛选与分页。
// @return 无
function readQuery(): void {
  const query = route.query
  filters.value = {
    id: typeof query.id === 'string' ? query.id : '',
    name: typeof query.name === 'string' ? query.name : '',
    type: typeof query.type === 'string' ? query.type : '',
  }
  const rawPage = Number(typeof query.page === 'string' ? query.page : '')
  page.value = Number.isFinite(rawPage) && rawPage >= 1 ? Math.floor(rawPage) : 1
  const rawSize = Number(typeof query.size === 'string' ? query.size : '')
  pageSize.value = Number.isFinite(rawSize) && rawSize >= 1 ? Math.min(1000, Math.floor(rawSize)) : 50
}

// 将筛选与分页写回路由 query。
// @return 无
async function syncQuery(): Promise<void> {
  const query: Record<string, string> = {}
  if (filters.value.id.trim()) query.id = filters.value.id.trim()
  if (filters.value.name.trim()) query.name = filters.value.name.trim()
  if (filters.value.type.trim()) query.type = filters.value.type.trim()
  if (page.value > 1) query.page = String(page.value)
  if (pageSize.value !== 50) query.size = String(pageSize.value)
  await router.replace({ query })
}

// 按条件加载组列表。
// @return 无
async function load(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    const result = await listGroups({
      id: filters.value.id.trim() || undefined,
      nameLike: filters.value.name.trim() || undefined,
      type: filters.value.type.trim() || undefined,
      start: (page.value - 1) * pageSize.value,
      size: pageSize.value,
      sort: 'id',
      order: 'asc',
    })
    rows.value = result.data ?? []
    total.value = result.total ?? 0
  } catch (cause) {
    error.value = cause
    if (isAuthError(cause)) markAuthError(cause)
  } finally {
    loading.value = false
  }
}

// 查询：回到第一页。
// @return 无
async function search(): Promise<void> {
  page.value = 1
  await syncQuery()
  await load()
}

// 重置筛选。
// @return 无
async function reset(): Promise<void> {
  filters.value = { id: '', name: '', type: '' }
  page.value = 1
  pageSize.value = 50
  await syncQuery()
  await load()
}

// 处理页码变化。
// @param value 新页码
// @return 无
async function changePage(value: number): Promise<void> {
  page.value = value
  await syncQuery()
  await load()
}

// 处理每页条数变化。
// @param value 新每页条数
// @return 无
async function changePageSize(value: number): Promise<void> {
  pageSize.value = Math.min(1000, Math.max(1, value))
  page.value = 1
  await syncQuery()
  await load()
}

// 打开新建表单。
// @return 无
function openCreate(): void {
  editing.value = null
  formError.value = null
  showForm.value = true
}

// 打开编辑表单。
// @param group 目标组
// @return 无
function openEdit(group: Group): void {
  editing.value = group
  formError.value = null
  showForm.value = true
}

// 关闭表单。
// @return 无
function closeForm(): void {
  showForm.value = false
  editing.value = null
  formError.value = null
}

// 提交新建或编辑。
// @param payload 表单载荷
// @return 无
async function save(payload: GroupCreatePayload | GroupUpdatePayload): Promise<void> {
  saving.value = true
  formError.value = null
  success.value = ''
  try {
    if (editing.value) {
      await updateGroup(editing.value.id, payload as GroupUpdatePayload)
      success.value = `已更新组 ${editing.value.id}`
    } else {
      const created = await createGroup(payload as GroupCreatePayload)
      success.value = `已创建组 ${created.id}`
    }
    closeForm()
    await load()
  } catch (cause) {
    formError.value = cause
    markAuthError(cause)
  } finally {
    saving.value = false
  }
}

// 删除组；若正在查看该组成员则收起面板。
// @param group 目标组
// @return 无
async function remove(group: Group): Promise<void> {
  deletingId.value = group.id
  actionError.value = null
  success.value = ''
  try {
    await deleteGroup(group.id)
    success.value = `已删除组 ${group.id}`
    if (selectedGroup.value?.id === group.id) selectedGroup.value = null
    await load()
  } catch (cause) {
    actionError.value = cause
    markAuthError(cause)
  } finally {
    deletingId.value = ''
  }
}

// 展开或收起组成员面板。
// @param group 目标组
// @return 无
function toggleMembers(group: Group): void {
  selectedGroup.value = selectedGroup.value?.id === group.id ? null : group
}

/** 表格列 */
const columns = computed<DataTableColumns<Group>>(() => [
  {
    title: '组 ID',
    key: 'id',
    width: 220,
    render: (row) => h('span', { class: 'mono', title: row.id }, shortId(row.id, 24, 8)),
  },
  { title: '名称', key: 'name', render: (row) => textOrDash(row.name) },
  { title: '类型', key: 'type', render: (row) => textOrDash(row.type) },
  {
    title: '操作',
    key: 'actions',
    width: 220,
    render: (row) =>
      h('div', { class: 'row' }, [
        h(NButton, { size: 'tiny', quaternary: true, onClick: () => toggleMembers(row) }, { default: () => '成员' }),
        h(NButton, { size: 'tiny', quaternary: true, onClick: () => openEdit(row) }, { default: () => '编辑' }),
        h(ConfirmButton, {
          label: '删除',
          title: '删除该组？',
          description: `组 ${row.id} 与全部成员关系将被删除，用户本身不受影响，且不可恢复。`,
          loading: deletingId.value === row.id,
          onConfirm: () => remove(row),
        }),
      ]),
  },
])

onMounted(() => {
  readQuery()
  void load()
})
</script>

<template>
  <div>
    <PageHeader title="组" description="维护组、组成员与用户归属；组 ID 创建后不可修改。">
      <template #actions>
        <n-button size="small" type="primary" @click="openCreate">新建组</n-button>
      </template>
    </PageHeader>

    <FilterBar :loading="loading" @search="search" @reset="reset">
      <label class="filter-bar__item">
        <span class="filter-bar__label">组 ID</span>
        <n-input v-model:value="filters.id" placeholder="精确匹配" clearable />
      </label>
      <label class="filter-bar__item">
        <span class="filter-bar__label">名称</span>
        <n-input v-model:value="filters.name" placeholder="名称模糊匹配，支持 % 与 _" clearable />
      </label>
      <label class="filter-bar__item">
        <span class="filter-bar__label">类型</span>
        <n-input v-model:value="filters.type" placeholder="精确匹配，如 assignment" clearable />
      </label>
    </FilterBar>

    <GroupForm
      v-if="showForm"
      :key="editing?.id ?? 'create'"
      :mode="editing ? 'edit' : 'create'"
      :initial="editing"
      :submitting="saving"
      :error="formError"
      @submit="save"
      @cancel="closeForm"
    />

    <div v-if="success" class="banner" role="status" style="margin: 0 0 12px">{{ success }}</div>
    <ErrorBlock v-if="actionError" :error="actionError" @retry="load" />
    <ErrorBlock v-if="error" :error="error" context="GET /service/identity/groups" @retry="load" />

    <DataTable
      :columns="columns"
      :data="rows"
      :loading="loading"
      :total="total"
      :page="page"
      :page-size="pageSize"
      row-key="id"
      :empty-description="filters.id || filters.name || filters.type ? '没有匹配的组，调整筛选条件后重试。' : '暂无组，点击右上角新建组。'"
      @update:page="changePage"
      @update:page-size="changePageSize"
    />

    <div v-if="selectedGroup" style="margin-top: 14px">
      <GroupMembersPanel :group="selectedGroup" @close="selectedGroup = null" />
    </div>
  </div>
</template>
