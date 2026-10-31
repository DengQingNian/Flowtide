// 工作流域（/workflow/** 门面）REST 客户端：R 信封解包、501 空实现识别与分类调用。
// 后端部分端点仍在接线（HTTP 501 + R 失败体），本客户端不抛错，
// 统一返回 WorkflowResult，由页面按 notImplemented 降级为空态、按 success=false 展示 ErrorMessage。
import { request, type RequestOptions } from './client'
import { ApiError } from './client'

/** workflow 门面统一返回信封（api.R，Java enm.R 兼容形态） */
export interface WorkflowEnvelope<T = unknown> {
  /** 状态码："200" 成功 / "500" 失败 */
  code?: string
  /** 成功消息 */
  msg?: string
  /** 失败消息（大写字段名兼容 Java 协议） */
  ErrorMessage?: string
  /** 业务数据 */
  data?: T
  /** 是否成功 */
  success?: boolean
}

/** 解包后的调用结果：页面据此分支渲染，永不抛错 */
export interface WorkflowResult<T = unknown> {
  /** 是否成功：HTTP 2xx 且信封 success != false */
  success: boolean
  /** 业务数据，失败时为 null */
  data: T | null
  /** 成功消息 */
  msg: string
  /** 失败消息（ErrorMessage 或 HTTP 层可读错误） */
  errorMessage: string
  /** 协议/HTTP 层错误码 */
  code: string
  /** HTTP 状态码，网络错误为 0 */
  httpStatus: number
  /** 后端未接线（501）标记，页面据此空态降级而非报错 */
  notImplemented: boolean
}

/** 带行键的列表行：后端行数据可能缺少 id，解包时统一补 _rid 供表格使用 */
export interface KeyedRow {
  /** 合成行键：优先取行内 id，否则取索引 */
  _rid: string
}

// envelopeToResult 把响应体解析为 WorkflowResult。
// 兼容两种形态：R 信封（含 success 字段）与裸数据（如 XML 字符串、数组）。
// @param body 响应体
// @param httpStatus HTTP 状态码
// @return 解包结果
export function envelopeToResult<T>(body: unknown, httpStatus = 200): WorkflowResult<T> {
  if (body && typeof body === 'object' && 'success' in (body as Record<string, unknown>)) {
    const envelope = body as WorkflowEnvelope<T>
    if (envelope.success === false) {
      return {
        success: false,
        data: null,
        msg: '',
        errorMessage: envelope.ErrorMessage || envelope.msg || '接口返回失败',
        code: envelope.code || '',
        httpStatus,
        notImplemented: false,
      }
    }
    return {
      success: true,
      data: (envelope.data ?? null) as T | null,
      msg: envelope.msg ?? '',
      errorMessage: '',
      code: envelope.code || '200',
      httpStatus,
      notImplemented: false,
    }
  }
  // 裸数据：整体视为业务数据（兼容直接返回 XML/数组/对象的端点）
  return { success: true, data: body as T, msg: '', errorMessage: '', code: '200', httpStatus, notImplemented: false }
}

// callWorkflow 发送请求并把任何失败（含 501）折叠为 WorkflowResult，不向上抛错。
// @param path 请求路径
// @param options 请求选项
// @return 解包结果
async function callWorkflow<T>(path: string, options: RequestOptions = {}): Promise<WorkflowResult<T>> {
  try {
    const body = await request<unknown>(path, options)
    return envelopeToResult<T>(body, 200)
  } catch (error) {
    if (error instanceof ApiError) {
      return {
        success: false,
        data: null,
        msg: '',
        errorMessage: error.message,
        code: error.code,
        httpStatus: error.status,
        // 501 = 后端尚未接线，页面据此展示"接口接线中"空态
        notImplemented: error.status === 501,
      }
    }
    // 理论上不会到达：request 层已把异常包装为 ApiError
    return {
      success: false,
      data: null,
      msg: '',
      errorMessage: error instanceof Error ? error.message : String(error),
      code: 'UNKNOWN',
      httpStatus: 0,
      notImplemented: false,
    }
  }
}

