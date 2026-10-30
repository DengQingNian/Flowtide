// BPMN 校验辅助：XML 语法、流程 Key、开始事件与悬空连线检查。

/** 校验问题级别 */
export type ValidationLevel = 'error' | 'warning'

/** 单条校验问题 */
export interface ValidationIssue {
  /** 级别：错误阻断保存，警告仅提示 */
  level: ValidationLevel
  /** 中文说明 */
  message: string
  /** 对应 BPMN 元素 ID，可用于定位 */
  elementId?: string
}

/** 校验结果 */
export interface ValidationResult {
  /** 全部问题 */
  issues: ValidationIssue[]
  /** 错误 */
  errors: ValidationIssue[]
  /** 警告 */
  warnings: ValidationIssue[]
  /** 是否通过（无错误） */
  valid: boolean
}

/** bpmn-moddle 警告的宽松结构，避免依赖其内部类型 */
export interface ModelerWarningLike {
  /** 警告文本 */
  message?: string
  /** 关联元素 */
  element?: { id?: string } | null
}

/** 导入文件大小上限：5MB */
export const MAX_IMPORT_BYTES = 5 * 1024 * 1024

// 判断导入文件是否超过 5MB 上限。
// @param size 文件字节数
// @return 是否超限
export function isFileTooLarge(size: number): boolean {
  return size > MAX_IMPORT_BYTES
}

// 读取元素的本地名，忽略 bpmn: 等命名空间前缀。
// @param element XML 元素
// @return 本地名
function localNameOf(element: Element): string {
  if (element.localName) return element.localName
  const name = element.nodeName
  const index = name.indexOf(':')
  return index >= 0 ? name.slice(index + 1) : name
}

// 判断元素是否为指定本地名。
// @param element XML 元素
// @param name 本地名
// @return 是否匹配
function isElement(element: Element, name: string): boolean {
  return localNameOf(element) === name
}

// 读取元素的直接或深层匹配后代。
// @param root 根元素
// @param name 本地名
// @return 匹配元素数组
function descendants(root: Element, name: string): Element[] {
  const result: Element[] = []
  const all = root.getElementsByTagName('*')
  for (let index = 0; index < all.length; index += 1) {
    if (isElement(all[index], name)) result.push(all[index])
  }
  return result
}

// 收集文档中所有带 ID 的元素。
// @param elements 全部元素
// @return ID 集合
function collectIds(elements: Element[]): Set<string> {
  const ids = new Set<string>()
  for (const element of elements) {
    const id = element.getAttribute('id')
    if (id) ids.add(id)
  }
  return ids
}

// 构造校验结果。
// @param issues 问题列表
// @return 分类后的结果
function buildResult(issues: ValidationIssue[]): ValidationResult {
  const errors = issues.filter((item) => item.level === 'error')
  const warnings = issues.filter((item) => item.level === 'warning')
  return { issues, errors, warnings, valid: errors.length === 0 }
}

// 解析 XML 并返回文档；语法错误时返回 null。
// @param xml XML 文本
// @return 文档或 null
function parseDocument(xml: string): Document | null {
  if (typeof DOMParser === 'undefined') return null
  const document = new DOMParser().parseFromString(xml, 'text/xml')
  const errors = document.getElementsByTagName('parsererror')
  if (errors.length > 0) return null
  return document
}

// 提取 XML 解析错误文本。
// @param xml XML 文本
// @return 精简后的错误说明
function parseErrorText(xml: string): string {
  if (typeof DOMParser === 'undefined') return '当前环境不支持 XML 解析'
  const document = new DOMParser().parseFromString(xml, 'text/xml')
  const errors = document.getElementsByTagName('parsererror')
  if (errors.length === 0) return 'XML 解析失败'
  const text = (errors[0].textContent ?? '').replace(/\s+/g, ' ').trim()
  return text || 'XML 解析失败'
}

