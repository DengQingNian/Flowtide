// 历史域 REST 封装：历史流程、任务、活动、变量、明细及其变量数据、评论、身份链接与删除。
// 路径与字段以 internal/rest/server.go 路由和 history_response.go 响应为准。
import { deleteJSON, getBlob, getJSON, postJSON } from './client'
import type { Page, PageQuery } from './client'
import type { FormData } from './types'

/** 历史分页基础参数 */
export type HistoricPageQuery = PageQuery & {
  /** 偏移量 */
  start?: number
  /** 每页条数 */
  size?: number
  /** 排序字段，历史查询支持 id/startTime/endTime 等 */
  sort?: string
  /** 排序方向 */
  order?: 'asc' | 'desc' | string
}

/** 历史变量值（DTO 内嵌的 variable 字段） */
export type HistoricVariableValue = {
  /** 变量名 */
  name: string
  /** 作用域：global 或 local */
  scope?: string | null
  /** 变量类型 */
  type?: string | null
  /** 变量值；二进制为空 */
  value?: unknown
  /** 二进制值的原始数据地址 */
  valueUrl?: string | null
}

/** 历史流程实例 DTO */
export type HistoricProcessInstance = {
  /** 实例 ID */
  id: string
  /** 资源地址 */
  url?: string
  /** 业务 Key */
  businessKey?: string | null
  /** 业务状态 */
  businessStatus?: string | null
  /** 回调标识 */
  callbackId?: string | null
  /** 回调类型 */
  callbackType?: string | null
  /** 签收时间 */
  claimTime?: string | null
  /** 签收人 */
  claimedBy?: string | null
  /** 删除原因 */
  deleteReason?: string | null
  /** 到期时间 */
  dueDate?: string | null
  /** 持续毫秒；0 输出 null */
  durationInMillis?: number | null
  /** 结束活动 ID */
  endActivityId?: string | null
  /** 结束时间 */
  endTime?: string | null
  /** 结束操作人 */
  endUserId?: string | null
  /** 实例名称 */
  name?: string | null
  /** 流程定义 ID */
  processDefinitionId: string
  /** 流程定义名称；定义缺失时为 null */
  processDefinitionName?: string | null
  /** 流程定义说明 */
  processDefinitionDescription?: string | null
  /** 流程定义资源地址 */
  processDefinitionUrl?: string | null
  /** 传播阶段实例 */
  propagatedStageInstanceId?: string | null
  /** 外部引用 */
  referenceId?: string | null
  /** 外部引用类型 */
  referenceType?: string | null
  /** 开始活动 ID */
  startActivityId?: string | null
  /** 开始时间 */
  startTime: string
  /** 发起人 */
  startUserId?: string | null
  /** 实例状态：running/completed */
  state?: string | null
  /** 父流程实例 */
  superProcessInstanceId?: string | null
  /** 租户 */
  tenantId?: string
  /** 变量集合，GET 列表恒为空数组 */
  variables?: HistoricVariableValue[]
}

/** 历史流程实例查询参数（POST /service/query/historic-process-instances） */
export type HistoricProcessQuery = HistoricPageQuery & {
  /** 实例 ID */
  id?: string
  /** 流程定义 ID */
  processDefinitionId?: string
  /** 流程定义 Key */
  processDefinitionKey?: string
  /** 业务 Key */
  businessKey?: string
  /** 业务 Key 模糊匹配 */
  businessKeyLike?: string
  /** 实例名称 */
  name?: string
  /** 实例名称模糊匹配 */
  nameLike?: string
  /** 删除原因 */
  deleteReason?: string
  /** 租户 */
  tenantId?: string
  /** 是否已结束 */
  finished?: boolean
  /** 开始时间下界，RFC3339 或 2006-01-02T15:04:05 */
  startedAfter?: string
  /** 开始时间上界 */
  startedBefore?: string
  /** 结束时间下界 */
  endedAfter?: string
  /** 结束时间上界 */
  endedBefore?: string
  /** 变量过滤条件 */
  variables?: HistoricVariableCondition[]
  /** 流程定义 Key 集合 */
  processDefinitionKeyIn?: string[]
}

/** 历史变量过滤条件 */
export type HistoricVariableCondition = {
  /** 变量名 */
  name: string
  /** 变量值 */
  value?: unknown
  /** 比较方式，缺省 equals */
  operation?: string
}

