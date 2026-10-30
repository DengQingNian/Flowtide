// 运维域展示辅助：作业桶文案、表分组、二进制判断与审计本地筛选。
import type { AuditEvent, JobBucket, TableInfo } from '@/api/management'

/** 作业桶展示名 */
export const JOB_BUCKET_LABELS: Record<JobBucket, string> = {
  executable: '可执行',
  timer: '定时',
  suspended: '挂起',
  deadletter: '死信',
}

/** 作业桶展示顺序 */
export const JOB_BUCKET_ORDER: JobBucket[] = ['executable', 'timer', 'suspended', 'deadletter']

/** 内置只读属性名，与 internal/engine/properties_admin.go 保持一致 */
const SYSTEM_PROPERTY_NAMES = new Set([
  'common.schema.version',
  'schema.version',
  'schema.history',
  'next.dbid',
  'engine.version',
  'historyLevel',
  'tenantId',
])

/** 表分组定义 */
const TABLE_GROUPS: { key: string; label: string; prefixes: string[] }[] = [
  { key: 'repository', label: '仓库', prefixes: ['ACT_RE_'] },
  { key: 'runtime', label: '运行', prefixes: ['ACT_RU_'] },
  { key: 'history', label: '历史', prefixes: ['ACT_HI_'] },
  { key: 'identity', label: '身份', prefixes: ['ACT_ID_'] },
]

/** 表分组结果 */
export interface TableGroup {
  /** 分组键 */
  key: string
  /** 分组名 */
  label: string
  /** 该组表 */
  tables: TableInfo[]
  /** 该组行数合计 */
  totalCount: number
}

/** 审计筛选条件（接口无筛选参数，页面拉取最近一段后在本地过滤） */
export interface AuditFilter {
  /** HTTP 方法，空表示不过滤 */
  method?: string
  /** 路径关键字，不区分大小写 */
  path?: string
  /** 状态码 */
  status?: string
  /** 主体关键字，不区分大小写 */
  userId?: string
}

// 判断属性是否内置只读。
// @param name 属性名
// @return 是否只读
export function isSystemProperty(name: string): boolean {
  return SYSTEM_PROPERTY_NAMES.has(name) || name.startsWith('flowtide.schema.')
}

// 按 ACT_* 前缀把表分为仓库、运行、历史、身份与其他。
// @param tables 表清单
// @return 固定顺序的分组，空分组也返回
export function groupTables(tables: TableInfo[]): TableGroup[] {
  const groups: TableGroup[] = TABLE_GROUPS.map((group) => ({ ...group, tables: [], totalCount: 0 }))
  const other: TableGroup = { key: 'other', label: '其他', tables: [], totalCount: 0 }
  for (const table of tables) {
    const matched = groups.find((group) => TABLE_GROUPS.find((item) => item.key === group.key)?.prefixes.some((prefix) => table.name.startsWith(prefix)))
    const target = matched ?? other
    target.tables.push(table)
    target.totalCount += table.count
  }
  return [...groups, other]
}

// 判断列类型是否为二进制。
// @param type 列类型
// @return 是否二进制
export function isBinaryColumn(type: string | undefined): boolean {
  const lower = (type ?? '').toLowerCase()
  return lower.includes('bytea') || lower.includes('blob') || lower.includes('binary')
}

// 按本地条件筛选审计事件。
// @param events 事件列表
// @param filter 筛选条件
// @return 命中事件
export function filterAuditEvents(events: AuditEvent[], filter: AuditFilter): AuditEvent[] {
  const method = filter.method?.trim().toUpperCase() ?? ''
  const path = filter.path?.trim().toLowerCase() ?? ''
  const status = filter.status?.trim() ?? ''
  const user = filter.userId?.trim().toLowerCase() ?? ''
  if (!method && !path && !status && !user) return events
  return events.filter((event) => {
    // 多个条件同时存在时要求全部命中
    if (method && event.method.toUpperCase() !== method) return false
    if (path && !event.path.toLowerCase().includes(path)) return false
    if (status && String(event.status) !== status) return false
    if (user && !(event.userId ?? '').toLowerCase().includes(user)) return false
    return true
  })
}
