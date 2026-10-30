// 运维域 API 测试：URL、方法、请求体与错误映射。
import { afterEach, describe, expect, it, vi } from 'vitest'
import { mockFetch, pageOf } from '../helpers'
import {
  applyJobAction,
  countJobs,
  createEngineProperty,
  deleteBatch,
  deleteEngineProperty,
  deleteJob,
  getBatch,
  getBatchDocument,
  getBatchPart,
  getBatchPartDocument,
  getBatchSummary,
  getJob,
  getJobStacktrace,
  getProcessInstanceBrief,
  getTableColumns,
  getTableData,
  listAuditLogs,
  listBatchParts,
  listBatches,
  listEngineProperties,
  listJobs,
  listTables,
  runDueJobs,
  updateEngineProperty,
} from '@/api/management'
import type { JobBucket } from '@/api/management'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('作业 API', () => {
  it.each([
    ['executable', '/service/management/jobs'],
    ['timer', '/service/management/timer-jobs'],
    ['suspended', '/service/management/suspended-jobs'],
    ['deadletter', '/service/management/deadletter-jobs'],
  ])('查询 %s 桶列表', async (bucket, path) => {
    const calls = mockFetch([{ method: 'GET', path: new RegExp(`^${path}\\?start=10&size=20$`), body: pageOf([], 7) }])
    const page = await listJobs(bucket as JobBucket, { start: 10, size: 20 })
    expect(page.total).toBe(7)
    expect(calls[0].url).toBe(`${path}?start=10&size=20`)
  })

  it('作业列表使用默认分页', async () => {
    const calls = mockFetch([{ method: 'GET', path: '/service/management/jobs?start=0&size=50', body: pageOf([]) }])
    await listJobs('executable', {})
    expect(calls[0].url).toBe('/service/management/jobs?start=0&size=50')
  })

  it('查询桶总数', async () => {
    const calls = mockFetch([{ method: 'GET', path: '/service/management/deadletter-jobs?start=0&size=1', body: pageOf([], 12) }])
    await expect(countJobs('deadletter')).resolves.toBe(12)
    expect(calls[0].method).toBe('GET')
  })

  it('查询作业详情并对 ID 编码', async () => {
    const job = { id: 'a b', retries: 2 }
    const calls = mockFetch([{ method: 'GET', path: '/service/management/jobs/a%20b', body: job }])
    await expect(getJob('executable', 'a b')).resolves.toEqual(job)
    expect(calls[0].url).toBe('/service/management/jobs/a%20b')
  })

  it('查询异常栈返回文本', async () => {
    const calls = mockFetch([{ method: 'GET', path: '/service/management/deadletter-jobs/j1/exception-stacktrace', text: 'Error: boom' }])
    await expect(getJobStacktrace('deadletter', 'j1')).resolves.toBe('Error: boom')
    expect(calls[0].method).toBe('GET')
  })

  it('设置可执行作业重试次数', async () => {
    const calls = mockFetch([{ method: 'POST', path: '/service/management/jobs/j1', status: 204 }])
    await applyJobAction('executable', 'j1', { action: 'setRetries', retries: 3 })
    expect(calls[0].body).toEqual({ action: 'setRetries', retries: 3 })
  })

  it('定时器重新调度只发送 dueDate', async () => {
    const calls = mockFetch([{ method: 'POST', path: '/service/management/timer-jobs/j2', status: 204 }])
    await applyJobAction('timer', 'j2', { action: 'reschedule', dueDate: '2026-11-01T00:00:00.000Z' })
    expect(calls[0].body).toEqual({ action: 'reschedule', dueDate: '2026-11-01T00:00:00.000Z' })
  })

  it('死信恢复发送 move 与 retries', async () => {
    const calls = mockFetch([{ method: 'POST', path: '/service/management/deadletter-jobs/j3', status: 204 }])
    await applyJobAction('deadletter', 'j3', { action: 'move', retries: 5 })
    expect(calls[0].body).toEqual({ action: 'move', retries: 5 })
  })

  it('挂起作业动作在客户端拒绝', async () => {
    const calls = mockFetch([])
    await expect(applyJobAction('suspended', 'j4', { action: 'move' })).rejects.toMatchObject({ status: 400, code: 'CLIENT-UNSUPPORTED' })
    expect(calls).toHaveLength(0)
  })

  it('删除作业与立即执行', async () => {
    const calls = mockFetch([
      { method: 'DELETE', path: '/service/management/timer-jobs/j5', status: 204 },
      { method: 'POST', path: '/service/management/jobs/run', body: { executed: 3 } },
    ])
    await deleteJob('timer', 'j5')
    await expect(runDueJobs()).resolves.toEqual({ executed: 3 })
    expect(calls[0].method).toBe('DELETE')
    expect(calls[1].method).toBe('POST')
  })
})

