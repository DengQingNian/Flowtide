<script setup lang="ts">
// 指标页：解析 Prometheus 文本为分组表格，支持搜索与原文切换。
import { computed, onMounted, ref } from 'vue'
import { NButton, NInput, NSpin } from 'naive-ui'
import { RefreshCw } from '@lucide/vue'
import PageHeader from '@/components/PageHeader.vue'
import ErrorBlock from '@/components/ErrorBlock.vue'
import EmptyState from '@/components/EmptyState.vue'
import StatCard from '@/components/StatCard.vue'
import { request } from '@/api/client'
import { formatNumber } from '@/lib/format'
import { groupMetrics, metricLabelText, parseMetrics, sumByPrefix } from '@/lib/prometheus'
import type { MetricSample } from '@/lib/prometheus'

const text = ref('')
const samples = ref<MetricSample[]>([])
const loading = ref(false)
const error = ref<unknown>(null)
const rawMode = ref(false)
const search = ref('')

// 读取指标文本。
// @return 无
async function load(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    const content = await request<string>('/metrics')
    text.value = content
    samples.value = parseMetrics(content)
  } catch (cause) {
    error.value = cause
    text.value = ''
    samples.value = []
  } finally {
    loading.value = false
  }
}

onMounted(() => void load())

/** 过滤后的样本 */
const filtered = computed(() => {
  const keyword = search.value.trim().toLowerCase()
  if (!keyword) return samples.value
  return samples.value.filter((sample) => sample.name.toLowerCase().includes(keyword) || JSON.stringify(sample.labels).toLowerCase().includes(keyword))
})
/** 分组 */
const groups = computed(() => groupMetrics(filtered.value))
/** 解析失败但存在文本 */
const parseFailed = computed(() => text.value.trim().length > 0 && samples.value.length === 0)
/** 摘要 */
const summary = computed(() => {
  const items = [
    { name: 'flowable_deployments', label: '部署' },
    { name: 'flowable_process_definitions', label: '流程定义' },
    { name: 'flowable_process_instances_active', label: '运行实例' },
    { name: 'flowable_tasks_active', label: '活动任务' },
    { name: 'flowable_jobs_active', label: '活动作业' },
    { name: 'flowable_deadletter_jobs', label: '死信作业' },
    { name: 'flowable_historic_process_instances', label: '历史实例' },
    { name: 'flowable_http_requests_total', label: 'HTTP 请求' },
  ]
  return items.map((item) => ({ ...item, value: sumByPrefix(samples.value, item.name) }))
})
</script>

<template>
  <div>
    <PageHeader title="指标" description="Prometheus 文本指标解析与原始内容查看。">
      <template #actions>
        <n-button size="small" :loading="loading" @click="load">
          <template #icon><RefreshCw :size="14" /></template>
          刷新
        </n-button>
      </template>
    </PageHeader>

    <ErrorBlock v-if="error" :error="error" context="GET /metrics" @retry="load" />

    <n-spin :show="loading">
      <section class="stat-grid" style="margin-bottom: 14px">
        <StatCard v-for="item in summary" :key="item.name" :label="item.label" :value="formatNumber(item.value)" :hint="item.name" />
      </section>

      <section class="panel">
        <div class="panel__header">
          <span class="panel__title">指标数据</span>
          <span class="muted">{{ groups.length }} 组 / {{ filtered.length }} 条</span>
          <span class="row" style="margin-left: auto">
            <n-input v-model:value="search" size="small" clearable placeholder="按指标名或标签过滤" style="width: 240px" />
            <n-button size="small" quaternary @click="rawMode = !rawMode">{{ rawMode ? '分组表格' : '原始文本' }}</n-button>
          </span>
        </div>
        <div class="panel__body">
          <div v-if="parseFailed" class="banner banner--warning" style="margin: 0 0 10px">
            指标文本无法解析为样本，已回退显示原始内容。
          </div>

          <pre v-if="rawMode || parseFailed" class="code-block" style="max-height: 560px">{{ text }}</pre>

          <div v-else-if="groups.length === 0" class="panel__body">
            <EmptyState title="暂无指标" description="服务端未启用指标，或过滤条件没有匹配项。" />
          </div>

          <div v-else style="max-height: 560px; overflow: auto">
            <table class="kv-table">
              <thead><tr><th>指标</th><th>标签</th><th style="text-align: right">值</th></tr></thead>
              <tbody>
                <template v-for="group in groups" :key="group.name">
                  <tr v-for="(sample, index) in group.samples" :key="`${group.name}-${index}`">
                    <td class="mono">{{ index === 0 ? group.name : '' }}</td>
                    <td class="mono muted">{{ metricLabelText(sample.labels) }}</td>
                    <td class="mono" style="text-align: right">{{ sample.value }}</td>
                  </tr>
                </template>
              </tbody>
            </table>
          </div>
        </div>
      </section>
    </n-spin>
  </div>
</template>