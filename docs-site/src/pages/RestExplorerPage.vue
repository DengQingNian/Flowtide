<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import { ChevronDown, ChevronLeft, ChevronRight, Copy, Filter, Search, X } from '@lucide/vue'
import factsRaw from '@/generated/facts.json'

interface Endpoint {
  id: number
  method: string
  path: string
  officialPath: string
  implPath: string
  status: string
  official: boolean
  extension: boolean
  domain: string
}

interface Facts {
  commit: { short: string }
  rest: {
    total: number
    officialCount: number
    extensionCount: number
    methods: Record<string, number>
    domains: Record<string, { total: number; official: number; extension: number }>
    endpoints: Endpoint[]
  }
}

const facts = factsRaw as Facts
const message = useMessage()
const query = ref('')
const method = ref('ALL')
const domain = ref('ALL')
const source = ref('all')
const page = ref(1)
const pageSize = 20

const domainLabels: Record<string, string> = {
  runtime: '运行时',
  management: '管理',
  repository: '仓库',
  identity: '身份',
  history: '历史',
  query: '查询',
  form: '表单',
}

const methods = computed(() => ['ALL', ...Object.keys(facts.rest.methods)])
const domains = computed(() => Object.keys(facts.rest.domains).sort())

const filtered = computed(() => {
  const keyword = query.value.trim().toLowerCase()
  return facts.rest.endpoints.filter((endpoint) => {
    if (method.value !== 'ALL' && endpoint.method !== method.value) return false
    if (domain.value !== 'ALL' && endpoint.domain !== domain.value) return false
    if (source.value === 'official' && !endpoint.official) return false
    if (source.value === 'extension' && !endpoint.extension) return false
    if (!keyword) return true
    return [endpoint.method, endpoint.path, endpoint.implPath, endpoint.status, endpoint.domain]
      .join(' ')
      .toLowerCase()
      .includes(keyword)
  })
})

const pageCount = computed(() => Math.max(1, Math.ceil(filtered.value.length / pageSize)))
const visible = computed(() => filtered.value.slice((page.value - 1) * pageSize, page.value * pageSize))

// reset 重置筛选条件。
// @return 无
function reset(): void {
  query.value = ''
  method.value = 'ALL'
  domain.value = 'ALL'
  source.value = 'all'
}

// copyPath 复制完整端点路径。
// @param endpoint 端点
// @return 无
async function copyPath(endpoint: Endpoint): Promise<void> {
  try {
    await navigator.clipboard.writeText(`${endpoint.method} /service${endpoint.path}`)
    message.success('端点已复制')
  } catch {
    message.error('浏览器未允许剪贴板访问')
  }
}

watch([query, method, domain, source], () => {
  page.value = 1
})
watch(pageCount, () => {
  if (page.value > pageCount.value) page.value = pageCount.value
})
</script>

