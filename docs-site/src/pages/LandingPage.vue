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
  Sparkles,
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

# 行仓储模式：单执行者独占锁
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
      <div class="hero__glow" aria-hidden="true" />
      <div class="hero__grid" aria-hidden="true" />
      <div class="hero__inner">
        <div class="hero__copy">
          <div class="eyebrow">
            <span class="eyebrow__dot" />
            重构进度文档站 · 截至 {{ facts.commit.short }} · {{ facts.commit.date }}
          </div>
          <h1>用纯 Go，重写<br /><span class="hero__accent">Flowable BPMN 引擎</span></h1>
          <p class="hero__lead">
            不依赖 JVM、不依赖 CGO，复用 Flowable 的 <code>ACT_*</code> Schema 与 REST 契约。
            当前已完成 BPMN 内核与 184 个官方 REST 方法路由，正在补齐真实数据库、Java 行为对照与迁移验收。
          </p>
          <div class="hero__status">
            <span class="status-pill status-pill--warn">原型阶段</span>
            <span>新建库可运行 · 单执行者 · 尚未通过停写、回退与集群验收</span>
          </div>
          <div class="hero__actions">
            <router-link to="/progress" class="button button--primary">
              查看当前进度 <ArrowRight :size="17" />
            </router-link>
            <router-link to="/docs/getting-started/quickstart-rest" class="button">
              <Terminal :size="17" /> 10 分钟跑通
            </router-link>
          </div>
          <div class="hero__install">
            <span class="prompt">$</span>
            <code>HOME=/root make build && ./bin/flowtide-server -addr :8080</code>
            <span class="hero__install-note">Go 1.27+</span>
          </div>
        </div>

        <div class="hero__panel">
          <div class="terminal">
            <div class="terminal__bar">
              <span class="terminal__dot" />
              <span class="terminal__dot" />
              <span class="terminal__dot" />
              <span class="terminal__title">flowtide · 当前提交</span>
            </div>
            <div class="terminal__body">
              <div class="terminal__line"><span class="prompt">$</span> curl -sS localhost:8080/healthz</div>
              <div class="terminal__out">{&quot;status&quot;:&quot;UP&quot;}</div>
              <div class="terminal__line"><span class="prompt">$</span> curl -sS localhost:8080/service/management/engine</div>
              <div class="terminal__out">{&quot;name&quot;:&quot;flowtide&quot;,&quot;version&quot;:&quot;{{ facts.version }}&quot;,&quot;historyLevel&quot;:&quot;audit&quot;}</div>
              <div class="terminal__line terminal__line--muted"># 41 张 ACT 表 / 双库验收 / 单执行者</div>
              <div class="terminal__line"><span class="prompt">$</span> curl -sS localhost:8080/metrics</div>
              <div class="terminal__out">flowable_http_requests_total{...} ...</div>
              <div class="terminal__cursor"><span /></div>
            </div>
          </div>
          <div class="hero__panel-foot">
            <span>官方基线 <code>f6017e3</code></span>
            <router-link to="/compatibility">能力矩阵 <ArrowRight :size="14" /></router-link>
          </div>
        </div>
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
          <div class="eyebrow eyebrow--plain"><Sparkles :size="14" /> 截至当前提交</div>
          <h2>内核已跑通，生产验收还在路上</h2>
          <p>每个阶段都对应可重复执行的证据。这里不把“代码存在”写成“已经兼容”。</p>
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
          <h2>六条能力主线，各自带边界</h2>
          <p>状态标签不是营销词：原型可用意味着主路径能跑，但生产验收未完成。</p>
        </div>
        <router-link to="/docs" class="text-link">浏览全部文档 <ArrowRight :size="15" /></router-link>
      </div>
      <div class="feature-grid">
        <router-link v-for="feature in features" :key="feature.title" :to="feature.href" class="feature-card">
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
        <h2>四个层面，分别判断</h2>
        <p>
          路由齐全不代表行为兼容，表名相同不代表数据可接管。使用前请按层面核对证据，
          而不是只看一个百分比。
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
          <h2>三条可复制的路径</h2>
          <p>REST 闭环、Go 嵌入与 MySQL 行仓储。完整步骤与预期输出在快速开始中。</p>
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
          <h2>进度是从提交里长出来的</h2>
          <p>最近的能力增量与文档更新。完整历史见源码仓库。</p>
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
          <h2>现在可以做什么，不能做什么</h2>
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
      <div class="final-cta__glow" aria-hidden="true" />
      <h2>先判断边界，再决定是否使用</h2>
      <p>如果你需要多节点高可用、完整 Java 兼容或可回退迁移，当前版本还不适合。先读能力矩阵，能节省很多时间。</p>
      <div class="final-cta__actions">
        <router-link to="/compatibility" class="button button--primary">打开能力矩阵 <ArrowRight :size="16" /></router-link>
        <router-link to="/docs/getting-started/installation" class="button button--ghost">从安装开始</router-link>
      </div>
    </section>
  </div>
</template>
