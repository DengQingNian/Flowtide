import { describe, expect, it } from 'vitest'
import {
  MAX_IMPORT_BYTES,
  isFileTooLarge,
  issuesFromModelerWarnings,
  validateBpmnXml,
} from '@/design/validation'
import { buildMinimalBpmn } from '@/design/minimalBpmn'
import { translate } from '@/design/translate'

/** 构造带指定流程内容的 BPMN 文档 */
function bpmn(processBody: string, processAttrs = 'id="leave" isExecutable="true"'): string {
  return `<?xml version="1.0" encoding="UTF-8"?>
<bpmn:definitions xmlns:bpmn="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <bpmn:process ${processAttrs}>
    ${processBody}
  </bpmn:process>
</bpmn:definitions>`
}

describe('validateBpmnXml', () => {
  it('接受包含开始事件且 Key 一致的最小模型', () => {
    const result = validateBpmnXml(buildMinimalBpmn('leave', '请假'), 'leave')
    expect(result.valid).toBe(true)
    expect(result.errors).toEqual([])
    expect(result.warnings).toEqual([])
  })

  it('空源码返回错误', () => {
    const result = validateBpmnXml('   ', 'leave')
    expect(result.valid).toBe(false)
    expect(result.errors[0].message).toContain('源码为空')
  })

  it('XML 语法错误返回解析失败信息', () => {
    const result = validateBpmnXml('<bpmn:definitions><broken', 'leave')
    expect(result.valid).toBe(false)
    expect(result.errors[0].message).toContain('XML 解析失败')
  })

  it('缺少 process 元素时拒绝', () => {
    const result = validateBpmnXml('<root><child/></root>', 'leave')
    expect(result.valid).toBe(false)
    expect(result.errors[0].message).toContain('未找到 bpmn:process')
  })

  it('至少需要一个开始事件', () => {
    const result = validateBpmnXml(bpmn('<bpmn:endEvent id="end1" />'), 'leave')
    expect(result.valid).toBe(false)
    expect(result.errors.some((item) => item.message.includes('至少需要一个开始事件'))).toBe(true)
    expect(result.errors[0].elementId).toBe('leave')
  })

  it('流程 ID 与模型 Key 不一致时报错并携带元素 ID', () => {
    const result = validateBpmnXml(bpmn('<bpmn:startEvent id="s1" />', 'id="other" isExecutable="true"'), 'leave')
    expect(result.valid).toBe(false)
    const issue = result.errors.find((item) => item.message.includes('不一致'))
    expect(issue?.elementId).toBe('other')
    expect(issue?.message).toContain('leave')
  })

  it('流程缺少 ID 时拒绝', () => {
    const result = validateBpmnXml(bpmn('<bpmn:startEvent id="s1" />', 'isExecutable="true"'), '')
    expect(result.valid).toBe(false)
    expect(result.errors[0].message).toContain('流程缺少 ID')
  })

  it('悬空连线指向不存在的元素时报告元素 ID', () => {
    const result = validateBpmnXml(
      bpmn(`
        <bpmn:startEvent id="s1" />
        <bpmn:sequenceFlow id="flow1" sourceRef="s1" targetRef="missing" />
      `),
      'leave',
    )
    expect(result.valid).toBe(false)
    const issue = result.errors.find((item) => item.elementId === 'flow1')
    expect(issue?.message).toContain('missing')
  })

  it('连线缺少源或目标时报告错误', () => {
    const result = validateBpmnXml(
      bpmn(`
        <bpmn:startEvent id="s1" />
        <bpmn:sequenceFlow id="flow1" sourceRef="s1" />
      `),
      'leave',
    )
    expect(result.valid).toBe(false)
    expect(result.errors.some((item) => item.message.includes('缺少源或目标'))).toBe(true)
  })

  it('不可执行流程与缺失 DI 给出警告但不阻断', () => {
    const result = validateBpmnXml(bpmn('<bpmn:startEvent id="s1" />', 'id="leave" isExecutable="false"'), 'leave')
    expect(result.valid).toBe(true)
    expect(result.warnings).toHaveLength(2)
    expect(result.warnings.some((item) => item.message.includes('不可执行'))).toBe(true)
    expect(result.warnings.some((item) => item.message.includes('DI'))).toBe(true)
  })

  it('多个流程逐一检查 Key', () => {
    const xml = `<bpmn:definitions xmlns:bpmn="http://www.omg.org/spec/BPMN/20100524/MODEL">
      <bpmn:process id="leave"><bpmn:startEvent id="s1" /></bpmn:process>
      <bpmn:process id="other"><bpmn:startEvent id="s2" /></bpmn:process>
    </bpmn:definitions>`
    const result = validateBpmnXml(xml, 'leave')
    expect(result.valid).toBe(false)
    expect(result.errors).toHaveLength(1)
  })
})

describe('issuesFromModelerWarnings', () => {
  it('转换文本与元素 ID 并过滤空警告', () => {
    const issues = issuesFromModelerWarnings([
      { message: '未知元素', element: { id: 'Task_1' } },
      { element: { id: 'Task_2' } },
      {},
    ])
    expect(issues).toEqual([
      { level: 'warning', message: '未知元素', elementId: 'Task_1' },
      { level: 'warning', message: 'BPMN 解析警告', elementId: 'Task_2' },
    ])
  })
})

describe('isFileTooLarge', () => {
  it('允许 5MB，拒绝超过 5MB', () => {
    expect(MAX_IMPORT_BYTES).toBe(5 * 1024 * 1024)
    expect(isFileTooLarge(MAX_IMPORT_BYTES)).toBe(false)
    expect(isFileTooLarge(MAX_IMPORT_BYTES + 1)).toBe(true)
  })
})

describe('translate', () => {
  it('翻译常用词并替换占位符', () => {
    expect(translate('User Task')).toBe('用户任务')
    expect(translate('Create StartEvent')).toBe('开始事件')
    expect(translate('未收录的模板')).toBe('未收录的模板')
    expect(translate('已处理 {count} 条', { count: 2 })).toBe('已处理 2 条')
  })
})
