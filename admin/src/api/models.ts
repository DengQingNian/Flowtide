// 流程模型域 REST 封装：模型元信息、编辑源码、扩展源码与模型部署。
import {
  deleteJSON,
  getBlob,
  getJSON,
  postJSON,
  putForm,
  putJSON,
  type Page,
  type PageQuery,
} from './client'

/** 流程模型元信息，与 GET /repository/models 响应字段一致 */
export interface Model extends Record<string, unknown> {
  /** 模型 ID */
  id: string
  /** 模型名称 */
  name: string
  /** 模型 Key，与 BPMN 流程 ID 保持一致 */
  key: string
  /** 分类 */
  category?: string
  /** 模型版本，从 1 开始 */
  version: number
  /** 元信息，JSON 文本或备注 */
  metaInfo?: string
  /** 最近一次部署 ID，空表示未部署 */
  deploymentId?: string
  /** 租户 */
  tenantId?: string
  /** 创建时间 */
  createTime?: string
  /** 最后更新时间 */
  lastUpdateTime?: string
}

/** 模型列表查询参数 */
export interface ModelQuery extends PageQuery {
  /** 精确匹配 ID */
  id?: string
  /** 精确匹配分类 */
  category?: string
  /** 分类模糊匹配，支持 % 与 _ */
  categoryLike?: string
  /** 精确匹配名称 */
  name?: string
  /** 名称模糊匹配，支持 % 与 _ */
  nameLike?: string
  /** 精确匹配 Key */
  key?: string
  /** 精确匹配部署 ID */
  deploymentId?: string
  /** 精确匹配租户 */
  tenantId?: string
  /** 租户模糊匹配 */
  tenantIdLike?: string
  /** 是否已部署 */
  deployed?: boolean
  /** 只返回无租户模型 */
  withoutTenantId?: boolean
  /** 每个 Key 只返回最新版本 */
  latestVersion?: boolean
  /** 精确匹配模型版本 */
  version?: number
}

/** 新建模型请求体 */
export interface CreateModelPayload {
  /** 名称 */
  name: string
  /** Key，重复时服务端返回 409 */
  key: string
  /** 分类 */
  category?: string
  /** 元信息 */
  metaInfo?: string
  /** 租户，留空使用服务端默认租户 */
  tenantId?: string
  /** 版本，0 或留空由服务端决定 */
  version?: number
}

/** 更新模型请求体，null 表示清空字段 */
export interface UpdateModelPayload {
  /** 名称 */
  name?: string
  /** Key */
  key?: string
  /** 分类 */
  category?: string | null
  /** 元信息 */
  metaInfo?: string | null
  /** 部署 ID */
  deploymentId?: string | null
  /** 租户 */
  tenantId?: string | null
  /** 版本 */
  version?: number
}

/** 部署结果，与 POST /repository/models/{id}/deployment 响应一致 */
export interface ModelDeployment {
  /** 部署 ID */
  id: string
  /** 部署名称 */
  name?: string
  /** 租户 */
  tenantId?: string
  /** 部署时间 */
  deployTime?: string
}

/** 流程定义摘要，用于展示模型部署生成的版本 */
export interface ProcessDefinitionSummary extends Record<string, unknown> {
  /** 定义 ID */
  id: string
  /** 定义 Key */
  key: string
  /** 定义名称 */
  name?: string
  /** 定义版本 */
  version: number
  /** 部署 ID */
  deploymentId?: string
  /** 资源名 */
  resourceName?: string
}

/** 模型基础路径 */
const MODELS_PATH = '/service/repository/models'

// 对路径中的模型 ID 做编码，避免特殊字符破坏 URL。
// @param id 模型 ID
// @return 编码后的 ID
function encodeId(id: string): string {
  return encodeURIComponent(id)
}

// 查询流程模型列表。
// @param query 查询与分页参数
// @param signal 取消信号
// @return 分页模型列表
export function listModels(query: ModelQuery = {}, signal?: AbortSignal): Promise<Page<Model>> {
  return getJSON<Page<Model>>(MODELS_PATH, query, signal)
}

// 获取单个流程模型。
// @param id 模型 ID
// @param signal 取消信号
// @return 模型元信息
export function getModel(id: string, signal?: AbortSignal): Promise<Model> {
  return getJSON<Model>(`${MODELS_PATH}/${encodeId(id)}`, undefined, signal)
}

