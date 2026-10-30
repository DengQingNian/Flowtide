<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { Code2, Menu, Moon, Search, Sun, X } from '@lucide/vue'
import { navLinks } from '@/lib/navigation'
import { openPalette } from '@/lib/palette'
import { themeState, toggleTheme } from '@/lib/theme'

const route = useRoute()
const mobileOpen = ref(false)
const platform = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform) ? '⌘' : 'Ctrl'

// isActive 判断导航项是否处于当前分区。
// @param to 目标路径
// @return 是否激活
function isActive(to: string): boolean {
  // 文档与发布入口包含子页面，其余入口按完整路径匹配。
  if (to === '/docs') return route.path === '/docs' || route.path.startsWith('/docs/')
  if (to === '/releases') return route.path === '/releases' || route.path.startsWith('/docs/releases')
  return route.path === to
}

watch(
  () => route.fullPath,
  () => {
    mobileOpen.value = false
  },
)

</script>

<template>
  <header class="site-header">
    <div class="site-header__inner">
      <router-link to="/" class="brand" aria-label="Flowtide 首页">
        <span class="brand__mark" aria-hidden="true">
          <svg viewBox="0 0 32 32" fill="none">
            <path d="M6 8h20M6 16h13M6 24h20" stroke="currentColor" stroke-width="3" stroke-linecap="round" />
            <circle cx="24" cy="16" r="3.5" fill="currentColor" />
          </svg>
        </span>
        <span class="brand__text">
          <strong>Flowtide</strong>
          <span>项目文档</span>
        </span>
      </router-link>

      <nav class="top-nav" aria-label="主导航">
        <router-link v-for="link in navLinks" :key="link.to" :to="link.to" :class="{ active: isActive(link.to) }">
          {{ link.label }}
        </router-link>
      </nav>

      <div class="header-actions">
        <button class="search-trigger" type="button" aria-label="搜索文档" @click="openPalette">
          <Search :size="15" />
          <span>搜索文档</span>
          <kbd>{{ platform }} K</kbd>
        </button>
        <button class="icon-button" type="button" :aria-label="themeState.isDark.value ? '切换亮色主题' : '切换暗色主题'" @click="toggleTheme">
          <Sun v-if="themeState.isDark.value" :size="18" />
          <Moon v-else :size="18" />
        </button>
        <a class="icon-button repo-link" href="https://github.com/DengQingNian/Flowtide" target="_blank" rel="noopener" aria-label="源码仓库">
          <Code2 :size="18" />
        </a>
        <button class="icon-button mobile-only" type="button" aria-label="打开导航" :aria-expanded="mobileOpen" aria-controls="mobile-nav" @click="mobileOpen = !mobileOpen">
          <X v-if="mobileOpen" :size="20" />
          <Menu v-else :size="20" />
        </button>
      </div>
    </div>

    <div v-if="mobileOpen" id="mobile-nav" class="mobile-nav">
      <router-link v-for="link in navLinks" :key="link.to" :to="link.to" :class="{ active: isActive(link.to) }">
        {{ link.label }}
      </router-link>
      <button class="mobile-search" type="button" @click="openPalette">
        <Search :size="16" /> 搜索文档
      </button>
    </div>
  </header>
</template>
