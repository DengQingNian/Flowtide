<script setup lang="ts">
// 一键复制组件：优先使用 Clipboard API，失败时回退到隐藏文本框。
import { ref } from 'vue'
import { Check, Copy } from '@lucide/vue'

const props = withDefaults(
  defineProps<{
    /** 要复制的文本 */
    value: string
    /** 展示文本，默认同 value */
    display?: string
    /** 是否截断显示 */
    truncate?: boolean
  }>(),
  { display: '', truncate: false },
)

const emit = defineEmits<{ copied: [string] }>()

const copied = ref(false)

// 使用隐藏文本框回退复制。
// @param text 文本
// @return 是否成功
function fallbackCopy(text: string): boolean {
  if (typeof document === 'undefined') return false
  const area = document.createElement('textarea')
  area.value = text
  area.setAttribute('readonly', 'true')
  area.style.position = 'fixed'
  area.style.opacity = '0'
  document.body.appendChild(area)
  area.select()
  let ok = false
  try {
    ok = typeof document.execCommand === 'function' ? document.execCommand('copy') : false
  } catch {
    ok = false
  }
  document.body.removeChild(area)
  return ok
}

// 复制文本并给出反馈。
// @return 无
async function copy(): Promise<void> {
  let ok = false
  try {
    if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(props.value)
      ok = true
    }
  } catch {
    ok = false
  }
  if (!ok) ok = fallbackCopy(props.value)
  if (!ok) return
  copied.value = true
  emit('copied', props.value)
  window.setTimeout(() => {
    copied.value = false
  }, 1500)
}
</script>

<template>
  <span class="copy-text">
    <span class="copy-text__value" :class="{ ellipsis: truncate }" :title="value">{{ display || value }}</span>
    <button class="copy-text__button" type="button" :title="copied ? '已复制' : '复制'" :aria-label="copied ? '已复制' : '复制'" @click="copy">
      <Check v-if="copied" :size="13" />
      <Copy v-else :size="13" />
    </button>
  </span>
</template>
