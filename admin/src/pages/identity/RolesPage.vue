<script setup lang="ts">
// 角色列表：分页、新建、编辑与删除；权限按服务端 permissions 字段展示。
import { computed, h, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NButton } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import DataTable from '@/components/DataTable.vue'
import ErrorBlock from '@/components/ErrorBlock.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import StatusTag from '@/components/StatusTag.vue'
import RoleForm from './components/RoleForm.vue'
import { createRole, deleteRole, listRoles, updateRole } from '@/api/identity'
import type { Role, RoleCreatePayload, RoleUpdatePayload } from '@/api/identity'
import type { StatusTone } from '@/components/StatusTag.vue'
import { isAuthError } from '@/api/client'
import { markAuthError } from '@/stores/ui'
import { shortId, textOrDash } from '@/lib/format'

const route = useRoute()
const router = useRouter()

/** 当前页 */
const page = ref(1)
/** 每页条数 */
const pageSize = ref(50)
/** 角色数据 */
const rows = ref<Role[]>([])
/** 总数 */
const total = ref(0)
/** 加载态 */
const loading = ref(false)
/** 列表错误 */
const error = ref<unknown>(null)
/** 是否展示表单 */
const showForm = ref(false)
/** 编辑中的角色，null 表示新建 */
const editing = ref<Role | null>(null)
/** 提交中 */
const saving = ref(false)
/** 表单错误 */
const formError = ref<unknown>(null)
/** 删除中的角色 ID */
const deletingId = ref('')
/** 删除或操作错误，原文展示 */
const actionError = ref<unknown>(null)
/** 成功提示 */
const success = ref('')

// 读取路由 query 中的分页。
// @return 无
function readQuery(): void {
  const query = route.query
  const rawPage = Number(typeof query.page === 'string' ? query.page : '')
  page.value = Number.isFinite(rawPage) && rawPage >= 1 ? Math.floor(rawPage) : 1
  const rawSize = Number(typeof query.size === 'string' ? query.size : '')
  pageSize.value = Number.isFinite(rawSize) && rawSize >= 1 ? Math.min(1000, Math.floor(rawSize)) : 50
}

// 将分页写回路由 query。
// @return 无
async function syncQuery(): Promise<void> {
  const query: Record<string, string> = {}
  if (page.value > 1) query.page = String(page.value)
  if (pageSize.value !== 50) query.size = String(pageSize.value)
  await router.replace({ query })
}

// 加载角色列表；服务端不支持过滤与排序。
// @return 无
async function load(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    const result = await listRoles({ start: (page.value - 1) * pageSize.value, size: pageSize.value })
    rows.value = result.data ?? []
    total.value = result.total ?? 0
  } catch (cause) {
    error.value = cause
    if (isAuthError(cause)) markAuthError(cause)
  } finally {
    loading.value = false
  }
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
// @param role 目标角色
// @return 无
function openEdit(role: Role): void {
  editing.value = role
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

// 提交新建或编辑；编辑为全量替换。
// @param payload 表单载荷
// @return 无
async function save(payload: RoleCreatePayload | RoleUpdatePayload): Promise<void> {
  saving.value = true
  formError.value = null
  success.value = ''
  try {
    if (editing.value) {
      await updateRole(editing.value.id, payload as RoleUpdatePayload)
      success.value = `已更新角色 ${editing.value.id}`
    } else {
      const created = await createRole(payload as RoleCreatePayload)
      success.value = `已创建角色 ${created.id}`
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

// 删除角色；内置或受保护角色的错误原文展示。
// @param role 目标角色
// @return 无
async function remove(role: Role): Promise<void> {
  deletingId.value = role.id
  actionError.value = null
  success.value = ''
  try {
    await deleteRole(role.id)
    success.value = `已删除角色 ${role.id}`
    await load()
  } catch (cause) {
    actionError.value = cause
    markAuthError(cause)
  } finally {
    deletingId.value = ''
  }
}

// 将权限映射到低饱和标签色调。
// @param permission 权限名
// @return 标签色调
function permissionTone(permission: string): StatusTone {
  if (permission === 'admin') return 'success'
  if (permission === 'write') return 'warning'
  if (permission === 'read') return 'info'
  return 'default'
}

/** 表格列 */
const columns = computed<DataTableColumns<Role>>(() => [
  {
    title: '角色 ID',
    key: 'id',
    width: 220,
    render: (row) => h('span', { class: 'mono', title: row.id }, shortId(row.id, 24, 8)),
  },
  { title: '名称', key: 'name', render: (row) => textOrDash(row.name) },
  {
    title: '权限',
    key: 'permissions',
    render: (row) => {
      const permissions = row.permissions ?? []
      if (!permissions.length) return h('span', { class: 'muted' }, '—')
      return h(
        'div',
        { class: 'row' },
        permissions.map((permission) => h(StatusTag, { value: permission, tone: permissionTone(permission) })),
      )
    },
  },
  {
    title: '操作',
    key: 'actions',
    width: 160,
    render: (row) =>
      h('div', { class: 'row' }, [
        h(NButton, { size: 'tiny', quaternary: true, onClick: () => openEdit(row) }, { default: () => '编辑' }),
        h(ConfirmButton, {
          label: '删除',
          title: '删除该角色？',
          description: `角色 ${row.id} 将从角色列表移除；内置或受保护角色会被服务端拒绝。`,
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
    <PageHeader title="角色" description="维护角色与 read / write / admin 权限；删除失败时原样展示服务端错误。">
      <template #actions>
        <n-button size="small" type="primary" @click="openCreate">新建角色</n-button>
      </template>
    </PageHeader>

    <RoleForm
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
    <ErrorBlock v-if="actionError" :error="actionError" context="DELETE /service/identity/roles/{id}" @retry="load" />
    <ErrorBlock v-if="error" :error="error" context="GET /service/identity/roles" @retry="load" />

    <DataTable
      :columns="columns"
      :data="rows"
      :loading="loading"
      :total="total"
      :page="page"
      :page-size="pageSize"
      row-key="id"
      empty-description="暂无角色，点击右上角新建角色。"
      @update:page="changePage"
      @update:page-size="changePageSize"
    />
  </div>
</template>
