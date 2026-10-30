<script setup lang="ts">
// 用户信息键值：列表接口只给键，逐键取回值后展示，支持新增、修改与删除。
import { onMounted, ref } from 'vue'
import { NButton, NInput } from 'naive-ui'
import ErrorBlock from '@/components/ErrorBlock.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import EmptyState from '@/components/EmptyState.vue'
import { createUserInfo, deleteUserInfo, loadUserInfo, updateUserInfo } from '@/api/identity'
import type { UserInfo } from '@/api/identity'
import { isAuthError } from '@/api/client'
import { markAuthError } from '@/stores/ui'
import { textOrDash } from '@/lib/format'

const props = defineProps<{
  /** 用户 ID */
  userId: string
}>()

/** 信息键值 */
const entries = ref<UserInfo[]>([])
/** 加载态 */
const loading = ref(false)
/** 加载错误 */
const loadError = ref<unknown>(null)
/** 操作错误 */
const actionError = ref<unknown>(null)
/** 成功提示 */
const success = ref('')
/** 新增键 */
const newKey = ref('')
/** 新增值 */
const newValue = ref('')
/** 新增中 */
const creating = ref(false)
/** 正在编辑的键 */
const editingKey = ref('')
/** 编辑中的值 */
const editValue = ref('')
/** 保存中 */
const saving = ref(false)
/** 正在删除的键 */
const deletingKey = ref('')
/** 本地校验提示 */
const validation = ref('')

// 加载全部信息键值。
// @return 无
async function load(): Promise<void> {
  loading.value = true
  loadError.value = null
  try {
    entries.value = await loadUserInfo(props.userId)
  } catch (cause) {
    loadError.value = cause
    if (isAuthError(cause)) markAuthError(cause)
  } finally {
    loading.value = false
  }
}

// 新增信息键值。
// @return 无
async function create(): Promise<void> {
  validation.value = ''
  const key = newKey.value.trim()
  // 键为空时服务端返回 400，这里先在前端提示。
  if (!key) {
    validation.value = '信息键不能为空。'
    return
  }
  creating.value = true
  actionError.value = null
  success.value = ''
  try {
    await createUserInfo(props.userId, key, newValue.value)
    success.value = `已新增信息 ${key}`
    newKey.value = ''
    newValue.value = ''
    await load()
  } catch (cause) {
    actionError.value = cause
    markAuthError(cause)
  } finally {
    creating.value = false
  }
}

// 进入某一项的编辑态。
// @param entry 信息项
// @return 无
function startEdit(entry: UserInfo): void {
  editingKey.value = entry.key
  editValue.value = entry.value
  actionError.value = null
  success.value = ''
}

// 保存编辑中的信息值。
// @return 无
async function saveEdit(): Promise<void> {
  if (!editingKey.value) return
  saving.value = true
  actionError.value = null
  success.value = ''
  try {
    await updateUserInfo(props.userId, editingKey.value, editValue.value)
    success.value = `已更新信息 ${editingKey.value}`
    editingKey.value = ''
    await load()
  } catch (cause) {
    actionError.value = cause
    markAuthError(cause)
  } finally {
    saving.value = false
  }
}

// 删除信息键。
// @param entry 信息项
// @return 无
async function remove(entry: UserInfo): Promise<void> {
  deletingKey.value = entry.key
  actionError.value = null
  success.value = ''
  try {
    await deleteUserInfo(props.userId, entry.key)
    success.value = `已删除信息 ${entry.key}`
    await load()
  } catch (cause) {
    actionError.value = cause
    markAuthError(cause)
  } finally {
    deletingKey.value = ''
  }
}

onMounted(() => void load())
</script>

<template>
  <section class="panel">
    <div class="panel__header">
      <span class="panel__title">用户信息</span>
      <span class="muted" style="font-size: 12px">GET/POST/DELETE /identity/users/{{ userId }}/info</span>
    </div>
    <div class="panel__body">
      <ErrorBlock v-if="loadError" :error="loadError" context="GET /service/identity/users/{id}/info" @retry="load" />
      <ErrorBlock v-if="actionError" :error="actionError" @retry="load" />
      <div v-if="success" class="banner" role="status" style="margin: 0 0 12px">{{ success }}</div>
      <div v-if="validation" class="banner banner--warning" role="alert" style="margin: 0 0 12px">{{ validation }}</div>
      <div class="row" style="margin-bottom: 12px; align-items: flex-end">
        <label class="info-field">
          <span class="identity-label">键</span>
          <n-input v-model:value="newKey" placeholder="如：department" style="width: 180px" />
        </label>
        <label class="info-field grow">
          <span class="identity-label">值</span>
          <n-input v-model:value="newValue" placeholder="值可以为空字符串" />
        </label>
        <n-button size="small" type="primary" :loading="creating" @click="create">新增</n-button>
      </div>
      <div v-if="loading" class="muted">正在加载用户信息…</div>
      <table v-else-if="entries.length" class="kv-table">
        <thead><tr><th style="width: 180px">键</th><th>值</th><th style="width: 160px">操作</th></tr></thead>
        <tbody>
          <tr v-for="entry in entries" :key="entry.key">
            <td class="mono">{{ textOrDash(entry.key) }}</td>
            <td>
              <n-input v-if="editingKey === entry.key" v-model:value="editValue" size="small" />
              <span v-else :title="entry.value">{{ textOrDash(entry.value) }}</span>
            </td>
            <td>
              <div class="row">
                <n-button v-if="editingKey === entry.key" size="tiny" type="primary" :loading="saving" @click="saveEdit">保存</n-button>
                <template v-else>
                  <n-button size="tiny" quaternary @click="startEdit(entry)">编辑</n-button>
                  <ConfirmButton
                    label="删除"
                    title="删除该用户信息？"
                    :description="`键 ${entry.key} 及其值将被永久删除，不影响用户本身。`"
                    :loading="deletingKey === entry.key"
                    @confirm="remove(entry)"
                  />
                </template>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
      <EmptyState v-else title="暂无用户信息" description="使用上方表单添加键值对。" />
    </div>
  </section>
</template>

<style scoped>
.info-field { display: flex; flex-direction: column; gap: 4px; }
.identity-label { font-size: 12px; color: var(--muted); }
</style>
