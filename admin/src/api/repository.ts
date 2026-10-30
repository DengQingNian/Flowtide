// 仓库域接口：部署、部署资源与流程定义的 REST 封装。
// 只发送服务端真实支持的参数；服务端缺失的过滤能力由页面在本地完成。
import { deleteJSON, getBlob, getJSON, getText, postForm, postJSON, putJSON } from './client'
import type { Page } from './client'
import type { FormData as FormDataType, IdentityLink } from './types'

/** 部署信息 */
export interface Deployment {
  /** 部署 ID */
  id: string
  /** 部署名称 */
  name: string
  /** 租户 */
  tenantId?: string
  /** 部署时间 */
  deployTime?: string
}

/** 部署列表查询参数，仅包含服务端支持的字段 */
export interface DeploymentQuery {
  /** 偏移量 */
  start?: number
  /** 每页条数 */
  size?: number
  /** 租户 */
  tenantId?: string
}

/** 部署资源元数据 */
export interface DeploymentResource {
  /** 资源名 */
  id: string
  /** 元数据地址 */
  url?: string
  /** 原始内容地址 */
  contentUrl?: string
  /** 媒体类型 */
  mediaType?: string
  /** 资源类型：processDefinition/processImage/resource */
  type?: string
}

/** 创建部署选项 */
export interface CreateDeploymentOptions {
  /** 部署名称 */
  name?: string
  /** 租户 */
  tenantId?: string
  /** 是否复用同租户同内容部署 */
  enableDuplicateFiltering?: boolean
}

/** 流程定义信息 */
export interface ProcessDefinition {
  /** 定义 ID */
  id: string
  /** 定义 Key */
  key: string
  /** 名称 */
  name?: string
  /** 说明 */
  description?: string
  /** 版本号 */
  version: number
  /** 所属部署 ID */
  deploymentId?: string
  /** 资源名 */
  resourceName?: string
  /** 租户 */
  tenantId?: string
  /** 是否挂起 */
  suspended?: boolean
}

/** 流程定义列表查询参数，仅包含服务端支持的字段 */
export interface ProcessDefinitionQuery {
  /** 定义 Key，精确匹配 */
  key?: string
  /** 版本号 */
  version?: number
  /** 候选发起人过滤 */
  startableByUser?: string
  /** 租户 */
  tenantId?: string
  /** 偏移量 */
  start?: number
  /** 每页条数 */
  size?: number
}

/** 流程定义动作载荷 */
export interface DefinitionActionPayload {
  /** suspend 或 activate */
  action: 'suspend' | 'activate'
  /** 是否同时处理运行中的实例 */
  includeProcessInstances?: boolean
}

/** 启动流程实例载荷 */
export interface StartProcessInstancePayload {
  /** 流程定义 ID */
  processDefinitionId?: string
  /** 流程定义 Key */
  processDefinitionKey?: string
  /** 业务 Key */
  businessKey?: string
  /** 实例名称 */
  name?: string
  /** 变量对象或变量数组 */
  variables?: Record<string, unknown> | Array<{ name: string; type?: string; value?: unknown }>
  /** 租户 */
  tenantId?: string
}

/** 启动后返回的流程实例摘要 */
export interface StartedProcessInstance {
  /** 实例 ID */
  id: string
  /** 流程定义 ID */
  processDefinitionId?: string
  /** 业务 Key */
  businessKey?: string
  /** 是否已结束 */
  ended?: boolean
}

/** 资源预览类型 */
export type ResourcePreviewKind = 'xml' | 'json' | 'text' | 'image' | 'binary'

/** 列表一次加载的最大条数，与通用分页上限一致 */
export const REPOSITORY_MAX_SIZE = 1000

// 生成部署列表路径。
// @return 部署列表路径
function deploymentsPath(): string {
  return '/service/repository/deployments'
}

// 生成部署详情路径。
// @param deploymentId 部署 ID
// @return 部署详情路径
function deploymentPath(deploymentId: string): string {
  return `${deploymentsPath()}/${deploymentId}`
}

// 生成部署资源元数据路径，资源名按 URL 编码。
// @param deploymentId 部署 ID
// @param resourceName 资源名
// @return 资源元数据路径
export function deploymentResourcePath(deploymentId: string, resourceName: string): string {
  return `${deploymentPath(deploymentId)}/resources/${encodeURIComponent(resourceName)}`
}

// 生成部署资源原始内容路径，资源名按 URL 编码。
// @param deploymentId 部署 ID
// @param resourceName 资源名
// @return 资源内容路径
export function deploymentResourceDataPath(deploymentId: string, resourceName: string): string {
  return `${deploymentPath(deploymentId)}/resourcedata/${encodeURIComponent(resourceName)}`
}

// 生成流程定义路径。
// @param definitionId 流程定义 ID
// @return 流程定义路径
function definitionPath(definitionId: string): string {
  return `/service/repository/process-definitions/${definitionId}`
}

