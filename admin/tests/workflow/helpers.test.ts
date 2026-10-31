// 工作流页面工具单测：列表状态机三态、字段兼容读取与操作错误文案。
import { afterEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { mount } from '@vue/test-utils'
import { actionErrorOf, boolField, pickField, useWorkflowListSource, yesNoText } from '@/pages/workflow/helpers'
import type { WorkflowResult } from '@/api/workflow'

afterEach(() => {
  vi.unstubAllGlobals()
})

// ok 构造成功结果。
// @param data 业务数据
// @return 成功 WorkflowResult
function ok(data: unknown): WorkflowResult<unknown> {
  return { success: true, data, msg: '', errorMessage: '', code: '200', httpStatus: 200, notImplemented: false }
}

// fail 构造失败结果。
// @param message 失败文案
// @param notImplemented 是否 501
// @return 失败 WorkflowResult
function fail(message: string, notImplemented = false): WorkflowResult<unknown> {
  return { success: false, data: null, msg: '', errorMessage: message, code: '500', httpStatus: notImplemented ? 501 : 400, notImplemented }
}

describe('pickField / boolField / yesNoText', () => {
  it('pickField 按优先级读取并跳过空值', () => {
    expect(pickField<string>({ a: '', b: 'x' }, 'a', 'b')).toBe('x')
    expect(pickField<number>({ sort: 3 }, 'sort', 'sortNo')).toBe(3)
    expect(pickField({}), 'a').toBeUndefined()
    expect(pickField(null, 'a')).toBeUndefined()
  })

  it('boolField 兼容 boolean / number / string', () => {
    expect(boolField({ a: true }, 'a')).toBe(true)
    expect(boolField({ a: 'true' }, 'a')).toBe(true)
    expect(boolField({ a: 1 }, 'a')).toBe(true)
    expect(boolField({ a: '0' }, 'a')).toBe(false)
    expect(boolField({}, 'a')).toBe(false)
  })

  it('yesNoText 输出是/否', () => {
    expect(yesNoText(true)).toBe('是')
    expect(yesNoText(false)).toBe('否')
  })
})

describe('actionErrorOf', () => {
  it('501 返回降级文案，失败返回 ErrorMessage，成功返回 null', () => {
    expect(actionErrorOf(fail('接口未实现', true), '部署接口接线中')).toBe('部署接口接线中')
    expect(actionErrorOf(fail('模型不存在'), 'x')).toBe('模型不存在')
    expect(actionErrorOf(ok(null), 'x')).toBeNull()
  })
})

describe('useWorkflowListSource', () => {
  // Harness 在组件 setup 内调用组合式函数，满足 onUnmounted 的实例上下文要求。
  const Harness = defineComponent({
    setup(_, { expose }) {
      let fetcher: () => Promise<WorkflowResult<unknown>> = () => Promise.resolve(ok([{ id: 'r1', name: '行一' }]))
      const source = useWorkflowListSource<Record<string, unknown>>(() => fetcher(), '接线中提示')
      expose({ source, setFetcher: (next: () => Promise<WorkflowResult<unknown>>) => { fetcher = next } })
      return () => h('div')
    },
  })

  it('通过暴露接口验证三态', async () => {
    const wrapper = mount(Harness)
    const exposed = wrapper.vm as unknown as {
      source: ReturnType<typeof useWorkflowListSource>
      setFetcher: (next: () => Promise<WorkflowResult<unknown>>) => void
    }
    await exposed.source.load()
    expect(exposed.source.rows.value).toHaveLength(1)
    expect(exposed.source.rows.value[0]._rid).toBe('r1')
    expect(exposed.source.total.value).toBe(1)
    expect(exposed.source.degraded.value).toBe('')
    expect(exposed.source.errorMessage.value).toBe('')

    // 501 → 降级文案
    exposed.setFetcher(() => Promise.resolve(fail('接口未实现', true)))
    await exposed.source.load()
    expect(exposed.source.degraded.value).toBe('接线中提示')
    expect(exposed.source.rows.value).toHaveLength(0)

    // 失败 → 错误文案
    exposed.setFetcher(() => Promise.resolve(fail('服务不可用')))
    await exposed.source.load()
    expect(exposed.source.errorMessage.value).toBe('服务不可用')
    expect(exposed.source.degraded.value).toBe('')
  })
})
