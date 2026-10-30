<script setup lang="ts">
// 用户头像：有头像地址时展示图片，加载失败或缺失时回退到姓名首字母。
import { computed, ref, watch } from 'vue'
import { NAvatar } from 'naive-ui'
import type { User } from '@/api/identity'

const props = withDefaults(
  defineProps<{
    /** 用户对象，用于取显示名与 pictureUrl */
    user?: User | null
    /** 显式图片地址，优先于用户字段（详情页使用 blob 地址） */
    src?: string | null
    /** 头像尺寸 */
    size?: number
  }>(),
  { user: null, src: null, size: 32 },
)

/** 图片加载是否失败 */
const failed = ref(false)

// 图片地址变化时重置失败状态，允许重新尝试。
// @param value 新图片地址
// @return 无
watch(
  () => props.src,
  () => {
    failed.value = false
  },
)

/** 实际图片地址，失败后不再回填 */
const image = computed(() => {
  if (failed.value) return undefined
  return props.src || props.user?.pictureUrl || undefined
})

/** 首字母：优先显示名，其次名/姓，最后 ID */
const initial = computed(() => {
  const user = props.user
  const raw = user?.displayName || user?.firstName || user?.lastName || user?.id || ''
  const trimmed = raw.trim()
  return trimmed ? trimmed.slice(0, 1).toUpperCase() : '?'
})

// 图片加载失败时回退首字母。
// @return 无
function onError(): void {
  failed.value = true
}
</script>

<template>
  <n-avatar round :size="size" :src="image" :on-error="onError" class="user-avatar">
    <span>{{ initial }}</span>
  </n-avatar>
</template>

<style scoped>
.user-avatar {
  flex-shrink: 0;
  background: var(--surface-3);
  color: var(--text-2);
  font-weight: 600;
}
</style>