// 生成流程定义原始资源路径。
// @param definitionId 流程定义 ID
// @return 原始资源路径
export function definitionResourceDataPath(definitionId: string): string {
  return `${definitionPath(definitionId)}/resourcedata`
}

// 生成流程定义身份链接路径。
// @param definitionId 流程定义 ID
// @param family users 或 groups
// @param identityId 用户或组 ID
// @return 身份链接路径
export function definitionIdentityLinkPath(definitionId: string, family: string, identityId: string): string {
  return `${definitionPath(definitionId)}/identitylinks/${family}/${encodeURIComponent(identityId)}`
}

// 读取部署分页列表。
// @param query 查询参数
// @param signal 取消信号
// @return 部署分页结果
export function listDeployments(query: DeploymentQuery = {}, signal?: AbortSignal): Promise<Page<Deployment>> {
  return getJSON<Page<Deployment>>(deploymentsPath(), {
    start: query.start ?? 0,
    size: query.size ?? 50,
    tenantId: query.tenantId,
  }, signal)
}

// 读取单个部署。
// @param deploymentId 部署 ID
// @param signal 取消信号
// @return 部署信息
export function getDeployment(deploymentId: string, signal?: AbortSignal): Promise<Deployment> {
  return getJSON<Deployment>(deploymentPath(deploymentId), undefined, signal)
}

// 创建部署，文件以任意字段名上传，资源名取文件名。
// @param files 上传文件
// @param options 部署选项
// @return 新建或复用的部署
export function createDeployment(files: File[], options: CreateDeploymentOptions = {}): Promise<Deployment> {
  const form = new FormData()
  for (const file of files) form.append('file', file, file.name)
  if (options.name) form.append('deploymentName', options.name)
  if (options.tenantId) form.append('tenantId', options.tenantId)
  const query = options.enableDuplicateFiltering ? '?enableDuplicateFiltering=true' : ''
  return postForm<Deployment>(`${deploymentsPath()}${query}`, form)
}

// 删除部署。
// @param deploymentId 部署 ID
// @param cascade 是否级联删除部署下的流程定义
// @return 无
export function deleteDeployment(deploymentId: string, cascade = true): Promise<void> {
  return deleteJSON<void>(deploymentPath(deploymentId), { cascade: cascade ? true : undefined })
}

// 读取部署资源元数据列表。
// @param deploymentId 部署 ID
// @param signal 取消信号
// @return 资源元数据数组
export function listDeploymentResources(deploymentId: string, signal?: AbortSignal): Promise<DeploymentResource[]> {
  return getJSON<DeploymentResource[]>(`${deploymentPath(deploymentId)}/resources`, undefined, signal)
}

// 读取单个部署资源元数据。
// @param deploymentId 部署 ID
// @param resourceName 资源名
// @return 资源元数据
export function getDeploymentResourceMetadata(deploymentId: string, resourceName: string): Promise<DeploymentResource> {
  return getJSON<DeploymentResource>(deploymentResourcePath(deploymentId, resourceName))
}

// 读取部署资源的文本内容。
// @param deploymentId 部署 ID
// @param resourceName 资源名
// @return 文本内容
export function getDeploymentResourceText(deploymentId: string, resourceName: string): Promise<string> {
  return getText(deploymentResourceDataPath(deploymentId, resourceName))
}

// 把客户端按文本解析的响应统一包装为 Blob，避免 XML/JSON 资源缺少 size 与 text 能力。
// @param value 原始响应
// @param mediaType 媒体类型
// @return 二进制内容
function toBlob(value: unknown, mediaType = 'application/octet-stream'): Blob {
  if (value instanceof Blob) return value
  return new Blob([typeof value === 'string' ? value : String(value)], { type: mediaType })
}

// 读取部署资源的二进制内容。
// @param deploymentId 部署 ID
// @param resourceName 资源名
// @param mediaType 媒体类型，用于文本响应包装
// @return 二进制内容
export async function getDeploymentResourceBlob(deploymentId: string, resourceName: string, mediaType?: string): Promise<Blob> {
  const value: unknown = await getBlob(deploymentResourceDataPath(deploymentId, resourceName))
  return toBlob(value, mediaType)
}

// 读取流程定义分页列表。
// @param query 查询参数
// @param signal 取消信号
// @return 流程定义分页结果
export function listProcessDefinitions(query: ProcessDefinitionQuery = {}, signal?: AbortSignal): Promise<Page<ProcessDefinition>> {
  return getJSON<Page<ProcessDefinition>>('/service/repository/process-definitions', {
    key: query.key,
    version: query.version,
    startableByUser: query.startableByUser,
    tenantId: query.tenantId,
    start: query.start ?? 0,
    size: query.size ?? 50,
  }, signal)
}

