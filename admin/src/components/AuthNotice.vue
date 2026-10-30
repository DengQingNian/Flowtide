<script setup lang="ts">
// 认证与权限提示：401/403 时说明原因，不清空页面、不跳转。
import { computed } from 'vue'
import { X } from '@lucide/vue'
import { authNotice, clearAuthNotice } from '@/stores/ui'

/** 提示文案 */
const message = computed(() => {
  if (authNotice.value === 'unauthorized') {
    return '服务端已启用认证，本期管理台未提供登录入口。请确认服务端未开启认证，或由前置反向代理注入认证请求头；当前页面与已填内容不会丢失。'
  }
  if (authNotice.value === 'forbidden') {
    return '当前账号没有该操作的权限（服务端返回 403）。页面与已填内容会保留，具体原因见操作结果提示。'
  }
  return ''
})

// 关闭提示。
// @return 无
function close(): void {
  clearAuthNotice()
}
</script>

<template>
  <div v-if="authNotice !== 'none'" class="banner banner--error" role="alert">
    <span>{{ message }}</span>
    <span class="banner__actions">
      <button class="icon-button" type="button" title="关闭提示" aria-label="关闭提示" @click="close">
        <X :size="14" />
      </button>
    </span>
  </div>
</template>
