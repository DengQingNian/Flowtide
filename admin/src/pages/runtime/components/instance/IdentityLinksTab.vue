<script setup lang="ts">
// 实例身份链接：候选发起人/参与者的用户与组维护。
import { onMounted, ref } from 'vue'
import { NButton, NInput, NRadio, NRadioGroup, NSelect, NSpin } from 'naive-ui'
import type { IdentityLink } from '@/api/types'
import { addInstanceIdentityLink, deleteInstanceIdentityLink, listInstanceIdentityLinks } from '@/api/runtime'
import { errorMessage } from '@/api/client'
import { formatDateTime, textOrDash } from '@/lib/format'
import { markAuthError } from '@/stores/ui'
import ErrorBlock from '@/components/ErrorBlock.vue'
import EmptyState from '@/components/EmptyState.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'

const props = defineProps<{
  /** 实例 ID */
  instanceId: string
}>()

const links = ref<IdentityLink[]>([])
const loading = ref(false)
const error = ref<unknown>(null)
const actionError = ref('')
const notice = ref('')
const submitting = ref(false)
const deletingKey = ref('')

const kind = ref<'user' | 'group'>('user')
const identityId = ref('')
const type = ref('candidate')

const typeOptions = [
  { label: 'candidate（候选人）', value: 'candidate' },
  { label: 'starter（发起人）', value: 'starter' },
  { label: 'participant（参与者）', value: 'participant' },
]

// 加载身份链接。
// @return 无
async function load(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    links.value = await listInstanceIdentityLinks(props.instanceId)
  } catch (cause) {
    error.value = cause
    markAuthError(cause)
  } finally {
    loading.value = false
  }
}

// 新增身份链接。
// @return 无
async function submit(): Promise<void> {
  actionError.value = ''
  notice.value = ''
  const id = identityId.value.trim()
  if (!id) {
    actionError.value = kind.value === 'user' ? '用户 ID 不能为空' : '组 ID 不能为空'
    return
  }
  submitting.value = true
  try {
    await addInstanceIdentityLink(props.instanceId, {
      type: type.value,
      userId: kind.value === 'user' ? id : undefined,
      groupId: kind.value === 'group' ? id : undefined,
    })
    identityId.value = ''
    notice.value = '身份链接已添加'
    await load()
  } catch (cause) {
    actionError.value = errorMessage(cause)
    markAuthError(cause)
  } finally {
    submitting.value = false
  }
}

// 删除身份链接。
// @param link 身份链接
// @return 无
async function remove(link: IdentityLink): Promise<void> {
  deletingKey.value = link.id || `${link.type}:${link.userId ?? link.groupId}`
  actionError.value = ''
  notice.value = ''
  try {
    await deleteInstanceIdentityLink(props.instanceId, link)
    notice.value = '身份链接已删除'
    await load()
  } catch (cause) {
    actionError.value = errorMessage(cause)
    markAuthError(cause)
  } finally {
    deletingKey.value = ''
  }
}

onMounted(load)
</script>

<template>
  <div class="stack">
    <div class="panel">
      <div class="panel__header"><span class="panel__title">添加身份链接</span></div>
      <div class="panel__body">
        <div class="row">
          <n-radio-group v-model:value="kind" size="small">
            <n-radio value="user">用户</n-radio>
            <n-radio value="group">组</n-radio>
          </n-radio-group>
          <n-input
            v-model:value="identityId"
            size="small"
            :placeholder="kind === 'user' ? '用户 ID' : '组 ID'"
            style="max-width: 200px"
            @keyup.enter="submit"
          />
          <n-select v-model:value="type" size="small" :options="typeOptions" style="width: 200px" />
          <n-button size="small" type="primary" :loading="submitting" @click="submit">添加</n-button>
        </div>
        <div class="row" style="margin-top: 8px">
          <span v-if="notice" class="text-success">{{ notice }}</span>
          <span v-if="actionError" class="text-danger" role="alert">{{ actionError }}</span>
        </div>
      </div>
    </div>

    <div class="panel">
      <div class="panel__header">
        <span class="panel__title">身份链接列表</span>
        <span class="muted" style="margin-left: auto; font-size: 12px">共 {{ links.length }} 条</span>
      </div>
      <div class="panel__body">
        <ErrorBlock v-if="error" :error="error" @retry="load" />
        <n-spin v-else :show="loading">
          <EmptyState v-if="!loading && links.length === 0" title="暂无身份链接" description="可添加候选发起人或参与者。" />
          <table v-else class="kv-table">
            <thead>
              <tr>
                <th>类型</th>
                <th>用户 ID</th>
                <th>组 ID</th>
                <th>创建时间</th>
                <th style="width: 90px">操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="link in links" :key="link.id || `${link.type}:${link.userId ?? link.groupId}`">
                <td>{{ textOrDash(link.type) }}</td>
                <td class="mono">{{ textOrDash(link.userId) }}</td>
                <td class="mono">{{ textOrDash(link.groupId) }}</td>
                <td>{{ formatDateTime(link.createTime) }}</td>
                <td>
                  <ConfirmButton
                    label="删除"
                    title="确认删除该身份链接？"
                    description="删除后对应主体不再参与该实例的权限判断。"
                    :loading="deletingKey === (link.id || `${link.type}:${link.userId ?? link.groupId}`)"
                    @confirm="remove(link)"
                  />
                </td>
              </tr>
            </tbody>
          </table>
        </n-spin>
      </div>
    </div>
  </div>
</template>
