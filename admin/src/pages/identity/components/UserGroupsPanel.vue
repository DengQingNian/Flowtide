<script setup lang="ts">
// 用户所属组：加载现有组关系，支持加入组与移出组。
import { computed, onMounted, ref } from 'vue'
import { NButton, NSelect } from 'naive-ui'
import ErrorBlock from '@/components/ErrorBlock.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import EmptyState from '@/components/EmptyState.vue'
import { addUserGroup, listGroups, listUserGroups, removeUserGroup } from '@/api/identity'
import type { Group } from '@/api/identity'
import { isAuthError } from '@/api/client'
import { markAuthError } from '@/stores/ui'
import { shortId, textOrDash } from '@/lib/format'

const props = defineProps<{
  /** 用户 ID */
  userId: string
}>()

/** 用户当前所属组 */
const groups = ref<Group[]>([])
/** 全部组，用于加入选择 */
const allGroups = ref<Group[]>([])
/** 加载态 */
const loading = ref(false)
/** 加载错误 */
const loadError = ref<unknown>(null)
/** 操作错误 */
const actionError = ref<unknown>(null)
/** 成功提示 */
const success = ref('')
/** 选择的组 ID */
const selectedGroup = ref<string | null>(null)
/** 加入中 */
const adding = ref(false)
/** 正在移除的组 ID */
const removingId = ref('')

/** 可加入的组选项：排除已在的组 */
const options = computed(() =>
  allGroups.value
    .filter((group) => !groups.value.some((item) => item.id === group.id))
    .map((group) => ({ label: group.name ? `${group.name}（${group.id}）` : group.id, value: group.id })),
)

// 加载用户组关系与全部组。
// @return 无
async function load(): Promise<void> {
  loading.value = true
  loadError.value = null
  try {
    const [current, page] = await Promise.all([listUserGroups(props.userId), listGroups({ size: 1000 })])
    groups.value = current ?? []
    allGroups.value = page.data ?? []
  } catch (cause) {
    loadError.value = cause
    if (isAuthError(cause)) markAuthError(cause)
  } finally {
    loading.value = false
  }
}

// 将用户加入所选组。
// @return 无
async function add(): Promise<void> {
  if (!selectedGroup.value) return
  adding.value = true
  actionError.value = null
  success.value = ''
  try {
    await addUserGroup(props.userId, selectedGroup.value)
    success.value = `已加入组 ${selectedGroup.value}`
    selectedGroup.value = null
    await load()
  } catch (cause) {
    actionError.value = cause
    markAuthError(cause)
  } finally {
    adding.value = false
  }
}

// 将用户移出指定组。
// @param group 目标组
// @return 无
async function remove(group: Group): Promise<void> {
  removingId.value = group.id
  actionError.value = null
  success.value = ''
  try {
    await removeUserGroup(props.userId, group.id)
    success.value = `已移出组 ${group.id}`
    await load()
  } catch (cause) {
    actionError.value = cause
    markAuthError(cause)
  } finally {
    removingId.value = ''
  }
}

onMounted(() => void load())
</script>

<template>
  <section class="panel">
    <div class="panel__header">
      <span class="panel__title">所属组</span>
      <span class="muted" style="font-size: 12px">GET/POST/DELETE /identity/users/{{ userId }}/groups</span>
    </div>
    <div class="panel__body">
      <ErrorBlock v-if="loadError" :error="loadError" context="GET /service/identity/users/{id}/groups" @retry="load" />
      <ErrorBlock v-if="actionError" :error="actionError" @retry="load" />
      <div v-if="success" class="banner" role="status" style="margin: 0 0 12px">{{ success }}</div>
      <div class="row" style="margin-bottom: 12px">
        <n-select
          v-model:value="selectedGroup"
          :options="options"
          filterable
          placeholder="选择要加入的组"
          style="max-width: 320px"
        />
        <n-button size="small" type="primary" :loading="adding" :disabled="!selectedGroup" @click="add">加入组</n-button>
        <span v-if="!options.length" class="muted" style="font-size: 12px">没有可加入的组。</span>
      </div>
      <div v-if="loading" class="muted">正在加载组…</div>
      <table v-else-if="groups.length" class="kv-table">
        <thead>
          <tr><th>组 ID</th><th>名称</th><th>类型</th><th style="width: 100px">操作</th></tr>
        </thead>
        <tbody>
          <tr v-for="group in groups" :key="group.id">
            <td class="mono">{{ textOrDash(shortId(group.id, 24, 8)) }}</td>
            <td>{{ textOrDash(group.name) }}</td>
            <td>{{ textOrDash(group.type) }}</td>
            <td>
              <ConfirmButton
                label="移出"
                title="将用户移出该组？"
                :description="`用户 ${userId} 将失去组 ${group.id} 的成员身份，组本身不会被删除。`"
                :loading="removingId === group.id"
                @confirm="remove(group)"
              />
            </td>
          </tr>
        </tbody>
      </table>
      <EmptyState v-else title="尚未加入任何组" description="从上方选择一个组并点击“加入组”。" />
    </div>
  </section>
</template>
