import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  addInstanceAttachment,
  addInstanceComment,
  addInstanceIdentityLink,
  createInstanceVariable,
  definitionKeyOf,
  definitionVersionOf,
  deleteInstanceAttachment,
  deleteInstanceComment,
  deleteInstanceIdentityLink,
  deleteInstanceVariable,
  deleteProcessInstance,
  getInstanceVariableData,
  getProcessDefinition,
  getProcessDefinitionXml,
  getProcessInstance,
  inferVariableType,
  listDeadletterJobs,
  listHistoricActivities,
  listInstanceAttachments,
  listInstanceComments,
  listInstanceIdentityLinks,
  listInstanceTasks,
  listInstanceVariables,
  listLatestDefinitions,
  listProcessDefinitions,
  listProcessInstances,
  listRuntimeActivities,
  migrateInstance,
  startProcessInstance,
  updateInstanceVariable,
  updateProcessInstanceState,
  validateInstanceMigration,
} from '@/api/runtime'
import { mockFetch, pageOf } from '../../helpers'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('类型与 ID 工具', () => {
  it('推断变量类型', () => {
    expect(inferVariableType(null)).toBe('null')
    expect(inferVariableType(true)).toBe('boolean')
    expect(inferVariableType(12)).toBe('long')
    expect(inferVariableType(1.5)).toBe('double')
    expect(inferVariableType({ a: 1 })).toBe('json')
    expect(inferVariableType([1])).toBe('json')
    expect(inferVariableType('x')).toBe('string')
  })

  it('解析定义 Key 与版本', () => {
    expect(definitionKeyOf('leave:2:abc')).toBe('leave')
    expect(definitionKeyOf('my:key:3:abc')).toBe('my:key')
    expect(definitionKeyOf('opaque')).toBe('opaque')
    expect(definitionKeyOf('')).toBe('')
    expect(definitionVersionOf('leave:2:abc')).toBe(2)
    expect(definitionVersionOf('leave:x:abc')).toBeNull()
    expect(definitionVersionOf('opaque')).toBeNull()
    expect(definitionVersionOf('')).toBeNull()
  })
})

describe('实例接口', () => {
  it('GET 列表带上筛选、分页与 onlyActive 映射', async () => {
    const calls = mockFetch([{ path: '/service/runtime/process-instances?', body: pageOf([{ id: 'pi-1' }]) }])
    const result = await listProcessInstances({
      processDefinitionKey: 'leave',
      businessKey: 'B-1',
      tenantId: 't1',
      onlyActive: true,
      start: 50,
      size: 50,
      sort: 'startTime',
      order: 'desc',
    })
    expect(result.total).toBe(1)
    expect(calls[0].url).toContain('processDefinitionKey=leave')
    expect(calls[0].url).toContain('businessKey=B-1')
    expect(calls[0].url).toContain('tenantId=t1')
    expect(calls[0].url).toContain('suspended=false')
    expect(calls[0].url).toContain('start=50')
    expect(calls[0].url).toContain('sort=startTime')
    expect(calls[0].url).toContain('order=desc')
  })

  it('GET 详情、启动、状态动作与删除', async () => {
    const calls = mockFetch([
      { path: '/service/runtime/process-instances/pi-1', body: { id: 'pi-1' } },
      { path: '/service/runtime/process-instances', method: 'POST', status: 201, body: { id: 'pi-2' } },
      { path: '/service/runtime/process-instances/pi-1', method: 'PUT', body: { id: 'pi-1', suspended: true } },
      { path: '/service/runtime/process-instances/pi-1', method: 'DELETE', status: 204 },
    ])
    await expect(getProcessInstance('pi-1')).resolves.toMatchObject({ id: 'pi-1' })
    await expect(startProcessInstance({ processDefinitionKey: 'leave', businessKey: 'B-1', variables: { a: 1 } })).resolves.toMatchObject({ id: 'pi-2' })
    await expect(updateProcessInstanceState('pi-1', 'suspend')).resolves.toMatchObject({ suspended: true })
    await expect(deleteProcessInstance('pi-1', '重复申请')).resolves.toBeUndefined()
    expect(calls[1].body).toEqual({ processDefinitionKey: 'leave', businessKey: 'B-1', variables: { a: 1 } })
    expect(calls[2].body).toEqual({ action: 'suspend' })
    expect(calls[3].url).toContain('deleteReason=%E9%87%8D%E5%A4%8D%E7%94%B3%E8%AF%B7')
  })

  it('错误映射为 ApiError', async () => {
    mockFetch([{ path: '/service/runtime/process-instances/pi-404', status: 404, body: { message: 'Not found', exception: 'gone' } }])
    await expect(getProcessInstance('pi-404')).rejects.toMatchObject({ status: 404, message: 'Not found', exception: 'gone' })
  })
})

