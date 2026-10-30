// 运行态任务 REST 封装：列表查询、独立任务、动作、变量、评论、附件与身份链接。
// 所有路径以 /service 开头，请求同源，不注入认证与租户头。
import { deleteJSON, getBlob, getJSON, postForm, postJSON, putJSON, request, type Page } from './client'
import type { Attachment, Comment, FormData as TaskForm, IdentityLink, Variable } from './types'

/** 运行态任务 DTO，字段与 internal/rest/task_response.go 对齐。 */
export interface Task extends Record<string, unknown> {
  id: string
  url?: string
  name: string
  description?: string | null
  assignee?: string | null
  owner?: string | null
  priority: number
  createTime: string
  dueDate?: string | null
  claimTime?: string | null
  suspended?: boolean
  tenantId?: string | null
  delegationState?: string | null
  category?: string | null
  formKey?: string | null
  taskDefinitionKey?: string | null
  processInstanceId?: string | null
  processDefinitionId?: string | null
  executionId?: string | null
  parentTaskId?: string | null
  parentTaskUrl?: string | null
  processInstanceUrl?: string | null
  processDefinitionUrl?: string | null
  variables?: Variable[]
}

/** 任务查询条件，与 GET /runtime/tasks 及 POST /query/tasks 共用字段一致。 */
export interface TaskQuery {
  assignee?: string
  candidateUser?: string
  candidateGroup?: string
  processInstanceId?: string
  processDefinitionKey?: string
  nameLike?: string
  start?: number
  size?: number
  /** 排序字段：id/name/description/assignee/owner/priority/createTime/dueDate */
  sort?: string
  order?: 'asc' | 'desc'
}

/** 独立任务创建请求体。 */
export interface TaskCreatePayload {
  name: string
  description?: string
  assignee?: string
  owner?: string
  priority?: number
  /** 到期时间，ISO-8601 */
  dueDate?: string
  category?: string
  formKey?: string
  parentTaskId?: string
  tenantId?: string
}

/** 任务属性局部更新请求；null 表示清空可空字段。 */
export interface TaskUpdatePayload {
  name?: string
  description?: string
  assignee?: string | null
  owner?: string | null
  priority?: number
  dueDate?: string | null
  category?: string
  formKey?: string
}

/** 批量更新请求体，PUT /runtime/tasks 逐条更新，失败即中断。 */
export interface TaskBulkUpdatePayload extends TaskUpdatePayload {
  taskIds: string[]
}

/** 任务动作名称。 */
export type TaskActionName = 'claim' | 'unclaim' | 'delegate' | 'resolve' | 'complete'

/** 任务动作请求体，对应 internal/rest 的 taskActionPayload。 */
export interface TaskActionPayload {
  action: TaskActionName
  /** 官方动作目标处理人（claim/delegate） */
  assignee?: string
  /** 操作人（resolve/complete） */
  userId?: string
  variables?: Variable[]
  localVariables?: Variable[]
  transientVariables?: Variable[]
}

/** 任务事件 DTO。 */
export interface TaskEvent {
  id: number
  action: string
  timestamp: string
  userId?: string
  taskId?: string
  executionId?: string
  processInstanceId?: string
  processDefinitionId?: string
  tenantId?: string
  data?: string
}

/** 任务删除选项。 */
export interface TaskDeleteOptions {
  cascadeHistory?: boolean
  deleteReason?: string
}

/** 任务事件查询条件。 */
export interface TaskEventQuery {
  action?: string
  userId?: string
  start?: number
  size?: number
}

// taskPath 生成任务资源路径。
// @param id 任务 ID
// @param suffix 追加的子资源路径
// @return 完整路径
function taskPath(id: string, suffix = ''): string {
  return `/service/runtime/tasks/${encodeURIComponent(id)}${suffix}`
}

// compact 移除 undefined、null 与空字符串字段。
// @param input 原始对象
// @return 精简后的对象
function compact(input: Record<string, unknown>): Record<string, unknown> {
  const output: Record<string, unknown> = {}
  for (const [key, value] of Object.entries(input)) {
    if (value === undefined || value === null) continue
    if (typeof value === 'string' && value.trim() === '') continue
    output[key] = value
  }
  return output
}

