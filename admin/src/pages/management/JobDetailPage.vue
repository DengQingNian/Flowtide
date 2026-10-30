<script setup lang="ts">
// 作业详情：字段、异常栈与重试/恢复/重新调度/删除动作。
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter, RouterLink } from 'vue-router'
import { NButton, NSpin } from 'naive-ui'
import { AlertTriangle, ArrowLeft } from '@lucide/vue'
import PageHeader from '@/components/PageHeader.vue'
import ErrorBlock from '@/components/ErrorBlock.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import CopyText from '@/components/CopyText.vue'
import StatusTag from '@/components/StatusTag.vue'
import { ApiError, errorMessage } from '@/api/client'
import { markAuthError } from '@/stores/ui'
import { applyJobAction, deleteJob, getJob, getJobStacktrace, getProcessInstanceBrief, isJobBucket, normalizeJobBucket } from '@/api/management'
import type { JobBucket, JobInfo } from '@/api/management'
import { JOB_BUCKET_LABELS } from './components/managementUtils'
import { formatDateTime, shortId, textOrDash, toDate } from '@/lib/format'

/** 流程实例摘要 */
interface InstanceBrief {
  id: string
  processDefinitionId?: string
  processDefinitionKey?: string
  businessKey?: string
  name?: string
}

const route = useRoute()
const router = useRouter()

/** 当前桶 */
const bucket = computed<JobBucket>(() => normalizeJobBucket(route.params.bucket))
/** 作业 ID */
const jobId = computed(() => String(route.params.id ?? ''))
/** 作业 */
const job = ref<JobInfo | null>(null)
/** 加载态 */
const loading = ref(false)
/** 页面错误 */
const error = ref<unknown>(null)
/** 异常栈 */
const stack = ref('')
/** 异常栈加载错误 */
const stackError = ref<unknown>(null)
/** 无异常栈 */
const stackMissing = ref(false)
/** 关联实例 */
const instance = ref<InstanceBrief | null>(null)
/** 关联实例不存在 */
const instanceMissing = ref(false)
/** 重试次数输入 */
const retries = ref(1)
/** 新到期时间输入 */
const dueDate = ref('')
/** 当前动作 */
const actionLoading = ref('')
/** 动作结果 */
const actionMessage = ref('')
/** 动作错误 */
const actionError = ref<unknown>(null)

/** 是否有动作执行中 */
const busy = computed(() => actionLoading.value !== '')

// 格式化到期时间，忽略零值时间。
// @param value 时间文本
// @return 展示文本
function dueText(value: string | null | undefined): string {
  const date = toDate(value)
  if (!date || date.getUTCFullYear() <= 1) return '—'
  return formatDateTime(value)
}

// 加载异常栈，404 视为没有异常。
// @return 无
async function loadStack(): Promise<void> {
  stackError.value = null
  stackMissing.value = false
  stack.value = ''
  try {
    stack.value = await getJobStacktrace(bucket.value, jobId.value)
  } catch (cause) {
    if (cause instanceof ApiError && cause.status === 404) {
      stackMissing.value = true
      return
    }
    stackError.value = cause
    markAuthError(cause)
  }
}

// 加载关联流程实例摘要，失败只提示不阻断页面。
// @return 无
async function loadInstance(): Promise<void> {
  instance.value = null
  instanceMissing.value = false
  const instanceId = job.value?.processInstanceId
  if (!instanceId) return
  try {
    instance.value = await getProcessInstanceBrief(instanceId)
  } catch {
    instanceMissing.value = true
  }
}

// 加载作业详情、异常栈与关联实例。
// @return 无
async function load(): Promise<void> {
  loading.value = true
  error.value = null
  actionError.value = null
  try {
    job.value = await getJob(bucket.value, jobId.value)
    retries.value = job.value.retries > 0 ? job.value.retries : 1
    dueDate.value = ''
    await Promise.all([loadStack(), loadInstance()])
  } catch (cause) {
    error.value = cause
    markAuthError(cause)
  } finally {
    loading.value = false
  }
}

// 校验重试次数。
// @return 合法次数，非法时返回 null
function validRetries(): number | null {
  const value = Number(retries.value)
  if (!Number.isInteger(value) || value < 0) return null
  return value
}

// 设置可执行作业的重试次数。
// @return 无
async function setRetries(): Promise<void> {
  const value = validRetries()
  if (value === null) {
    actionError.value = new Error('重试次数必须是不小于 0 的整数')
    return
  }
  await runAction('setRetries', () => applyJobAction(bucket.value, jobId.value, { action: 'setRetries', retries: value }), '重试次数已更新。')
}

// 把定时器或死信作业移回可执行桶。
// @return 无
async function moveJob(): Promise<void> {
  const retriesValue = validRetries()
  if (bucket.value === 'deadletter' && retriesValue === null) {
    actionError.value = new Error('重试次数必须是不小于 0 的整数')
    return
  }
  await runAction(
    'move',
    () =>
      applyJobAction(bucket.value, jobId.value, {
        action: 'move',
        retries: bucket.value === 'deadletter' ? (retriesValue ?? 1) : undefined,
      }),
    '作业已移回可执行桶。',
  )
  if (!actionError.value) await backToExecutable()
}

