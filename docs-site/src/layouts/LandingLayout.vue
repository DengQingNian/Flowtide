<script setup lang="ts">
import SiteFooter from '@/components/SiteFooter.vue'
import SiteHeader from '@/components/SiteHeader.vue'
</script>

<template>
  <div class="app-shell">
    <SiteHeader />
    <main class="landing-main">
      <slot />
    </main>
    <SiteFooter />
  </div>
</template>
