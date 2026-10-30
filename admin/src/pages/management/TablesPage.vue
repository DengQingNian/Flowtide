<script setup lang="ts">
// 表清单：按 ACT_* 前缀分组展示表名与行数，进入表数据浏览页。
import { computed, onMounted, ref } from 'vue'
import { NButton, NSpin } from 'naive-ui'
import { RefreshCw } from '@lucide/vue'
import PageHeader from '@/components/PageHeader.vue'
import ErrorBlock from '@/components/ErrorBlock.vue'
import EmptyState from '@/components/EmptyState.vue'
import { markAuthError } from '@/stores/ui'
import { listTables } from '@/api/management'
import type { TableInfo } from '@/api/management'
import { groupTables } from './components/managementUtils'
import { formatNumber } from '@/lib/format'

/** 表清单 */
const tables = ref<TableInfo[]>([])
/** 加载态 */
const loading = ref(false)
/** 错误 */
const error = ref<unknown>(null)

/** 非空分组 */
const groups = computed(() => groupTables(tables.value).filter((group) => group.tables.length > 0))
/** 表总数 */
const tableCount = computed(() => tables.value.length)
/** 行数合计 */
const rowCount = computed(() => tables.value.reduce((sum, table) => sum + table.count, 0))

// 加载表清单。
// @return 无
async function load(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    tables.value = await listTables()
  } catch (cause) {
    error.value = cause
    markAuthError(cause)
  } finally {
    loading.value = false
  }
}

onMounted(() => void load())
</script>

<template>
  <div>
    <PageHeader title="表数据" :description="`共 ${formatNumber(tableCount)} 张 ACT_* 表、${formatNumber(rowCount)} 行数据，按仓库/运行/历史/身份分组。`">
      <template #actions>
        <n-button size="small" :loading="loading" @click="load">
          <template #icon><RefreshCw :size="14" /></template>
          刷新
        </n-button>
      </template>
    </PageHeader>

    <ErrorBlock v-if="error" :error="error" context="GET /service/management/tables" @retry="load" />

    <n-spin :show="loading">
      <div v-if="groups.length" class="stack">
        <section v-for="group in groups" :key="group.key" class="panel">
          <div class="panel__header">
            <span class="panel__title">{{ group.label }}</span>
            <span class="muted" style="font-size: 12px">{{ group.tables.length }} 张表 · {{ formatNumber(group.totalCount) }} 行</span>
          </div>
          <div class="panel__body" style="padding: 0">
            <table class="kv-table">
              <thead>
                <tr>
                  <th>表名</th>
                  <th style="width: 140px; text-align: right">行数</th>
                  <th style="width: 80px"></th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="table in group.tables" :key="table.name">
                  <td>
                    <router-link :to="`/management/tables/${table.name}`" class="mono">{{ table.name }}</router-link>
                  </td>
                  <td style="text-align: right">{{ formatNumber(table.count) }}</td>
                  <td style="text-align: right">
                    <router-link :to="`/management/tables/${table.name}`">浏览</router-link>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>
      </div>
      <EmptyState v-else-if="!error" title="没有可展示的表" description="引擎尚未初始化表结构，或接口返回空数组。" />
    </n-spin>
  </div>
</template>
