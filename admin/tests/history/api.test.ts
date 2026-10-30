import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  deleteHistoricDetail,
  deleteHistoricProcessInstance,
  deleteHistoricTaskInstance,
  deleteHistoricVariable,
  getHistoricActivity,
  getHistoricDetailData,
  getHistoricProcessInstance,
  getHistoricProcessVariableData,
  getHistoricTaskForm,
  getHistoricTaskInstance,
  getHistoricTaskVariableData,
  getHistoricVariable,
  getHistoricVariableData,
  listHistoricProcessComments,
  listHistoricProcessIdentityLinks,
  listHistoricProcessVariables,
  listHistoricTaskIdentityLinks,
  listHistoricTaskVariables,
  queryHistoricActivities,
  queryHistoricDetails,
  queryHistoricProcessInstances,
  queryHistoricTaskInstances,
  queryHistoricVariables,
} from '@/api/history'
import { ApiError } from '@/api/client'
import { mockFetch, pageOf } from '../helpers'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('历史流程接口', () => {
  it('查询历史流程实例：POST 查询体保留 false 并去掉空值', async () => {
    const calls = mockFetch([{ path: '/service/query/historic-process-instances', method: 'POST', body: pageOf([{ id: 'pi-1' }]) }])
    const result = await queryHistoricProcessInstances({
      processDefinitionKey: 'leave',
      businessKey: '',
      tenantId: undefined,
      finished: false,
      startedAfter: '2026-10-01T00:00:00Z',
      start: 0,
      size: 50,
      sort: 'startTime',
      order: 'desc',
    })
    expect(result.data).toHaveLength(1)
    expect(calls[0].url).toBe('/service/query/historic-process-instances')
    expect(calls[0].method).toBe('POST')
    expect(calls[0].body).toEqual({
      processDefinitionKey: 'leave',
      finished: false,
      startedAfter: '2026-10-01T00:00:00Z',
      start: 0,
      size: 50,
      sort: 'startTime',
      order: 'desc',
    })
  })

  it('读取、评论、身份链接与删除历史流程实例', async () => {
    const calls = mockFetch([
      { path: '/service/history/historic-process-instances/pi%201/variables', body: pageOf([{ id: 'v1', name: 'amount' }]) },
      { path: '/service/history/historic-process-instances/pi%201/comments', body: [{ id: 'c1', message: 'ok' }] },
      { path: '/service/history/historic-process-instances/pi%201/identitylinks', body: [{ type: 'starter', userId: 'alice' }] },
      { path: '/service/history/historic-process-instances/pi%201', method: 'DELETE', status: 204 },
      { path: '/service/history/historic-process-instances/pi%201', body: { id: 'pi 1' } },
    ])
    await expect(getHistoricProcessInstance('pi 1')).resolves.toEqual({ id: 'pi 1' })
    await expect(listHistoricProcessVariables('pi 1', { start: 0, size: 1000 })).resolves.toMatchObject({ data: [{ name: 'amount' }] })
    await expect(listHistoricProcessComments('pi 1')).resolves.toHaveLength(1)
    await expect(listHistoricProcessIdentityLinks('pi 1')).resolves.toHaveLength(1)
    await expect(deleteHistoricProcessInstance('pi 1')).resolves.toBeUndefined()
    expect(calls[1].url).toBe('/service/history/historic-process-instances/pi%201/variables?start=0&size=1000')
    expect(calls[4].method).toBe('DELETE')
  })

  it('读取历史流程变量原始数据（二进制）', async () => {
    const calls = mockFetch([
      { path: '/service/history/historic-process-instances/pi-1/variables/a%2Fb/data', text: 'blob', contentType: 'application/octet-stream' },
    ])
    const blob = await getHistoricProcessVariableData('pi-1', 'a/b')
    expect(blob).toBeInstanceOf(Blob)
    expect(blob.size).toBe(4)
    expect(calls[0].url).toBe('/service/history/historic-process-instances/pi-1/variables/a%2Fb/data')
  })
})

