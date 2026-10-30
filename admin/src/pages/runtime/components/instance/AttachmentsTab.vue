<script setup lang="ts">
// 实例附件：外链附件的新增、列表与删除。
// 服务端流程实例附件 POST 只接受 JSON，且没有附件内容下载端点；上传能力仅任务附件提供。
import { onMounted, ref } from 'vue'
import { NButton, NInput, NPagination, NSpin } from 'naive-ui'
import type { Attachment } from '@/api/types'
import { addInstanceAttachment, deleteInstanceAttachment, listInstanceAttachments } from '@/api/runtime'
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

const rows = ref<Attachment[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(50)
const loading = ref(false)
const error = ref<unknown>(null)
const actionError = ref('')
const notice = ref('')
const submitting = ref(false)
const deletingId = ref('')

const draftName = ref('')
const draftDesc = ref('')
const draftType = ref('url')
const draftLink = ref('')

// 加载附件分页。
// @return 无
async function load(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    const result = await listInstanceAttachments(props.instanceId, { start: (page.value - 1) * pageSize.value, size: pageSize.value })
    rows.value = result.data ?? []
    total.value = result.total ?? 0
  } catch (cause) {
    error.value = cause
    markAuthError(cause)
  } finally {
    loading.value = false
  }
}

// 新增外链附件。
// @return 无
async function submit(): Promise<void> {
  actionError.value = ''
  notice.value = ''
  if (!draftName.value.trim()) {
    actionError.value = '附件名称不能为空'
    return
  }
  submitting.value = true
  try {
    await addInstanceAttachment(props.instanceId, {
      name: draftName.value.trim(),
      description: draftDesc.value.trim() || undefined,
      type: draftType.value.trim() || 'url',
      url: draftLink.value.trim() || undefined,
    })
    draftName.value = ''
    draftDesc.value = ''
    draftLink.value = ''
    notice.value = '附件已添加'
    page.value = 1
    await load()
  } catch (cause) {
    actionError.value = errorMessage(cause)
    markAuthError(cause)
  } finally {
    submitting.value = false
  }
}

// 删除附件。
// @param row 附件
// @return 无
async function remove(row: Attachment): Promise<void> {
  deletingId.value = row.id
  actionError.value = ''
  notice.value = ''
  try {
    await deleteInstanceAttachment(props.instanceId, row.id)
    notice.value = '附件已删除'
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
    <div class="banner banner--warning" style="margin: 0">
      <span class="grow">流程实例附件接口只接受 JSON 外链，没有内容上传与下载端点；需要上传文件时请在任务附件中操作。</span>
    </div>

    <div class="panel">
      <div class="panel__header"><span class="panel__title">添加外链附件</span></div>
      <div class="panel__body">
        <div class="row">
          <n-input v-model:value="draftName" size="small" placeholder="附件名称（必填）" style="max-width: 220px" />
          <n-input v-model:value="draftType" size="small" placeholder="类型，默认 url" style="max-width: 140px" />
          <n-input :value="draftLink" size="small" placeholder="外链地址 https://…" style="flex: 1; min-width: 220px" @update:value="draftLink = $event" />
        </div>
        <div class="row" style="margin-top: 8px">
          <n-input :value="draftDesc" size="small" placeholder="描述（可选）" style="flex: 1" @update:value="draftDesc = $event" />
          <n-button size="small" type="primary" :loading="submitting" @click="submit">添加附件</n-button>
        </div>
        <div class="row" style="margin-top: 8px">
          <span v-if="notice" class="text-success">{{ notice }}</span>
          <span v-if="actionError" class="text-danger" role="alert">{{ actionError }}</span>
        </div>
      </div>
    </div>

    <div class="panel">
      <div class="panel__header">
        <span class="panel__title">附件列表</span>
        <span class="muted" style="margin-left: auto; font-size: 12px">共 {{ total }} 条</span>
      </div>
      <div class="panel__body">
        <ErrorBlock v-if="error" :error="error" @retry="load" />
        <n-spin v-else :show="loading">
          <EmptyState v-if="!loading && rows.length === 0" title="暂无附件" description="流程实例附件仅支持登记外链地址。" />
          <div v-else>
            <table class="kv-table">
              <thead>
                <tr>
                  <th>名称</th>
                  <th>类型</th>
                  <th>描述</th>
                  <th>链接</th>
                  <th>创建时间</th>
                  <th style="width: 90px">操作</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="row in rows" :key="row.id">
                  <td>{{ row.name }}</td>
                  <td>{{ textOrDash(row.type) }}</td>
                  <td>{{ textOrDash(row.description) }}</td>
                  <td>
                    <a v-if="row.url" :href="row.url" target="_blank" rel="noopener noreferrer" class="mono">打开链接</a>
                    <span v-else class="muted">—</span>
                  </td>
                  <td>{{ formatDateTime(row.time) }}</td>
                  <td>
                    <ConfirmButton
                      label="删除"
                      title="确认删除该附件记录？"
                      description="仅删除附件登记信息，不会删除外链指向的文件。"
                      :loading="deletingId === row.id"
                      @confirm="remove(row)"
                    />
                  </td>
                </tr>
              </tbody>
            </table>
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