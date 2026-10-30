// 跨域复用的 REST 数据类型。域内专属类型放在各自 api/*.ts 中。

/** 变量写入载荷 */
export interface Variable {
  /** 变量名 */
  name: string
  /** 类型：string/long/double/boolean/date/json 等 */
  type?: string
  /** 变量值 */
  value?: unknown
  /** 作用域：global 或 local */
  scope?: string
}

/** 变量实例（查询响应） */
export interface VariableInstance extends Variable {
  /** 变量实例 ID */
  id?: string
  /** 流程实例 ID */
  processInstanceId?: string
  /** 执行 ID */
  executionId?: string
  /** 任务 ID */
  taskId?: string
  /** 是否瞬态变量 */
  transient?: boolean
}

/** 表单枚举值 */
export interface FormValue {
  id: string
  name?: string
}

/** 表单字段 */
export interface FormProperty {
  id: string
  name?: string
  type?: string
  required?: boolean
  readable?: boolean
  writable?: boolean
  value?: unknown
  enumValues?: FormValue[]
}

/** 表单数据 */
export interface FormData {
  formKey?: string
  formProperties: FormProperty[]
}

/** 身份链接 */
export interface IdentityLink {
  id?: string
  type?: string
  userId?: string
  groupId?: string
  taskId?: string
  processInstanceId?: string
  processDefinitionId?: string
  createTime?: string
}

/** 评论 */
export interface Comment {
  id: string
  userId?: string
  time?: string
  message: string
  taskId?: string
  processInstanceId?: string
}

/** 附件 */
export interface Attachment {
  id: string
  name?: string
  description?: string
  type?: string
  taskId?: string
  processInstanceId?: string
  url?: string
  time?: string
}

/** 健康检查结果 */
export interface HealthResult {
  status: string
  [key: string]: unknown
}
