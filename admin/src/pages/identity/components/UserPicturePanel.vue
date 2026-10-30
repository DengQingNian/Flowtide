<script setup lang="ts">
// 用户头像：读取二进制预览，无头像回退首字母，支持上传新图片。
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { NButton } from 'naive-ui'
import ErrorBlock from '@/components/ErrorBlock.vue'
import UserAvatar from './UserAvatar.vue'
import { getUserPicture, setUserPicture } from '@/api/identity'
import type { User } from '@/api/identity'
import { ApiError, isAuthError } from '@/api/client'
import { markAuthError } from '@/stores/ui'

const props = defineProps<{
  /** 目标用户 */
  user: User
}>()

/** 预览地址 */
const previewUrl = ref('')
/** 头像是否存在 */
const hasPicture = ref(false)
/** 加载态 */
const loading = ref(false)
/** 读取错误 */
const loadError = ref<unknown>(null)
/** 上传错误 */
const uploadError = ref<unknown>(null)
/** 上传中 */
const uploading = ref(false)
/** 成功提示 */
const success = ref('')
/** 文件输入 */
const fileInput = ref<HTMLInputElement | null>(null)

// 释放旧预览地址，避免内存泄漏。
// @return 无
function revoke(): void {
  if (previewUrl.value) URL.revokeObjectURL(previewUrl.value)
  previewUrl.value = ''
}

// 读取头像；404 视为未设置头像而非错误。
// @return 无
async function load(): Promise<void> {
  loading.value = true
  loadError.value = null
  revoke()
  try {
    const blob = await getUserPicture(props.user.id)
    previewUrl.value = URL.createObjectURL(blob)
    hasPicture.value = true
  } catch (cause) {
    // 无头像时服务端返回 404，界面降级为首字母，不阻断页面。
    if (cause instanceof ApiError && cause.status === 404) {
      hasPicture.value = false
      return
    }
    loadError.value = cause
    if (isAuthError(cause)) markAuthError(cause)
  } finally {
    loading.value = false
  }
}

// 文件名扩展名仅用于提示，不参与请求构造。
// @param name 文件名
// @return 小写扩展名
function extensionOf(name: string): string {
  const index = name.lastIndexOf('.')
  return index >= 0 ? name.slice(index + 1).toLowerCase() : ''
}

// 处理头像文件选择并上传。
// @param event 文件输入事件
// @return 无
async function upload(event: Event): Promise<void> {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  uploading.value = true
  uploadError.value = null
  success.value = ''
  try {
    await setUserPicture(props.user.id, file)
    success.value = `头像已更新（${extensionOf(file.name) || 'image'}）`
    await load()
  } catch (cause) {
    uploadError.value = cause
    markAuthError(cause)
  } finally {
    uploading.value = false
    input.value = ''
  }
}

// 打开系统文件选择器。
// @return 无
function pickFile(): void {
  fileInput.value?.click()
}

watch(() => props.user.id, () => void load())
onMounted(() => void load())
onBeforeUnmount(revoke)
</script>

<template>
  <section class="panel">
    <div class="panel__header">
      <span class="panel__title">头像</span>
      <span class="muted" style="font-size: 12px">GET/PUT /identity/users/{{ user.id }}/picture</span>
    </div>
    <div class="panel__body">
      <ErrorBlock v-if="loadError" :error="loadError" context="GET /service/identity/users/{id}/picture" @retry="load" />
      <ErrorBlock v-if="uploadError" :error="uploadError" @retry="load" />
      <div v-if="success" class="banner" role="status" style="margin: 0 0 12px">{{ success }}</div>
      <div class="row" style="align-items: center; gap: 16px">
        <UserAvatar :user="user" :src="previewUrl" :size="72" />
        <div class="grow">
          <div>{{ hasPicture ? '当前使用上传的头像。' : '未设置头像，显示姓名首字母。' }}</div>
          <div class="muted" style="font-size: 12px">服务端按文件 part 的 Content-Type 保存图片，支持 PNG/JPEG/WebP。</div>
          <div class="row" style="margin-top: 8px">
            <input ref="fileInput" type="file" accept="image/*" style="display: none" data-test="picture-input" @change="upload" />
            <n-button size="small" :loading="uploading || loading" @click="pickFile">选择图片并上传</n-button>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>
