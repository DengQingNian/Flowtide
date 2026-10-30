<script setup lang="ts">
// 历史原始数据弹窗：加载二进制变量/明细、识别文本、下载与重试。
import { computed, ref, watch } from 'vue'
import { NButton, NModal, NSpin } from 'naive-ui'
import ErrorBlock from '@/components/ErrorBlock.vue'
import { isAuthError, errorMessage } from '@/api/client'
import { markAuthError } from '@/stores/ui'
import { downloadBlob } from '@/lib/download'
import { formatBytes } from '@/lib/format'

const props = withDefaults(
  defineProps<{
    /** 是否展示 */
    show: boolean
    /** 弹窗标题 */
    title?: string
    /** 下载文件名 */
    filename?: string
    /** 数据加载函数，为空时展示提示 */
    loader?: (() => Promise<Blob>) | null
  }>(),
  { title: '原始数据', filename: 'value.bin', loader: null },
)

const emit = defineEmits<{ 'update:show': [boolean] }>()

/** 加载态 */
const loading = ref(false)
/** 加载错误 */
const error = ref<unknown>(null)
/** 文本内容；二进制时为 null */
const text = ref<string | null>(null)
/** 是否为二进制内容 */
const binary = ref(false)
/** 内容大小 */
const size = ref(0)
/** 原始数据，供下载复用 */
const blob = ref<Blob | null>(null)

/** 弹窗显隐双向绑定 */
const visible = computed({
  get: () => props.show,
  set: (value: boolean) => emit('update:show', value),
})

// 判断文本是否更像二进制内容。
// @param value 解码后的文本
// @return 是否按二进制展示
function looksBinary(value: string): boolean {
  // 含替换字符或不可打印控制字符时按二进制处理
  return /\uFFFD|[\u0000-\u0008\u000E-\u001F]/.test(value)
}

// 用 FileReader 读取 Blob 文本，兼容不支持 Blob.text 的浏览器与测试环境。
// @param value 二进制数据
// @return 文本内容
function readText(value: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result ?? ''))
    reader.onerror = () => reject(reader.error ?? new Error('读取原始数据失败'))
    reader.readAsText(value)
  })
}

// 加载原始数据并按内容类型选择展示方式。
// @return 无
async function load(): Promise<void> {
  if (!props.loader) return
  loading.value = true
  error.value = null
  text.value = null
  binary.value = false
  size.value = 0
  blob.value = null
  try {
    const data = await props.loader()
    blob.value = data
    size.value = data.size
    const decoded = await readText(data)
    // 非空内容先判断是否为二进制，再决定展示文本还是下载提示
    if (decoded.length > 0 && looksBinary(decoded)) {
      binary.value = true
    } else {
      text.value = decoded
    }
  } catch (cause) {
    error.value = cause
    if (isAuthError(cause)) markAuthError(cause)
  } finally {
    loading.value = false
  }
}

// 下载当前原始数据。
// @return 无
function download(): void {
  if (!blob.value) return
  downloadBlob(props.filename, blob.value)
}

watch(
  () => props.show,
  (value) => {
    // 每次打开都重新拉取，避免展示上一次的数据
    if (value) void load()
  },
  { immediate: true },
)
</script>

<template>
  <n-modal v-model:show="visible" preset="card" :title="title" :style="{ width: '720px', maxWidth: '94vw' }">
    <n-spin :show="loading">
      <ErrorBlock v-if="error" :error="error" @retry="load" />
      <template v-else>
        <p v-if="!loader" class="muted">该记录没有可读取的原始数据地址。</p>
        <template v-else>
          <div class="row" style="margin-bottom: 8px">
            <span class="muted">大小：{{ formatBytes(size) }}</span>
            <span v-if="binary" class="muted">内容为二进制，无法直接阅读，请下载后查看。</span>
          </div>
          <pre v-if="!binary" class="code-block">{{ text || '（空内容）' }}</pre>
        </template>
      </template>
    </n-spin>
    <template #footer>
      <div class="row" style="justify-content: flex-end">
        <span v-if="error" class="muted ellipsis" :title="errorMessage(error)">{{ errorMessage(error) }}</span>
        <n-button size="small" :disabled="!blob" @click="download">下载原始数据</n-button>
        <n-button size="small" @click="visible = false">关闭</n-button>
      </div>
    </template>
  </n-modal>
</template>
