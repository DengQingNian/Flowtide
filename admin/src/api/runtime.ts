// 运行态流程实例 REST 封装：实例、变量、任务、评论、附件、身份链接与迁移。
// 所有接口路径与字段以 internal/rest/server.go 的路由和 handler 为准。

import {
  deleteJSON,
  getBlob,
  getJSON,
  getText,
  postJSON,
  putJSON,
  type Page,
} from './client'
import type { Attachment, Comment, IdentityLink, Variable, VariableInstance } from './types'

/** 运行态实例 DTO；服务端响应没有 processDefinitionKey，Key 由定义 ID 推导 */
export type ProcessInstance = {
  /** 实例 ID */
  id: string
  /** 资源地址 */
  url?: string
  /** 流程定义 ID，格式为 key:version:uuid */
  processDefinitionId: string
  /** 流程定义名称 */
  processDefinitionName?: string | null
  /** 流程定义说明 */
  processDefinitionDescription?: string | null
  /** 业务 Key */
  businessKey?: string | null
  /** 实例名称 */
  name?: string | null
  /** 发起人 */
  startUserId?: string | null
  /** 开始时间 */
  startTime?: string
  /** 结束时间：运行中实例通常为空 */
  endTime?: string | null
  /** 到期时间 */
  dueDate?: string | null
  /** 签收时间 */
  claimTime?: string | null
  /** 是否已结束 */
  ended?: boolean
  /** 是否已完成，服务端与 ended 同值 */
  completed?: boolean
  /** 是否挂起 */
  suspended?: boolean
  /** 当前活动 ID */
  activityId?: string | null
  /** 租户 */
  tenantId?: string | null
  /** 业务状态 */
  businessStatus?: string | null
  /** 签收人 */
  claimedBy?: string | null
  /** 删除原因：仅历史实例或兼容字段可能返回 */
  deleteReason?: string | null
  /** 实例变量，默认列表不携带 */
  variables?: RuntimeVariable[]
}

/** 实例变量；GET /variables 只返回 name/value，type 由前端推断 */
export type RuntimeVariable = VariableInstance & {
  /** 原始数据地址，二进制变量返回 */
  valueUrl?: string
  /** 是否瞬态变量，服务端当前不持久化瞬态变量 */
  transient?: boolean
}

/** 流程定义摘要 */
export type ProcessDefinitionSummary = {
  /** 定义 ID */
  id: string
  /** 定义 Key */
  key: string
  /** 定义名称 */
  name?: string | null
  /** 版本号 */
  version?: number
  /** 是否挂起 */
  suspended?: boolean
  /** 部署 ID */
  deploymentId?: string
  /** 资源名 */
  resourceName?: string
  /** 租户 */
  tenantId?: string | null
}

/** 实例任务裸实体：GET /process-instances/{id}/tasks 返回该结构 */
export type RuntimeTask = {
  /** 任务 ID */
  id: string
  /** 任务名称 */
  name?: string
  /** 任务描述 */
  description?: string
  /** 办理人 */
  assignee?: string | null
  /** 所有者 */
  owner?: string | null
  /** 优先级 */
  priority?: number
  /** 创建时间 */
  createTime?: string
  /** 到期时间 */
  dueDate?: string | null
  /** 任务定义 Key */
  taskDefinitionKey?: string
  /** 流程实例 ID */
  processInstanceId?: string
  /** 流程定义 ID */
  processDefinitionId?: string
  /** 委派状态 */
  delegationState?: string | null
  /** 候选用户 */
  candidateUsers?: string[]
  /** 候选组 */
  candidateGroups?: string[]
}

/** 活动实例：运行态与历史活动共用的展示结构 */
export type ActivityRow = {
  /** 记录 ID */
  id: string
  /** BPMN 活动 ID */
  activityId: string
  /** 活动名称 */
  activityName?: string
  /** 活动类型 */
  activityType?: string
  /** 执行 ID */
  executionId?: string
  /** 任务 ID */
  taskId?: string
  /** 办理人 */
  assignee?: string
  /** 开始时间 */
  startTime?: string
  /** 结束时间；为空表示仍在执行 */
  endTime?: string | null
  /** 持续时间毫秒 */
  durationInMillis?: number
}

/** 死信作业摘要，用于把异常活动标红 */
export type DeadletterJob = {
  /** 作业 ID */
  id: string
  /** 流程实例 ID */
  processInstanceId?: string
  /** 活动 ID */
  activityId?: string
  /** 重试次数 */
  retries?: number
  /** 异常摘要 */
  exception?: string
}