/** 历史任务 DTO */
export type HistoricTaskInstance = {
  /** 任务 ID */
  id: string
  /** 资源地址 */
  url?: string
  /** 办理人 */
  assignee?: string | null
  /** 分类 */
  category?: string | null
  /** 签收时间 */
  claimTime?: string | null
  /** 删除原因 */
  deleteReason?: string | null
  /** 任务描述 */
  description?: string | null
  /** 到期时间 */
  dueDate?: string | null
  /** 持续毫秒；0 输出 null */
  durationInMillis?: number | null
  /** 结束时间 */
  endTime?: string | null
  /** 执行 ID */
  executionId?: string | null
  /** 表单 Key */
  formKey?: string | null
  /** 任务名称 */
  name?: string | null
  /** 所有者 */
  owner?: string | null
  /** 父任务 ID */
  parentTaskId?: string | null
  /** 优先级 */
  priority?: number
  /** 流程定义 ID */
  processDefinitionId?: string | null
  /** 流程定义资源地址 */
  processDefinitionUrl?: string | null
  /** 流程实例 ID */
  processInstanceId?: string | null
  /** 流程实例资源地址 */
  processInstanceUrl?: string | null
  /** 任务定义 Key */
  taskDefinitionKey?: string | null
  /** 开始时间 */
  startTime: string
  /** 租户 */
  tenantId?: string
  /** 变量集合，需查询体开启 includeTaskLocalVariables/includeProcessVariables */
  variables?: HistoricVariableValue[]
  /** 工作时长，服务端恒为 null */
  workTimeInMillis?: number | null
}

/** 历史任务查询参数（POST /service/query/historic-task-instances） */
export type HistoricTaskQuery = HistoricPageQuery & {
  /** 任务 ID */
  id?: string
  /** 任务 ID 别名 */
  taskId?: string
  /** 任务名称 */
  name?: string
  /** 任务名称别名 */
  taskName?: string
  /** 任务名称模糊匹配 */
  nameLike?: string
  /** 办理人 */
  assignee?: string
  /** 办理人模糊匹配 */
  assigneeLike?: string
  /** 任务定义 Key */
  taskDefinitionKey?: string
  /** 流程实例 ID */
  processInstanceId?: string
  /** 流程定义 Key */
  processDefinitionKey?: string
  /** 租户 */
  tenantId?: string
  /** 是否已结束 */
  finished?: boolean
  /** 开始时间下界 */
  startedAfter?: string
  /** 开始时间上界 */
  startedBefore?: string
  /** 结束时间下界 */
  endedAfter?: string
  /** 结束时间上界 */
  endedBefore?: string
  /** 是否填充任务局部变量 */
  includeTaskLocalVariables?: boolean
  /** 是否填充流程变量 */
  includeProcessVariables?: boolean
  /** 变量过滤条件 */
  variables?: HistoricVariableCondition[]
}

/** 历史活动 DTO */
export type HistoricActivityInstance = {
  /** 历史活动 ID */
  id: string
  /** 活动 ID */
  activityId: string
  /** 活动名称 */
  activityName?: string | null
  /** 活动类型 */
  activityType: string
  /** 办理人 */
  assignee?: string | null
  /** 调用活动实例 ID */
  calledProcessInstanceId?: string | null
  /** 持续毫秒；进行中为 null */
  durationInMillis?: number | null
  /** 结束时间 */
  endTime?: string | null
  /** 执行 ID */
  executionId?: string | null
  /** 流程定义 ID */
  processDefinitionId?: string | null
  /** 流程定义资源地址 */
  processDefinitionUrl?: string | null
  /** 流程实例 ID */
  processInstanceId: string
  /** 流程实例资源地址 */
  processInstanceUrl?: string | null
  /** 开始时间 */
  startTime: string
  /** 对应任务 ID */
  taskId?: string | null
  /** 租户 */
  tenantId?: string
}

/** 历史活动查询参数（POST /service/query/historic-activity-instances） */
export type HistoricActivityQuery = HistoricPageQuery & {
  /** 历史活动 ID */
  id?: string
  /** 流程实例 ID */
  processInstanceId?: string
  /** 流程定义 ID */
  processDefinitionId?: string
  /** 流程定义 Key */
  processDefinitionKey?: string
  /** 活动 ID */
  activityId?: string
  /** 活动名称 */
  activityName?: string
  /** 活动名称模糊匹配 */
  activityNameLike?: string
  /** 活动类型 */
  activityType?: string
  /** 租户 */
  tenantId?: string
  /** 是否已结束 */
  finished?: boolean
  /** 开始时间下界 */
  startedAfter?: string
  /** 开始时间上界 */
  startedBefore?: string
  /** 结束时间下界 */
  endedAfter?: string
  /** 结束时间上界 */
  endedBefore?: string
}