// extractRows 从业务数据中兼容提取行数组并补 _rid 行键。
// 兼容：数组、{list}、{records}、{rows}、{items}、{data} 等 Java 分页信封。
// @param data 业务数据
// @return 带行键的行数组
export function extractRows<T extends Record<string, unknown>>(data: unknown): (T & KeyedRow)[] {
  let list: unknown[] = []
  if (Array.isArray(data)) {
    list = data
  } else if (data && typeof data === 'object') {
    const obj = data as Record<string, unknown>
    for (const key of ['list', 'records', 'rows', 'items', 'data']) {
      if (Array.isArray(obj[key])) {
        list = obj[key] as unknown[]
        break
      }
    }
  }
  return list.map((row, index) => {
    const item = (row ?? {}) as T & Record<string, unknown>
    const rid = item.id ?? index + 1
    return { ...item, _rid: String(rid) } as T & KeyedRow
  })
}

// extractTotal 从业务数据中提取总条数；无分页信封时回退为行数。
// @param data 业务数据
// @param rows 已提取的行
// @return 总条数
export function extractTotal(data: unknown, rows: unknown[]): number {
  if (data && typeof data === 'object' && !Array.isArray(data)) {
    const obj = data as Record<string, unknown>
    for (const key of ['total', 'totalCount', 'count']) {
      const value = obj[key]
      if (typeof value === 'number') return value
    }
  }
  return rows.length
}

/** workflow 门面根路径 */
const ROOT = '/workflow'

/** workflow 分页查询参数（Java 兼容 pageNum/pageSize 形态） */
export type WorkflowPageQuery = {
  /** 页码，从 1 开始 */
  pageNum?: number
  /** 每页条数 */
  pageSize?: number
  [key: string]: unknown
}

// joinIds 把 ID 列表编码为路径段（逗号分隔），与 Java delete/{ids} 协议一致。
// @param ids ID 列表
// @return 编码后的路径段
function joinIds(ids: string[]): string {
  return ids.map((id) => encodeURIComponent(id)).join(',')
}

// ── 流程分类（D11：workflow_model 一表两页之分类树投影）──

/** 流程分类实体 */
export type WorkflowCategory = {
  /** 分类 ID */
  id?: string
  /** 父分类 ID，空表示根分类 */
  parentId?: string | null
  /** 分类名称 */
  categoryName?: string
  /** 分类代号 */
  categoryCode?: string
  /** 排序号 */
  sort?: number
  /** 是否可见 */
  visible?: boolean
  /** 是否启用 */
  enabled?: boolean
  /** 备注 */
  remark?: string
  [key: string]: unknown
}

/** 分类新增/编辑载荷 */
export type WorkflowCategoryPayload = {
  /** 分类 ID，编辑时必填 */
  id?: string
  /** 父分类 ID */
  parentId?: string | null
  /** 分类名称（必填） */
  categoryName: string
  /** 分类代号（必填） */
  categoryCode: string
  /** 排序号 */
  sort?: number
  /** 是否可见 */
  visible?: boolean
  /** 是否启用 */
  enabled?: boolean
  /** 备注 */
  remark?: string
  [key: string]: unknown
}

// listWorkflowCategories 查询分类列表（平铺，页面自行组树）。
// @param query 过滤参数（如 parentId、关键字）
// @return 分类列表结果
export function listWorkflowCategories(query?: Record<string, unknown>): Promise<WorkflowResult<unknown>> {
  return callWorkflow(`${ROOT}/category/list`, { query })
}

// addWorkflowCategory 新增分类。
// @param payload 分类字段
// @return 新增结果
export function addWorkflowCategory(payload: WorkflowCategoryPayload): Promise<WorkflowResult<unknown>> {
  return callWorkflow(`${ROOT}/category/add`, { method: 'POST', json: payload })
}

// editWorkflowCategory 编辑分类。
// @param payload 含 id 的分类字段
// @return 编辑结果
export function editWorkflowCategory(payload: WorkflowCategoryPayload): Promise<WorkflowResult<unknown>> {
  return callWorkflow(`${ROOT}/category/edit`, { method: 'POST', json: payload })
}

