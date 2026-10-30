<script setup lang="ts">
// 应用根组件：Naive UI 主题、语言与全局 Provider。
import { computed } from 'vue'
import {
  NConfigProvider,
  NDialogProvider,
  NLoadingBarProvider,
  NMessageProvider,
  NNotificationProvider,
  darkTheme,
  dateZhCN,
  zhCN,
} from 'naive-ui'
import AppShell from '@/components/AppShell.vue'
import { isDark } from '@/lib/theme'
import { darkOverrides, lightOverrides } from '@/lib/naiveTheme'

const theme = computed(() => (isDark.value ? darkTheme : null))
const themeOverrides = computed(() => (isDark.value ? darkOverrides : lightOverrides))
</script>

<template>
  <n-config-provider
    :theme="theme"
    :theme-overrides="themeOverrides"
    :locale="zhCN"
    :date-locale="dateZhCN"
  >
    <n-loading-bar-provider>
      <n-dialog-provider>
        <n-notification-provider>
          <n-message-provider>
            <AppShell />
          </n-message-provider>
        </n-notification-provider>
      </n-dialog-provider>
    </n-loading-bar-provider>
  </n-config-provider>
</template>
