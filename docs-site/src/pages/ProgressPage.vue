<script setup lang="ts">
import { computed } from 'vue'
import {
  ArrowRight,
  CheckCircle2,
  CircleDashed,
  Clock3,
  Database,
  GitCommitHorizontal,
  ShieldAlert,
  TriangleAlert,
} from '@lucide/vue'
import factsRaw from '@/generated/facts.json'
import { milestones } from '@/lib/milestones'

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
  commit: { short: string; date: string; count: number; branch: string; subject: string }
  stats: { goFiles: number; goLines: number; tests: number; tables: number; docsFiles: number }
  coverage: { percent: number; covered: number; total: number }
  rest: { total: number; officialCount: number; extensionCount: number }
  capabilities: { rows: CapabilityRow[]; levels: Record<string, number> }
  recentCommits: { hash: string; date: string; subject: string }[]
  status: { phase: string; migration: string; deployment: string; javaBaseline: string; schemaVersion: string }
}

const facts = factsRaw as Facts
const number = new Intl.NumberFormat('zh-CN')

const metrics = computed(() => [
  { label: 'Git 提交', value: number.format(facts.commit.count), note: facts.commit.branch, tone: 'neutral' },
  { label: 'Go 代码行', value: number.format(facts.stats.goLines), note: `${facts.stats.goFiles} 个文件`, tone: 'neutral' },
  { label: '语句覆盖率', value: `${facts.coverage.percent}%`, note: `${number.format(facts.coverage.covered)} / ${number.format(facts.coverage.total)}`, tone: 'good' },
  { label: 'REST 方法端点', value: String(facts.rest.total), note: `官方 ${facts.rest.officialCount} + 扩展 ${facts.rest.extensionCount}`, tone: 'warn' },
  { label: 'ACT 表', value: String(facts.stats.tables), note: `Schema ${facts.status.schemaVersion}`, tone: 'good' },
  { label: '测试/基准', value: number.format(facts.stats.tests), note: '函数声明数', tone: 'neutral' },
])

const stages = milestones
const partialStages = computed(() => stages.filter((stage) => stage.state === 'partial').length)
const pendingStages = computed(() => stages.filter((stage) => stage.state === 'todo').length)

const levelMeta: Record<string, { label: string; tone: string }> = {
  verified: { label: '已验证', tone: 'good' },
  prototype: { label: '原型可用', tone: 'warn' },
  partial: { label: '部分支持', tone: 'warn' },
  conditional: { label: '有条件试用', tone: 'warn' },
  restricted: { label: '受限/禁止', tone: 'bad' },
}

const distribution = computed(() => {
  const total = facts.capabilities.rows.length
  return Object.entries(facts.capabilities.levels)
    .sort((a, b) => b[1] - a[1])
    .map(([level, count]) => ({
      level,
      count,
      percent: Math.round((count / total) * 100),
      ...(levelMeta[level] ?? { label: level, tone: 'neutral' }),
    }))
})

const migrationStates = [
  { name: '新建库原型', status: '双库本地验收通过', tone: 'good', detail: '单执行者、新建 Schema、新流程' },
  { name: '只读预检', status: '保守子集已实现', tone: 'warn', detail: '无阻断不代表可转换或可切换' },
  { name: '停写切换续跑', status: '临时库演练通过，生产待验收', tone: 'bad', detail: '双库黄金样本转换、核对、行模式续跑与重启通过' },
  { name: '可回退迁移', status: '未完成', tone: 'bad', detail: '源快照还原通过；反向转换与 Java 增量接管未完成' },
]

const gaps = [
  { title: 'Java 行为对照', impact: '客户端可能遇到响应、分页、错误码差异', next: '请求/响应回放与逐字段契约测试' },
  { title: '停写切换与回退', impact: '存量库无法安全接管', next: '真实样本上的备份、转换、核对与回退演练' },
  { title: '集群与故障转移', impact: '只能单节点，故障需人工恢复', next: '作业租约、抢占、幂等与恢复' },
  { title: '查询下推不完整', impact: '数据量增长后内存扫描变慢', next: '实例/历史/多租户下推与索引' },
  { title: '异步集群协调', impact: '持久变量作业已通过单执行者验收', next: '跨节点租约、抢占与故障转移' },
]
</script>