// deleteWorkflowCategories 批量删除分类。
// @param ids 待删除的分类 ID
// @return 删除结果
export function deleteWorkflowCategories(ids: string[]): Promise<WorkflowResult<unknown>> {
  return callWorkflow(`${ROOT}/category/delete/${joinIds(ids)}`, { method: 'DELETE' })
}

// ── 流程模型（D11 模型页）──

/** 流程模型实体（/workflow/model 投影） */
export type WorkflowModel = {
  /** 模型 ID */
  id?: string
  /** 模型名称 */
  modelName?: string
  /** 模型代号 */
  modelCode?: string
  /** 所属分类/系统 ID */
  categoryId?: string
  /** 所属分类名称 */
  categoryName?: string
  /** 是否启用 */
  enabled?: boolean
  /** 排序号 */
  sort?: number
  /** 租户（业务系统代号） */
  tenantId?: string
  /** 最近部署时间 */
  lastDeployTime?: string
  /** 备注 */
  remark?: string
  [key: string]: unknown
}

/** 模型新增/编辑载荷 */
export type WorkflowModelPayload = {
  /** 模型 ID，编辑时必填 */
  id?: string
  /** 模型名称（必填） */
  modelName: string
  /** 模型代号（必填） */
  modelCode: string
  /** 所属分类 ID */
  categoryId?: string | null
  /** 是否启用 */
  enabled?: boolean
  /** 排序号 */
  sort?: number
  /** 租户 */
  tenantId?: string
  /** 备注 */
  remark?: string
  [key: string]: unknown
}

/** 流程部署请求体 */
export type DeployProcessPayload = {
  /** 模型 ID（必填） */
  modelId: string
  /** 租户（业务系统代号） */
  tenantId?: string
  [key: string]: unknown
}

// listWorkflowModels 查询模型列表。
// @param query 过滤与分页参数（如 modelName/modelCode/页码）
// @return 模型列表结果
export function listWorkflowModels(query?: WorkflowPageQuery): Promise<WorkflowResult<unknown>> {
  return callWorkflow(`${ROOT}/model/list`, { query })
}

// addWorkflowModel 新增模型。
// @param payload 模型字段
// @return 新增结果
export function addWorkflowModel(payload: WorkflowModelPayload): Promise<WorkflowResult<unknown>> {
  return callWorkflow(`${ROOT}/model/add`, { method: 'POST', json: payload })
}

// editWorkflowModel 编辑模型。
// @param payload 含 id 的模型字段
// @return 编辑结果
export function editWorkflowModel(payload: WorkflowModelPayload): Promise<WorkflowResult<unknown>> {
  return callWorkflow(`${ROOT}/model/edit`, { method: 'POST', json: payload })
}

// deleteWorkflowModels 批量删除模型。
// @param ids 待删除的模型 ID
// @return 删除结果
export function deleteWorkflowModels(ids: string[]): Promise<WorkflowResult<unknown>> {
  return callWorkflow(`${ROOT}/model/delete/${joinIds(ids)}`, { method: 'DELETE' })
}

// deployWorkflowProcess 部署模型为流程定义（D8 同时保留 deplay 兼容拼写，本客户端走规范别名 deploy）。
// @param payload 模型 ID 与租户
// @return 部署结果（失败时页面提示 ErrorMessage）
export function deployWorkflowProcess(payload: DeployProcessPayload): Promise<WorkflowResult<unknown>> {
  return callWorkflow(`${ROOT}/process/deploy`, { method: 'POST', json: payload })
}

// ── 步骤管理（流程配置 Tab1）──

/** 流程步骤实体（核心字段子集） */
export type WorkflowStep = {
  /** 步骤 ID */
  id?: string
  /** 所属模型 ID */
  modelId?: string
  /** 步骤名称 */
  stepName?: string
  /** 步骤代号 */
  stepCode?: string
  /** 排序号 */
  sort?: number
  /** 是否允许退回 */
  allowReturn?: boolean
  /** 可退回的步骤代号集合（逗号分隔或数组） */
  backStepCodes?: string | string[]
  /** 是否需要退回审批 */
  returnApprovalStep?: boolean
  /** 步骤表单/跳转 URL */
  url?: string
  [key: string]: unknown
}

