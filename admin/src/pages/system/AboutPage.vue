<script setup lang="ts">
// 关于页：构建版本、提交号、构建时间与服务端引擎信息。
import { onMounted, ref } from 'vue'
import PageHeader from '@/components/PageHeader.vue'
import ErrorBlock from '@/components/ErrorBlock.vue'
import { getJSON } from '@/api/client'
import type { EngineInfo } from '@/api/dashboard'
import { formatDateTime, textOrDash } from '@/lib/format'

const engine = ref<EngineInfo | null>(null)
const error = ref<unknown>(null)

/** 前端版本 */
const version = __ADMIN_VERSION__
/** 提交号 */
const commit = __ADMIN_COMMIT__
/** 构建时间 */
const buildTime = __ADMIN_BUILD_TIME__

// 读取服务端引擎信息。
// @return 无
async function load(): Promise<void> {
  error.value = null
  try {
    engine.value = await getJSON<EngineInfo>('/service/management/engine')
  } catch (cause) {
    error.value = cause
  }
}

onMounted(() => void load())
</script>

<template>
  <div>
    <PageHeader title="关于" description="管理台与服务端的构建信息。" />
    <ErrorBlock v-if="error" :error="error" context="GET /service/management/engine" @retry="load" />

    <section class="panel">
      <div class="panel__header"><span class="panel__title">管理台</span></div>
      <div class="panel__body">
        <div class="detail-grid">
          <div class="detail-item"><span class="detail-item__label">前端版本</span><span class="detail-item__value">{{ version }}</span></div>
          <div class="detail-item"><span class="detail-item__label">提交号</span><span class="detail-item__value mono">{{ commit }}</span></div>
          <div class="detail-item"><span class="detail-item__label">构建时间</span><span class="detail-item__value">{{ formatDateTime(buildTime) }}</span></div>
          <div class="detail-item"><span class="detail-item__label">路由模式</span><span class="detail-item__value">Hash · /adminpage/#/</span></div>
        </div>
      </div>
    </section>

    <section class="panel">
      <div class="panel__header"><span class="panel__title">服务端</span></div>
      <div class="panel__body">
        <div class="detail-grid">
          <div class="detail-item"><span class="detail-item__label">引擎</span><span class="detail-item__value">{{ textOrDash(engine?.name) }}</span></div>
          <div class="detail-item"><span class="detail-item__label">版本</span><span class="detail-item__value">{{ textOrDash(engine?.version) }}</span></div>
          <div class="detail-item"><span class="detail-item__label">历史级别</span><span class="detail-item__value">{{ textOrDash(engine?.historyLevel) }}</span></div>
          <div class="detail-item"><span class="detail-item__label">状态</span><span class="detail-item__value">{{ engine?.closed ? '已关闭' : '运行中' }}</span></div>
        </div>
      </div>
    </section>

    <section class="panel">
      <div class="panel__header"><span class="panel__title">说明</span></div>
      <div class="panel__body muted">
        <p>管理台是 Flowtide 单二进制内嵌的静态界面，全部数据通过同源 REST 读取。</p>
        <p>本期不提供登录、会话与权限控制；服务端启用认证时，数据请求会返回 401 并给出说明。</p>
      </div>
    </section>
  </div>
</template>