// 修改定时器作业的到期时间。
// @return 无
async function reschedule(): Promise<void> {
  if (!dueDate.value) {
    actionError.value = new Error('请选择新的到期时间')
    return
  }
  const parsed = new Date(dueDate.value)
  if (Number.isNaN(parsed.getTime())) {
    actionError.value = new Error('到期时间格式非法')
    return
  }
  await runAction('reschedule', () => applyJobAction(bucket.value, jobId.value, { action: 'reschedule', dueDate: parsed.toISOString() }), '到期时间已更新。')
  if (!actionError.value) await load()
}

// 删除作业。
// @return 无
async function removeJob(): Promise<void> {
  actionLoading.value = 'delete'
  actionError.value = null
  try {
    await deleteJob(bucket.value, jobId.value)
    await router.push({ name: 'jobs', params: { bucket: bucket.value } })
  } catch (cause) {
    actionError.value = cause
    markAuthError(cause)
  } finally {
    actionLoading.value = ''
  }
}

// 执行动作并维护加载态与提示。
// @param key 动作标识
// @param task 实际请求
// @param message 成功提示
// @return 无
async function runAction(key: string, task: () => Promise<void>, message: string): Promise<void> {
  actionLoading.value = key
  actionError.value = null
  actionMessage.value = ''
  try {
    await task()
    actionMessage.value = message
  } catch (cause) {
    actionError.value = cause
    markAuthError(cause)
  } finally {
    actionLoading.value = ''
  }
}

// 返回可执行作业列表。
// @return 无
async function backToExecutable(): Promise<void> {
  await router.push({ name: 'jobs', params: { bucket: 'executable' } })
}

// 监听路由参数并纠正非法桶。
watch(
  () => [route.params.bucket, route.params.id],
  () => {
    if (!isJobBucket(route.params.bucket)) {
      void router.replace({ name: 'jobs', params: { bucket: 'executable' } })
      return
    }
    if (!jobId.value) return
    void load()
  },
  { immediate: true },
)
</script>