/** 步骤新增/编辑载荷 */
export type WorkflowStepPayload = {
  /** 步骤 ID，编辑时必填 */
  id?: string
  /** 所属模型 ID（必填） */
  modelId: string
  /** 步骤名称（必填） */
  stepName: string
  /** 步骤代号（必填） */
  stepCode: string
  /** 排序号 */
  sort?: number
  /** 是否允许退回 */
  allowReturn?: boolean
  /** 可退回步骤代号（逗号分隔） */
  backStepCodes?: string
  /** 是否需要退回审批 */
  returnApprovalStep?: boolean
  /** 步骤 URL */
  url?: string
  [key: string]: unknown
}

// listWorkflowSteps 查询指定模型的步骤列表（模型态）。
// @param query 过滤参数（modelId 必填、关键字、分页）
// @return 步骤列表结果
export function listWorkflowSteps(query?: WorkflowPageQuery): Promise<WorkflowResult<unknown>> {
  return callWorkflow(`${ROOT}/step/list`, { query })
}

// addWorkflowStep 新增步骤。
// @param payload 步骤字段
// @return 新增结果
export function addWorkflowStep(payload: WorkflowStepPayload): Promise<WorkflowResult<unknown>> {
  return callWorkflow(`${ROOT}/step/add`, { method: 'POST', json: payload })
}

// editWorkflowStep 编辑步骤。
// @param payload 含 id 的步骤字段
// @return 编辑结果
export function editWorkflowStep(payload: WorkflowStepPayload): Promise<WorkflowResult<unknown>> {
  return callWorkflow(`${ROOT}/step/edit`, { method: 'POST', json: payload })
}

// deleteWorkflowSteps 批量删除步骤。
// @param ids 待删除的步骤 ID
// @return 删除结果
export function deleteWorkflowSteps(ids: string[]): Promise<WorkflowResult<unknown>> {
  return callWorkflow(`${ROOT}/step/delete/${joinIds(ids)}`, { method: 'DELETE' })
}

// ── 流程模板（流程配置 Tab2）──

/** 流程模板实体 */
export type WorkflowTemplate = {
  /** 模板 ID */
  id?: string
  /** 模板名称 */
  templateName?: string
  /** 模板代号 */
  templateCode?: string
  /** 关联模型 ID */
  modelId?: string
  /** BPMN XML 内容（列表可只读预览） */
  flowXml?: string
  /** 备注 */
  remark?: string
  /** 更新时间 */
  updateTime?: string
  [key: string]: unknown
}

/** 模板新增/编辑载荷 */
export type WorkflowTemplatePayload = {
  /** 模板 ID，编辑时必填 */
  id?: string
  /** 模板名称（必填） */
  templateName: string
  /** 模板代号（必填） */
  templateCode: string
  /** 关联模型 ID */
  modelId?: string | null
  /** BPMN XML（本期只读预览，不编辑） */
  flowXml?: string
  /** 备注 */
  remark?: string
  [key: string]: unknown
}

// listWorkflowTemplates 查询流程模板列表。
// @param query 过滤与分页参数
// @return 模板列表结果
export function listWorkflowTemplates(query?: WorkflowPageQuery): Promise<WorkflowResult<unknown>> {
  return callWorkflow(`${ROOT}/template/list`, { query })
}

// addWorkflowTemplate 新增流程模板。
// @param payload 模板字段
// @return 新增结果
export function addWorkflowTemplate(payload: WorkflowTemplatePayload): Promise<WorkflowResult<unknown>> {
  return callWorkflow(`${ROOT}/template/add`, { method: 'POST', json: payload })
}