describe('历史任务接口', () => {
  it('查询历史任务：POST 查询体与布尔过滤', async () => {
    const calls = mockFetch([{ path: '/service/query/historic-task-instances', method: 'POST', body: pageOf([{ id: 't1' }]) }])
    const result = await queryHistoricTaskInstances({ assignee: 'alice', finished: true, name: '', start: 10, size: 20 })
    expect(result.total).toBe(1)
    expect(calls[0].body).toEqual({ assignee: 'alice', finished: true, start: 10, size: 20 })
  })

  it('读取任务、表单、变量、原始数据、身份链接与删除', async () => {
    const calls = mockFetch([
      { path: '/service/history/historic-task-instances/t1/form', body: { formKey: 'f', formProperties: [] } },
      { path: '/service/history/historic-task-instances/t1/variables/x/data', text: 'raw', contentType: 'application/octet-stream' },
      { path: '/service/history/historic-task-instances/t1/identitylinks', body: [{ type: 'candidate', userId: 'bob' }] },
      { path: '/service/query/historic-task-instances', method: 'POST', body: pageOf([{ id: 't1', variables: [{ name: 'x', type: 'string', value: '1' }] }]) },
      { path: '/service/history/historic-task-instances/t1', method: 'DELETE', status: 204 },
      { path: '/service/history/historic-task-instances/t1', body: { id: 't1' } },
    ])
    await expect(getHistoricTaskForm('t1')).resolves.toMatchObject({ formKey: 'f' })
    const blob = await getHistoricTaskVariableData('t1', 'x')
    expect(blob.size).toBe(3)
    await expect(listHistoricTaskIdentityLinks('t1')).resolves.toHaveLength(1)
    await expect(listHistoricTaskVariables('t1')).resolves.toEqual([{ name: 'x', type: 'string', value: '1' }])
    await expect(getHistoricTaskInstance('t1')).resolves.toEqual({ id: 't1' })
    await expect(deleteHistoricTaskInstance('t1')).resolves.toBeUndefined()
    expect(calls[3].body).toEqual({ id: 't1', includeTaskLocalVariables: true, includeProcessVariables: true, start: 0, size: 1 })
  })

  it('历史任务没有变量时返回空数组', async () => {
    mockFetch([{ path: '/service/query/historic-task-instances', method: 'POST', body: pageOf([]) }])
    await expect(listHistoricTaskVariables('missing')).resolves.toEqual([])
  })
})

describe('历史活动接口', () => {
  it('查询与读取历史活动', async () => {
    const calls = mockFetch([
      { path: '/service/query/historic-activity-instances', method: 'POST', body: pageOf([{ id: 'a1' }]) },
      { path: '/service/history/historic-activity-instances/a1', body: { id: 'a1', activityId: 'task1' } },
    ])
    await expect(queryHistoricActivities({ processInstanceId: 'pi-1', finished: false, size: 50 })).resolves.toMatchObject({ total: 1 })
    await expect(getHistoricActivity('a1')).resolves.toMatchObject({ activityId: 'task1' })
    expect(calls[0].body).toEqual({ processInstanceId: 'pi-1', finished: false, size: 50 })
  })
})

describe('历史变量与明细接口', () => {
  it('查询、读取、下载与删除历史变量', async () => {
    const calls = mockFetch([
      { path: '/service/query/historic-variable-instances', method: 'POST', body: pageOf([{ id: 'v1' }]) },
      { path: '/service/history/historic-variable-instances/v1/data', text: 'value', contentType: 'application/octet-stream' },
      { path: '/service/history/historic-variable-instances/v1', body: { id: 'v1' } },
      { path: '/service/history/historic-variable-instances/v1', method: 'DELETE', status: 204 },
    ])
    await expect(queryHistoricVariables({ name: 'amount', type: 'string', size: 50 })).resolves.toMatchObject({ total: 1 })
    await expect(getHistoricVariable('v1')).resolves.toEqual({ id: 'v1' })
    const blob = await getHistoricVariableData('v1')
    expect(blob.size).toBe(5)
    await expect(deleteHistoricVariable('v1')).resolves.toBeUndefined()
    expect(calls[0].body).toEqual({ name: 'amount', type: 'string', size: 50 })
  })

  it('查询、下载与删除历史明细', async () => {
    const calls = mockFetch([
      { path: '/service/query/historic-detail', method: 'POST', body: pageOf([{ id: 'd1' }]) },
      { path: '/service/history/historic-detail/d1/data', text: 'raw', contentType: 'application/octet-stream' },
      { path: '/service/history/historic-detail/d1', method: 'DELETE', status: 204 },
    ])
    await expect(queryHistoricDetails({ taskId: 't1', timeAfter: '2026-10-01T00:00:00Z', size: 50 })).resolves.toMatchObject({ total: 1 })
    const blob = await getHistoricDetailData('d1')
    expect(blob.size).toBe(3)
    await expect(deleteHistoricDetail('d1')).resolves.toBeUndefined()
    expect(calls[0].body).toEqual({ taskId: 't1', timeAfter: '2026-10-01T00:00:00Z', size: 50 })
  })
})

describe('错误映射', () => {
  it('把服务端错误体映射为 ApiError', async () => {
    mockFetch([{ path: '/service/history/historic-process-instances', status: 404, body: { code: 'REST-0404', message: '未找到' } }])
    const error = await getHistoricProcessInstance('missing').catch((cause: unknown) => cause)
    expect(error).toBeInstanceOf(ApiError)
    expect((error as ApiError).status).toBe(404)
    expect((error as ApiError).code).toBe('REST-0404')
    expect((error as ApiError).message).toBe('未找到')
  })

  it('删除运行时返回 500 文本时保留服务端说明', async () => {
    mockFetch([
      {
        path: '/service/history/historic-process-instances/running',
        method: 'DELETE',
        status: 500,
        body: { message: 'Process instance is still running' },
      },
    ])
    await expect(deleteHistoricProcessInstance('running')).rejects.toMatchObject({
      status: 500,
      message: 'Process instance is still running',
    })
  })
})
