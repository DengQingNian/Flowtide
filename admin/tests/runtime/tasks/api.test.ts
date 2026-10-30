// 运行态任务 API 模块测试：URL、方法、请求体、multipart 与错误映射。
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/api/client'
import {
  addTaskCandidate,
  addTaskComment,
  bulkUpdateTasks,
  createTask,
  createTaskVariable,
  deleteTask,
  deleteTaskAttachment,
  deleteTaskComment,
  deleteTaskIdentityLink,
  deleteTaskVariable,
  getTask,
  getTaskAttachmentContent,
  getTaskForm,
  getTaskVariableData,
  listSubTasks,
  listTaskAttachments,
  listTaskComments,
  listTaskEvents,
  listTaskIdentityLinks,
  listTaskVariables,
  queryTasks,
  taskAction,
  updateTask,
  updateTaskVariable,
  uploadTaskAttachment,
} from '@/api/task'
import { mockFetch, pageOf } from '../../helpers'

afterEach(() => {
  vi.unstubAllGlobals()
})

/** 任务样例 */
const task = {
  id: 't-1',
  name: '审批',
  assignee: 'alice',
  priority: 50,
  createTime: '2026-10-30T03:00:00Z',
  processInstanceId: 'pi-1',
  processDefinitionId: 'leave:1:1',
}