// editWorkflowTemplate 编辑流程模板。
// @param payload 含 id 的模板字段
// @return 编辑结果
export function editWorkflowTemplate(payload: WorkflowTemplatePayload): Promise<WorkflowResult<unknown>> {
  return callWorkflow(`${ROOT}/template/edit`, { method: 'POST', json: payload })
}

// deleteWorkflowTemplates 批量删除流程模板。
// @param ids 待删除的模板 ID
// @return 删除结果
export function deleteWorkflowTemplates(ids: string[]): Promise<WorkflowResult<unknown>> {
  return callWorkflow(`${ROOT}/template/delete/${joinIds(ids)}`, { method: 'DELETE' })
}

// ── 待办 / 已办任务（P1.8 查询门面）──

/** 待办/已办任务实体（P1.8 行投影） */
export type WorkflowTaskRow = {
  /** 任务 ID */
  id?: string
  /** 任务标题 */
  title?: string
  /** 流程实例 ID */
  processInstanceId?: string
  /** 流程名称 */
  processName?: string
  /** 步骤名称 */
  stepName?: string
  /** 发送人 */
  senderName?: string
  /** 发起人 */
  initiatorName?: string
  /** 业务单据 ID */
  businessKey?: string
  /** 下发/创建时间 */
  createTime?: string
  /** 完成时间（已办） */
  endTime?: string
  [key: string]: unknown
}

// listTodoTasks 查询待办任务列表。
// @param query identityId（身份 Id，必填）+ 分页
// @return 待办列表结果
export function listTodoTasks(query?: WorkflowPageQuery): Promise<WorkflowResult<unknown>> {
  return callWorkflow(`${ROOT}/process/task/all/todoList`, { query })
}

// listDoneTasks 查询已办任务列表。
// @param query identityId（身份 Id，必填）+ 分页
// @return 已办列表结果
export function listDoneTasks(query?: WorkflowPageQuery): Promise<WorkflowResult<unknown>> {
  return callWorkflow(`${ROOT}/process/task/all/doneList`, { query })
}

/** 办结任务载荷（CompleteTaskVo 核心字段；本期弹窗只采集意见与变量） */
export type CompleteTaskPayload = {
  /** 任务 ID（必填） */
  taskId: string
  /** 操作人身份 Id（必填） */
  userCode: string
  /** 审批意见（必填） */
  message: string
  /** 流程实例 ID */
  processInstanceId?: string
  /** 意见类型代码（如 SP 审批） */
  type?: string
  /** 流程变量 */
  variables?: Record<string, unknown>
  [key: string]: unknown
}

// completeWorkflowTask 办结（办理）待办任务。
// @param payload 意见、变量与任务标识
// @return 办结结果
export function completeWorkflowTask(payload: CompleteTaskPayload): Promise<WorkflowResult<unknown>> {
  return callWorkflow(`${ROOT}/process/complete/task`, { method: 'POST', json: payload })
}

// ── 流程实例（P1.8 列表 + P2.6 状态操作）──

/** 流程实例实体（P1.8 行投影） */
export type WorkflowInstanceRow = {
  /** 流程实例 ID */
  id?: string
  /** 实例标题 */
  title?: string
  /** 流程定义 Key */
  processDefinitionKey?: string
  /** 流程名称 */
  processName?: string
  /** 业务单据 ID */
  businessKey?: string
  /** 发起人 */
  initiatorName?: string
  /** 状态：0 已结束 / 1 激活 / 2 挂起 */
  status?: number | string
  /** 当前步骤名称 */
  stepName?: string
  /** 发起时间 */
  createTime?: string
  /** 结束时间 */
  endTime?: string
  /** 租户 */
  tenantId?: string
  [key: string]: unknown
}

// listWorkflowInstances 查询流程实例分页（501 时空态降级）。
// @param query 过滤与分页参数
// @return 实例列表结果
export function listWorkflowInstances(query?: WorkflowPageQuery): Promise<WorkflowResult<unknown>> {
  return callWorkflow(`${ROOT}/process/instance/all/list`, { query })
}

