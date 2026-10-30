<script setup lang="ts">
// 作业列表：四类作业桶切换、分类计数、远程分页与到期作业立即执行。
import { computed, h, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter, RouterLink } from 'vue-router'
import { NButton } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { AlertTriangle, RefreshCw } from '@lucide/vue'
import PageHeader from '@/components/PageHeader.vue'
import DataTable from '@/components/DataTable.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import CopyText from '@/components/CopyText.vue'
import { errorMessage, isAuthError } from '@/api/client'
import { markAuthError } from '@/stores/ui'
import { countJobs, isJobBucket, listJobs, normalizeJobBucket, runDueJobs } from '@/api/management'
import type { JobBucket, JobInfo } from '@/api/management'
import { JOB_BUCKET_LABELS, JOB_BUCKET_ORDER } from './components/managementUtils'
import { formatDateTime, formatNumber, shortId, textOrDash, toDate } from '@/lib/format'

const route = useRoute()
const router = useRouter()

/** 当前作业桶 */
const bucket = computed<JobBucket>(() => normalizeJobBucket(route.params.bucket))
/** 作业数据 */
const jobs = ref<JobInfo[]>([])
/** 总条数 */
const total = ref(0)
/** 当前页，从 1 开始 */
const page = ref(1)
/** 每页条数 */
const pageSize = ref(50)
/** 加载态 */
const loading = ref(false)
/** 列表错误 */
const error = ref<unknown>(null)
/** 分类计数 */
const counts = ref<Record<JobBucket, number | null>>({ executable: null, timer: null, suspended: null, deadletter: null })
/** 立即执行结果 */
const runMessage = ref('')
/** 立即执行错误 */
const runError = ref<unknown>(null)
/** 立即执行加载态 */
const running = ref(false)

/** 各桶空态文案 */
const EMPTY_TEXT: Record<JobBucket, string> = {
  executable: '当前没有可执行作业，流程会按计划继续推进。',
  timer: '当前没有等待中的定时器作业。',
  suspended: '当前没有挂起实例产生的作业。',
  deadletter: '死信桶为空，暂无需要人工介入的失败作业。',
}

// 格式化到期时间，忽略 Go 的零值时间。
// @param value 时间文本
// @return 展示文本
function dueText(value: string | null | undefined): string {
  const date = toDate(value)
  if (!date || date.getUTCFullYear() <= 1) return '—'
  return formatDateTime(value)
}

// 加载当前桶作业。
// @return 无
async function load(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    const result = await listJobs(bucket.value, { start: (page.value - 1) * pageSize.value, size: pageSize.value })
    jobs.value = result.data
    total.value = result.total
  } catch (cause) {
    error.value = cause
    markAuthError(cause)
  } finally {
    loading.value = false
  }
}

// 加载四类作业计数。
// @return 无
async function loadCounts(): Promise<void> {
  const next: Record<JobBucket, number | null> = { executable: null, timer: null, suspended: null, deadletter: null }
  await Promise.all(
    JOB_BUCKET_ORDER.map(async (key) => {
      try {
        next[key] = await countJobs(key)
      } catch (cause) {
        // 计数失败不阻塞列表，只把对应数字置为未知
        if (isAuthError(cause)) markAuthError(cause)
      }
    }),
  )
  counts.value = next
}

// 切换作业桶。
// @param key 目标桶
// @return 无
function switchBucket(key: JobBucket): void {
  if (key === bucket.value) return
  page.value = 1
  void router.push({ name: 'jobs', params: { bucket: key } })
}

// 处理页码变化。
// @param value 新页码
// @return 无
function changePage(value: number): void {
  page.value = value
  void load()
}

// 处理每页条数变化。
// @param value 新条数
// @return 无
function changePageSize(value: number): void {
  pageSize.value = value
  page.value = 1
  void load()
}

// 立即执行到期定时器作业。
// @return 无
async function runJobs(): Promise<void> {
  running.value = true
  runError.value = null
  runMessage.value = ''
  try {
    const result = await runDueJobs()
    runMessage.value = `已执行 ${result.executed} 个到期作业。`
    await Promise.all([load(), loadCounts()])
  } catch (cause) {
    runError.value = cause
    markAuthError(cause)
  } finally {
    running.value = false
  }
}