describe('实例变量接口', () => {
  it('列表按值补类型，并支持增删改与原始数据', async () => {
    const calls = mockFetch([
      { path: '/service/runtime/process-instances/pi-1/variables/a/data', text: 'raw', contentType: 'application/octet-stream' },
      { path: '/service/runtime/process-instances/pi-1/variables', body: [{ name: 'a', value: 1 }, { name: 'b', type: 'string', value: 'x' }] },
      { path: '/service/runtime/process-instances/pi-1/variables', method: 'POST', status: 201, body: { name: 'c', value: true } },
      { path: '/service/runtime/process-instances/pi-1/variables/a', method: 'PUT', status: 204 },
      { path: '/service/runtime/process-instances/pi-1/variables/a', method: 'DELETE', status: 204 },
    ])
    await expect(listInstanceVariables('pi-1')).resolves.toEqual([
      { name: 'a', value: 1, type: 'long' },
      { name: 'b', value: 'x', type: 'string' },
    ])
    await createInstanceVariable('pi-1', { name: 'c', type: 'boolean', value: true })
    await updateInstanceVariable('pi-1', 'a', { type: 'long', value: 2 })
    await deleteInstanceVariable('pi-1', 'a')
    await expect(getInstanceVariableData('pi-1', 'a')).resolves.toBeInstanceOf(Blob)
    expect(calls[1].body).toEqual({ name: 'c', type: 'boolean', value: true })
    expect(calls[2].body).toEqual({ name: 'a', type: 'long', value: 2 })
  })

  it('变量列表为空时返回空数组', async () => {
    mockFetch([{ path: '/service/runtime/process-instances/pi-1/variables', body: [] }])
    await expect(listInstanceVariables('pi-1')).resolves.toEqual([])
  })
})

describe('实例任务、评论、附件与身份链接', () => {
  it('任务分页使用 start/size', async () => {
    const calls = mockFetch([{ path: '/service/runtime/process-instances/pi-1/tasks', body: pageOf([{ id: 't-1', name: '审批' }]) }])
    await expect(listInstanceTasks('pi-1', { start: 10, size: 20 })).resolves.toMatchObject({ total: 1 })
    expect(calls[0].url).toContain('start=10')
    expect(calls[0].url).toContain('size=20')
  })

  it('评论增删与附件增删', async () => {
    const calls = mockFetch([
      { path: '/service/runtime/process-instances/pi-1/comments', body: pageOf([{ id: 'c-1', message: 'hi' }]) },
      { path: '/service/runtime/process-instances/pi-1/comments', method: 'POST', status: 201, body: { id: 'c-2', message: 'ok' } },
      { path: '/service/runtime/process-instances/pi-1/comments/c-1', method: 'DELETE', status: 204 },
      { path: '/service/runtime/process-instances/pi-1/attachments', body: pageOf([{ id: 'a-1', name: 'f' }]) },
      { path: '/service/runtime/process-instances/pi-1/attachments', method: 'POST', status: 201, body: { id: 'a-2' } },
      { path: '/service/runtime/process-instances/pi-1/attachments/a-1', method: 'DELETE', status: 204 },
    ])
    await expect(listInstanceComments('pi-1', { start: 0, size: 50 })).resolves.toMatchObject({ total: 1 })
    await addInstanceComment('pi-1', 'ok', 'alice')
    await deleteInstanceComment('pi-1', 'c-1')
    await expect(listInstanceAttachments('pi-1', { start: 0, size: 50 })).resolves.toMatchObject({ total: 1 })
    await addInstanceAttachment('pi-1', { name: 'f', type: 'url', url: 'https://example.com' })
    await deleteInstanceAttachment('pi-1', 'a-1')
    expect(calls[1].body).toEqual({ message: 'ok', userId: 'alice' })
    expect(calls[4].body).toEqual({ name: 'f', type: 'url', url: 'https://example.com' })
  })

  it('身份链接按用户或组生成删除路径', async () => {
    const calls = mockFetch([
      { path: '/service/runtime/process-instances/pi-1/identitylinks', body: [{ id: 'l-1', type: 'candidate', userId: 'alice' }] },
      { path: '/service/runtime/process-instances/pi-1/identitylinks', method: 'POST', status: 201, body: { id: 'l-2' } },
      { path: '/service/runtime/process-instances/pi-1/identitylinks/users/alice/candidate', method: 'DELETE', status: 204 },
      { path: '/service/runtime/process-instances/pi-1/identitylinks/groups/team/starter', method: 'DELETE', status: 204 },
    ])
    await expect(listInstanceIdentityLinks('pi-1')).resolves.toHaveLength(1)
    await addInstanceIdentityLink('pi-1', { type: 'candidate', userId: 'alice' })
    await deleteInstanceIdentityLink('pi-1', { type: 'candidate', userId: 'alice' })
    await deleteInstanceIdentityLink('pi-1', { type: 'starter', groupId: 'team' })
    expect(calls[1].body).toEqual({ type: 'candidate', userId: 'alice' })
    expect(calls[2].url).toContain('/identitylinks/users/alice/candidate')
    expect(calls[3].url).toContain('/identitylinks/groups/team/starter')
  })
})