/** 实例状态查询结果（按业务键） */
export type ProcessStatusInfo = {
  /** 流程实例 ID */
  processInstanceId?: string
  /** 业务单据 ID */
  businessKey?: string
  /** 状态：0 已结束 / 1 激活 / 2 挂起 */
  status?: number | string
  /** 当前步骤名称 */
  stepName?: string
  [key: string]: unknown
}

// getProcessStatusByBusinessKey 按业务键查询流程状态（public/processes/id 门面）。
// 同时在 query 与 body 携带 businessKey，兼容两种取参实现。
// @param businessKey 业务单据 ID
// @return 状态信息结果
export function getProcessStatusByBusinessKey(businessKey: string): Promise<WorkflowResult<ProcessStatusInfo>> {
  return callWorkflow<ProcessStatusInfo>(`${ROOT}/process/public/processes/id`, {
    method: 'POST',
    query: { businessKey },
    json: { businessKey, businessId: businessKey },
  })
}

// stopWorkflowProcessInstance 终止流程实例。
// @param pid 流程实例 ID
// @return 操作结果
export function stopWorkflowProcessInstance(pid: string): Promise<WorkflowResult<unknown>> {
  return callWorkflow(`${ROOT}/process/stop/processInstance/${encodeURIComponent(pid)}`, { method: 'POST' })
}

// suspendWorkflowProcessInstance 挂起流程实例。
// @param pid 流程实例 ID
// @return 操作结果
export function suspendWorkflowProcessInstance(pid: string): Promise<WorkflowResult<unknown>> {
  return callWorkflow(`${ROOT}/process/suspend/processInstance/${encodeURIComponent(pid)}`, { method: 'POST' })
}

// activateWorkflowProcessInstance 激活流程实例。
// @param pid 流程实例 ID
// @return 操作结果
export function activateWorkflowProcessInstance(pid: string): Promise<WorkflowResult<unknown>> {
  return callWorkflow(`${ROOT}/process/activate/processInstance/${encodeURIComponent(pid)}`, { method: 'POST' })
}

// deleteWorkflowInstance 删除流程实例。
// @param id 流程实例 ID
// @return 操作结果
export function deleteWorkflowInstance(id: string): Promise<WorkflowResult<unknown>> {
  return callWorkflow(`${ROOT}/process/instance/all/delete/${encodeURIComponent(id)}`, { method: 'DELETE' })
}

// ── 流程图高亮（P5.5）──

/** 流程图高亮信息（bpmn XML 与已走/在走节点集合） */
export type HighlightInfo = {
  /** BPMN XML 文本 */
  bpmnXml?: string
  /** 已完成活动 ID */
  completed?: string[]
  /** 当前活动 ID */
  active?: string[]
  [key: string]: unknown
}

// getWorkflowHighlight 查询实例流程图与高亮节点。
// 路由表登记为 DELETE（Java ANY 兼容），优先 GET，404/405 时降级 DELETE 重试。
// @param processInstanceId 流程实例 ID
// @return 高亮信息结果
export async function getWorkflowHighlight(processInstanceId: string): Promise<WorkflowResult<HighlightInfo>> {
  const query = { processInstanceId }
  const primary = await callWorkflow<HighlightInfo>(`${ROOT}/process/highlight/img`, { query })
  if (primary.notImplemented || primary.success) return primary
  // HTTP 层失败（404/405 等）时按 DELETE 方法重试一次
  const fallback = await callWorkflow<HighlightInfo>(`${ROOT}/process/highlight/img`, { method: 'DELETE', query })
  return fallback.notImplemented || fallback.success ? fallback : primary
}

/** BPMN XML 查询参数 */
export type ModelJsonQuery = {
  /** 自定义模型 ID */
  customModelId?: string
  /** 取数类型：0 模型态 / 1 实例态 */
  dataObtainType?: number
  /** 流程实例 ID（实例态） */
  processInstanceId?: string
  [key: string]: unknown
}

