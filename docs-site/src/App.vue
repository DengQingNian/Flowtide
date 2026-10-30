<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { NConfigProvider, NMessageProvider, darkTheme, dateZhCN, zhCN } from 'naive-ui'
import LandingLayout from '@/layouts/LandingLayout.vue'
import DocsLayout from '@/layouts/DocsLayout.vue'
import SearchPalette from '@/components/SearchPalette.vue'
import { themeState } from '@/lib/theme'

const route = useRoute()
const layout = computed(() => (route.meta.layout === 'landing' ? LandingLayout : DocsLayout))

const themeOverrides = {
  common: {
    primaryColor: '#62605a',
    primaryColorHover: '#78746c',
    primaryColorPressed: '#37352f',
    primaryColorSuppl: '#62605a',
    borderRadius: '6px',
    fontFamily:
      '-apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif',
    fontFamilyMono: '"JetBrains Mono", "SFMono-Regular", Consolas, "Liberation Mono", monospace',
  },
}
</script>

<template>
  <n-config-provider
    :theme="themeState.isDark.value ? darkTheme : null"
    :theme-overrides="themeOverrides"
    :locale="zhCN"
    :date-locale="dateZhCN"
  >
    <n-message-provider>
      <component :is="layout">
        <router-view />
      </component>
      <SearchPalette />
    </n-message-provider>
  </n-config-provider>
</template>
