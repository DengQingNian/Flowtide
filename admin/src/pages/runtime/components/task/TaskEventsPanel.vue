<script setup lang="ts">
// 任务事件面板：按时间展示任务生命周期事件。
import { onMounted, ref } from 'vue'
import { NSpin } from 'naive-ui'
import { listTaskEvents, type TaskEvent } from '@/api/task'
import { markAuthError } from '@/stores/ui'
import { formatDateTime, textOrDash } from '@/lib/format'
import EmptyState from '@/components/EmptyState.vue'
import ErrorBlock from '@/components/ErrorBlock.vue'

const props = defineProps<{
  /** 任务 ID */
  taskId: string
}>()

const events = ref<TaskEvent[]>([])
const loading = ref(false)
const error = ref<unknown>(null)

/** 动作中文名 */
const ACTION_LABELS: Record<string, string> = {
  create: '创建',
  claim: '签收',
  unclaim: '取消签收',
  complete: '完成',
  delegate: '委派',
  resolve: '解决委派',
  update: '更新',
  delete: '删除',
}

// 返回动作中文名。
// @param action 动作
// @return 中文名
function actionLabel(action: string): string {
  return ACTION_LABELS[action.toLowerCase()] ?? action
}

// 加载任务事件。
// @return 无
async function load(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    events.value = await listTaskEvents(props.taskId)
  } catch (err) {
    markAuthError(err)
    error.value = err
    events.value = []
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <div>
    <div class="row row--between" style="margin-bottom: 10px">
      <span class="section-title" style="margin: 0">事件（{{ events.length }}）</span>
    </div>
    <error-block v-if="error" :error="error" :context="`GET /service/runtime/tasks/${taskId}/events`" @retry="load" />
    <n-spin :show="loading">
      <empty-state v-if="!loading && events.length === 0" title="暂无事件" description="任务创建、签收、委派与完成后会记录事件。" />
      <table v-else class="kv-table">
        <thead><tr><th style="width: 170px">时间</th><th style="width: 120px">动作</th><th style="width: 140px">操作人</th><th>附加数据</th></tr></thead>
        <tbody>
          <tr v-for="event in events" :key="event.id">
            <td>{{ formatDateTime(event.timestamp) }}</td>
            <td>{{ actionLabel(event.action) }}</td>
            <td>{{ textOrDash(event.userId) }}</td>
            <td class="mono ellipsis" :title="event.data">{{ textOrDash(event.data) }}</td>
          </tr>
        </tbody>
      </table>
    </n-spin>
  </div>
</template>