// getWorkflowModelJson 读取模型态 BPMN XML（getModelJsonByType 门面）。
// @param query 模型 ID 与取数类型
// @return BPMN XML 文本结果（裸字符串或 R.data）
export function getWorkflowModelJson(query: ModelJsonQuery): Promise<WorkflowResult<string>> {
  return callWorkflow<string>(`${ROOT}/process/getModelJsonByType/`, { query })
}

// getWorkflowHisModelJson 读取实例态 BPMN XML（getHisModelJson 门面）。
// @param query 流程实例 ID
// @return BPMN XML 文本结果
export function getWorkflowHisModelJson(query: ModelJsonQuery): Promise<WorkflowResult<string>> {
  return callWorkflow<string>(`${ROOT}/process/getHisModelJson/`, { query })
}

// ── 委托配置（P4.1-P4.2）──

/** 委托配置实体 */
export type WorkflowDelegate = {
  /** 配置 ID */
  id?: string
  /** 委托人身份 Id */
  ownerId?: string
  /** 委托人姓名 */
  ownerName?: string
  /** 受托人身份 Id */
  trusteeId?: string
  /** 受托人姓名 */
  trusteeName?: string
  /** 委托条件（字典代码） */
  condition?: string
  /** 委托开始时间 */
  startTime?: string
  /** 委托结束时间 */
  endTime?: string
  /** 系统代号 */
  systemSymbol?: string
  /** 是否启用 */
  enabled?: boolean
  /** 备注 */
  remark?: string
  [key: string]: unknown
}

/** 委托新增/编辑载荷 */
export type WorkflowDelegatePayload = {
  /** 配置 ID，编辑时必填 */
  id?: string
  /** 委托人身份 Id（必填） */
  ownerId: string
  /** 受托人身份 Id（必填） */
  trusteeId: string
  /** 委托条件（必填，字典代码） */
  condition?: string
  /** 委托开始时间 */
  startTime?: string
  /** 委托结束时间 */
  endTime?: string
  /** 系统代号（必填） */
  systemSymbol?: string
  /** 是否启用 */
  enabled?: boolean
  /** 备注（必填） */
  remark?: string
  [key: string]: unknown
}

// listWorkflowDelegates 查询委托配置列表。
// @param query ownerId（委托人，可空）+ 分页
// @return 委托列表结果
export function listWorkflowDelegates(query?: WorkflowPageQuery): Promise<WorkflowResult<unknown>> {
  return callWorkflow(`${ROOT}/delegate/list`, { query })
}

// addWorkflowDelegate 新增委托配置（时间重叠/禁止传递由后端校验）。
// @param payload 委托字段
// @return 新增结果
export function addWorkflowDelegate(payload: WorkflowDelegatePayload): Promise<WorkflowResult<unknown>> {
  return callWorkflow(`${ROOT}/delegate/add`, { method: 'POST', json: payload })
}

// editWorkflowDelegate 编辑委托配置（亦用于启用/停用切换）。
// @param payload 含 id 的委托字段
// @return 编辑结果
export function editWorkflowDelegate(payload: WorkflowDelegatePayload): Promise<WorkflowResult<unknown>> {
  return callWorkflow(`${ROOT}/delegate/edit`, { method: 'POST', json: payload })
}

// deleteWorkflowDelegates 批量删除委托配置。
// @param ids 待删除的配置 ID
// @return 删除结果
export function deleteWorkflowDelegates(ids: string[]): Promise<WorkflowResult<unknown>> {
  return callWorkflow(`${ROOT}/delegate/delete/${joinIds(ids)}`, { method: 'DELETE' })
}

// ── 催办配置（P4.3）──

/** 催办配置实体 */
export type WorkflowRemind = {
  /** 配置 ID */
  id?: string
  /** 系统代号 */
  systemSymbol?: string
  /** 流程定义 Key */
  processDefinitionKey?: string
  /** 流程名称 */
  processName?: string
  /** 步骤代号集合（逗号分隔） */
  stepCodes?: string
  /** 步骤名称 */
  stepName?: string
  /** 是否启用预警提醒 */
  aheadEnabled?: boolean
  /** 预警提前量 */
  ahead?: number
  /** 再次提醒次数 */
  retry?: number
  /** 提醒间隔 */
  interval?: number
  /** 间隔单位（minute/hour/day） */
  intervalUnit?: string
  /** 是否启用延时提醒 */
  delayEnabled?: boolean
  /** 延时时长 */
  delay?: number
  /** 创建时间 */
  createTime?: string
  [key: string]: unknown
}

