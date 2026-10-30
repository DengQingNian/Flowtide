<script setup lang="ts">
import { computed, ref } from 'vue'
import { useMessage } from 'naive-ui'
import {
  ArrowRight,
  Boxes,
  Check,
  CircleDot,
  Code2,
  Copy,
  Database,
  Gauge,
  GitBranch,
  GitCommitHorizontal,
  Layers,
  Route,
  ShieldCheck,
  BookOpen,
  FileText,
  Terminal,
  Workflow,
} from '@lucide/vue'
import factsRaw from '@/generated/facts.json'
import { milestones } from '@/lib/milestones'

interface Facts {
  version: string
  commit: { short: string; date: string; count: number; subject: string }
  stats: { goFiles: number; goLines: number; tests: number; tables: number; binaries: number; docsFiles: number }
  coverage: { percent: number; covered: number; total: number }
  rest: { total: number; officialCount: number; extensionCount: number; domains: Record<string, { total: number }> }
  recentCommits: { hash: string; date: string; subject: string }[]
  status: { phase: string; migration: string; deployment: string }
}

const facts = factsRaw as Facts
const message = useMessage()
const tab = ref<'rest' | 'go' | 'mysql'>('rest')
const number = new Intl.NumberFormat('zh-CN')

const stats = computed(() => [
  { value: number.format(facts.commit.count), label: 'Git 提交', href: '/progress' },
  { value: number.format(facts.stats.goLines), label: 'Go 代码行', href: '/progress' },
  { value: String(facts.rest.total), label: 'REST 方法', href: '/rest-explorer' },
  { value: `${facts.coverage.percent}%`, label: '语句覆盖率', href: '/progress/quality' },
  { value: String(facts.stats.tables), label: 'ACT 表', href: '/progress/persistence' },
  { value: number.format(facts.stats.tests), label: '测试/基准', href: '/progress/quality' },
])

const phases = milestones

const features = [
  {
    icon: Workflow,
    title: 'BPMN 执行内核',
    text: '执行树、Command + Agenda、用户/服务任务、网关、子流程、多实例、事件、补偿与迁移。',
    status: '原型可用',
    href: '/docs/progress/kernel',
  },
  {
    icon: Database,
    title: '复用 ACT_* Schema',
    text: '41 张 Flowable 8.1 表，MySQL 8.0 与 PostgreSQL 16 双库验收，行级写入与启动恢复。',
    status: '单执行者原型',
    href: '/docs/progress/persistence',
  },
  {
    icon: Route,
    title: '官方 REST 路由',
    text: '官方 184 个流程 REST 方法全部有路由，另有 58 个扩展端点；行为兼容仍待逐项验收。',
    status: '路由完成',
    href: '/rest-explorer',
  },
  {
    icon: ShieldCheck,
    title: '认证与隔离',
    text: 'Basic、静态 Bearer、JWT/JWKS、mTLS 证书身份、RBAC、租户绑定与限流。',
    status: '有条件试用',
    href: '/docs/rest/auth',
  },
  {
    icon: GitBranch,
    title: 'Java 迁移工具',
    text: '只读预检、简单执行树离线转换、停写静默检查与状态文件恢复校验。',
    status: '预检可用',
    href: '/docs/migration',
  },
  {
    icon: Gauge,
    title: '可重复的质量门禁',
    text: '语句覆盖率 90%+，格式、注释、vet、竞态、双库集成、端点清单与独立 SDK 验收。',
    status: '最小门禁',
    href: '/docs/progress/quality',
  },
]

const layers = [
  { label: '数据层', value: '复用 ACT_* 41 张表', note: '双库通过', tone: 'good' },
  { label: '语义层', value: 'BPMN 主路径', note: 'Java 对照未完成', tone: 'warn' },
  { label: '接口层', value: '184 / 184 官方路由', note: '行为待验收', tone: 'warn' },
  { label: '运行层', value: '单执行者', note: '无集群与故障转移', tone: 'bad' },
]