/** 实例列表查询参数 */
export type InstanceListQuery = {
  /** 流程定义 Key */
  processDefinitionKey?: string
  /** 业务 Key */
  businessKey?: string
  /** 租户 */
  tenantId?: string
  /** 仅显示未挂起实例，映射为 suspended=false */
  onlyActive?: boolean
  /** 偏移量 */
  start?: number
  /** 每页数量 */
  size?: number
  /** 排序字段：id/processDefinitionId/processDefinitionKey/startTime/tenantId/businessKey */
  sort?: string
  /** 排序方向 */
  order?: 'asc' | 'desc'
}

/** 启动实例载荷 */
export type StartInstancePayload = {
  /** 定义 ID，与 key 二选一 */
  processDefinitionId?: string
  /** 定义 Key，与 ID 二选一 */
  processDefinitionKey?: string
  /** 业务 Key */
  businessKey?: string
  /** 实例名称 */
  name?: string
  /** 变量对象 */
  variables?: Record<string, unknown>
}

/** 实例状态动作 */
export type InstanceAction = 'suspend' | 'activate'

/** 迁移载荷 */
export type MigrationPayload = {
  /** 目标流程定义 ID */
  targetProcessDefinitionId: string
  /** 活动映射：源活动 ID → 目标活动 ID */
  activityMappings?: Record<string, string>
}

/** 迁移校验结果 */
export type MigrationValidation = {
  /** 阻断或提示信息，空数组表示可迁移 */
  validationMessages?: string[]
}

/** 分页参数 */
export type TabPageQuery = { start: number; size: number }

/** 实例接口前缀 */
const INSTANCES = '/service/runtime/process-instances'

/** 流程定义接口前缀 */
const DEFINITIONS = '/service/repository/process-definitions'

// 拼装实例子资源路径。
// @param id 实例 ID
// @param suffix 子资源路径
// @return 完整路径
function instancePath(id: string, suffix = ''): string {
  return `${INSTANCES}/${encodeURIComponent(id)}${suffix}`
}

// 拼装变量子资源路径。
// @param id 实例 ID
// @param name 变量名
// @param suffix 子资源路径
// @return 完整路径
function variablePath(id: string, name: string, suffix = ''): string {
  return instancePath(id, `/variables/${encodeURIComponent(name)}${suffix}`)
}

// 根据值推断变量类型，服务端变量集合端点不返回类型。
// @param value 变量值
// @return Flowable 风格类型名
export function inferVariableType(value: unknown): string {
  if (value === null || value === undefined) return 'null'
  if (typeof value === 'boolean') return 'boolean'
  if (typeof value === 'number') return Number.isInteger(value) ? 'long' : 'double'
  if (Array.isArray(value) || typeof value === 'object') return 'json'
  return 'string'
}

// 从定义 ID 中推导 Key；定义 ID 形如 key:version:uuid。
// @param definitionId 流程定义 ID
// @return 定义 Key，无法解析时返回原值
export function definitionKeyOf(definitionId: string | null | undefined): string {
  if (!definitionId) return ''
  const parts = definitionId.split(':')
  // 少于三段说明不是标准 ID，直接返回原值，避免误截断含冒号的 Key。
  if (parts.length < 3) return definitionId
  return parts.slice(0, parts.length - 2).join(':')
}

// 从定义 ID 中推导版本号。
// @param definitionId 流程定义 ID
// @return 版本号，无法解析时返回 null
export function definitionVersionOf(definitionId: string | null | undefined): number | null {
  if (!definitionId) return null
  const parts = definitionId.split(':')
  // 标准 ID 的倒数第二段是版本；非数字视为无法解析。
  if (parts.length < 3) return null
  const version = Number(parts[parts.length - 2])
  return Number.isInteger(version) && version > 0 ? version : null
}

// 查询运行态实例列表。
// @param query 查询参数
// @param signal 取消信号
// @return 分页实例
export function listProcessInstances(query: InstanceListQuery = {}, signal?: AbortSignal): Promise<Page<ProcessInstance>> {
  const { onlyActive, ...rest } = query
  return getJSON<Page<ProcessInstance>>(INSTANCES, {
    ...rest,
    suspended: onlyActive ? false : undefined,
  }, signal)
}

// 查询单个运行态实例。
// @param id 实例 ID
// @param signal 取消信号
// @return 实例 DTO
export function getProcessInstance(id: string, signal?: AbortSignal): Promise<ProcessInstance> {
  return getJSON<ProcessInstance>(instancePath(id), undefined, signal)
}

// 启动流程实例。
// @param payload 启动载荷
// @return 新建实例
export function startProcessInstance(payload: StartInstancePayload): Promise<ProcessInstance> {
  return postJSON<ProcessInstance>(INSTANCES, payload)
}