/** 催办新增/编辑载荷 */
export type WorkflowRemindPayload = {
  /** 配置 ID，编辑时必填 */
  id?: string
  /** 系统代号（必填） */
  systemSymbol: string
  /** 流程定义 Key */
  processDefinitionKey?: string
  /** 步骤代号（逗号分隔） */
  stepCodes?: string
  /** 是否启用预警提醒 */
  aheadEnabled?: boolean
  /** 预警提前量 */
  ahead?: number
  /** 再次提醒次数 */
  retry?: number
  /** 提醒间隔 */
  interval?: number
  /** 间隔单位 */
  intervalUnit?: string
  /** 是否启用延时提醒 */
  delayEnabled?: boolean
  /** 延时时长 */
  delay?: number
  [key: string]: unknown
}

// listWorkflowReminds 查询催办配置分页。
// @param query 过滤与分页参数（如 systemSymbol）
// @return 催办列表结果
export function listWorkflowReminds(query?: WorkflowPageQuery): Promise<WorkflowResult<unknown>> {
  return callWorkflow(`${ROOT}/remind/getPageList`, { query })
}

// getWorkflowRemind 读取催办配置详情。
// @param id 配置 ID
// @return 配置详情结果
export function getWorkflowRemind(id: string): Promise<WorkflowResult<unknown>> {
  return callWorkflow(`${ROOT}/remind/getWorkflowRemind/${encodeURIComponent(id)}`)
}

// addWorkflowRemind 新增催办配置。
// @param payload 配置字段
// @return 新增结果
export function addWorkflowRemind(payload: WorkflowRemindPayload): Promise<WorkflowResult<unknown>> {
  return callWorkflow(`${ROOT}/remind/add`, { method: 'POST', json: payload })
}

// editWorkflowRemind 编辑催办配置。
// @param payload 含 id 的配置字段
// @return 编辑结果
export function editWorkflowRemind(payload: WorkflowRemindPayload): Promise<WorkflowResult<unknown>> {
  return callWorkflow(`${ROOT}/remind/edit`, { method: 'POST', json: payload })
}

// deleteWorkflowRemind 删除催办配置。
// @param id 配置 ID
// @return 删除结果
export function deleteWorkflowRemind(id: string): Promise<WorkflowResult<unknown>> {
  return callWorkflow(`${ROOT}/remind/delete/${encodeURIComponent(id)}`, { method: 'DELETE' })
}

// ── 流程异常（P4.5）──

/** 流程异常实体 */
export type WorkflowException = {
  /** 异常 ID */
  id?: string
  /** 流程实例 ID */
  processInstanceId?: string
  /** 任务标题 */
  title?: string
  /** 流程名称 */
  processName?: string
  /** 异常类型（字典代码） */
  exceptionType?: string
  /** 异常信息 */
  errorMessage?: string
  /** 发起人 */
  initiatorName?: string
  /** 发生时间 */
  createTime?: string
  /** 处理状态 */
  handleStatus?: string
  [key: string]: unknown
}

// listWorkflowExceptions 查询异常列表。
// @param query processInstanceId（可空）+ 分页
// @return 异常列表结果
export function listWorkflowExceptions(query?: WorkflowPageQuery): Promise<WorkflowResult<unknown>> {
  return callWorkflow(`${ROOT}/exception/list`, { query })
}

// handleWorkflowExceptions 标记异常为已处理（exception/delete/{ids} 门面）。
// @param ids 待处理的异常 ID
// @return 处理结果
export function handleWorkflowExceptions(ids: string[]): Promise<WorkflowResult<unknown>> {
  return callWorkflow(`${ROOT}/exception/delete/${joinIds(ids)}`, { method: 'DELETE' })
}
