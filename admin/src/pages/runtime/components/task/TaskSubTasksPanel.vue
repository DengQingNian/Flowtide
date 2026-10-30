<script setup lang="ts">
// 子任务面板：展示当前任务的子任务并提供详情入口。
import { onMounted, ref } from 'vue'
import { NSpin } from 'naive-ui'
import { listSubTasks, type Task } from '@/api/task'
import { markAuthError } from '@/stores/ui'
import { formatDateTime, textOrDash } from '@/lib/format'
import EmptyState from '@/components/EmptyState.vue'
import ErrorBlock from '@/components/ErrorBlock.vue'

const props = defineProps<{
  /** 父任务 ID */
  taskId: string
}>()

const items = ref<Task[]>([])
const loading = ref(false)
const error = ref<unknown>(null)

// 加载子任务。
// @return 无
async function load(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    items.value = await listSubTasks(props.taskId)
  } catch (err) {
    markAuthError(err)
    error.value = err
    items.value = []
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <div>
    <div class="row row--between" style="margin-bottom: 10px">
      <span class="section-title" style="margin: 0">子任务（{{ items.length }}）</span>
    </div>
    <error-block v-if="error" :error="error" :context="`GET /service/runtime/tasks/${taskId}/subtasks`" @retry="load" />
    <n-spin :show="loading">
      <empty-state v-if="!loading && items.length === 0" title="暂无子任务" description="通过创建任务时填写父任务 ID 可生成子任务。" />
      <table v-else class="kv-table">
        <thead><tr><th>名称</th><th>办理人</th><th>优先级</th><th>创建时间</th><th>到期时间</th></tr></thead>
        <tbody>
          <tr v-for="item in items" :key="item.id">
            <td><router-link :to="`/runtime/tasks/${item.id}`">{{ textOrDash(item.name) }}</router-link></td>
            <td>{{ textOrDash(item.assignee) }}</td>
            <td>{{ item.priority }}</td>
            <td>{{ formatDateTime(item.createTime) }}</td>
            <td>{{ formatDateTime(item.dueDate) }}</td>
          </tr>
        </tbody>
      </table>
    </n-spin>
  </div>
</template>
