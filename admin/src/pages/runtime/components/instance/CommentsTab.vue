<script setup lang="ts">
// 实例评论：分页展示、新增与删除。
import { onMounted, ref } from 'vue'
import { NButton, NInput, NPagination, NSpin } from 'naive-ui'
import type { Comment } from '@/api/types'
import { addInstanceComment, deleteInstanceComment, listInstanceComments } from '@/api/runtime'
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

const comments = ref<Comment[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(50)
const loading = ref(false)
const error = ref<unknown>(null)
const actionError = ref('')
const notice = ref('')

const message = ref('')
const userId = ref('')
const submitting = ref(false)
const deletingId = ref('')

// 加载评论分页。
// @return 无
async function load(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    const result = await listInstanceComments(props.instanceId, { start: (page.value - 1) * pageSize.value, size: pageSize.value })
    comments.value = result.data ?? []
    total.value = result.total ?? 0
  } catch (cause) {
    error.value = cause
    markAuthError(cause)
  } finally {
    loading.value = false
  }
}

// 提交评论。
// @return 无
async function submit(): Promise<void> {
  actionError.value = ''
  notice.value = ''
  if (!message.value.trim()) {
    actionError.value = '评论内容不能为空'
    return
  }
  submitting.value = true
  try {
    await addInstanceComment(props.instanceId, message.value.trim(), userId.value.trim())
    message.value = ''
    notice.value = '评论已提交'
    page.value = 1
    await load()
  } catch (cause) {
    actionError.value = errorMessage(cause)
    markAuthError(cause)
  } finally {
    submitting.value = false
  }
}

// 删除评论。
// @param comment 评论
// @return 无
async function remove(comment: Comment): Promise<void> {
  deletingId.value = comment.id
  actionError.value = ''
  notice.value = ''
  try {
    await deleteInstanceComment(props.instanceId, comment.id)
    notice.value = '评论已删除'
    await load()
  } catch (cause) {
    actionError.value = errorMessage(cause)
    markAuthError(cause)
  } finally {
    deletingId.value = ''
  }
}

// 切换页码。
// @param value 页码
// @return 无
function changePage(value: number): void {
  page.value = value
  void load()
}

onMounted(load)
</script>

<template>
  <div class="stack">
    <div class="panel">
      <div class="panel__header"><span class="panel__title">发表评论</span></div>
      <div class="panel__body">
        <div class="row" style="align-items: flex-start">
          <n-input v-model:value="message" type="textarea" :rows="3" placeholder="输入评论内容，提交后写入实例评论" style="flex: 1" />
          <n-input v-model:value="userId" placeholder="作者（可选）" style="width: 160px" />
        </div>
        <div class="row" style="margin-top: 8px">
          <span v-if="notice" class="text-success">{{ notice }}</span>
          <span v-if="actionError" class="text-danger" role="alert">{{ actionError }}</span>
          <n-button size="small" type="primary" style="margin-left: auto" :loading="submitting" @click="submit">提交评论</n-button>
        </div>
      </div>
    </div>

    <div class="panel">
      <div class="panel__header">
        <span class="panel__title">评论列表</span>
        <span class="muted" style="margin-left: auto; font-size: 12px">共 {{ total }} 条</span>
      </div>
      <div class="panel__body">
        <ErrorBlock v-if="error" :error="error" @retry="load" />
        <n-spin v-else :show="loading">
          <EmptyState v-if="!loading && comments.length === 0" title="暂无评论" description="在上方输入框发表第一条评论。" />
          <div v-else>
            <div
              v-for="comment in comments"
              :key="comment.id"
              style="padding: 10px 0; border-bottom: 1px solid var(--border)"
            >
              <div class="row">
                <span class="mono">{{ textOrDash(comment.userId) }}</span>
                <span class="muted">{{ formatDateTime(comment.time) }}</span>
                <span style="margin-left: auto">
                  <ConfirmButton
                    label="删除"
                    title="确认删除该评论？"
                    description="删除后评论不可恢复，历史记录中也不再展示。"
                    :loading="deletingId === comment.id"
                    @confirm="remove(comment)"
                  />
                </span>
              </div>
              <div style="white-space: pre-wrap; margin-top: 4px">{{ comment.message }}</div>
            </div>
            <div class="pager">
              <n-pagination
                v-if="total > pageSize"
                :page="page"
                :page-size="pageSize"
                :item-count="total"
                @update:page="changePage"
              />
            </div>
          </div>
        </n-spin>
      </div>
    </div>
  </div>
</template>
