<script setup lang="ts">
import { Compass, Home, Search } from '@lucide/vue'
import { openPalette } from '@/lib/palette'
</script>

<template>
  <div class="not-found">
    <div class="not-found__code">404</div>
    <h1>这个页面不存在</h1>
    <p>它可能还没写，或者链接已经调整。可以返回文档首页，或直接搜索关键词。</p>
    <div class="not-found__actions">
      <router-link to="/docs" class="button button--primary"><Home :size="16" /> 文档首页</router-link>
      <button type="button" class="button" @click="openPalette"><Search :size="16" /> 搜索文档</button>
      <router-link to="/progress" class="button"><Compass :size="16" /> 当前进度</router-link>
    </div>
  </div>
</template>