// 读取最多 REPOSITORY_MAX_SIZE 条流程定义，用于按 Key 分组与本地过滤。
// @param tenantId 租户
// @param signal 取消信号
// @return 流程定义数组
export async function listAllProcessDefinitions(tenantId?: string, signal?: AbortSignal): Promise<ProcessDefinition[]> {
  const page = await listProcessDefinitions({ start: 0, size: REPOSITORY_MAX_SIZE, tenantId }, signal)
  return page.data ?? []
}

// 读取单个流程定义。
// @param definitionId 流程定义 ID
// @param signal 取消信号
// @return 流程定义
export function getProcessDefinition(definitionId: string, signal?: AbortSignal): Promise<ProcessDefinition> {
  return getJSON<ProcessDefinition>(definitionPath(definitionId), undefined, signal)
}

// 读取流程定义 BPMN XML。
// @param definitionId 流程定义 ID
// @param signal 取消信号
// @return XML 文本
export function getProcessDefinitionXML(definitionId: string, signal?: AbortSignal): Promise<string> {
  return getText(`${definitionPath(definitionId)}/xml`, undefined, signal)
}

// 读取流程定义图片（默认 PNG）。
// @param definitionId 流程定义 ID
// @param signal 取消信号
// @return 图片二进制
export function getProcessDefinitionImage(definitionId: string, signal?: AbortSignal): Promise<Blob> {
  return getBlob(`${definitionPath(definitionId)}/image`, undefined, signal)
}

// 读取流程定义启动表单，空响应归一为无字段表单。
// @param definitionId 流程定义 ID
// @param signal 取消信号
// @return 启动表单
export async function getProcessDefinitionStartForm(definitionId: string, signal?: AbortSignal): Promise<FormDataType> {
  const form = await getJSON<FormDataType | null>(`${definitionPath(definitionId)}/start-form`, undefined, signal)
  return { formKey: form?.formKey, formProperties: form?.formProperties ?? [] }
}

// 读取流程定义的身份链接（候选发起人）。
// @param definitionId 流程定义 ID
// @param signal 取消信号
// @return 身份链接数组
export function listDefinitionIdentityLinks(definitionId: string, signal?: AbortSignal): Promise<IdentityLink[]> {
  return getJSON<IdentityLink[]>(`${definitionPath(definitionId)}/identitylinks`, undefined, signal)
}

// 添加流程定义身份链接。
// @param definitionId 流程定义 ID
// @param userId 用户 ID，与 groupId 二选一
// @param groupId 组 ID，与 userId 二选一
// @return 新建的身份链接
export function addDefinitionIdentityLink(definitionId: string, userId?: string, groupId?: string): Promise<IdentityLink> {
  return postJSON<IdentityLink>(`${definitionPath(definitionId)}/identitylinks`, { userId, groupId })
}

// 删除流程定义身份链接。
// @param definitionId 流程定义 ID
// @param family users 或 groups
// @param identityId 用户或组 ID
// @return 无
export function deleteDefinitionIdentityLink(definitionId: string, family: 'users' | 'groups', identityId: string): Promise<void> {
  return deleteJSON<void>(definitionIdentityLinkPath(definitionId, family, identityId))
}

// 挂起或激活流程定义。
// @param definitionId 流程定义 ID
// @param payload 动作载荷
// @return 无
export function updateProcessDefinition(definitionId: string, payload: DefinitionActionPayload): Promise<void> {
  return putJSON<void>(definitionPath(definitionId), payload)
}

// 启动流程实例。
// @param payload 启动载荷
// @return 启动后的实例摘要
export function startProcessInstance(payload: StartProcessInstancePayload): Promise<StartedProcessInstance> {
  return postJSON<StartedProcessInstance>('/service/runtime/process-instances', payload)
}

// 读取流程定义原始资源。
// @param definitionId 流程定义 ID
// @return 二进制内容
export async function getDefinitionResourceBlob(definitionId: string): Promise<Blob> {
  const value: unknown = await getBlob(definitionResourceDataPath(definitionId))
  return toBlob(value, 'application/xml')
}

// 按资源名与媒体类型判断预览方式。
// @param name 资源名
// @param mediaType 媒体类型
// @return 预览类型
export function classifyResource(name: string, mediaType?: string): ResourcePreviewKind {
  const lowerName = name.toLowerCase()
  const lowerType = (mediaType ?? '').toLowerCase()
  // 图片优先判断，避免 SVG 被 XML 分支截获
  if (lowerType.startsWith('image/')) return 'image'
  if (/\.(png|jpe?g|gif|svg|webp|bmp|ico)$/.test(lowerName)) return 'image'
  if (lowerType.includes('xml') || /\.(xml|bpmn|bpmn20\.xml|cmmn|dmn)$/.test(lowerName)) return 'xml'
  if (lowerType.includes('json') || /\.(json|form)$/.test(lowerName)) return 'json'
  if (lowerType.startsWith('text/') || /\.(txt|md|csv|sql|log)$/.test(lowerName)) return 'text'
  return 'binary'
}