/** 历史变量实体（GET 历史流程实例变量时返回裸实体） */
export type HistoricVariableRecord = {
  /** 变量 ID */
  id: string
  /** 变量名 */
  name: string
  /** 变量类型 */
  type?: string | null
  /** 流程实例 ID */
  processInstanceId?: string
  /** 执行 ID */
  executionId?: string
  /** 任务 ID */
  taskId?: string
  /** 变量值 */
  value?: unknown
  /** 创建时间 */
  createTime?: string
  /** 最后更新时间 */
  lastUpdatedTime?: string
}

/** 历史变量 DTO（查询端点嵌套 variable） */
export type HistoricVariableInstance = {
  /** 变量实例 ID */
  id: string
  /** 执行 ID */
  executionId?: string | null
  /** 流程实例 ID */
  processInstanceId: string
  /** 流程实例资源地址 */
  processInstanceUrl?: string | null
  /** 任务 ID */
  taskId?: string | null
  /** 变量值对象 */
  variable: HistoricVariableValue
}

/** 历史变量查询参数（POST /service/query/historic-variable-instances） */
export type HistoricVariableQuery = HistoricPageQuery & {
  /** 流程实例 ID */
  processInstanceId?: string
  /** 执行 ID */
  executionId?: string
  /** 任务 ID */
  taskId?: string
  /** 变量名 */
  name?: string
  /** 变量名别名 */
  variableName?: string
  /** 变量名模糊匹配 */
  nameLike?: string
  /** 变量类型 */
  type?: string
  /** 变量类型别名 */
  variableType?: string
}

/** 历史明细 DTO */
export type HistoricDetail = {
  /** 明细 ID */
  id: string
  /** 活动实例 ID，服务端恒为 null */
  activityInstanceId?: string | null
  /** 明细类型 */
  detailType?: string | null
  /** 执行 ID */
  executionId?: string | null
  /** 流程实例 ID */
  processInstanceId?: string | null
  /** 流程实例资源地址 */
  processInstanceUrl?: string | null
  /** 属性 ID，服务端恒为 null */
  propertyId?: string | null
  /** 属性值，服务端恒为 null */
  propertyValue?: unknown
  /** 修订号 */
  revision?: number
  /** 任务 ID */
  taskId?: string | null
  /** 任务资源地址 */
  taskUrl?: string | null
  /** 记录时间 */
  time: string
  /** 变量值对象 */
  variable: HistoricVariableValue
}

/** 历史明细查询参数（POST /service/query/historic-detail） */
export type HistoricDetailQuery = HistoricPageQuery & {
  /** 流程实例 ID */
  processInstanceId?: string
  /** 执行 ID */
  executionId?: string
  /** 任务 ID */
  taskId?: string
  /** 活动实例 ID */
  activityInstanceId?: string
  /** 活动 ID */
  activityId?: string
  /** 变量名 */
  name?: string
  /** 变量名别名 */
  variableName?: string
  /** 变量名模糊匹配 */
  nameLike?: string
  /** 变量类型 */
  type?: string
  /** 变量类型别名 */
  variableType?: string
  /** 记录时间下界 */
  timeAfter?: string
  /** 记录时间上界 */
  timeBefore?: string
}

/** 历史评论 DTO */
export type HistoricComment = {
  /** 评论 ID */
  id: string
  /** 评论人 */
  author?: string | null
  /** 评论内容 */
  message?: string | null
  /** 流程实例 ID */
  processInstanceId?: string | null
  /** 流程实例资源地址 */
  processInstanceUrl?: string | null
  /** 任务 ID */
  taskId?: string | null
  /** 任务资源地址 */
  taskUrl?: string | null
  /** 评论时间 */
  time: string
}

/** 历史身份链接 DTO */
export type HistoricIdentityLink = {
  /** 组 ID */
  groupId?: string | null
  /** 链接类型 */
  type: string
  /** 用户 ID */
  userId?: string | null
  /** 流程实例 ID */
  processInstanceId?: string | null
  /** 流程实例资源地址 */
  processInstanceUrl?: string | null
  /** 任务 ID */
  taskId?: string | null
  /** 任务资源地址 */
  taskUrl?: string | null
}

/** 历史流程实例基础路径 */
const PROCESS_PATH = '/service/history/historic-process-instances'
/** 历史任务基础路径 */
const TASK_PATH = '/service/history/historic-task-instances'
/** 历史活动基础路径 */
const ACTIVITY_PATH = '/service/history/historic-activity-instances'
/** 历史变量基础路径 */
const VARIABLE_PATH = '/service/history/historic-variable-instances'
/** 历史明细基础路径 */
const DETAIL_PATH = '/service/history/historic-detail'