/** 表格列 */
const columns = computed<DataTableColumns<JobInfo>>(() => [
  {
    key: 'id',
    title: '作业 ID',
    width: 170,
    render: (row) => h(RouterLink, { to: `/management/jobs/${bucket.value}/${row.id}`, class: 'mono' }, { default: () => shortId(row.id) }),
  },
  { key: 'type', title: '类型', width: 160, ellipsis: { tooltip: true }, render: (row) => textOrDash(row.type) },
  { key: 'retries', title: '重试次数', width: 92, render: (row) => String(row.retries ?? 0) },
  { key: 'exception', title: '异常摘要', minWidth: 260, ellipsis: { tooltip: true }, render: (row) => textOrDash(row.exception) },
  { key: 'dueDate', title: '到期时间', width: 170, render: (row) => dueText(row.dueDate) },
  {
    key: 'executionId',
    title: '执行',
    width: 160,
    render: (row) => (row.executionId ? h(CopyText, { value: row.executionId, display: shortId(row.executionId) }) : '—'),
  },
  {
    key: 'processInstanceId',
    title: '流程实例',
    width: 160,
    render: (row) =>
      row.processInstanceId
        ? h(RouterLink, { to: `/runtime/instances/${row.processInstanceId}`, class: 'mono' }, { default: () => shortId(row.processInstanceId) })
        : '—',
  },
])

// 监听桶变化并按需纠正非法路由。
watch(
  () => route.params.bucket,
  (value) => {
    if (!isJobBucket(value)) {
      void router.replace({ name: 'jobs', params: { bucket: 'executable' } })
      return
    }
    page.value = 1
    void load()
  },
  { immediate: true },
)

onMounted(() => {
  void loadCounts()
})
</script>

<template>
  <div>
    <PageHeader title="作业" description="按可执行、定时、挂起与死信四类桶查看作业，处理失败与到期任务。">
      <template #actions>
        <ConfirmButton
          label="立即执行到期作业"
          title="立即执行全部到期定时器作业？"
          description="将执行所有已到期的定时器作业，可能推进流程实例并产生新的任务。"
          :danger="false"
          :loading="running"
          @confirm="runJobs"
        />
        <n-button size="small" :loading="loading" @click="load">
          <template #icon><RefreshCw :size="14" /></template>
          刷新
        </n-button>
      </template>
    </PageHeader>

    <div class="bucket-tabs" role="tablist" aria-label="作业分类">
      <button
        v-for="key in JOB_BUCKET_ORDER"
        :key="key"
        type="button"
        role="tab"
        class="bucket-tab"
        :class="{ 'bucket-tab--active': key === bucket }"
        :aria-selected="key === bucket"
        @click="switchBucket(key)"
      >
        <span>{{ JOB_BUCKET_LABELS[key] }}</span>
        <span class="bucket-tab__count">{{ counts[key] === null ? '—' : formatNumber(counts[key]) }}</span>
      </button>
    </div>

    <div v-if="runMessage" class="banner" role="status" style="margin-bottom: 12px">{{ runMessage }}</div>
    <div v-if="runError" class="banner banner--error" role="alert" style="margin-bottom: 12px">
      <AlertTriangle :size="16" aria-hidden="true" />
      <div class="grow">{{ errorMessage(runError) }}</div>
    </div>

    <DataTable
      :columns="columns"
      :data="jobs"
      :loading="loading"
      :total="total"
      :page="page"
      :page-size="pageSize"
      :page-sizes="[10, 20, 50, 100, 200, 500, 1000]"
      row-key="id"
      :error="error"
      :empty-description="EMPTY_TEXT[bucket]"
      :scroll-x="1100"
      @update:page="changePage"
      @update:page-size="changePageSize"
      @retry="load"
    />
  </div>
</template>

<style scoped>
.bucket-tabs { display: flex; gap: 4px; flex-wrap: wrap; margin-bottom: 14px; border-bottom: 1px solid var(--border); }
.bucket-tab {
  border: 0;
  border-bottom: 2px solid transparent;
  background: transparent;
  color: var(--text-2);
  padding: 8px 12px;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
}
.bucket-tab:hover { color: var(--text); background: var(--hover-bg); }
.bucket-tab--active { color: var(--text); border-bottom-color: var(--text); font-weight: 500; }
.bucket-tab__count { font-size: 12px; color: var(--muted); background: var(--surface-2); padding: 0 6px; border-radius: 5px; }
</style>