describe('定义与活动接口', () => {
  it('定义列表、详情、XML 与最新版本', async () => {
    mockFetch([
      { path: '/service/repository/process-definitions?', body: pageOf([{ id: 'd-1', key: 'leave', version: 1 }]) },
      { path: '/service/repository/process-definitions/d-1/xml', text: '<definitions />', contentType: 'application/xml' },
      { path: '/service/repository/process-definitions/d-1', body: { id: 'd-1', key: 'leave', version: 1 } },
    ])
    await expect(listProcessDefinitions({ key: 'leave', size: 10 })).resolves.toMatchObject({ total: 1 })
    await expect(getProcessDefinitionXml('d-1')).resolves.toContain('<definitions')
    await expect(getProcessDefinition('d-1')).resolves.toMatchObject({ key: 'leave' })
  })

  it('最新定义按 Key 去重、排除挂起并排序', async () => {
    mockFetch([
      {
        path: '/service/repository/process-definitions?',
        body: pageOf([
          { id: 'z:3:1', key: 'z', name: 'Z', version: 3 },
          { id: 'a:1:1', key: 'a', name: 'A', version: 1 },
          { id: 'a:2:1', key: 'a', name: 'A', version: 2 },
          { id: 'b:1:1', key: 'b', name: 'B', version: 1, suspended: true },
        ]),
      },
    ])
    const latest = await listLatestDefinitions()
    expect(latest.map((item) => item.id)).toEqual(['a:2:1', 'z:3:1'])
  })

  it('活动与死信作业', async () => {
    mockFetch([
      { path: '/service/runtime/activity-instances', body: pageOf([{ id: 'e-1', activityId: 'taskA' }]) },
      { path: '/service/history/historic-activity-instances', body: pageOf([{ id: 'h-1', activityId: 'start' }]) },
      { path: '/service/management/deadletter-jobs', body: pageOf([{ id: 'j-1', activityId: 'taskA' }]) },
    ])
    await expect(listRuntimeActivities('pi-1')).resolves.toMatchObject({ total: 1 })
    await expect(listHistoricActivities('pi-1')).resolves.toMatchObject({ total: 1 })
    await expect(listDeadletterJobs()).resolves.toMatchObject({ total: 1 })
  })
})

describe('迁移接口', () => {
  it('校验返回阻断信息，执行迁移无返回体', async () => {
    const calls = mockFetch([
      { path: '/service/runtime/process-instances/pi-1/validate-migration', method: 'POST', body: { validationMessages: ['活动不匹配'] } },
      { path: '/service/runtime/process-instances/pi-1/migrate', method: 'POST', status: 204 },
    ])
    await expect(validateInstanceMigration('pi-1', { targetProcessDefinitionId: 'd-2' })).resolves.toEqual({ validationMessages: ['活动不匹配'] })
    await expect(migrateInstance('pi-1', { targetProcessDefinitionId: 'd-2', activityMappings: { a: 'b' } })).resolves.toBeUndefined()
    expect(calls[1].body).toEqual({ targetProcessDefinitionId: 'd-2', activityMappings: { a: 'b' } })
  })
})