// 挂起或激活实例。
// @param id 实例 ID
// @param action 动作
// @param assignee 签收动作使用的用户，仅兼容保留
// @return 更新后的实例
export function updateProcessInstanceState(id: string, action: InstanceAction, assignee?: string): Promise<ProcessInstance> {
  return putJSON<ProcessInstance>(instancePath(id), assignee ? { action, assignee } : { action })
}

// 删除运行态实例。
// @param id 实例 ID
// @param deleteReason 删除原因
// @return 无
export function deleteProcessInstance(id: string, deleteReason = ''): Promise<void> {
  return deleteJSON<void>(instancePath(id), { deleteReason: deleteReason || undefined })
}

// 查询实例变量集合。
// @param id 实例 ID
// @param signal 取消信号
// @return 变量数组，缺失类型时按值推断
export function listInstanceVariables(id: string, signal?: AbortSignal): Promise<RuntimeVariable[]> {
  return getJSON<RuntimeVariable[]>(instancePath(id, '/variables'), undefined, signal).then((rows) =>
    (rows ?? []).map((row) => ({ ...row, type: row.type || inferVariableType(row.value) })),
  )
}

// 新增实例变量。
// @param id 实例 ID
// @param variable 变量
// @return 创建的变量
export function createInstanceVariable(id: string, variable: Variable): Promise<RuntimeVariable> {
  return postJSON<RuntimeVariable>(instancePath(id, '/variables'), variable)
}

// 修改实例变量。
// @param id 实例 ID
// @param name 变量名
// @param variable 变量值
// @return 无
export function updateInstanceVariable(id: string, name: string, variable: Pick<Variable, 'value' | 'type'>): Promise<void> {
  return putJSON<void>(variablePath(id, name), { name, type: variable.type, value: variable.value })
}

// 删除实例变量。
// @param id 实例 ID
// @param name 变量名
// @return 无
export function deleteInstanceVariable(id: string, name: string): Promise<void> {
  return deleteJSON<void>(variablePath(id, name))
}

// 下载变量原始数据。
// @param id 实例 ID
// @param name 变量名
// @return 原始二进制
export function getInstanceVariableData(id: string, name: string): Promise<Blob> {
  return getBlob(variablePath(id, name, '/data'))
}

// 查询实例任务。
// @param id 实例 ID
// @param query 分页参数
// @return 分页任务
export function listInstanceTasks(id: string, query: TabPageQuery): Promise<Page<RuntimeTask>> {
  return getJSON<Page<RuntimeTask>>(instancePath(id, '/tasks'), {
    start: query.start,
    size: query.size,
  })
}

// 查询实例评论。
// @param id 实例 ID
// @param query 分页参数
// @return 分页评论
export function listInstanceComments(id: string, query: TabPageQuery): Promise<Page<Comment>> {
  return getJSON<Page<Comment>>(instancePath(id, '/comments'), {
    start: query.start,
    size: query.size,
  })
}

// 新增实例评论。
// @param id 实例 ID
// @param message 内容
// @param userId 作者
// @return 新评论
export function addInstanceComment(id: string, message: string, userId = ''): Promise<Comment> {
  return postJSON<Comment>(instancePath(id, '/comments'), { message, userId })
}

// 删除实例评论。
// @param id 实例 ID
// @param commentId 评论 ID
// @return 无
export function deleteInstanceComment(id: string, commentId: string): Promise<void> {
  return deleteJSON<void>(instancePath(id, `/comments/${encodeURIComponent(commentId)}`))
}

// 查询实例附件。
// @param id 实例 ID
// @param query 分页参数
// @return 分页附件
export function listInstanceAttachments(id: string, query: TabPageQuery): Promise<Page<Attachment>> {
  return getJSON<Page<Attachment>>(instancePath(id, '/attachments'), {
    start: query.start,
    size: query.size,
  })
}

// 新增实例外链附件；服务端只接受 JSON，不提供实例附件内容上传端点。
// @param id 实例 ID
// @param payload 附件信息
// @return 新附件
export function addInstanceAttachment(
  id: string,
  payload: { name: string; description?: string; type?: string; url?: string },
): Promise<Attachment> {
  return postJSON<Attachment>(instancePath(id, '/attachments'), payload)
}

// 删除实例附件。
// @param id 实例 ID
// @param attachmentId 附件 ID
// @return 无
export function deleteInstanceAttachment(id: string, attachmentId: string): Promise<void> {
  return deleteJSON<void>(instancePath(id, `/attachments/${encodeURIComponent(attachmentId)}`))
}