// compactQuery 去掉空字符串与空值但保留 false 与 0，避免过滤条件被误删。
// @param query 查询对象
// @return 精简后的查询对象
function compactQuery<T extends Record<string, unknown>>(query: T): Record<string, unknown> {
  const result: Record<string, unknown> = {}
  for (const [key, value] of Object.entries(query)) {
    // 空值不参与查询，false 与 0 属于有效条件
    if (value === undefined || value === null || value === '') continue
    result[key] = value
  }
  return result
}

// encodeID 编码路径参数，避免特殊字符破坏路由。
// @param id 资源标识
// @return 编码后的路径片段
function encodeID(id: string): string {
  return encodeURIComponent(id)
}

// queryHistoricProcessInstances 查询历史流程实例（复杂筛选必须走查询端点）。
// @param query 过滤与分页条件
// @return 历史流程实例分页结果
export function queryHistoricProcessInstances(query: HistoricProcessQuery = {}): Promise<Page<HistoricProcessInstance>> {
  return postJSON<Page<HistoricProcessInstance>>('/service/query/historic-process-instances', compactQuery(query))
}

// getHistoricProcessInstance 查询单个历史流程实例。
// @param id 实例 ID
// @param signal 取消信号
// @return 历史流程实例
export function getHistoricProcessInstance(id: string, signal?: AbortSignal): Promise<HistoricProcessInstance> {
  return getJSON<HistoricProcessInstance>(`${PROCESS_PATH}/${encodeID(id)}`, undefined, signal)
}

// listHistoricProcessVariables 查询历史流程实例变量（服务端返回裸变量实体）。
// @param id 实例 ID
// @param query 分页参数
// @param signal 取消信号
// @return 变量实体分页结果
export function listHistoricProcessVariables(
  id: string,
  query: HistoricPageQuery = {},
  signal?: AbortSignal,
): Promise<Page<HistoricVariableRecord>> {
  return getJSON<Page<HistoricVariableRecord>>(`${PROCESS_PATH}/${encodeID(id)}/variables`, compactQuery(query), signal)
}

// getHistoricProcessVariableData 读取历史流程实例变量原始值。
// @param id 实例 ID
// @param name 变量名
// @return 原始二进制数据
export function getHistoricProcessVariableData(id: string, name: string): Promise<Blob> {
  return getBlob(`${PROCESS_PATH}/${encodeID(id)}/variables/${encodeID(name)}/data`)
}

// listHistoricProcessComments 查询历史流程实例评论。
// @param id 实例 ID
// @param signal 取消信号
// @return 评论数组
export function listHistoricProcessComments(id: string, signal?: AbortSignal): Promise<HistoricComment[]> {
  return getJSON<HistoricComment[]>(`${PROCESS_PATH}/${encodeID(id)}/comments`, undefined, signal)
}

// listHistoricProcessIdentityLinks 查询历史流程实例身份链接。
// @param id 实例 ID
// @param signal 取消信号
// @return 身份链接数组
export function listHistoricProcessIdentityLinks(id: string, signal?: AbortSignal): Promise<HistoricIdentityLink[]> {
  return getJSON<HistoricIdentityLink[]>(`${PROCESS_PATH}/${encodeID(id)}/identitylinks`, undefined, signal)
}

// deleteHistoricProcessInstance 删除历史流程实例；运行中实例服务端返回 500 文本。
// @param id 实例 ID
// @return 无
export function deleteHistoricProcessInstance(id: string): Promise<void> {
  return deleteJSON<void>(`${PROCESS_PATH}/${encodeID(id)}`)
}

// queryHistoricTaskInstances 查询历史任务。
// @param query 过滤与分页条件
// @return 历史任务分页结果
export function queryHistoricTaskInstances(query: HistoricTaskQuery = {}): Promise<Page<HistoricTaskInstance>> {
  return postJSON<Page<HistoricTaskInstance>>('/service/query/historic-task-instances', compactQuery(query))
}

// getHistoricTaskInstance 查询单个历史任务。
// @param id 任务 ID
// @param signal 取消信号
// @return 历史任务
export function getHistoricTaskInstance(id: string, signal?: AbortSignal): Promise<HistoricTaskInstance> {
  return getJSON<HistoricTaskInstance>(`${TASK_PATH}/${encodeID(id)}`, undefined, signal)
}

// getHistoricTaskForm 查询历史任务表单；引擎未内置表单服务时服务端固定返回 400。
// @param id 任务 ID
// @param signal 取消信号
// @return 表单数据
export function getHistoricTaskForm(id: string, signal?: AbortSignal): Promise<FormData> {
  return getJSON<FormData>(`${TASK_PATH}/${encodeID(id)}/form`, undefined, signal)
}

