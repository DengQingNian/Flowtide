// 运维域 REST 适配：作业、批处理、表数据、引擎属性与审计日志。
// 只负责请求构造与响应类型声明，筛选/分组等展示逻辑见 pages/management/components。

import { ApiError, deleteJSON, getJSON, getText, postJSON, putJSON } from './client'
import type { Page } from './client'

/** 作业桶 */
export type JobBucket = 'executable' | 'timer' | 'suspended' | 'deadletter'

/** 作业信息 */
export interface JobInfo extends Record<string, unknown> {
  /** 作业 ID */
  id: string
  /** 作业类型，如 async-continuation、timer-transition */
  type?: string
  /** 流程实例 ID */
  processInstanceId?: string
  /** 执行 ID */
  executionId?: string
  /** 活动 ID */
  activityId?: string
  /** 剩余重试次数 */
  retries: number
  /** 最近异常信息 */
  exception?: string
  /** 到期时间，RFC3339 */
  dueDate?: string | null
  /** 是否已进入可执行桶 */
  executable?: boolean
}
/** 立即执行到期作业的结果 */
export interface RunJobsResult {
  /** 成功执行的作业数量 */
  executed: number
}

/** 作业动作载荷 */
export interface JobActionPayload {
  /** 引擎动作名 */
  action: 'moveToExecutable' | 'setRetries' | 'move' | 'reschedule'
  /** 重试次数，move/setRetries 使用 */
  retries?: number
  /** 新到期时间，reschedule 使用 */
  dueDate?: string
}

/** 批处理 */
export interface Batch extends Record<string, unknown> {
  /** 批次 ID */
  id: string
  /** 批次类型 */
  type: string
  /** 分片总数 */
  total: number
  /** 已完成分片数 */
  completed: number
  /** 批次状态 */
  status: string
  /** 开始时间 */
  startTime: string
  /** 结束时间 */
  endTime?: string | null
  /** 租户 */
  tenantId?: string
}

/** 批处理分片 */
export interface BatchPart extends Record<string, unknown> {
  /** 分片 ID */
  id: string
  /** 所属批次 ID */
  batchId: string
  /** 分片类型 */
  type: string
  /** 关联流程实例 ID */
  processInstanceId?: string
  /** 分片状态 */
  status: string
  /** 开始时间 */
  startTime: string
  /** 结束时间 */
  endTime?: string | null
  /** 失败原因 */
  errorMessage?: string
}

/** 批处理摘要 */
export interface BatchSummary {
  /** 批次 ID */
  id: string
  /** 批次 ID 别名 */
  batchId: string
  /** 批次类型 */
  type: string
  /** 批次状态 */
  status: string
  /** 分片总数 */
  total: number
  /** 完成数 */
  completed: number
  /** 失败数 */
  failed: number
  /** 执行中数量 */
  inProgress: number
  /** 排队数量 */
  queued: number
  /** 开始时间 */
  startTime: string
  /** 结束时间 */
  endTime?: string | null
  /** 租户 */
  tenantId?: string
}

/** 批处理列表查询 */
export interface BatchQuery {
  /** 批次类型 */
  type?: string
  /** 批次状态 */
  status?: string
  /** 租户 */
  tenantId?: string
  /** 偏移 */
  start?: number
  /** 每页数量 */
  size?: number
}

/** 表信息 */
export interface TableInfo extends Record<string, unknown> {
  /** 表名 */
  name: string
  /** 行数 */
  count: number
  /** 资源地址 */
  url?: string
}

/** 表列元数据 */
export interface TableMetaData {
  /** 表名 */
  tableName: string
  /** 列名 */
  columnNames: string[]
  /** 列类型 */
  columnTypes: string[]
}

/** 表数据行 */
export type TableRow = Record<string, unknown>

/** 表数据查询 */
export interface TableDataQuery {
  /** 偏移 */
  start?: number
  /** 每页数量 */
  size?: number
  /** 排序列 */
  sortColumn?: string
  /** 排序方向 */
  order?: 'asc' | 'desc'
}

/** 引擎属性 */
export interface EngineProperty {
  /** 属性名 */
  name: string
  /** 属性值 */
  value: string
}

/** 审计事件 */
export interface AuditEvent extends Record<string, unknown> {
  /** 事件 ID */
  id: number
  /** 发生时间 */
  time: string
  /** 主体 */
  userId?: string
  /** 租户 */
  tenantId?: string
  /** HTTP 方法 */
  method: string
  /** 请求路径 */
  path: string
  /** 响应状态码 */
  status: number
  /** 耗时毫秒 */
  durationMillis: number
  /** 客户端地址 */
  remoteAddr?: string
}

