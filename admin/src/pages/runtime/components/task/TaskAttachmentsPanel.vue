<script setup lang="ts">
// 任务附件面板：multipart 上传、内容下载与删除。
import { onMounted, ref } from 'vue'
import { NAlert, NButton, NInput, NSpin } from 'naive-ui'
import { errorMessage } from '@/api/client'
import { deleteTaskAttachment, getTaskAttachmentContent, listTaskAttachments, uploadTaskAttachment } from '@/api/task'
import { markAuthError } from '@/stores/ui'
import type { Attachment } from '@/api/types'
import { downloadBlob } from '@/lib/download'
import { formatDateTime, textOrDash } from '@/lib/format'
import ConfirmButton from '@/components/ConfirmButton.vue'
import EmptyState from '@/components/EmptyState.vue'
import ErrorBlock from '@/components/ErrorBlock.vue'

const props = defineProps<{
  /** 任务 ID */
  taskId: string
}>()

const attachments = ref<Attachment[]>([])
const total = ref(0)
const loading = ref(false)
const error = ref<unknown>(null)
const feedback = ref('')
const file = ref<File | null>(null)
const name = ref('')
const description = ref('')
const type = ref('')
const uploading = ref(false)
const downloadingId = ref('')

// 加载附件列表。
// @return 无
async function load(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    const page = await listTaskAttachments(props.taskId, 0, 100)
    attachments.value = page.data
    total.value = page.total
  } catch (err) {
    markAuthError(err)
    error.value = err
    attachments.value = []
  } finally {
    loading.value = false
  }
}

// 选择文件并带出默认名称。
// @param event 文件输入事件
// @return 无
function onFile(event: Event): void {
  const target = event.target as HTMLInputElement | null
  file.value = target?.files?.[0] ?? null
  if (file.value && !name.value.trim()) name.value = file.value.name
}

// 上传附件并刷新列表。
// @return 无
async function upload(): Promise<void> {
  feedback.value = ''
  if (!file.value) {
    feedback.value = '请先选择文件'
    return
  }
  uploading.value = true
  try {
    await uploadTaskAttachment(props.taskId, file.value, {
      name: name.value.trim() || file.value.name,
      description: description.value.trim() || undefined,
      type: type.value.trim() || file.value.type || undefined,
    })
    feedback.value = '附件已上传'
    file.value = null
    name.value = ''
    description.value = ''
    type.value = ''
    await load()
  } catch (err) {
    markAuthError(err)
    feedback.value = `上传失败：${errorMessage(err)}`
  } finally {
    uploading.value = false
  }
}

// 下载附件内容。
// @param attachment 附件
// @return 无
async function download(attachment: Attachment): Promise<void> {
  feedback.value = ''
  downloadingId.value = attachment.id
  try {
    const blob = await getTaskAttachmentContent(props.taskId, attachment.id)
    downloadBlob(attachment.name || 'attachment.bin', blob)
    feedback.value = `已开始下载 ${attachment.name || attachment.id}`
  } catch (err) {
    markAuthError(err)
    feedback.value = `下载失败：${errorMessage(err)}`
  } finally {
    downloadingId.value = ''
  }
}

// 删除附件。
// @param attachment 附件
// @return 无
async function remove(attachment: Attachment): Promise<void> {
  feedback.value = ''
  try {
    await deleteTaskAttachment(props.taskId, attachment.id)
    feedback.value = '附件已删除'
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
      <span class="section-title" style="margin: 0">附件（{{ total }}）</span>
    </div>
    <n-alert v-if="feedback" :type="feedback.includes('失败') || feedback.includes('请先') ? 'error' : 'success'" :bordered="false" style="margin-bottom: 10px">
      {{ feedback }}
    </n-alert>
    <div class="panel" style="margin-bottom: 12px">
      <div class="panel__body">
        <div class="row" style="align-items: flex-end">
          <div class="filter-bar__item">
            <span class="filter-bar__label">选择文件</span>
            <input type="file" data-test="attachment-file" @change="onFile" />
          </div>
          <div class="filter-bar__item">
            <span class="filter-bar__label">名称</span>
            <n-input v-model:value="name" placeholder="默认取文件名" />
          </div>
          <div class="filter-bar__item">
            <span class="filter-bar__label">类型</span>
            <n-input v-model:value="type" placeholder="默认取文件 MIME" />
          </div>
          <n-button size="small" type="primary" :loading="uploading" @click="upload">上传</n-button>
        </div>
        <div class="filter-bar__item" style="margin-top: 8px">
          <span class="filter-bar__label">描述</span>
          <n-input v-model:value="description" placeholder="可选" />
        </div>
      </div>
    </div>
    <error-block v-if="error" :error="error" :context="`GET /service/runtime/tasks/${taskId}/attachments`" @retry="load" />
    <n-spin :show="loading">
      <empty-state v-if="!loading && attachments.length === 0" title="暂无附件" description="上传处理材料后，可在此下载或删除。" />
      <table v-else class="kv-table">
        <thead>
          <tr><th>名称</th><th style="width: 150px">类型</th><th style="width: 160px">时间</th><th>描述</th><th style="width: 150px">操作</th></tr>
        </thead>
        <tbody>
          <tr v-for="attachment in attachments" :key="attachment.id">
            <td>{{ textOrDash(attachment.name) }}</td>
            <td class="mono">{{ textOrDash(attachment.type) }}</td>
            <td>{{ formatDateTime(attachment.time) }}</td>
            <td>{{ textOrDash(attachment.description) }}</td>
            <td>
              <div class="row">
                <n-button size="tiny" quaternary :loading="downloadingId === attachment.id" @click="download(attachment)">下载</n-button>
                <confirm-button label="删除" title="确认删除该附件？" description="附件内容会一并删除，无法恢复。" @confirm="remove(attachment)" />
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </n-spin>
  </div>
</template>