<template>
  <div class="progress-page">
    <header class="dash-hero">
      <div class="dash-hero__badge"><Clock3 :size="14" /> 构建时更新 · 截至提交 {{ facts.commit.short }}</div>
      <h1>重构进度看板</h1>
      <p>
        这里记录当前提交的实现和验收情况。代码规模、覆盖率、端点清单与能力矩阵在构建时从
        <code>coverage.out</code>、<code>docs/003</code> 和 Git 历史读取。
      </p>
      <div class="dash-hero__meta">
        <span><GitCommitHorizontal :size="14" /> {{ facts.commit.subject }}</span>
        <span>{{ facts.commit.date }}</span>
        <span>{{ facts.status.phase }}</span>
      </div>
    </header>

    <section id="snapshot" class="dash-section">
      <div class="dash-section__head">
        <h2>当前快照</h2>
        <router-link to="/docs/progress/summary">阅读总览 <ArrowRight :size="14" /></router-link>
      </div>
      <div class="metric-grid">
        <div v-for="metric in metrics" :key="metric.label" class="metric-card" :class="`metric-card--${metric.tone}`">
          <span class="metric-card__label">{{ metric.label }}</span>
          <strong>{{ metric.value }}</strong>
          <span class="metric-card__note">{{ metric.note }}</span>
        </div>
      </div>
    </section>

    <section id="stages" class="dash-section">
      <div class="dash-section__head">
        <h2>阶段进展</h2>
        <span class="muted">{{ stages.length }} 个阶段 · {{ partialStages }} 个部分完成 · {{ pendingStages }} 个未完成</span>
      </div>
      <div class="stage-grid">
        <div v-for="stage in stages" :key="stage.id" class="stage-card" :class="`stage-card--${stage.state}`">
          <div class="stage-card__top">
            <span class="stage-card__id">{{ stage.id }}</span>
            <CheckCircle2 v-if="stage.state === 'done'" :size="16" />
            <CircleDashed v-else-if="stage.state === 'partial'" :size="16" />
            <TriangleAlert v-else :size="16" />
          </div>
          <h3>{{ stage.title }}</h3>
          <p>{{ stage.detail }}</p>
          <span class="stage-card__state">{{ stage.state === 'done' ? '完成' : stage.state === 'partial' ? '部分完成' : '未完成' }}</span>
        </div>
      </div>
    </section>

    <section id="capabilities" class="dash-section">
      <div class="dash-section__head">
        <h2>能力分布</h2>
        <router-link to="/compatibility">完整矩阵 <ArrowRight :size="14" /></router-link>
      </div>
      <div class="dist-grid">
        <div class="dist-bars">
          <div v-for="item in distribution" :key="item.level" class="dist-row">
            <span class="dist-row__label">{{ item.label }}</span>
            <div class="dist-row__track"><span class="dist-row__fill" :class="`tone-${item.tone}`" :style="{ width: item.percent + '%' }" /></div>
            <span class="dist-row__value">{{ item.count }}</span>
          </div>
        </div>
        <div class="dist-note">
          <Database :size="20" />
          <h3>{{ facts.capabilities.rows.length }} 项能力的验收结果</h3>
          <p>每项能力分别记录代码与单测、数据库、Java 对照和发布状态。图中按发布状态统计条目数，具体限制可在能力矩阵中查看。</p>
          <router-link to="/docs/compatibility/known-issues" class="text-link">查看已知问题 <ArrowRight :size="14" /></router-link>
        </div>
      </div>
    </section>

    <section id="migration" class="dash-section">
      <div class="dash-section__head">
        <h2>四种迁移状态</h2>
        <router-link to="/docs/migration">迁移手册 <ArrowRight :size="14" /></router-link>
      </div>
      <div class="migration-grid">
        <div v-for="state in migrationStates" :key="state.name" class="migration-card" :class="`migration-card--${state.tone}`">
          <div class="migration-card__head">
            <h3>{{ state.name }}</h3>
            <span class="migration-card__dot" />
          </div>
          <strong>{{ state.status }}</strong>
          <p>{{ state.detail }}</p>
        </div>
      </div>
      <div class="inline-warning">
        <ShieldAlert :size="17" />
        <span>切换前仍需完成备份、数据核对和回退演练。预检退出码 0、转换成功或静默检查 stable=true 只能确认各自检查的结果。</span>
      </div>
    </section>

    <section id="commits" class="dash-section">
      <div class="dash-section__head">
        <h2>最近提交</h2>
        <router-link to="/releases">版本与变更 <ArrowRight :size="14" /></router-link>
      </div>
      <div class="commit-list commit-list--dash">
        <div v-for="commit in facts.recentCommits.slice(0, 8)" :key="commit.hash" class="commit-row">
          <code class="commit-row__hash">{{ commit.hash }}</code>
          <span class="commit-row__subject">{{ commit.subject }}</span>
          <span class="commit-row__date">{{ commit.date }}</span>
        </div>
      </div>
    </section>

    <section id="gaps" class="dash-section">
      <div class="dash-section__head">
        <h2>下一步优先级</h2>
        <router-link to="/docs/progress/gaps">差距全表 <ArrowRight :size="14" /></router-link>
      </div>
      <div class="gap-table">
        <div class="gap-row gap-row--head">
          <span>差距</span><span>影响</span><span>下一步</span>
        </div>
        <div v-for="gap in gaps" :key="gap.title" class="gap-row">
          <span class="gap-row__title">{{ gap.title }}</span>
          <span>{{ gap.impact }}</span>
          <span>{{ gap.next }}</span>
        </div>
      </div>
      <div class="dash-callout">
        <TriangleAlert :size="18" />
        <div>
          <strong>当前版本不适合直接替换生产 Java Flowable。</strong>
          <p>如果场景要求多节点、高可用、完整 Java 委托兼容或可回退迁移，请先完成对应验收或继续使用 Java 版本。</p>
        </div>
        <router-link to="/compatibility" class="button button--dark">核对能力矩阵</router-link>
      </div>
    </section>
  </div>
</template>
