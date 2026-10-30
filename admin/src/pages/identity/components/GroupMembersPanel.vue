<script setup lang="ts">
// 组成员维护：列出成员、从用户中选择加入、移除成员。
import { computed, onMounted, ref, watch } from 'vue'
import { NButton, NSelect } from 'naive-ui'
import ErrorBlock from '@/components/ErrorBlock.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import EmptyState from '@/components/EmptyState.vue'
import { addMember, listMembers, listUsers, removeMember } from '@/api/identity'
import type { Group, User } from '@/api/identity'
import { isAuthError } from '@/api/client'
import { markAuthError } from '@/stores/ui'
import { textOrDash } from '@/lib/format'

const props = defineProps<{
  /** 目标组 */
  group: Group
}>()

const emit = defineEmits<{ close: [] }>()

/** 成员 */
const members = ref<User[]>([])
/** 全部用户，用于选择 */
const allUsers = ref<User[]>([])
/** 加载态 */
const loading = ref(false)
/** 加载错误 */
const loadError = ref<unknown>(null)
/** 操作错误 */
const actionError = ref<unknown>(null)
/** 成功提示 */
const success = ref('')
/** 选择的用户 ID */
const selectedUser = ref<string | null>(null)
/** 加入中 */
const adding = ref(false)
/** 正在移除的用户 ID */
const removingId = ref('')

/** 可加入的用户选项 */
const options = computed(() =>
  allUsers.value
    .filter((user) => !members.value.some((item) => item.id === user.id))
    .map((user) => ({ label: user.displayName ? `${user.displayName}（${user.id}）` : user.id, value: user.id })),
)

// 加载成员与全部用户。
// @return 无
async function load(): Promise<void> {
  loading.value = true
  loadError.value = null
  try {
    const [current, page] = await Promise.all([listMembers(props.group.id), listUsers({ size: 1000 })])
    members.value = current ?? []
    allUsers.value = page.data ?? []
  } catch (cause) {
    loadError.value = cause
    if (isAuthError(cause)) markAuthError(cause)
  } finally {
    loading.value = false
  }
}

// 添加所选用户为成员。
// @return 无
async function add(): Promise<void> {
  if (!selectedUser.value) return
  adding.value = true
  actionError.value = null
  success.value = ''
  try {
    await addMember(props.group.id, selectedUser.value)
    success.value = `已添加成员 ${selectedUser.value}`
    selectedUser.value = null
    await load()
  } catch (cause) {
    actionError.value = cause
    markAuthError(cause)
  } finally {
    adding.value = false
  }
}

// 移除成员。
// @param user 成员
// @return 无
async function remove(user: User): Promise<void> {
  removingId.value = user.id
  actionError.value = null
  success.value = ''
  try {
    await removeMember(props.group.id, user.id)
    success.value = `已移除成员 ${user.id}`
    await load()
  } catch (cause) {
    actionError.value = cause
    markAuthError(cause)
  } finally {
    removingId.value = ''
  }
}

// 成员展示名。
// @param user 用户
// @return 显示名或 ID
function memberName(user: User): string {
  return textOrDash(user.displayName || user.id)
}

watch(() => props.group.id, () => void load())
onMounted(() => void load())
</script>

<template>
  <section class="panel">
    <div class="panel__header">
      <span class="panel__title">组成员：{{ textOrDash(group.name || group.id) }}</span>
      <span class="muted" style="font-size: 12px">GET/POST/DELETE /identity/groups/{{ group.id }}/members</span>
      <n-button size="tiny" quaternary style="margin-left: auto" @click="emit('close')">关闭</n-button>
    </div>
    <div class="panel__body">
      <ErrorBlock v-if="loadError" :error="loadError" context="GET /service/identity/groups/{id}/members" @retry="load" />
      <ErrorBlock v-if="actionError" :error="actionError" @retry="load" />
      <div v-if="success" class="banner" role="status" style="margin: 0 0 12px">{{ success }}</div>
      <div class="row" style="margin-bottom: 12px">
        <n-select
          v-model:value="selectedUser"
          :options="options"
          filterable
          placeholder="选择要加入的用户"
          style="max-width: 320px"
        />
        <n-button size="small" type="primary" :loading="adding" :disabled="!selectedUser" @click="add">添加成员</n-button>
      </div>
      <div v-if="loading" class="muted">正在加载成员…</div>
      <table v-else-if="members.length" class="kv-table">
        <thead><tr><th>用户 ID</th><th>显示名</th><th>邮箱</th><th style="width: 100px">操作</th></tr></thead>
        <tbody>
          <tr v-for="member in members" :key="member.id">
            <td class="mono">{{ textOrDash(member.id) }}</td>
            <td>{{ memberName(member) }}</td>
            <td>{{ textOrDash(member.email) }}</td>
            <td>
              <ConfirmButton
                label="移除"
                title="将该用户移出组？"
                :description="`用户 ${member.id} 将失去组 ${group.id} 的成员身份，用户本身不会被删除。`"
                :loading="removingId === member.id"
                @confirm="remove(member)"
              />
            </td>
          </tr>
        </tbody>
      </table>
      <EmptyState v-else title="组内暂无成员" description="从上方选择一个用户并点击“添加成员”。" />
    </div>
  </section>
</template>