// queryTasks 使用查询端点分页检索运行中任务，支持组合过滤与排序。
// @param query 过滤、分页与排序条件
// @param signal 取消信号
// @return 分页任务
export function queryTasks(query: TaskQuery = {}, signal?: AbortSignal): Promise<Page<Task>> {
  return request<Page<Task>>('/service/query/tasks', { method: 'POST', json: compact({ ...query }), signal })
}

// getTask 查询任务详情。
// @param id 任务 ID
// @param signal 取消信号
// @return 任务详情
export function getTask(id: string, signal?: AbortSignal): Promise<Task> {
  return getJSON<Task>(taskPath(id), undefined, signal)
}

// createTask 创建独立任务。
// @param payload 创建请求
// @return 创建后的任务
export function createTask(payload: TaskCreatePayload): Promise<Task> {
  return postJSON<Task>('/service/runtime/tasks', compact({ ...payload }))
}

// updateTask 局部更新任务属性，null 用于清空办理人或到期日。
// @param id 任务 ID
// @param payload 待更新字段
// @return 更新后的任务
export function updateTask(id: string, payload: TaskUpdatePayload): Promise<Task> {
  return putJSON<Task>(taskPath(id), payload)
}

// deleteTask 删除任务，可选择级联历史并记录原因。
// @param id 任务 ID
// @param options 删除选项
// @return 无
export function deleteTask(id: string, options: TaskDeleteOptions = {}): Promise<void> {
  return deleteJSON<void>(taskPath(id), compact({ ...options }))
}

// bulkUpdateTasks 批量更新任务属性；服务端逐条更新，遇到失败即中断。
// @param payload 任务 ID 列表与待更新字段
// @return 更新后的分页任务
export function bulkUpdateTasks(payload: TaskBulkUpdatePayload): Promise<Page<Task>> {
  return putJSON<Page<Task>>('/service/runtime/tasks', payload)
}

// taskAction 执行签收、取消签收、委派、解决或完成；成功返回 200 空体。
// @param id 任务 ID
// @param payload 动作请求
// @return 无
export function taskAction(id: string, payload: TaskActionPayload): Promise<void> {
  return postJSON<void>(taskPath(id), payload)
}

// getTaskForm 获取任务表单；无表单时服务端返回 400（REST-0406）。
// @param id 任务 ID
// @param signal 取消信号
// @return 表单定义
export function getTaskForm(id: string, signal?: AbortSignal): Promise<TaskForm> {
  return getJSON<TaskForm>(taskPath(id, '/form'), undefined, signal)
}

// listTaskVariables 获取任务局部变量。
// @param id 任务 ID
// @return 变量数组
export function listTaskVariables(id: string): Promise<Variable[]> {
  return getJSON<Variable[]>(taskPath(id, '/variables'))
}

// createTaskVariable 新增任务变量。
// @param id 任务 ID
// @param variable 变量
// @return 服务端回显的变量
export function createTaskVariable(id: string, variable: Variable): Promise<Variable> {
  return postJSON<Variable>(taskPath(id, '/variables'), variable)
}

// updateTaskVariable 更新指定名称的任务变量。
// @param id 任务 ID
// @param name 变量名
// @param value 新值
// @return 无
export function updateTaskVariable(id: string, name: string, value: unknown): Promise<void> {
  return putJSON<void>(taskPath(id, `/variables/${encodeURIComponent(name)}`), { value })
}

// deleteTaskVariable 删除指定名称的任务变量。
// @param id 任务 ID
// @param name 变量名
// @return 无
export function deleteTaskVariable(id: string, name: string): Promise<void> {
  return deleteJSON<void>(taskPath(id, `/variables/${encodeURIComponent(name)}`))
}

// getTaskVariableData 获取任务变量原始字节。
// @param id 任务 ID
// @param name 变量名
// @return 原始内容
export function getTaskVariableData(id: string, name: string): Promise<Blob> {
  return getBlob(taskPath(id, `/variables/${encodeURIComponent(name)}/data`))
}

// listTaskComments 分页获取任务评论。
// @param id 任务 ID
// @param start 偏移量
// @param size 条数
// @return 分页评论
export function listTaskComments(id: string, start = 0, size = 50): Promise<Page<Comment>> {
  return getJSON<Page<Comment>>(taskPath(id, '/comments'), { start, size })
}

