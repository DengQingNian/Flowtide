<script setup lang="ts">
// 任务身份链接面板：维护候选用户与候选组。
import { computed, onMounted, ref } from 'vue'
import { NAlert, NButton, NInput, NSelect, NSpin } from 'naive-ui'
import { errorMessage } from '@/api/client'
import { addTaskCandidate, deleteTaskIdentityLink, listTaskIdentityLinks } from '@/api/task'
import { markAuthError } from '@/stores/ui'
import type { IdentityLink } from '@/api/types'
import { textOrDash } from '@/lib/format'
import ConfirmButton from '@/components/ConfirmButton.vue'
import EmptyState from '@/components/EmptyState.vue'
import ErrorBlock from '@/components/ErrorBlock.vue'

const props = defineProps<{
  /** 任务 ID */
  taskId: string
}>()

const links = ref<IdentityLink[]>([])
const loading = ref(false)
const error = ref<unknown>(null)
const feedback = ref('')
const kind = ref<'user' | 'group'>('user')
const identityId = ref('')
const saving = ref(false)

const kindOptions = [
  { label: '候选用户', value: 'user' },
  { label: '候选组', value: 'group' },
]

/** 候选人列表 */
const users = computed(() => links.value.filter((link) => Boolean(link.userId)))
/** 候选组列表 */
const groups = computed(() => links.value.filter((link) => Boolean(link.groupId)))

// 加载身份链接。
// @return 无
async function load(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    links.value = await listTaskIdentityLinks(props.taskId)
  } catch (err) {
    markAuthError(err)
    error.value = err
    links.value = []
  } finally {
    loading.value = false
  }
}

// 添加候选用户或候选组。
// @return 无
async function add(): Promise<void> {
  feedback.value = ''
  const target = identityId.value.trim()
  if (!target) {
    feedback.value = kind.value === 'user' ? '请输入用户 ID' : '请输入组 ID'
    return
  }
  saving.value = true
  try {
    await addTaskCandidate(props.taskId, kind.value === 'user' ? { userId: target } : { groupId: target })
    feedback.value = kind.value === 'user' ? '候选人已添加' : '候选组已添加'
    identityId.value = ''
    await load()
  } catch (err) {
    markAuthError(err)
    feedback.value = `添加失败：${errorMessage(err)}`
  } finally {
    saving.value = false
  }
}

// 移除候选用户或候选组。
// @param link 身份链接
// @return 无
async function remove(link: IdentityLink): Promise<void> {
  feedback.value = ''
  const family = link.groupId ? 'groups' : 'users'
  const target = link.groupId || link.userId || ''
  try {
    await deleteTaskIdentityLink(props.taskId, family, target)
    feedback.value = '身份链接已移除'
    await load()
  } catch (err) {
    markAuthError(err)
    feedback.value = `移除失败：${errorMessage(err)}`
  }
}

onMounted(load)
</script>

<template>
  <div>
    <div class="row row--between" style="margin-bottom: 10px">
      <span class="section-title" style="margin: 0">候选人 / 候选组</span>
    </div>
    <n-alert v-if="feedback" :type="feedback.includes('失败') || feedback.includes('请输入') ? 'error' : 'success'" :bordered="false" style="margin-bottom: 10px">
      {{ feedback }}
    </n-alert>
    <div class="row" style="align-items: flex-end; margin-bottom: 12px">
      <div class="filter-bar__item" style="max-width: 150px">
        <span class="filter-bar__label">类型</span>
        <n-select v-model:value="kind" :options="kindOptions" />
      </div>
      <div class="filter-bar__item">
        <span class="filter-bar__label">标识</span>
        <n-input v-model:value="identityId" :placeholder="kind === 'user' ? '用户 ID' : '组 ID'" @keyup.enter="add" />
      </div>
      <n-button size="small" type="primary" :loading="saving" @click="add">添加</n-button>
    </div>
    <error-block v-if="error" :error="error" :context="`GET /service/runtime/tasks/${taskId}/identitylinks`" @retry="load" />
    <n-spin :show="loading">
      <empty-state v-if="!loading && links.length === 0" title="暂无候选人" description="添加候选用户或候选组后，未签收任务将出现在对应人员的待办中。" />
      <template v-else>
        <div v-if="users.length > 0" style="margin-bottom: 14px">
          <div class="section-title">候选用户</div>
          <table class="kv-table">
            <thead><tr><th>用户 ID</th><th>链接类型</th><th style="width: 90px">操作</th></tr></thead>
            <tbody>
              <tr v-for="link in users" :key="link.id || link.userId">
                <td class="mono">{{ textOrDash(link.userId) }}</td>
                <td>{{ textOrDash(link.type) }}</td>
                <td><confirm-button label="移除" title="确认移除该候选人？" description="移除后该用户不再出现在任务候选人中。" :danger="false" @confirm="remove(link)" /></td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-if="groups.length > 0">
          <div class="section-title">候选组</div>
          <table class="kv-table">
            <thead><tr><th>组 ID</th><th>链接类型</th><th style="width: 90px">操作</th></tr></thead>
            <tbody>
              <tr v-for="link in groups" :key="link.id || link.groupId">
                <td class="mono">{{ textOrDash(link.groupId) }}</td>
                <td>{{ textOrDash(link.type) }}</td>
                <td><confirm-button label="移除" title="确认移除该候选组？" description="移除后该组不再出现在任务候选中。" :danger="false" @confirm="remove(link)" /></td>
              </tr>
            </tbody>
          </table>
        </div>
      </template>
    </n-spin>
  </div>
</template>