// 新建流程模型。
// @param payload 创建请求体
// @return 创建后的模型
export function createModel(payload: CreateModelPayload): Promise<Model> {
  return postJSON<Model>(MODELS_PATH, payload)
}

// 更新流程模型元信息。
// @param id 模型 ID
// @param payload 局部更新请求体
// @return 更新后的模型
export function updateModel(id: string, payload: UpdateModelPayload): Promise<Model> {
  return putJSON<Model>(`${MODELS_PATH}/${encodeId(id)}`, payload)
}

// 删除流程模型。
// @param id 模型 ID
// @return 无
export function deleteModel(id: string): Promise<void> {
  return deleteJSON<void>(`${MODELS_PATH}/${encodeId(id)}`)
}

// 用 FileReader 把二进制源码读成文本，兼容缺少 Blob.text 的环境。
// @param blob 源码二进制
// @return 文本内容
function readBlobText(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result ?? ''))
    reader.onerror = () => reject(reader.error ?? new Error('读取模型源码失败'))
    reader.readAsText(blob)
  })
}

// 读取模型编辑源码；服务端返回 application/octet-stream，需要先转成文本。
// @param id 模型 ID
// @param signal 取消信号
// @return BPMN XML 文本
export async function getModelSource(id: string, signal?: AbortSignal): Promise<string> {
  const blob = await getBlob(`${MODELS_PATH}/${encodeId(id)}/source`, undefined, signal)
  return readBlobText(blob)
}

// 读取模型扩展源码。
// @param id 模型 ID
// @param signal 取消信号
// @return 扩展源码文本
export async function getModelSourceExtra(id: string, signal?: AbortSignal): Promise<string> {
  const blob = await getBlob(`${MODELS_PATH}/${encodeId(id)}/source-extra`, undefined, signal)
  return readBlobText(blob)
}

// 构造上传模型的 FormData，字段名固定 file，文件名用于服务端识别资源类型。
// @param xml 源码文本
// @param filename 文件名
// @return multipart 表单
function buildSourceForm(xml: string, filename: string): FormData {
  const form = new FormData()
  form.append('file', new Blob([xml], { type: 'application/xml' }), filename)
  return form
}

// 保存模型编辑源码。
// @param id 模型 ID
// @param xml BPMN XML
// @param filename 上传文件名，建议 {key}.bpmn20.xml
// @return 无
export function saveModelSource(id: string, xml: string, filename: string): Promise<void> {
  return putForm<void>(`${MODELS_PATH}/${encodeId(id)}/source`, buildSourceForm(xml, filename))
}

// 保存模型扩展源码。
// @param id 模型 ID
// @param source 扩展源码
// @param filename 上传文件名
// @return 无
export function saveModelSourceExtra(id: string, source: string, filename = 'source-extra.bpmn'): Promise<void> {
  return putForm<void>(`${MODELS_PATH}/${encodeId(id)}/source-extra`, buildSourceForm(source, filename))
}

// 部署模型源码，服务端以 {key}.bpmn20.xml 为资源名生成部署。
// @param id 模型 ID
// @return 部署结果
export function deployModel(id: string): Promise<ModelDeployment> {
  return postJSON<ModelDeployment>(`${MODELS_PATH}/${encodeId(id)}/deployment`)
}

// 查询指定 Key 的流程定义，用于展示模型部署后的定义版本。
// @param key 流程 Key
// @param signal 取消信号
// @return 分页流程定义
export function listProcessDefinitionsByKey(key: string, signal?: AbortSignal): Promise<Page<ProcessDefinitionSummary>> {
  return getJSON<Page<ProcessDefinitionSummary>>('/service/repository/process-definitions', {
    key,
    start: 0,
    size: 1000,
  }, signal)
}

// 选择版本号最大的流程定义。
// @param definitions 定义列表
// @return 最新定义，列表为空时返回 null
export function pickLatestDefinition(definitions: ProcessDefinitionSummary[]): ProcessDefinitionSummary | null {
  let latest: ProcessDefinitionSummary | null = null
  for (const item of definitions) {
    if (!latest || item.version > latest.version) latest = item
  }
  return latest
}

// 计算模型源码上传文件名。
// @param model 模型元信息
// @return {key}.bpmn20.xml，Key 为空时回退为 model-{id}.bpmn20.xml
export function sourceFileName(model: Pick<Model, 'id' | 'key'>): string {
  const key = (model.key ?? '').trim()
  if (key) return `${key}.bpmn20.xml`
  return `model-${model.id}.bpmn20.xml`
}