describe('task API', () => {
  it('queryTasks 使用 POST /query/tasks 并携带过滤与分页', async () => {
    const calls = mockFetch([{ path: '/service/query/tasks', method: 'POST', body: pageOf([task]) }])
    const page = await queryTasks({ assignee: 'alice', nameLike: '审', start: 50, size: 50, sort: 'createTime', order: 'desc' })
    expect(page.total).toBe(1)
    expect(calls[0].url).toBe('/service/query/tasks')
    expect(calls[0].body).toEqual({ assignee: 'alice', nameLike: '审', start: 50, size: 50, sort: 'createTime', order: 'desc' })
  })

  it('queryTasks 过滤空字符串但保留 0 与 false', async () => {
    const calls = mockFetch([{ path: '/service/query/tasks', method: 'POST', body: pageOf([]) }])
    await queryTasks({ assignee: '', candidateUser: undefined, start: 0, size: 10 })
    expect(calls[0].body).toEqual({ start: 0, size: 10 })
  })

  it('getTask 与编码后的任务路径', async () => {
    const calls = mockFetch([{ path: '/service/runtime/tasks/a%2Fb', body: task }])
    await getTask('a/b')
    expect(calls[0].method).toBe('GET')
    expect(calls[0].url).toBe('/service/runtime/tasks/a%2Fb')
  })

  it('createTask 提交 JSON 并忽略空字段', async () => {
    const calls = mockFetch([{ path: '/service/runtime/tasks', method: 'POST', status: 201, body: task }])
    const created = await createTask({ name: '审批', priority: 70, assignee: '', dueDate: '2026-11-01T00:00:00Z' })
    expect(created.id).toBe('t-1')
    expect(calls[0].body).toEqual({ name: '审批', priority: 70, dueDate: '2026-11-01T00:00:00Z' })
  })

  it('updateTask 与 deleteTask 使用真实方法与查询参数', async () => {
    const calls = mockFetch([
      { path: '/service/runtime/tasks/t-1', method: 'PUT', body: task },
      { path: '/service/runtime/tasks/t-1', method: 'DELETE', status: 204 },
    ])
    await updateTask('t-1', { assignee: null, dueDate: null, priority: 80 })
    await deleteTask('t-1', { cascadeHistory: true, deleteReason: '重复' })
    expect(calls[0].body).toEqual({ assignee: null, dueDate: null, priority: 80 })
    expect(calls[1].url).toBe('/service/runtime/tasks/t-1?cascadeHistory=true&deleteReason=%E9%87%8D%E5%A4%8D')
  })

  it('bulkUpdateTasks 批量提交任务 ID 与字段', async () => {
    const calls = mockFetch([{ path: '/service/runtime/tasks', method: 'PUT', body: pageOf([task]) }])
    await bulkUpdateTasks({ taskIds: ['t-1'], assignee: 'bob' })
    expect(calls[0].body).toEqual({ taskIds: ['t-1'], assignee: 'bob' })
  })

  it('taskAction 提交动作与变量', async () => {
    const calls = mockFetch([{ path: '/service/runtime/tasks/t-1', method: 'POST', body: {} }])
    await taskAction('t-1', { action: 'complete', variables: [{ name: 'approved', value: true }] })
    expect(calls[0].body).toEqual({ action: 'complete', variables: [{ name: 'approved', value: true }] })
  })

  it('taskAction 将冲突映射为 ApiError', async () => {
    mockFetch([{ path: '/service/runtime/tasks/t-1', method: 'POST', status: 409, body: { message: 'Task was already claimed', exception: 'claimed' } }])
    await expect(taskAction('t-1', { action: 'claim', assignee: 'bob' })).rejects.toMatchObject({
      status: 409,
      message: 'Task was already claimed',
      exception: 'claimed',
    })
  })

  it('getTaskForm 读取表单', async () => {
    const calls = mockFetch([
      { path: '/service/runtime/tasks/t-1/form', body: { formKey: 'f', formProperties: [{ id: 'approved', type: 'boolean' }] } },
    ])
    const form = await getTaskForm('t-1')
    expect(form.formKey).toBe('f')
    expect(calls[0].url).toBe('/service/runtime/tasks/t-1/form')
  })

  it('任务变量增删改查与原始数据', async () => {
    const calls = mockFetch([
      { path: '/service/runtime/tasks/t-1/variables/a%2Fb/data', contentType: 'application/octet-stream', text: 'raw' },
      { path: '/service/runtime/tasks/t-1/variables', method: 'GET', body: [{ name: 'a', value: 1 }] },
      { path: '/service/runtime/tasks/t-1/variables', method: 'POST', status: 201, body: { name: 'a', value: 1 } },
      { path: '/service/runtime/tasks/t-1/variables/a', method: 'PUT', status: 204 },
      { path: '/service/runtime/tasks/t-1/variables/a%2Fb', method: 'DELETE', status: 204 },
    ])
    expect(await listTaskVariables('t-1')).toEqual([{ name: 'a', value: 1 }])
    await createTaskVariable('t-1', { name: 'a', value: 1 })
    await updateTaskVariable('t-1', 'a', 2)
    await deleteTaskVariable('t-1', 'a/b')
    const raw = await getTaskVariableData('t-1', 'a/b')
    expect(raw).toBeInstanceOf(Blob)
    expect(calls[2].body).toEqual({ value: 2 })
    expect(calls[4].url).toBe('/service/runtime/tasks/t-1/variables/a%2Fb/data')
  })

  it('评论列表、新增与删除', async () => {
    const calls = mockFetch([
      { path: '/service/runtime/tasks/t-1/comments', method: 'GET', body: pageOf([{ id: 'c1', message: 'hi' }]) },
      { path: '/service/runtime/tasks/t-1/comments', method: 'POST', status: 201, body: { id: 'c2', message: 'ok' } },
      { path: '/service/runtime/tasks/t-1/comments/c2', method: 'DELETE', status: 204 },
    ])
    const page = await listTaskComments('t-1', 0, 100)
    await addTaskComment('t-1', 'ok', 'alice')
    await deleteTaskComment('t-1', 'c2')
    expect(page.total).toBe(1)
    expect(calls[0].url).toContain('start=0&size=100')
    expect(calls[1].body).toEqual({ message: 'ok', userId: 'alice' })
  })

  it('附件列表、multipart 上传、下载与删除', async () => {
    const calls = mockFetch([
      { path: '/service/runtime/tasks/t-1/attachments?', method: 'GET', body: pageOf([{ id: 'a1', name: 'x.txt' }]) },
      { path: '/service/runtime/tasks/t-1/attachments', method: 'POST', status: 201, body: { id: 'a2', name: 'y.txt' } },
      { path: '/service/runtime/tasks/t-1/attachments/a2/content', contentType: 'application/octet-stream', text: 'content' },
      { path: '/service/runtime/tasks/t-1/attachments/a2', method: 'DELETE', status: 204 },
    ])
    const page = await listTaskAttachments('t-1')
    const file = new File(['hello'], 'y.txt', { type: 'text/plain' })
    await uploadTaskAttachment('t-1', file, { name: 'Y', description: 'desc', type: 'text/plain' })
    const blob = await getTaskAttachmentContent('t-1', 'a2')
    await deleteTaskAttachment('t-1', 'a2')
    expect(page.data[0].name).toBe('x.txt')
    const form = calls[1].body as FormData
    expect(form).toBeInstanceOf(FormData)
    expect(form.get('name')).toBe('Y')
    expect(form.get('description')).toBe('desc')
    expect((form.get('file') as File).name).toBe('y.txt')
    expect(blob).toBeInstanceOf(Blob)
  })

  it('身份链接查询、新增与移除', async () => {
    const calls = mockFetch([
      { path: '/service/runtime/tasks/t-1/identitylinks', method: 'GET', body: [{ type: 'candidate', userId: 'alice' }] },
      { path: '/service/runtime/tasks/t-1/identitylinks', method: 'POST', status: 201 },
      { path: '/service/runtime/tasks/t-1/identitylinks/users/alice', method: 'DELETE', status: 204 },
    ])
    expect(await listTaskIdentityLinks('t-1')).toHaveLength(1)
    await addTaskCandidate('t-1', { userId: 'alice' })
    await deleteTaskIdentityLink('t-1', 'users', 'alice')
    expect(calls[1].body).toEqual({ type: 'candidate', userId: 'alice' })
    expect(calls[2].url).toBe('/service/runtime/tasks/t-1/identitylinks/users/alice')
  })

  it('子任务与事件', async () => {
    const calls = mockFetch([
      { path: '/service/runtime/tasks/t-1/subtasks', body: [task] },
      { path: '/service/runtime/tasks/t-1/events', body: [{ id: 1, action: 'create', timestamp: '2026-10-30T03:00:00Z' }] },
    ])
    expect(await listSubTasks('t-1')).toHaveLength(1)
    const events = await listTaskEvents('t-1', { action: 'create', userId: 'alice', start: 0, size: 10 })
    expect(events[0].action).toBe('create')
    expect(calls[1].url).toContain('action=create&userId=alice&start=0&size=10')
  })

  it('错误体映射为 ApiError', async () => {
    mockFetch([{ path: '/service/runtime/tasks/missing', status: 404, body: { message: 'Not found' } }])
    await expect(getTask('missing')).rejects.toBeInstanceOf(ApiError)
  })
})
