// 工作流域 API 客户端单测：R 信封三态解包（成功 / 失败 / 501）与行提取兼容性。
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  deployWorkflowProcess,
  envelopeToResult,
  extractRows,
  extractTotal,
  getWorkflowHighlight,
  listTodoTasks,
  listWorkflowCategories,
  stopWorkflowProcessInstance,
} from '@/api/workflow'
import { mockFetch } from '../helpers'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('R 信封解包', () => {
  it('成功态：R{code:200, success:true, data} 解包出业务数据', async () => {
    const calls = mockFetch([
      {
        path: '/workflow/category/list',
        body: { code: '200', msg: 'ok', data: { list: [{ id: 'c1', categoryName: '行政' }] }, success: true },
      },
    ])
    const result = await listWorkflowCategories()
    expect(result.success).toBe(true)
    expect(result.notImplemented).toBe(false)
    expect(result.code).toBe('200')
    expect(result.msg).toBe('ok')
    expect(result.data).toEqual({ list: [{ id: 'c1', categoryName: '行政' }] })
    expect(calls[0].method).toBe('GET')
  })

  it('失败态：R{success:false, ErrorMessage} 折叠为失败结果并透出 ErrorMessage', async () => {
    const calls = mockFetch([
      {
        path: '/workflow/process/deploy',
        method: 'POST',
        status: 200,
        body: { code: '500', ErrorMessage: '模型尚未配置 BPMN', success: false },
      },
    ])
    const result = await deployWorkflowProcess({ modelId: 'm1', tenantId: 'crm' })
    expect(result.success).toBe(false)
    expect(result.notImplemented).toBe(false)
    expect(result.errorMessage).toBe('模型尚未配置 BPMN')
    expect(result.code).toBe('500')
    expect(result.data).toBeNull()
    expect(calls[0].body).toEqual({ modelId: 'm1', tenantId: 'crm' })
  })

  it('501 未接线：HTTP 501 + R 失败体识别为 notImplemented，页面据此空态降级', async () => {
    mockFetch([
      {
        path: '/workflow/process/task/all/todoList',
        status: 501,
        body: { code: '500', ErrorMessage: '接口未实现', success: false },
      },
    ])
    const result = await listTodoTasks({ identityId: 'u1', pageNum: 1, pageSize: 20 })
    expect(result.success).toBe(false)
    expect(result.notImplemented).toBe(true)
    expect(result.httpStatus).toBe(501)
    expect(result.errorMessage).toBe('接口未实现')
  })

  it('裸数据响应（无 success 字段）整体视为业务数据', () => {
    const result = envelopeToResult<string>('<definitions/>')
    expect(result.success).toBe(true)
    expect(result.data).toBe('<definitions/>')
    expect(result.notImplemented).toBe(false)
  })
})

describe('高亮接口方法降级', () => {
  it('GET 失败（非 501）时按 DELETE 重试一次（路由表登记为 DELETE 的兼容处理）', async () => {
    const calls = mockFetch([
      { path: '/workflow/process/highlight/img', status: 404, body: { code: 'REST-0404', message: '未模拟' } },
      {
        path: '/workflow/process/highlight/img',
        method: 'DELETE',
        body: {
          code: '200',
          success: true,
          data: { bpmnXml: '<definitions/>', completed: ['a1'], active: ['a2'] },
        },
      },
    ])
    const result = await getWorkflowHighlight('pi-1')
    expect(result.success).toBe(true)
    expect(result.data?.bpmnXml).toBe('<definitions/>')
    expect(calls[0].method).toBe('GET')
    expect(calls[1].method).toBe('DELETE')
    expect(calls[1].url).toContain('processInstanceId=pi-1')
  })

  it('GET 直接 501 时不重试，返回 notImplemented', async () => {
    const calls = mockFetch([
      { path: '/workflow/process/highlight/img', status: 501, body: { code: '500', ErrorMessage: '接口未实现', success: false } },
    ])
    const result = await getWorkflowHighlight('pi-1')
    expect(result.notImplemented).toBe(true)
    expect(calls).toHaveLength(1)
  })
})

describe('实例行操作', () => {
  it('终止实例走 POST /workflow/process/stop/processInstance/{pid}', async () => {
    const calls = mockFetch([
      { path: '/workflow/process/stop/processInstance/pi-1', method: 'POST', body: { code: '200', success: true, msg: '已终止' } },
    ])
    const result = await stopWorkflowProcessInstance('pi-1')
    expect(result.success).toBe(true)
    expect(result.msg).toBe('已终止')
    expect(calls[0].method).toBe('POST')
    expect(calls[0].url).toBe('/workflow/process/stop/processInstance/pi-1')
  })
})

describe('行提取兼容性', () => {
  it('extractRows 兼容数组 / list / records 信封并补 _rid 行键', () => {
    expect(extractRows([{ id: 'a' }, { id: 'b' }]).map((row) => row._rid)).toEqual(['a', 'b'])
    expect(extractRows({ list: [{ id: 'c1' }] })[0]._rid).toBe('c1')
    expect(extractRows({ records: [{ code: 'x' }] })[0]._rid).toBe('1')
    expect(extractRows(null)).toEqual([])
  })

  it('extractTotal 兼容 total / totalCount，缺失时回退为行数', () => {
    expect(extractTotal({ list: [], total: 42 }, [])).toBe(42)
    expect(extractTotal({ records: [], totalCount: 7 }, [])).toBe(7)
    expect(extractTotal([{ id: 'a' }], [{}])).toBe(1)
    expect(extractTotal(null, [1, 2, 3])).toBe(3)
  })
})
