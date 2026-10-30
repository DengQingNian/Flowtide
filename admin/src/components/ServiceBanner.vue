<script setup lang="ts">
// 服务可用性横幅：健康检查失败时常驻提示并可重试。
import { computed } from 'vue'
import { RefreshCw } from '@lucide/vue'
import { checkService, serviceDetail, serviceStatus } from '@/stores/ui'

/** 是否显示横幅 */
const visible = computed(() => serviceStatus.value === 'degraded')
</script>

<template>
  <div v-if="visible" class="banner banner--warning" role="alert">
    <span>
      服务端健康检查未通过<template v-if="serviceDetail">（{{ serviceDetail }}）</template>。
      管理台请求可能失败，请确认服务已启动且网络可达。
    </span>
    <span class="banner__actions">
      <router-link class="icon-button" to="/system/metrics" title="查看健康检查">健康</router-link>
      <button class="icon-button" type="button" title="重试" @click="checkService">
        <RefreshCw :size="14" />
      </button>
    </span>
  </div>
</template>
