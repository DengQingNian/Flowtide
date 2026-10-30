<script setup lang="ts">
// 流程定义身份链接：展示候选发起人，支持添加用户/组与移除。
import { computed, onMounted, ref } from 'vue'
import { NButton, NInput, NSelect } from 'naive-ui'
import ErrorBlock from '@/components/ErrorBlock.vue'
import EmptyState from '@/components/EmptyState.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import {
  addDefinitionIdentityLink,
  deleteDefinitionIdentityLink,
  listDefinitionIdentityLinks,
} from '@/api/repository'
import type { IdentityLink } from '@/api/types'
import { textOrDash } from '@/lib/format'
import { markAuthError } from '@/stores/ui'

const props = defineProps<{
  /** 流程定义 ID */
  definitionId: string
}>()

const links = ref<IdentityLink[]>([])
const loading = ref(false)
const error = ref<unknown>(null)
const actionError = ref<unknown>(null)
const submitting = ref(false)
const identityType = ref<'user' | 'group'>('user')
const identityId = ref('')
const retryAction = ref<() => void>(() => {})

const typeOptions = [
  { label: '用户', value: 'user' },
  { label: '组', value: 'group' },
]

/** 已用用户与组的数量摘要 */
const summary = computed(() => `用户 ${links.value.filter((item) => item.userId).length} 个，组 ${links.value.filter((item) => item.groupId).length} 个`)

// 读取身份链接列表。
// @return 无
async function load(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    links.value = await listDefinitionIdentityLinks(props.definitionId)
  } catch (cause) {
    markAuthError(cause)
    error.value = cause
  } finally {
    loading.value = false
  }
}

// 记录操作错误。
// @param cause 错误
// @param retry 重试函数
// @return 无
function fail(cause: unknown, retry: () => void): void {
  markAuthError(cause)
  actionError.value = cause
  retryAction.value = retry
}

// 添加候选发起人。
// @return 无
async function add(): Promise<void> {
  const value = identityId.value.trim()
  if (!value) return
  submitting.value = true
  actionError.value = null
  try {
    await addDefinitionIdentityLink(props.definitionId, identityType.value === 'user' ? value : undefined, identityType.value === 'group' ? value : undefined)
    identityId.value = ''
    await load()
  } catch (cause) {
    fail(cause, () => void add())
  } finally {
    submitting.value = false
  }
}

// 移除身份链接。
// @param link 身份链接
// @return 无
async function remove(link: IdentityLink): Promise<void> {
  actionError.value = null
  try {
    if (link.userId) await deleteDefinitionIdentityLink(props.definitionId, 'users', link.userId)
    else if (link.groupId) await deleteDefinitionIdentityLink(props.definitionId, 'groups', link.groupId)
    else return
    await load()
  } catch (cause) {
    fail(cause, () => void remove(link))
  }
}

onMounted(() => void load())
</script>

<template>
  <div>
    <ErrorBlock v-if="error" :error="error" context="GET /service/repository/process-definitions/{id}/identitylinks" @retry="load" />
    <ErrorBlock v-if="actionError" :error="actionError" context="流程定义身份链接操作" @retry="retryAction" />
    <div class="row" style="margin-bottom: 10px">
      <n-select v-model:value="identityType" size="small" :options="typeOptions" style="width: 100px" />
      <n-input v-model:value="identityId" size="small" placeholder="用户 ID 或组 ID" style="max-width: 240px" @keyup.enter="add" />
      <n-button size="small" type="primary" :loading="submitting" :disabled="!identityId.trim()" @click="add">添加</n-button>
      <span class="muted" style="font-size: 12px">{{ summary }}</span>
    </div>
    <div v-if="loading" class="muted">加载中…</div>
    <EmptyState v-else-if="links.length === 0" title="没有候选发起人" description="未设置候选发起人时，所有用户都可以启动该流程。" />
    <div v-else>
      <div v-for="link in links" :key="link.id ?? `${link.userId ?? link.groupId}`" class="row row--between" style="padding: 6px 0; border-bottom: 1px solid var(--border)">
        <div class="row">
          <span class="status-tag status-tag--default">{{ link.userId ? '候选用户' : '候选组' }}</span>
          <span class="mono">{{ textOrDash(link.userId ?? link.groupId) }}</span>
          <span v-if="link.type" class="muted" style="font-size: 12px">{{ link.type }}</span>
        </div>
        <ConfirmButton
          label="移除"
          title="移除该候选发起人？"
          description="移除后，对应用户或组将不能再启动该流程定义。"
          @confirm="remove(link)"
        />
      </div>
    </div>
  </div>
</template>
