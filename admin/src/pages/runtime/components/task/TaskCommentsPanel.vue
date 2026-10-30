<script setup lang="ts">
// 任务评论面板：分页读取、新增与删除评论。
import { onMounted, ref } from 'vue'
import { NAlert, NButton, NInput, NSpin } from 'naive-ui'
import { errorMessage } from '@/api/client'
import { addTaskComment, deleteTaskComment, listTaskComments } from '@/api/task'
import { markAuthError } from '@/stores/ui'
import type { Comment } from '@/api/types'
import { formatDateTime, textOrDash } from '@/lib/format'
import ConfirmButton from '@/components/ConfirmButton.vue'
import EmptyState from '@/components/EmptyState.vue'
import ErrorBlock from '@/components/ErrorBlock.vue'

const props = defineProps<{
  /** 任务 ID */
  taskId: string
}>()

const comments = ref<Comment[]>([])
const total = ref(0)
const loading = ref(false)
const error = ref<unknown>(null)
const message = ref('')
const userId = ref('')
const submitting = ref(false)
const feedback = ref('')

// 加载评论。
// @return 无
async function load(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    const page = await listTaskComments(props.taskId, 0, 100)
    comments.value = page.data
    total.value = page.total
  } catch (err) {
    markAuthError(err)
    error.value = err
    comments.value = []
  } finally {
    loading.value = false
  }
}

// 新增评论并刷新列表。
// @return 无
async function add(): Promise<void> {
  feedback.value = ''
  if (!message.value.trim()) {
    feedback.value = '评论内容不能为空'
    return
  }
  submitting.value = true
  try {
    await addTaskComment(props.taskId, message.value.trim(), userId.value.trim() || undefined)
    message.value = ''
    feedback.value = '评论已发布'
    await load()
  } catch (err) {
    markAuthError(err)
    feedback.value = `发布失败：${errorMessage(err)}`
  } finally {
    submitting.value = false
  }
}

// 删除评论。
// @param comment 评论
// @return 无
async function remove(comment: Comment): Promise<void> {
  feedback.value = ''
  try {
    await deleteTaskComment(props.taskId, comment.id)
    feedback.value = '评论已删除'
    await load()
  } catch (err) {
    markAuthError(err)
    feedback.value = `删除失败：${errorMessage(err)}`
  }
}

onMounted(load)
</script>

<template>
  <div>
    <div class="row row--between" style="margin-bottom: 10px">
      <span class="section-title" style="margin: 0">评论（{{ total }}）</span>
    </div>
    <n-alert v-if="feedback" :type="feedback.includes('失败') || feedback.includes('不能为空') ? 'error' : 'success'" :bordered="false" style="margin-bottom: 10px">
      {{ feedback }}
    </n-alert>
    <div class="row" style="align-items: flex-end; margin-bottom: 12px">
      <div class="filter-bar__item grow">
        <span class="filter-bar__label">评论内容</span>
        <n-input v-model:value="message" type="textarea" :rows="2" placeholder="记录处理说明" />
      </div>
      <div class="filter-bar__item">
        <span class="filter-bar__label">作者</span>
        <n-input v-model:value="userId" placeholder="可选用户 ID" />
      </div>
      <n-button size="small" type="primary" :loading="submitting" @click="add">发布</n-button>
    </div>
    <error-block v-if="error" :error="error" :context="`GET /service/runtime/tasks/${taskId}/comments`" @retry="load" />
    <n-spin :show="loading">
      <empty-state v-if="!loading && comments.length === 0" title="暂无评论" description="处理说明与交接记录会显示在这里。" />
      <table v-else class="kv-table">
        <thead>
          <tr><th style="width: 160px">时间</th><th style="width: 120px">作者</th><th>内容</th><th style="width: 90px">操作</th></tr>
        </thead>
        <tbody>
          <tr v-for="comment in comments" :key="comment.id">
            <td>{{ formatDateTime(comment.time) }}</td>
            <td>{{ textOrDash(comment.userId) }}</td>
            <td style="white-space: pre-wrap">{{ comment.message }}</td>
            <td><confirm-button label="删除" title="确认删除该评论？" description="删除后不可恢复。" @confirm="remove(comment)" /></td>
          </tr>
        </tbody>
      </table>
    </n-spin>
  </div>
</template>