// addTaskComment 新增任务评论。
// @param id 任务 ID
// @param message 评论内容
// @param userId 作者
// @return 创建后的评论
export function addTaskComment(id: string, message: string, userId?: string): Promise<Comment> {
  return postJSON<Comment>(taskPath(id, '/comments'), compact({ message, userId }))
}

// deleteTaskComment 删除任务评论。
// @param id 任务 ID
// @param commentId 评论 ID
// @return 无
export function deleteTaskComment(id: string, commentId: string): Promise<void> {
  return deleteJSON<void>(taskPath(id, `/comments/${encodeURIComponent(commentId)}`))
}

// listTaskAttachments 分页获取任务附件。
// @param id 任务 ID
// @param start 偏移量
// @param size 条数
// @return 分页附件
export function listTaskAttachments(id: string, start = 0, size = 50): Promise<Page<Attachment>> {
  return getJSON<Page<Attachment>>(taskPath(id, '/attachments'), { start, size })
}

// uploadTaskAttachment 以 multipart 上传任务附件，文件字段名任意，元数据走普通字段。
// @param id 任务 ID
// @param file 文件
// @param meta 名称、描述与类型
// @return 创建后的附件
export function uploadTaskAttachment(
  id: string,
  file: File,
  meta: { name?: string; description?: string; type?: string } = {},
): Promise<Attachment> {
  const form = new FormData()
  form.append('file', file)
  if (meta.name) form.append('name', meta.name)
  if (meta.description) form.append('description', meta.description)
  if (meta.type) form.append('type', meta.type)
  return postForm<Attachment>(taskPath(id, '/attachments'), form)
}

// deleteTaskAttachment 删除任务附件。
// @param id 任务 ID
// @param attachmentId 附件 ID
// @return 无
export function deleteTaskAttachment(id: string, attachmentId: string): Promise<void> {
  return deleteJSON<void>(taskPath(id, `/attachments/${encodeURIComponent(attachmentId)}`))
}

// getTaskAttachmentContent 下载附件内容。
// @param id 任务 ID
// @param attachmentId 附件 ID
// @return 二进制内容
export function getTaskAttachmentContent(id: string, attachmentId: string): Promise<Blob> {
  return getBlob(taskPath(id, `/attachments/${encodeURIComponent(attachmentId)}/content`))
}

// listTaskIdentityLinks 获取任务参与者（候选人/候选组）。
// @param id 任务 ID
// @return 身份链接数组
export function listTaskIdentityLinks(id: string): Promise<IdentityLink[]> {
  return getJSON<IdentityLink[]>(taskPath(id, '/identitylinks'))
}

// addTaskCandidate 添加候选用户或候选组。
// @param id 任务 ID
// @param candidate 候选人（userId）或候选组（groupId）
// @return 无
export function addTaskCandidate(id: string, candidate: { userId?: string; groupId?: string }): Promise<void> {
  return postJSON<void>(taskPath(id, '/identitylinks'), { type: 'candidate', ...candidate })
}

// deleteTaskIdentityLink 移除候选用户或候选组。
// @param id 任务 ID
// @param family users 或 groups
// @param identityId 用户或组 ID
// @return 无
export function deleteTaskIdentityLink(id: string, family: 'users' | 'groups', identityId: string): Promise<void> {
  return deleteJSON<void>(taskPath(id, `/identitylinks/${family}/${encodeURIComponent(identityId)}`))
}

// listSubTasks 获取子任务，无分页。
// @param id 父任务 ID
// @return 子任务数组
export function listSubTasks(id: string): Promise<Task[]> {
  return getJSON<Task[]>(taskPath(id, '/subtasks'))
}

// listTaskEvents 获取任务事件；传 start/size 时服务端才切片。
// @param id 任务 ID
// @param query 动作、操作人与分页
// @return 事件数组
export function listTaskEvents(id: string, query: TaskEventQuery = {}): Promise<TaskEvent[]> {
  return getJSON<TaskEvent[]>(taskPath(id, '/events'), compact({ ...query }))
}