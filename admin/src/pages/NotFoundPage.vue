<script setup lang="ts">
// 404 页面：提示路由不存在并返回仪表盘。
import { useRoute } from 'vue-router'
import PageHeader from '@/components/PageHeader.vue'
import EmptyState from '@/components/EmptyState.vue'

const route = useRoute()
</script>

<template>
  <div>
    <PageHeader title="页面不存在" :description="`未找到路由 ${route.fullPath}`" />
    <EmptyState title="地址可能已失效" description="检查链接或从左侧导航重新进入。">
      <router-link to="/dashboard">返回仪表盘</router-link>
    </EmptyState>
  </div>
</template>