// listHistoricTaskVariables 通过查询端点填充历史任务变量（服务端没有任务变量列表端点）。
// @param id 任务 ID
// @return 变量值数组
export async function listHistoricTaskVariables(id: string): Promise<HistoricVariableValue[]> {
  const page = await postJSON<Page<HistoricTaskInstance>>('/service/query/historic-task-instances', {
    id,
    includeTaskLocalVariables: true,
    includeProcessVariables: true,
    start: 0,
    size: 1,
  })
  return page.data[0]?.variables ?? []
}

// getHistoricTaskVariableData 读取历史任务变量原始值。
// @param id 任务 ID
// @param name 变量名
// @return 原始二进制数据
export function getHistoricTaskVariableData(id: string, name: string): Promise<Blob> {
  return getBlob(`${TASK_PATH}/${encodeID(id)}/variables/${encodeID(name)}/data`)
}

// listHistoricTaskIdentityLinks 查询历史任务身份链接。
// @param id 任务 ID
// @param signal 取消信号
// @return 身份链接数组
export function listHistoricTaskIdentityLinks(id: string, signal?: AbortSignal): Promise<HistoricIdentityLink[]> {
  return getJSON<HistoricIdentityLink[]>(`${TASK_PATH}/${encodeID(id)}/identitylinks`, undefined, signal)
}

// deleteHistoricTaskInstance 删除历史任务。
// @param id 任务 ID
// @return 无
export function deleteHistoricTaskInstance(id: string): Promise<void> {
  return deleteJSON<void>(`${TASK_PATH}/${encodeID(id)}`)
}

// queryHistoricActivities 查询历史活动。
// @param query 过滤与分页条件
// @return 历史活动分页结果
export function queryHistoricActivities(query: HistoricActivityQuery = {}): Promise<Page<HistoricActivityInstance>> {
  return postJSON<Page<HistoricActivityInstance>>('/service/query/historic-activity-instances', compactQuery(query))
}

// getHistoricActivity 查询单条历史活动。
// @param id 历史活动 ID
// @param signal 取消信号
// @return 历史活动
export function getHistoricActivity(id: string, signal?: AbortSignal): Promise<HistoricActivityInstance> {
  return getJSON<HistoricActivityInstance>(`${ACTIVITY_PATH}/${encodeID(id)}`, undefined, signal)
}

// queryHistoricVariables 查询历史变量实例。
// @param query 过滤与分页条件
// @return 历史变量分页结果
export function queryHistoricVariables(query: HistoricVariableQuery = {}): Promise<Page<HistoricVariableInstance>> {
  return postJSON<Page<HistoricVariableInstance>>('/service/query/historic-variable-instances', compactQuery(query))
}

// getHistoricVariable 查询单条历史变量。
// @param id 变量 ID
// @param signal 取消信号
// @return 历史变量
export function getHistoricVariable(id: string, signal?: AbortSignal): Promise<HistoricVariableInstance> {
  return getJSON<HistoricVariableInstance>(`${VARIABLE_PATH}/${encodeID(id)}`, undefined, signal)
}

// getHistoricVariableData 读取历史变量原始值。
// @param id 变量 ID
// @return 原始二进制数据
export function getHistoricVariableData(id: string): Promise<Blob> {
  return getBlob(`${VARIABLE_PATH}/${encodeID(id)}/data`)
}

// deleteHistoricVariable 删除历史变量。
// @param id 变量 ID
// @return 无
export function deleteHistoricVariable(id: string): Promise<void> {
  return deleteJSON<void>(`${VARIABLE_PATH}/${encodeID(id)}`)
}

// queryHistoricDetails 查询历史变量明细。
// @param query 过滤与分页条件
// @return 历史明细分页结果
export function queryHistoricDetails(query: HistoricDetailQuery = {}): Promise<Page<HistoricDetail>> {
  return postJSON<Page<HistoricDetail>>('/service/query/historic-detail', compactQuery(query))
}

// getHistoricDetailData 读取历史明细原始值。
// @param id 明细 ID
// @return 原始二进制数据
export function getHistoricDetailData(id: string): Promise<Blob> {
  return getBlob(`${DETAIL_PATH}/${encodeID(id)}/data`)
}

// deleteHistoricDetail 删除历史明细。
// @param id 明细 ID
// @return 无
export function deleteHistoricDetail(id: string): Promise<void> {
  return deleteJSON<void>(`${DETAIL_PATH}/${encodeID(id)}`)
}