// 查询实例身份链接。
// @param id 实例 ID
// @param signal 取消信号
// @return 身份链接数组
export function listInstanceIdentityLinks(id: string, signal?: AbortSignal): Promise<IdentityLink[]> {
  return getJSON<IdentityLink[]>(instancePath(id, '/identitylinks'), undefined, signal)
}

// 新增实例身份链接。
// @param id 实例 ID
// @param payload 类型与用户/组
// @return 新链接
export function addInstanceIdentityLink(
  id: string,
  payload: { type: string; userId?: string; groupId?: string },
): Promise<IdentityLink> {
  return postJSON<IdentityLink>(instancePath(id, '/identitylinks'), payload)
}

// 删除实例身份链接。
// @param id 实例 ID
// @param link 身份链接，必须提供 userId 或 groupId
// @return 无
export function deleteInstanceIdentityLink(id: string, link: IdentityLink): Promise<void> {
  const family = link.userId ? 'users' : 'groups'
  const identityId = link.userId || link.groupId || ''
  const type = link.type || ''
  return deleteJSON<void>(instancePath(id, `/identitylinks/${family}/${encodeURIComponent(identityId)}/${encodeURIComponent(type)}`))
}

// 查询流程定义列表。
// @param query 查询参数
// @return 分页定义
export function listProcessDefinitions(
  query: { key?: string; tenantId?: string; start?: number; size?: number } = {},
): Promise<Page<ProcessDefinitionSummary>> {
  return getJSON<Page<ProcessDefinitionSummary>>(DEFINITIONS, query)
}

// 查询单个流程定义。
// @param id 定义 ID
// @return 定义摘要
export function getProcessDefinition(id: string): Promise<ProcessDefinitionSummary> {
  return getJSON<ProcessDefinitionSummary>(`${DEFINITIONS}/${encodeURIComponent(id)}`)
}

// 拉取可启动定义：按 Key 取最新版本并排除挂起定义。
// @return 最新定义列表
export async function listLatestDefinitions(): Promise<ProcessDefinitionSummary[]> {
  const page = await listProcessDefinitions({ start: 0, size: 1000 })
  const latest = new Map<string, ProcessDefinitionSummary>()
  for (const definition of page.data ?? []) {
    // 挂起定义不能启动，直接跳过。
    if (definition.suspended) continue
    const current = latest.get(definition.key)
    // 仅保留版本号更大的定义。
    if (!current || (definition.version ?? 0) > (current.version ?? 0)) latest.set(definition.key, definition)
  }
  return [...latest.values()].sort((left, right) => left.key.localeCompare(right.key))
}

// 读取流程定义 BPMN XML。
// @param definitionId 定义 ID
// @param signal 取消信号
// @return XML 文本
export function getProcessDefinitionXml(definitionId: string, signal?: AbortSignal): Promise<string> {
  return getText(`${DEFINITIONS}/${encodeURIComponent(definitionId)}/xml`, undefined, signal)
}

// 查询实例运行态活动。
// @param instanceId 实例 ID
// @return 分页活动
export function listRuntimeActivities(instanceId: string): Promise<Page<ActivityRow>> {
  return getJSON<Page<ActivityRow>>('/service/runtime/activity-instances', {
    processInstanceId: instanceId,
    start: 0,
    size: 1000,
  })
}

// 查询实例历史活动，用于补全已完成节点。
// @param instanceId 实例 ID
// @return 分页活动
export function listHistoricActivities(instanceId: string): Promise<Page<ActivityRow>> {
  return getJSON<Page<ActivityRow>>('/service/history/historic-activity-instances', {
    processInstanceId: instanceId,
    start: 0,
    size: 1000,
    sort: 'startTime',
    order: 'asc',
  })
}

// 查询死信作业，用于把异常活动标红。
// @return 分页死信作业
export function listDeadletterJobs(): Promise<Page<DeadletterJob>> {
  return getJSON<Page<DeadletterJob>>('/service/management/deadletter-jobs', { start: 0, size: 1000 })
}

// 校验实例迁移。
// @param id 实例 ID
// @param payload 迁移载荷
// @return 校验信息
export function validateInstanceMigration(id: string, payload: MigrationPayload): Promise<MigrationValidation> {
  return postJSON<MigrationValidation>(instancePath(id, '/validate-migration'), payload)
}

// 执行实例迁移。
// @param id 实例 ID
// @param payload 迁移载荷
// @return 无
export function migrateInstance(id: string, payload: MigrationPayload): Promise<void> {
  return postJSON<void>(instancePath(id, '/migrate'), payload)
}
