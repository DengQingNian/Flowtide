<script setup lang="ts">
// 用户列表：筛选、分页、新建、编辑与删除；筛选条件写入路由 query 以便刷新还原。
import { computed, h, onMounted, ref } from 'vue'
import { useRoute, useRouter, RouterLink } from 'vue-router'
import { NButton, NInput } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import FilterBar from '@/components/FilterBar.vue'
import DataTable from '@/components/DataTable.vue'
import ErrorBlock from '@/components/ErrorBlock.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import UserForm from './components/UserForm.vue'
import UserAvatar from './components/UserAvatar.vue'
import { createUser, deleteUser, listUsers, updateUser } from '@/api/identity'
import type { User, UserCreatePayload, UserUpdatePayload } from '@/api/identity'
import { isAuthError } from '@/api/client'
import { markAuthError } from '@/stores/ui'

const route = useRoute()
const router = useRouter()

/** 筛选条件 */
const filters = ref({ id: '', name: '', email: '' })
/** 当前页，从 1 开始 */
const page = ref(1)
/** 每页条数 */
const pageSize = ref(50)
/** 用户数据 */
const rows = ref<User[]>([])
/** 总数 */
const total = ref(0)
/** 加载态 */
const loading = ref(false)
/** 列表错误 */
const error = ref<unknown>(null)
/** 是否展示表单 */
const showForm = ref(false)
/** 编辑中的用户，null 表示新建 */
const editing = ref<User | null>(null)
/** 表单提交中 */
const saving = ref(false)
/** 表单错误 */
const formError = ref<unknown>(null)
/** 删除中的用户 ID */
const deletingId = ref('')
/** 删除错误 */
const actionError = ref<unknown>(null)
/** 成功提示 */
const success = ref('')

// 读取路由 query 中的筛选与分页，刷新后可还原。
// @return 无
function readQuery(): void {
  const query = route.query
  filters.value = {
    id: typeof query.id === 'string' ? query.id : '',
    name: typeof query.name === 'string' ? query.name : '',
    email: typeof query.email === 'string' ? query.email : '',
  }
  const rawPage = Number(typeof query.page === 'string' ? query.page : '')
  page.value = Number.isFinite(rawPage) && rawPage >= 1 ? Math.floor(rawPage) : 1
  const rawSize = Number(typeof query.size === 'string' ? query.size : '')
  pageSize.value = Number.isFinite(rawSize) && rawSize >= 1 ? Math.min(1000, Math.floor(rawSize)) : 50
}

// 将当前筛选与分页写回路由 query，省略默认值。
// @return 无
async function syncQuery(): Promise<void> {
  const query: Record<string, string> = {}
  if (filters.value.id.trim()) query.id = filters.value.id.trim()
  if (filters.value.name.trim()) query.name = filters.value.name.trim()
  if (filters.value.email.trim()) query.email = filters.value.email.trim()
  if (page.value > 1) query.page = String(page.value)
  if (pageSize.value !== 50) query.size = String(pageSize.value)
  await router.replace({ query })
}

// 按当前条件加载用户列表。
// @return 无
async function load(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    const result = await listUsers({
      id: filters.value.id.trim() || undefined,
      displayNameLike: filters.value.name.trim() || undefined,
      emailLike: filters.value.email.trim() || undefined,
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

// 查询：回到第一页并同步路由。
// @return 无
async function search(): Promise<void> {
  page.value = 1
  await syncQuery()
  await load()
}

// 重置筛选与分页。
// @return 无
async function reset(): Promise<void> {
  filters.value = { id: '', name: '', email: '' }
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
// @param user 目标用户
// @return 无
function openEdit(user: User): void {
  editing.value = user
  formError.value = null
  showForm.value = true
}

// 关闭表单并清理错误。
// @return 无
function closeForm(): void {
  showForm.value = false
  editing.value = null
  formError.value = null
}

// 提交新建或编辑。
// @param payload 表单载荷
// @return 无
async function save(payload: UserCreatePayload | UserUpdatePayload): Promise<void> {
  saving.value = true
  formError.value = null
  success.value = ''
  try {
    if (editing.value) {
      await updateUser(editing.value.id, payload as UserUpdatePayload)
      success.value = `已更新用户 ${editing.value.id}`
    } else {
      const created = await createUser(payload as UserCreatePayload)
      success.value = `已创建用户 ${created.id}`
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

// 删除用户并刷新列表。
// @param user 目标用户
// @return 无
async function remove(user: User): Promise<void> {
  deletingId.value = user.id
  actionError.value = null
  success.value = ''
  try {
    await deleteUser(user.id)
    success.value = `已删除用户 ${user.id}`
    await load()
  } catch (cause) {
    actionError.value = cause
    markAuthError(cause)
  } finally {
    deletingId.value = ''
  }
}

/** 表格列 */
const columns = computed<DataTableColumns<User>>(() => [
  {
    title: '用户',
    key: 'id',
    width: 240,
    render: (row) =>
      h('div', { class: 'row' }, [
        h(UserAvatar, { user: row, size: 24 }),
        h(RouterLink, { to: `/identity/users/${encodeURIComponent(row.id)}` }, { default: () => h('span', { class: 'mono' }, row.id) }),
      ]),
  },
  { title: '显示名', key: 'displayName', render: (row) => row.displayName || '—' },
  {
    title: '姓名',
    key: 'name',
    render: (row) => {
      const name = [row.lastName, row.firstName].filter(Boolean).join(' ')
      return name || '—'
    },
  },
  { title: '邮箱', key: 'email', render: (row) => row.email || '—' },
  { title: '租户', key: 'tenantId', width: 120, render: (row) => row.tenantId || '—' },
  {
    title: '操作',
    key: 'actions',
    width: 160,
    render: (row) =>
      h('div', { class: 'row' }, [
        h(NButton, { size: 'tiny', quaternary: true, onClick: () => openEdit(row) }, { default: () => '编辑' }),
        h(ConfirmButton, {
          label: '删除',
          title: '删除该用户？',
          description: `用户 ${row.id} 的组关系、用户信息与头像将一并删除，且不可恢复。`,
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
    <PageHeader title="用户" description="维护用户资料、角色、所属组、信息键值与头像。">
      <template #actions>
        <n-button size="small" type="primary" @click="openCreate">新建用户</n-button>
      </template>
    </PageHeader>

    <FilterBar :loading="loading" @search="search" @reset="reset">
      <label class="filter-bar__item">
        <span class="filter-bar__label">用户 ID</span>
        <n-input v-model:value="filters.id" placeholder="精确匹配" clearable data-test="filter-id" />
      </label>
      <label class="filter-bar__item">
        <span class="filter-bar__label">姓名</span>
        <n-input v-model:value="filters.name" placeholder="显示名模糊匹配，支持 % 与 _" clearable data-test="filter-name" />
      </label>
      <label class="filter-bar__item">
        <span class="filter-bar__label">邮箱</span>
        <n-input v-model:value="filters.email" placeholder="邮箱模糊匹配" clearable data-test="filter-email" />
      </label>
    </FilterBar>

    <UserForm
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
    <ErrorBlock v-if="error" :error="error" context="GET /service/identity/users" @retry="load" />

    <DataTable
      :columns="columns"
      :data="rows"
      :loading="loading"
      :total="total"
      :page="page"
      :page-size="pageSize"
      row-key="id"
      :empty-description="filters.id || filters.name || filters.email ? '没有匹配的用户，调整筛选条件后重试。' : '暂无用户，点击右上角新建用户。'"
      @update:page="changePage"
      @update:page-size="changePageSize"
    />
  </div>
</template>
