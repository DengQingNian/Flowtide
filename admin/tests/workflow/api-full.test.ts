// 工作流 API 全函数冒烟：逐一调用导出的门面函数，断言路径与方法；并覆盖方法降级与分页信封。
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  activateWorkflowProcessInstance,
  addWorkflowCategory,
  addWorkflowDelegate,
  addWorkflowModel,
  addWorkflowRemind,
  addWorkflowStep,
  addWorkflowTemplate,
  completeWorkflowTask,
  deleteWorkflowCategories,
  deleteWorkflowDelegates,
  deleteWorkflowInstance,
  deleteWorkflowModels,
  deleteWorkflowRemind,
  deleteWorkflowSteps,
  deleteWorkflowTemplates,
  editWorkflowCategory,
  editWorkflowDelegate,
  editWorkflowModel,
  editWorkflowRemind,
  editWorkflowStep,
  editWorkflowTemplate,
  getProcessStatusByBusinessKey,
  getWorkflowHisModelJson,
  getWorkflowModelJson,
  getWorkflowRemind,
  handleWorkflowExceptions,
  listDoneTasks,
  listWorkflowDelegates,
  listWorkflowExceptions,
  listWorkflowInstances,
  listWorkflowModels,
  listWorkflowSteps,
  listWorkflowTemplates,
  suspendWorkflowProcessInstance,
} from '@/api/workflow'
import { mockFetch } from '../helpers'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('API 全函数冒烟', () => {
  it('GET 类函数命中对应门面路径', async () => {
    const calls = mockFetch([
      { path: /^\/workflow\//, body: { code: '200', success: true, data: [] } },
    ])
    await listWorkflowModels({ pageNum: 1 })
    await listWorkflowSteps({ modelId: 'm1' })
    await listWorkflowTemplates()
    await listDoneTasks({ identityId: 'u1' })
    await listWorkflowInstances({ pageNum: 1 })
    await listWorkflowDelegates({ ownerId: 'u1' })
    await listWorkflowExceptions({ processInstanceId: 'pi-1' })
    await getWorkflowRemind('r1')
    await getWorkflowModelJson({ customModelId: 'm1', dataObtainType: 0 })
    await getWorkflowHisModelJson({ processInstanceId: 'pi-1' })
    const urls = calls.map((call) => call.url)
    expect(urls).toContain('/workflow/model/list?pageNum=1')
    expect(urls).toContain('/workflow/step/list?modelId=m1')
    expect(urls).toContain('/workflow/template/list')
    expect(urls).toContain('/workflow/process/task/all/doneList?identityId=u1')
    expect(urls).toContain('/workflow/process/instance/all/list?pageNum=1')
    expect(urls).toContain('/workflow/delegate/list?ownerId=u1')
    expect(urls).toContain('/workflow/exception/list?processInstanceId=pi-1')
    expect(urls).toContain('/workflow/remind/getWorkflowRemind/r1')
    expect(urls.some((url) => url.startsWith('/workflow/process/getModelJsonByType/'))).toBe(true)
    expect(urls.some((url) => url.startsWith('/workflow/process/getHisModelJson/'))).toBe(true)
  })

  it('POST/DELETE 类写操作命中对应路径并携带请求体', async () => {
    const calls = mockFetch([
      { path: /^\/workflow\//, method: 'POST', body: { code: '200', success: true, data: null } },
      { path: /^\/workflow\//, method: 'DELETE', body: { code: '200', success: true, data: null } },
    ])
    await addWorkflowCategory({ categoryName: 'a', categoryCode: 'a' })
    await editWorkflowCategory({ id: 'c1', categoryName: 'a', categoryCode: 'a' })
    await deleteWorkflowCategories(['c1', 'c2'])
    await addWorkflowModel({ modelName: 'm', modelCode: 'm' })
    await editWorkflowModel({ id: 'm1', modelName: 'm', modelCode: 'm' })
    await deleteWorkflowModels(['m1'])
    await addWorkflowStep({ modelId: 'm1', stepName: 's', stepCode: 's' })
    await editWorkflowStep({ id: 's1', modelId: 'm1', stepName: 's', stepCode: 's' })
    await deleteWorkflowSteps(['s1'])
    await addWorkflowTemplate({ templateName: 't', templateCode: 't' })
    await editWorkflowTemplate({ id: 't1', templateName: 't', templateCode: 't' })
    await deleteWorkflowTemplates(['t1'])
    await completeWorkflowTask({ taskId: 't1', userCode: 'u1', message: 'ok' })
    await getProcessStatusByBusinessKey('B1')
    await suspendWorkflowProcessInstance('pi-1')
    await activateWorkflowProcessInstance('pi-2')
    await deleteWorkflowInstance('pi-3')
    await addWorkflowDelegate({ ownerId: 'u1', trusteeId: 'u2' })
    await editWorkflowDelegate({ id: 'd1', ownerId: 'u1', trusteeId: 'u2' })
    await deleteWorkflowDelegates(['d1'])
    await addWorkflowRemind({ systemSymbol: 'crm' })
    await editWorkflowRemind({ id: 'r1', systemSymbol: 'crm' })
    await deleteWorkflowRemind('r1')
    await handleWorkflowExceptions(['e1', 'e2'])
    const byUrl = new Map(calls.map((call) => [`${call.method} ${call.url}`, call.body]))
    expect(byUrl.get('POST /workflow/category/add')).toMatchObject({ categoryName: 'a' })
    expect(byUrl.get('DELETE /workflow/category/delete/c1,c2')).toBeNull()
    expect(byUrl.get('POST /workflow/model/add')).toMatchObject({ modelName: 'm' })
    expect(byUrl.get('DELETE /workflow/model/delete/m1')).toBeNull()
    expect(byUrl.get('POST /workflow/step/add')).toMatchObject({ modelId: 'm1' })
    expect(byUrl.get('DELETE /workflow/step/delete/s1')).toBeNull()
    expect(byUrl.get('POST /workflow/template/add')).toMatchObject({ templateName: 't' })
    expect(byUrl.get('DELETE /workflow/template/delete/t1')).toBeNull()
    expect(byUrl.get('POST /workflow/process/complete/task')).toMatchObject({ taskId: 't1', message: 'ok' })
    expect(byUrl.get('POST /workflow/process/public/processes/id?businessKey=B1')).toMatchObject({ businessKey: 'B1', businessId: 'B1' })
    expect(byUrl.get('POST /workflow/process/suspend/processInstance/pi-1')).toBeNull()
    expect(byUrl.get('POST /workflow/process/activate/processInstance/pi-2')).toBeNull()
    expect(byUrl.get('DELETE /workflow/process/instance/all/delete/pi-3')).toBeNull()
    expect(byUrl.get('POST /workflow/delegate/add')).toMatchObject({ ownerId: 'u1', trusteeId: 'u2' })
    expect(byUrl.get('DELETE /workflow/delegate/delete/d1')).toBeNull()
    expect(byUrl.get('POST /workflow/remind/add')).toMatchObject({ systemSymbol: 'crm' })
    expect(byUrl.get('DELETE /workflow/remind/delete/r1')).toBeNull()
    expect(byUrl.get('DELETE /workflow/exception/delete/e1,e2')).toBeNull()
  })

  it('网络错误折叠为失败结果而不抛错', async () => {
    mockFetch([])
    const result = await listWorkflowModels()
    expect(result.success).toBe(false)
    expect(result.notImplemented).toBe(false)
    expect(result.errorMessage).toContain('未模拟的请求')
  })
})