/** 各作业桶的 REST 前缀 */
const JOB_PATHS: Record<JobBucket, string> = {
  executable: '/service/management/jobs',
  timer: '/service/management/timer-jobs',
  suspended: '/service/management/suspended-jobs',
  deadletter: '/service/management/deadletter-jobs',
}
// 判断值是否为合法作业桶。
// @param value 待判断值
// @return 是否合法
export function isJobBucket(value: unknown): value is JobBucket {
  return value === 'executable' || value === 'timer' || value === 'suspended' || value === 'deadletter'
}
// 将路由参数归一化为作业桶。
// @param value 路由参数
// @return 合法桶，非法时回退到 executable
export function normalizeJobBucket(value: unknown): JobBucket {
  return isJobBucket(value) ? value : 'executable'
}
// 生成编码后的路径片段。
// @param value 原始值
// @return URL 安全片段
function segment(value: string): string {
  return encodeURIComponent(value)
}
// 查询作业列表。
// @param bucket 作业桶
// @param query 分页参数
// @param signal 取消信号
// @return 作业分页
export function listJobs(bucket: JobBucket, query: { start?: number; size?: number }, signal?: AbortSignal): Promise<Page<JobInfo>> {
  return getJSON<Page<JobInfo>>(JOB_PATHS[bucket], { start: query.start ?? 0, size: query.size ?? 50 }, signal)
}
// 查询单个作业桶的总数。
// @param bucket 作业桶
// @param signal 取消信号
// @return 总数
export async function countJobs(bucket: JobBucket, signal?: AbortSignal): Promise<number> {
  const page = await listJobs(bucket, { start: 0, size: 1 }, signal)
  return page.total
}
// 查询作业详情。
// @param bucket 作业桶
// @param id 作业 ID
// @param signal 取消信号
// @return 作业信息
export function getJob(bucket: JobBucket, id: string, signal?: AbortSignal): Promise<JobInfo> {
  return getJSON<JobInfo>(`${JOB_PATHS[bucket]}/${segment(id)}`, undefined, signal)
}
// 查询作业异常栈。
// @param bucket 作业桶
// @param id 作业 ID
// @param signal 取消信号
// @return 纯文本异常栈
export function getJobStacktrace(bucket: JobBucket, id: string, signal?: AbortSignal): Promise<string> {
  return getText(`${JOB_PATHS[bucket]}/${segment(id)}/exception-stacktrace`, undefined, signal)
}
// 执行作业动作。
// @param bucket 作业桶
// @param id 作业 ID
// @param payload 动作载荷
// @return 无
export async function applyJobAction(bucket: JobBucket, id: string, payload: JobActionPayload): Promise<void> {
  // 挂起作业在服务端没有动作端点，提前给出明确错误
  if (bucket === 'suspended') throw new ApiError(400, '挂起作业不支持该动作，请先激活所属流程实例', 'CLIENT-UNSUPPORTED')
  const body: Record<string, unknown> = { action: payload.action }
  if (payload.retries !== undefined) body.retries = payload.retries
  if (payload.dueDate !== undefined) body.dueDate = payload.dueDate
  await postJSON<void>(`${JOB_PATHS[bucket]}/${segment(id)}`, body)
}
// 删除作业。
// @param bucket 作业桶
// @param id 作业 ID
// @return 无
export function deleteJob(bucket: JobBucket, id: string): Promise<void> {
  return deleteJSON<void>(`${JOB_PATHS[bucket]}/${segment(id)}`)
}
// 立即执行到期定时器作业。
// @return 执行数量
export function runDueJobs(): Promise<RunJobsResult> {
  return postJSON<RunJobsResult>('/service/management/jobs/run')
}
// 查询批处理列表。
// @param query 类型、状态、租户与分页
// @param signal 取消信号
// @return 批次分页
export function listBatches(query: BatchQuery = {}, signal?: AbortSignal): Promise<Page<Batch>> {
  return getJSON<Page<Batch>>('/service/management/batches', { ...query }, signal)
}
// 查询批处理详情。
// @param id 批次 ID
// @param signal 取消信号
// @return 批次
export function getBatch(id: string, signal?: AbortSignal): Promise<Batch> {
  return getJSON<Batch>(`/service/management/batches/${segment(id)}`, undefined, signal)
}
// 查询批处理分片。
// @param id 批次 ID
// @param query 分页参数
// @param signal 取消信号
// @return 分片分页
export function listBatchParts(id: string, query: { start?: number; size?: number } = {}, signal?: AbortSignal): Promise<Page<BatchPart>> {
  return getJSON<Page<BatchPart>>(`/service/management/batches/${segment(id)}/batch-parts`, { start: query.start ?? 0, size: query.size ?? 50 }, signal)
}
// 查询批处理分片详情。
// @param id 分片 ID
// @param signal 取消信号
// @return 分片
export function getBatchPart(id: string, signal?: AbortSignal): Promise<BatchPart> {
  return getJSON<BatchPart>(`/service/management/batch-parts/${segment(id)}`, undefined, signal)
}
// 查询批处理摘要。
// @param id 批次 ID
// @param signal 取消信号
// @return 摘要
export function getBatchSummary(id: string, signal?: AbortSignal): Promise<BatchSummary> {
  return getJSON<BatchSummary>(`/service/management/batches/${segment(id)}/summary`, undefined, signal)
}
// 查询批处理文档。
// @param id 批次 ID
// @param signal 取消信号
// @return 原始 JSON 文档
export function getBatchDocument(id: string, signal?: AbortSignal): Promise<unknown> {
  return getJSON<unknown>(`/service/management/batches/${segment(id)}/batch-document`, undefined, signal)
}
// 查询批处理分片文档。
// @param id 分片 ID
// @param signal 取消信号
// @return 原始 JSON 文档
export function getBatchPartDocument(id: string, signal?: AbortSignal): Promise<unknown> {
  return getJSON<unknown>(`/service/management/batch-parts/${segment(id)}/batch-part-document`, undefined, signal)
}
// 删除批处理及其分片。
// @param id 批次 ID
// @return 无
export function deleteBatch(id: string): Promise<void> {
  return deleteJSON<void>(`/service/management/batches/${segment(id)}`)
}
// 查询表清单。
// @param signal 取消信号
// @return 表数组
export function listTables(signal?: AbortSignal): Promise<TableInfo[]> {
  return getJSON<TableInfo[]>('/service/management/tables', undefined, signal)
}
// 查询表列元数据。
// @param name 表名
// @param signal 取消信号
// @return 列元数据
export function getTableColumns(name: string, signal?: AbortSignal): Promise<TableMetaData> {
  return getJSON<TableMetaData>(`/service/management/tables/${segment(name)}/columns`, undefined, signal)
}
// 查询表数据。
// @param name 表名
// @param query 分页与排序
// @param signal 取消信号
// @return 行分页
export function getTableData(name: string, query: TableDataQuery = {}, signal?: AbortSignal): Promise<Page<TableRow>> {
  const params: Record<string, unknown> = { start: query.start ?? 0, size: query.size ?? 50 }
  // 服务端用两个互斥参数表达排序方向
  if (query.sortColumn) params[query.order === 'desc' ? 'orderDescendingColumn' : 'orderAscendingColumn'] = query.sortColumn
  return getJSON<Page<TableRow>>(`/service/management/tables/${segment(name)}/data`, params, signal)
}
// 查询引擎属性。
// @param signal 取消信号
// @return 属性名到值的映射
export function listEngineProperties(signal?: AbortSignal): Promise<Record<string, string>> {
  return getJSON<Record<string, string>>('/service/management/engine-properties', undefined, signal)
}
// 新增引擎属性。
// @param name 属性名
// @param value 属性值
// @return 创建结果
export function createEngineProperty(name: string, value: string): Promise<EngineProperty> {
  return postJSON<EngineProperty>('/service/management/engine-properties', { name, value })
}
// 更新引擎属性。
// @param name 属性名
// @param value 新值
// @return 更新结果
export function updateEngineProperty(name: string, value: string): Promise<EngineProperty> {
  return putJSON<EngineProperty>(`/service/management/engine-properties/${segment(name)}`, { name, value })
}
// 删除引擎属性。
// @param name 属性名
// @return 无
export function deleteEngineProperty(name: string): Promise<void> {
  return deleteJSON<void>(`/service/management/engine-properties/${segment(name)}`)
}
// 查询审计日志。
// @param query 分页参数（接口无筛选参数）
// @param signal 取消信号
// @return 审计分页
export function listAuditLogs(query: { start?: number; size?: number } = {}, signal?: AbortSignal): Promise<Page<AuditEvent>> {
  return getJSON<Page<AuditEvent>>('/service/management/audit-logs', { start: query.start ?? 0, size: query.size ?? 50 }, signal)
}
// 查询流程实例摘要，用于作业详情跳转流程定义。
// @param id 流程实例 ID
// @param signal 取消信号
// @return 实例摘要
export function getProcessInstanceBrief(
  id: string,
  signal?: AbortSignal,
): Promise<{ id: string; processDefinitionId?: string; processDefinitionKey?: string; businessKey?: string; name?: string }> {
  return getJSON(`/service/runtime/process-instances/${segment(id)}`, undefined, signal)
}