const snippets = {
  rest: `# 部署、启动、查询待办
curl -sS -X POST 'localhost:8080/service/repository/deployments?deploymentName=hello' \\
  -H 'Content-Type: application/xml' --data-binary @hello.bpmn20.xml

curl -sS -X POST 'localhost:8080/service/runtime/process-instances' \\
  -H 'Content-Type: application/json' \\
  -d '{"processDefinitionKey":"helloWorld","variables":[{"name":"initiator","value":"alice"}]}'

curl -sS 'localhost:8080/service/runtime/tasks?assignee=alice'`,
  go: `app, err := flowtide.New(flowtide.Config{StatePath: "./data/engine.json"})
if err != nil {
    log.Fatal(err)
}
defer app.Close(ctx)

app.RegisterDelegate("notify", func(ctx context.Context, exec *flowtide.Execution) error {
    exec.SetVariable("preparedBy", "system")
    return nil
})

pi, err := app.RuntimeService().StartProcessInstance(ctx, api.StartRequest{
    ProcessDefinitionKey: "onboarding",
    Variables: []api.Variable{{Name: "initiator", Value: "alice"}},
})`,
  mysql: `# 初始化 Schema（重复执行会校验版本）
go run ./cmd/flowtide-migrate -dialect mysql -apply -env .env

# 行仓储模式，单执行者独占锁
go run ./cmd/flowtide-server -storage mysql-rows -env .env

# 既有 Java 库只读预检
./bin/flowtide-migrate -preflight -dialect mysql -env .env -out preflight.json`,
}

// copySnippet 复制示例代码。
// @return 无
async function copySnippet(): Promise<void> {
  try {
    await navigator.clipboard.writeText(snippets[tab.value])
    message.success('示例已复制')
  } catch {
    message.error('浏览器未允许剪贴板访问')
  }
}
</script>

