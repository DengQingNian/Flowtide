import { describe, expect, it } from 'vitest'
import { buildMinimalBpmn, extractProcessId, hasStartEvent } from '@/design/minimalBpmn'
import { translate, translateModule } from '@/design/translate'

describe('最小 BPMN 生成', () => {
  it('生成含开始事件与 DI 的流程', () => {
    const xml = buildMinimalBpmn('leave', '请假')
    expect(xml).toContain('id="leave"')
    expect(xml).toContain('name="请假"')
    expect(hasStartEvent(xml)).toBe(true)
    expect(extractProcessId(xml)).toBe('leave')
  })

  it('转义流程 Key 与名称', () => {
    const xml = buildMinimalBpmn('a&b', '<审批>')
    expect(xml).toContain('id="a&amp;b"')
    expect(xml).toContain('name="&lt;审批&gt;"')
  })

  it('缺失流程时返回空串与 false', () => {
    expect(extractProcessId('<definitions />')).toBe('')
    expect(hasStartEvent('<definitions />')).toBe(false)
  })
})

describe('BPMN 翻译', () => {
  it('翻译常用词并替换占位符', () => {
    expect(translate('General')).toBe('常规')
    expect(translate('Name')).toBe('名称')
    expect(translate('Hello {name}', { name: '世界' })).toBe('Hello 世界')
    expect(translate('未收录')).toBe('未收录')
    const translator = translateModule.translate[1] as (template: string) => string
    expect(translator('General')).toBe('常规')
  })
})