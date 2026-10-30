<script setup lang="ts">
import { computed, ref } from 'vue'
import { ChevronDown, Download, Filter, Search, X } from '@lucide/vue'
import factsRaw from '@/generated/facts.json'

interface CapabilityRow {
  capability: string
  encoded: string
  database: string
  java: string
  limits: string
  owner: string
  release: string
  level: string
}

interface Facts {
  commit: { short: string }
  capabilities: { rows: CapabilityRow[]; levels: Record<string, number> }
}

const facts = factsRaw as Facts
const query = ref('')
const level = ref('all')
const expanded = ref<Set<string>>(new Set())

const levelMeta: Record<string, { label: string; tone: string }> = {
  verified: { label: '已验证', tone: 'good' },
  prototype: { label: '原型可用', tone: 'warn' },
  partial: { label: '部分支持', tone: 'warn' },
  conditional: { label: '有条件试用', tone: 'warn' },
  restricted: { label: '受限/禁止', tone: 'bad' },
}

const summary = computed(() => {
  const total = facts.capabilities.rows.length
  const counts = { verified: 0, prototype: 0, partial: 0, restricted: 0, conditional: 0 }
  for (const row of facts.capabilities.rows) {
    if (row.level in counts) counts[row.level as keyof typeof counts] += 1
  }
  return [
    { label: '能力条目', value: total, tone: 'neutral' },
    { label: '已验证', value: counts.verified, tone: 'good' },
    { label: '原型可用', value: counts.prototype, tone: 'warn' },
    { label: '部分支持', value: counts.partial, tone: 'warn' },
    { label: '有条件试用', value: counts.conditional, tone: 'warn' },
    { label: '受限/禁止', value: counts.restricted, tone: 'bad' },
  ]
})

const filtered = computed(() => {
  const keyword = query.value.trim().toLowerCase()
  return facts.capabilities.rows.filter((row) => {
    if (level.value !== 'all' && row.level !== level.value) return false
    if (!keyword) return true
    return [row.capability, row.encoded, row.database, row.java, row.limits, row.release, row.owner]
      .join(' ')
      .toLowerCase()
      .includes(keyword)
  })
})

// toggleRow 展开/收起一行的限制详情。
// @param capability 能力名称
// @return 无
function toggleRow(capability: string): void {
  const next = new Set(expanded.value)
  if (next.has(capability)) next.delete(capability)
  else next.add(capability)
  expanded.value = next
}

// levelLabel 返回状态标签。
// @param value 状态值
// @return 标签
function levelLabel(value: string): string {
  return levelMeta[value]?.label ?? value
}

// downloadJson 导出当前能力矩阵数据。
// @return 无
function downloadJson(): void {
  const blob = new Blob([JSON.stringify(facts.capabilities, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = 'flowtide-capabilities.json'
  anchor.click()
  URL.revokeObjectURL(url)
}
</script>

<template>
  <div class="compat-page">
    <header class="dash-hero">
      <div class="dash-hero__badge">能力矩阵 · 数据截至 {{ facts.commit.short }}</div>
      <h1>一项能力，四种证据</h1>
      <p>
        已编码/已单测、真实数据库、Java 对照、可发布，分别取证。任何一列通过都不能自动推导下一列；
        本页按发布状态分组，展开可查看限制与下一验收门槛。
      </p>
    </header>

    <section id="summary" class="dash-section">
      <div class="matrix-summary">
        <div v-for="item in summary" :key="item.label" class="matrix-summary__item" :class="`tone-${item.tone}`">
          <strong>{{ item.value }}</strong>
          <span>{{ item.label }}</span>
        </div>
        <button type="button" class="matrix-download" @click="downloadJson"><Download :size="15" /> 导出 JSON</button>
      </div>
      <div class="matrix-note">
        <Filter :size="15" />
        <span>“发布状态”是保守归类；具体限制以每行展开内容与仓库能力矩阵为准。</span>
      </div>
    </section>

    <section id="matrix" class="dash-section">
      <div class="matrix-toolbar">
        <div class="field">
          <Search :size="16" />
          <input v-model="query" type="search" placeholder="搜索能力、限制、责任职责…" aria-label="搜索能力" />
          <button v-if="query" type="button" class="field-clear" aria-label="清空" @click="query = ''"><X :size="14" /></button>
        </div>
        <div class="field field--select">
          <select v-model="level" aria-label="按发布状态筛选">
            <option value="all">全部状态</option>
            <option value="verified">已验证</option>
            <option value="prototype">原型可用</option>
            <option value="partial">部分支持</option>
            <option value="conditional">有条件试用</option>
            <option value="restricted">受限/禁止</option>
          </select>
          <ChevronDown :size="15" class="select-chevron" />
        </div>
        <span class="matrix-count">{{ filtered.length }} / {{ facts.capabilities.rows.length }}</span>
      </div>

      <div class="matrix-table-wrap">
        <table class="matrix-table">
          <thead>
            <tr>
              <th>能力与计划范围</th>
              <th>已编码/单测</th>
              <th>真实数据库</th>
              <th>Java 对照</th>
              <th>发布状态</th>
              <th />
            </tr>
          </thead>
          <tbody>
            <template v-for="row in filtered" :key="row.capability">
              <tr class="matrix-row" @click="toggleRow(row.capability)">
                <td class="matrix-capability">{{ row.capability }}</td>
                <td>{{ row.encoded }}</td>
                <td>{{ row.database }}</td>
                <td>{{ row.java }}</td>
                <td><span class="level-pill" :class="`level-pill--${row.level}`">{{ levelLabel(row.level) }}</span></td>
                <td class="matrix-expand"><ChevronDown :size="15" :class="{ rotated: !expanded.has(row.capability) }" /></td>
              </tr>
              <tr v-if="expanded.has(row.capability)" class="matrix-detail">
                <td colspan="6">
                  <div class="matrix-detail__grid">
                    <div><span>发布状态原文</span><p>{{ row.release }}</p></div>
                    <div><span>限制与下一验收门槛</span><p>{{ row.limits }}</p></div>
                    <div><span>责任职责</span><p>{{ row.owner }}</p></div>
                  </div>
                </td>
              </tr>
            </template>
            <tr v-if="filtered.length === 0">
              <td colspan="6" class="matrix-empty">没有匹配的能力项。试试“MySQL”“REST”“迁移”或清空筛选。</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <section id="legend" class="dash-section">
      <h2>状态说明</h2>
      <div class="legend-grid">
        <div class="legend-item"><span class="level-pill level-pill--verified">已验证</span><p>当前提交可复现，且有回归或数据库证据。</p></div>
        <div class="legend-item"><span class="level-pill level-pill--prototype">原型可用</span><p>主路径可运行，但只适用于单执行者/受控场景。</p></div>
        <div class="legend-item"><span class="level-pill level-pill--partial">部分支持</span><p>语义子集或边界不完整，不能按全量能力使用。</p></div>
        <div class="legend-item"><span class="level-pill level-pill--conditional">有条件试用</span><p>需要额外配置或人工控制点，权限审计未完成。</p></div>
        <div class="legend-item"><span class="level-pill level-pill--restricted">受限/禁止</span><p>存在明确阻断，或禁止多执行者/生产发布。</p></div>
      </div>
      <p class="matrix-footnote">
        完整证据与退出条件见仓库
        <a href="https://github.com/DengQingNian/Flowtide/src/branch/master/docs/003-迁移审计/003-能力矩阵与迁移退出条件.md" target="_blank" rel="noopener">003 能力矩阵与迁移退出条件</a>。
      </p>
    </section>
  </div>
</template>