<template>
  <div class="landing">
    <section class="hero">
      <div class="hero__inner">
        <div class="hero__copy">
          <div class="eyebrow"><BookOpen :size="14" /> Flowtide / 项目文档</div>
          <div class="hero__page-icon" aria-hidden="true"><FileText :size="42" :stroke-width="1.4" /></div>
          <h1>Flowtide 文档</h1>
          <p class="hero__subtitle">用 Go 运行 Flowable 流程</p>
          <p class="hero__lead">
            Flowtide 用纯 Go 实现 Flowable BPMN 引擎，复用 <code>ACT_*</code> 表结构与 REST 接口，无需 JVM 或 CGO。
            在这里查看实现进度、运行示例，或评估现有 Java 项目的迁移工作。
          </p>
          <div class="hero__actions">
            <router-link to="/docs/getting-started/quickstart-rest" class="button button--primary">
              <Terminal :size="16" /> 运行第一个流程 <ArrowRight :size="16" />
            </router-link>
            <router-link to="/docs" class="button">浏览文档</router-link>
          </div>
          <div class="hero__status">
            <span class="status-pill status-pill--warn">原型阶段</span>
            <span>单执行者 · 生产切换、回退与集群尚未验收</span>
          </div>
          <div class="hero__revision">更新至 <code>{{ facts.commit.short }}</code><span>{{ facts.commit.date }}</span></div>
        </div>

        <nav class="hero__panel reading-panel" aria-label="开始阅读">
          <div class="reading-panel__head"><BookOpen :size="17" /><h2>开始阅读</h2></div>
          <router-link to="/docs/getting-started/installation" class="reading-link group">
            <span class="block-handle" aria-hidden="true">⋮⋮</span>
            <Terminal :size="19" />
            <span><strong>安装与快速开始</strong><small>从源码构建，启动服务并运行流程。</small></span>
            <ArrowRight :size="16" />
          </router-link>
          <router-link to="/docs/sdk" class="reading-link group">
            <span class="block-handle" aria-hidden="true">⋮⋮</span>
            <Code2 :size="19" />
            <span><strong>在 Go 项目中使用</strong><small>配置引擎，注册委托，调用服务接口。</small></span>
            <ArrowRight :size="16" />
          </router-link>
          <router-link to="/docs/migration" class="reading-link group">
            <span class="block-handle" aria-hidden="true">⋮⋮</span>
            <GitBranch :size="19" />
            <span><strong>从 Java Flowable 迁移</strong><small>了解预检、转换和切换前的准备工作。</small></span>
            <ArrowRight :size="16" />
          </router-link>
          <div class="reading-panel__note">使用前请核对 <router-link to="/compatibility">能力矩阵</router-link>与<router-link to="/docs/compatibility/known-issues">已知问题</router-link>。</div>
        </nav>
      </div>
    </section>

    <section class="trust-bar" aria-label="规模快照">
      <router-link v-for="stat in stats" :key="stat.label" :to="stat.href" class="trust-stat">
        <strong>{{ stat.value }}</strong>
        <span>{{ stat.label }}</span>
      </router-link>
    </section>

    <section class="section-block progress-snapshot">
      <div class="section-heading">
        <div>
          <div class="eyebrow eyebrow--plain"><Check :size="14" /> 开发进度</div>
          <h2>当前实现进度</h2>
          <p>BPMN 内核已能运行。REST 行为对照、生产切换和集群支持仍需继续验收。</p>
        </div>
        <router-link to="/progress" class="text-link">完整进度看板 <ArrowRight :size="15" /></router-link>
      </div>
      <div class="phase-track">
        <div v-for="phase in phases" :key="phase.id" class="phase-card" :class="`phase-card--${phase.state}`">
          <div class="phase-card__top">
            <span class="phase-card__id">{{ phase.id }}</span>
            <span class="phase-card__icon">
              <Check v-if="phase.state === 'done'" :size="15" />
              <CircleDot v-else :size="15" />
            </span>
          </div>
          <div class="phase-card__title">{{ phase.title }}</div>
          <div class="phase-card__detail">{{ phase.detail }}</div>
        </div>
      </div>
    </section>

    <section class="section-block capabilities">
      <div class="section-heading">
        <div>
          <div class="eyebrow eyebrow--plain">能力概览</div>
          <h2>已实现的功能</h2>
          <p>以下功能已有实现与回归测试。各项数据库和 Java 对照结果见对应文档。</p>
        </div>
        <router-link to="/docs" class="text-link">浏览全部文档 <ArrowRight :size="15" /></router-link>
      </div>
      <div class="feature-grid">
        <router-link v-for="feature in features" :key="feature.title" :to="feature.href" class="feature-card group">
          <span class="block-handle" aria-hidden="true">⋮⋮</span>
          <div class="feature-card__icon"><component :is="feature.icon" :size="20" /></div>
          <div class="feature-card__head">
            <h3>{{ feature.title }}</h3>
            <span class="feature-card__status">{{ feature.status }}</span>
          </div>
          <p>{{ feature.text }}</p>
          <span class="feature-card__more">阅读 <ArrowRight :size="14" /></span>
        </router-link>
      </div>
    </section>

    <section class="section-block compatibility-band">
      <div class="compat-copy">
        <div class="eyebrow eyebrow--plain"><ShieldCheck :size="14" /> 兼容性</div>
        <h2>兼容性与限制</h2>
        <p>
          评估接入时，需要分别检查数据库、流程语义、REST 行为和部署方式。
          这些层面的验收进度各不相同，具体结果见能力矩阵。
        </p>
        <router-link to="/compatibility" class="button button--dark">
          打开能力矩阵 <ArrowRight :size="16" />
        </router-link>
      </div>
      <div class="compat-layers">
        <div v-for="layer in layers" :key="layer.label" class="compat-layer" :class="`compat-layer--${layer.tone}`">
          <span class="compat-layer__label">{{ layer.label }}</span>
          <span class="compat-layer__value">{{ layer.value }}</span>
          <span class="compat-layer__note">{{ layer.note }}</span>
        </div>
      </div>
    </section>

    <section class="section-block architecture-block">
      <div class="section-heading">
        <div>
          <div class="eyebrow eyebrow--plain"><Layers :size="14" /> 架构</div>
          <h2>单进程、单执行者、可替换状态层</h2>
          <p>REST 只做 HTTP 与认证，流程语义在引擎内完成；状态层可在内存、文件与 SQL 之间切换。</p>
        </div>
        <router-link to="/docs/architecture/overview" class="text-link">架构总览 <ArrowRight :size="15" /></router-link>
      </div>
      <div class="arch-stack">
        <div class="arch-row arch-row--entry">
          <span>客户端</span><span>curl · Go SDK · Java 客户端</span>
        </div>
        <div class="arch-arrow">↓</div>
        <div class="arch-row">
          <span>接入层</span><span><code>cmd/flowtide-server</code> · REST · 认证 · 指标</span>
        </div>
        <div class="arch-arrow">↓</div>
        <div class="arch-row">
          <span>服务接口</span><span><code>api</code> · Repository / Runtime / Task / History</span>
        </div>
        <div class="arch-arrow">↓</div>
        <div class="arch-row arch-row--core">
          <span>引擎</span><span>Command · Agenda · 执行树 · 任务 · 作业 · 历史</span>
        </div>
        <div class="arch-arrow">↓</div>
        <div class="arch-row">
          <span>持久化</span><span>内存 · JSON 快照 · SQL 快照 · ACT_* 行仓储</span>
        </div>
        <div class="arch-arrow">↓</div>
        <div class="arch-row arch-row--db">
          <span>存储</span><span>MySQL 8.0 · PostgreSQL 16</span>
        </div>
      </div>
    </section>

    <section class="section-block code-block-section">
      <div class="section-heading">
        <div>
          <div class="eyebrow eyebrow--plain"><Code2 :size="14" /> 直接开始</div>
          <h2>运行示例</h2>
          <p>选择 REST、Go SDK 或 MySQL 示例。完整步骤与预期输出见快速开始。</p>
        </div>
        <router-link to="/docs/getting-started/quickstart-go" class="text-link">Go 快速开始 <ArrowRight :size="15" /></router-link>
      </div>
      <div class="code-tabs">
        <div class="code-tabs__bar">
          <div class="code-tabs__buttons">
            <button type="button" :class="{ active: tab === 'rest' }" @click="tab = 'rest'">REST</button>
            <button type="button" :class="{ active: tab === 'go' }" @click="tab = 'go'">Go SDK</button>
            <button type="button" :class="{ active: tab === 'mysql' }" @click="tab = 'mysql'">MySQL</button>
          </div>
          <button type="button" class="copy-inline" @click="copySnippet"><Copy :size="14" /> 复制</button>
        </div>
        <pre class="code-tabs__code"><code>{{ snippets[tab] }}</code></pre>
      </div>
    </section>

    <section class="section-block timeline-block">
      <div class="section-heading">
        <div>
          <div class="eyebrow eyebrow--plain"><GitCommitHorizontal :size="14" /> 最近提交</div>
          <h2>最近更新</h2>
          <p>查看最近的代码与文档变更，完整记录见源码仓库。</p>
        </div>
        <router-link to="/releases" class="text-link">版本与变更 <ArrowRight :size="15" /></router-link>
      </div>
      <div class="commit-list">
        <div v-for="commit in facts.recentCommits.slice(0, 7)" :key="commit.hash" class="commit-row">
          <code class="commit-row__hash">{{ commit.hash }}</code>
          <span class="commit-row__subject">{{ commit.subject }}</span>
          <span class="commit-row__date">{{ commit.date }}</span>
        </div>
      </div>
    </section>

    <section class="section-block status-block">
      <div class="status-card">
        <div class="status-card__icon"><Boxes :size="22" /></div>
        <div class="status-card__copy">
          <h2>使用范围</h2>
          <p>{{ facts.status.phase }}。<strong>单执行者</strong>，没有集群租约、作业抢占与故障转移；停写切换与回退尚未完成演练。</p>
        </div>
        <ul class="status-card__list">
          <li><Check :size="15" /> 新建库原型、单进程、功能验证</li>
          <li><Check :size="15" /> REST 路由探测与 Go SDK 嵌入</li>
          <li><Check :size="15" /> 存量库只读预检</li>
          <li class="no">× 生产替换、多节点、双写、灰度</li>
        </ul>
        <router-link to="/docs/compatibility/known-issues" class="button button--dark">阅读已知问题 <ArrowRight :size="15" /></router-link>
      </div>
    </section>

    <section class="final-cta">
      <h2>准备使用 Flowtide？</h2>
      <p>先核对项目需要的功能。当前版本适合新建库原型和单进程验证，多节点高可用、完整 Java 兼容与可回退迁移仍待验收。</p>
      <div class="final-cta__actions">
        <router-link to="/compatibility" class="button button--primary">打开能力矩阵 <ArrowRight :size="16" /></router-link>
        <router-link to="/docs/getting-started/installation" class="button button--ghost">从安装开始</router-link>
      </div>
    </section>
  </div>
</template>