// 检查流程 Key、开始事件与可执行标记。
// @param process 流程元素
// @param expectedKey 模型 Key
// @param issues 问题收集数组
// @return 无
function validateProcess(process: Element, expectedKey: string, issues: ValidationIssue[]): void {
  const processId = process.getAttribute('id') ?? ''
  if (!processId) {
    issues.push({ level: 'error', message: '流程缺少 ID，需与模型 Key 保持一致' })
  } else if (expectedKey && processId !== expectedKey) {
    // Key 不一致会导致部署后的定义 Key 与模型 Key 分叉
    issues.push({
      level: 'error',
      message: `流程 ID「${processId}」与模型 Key「${expectedKey}」不一致`,
      elementId: processId,
    })
  }
  const startEvents = descendants(process, 'startEvent')
  if (startEvents.length === 0) {
    issues.push({
      level: 'error',
      message: '流程至少需要一个开始事件',
      elementId: processId || undefined,
    })
  }
  const executable = process.getAttribute('isExecutable')
  if (executable === 'false') {
    issues.push({
      level: 'warning',
      message: '流程标记为不可执行，部署后无法启动实例',
      elementId: processId || undefined,
    })
  }
}

// 检查顺序流的源与目标是否存在。
// @param flows 顺序流元素
// @param ids 文档内所有元素 ID
// @param issues 问题收集数组
// @return 无
function validateFlows(flows: Element[], ids: Set<string>, issues: ValidationIssue[]): void {
  for (const flow of flows) {
    const flowId = flow.getAttribute('id') ?? ''
    const sourceRef = flow.getAttribute('sourceRef') ?? ''
    const targetRef = flow.getAttribute('targetRef') ?? ''
    if (!sourceRef || !targetRef) {
      issues.push({
        level: 'error',
        message: `连线「${flowId || '未命名'}」缺少源或目标`,
        elementId: flowId || undefined,
      })
      continue
    }
    // 悬空引用通常来自手工编辑或合并 XML
    if (!ids.has(sourceRef) || !ids.has(targetRef)) {
      const missing = !ids.has(sourceRef) ? sourceRef : targetRef
      issues.push({
        level: 'error',
        message: `连线「${flowId || '未命名'}」指向不存在的元素「${missing}」`,
        elementId: flowId || undefined,
      })
    }
  }
}

// 校验 BPMN XML：语法、流程 ID、开始事件与悬空连线。
// @param xml BPMN XML
// @param expectedKey 模型 Key，为空时跳过 Key 一致性检查
// @return 校验结果
export function validateBpmnXml(xml: string, expectedKey = ''): ValidationResult {
  const issues: ValidationIssue[] = []
  if (!xml.trim()) {
    return buildResult([{ level: 'error', message: '模型源码为空，无法保存或部署' }])
  }
  const document = parseDocument(xml)
  if (!document) {
    return buildResult([{ level: 'error', message: `XML 解析失败：${parseErrorText(xml)}` }])
  }
  const elements: Element[] = []
  const all = document.getElementsByTagName('*')
  for (let index = 0; index < all.length; index += 1) elements.push(all[index])
  const processes = elements.filter((element) => isElement(element, 'process'))
  if (processes.length === 0) {
    return buildResult([{ level: 'error', message: '未找到 bpmn:process 元素，文件可能不是 BPMN 2.0' }])
  }
  for (const process of processes) validateProcess(process, expectedKey, issues)
  const flows = elements.filter((element) => isElement(element, 'sequenceFlow'))
  validateFlows(flows, collectIds(elements), issues)
  const diagrams = elements.filter((element) => isElement(element, 'BPMNDiagram') || isElement(element, 'BPMNPlane'))
  if (diagrams.length === 0) {
    issues.push({ level: 'warning', message: '缺少 BPMN DI 图形信息，设计器与图片渲染可能异常' })
  }
  return buildResult(issues)
}

// 把 bpmn-moddle 的警告转换成统一问题结构。
// @param warnings bpmn-moddle 警告
// @return 警告问题列表
export function issuesFromModelerWarnings(warnings: ModelerWarningLike[]): ValidationIssue[] {
  return warnings
    .filter((item) => Boolean(item.message) || Boolean(item.element?.id))
    .map((item) => ({
      level: 'warning' as const,
      message: item.message ?? 'BPMN 解析警告',
      elementId: item.element?.id,
    }))
}