<template>
  <div>
    <PageHeader :title="`作业详情 ${jobId ? shortId(jobId) : ''}`" description="查看作业字段与异常栈，并按桶执行恢复操作。">
      <template #actions>
        <router-link :to="`/management/jobs/${bucket}`" custom v-slot="{ navigate }">
          <n-button size="small" @click="navigate">
            <template #icon><ArrowLeft :size="14" /></template>
            返回列表
          </n-button>
        </router-link>
      </template>
    </PageHeader>

    <ErrorBlock v-if="error" :error="error" :context="`GET /service/management/${bucket === 'executable' ? 'jobs' : bucket + '-jobs'}/${jobId}`" @retry="load" />

    <n-spin :show="loading">
      <div v-if="job" class="stack">
        <section class="panel">
          <div class="panel__header">
            <span class="panel__title">作业信息</span>
            <StatusTag :value="JOB_BUCKET_LABELS[bucket]" :tone="bucket === 'deadletter' ? 'danger' : bucket === 'timer' ? 'warning' : bucket === 'suspended' ? 'info' : 'success'" />
            <span class="muted" style="margin-left: auto; font-size: 12px">{{ job.executable ? '已进入可执行桶' : '未进入可执行桶' }}</span>
          </div>
          <div class="panel__body">
            <div class="detail-grid">
              <div class="detail-item">
                <span class="detail-item__label">作业 ID</span>
                <span class="detail-item__value"><CopyText :value="job.id" /></span>
              </div>
              <div class="detail-item"><span class="detail-item__label">类型</span><span class="detail-item__value">{{ textOrDash(job.type) }}</span></div>
              <div class="detail-item"><span class="detail-item__label">重试次数</span><span class="detail-item__value">{{ job.retries ?? 0 }}</span></div>
              <div class="detail-item"><span class="detail-item__label">到期时间</span><span class="detail-item__value">{{ dueText(job.dueDate) }}</span></div>
              <div class="detail-item">
                <span class="detail-item__label">执行</span>
                <span class="detail-item__value">
                  <CopyText v-if="job.executionId" :value="job.executionId" />
                  <span v-else>—</span>
                </span>
              </div>
              <div class="detail-item"><span class="detail-item__label">活动 ID</span><span class="detail-item__value">{{ textOrDash(job.activityId) }}</span></div>
              <div class="detail-item">
                <span class="detail-item__label">流程实例</span>
                <span class="detail-item__value">
                  <router-link v-if="job.processInstanceId" :to="`/runtime/instances/${job.processInstanceId}`" class="mono">
                    {{ shortId(job.processInstanceId) }}
                  </router-link>
                  <span v-else>—</span>
                </span>
              </div>
              <div class="detail-item">
                <span class="detail-item__label">流程定义</span>
                <span class="detail-item__value">
                  <router-link v-if="instance?.processDefinitionId" :to="`/repository/definitions/${instance.processDefinitionId}`" class="mono">
                    {{ shortId(instance.processDefinitionId) }}
                  </router-link>
                  <span v-else-if="instanceMissing" class="muted">关联实例已结束或不存在</span>
                  <span v-else>—</span>
                </span>
              </div>
              <div class="detail-item"><span class="detail-item__label">业务键</span><span class="detail-item__value">{{ textOrDash(instance?.businessKey) }}</span></div>
            </div>
          </div>
        </section>

        <section class="panel">
          <div class="panel__header">
            <span class="panel__title">异常栈</span>
            <CopyText v-if="stack" :value="stack" display="复制异常栈" style="margin-left: auto" />
          </div>
          <div class="panel__body">
            <p v-if="stackMissing" class="muted" style="margin: 0">该作业没有记录异常栈。</p>
            <ErrorBlock v-else-if="stackError" :error="stackError" context="GET /exception-stacktrace" @retry="loadStack" />
            <details v-else-if="stack" class="stack-details">
              <summary>展开异常栈（{{ stack.length }} 字符）</summary>
              <pre class="code-block">{{ stack }}</pre>
            </details>
          </div>
        </section>

        <section class="panel">
          <div class="panel__header">
            <span class="panel__title">操作</span>
            <span class="muted" style="margin-left: auto; font-size: 12px">{{ JOB_BUCKET_LABELS[bucket] }}作业</span>
          </div>
          <div class="panel__body">
            <div v-if="actionMessage" class="banner" role="status" style="margin-bottom: 12px">{{ actionMessage }}</div>
            <div v-if="actionError" class="banner banner--error" role="alert" style="margin-bottom: 12px">
              <AlertTriangle :size="16" aria-hidden="true" />
              <div class="grow">{{ errorMessage(actionError) }}</div>
            </div>

            <div v-if="bucket === 'executable'" class="action-row">
              <label class="field">
                <span class="field__label">重试次数</span>
                <input v-model.number="retries" class="native-input" type="number" min="0" step="1" :disabled="busy" />
              </label>
              <n-button size="small" :loading="actionLoading === 'setRetries'" :disabled="busy" @click="setRetries">设置重试次数</n-button>
              <ConfirmButton label="删除作业" title="删除该作业？" description="删除后该作业不再执行，且无法恢复。" :loading="actionLoading === 'delete'" :disabled="busy" @confirm="removeJob" />
            </div>

            <div v-else-if="bucket === 'timer'" class="stack" style="gap: 12px">
              <div class="action-row">
                <ConfirmButton label="转为可执行" title="立即把定时器作业移入可执行桶？" description="作业会立即变为可执行，可能在下一轮调度中执行。" :danger="false" :loading="actionLoading === 'move'" :disabled="busy" @confirm="moveJob" />
                <ConfirmButton label="删除作业" title="删除该定时器作业？" description="删除后流程不会再收到该定时器触发，且无法恢复。" :loading="actionLoading === 'delete'" :disabled="busy" @confirm="removeJob" />
              </div>
              <div class="action-row">
                <label class="field">
                  <span class="field__label">新到期时间</span>
                  <input v-model="dueDate" class="native-input" type="datetime-local" :disabled="busy" />
                </label>
                <n-button size="small" :loading="actionLoading === 'reschedule'" :disabled="busy" @click="reschedule">重新调度</n-button>
              </div>
            </div>

            <div v-else-if="bucket === 'deadletter'" class="action-row">
              <label class="field">
                <span class="field__label">恢复后重试次数</span>
                <input v-model.number="retries" class="native-input" type="number" min="0" step="1" :disabled="busy" />
              </label>
              <ConfirmButton label="恢复到可执行" title="把死信作业恢复到可执行桶？" description="作业会清除异常并回到可执行桶，建议先确认根因已修复。" :danger="false" :loading="actionLoading === 'move'" :disabled="busy" @confirm="moveJob" />
              <ConfirmButton label="删除作业" title="删除该死信作业？" description="删除后失败记录与重试入口都会消失，且无法恢复。" :loading="actionLoading === 'delete'" :disabled="busy" @confirm="removeJob" />
            </div>

            <div v-else class="action-row">
              <p class="muted" style="margin: 0">挂起作业由所属流程实例决定，服务端未提供恢复动作；请先激活实例。</p>
              <ConfirmButton label="删除作业" title="删除该挂起作业？" description="删除后实例恢复时不再执行该作业，且无法恢复。" :loading="actionLoading === 'delete'" :disabled="busy" @confirm="removeJob" />
            </div>
          </div>
        </section>
      </div>
    </n-spin>
  </div>
</template>

<style scoped>
.action-row { display: flex; align-items: flex-end; gap: 10px; flex-wrap: wrap; }
.field { display: flex; flex-direction: column; gap: 4px; }
.field__label { font-size: 12px; color: var(--muted); }
.native-input {
  height: 30px;
  padding: 0 8px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: var(--surface);
  color: var(--text);
  font-size: 13px;
}
.stack-details summary { cursor: pointer; color: var(--text-2); font-size: 13px; margin-bottom: 8px; }
</style>