describe('批处理 API', () => {
  it('查询批次列表并传递筛选分页', async () => {
    const calls = mockFetch([{ method: 'GET', path: /^\/service\/management\/batches\?/, body: pageOf([], 2) }])
    await listBatches({ type: 'migration', status: 'failed', tenantId: 't1', start: 0, size: 50 })
    expect(calls[0].url).toContain('type=migration')
    expect(calls[0].url).toContain('status=failed')
    expect(calls[0].url).toContain('tenantId=t1')
    expect(calls[0].url).toContain('start=0')
    expect(calls[0].url).toContain('size=50')
  })

  it('查询批次、分片、摘要与文档', async () => {
    const calls = mockFetch([
      { method: 'GET', path: '/service/management/batches/b1/batch-document', body: { a: 1 } },
      { method: 'GET', path: '/service/management/batches/b1/summary', body: { id: 'b1' } },
      { method: 'GET', path: '/service/management/batches/b1/batch-parts?start=0&size=50', body: pageOf([], 1) },
      { method: 'GET', path: '/service/management/batch-parts/p1/batch-part-document', body: { b: 2 } },
      { method: 'GET', path: '/service/management/batch-parts/p1', body: { id: 'p1' } },
      { method: 'GET', path: '/service/management/batches/b1', body: { id: 'b1' } },
    ])
    await getBatch('b1')
    await listBatchParts('b1')
    await getBatchPart('p1')
    await getBatchSummary('b1')
    await expect(getBatchDocument('b1')).resolves.toEqual({ a: 1 })
    await expect(getBatchPartDocument('p1')).resolves.toEqual({ b: 2 })
    expect(calls).toHaveLength(6)
  })

  it('删除批次', async () => {
    const calls = mockFetch([{ method: 'DELETE', path: '/service/management/batches/b2', status: 204 }])
    await deleteBatch('b2')
    expect(calls[0].method).toBe('DELETE')
  })

  it('错误映射为 ApiError', async () => {
    mockFetch([
      {
        method: 'GET',
        path: '/service/management/batches/missing',
        status: 404,
        body: { code: 'ENGINE-0700', message: '批处理作业不存在', exception: 'stack' },
      },
    ])
    await expect(getBatch('missing')).rejects.toMatchObject({
      status: 404,
      code: 'ENGINE-0700',
      message: '批处理作业不存在',
      exception: 'stack',
    })
  })
})

describe('表数据 API', () => {
  it('查询表清单与列元数据', async () => {
    const calls = mockFetch([
      { method: 'GET', path: '/service/management/tables/ACT_RE_MODEL/columns', body: { tableName: 'ACT_RE_MODEL', columnNames: ['ID_'], columnTypes: ['varchar'] } },
      { method: 'GET', path: '/service/management/tables', body: [{ name: 'ACT_RE_MODEL', count: 2 }] },
    ])
    await expect(listTables()).resolves.toHaveLength(1)
    const meta = await getTableColumns('ACT_RE_MODEL')
    expect(meta.columnNames).toEqual(['ID_'])
    expect(calls[1].method).toBe('GET')
  })

  it('表数据默认分页并按方向映射排序参数', async () => {
    const calls = mockFetch([
      { method: 'GET', path: /^\/service\/management\/tables\/ACT_RU_JOBS\/data\?/, body: pageOf([]) },
    ])
    await getTableData('ACT_RU_JOBS', { start: 50, size: 100, sortColumn: 'DUEDATE_', order: 'desc' })
    expect(calls[0].url).toContain('start=50')
    expect(calls[0].url).toContain('size=100')
    expect(calls[0].url).toContain('orderDescendingColumn=DUEDATE_')
    expect(calls[0].url).not.toContain('orderAscendingColumn')
  })

  it('表数据默认按 ID_ 且无排序参数', async () => {
    const calls = mockFetch([{ method: 'GET', path: /^\/service\/management\/tables\/T\/data\?/, body: pageOf([]) }])
    await getTableData('T')
    expect(calls[0].url).toBe('/service/management/tables/T/data?start=0&size=50')
  })
})

describe('引擎属性 API', () => {
  it('查询、创建、更新与删除属性', async () => {
    const calls = mockFetch([
      { method: 'GET', path: '/service/management/engine-properties', body: { 'schema.version': '8' } },
      { method: 'POST', path: '/service/management/engine-properties', status: 201, body: { name: 'feature.x', value: '1' } },
      { method: 'PUT', path: '/service/management/engine-properties/feature.x', body: { name: 'feature.x', value: '2' } },
      { method: 'DELETE', path: '/service/management/engine-properties/feature.x', status: 204 },
    ])
    await expect(listEngineProperties()).resolves.toEqual({ 'schema.version': '8' })
    await createEngineProperty('feature.x', '1')
    await updateEngineProperty('feature.x', '2')
    await deleteEngineProperty('feature.x')
    expect(calls[1].body).toEqual({ name: 'feature.x', value: '1' })
    expect(calls[2].body).toEqual({ name: 'feature.x', value: '2' })
    expect(calls[2].method).toBe('PUT')
    expect(calls[3].method).toBe('DELETE')
  })
})

describe('审计与实例摘要 API', () => {
  it('查询审计日志', async () => {
    const calls = mockFetch([{ method: 'GET', path: '/service/management/audit-logs?start=0&size=1000', body: pageOf([], 8) }])
    await expect(listAuditLogs({ start: 0, size: 1000 })).resolves.toMatchObject({ total: 8 })
    expect(calls[0].url).toContain('size=1000')
  })

  it('查询流程实例摘要', async () => {
    const calls = mockFetch([
      { method: 'GET', path: '/service/runtime/process-instances/pi-1', body: { id: 'pi-1', processDefinitionId: 'def-1' } },
    ])
    await expect(getProcessInstanceBrief('pi-1')).resolves.toMatchObject({ processDefinitionId: 'def-1' })
    expect(calls[0].url).toBe('/service/runtime/process-instances/pi-1')
  })
})
