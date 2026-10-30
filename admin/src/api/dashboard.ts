// 仪表盘聚合接口：并发读取引擎信息、关键计数与最近数据，单点失败不影响其他卡片。
import { getJSON, request } from './client'
import type { Page } from './client'

/** 引擎信息 */
export interface EngineInfo {
  name?: string
  version?: string
  historyLevel?: string
  closed?: boolean
}

/** 部署摘要 */
export interface DeploymentSummary {
  id: string
  name?: string
  tenantId?: string
  deployTime?: string
}

/** 实例摘要 */
export interface InstanceSummary {
  id: string
  processDefinitionId?: string
  processDefinitionName?: string
  businessKey?: string
  startTime?: string
  ended?: boolean
  suspended?: boolean
  activityId?: string
}

/** 死信作业摘要 */
export interface JobSummary {
  id: string
  retries?: number
  exceptionMessage?: string
  dueDate?: string
  processInstanceId?: string
  executionId?: string
}

/** 仪表盘计数，接口失败时为 null */
export interface DashboardCounts {
  deployments: number | null
  definitions: number | null
  instances: number | null
  tasks: number | null
  deadletters: number | null
}

/** 健康状态 */
export interface DashboardHealth {
  health: string
  ready: string
  metrics: Record<string, number>
}

/** 仪表盘数据 */
export interface DashboardData {
  engine: EngineInfo | null
  properties: Record<string, string>
  counts: DashboardCounts
  deployments: DeploymentSummary[]
  instances: InstanceSummary[]
  deadletters: JobSummary[]
  health: DashboardHealth
  errors: string[]
}

// 读取分页总数。
// @param path 接口路径
// @param query 查询参数
// @return 总数，失败返回 null
async function countOf(path: string, query: Record<string, unknown> = {}): Promise<number | null> {
  try {
    const page = await getJSON<Page<unknown>>(path, { start: 0, size: 1, ...query })
    return page.total ?? page.data?.length ?? null
  } catch {
    return null
  }
}

// 读取引擎信息。
// @return 引擎信息或 null
async function loadEngine(): Promise<EngineInfo | null> {
  try {
    return await getJSON<EngineInfo>('/service/management/engine')
  } catch {
    return null
  }
}

// 读取引擎属性。
// @return 属性键值
async function loadProperties(): Promise<Record<string, string>> {
  try {
    return await getJSON<Record<string, string>>('/service/management/engine-properties')
  } catch {
    return {}
  }
}

// 读取最近部署。
// @return 部署列表
async function loadDeployments(): Promise<DeploymentSummary[]> {
  try {
    const page = await getJSON<Page<DeploymentSummary>>('/service/repository/deployments', { start: 0, size: 5 })
    return page.data ?? []
  } catch {
    return []
  }
}

// 读取最近启动的实例。
// @return 实例列表
async function loadInstances(): Promise<InstanceSummary[]> {
  try {
    const page = await getJSON<Page<InstanceSummary>>('/service/runtime/process-instances', {
      start: 0,
      size: 5,
      sort: 'startTime',
      order: 'desc',
    })
    return page.data ?? []
  } catch {
    return []
  }
}

// 读取死信作业。
// @return 作业列表
async function loadDeadletters(): Promise<JobSummary[]> {
  try {
    const page = await getJSON<Page<JobSummary>>('/service/management/deadletter-jobs', { start: 0, size: 5 })
    return page.data ?? []
  } catch {
    return []
  }
}

// 读取健康与指标摘要。
// @return 健康状态
async function loadHealth(): Promise<DashboardHealth> {
  const metrics: Record<string, number> = {}
  let health = '—'
  let ready = '—'
  try {
    const result = await request<{ status?: string }>('/healthz')
    health = result.status ?? 'UP'
  } catch {
    health = 'DOWN'
  }
  try {
    const result = await request<{ status?: string }>('/readyz')
    ready = result.status ?? 'UP'
  } catch {
    ready = 'DOWN'
  }
  try {
    const text = await request<string>('/metrics')
    for (const line of text.split('\n')) {
      const match = /^(flowable_[a-zA-Z0-9_]+)\s+([0-9.eE+-]+)$/.exec(line.trim())
      if (match) metrics[match[1]] = Number(match[2])
    }
  } catch {
    // 指标不可用时保持空对象，不影响其他卡片
  }
  return { health, ready, metrics }
}

// 汇总仪表盘数据。
// @return 仪表盘数据
export async function loadDashboard(): Promise<DashboardData> {
  const [engine, properties, deployments, definitions, instances, tasks, deadletters, recentDeployments, recentInstances, recentDeadletters, health] =
    await Promise.all([
      loadEngine(),
      loadProperties(),
      countOf('/service/repository/deployments'),
      countOf('/service/repository/process-definitions'),
      countOf('/service/runtime/process-instances'),
      countOf('/service/runtime/tasks'),
      countOf('/service/management/deadletter-jobs'),
      loadDeployments(),
      loadInstances(),
      loadDeadletters(),
      loadHealth(),
    ])
  const errors: string[] = []
  if (!engine) errors.push('引擎信息读取失败，请检查服务状态与权限。')
  if (!health || health.health === 'DOWN') errors.push('健康检查未通过。')
  return {
    engine,
    properties,
    counts: { deployments, definitions, instances, tasks, deadletters },
    deployments: recentDeployments,
    instances: recentInstances,
    deadletters: recentDeadletters,
    health,
    errors,
  }
}
