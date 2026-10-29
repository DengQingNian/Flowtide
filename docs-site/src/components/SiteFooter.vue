<script setup lang="ts">
import { computed } from 'vue'
import { content } from '@/lib/content'

const commit = computed(() => content.commit)
</script>

<template>
  <footer class="site-footer">
    <div class="site-footer__inner">
      <div class="footer-brand">
        <div class="footer-brand__title">
          <span class="brand__mark brand__mark--small" aria-hidden="true">
            <svg viewBox="0 0 32 32" fill="none">
              <path d="M6 8h20M6 16h13M6 24h20" stroke="currentColor" stroke-width="3" stroke-linecap="round" />
              <circle cx="24" cy="16" r="3.5" fill="currentColor" />
            </svg>
          </span>
          Flowable-go
        </div>
        <p>用纯 Go 重构 Flowable BPMN 引擎。当前为新建库原型，生产使用前请阅读能力矩阵与已知问题。</p>
        <div class="footer-meta">
          <span>数据截至 <code>{{ commit }}</code></span>
          <span>Apache-2.0 License</span>
        </div>
      </div>
      <div class="footer-links">
        <div class="footer-column">
          <h3>文档</h3>
          <router-link to="/docs/getting-started/installation">安装</router-link>
          <router-link to="/docs/getting-started/quickstart-rest">快速开始</router-link>
          <router-link to="/docs/sdk">Go SDK</router-link>
          <router-link to="/docs/rest">REST API</router-link>
        </div>
        <div class="footer-column">
          <h3>判断边界</h3>
          <router-link to="/compatibility">能力矩阵</router-link>
          <router-link to="/docs/compatibility/known-issues">已知问题</router-link>
          <router-link to="/docs/migration/limits">迁移限制</router-link>
          <router-link to="/progress">当前进度</router-link>
        </div>
        <div class="footer-column">
          <h3>参考</h3>
          <router-link to="/docs/reference/config">配置</router-link>
          <router-link to="/docs/reference/errors">错误码</router-link>
          <router-link to="/docs/reference/glossary">术语表</router-link>
          <router-link to="/releases">版本与变更</router-link>
        </div>
      </div>
    </div>
    <div class="footer-bottom">
      <span>© 2026 Flowable-go 项目</span>
      <span>参考基线 Flowable 8.1.0 · Java 提交 f6017e3</span>
      <a href="https://git.dengqn.com/dqn/flowable-engine-go" target="_blank" rel="noopener">源码仓库</a>
    </div>
  </footer>
</template>