<template>
  <div class="rest-page">
    <header class="dash-hero">
      <div class="dash-hero__badge">REST 浏览器 · {{ facts.rest.total }} 个方法端点</div>
      <h1>找到端点，也看到差异</h1>
      <p>
        官方 {{ facts.rest.officialCount }} 个流程 REST 方法全部有路由，另有 {{ facts.rest.extensionCount }} 个扩展端点。
        这里显示的是路由覆盖与实现状态；请求/响应模式仍需逐项 Java 对照。
      </p>
    </header>

    <section id="summary" class="dash-section">
      <div class="metric-grid metric-grid--four">
        <div class="metric-card"><span class="metric-card__label">官方端点</span><strong>{{ facts.rest.officialCount }}</strong><span class="metric-card__note">官方缺失 0</span></div>
        <div class="metric-card"><span class="metric-card__label">扩展端点</span><strong>{{ facts.rest.extensionCount }}</strong><span class="metric-card__note">不计入官方覆盖</span></div>
        <div class="metric-card"><span class="metric-card__label">方法种类</span><strong>{{ Object.keys(facts.rest.methods).length }}</strong><span class="metric-card__note">GET/POST/PUT/DELETE</span></div>
        <div class="metric-card"><span class="metric-card__label">业务域</span><strong>{{ domains.length }}</strong><span class="metric-card__note">runtime 最多</span></div>
      </div>
      <div class="matrix-note">
        <Filter :size="15" />
        <span>筛选后共 <strong>{{ filtered.length }}</strong> 个端点。点击复制可得到带 <code>/service</code> 前缀的调用路径。</span>
      </div>
    </section>

    <section id="filters" class="dash-section">
      <div class="rest-toolbar">
        <div class="field">
          <Search :size="16" />
          <input v-model="query" type="search" placeholder="搜索路径、方法、状态…" aria-label="搜索端点" />
          <button v-if="query" type="button" class="field-clear" aria-label="清空" @click="query = ''"><X :size="14" /></button>
        </div>
        <div class="field field--select">
          <select v-model="method" aria-label="按方法筛选">
            <option v-for="item in methods" :key="item" :value="item">{{ item === 'ALL' ? '全部方法' : item }}</option>
          </select>
          <ChevronDown :size="15" class="select-chevron" />
        </div>
        <div class="field field--select">
          <select v-model="domain" aria-label="按业务域筛选">
            <option value="ALL">全部业务域</option>
            <option v-for="item in domains" :key="item" :value="item">{{ domainLabels[item] ?? item }} / {{ item }}</option>
          </select>
          <ChevronDown :size="15" class="select-chevron" />
        </div>
        <div class="segmented" role="group" aria-label="端点来源">
          <button type="button" :class="{ active: source === 'all' }" @click="source = 'all'">全部</button>
          <button type="button" :class="{ active: source === 'official' }" @click="source = 'official'">官方</button>
          <button type="button" :class="{ active: source === 'extension' }" @click="source = 'extension'">扩展</button>
        </div>
        <button type="button" class="button button--small" @click="reset">重置</button>
      </div>

      <div class="endpoint-table-wrap">
        <table class="endpoint-table">
          <thead>
            <tr>
              <th>方法</th>
              <th>规范化路径</th>
              <th>业务域</th>
              <th>来源</th>
              <th>实现状态</th>
              <th />
            </tr>
          </thead>
          <tbody>
            <tr v-for="endpoint in visible" :key="endpoint.id">
              <td><span class="method-pill" :class="`method-pill--${endpoint.method.toLowerCase()}`">{{ endpoint.method }}</span></td>
              <td class="endpoint-path"><code>{{ endpoint.path }}</code></td>
              <td>{{ domainLabels[endpoint.domain] ?? endpoint.domain }}</td>
              <td><span class="source-pill" :class="endpoint.official ? 'source-pill--official' : 'source-pill--extension'">{{ endpoint.official ? '官方' : '扩展' }}</span></td>
              <td class="endpoint-status">{{ endpoint.status }}</td>
              <td><button type="button" class="copy-icon" aria-label="复制端点" @click="copyPath(endpoint)"><Copy :size="14" /></button></td>
            </tr>
            <tr v-if="visible.length === 0">
              <td colspan="6" class="matrix-empty">没有匹配的端点。试试“tasks”“deployment”“migration”或重置筛选。</td>
            </tr>
          </tbody>
        </table>
      </div>

      <div class="pagination">
        <span>第 {{ page }} / {{ pageCount }} 页 · {{ filtered.length }} 个端点</span>
        <div class="pagination__controls">
          <button type="button" :disabled="page <= 1" aria-label="上一页" @click="page -= 1"><ChevronLeft :size="15" /></button>
          <button type="button" :disabled="page >= pageCount" aria-label="下一页" @click="page += 1"><ChevronRight :size="15" /></button>
        </div>
      </div>
    </section>
  </div>
</template>
