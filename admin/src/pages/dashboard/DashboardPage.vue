<script setup lang="ts">
// 仪表盘：引擎信息、关键计数、最近部署/实例/死信与健康指标摘要。
import { computed, onMounted, ref } from 'vue'
import { NButton, NSpin } from 'naive-ui'
import { RefreshCw } from '@lucide/vue'
import PageHeader from '@/components/PageHeader.vue'
import StatCard from '@/components/StatCard.vue'
import StatusTag from '@/components/StatusTag.vue'
import EmptyState from '@/components/EmptyState.vue'
import ErrorBlock from '@/components/ErrorBlock.vue'
import { loadDashboard } from '@/api/dashboard'
import type { DashboardData } from '@/api/dashboard'
import { formatDateTime, formatNumber, shortId, textOrDash } from '@/lib/format'

const data = ref<DashboardData | null>(null)
const loading = ref(false)
const error = ref<unknown>(null)

// 加载仪表盘数据。
// @return 无
async function load(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    data.value = await loadDashboard()
  } catch (cause) {
    error.value = cause
  } finally {
    loading.value = false
  }
}

onMounted(() => void load())

/** 引擎信息 */
const engine = computed(() => data.value?.engine ?? null)
/** 属性表 */
const properties = computed(() => data.value?.properties ?? {})
/** 引擎状态文本 */
const engineStatus = computed(() => (engine.value ? (engine.value.closed ? '已关闭' : '运行中') : '未知'))
/** 引擎状态色调 */
const engineTone = computed(() => (engine.value ? (engine.value.closed ? 'danger' : 'success') : 'default'))
/** 存储模式 */
const storageMode = computed(() => properties.value['storage.mode'] || properties.value['storageMode'] || '—')
/** 计数卡片 */
const cards = computed(() => {
  const counts = data.value?.counts
  return [
    { label: '部署', value: counts?.deployments, to: '/repository/deployments' },
    { label: '流程定义', value: counts?.definitions, to: '/repository/definitions' },
    { label: '运行实例', value: counts?.instances, to: '/runtime/instances' },
    { label: '待办任务', value: counts?.tasks, to: '/runtime/tasks' },
    { label: '死信作业', value: counts?.deadletters, to: '/management/jobs/deadletter' },
  ]
})
/** 关键指标 */
const metrics = computed(() => data.value?.health.metrics ?? {})
/** 指标项 */
const metricItems = computed(() =>
  [
    ['flowable_deployments', '部署'],
    ['flowable_process_definitions', '定义'],
    ['flowable_process_instances_active', '运行实例'],
    ['flowable_tasks_active', '活动任务'],
    ['flowable_jobs_active', '活动作业'],
    ['flowable_deadletter_jobs', '死信'],
    ['flowable_historic_process_instances', '历史实例'],
  ]
    .filter(([key]) => metrics.value[key] !== undefined)
    .map(([key, label]) => ({ key, label, value: metrics.value[key] })),
)

// 格式化计数。
// @param value 计数
// @return 展示文本
function countText(value: number | null | undefined): string {
  return value === null || value === undefined ? '—' : formatNumber(value)
}
</script>

<template>
  <div>
    <PageHeader title="仪表盘" description="引擎状态、关键计数与最近动态。">
      <template #actions>
        <n-button size="small" :loading="loading" @click="load">
          <template #icon><RefreshCw :size="14" /></template>
          刷新
        </n-button>
      </template>
    </PageHeader>

    <ErrorBlock v-if="error" :error="error" context="GET /service/management/engine" @retry="load" />
    <div v-for="message in data?.errors ?? []" :key="message" class="banner banner--warning" role="alert">{{ message }}</div>

    <n-spin :show="loading">
      <div class="stack">
        <section class="panel">
          <div class="panel__header">
            <span class="panel__title">引擎</span>
            <StatusTag :value="engineStatus" :tone="engineTone" />
            <span class="muted" style="margin-left: auto; font-size: 12px">{{ engine?.name || 'Flowtide' }}</span>
          </div>
          <div class="panel__body">
            <div class="detail-grid">
              <div class="detail-item"><span class="detail-item__label">版本</span><span class="detail-item__value">{{ textOrDash(engine?.version) }}</span></div>
              <div class="detail-item"><span class="detail-item__label">Schema 版本</span><span class="detail-item__value">{{ textOrDash(properties['schema.version']) }}</span></div>
              <div class="detail-item"><span class="detail-item__label">租户</span><span class="detail-item__value">{{ textOrDash(properties['tenantId']) }}</span></div>
              <div class="detail-item"><span class="detail-item__label">历史级别</span><span class="detail-item__value">{{ textOrDash(engine?.historyLevel) }}</span></div>
              <div class="detail-item"><span class="detail-item__label">存储模式</span><span class="detail-item__value">{{ storageMode }}</span></div>
              <div class="detail-item"><span class="detail-item__label">健康检查</span><span class="detail-item__value">healthz {{ data?.health.health || '—' }} · readyz {{ data?.health.ready || '—' }}</span></div>
            </div>
          </div>
        </section>

        <section class="stat-grid">
          <StatCard v-for="card in cards" :key="card.label" :label="card.label" :value="countText(card.value)" :to="card.to" hint="点击查看列表" />
        </section>

        <div class="two-col">
          <section class="panel">
            <div class="panel__header"><span class="panel__title">最近部署</span><router-link to="/repository/deployments" class="muted">全部</router-link></div>
            <div class="panel__body">
              <table v-if="data?.deployments.length" class="kv-table">
                <thead><tr><th>名称</th><th>租户</th><th>时间</th></tr></thead>
                <tbody>
                  <tr v-for="item in data.deployments" :key="item.id">
                    <td><router-link :to="`/repository/deployments/${item.id}`">{{ textOrDash(item.name || shortId(item.id)) }}</router-link></td>
                    <td>{{ textOrDash(item.tenantId) }}</td>
                    <td>{{ formatDateTime(item.deployTime) }}</td>
                  </tr>
                </tbody>
              </table>
              <EmptyState v-else title="暂无部署" description="从部署页上传第一个 BPMN 文件。" />
            </div>
          </section>

          <section class="panel">
            <div class="panel__header"><span class="panel__title">最近启动实例</span><router-link to="/runtime/instances" class="muted">全部</router-link></div>
            <div class="panel__body">
              <table v-if="data?.instances.length" class="kv-table">
                <thead><tr><th>实例</th><th>定义</th><th>开始时间</th></tr></thead>
                <tbody>
                  <tr v-for="item in data.instances" :key="item.id">
                    <td><router-link :to="`/runtime/instances/${item.id}`" class="mono">{{ shortId(item.id) }}</router-link></td>
                    <td>{{ textOrDash(item.processDefinitionName || item.processDefinitionId) }}</td>
                    <td>{{ formatDateTime(item.startTime) }}</td>
                  </tr>
                </tbody>
              </table>
              <EmptyState v-else title="暂无运行实例" description="从流程定义页启动一个实例。" />
            </div>
          </section>
        </div>

        <div class="two-col">
          <section class="panel">
            <div class="panel__header"><span class="panel__title">死信作业</span><router-link to="/management/jobs/deadletter" class="muted">全部</router-link></div>
            <div class="panel__body">
              <table v-if="data?.deadletters.length" class="kv-table">
                <thead><tr><th>作业</th><th>执行</th><th>异常</th></tr></thead>
                <tbody>
                  <tr v-for="item in data.deadletters" :key="item.id">
                    <td><router-link :to="`/management/jobs/deadletter/${item.id}`" class="mono">{{ shortId(item.id) }}</router-link></td>
                    <td class="mono">{{ shortId(item.executionId) }}</td>
                    <td class="ellipsis" :title="item.exceptionMessage">{{ textOrDash(item.exceptionMessage) }}</td>
                  </tr>
                </tbody>
              </table>
              <EmptyState v-else title="没有死信作业" description="失败作业会进入死信桶，便于集中排障。" />
            </div>
          </section>

          <section class="panel">
            <div class="panel__header"><span class="panel__title">关键指标</span><router-link to="/system/metrics" class="muted">指标页</router-link></div>
            <div class="panel__body">
              <div v-if="metricItems.length" class="detail-grid">
                <div v-for="item in metricItems" :key="item.key" class="detail-item">
                  <span class="detail-item__label">{{ item.label }}</span>
                  <span class="detail-item__value">{{ formatNumber(item.value) }}</span>
                </div>
              </div>
              <EmptyState v-else title="指标不可用" description="服务端未启用指标或 /metrics 读取失败。" />
            </div>
          </section>
        </div>
      </div>
    </n-spin>
  </div>
</template>

<style scoped>
.two-col { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px; }
@media (max-width: 1100px) { .two-col { grid-template-columns: 1fr; } }
</style>