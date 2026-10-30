import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  addDefinitionIdentityLink,
  classifyResource,
  createDeployment,
  definitionIdentityLinkPath,
  definitionResourceDataPath,
  deleteDefinitionIdentityLink,
  deleteDeployment,
  deploymentResourceDataPath,
  deploymentResourcePath,
  getDefinitionResourceBlob,
  getDeployment,
  getDeploymentResourceBlob,
  getDeploymentResourceMetadata,
  getDeploymentResourceText,
  getProcessDefinition,
  getProcessDefinitionImage,
  getProcessDefinitionStartForm,
  getProcessDefinitionXML,
  listAllProcessDefinitions,
  listDefinitionIdentityLinks,
  listDeploymentResources,
  listDeployments,
  listProcessDefinitions,
  startProcessInstance,
  updateProcessDefinition,
} from '@/api/repository'
import { mockFetch, pageOf } from '../helpers'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('部署接口', () => {
  it('读取部署列表并只发送 start/size/tenantId', async () => {
    const calls = mockFetch([{ path: '/service/repository/deployments', body: pageOf([{ id: 'd1', name: '部署' }]) }])
    const page = await listDeployments({ start: 10, size: 20, tenantId: 't1' })
    expect(page.data[0].id).toBe('d1')
    expect(calls[0].method).toBe('GET')
    expect(calls[0].url).toBe('/service/repository/deployments?start=10&size=20&tenantId=t1')
    await listDeployments()
    expect(calls[1].url).toBe('/service/repository/deployments?start=0&size=50')
  })

  it('读取部署详情', async () => {
    const calls = mockFetch([{ path: '/service/repository/deployments/d1', body: { id: 'd1', name: '部署' } }])
    await expect(getDeployment('d1')).resolves.toMatchObject({ id: 'd1' })
    expect(calls[0].url).toBe('/service/repository/deployments/d1')
  })

  it('创建部署时提交 multipart 文件与名称租户', async () => {
    const calls = mockFetch([{ path: '/service/repository/deployments?enableDuplicateFiltering=true', method: 'POST', body: { id: 'd2' } }])
    const file = new File(['<definitions/>'], 'leave.bpmn20.xml', { type: 'application/xml' })
    const deployment = await createDeployment([file], { name: '请假部署', tenantId: 't1', enableDuplicateFiltering: true })
    expect(deployment.id).toBe('d2')
    expect(calls[0].method).toBe('POST')
    expect(calls[0].url).toBe('/service/repository/deployments?enableDuplicateFiltering=true')
    const form = calls[0].body as FormData
    expect(form.get('deploymentName')).toBe('请假部署')
    expect(form.get('tenantId')).toBe('t1')
    expect((form.get('file') as File).name).toBe('leave.bpmn20.xml')
  })

  it('创建部署不带可选字段与重复过滤', async () => {
    const calls = mockFetch([{ path: '/service/repository/deployments', method: 'POST', body: { id: 'd3' } }])
    await createDeployment([new File(['x'], 'a.bpmn')])
    const form = calls[0].body as FormData
    expect(form.get('deploymentName')).toBeNull()
    expect(form.get('tenantId')).toBeNull()
    expect(calls[0].url).toBe('/service/repository/deployments')
  })

  it('删除部署支持级联开关', async () => {
    const calls = mockFetch([
      { path: '/service/repository/deployments/d1', method: 'DELETE', status: 204 },
      { path: '/service/repository/deployments/d2', method: 'DELETE', status: 204 },
    ])
    await deleteDeployment('d1')
    await deleteDeployment('d2', false)
    expect(calls[0].url).toBe('/service/repository/deployments/d1?cascade=true')
    expect(calls[1].url).toBe('/service/repository/deployments/d2')
  })

  it('读取资源元数据、文本与二进制，并编码资源名', async () => {
    const calls = mockFetch([
      { path: '/service/repository/deployments/d1/resources/', body: { id: 'forms/a.bpmn20.xml', mediaType: 'text/xml' } },
      { path: '/service/repository/deployments/d1/resources', body: [{ id: 'a.bpmn20.xml' }] },
      { path: '/service/repository/deployments/d1/resourcedata/a.bpmn20.xml', text: '<definitions/>', contentType: 'text/xml' },
      { path: '/service/repository/deployments/d1/resourcedata/logo.png', text: 'png-bytes', contentType: 'image/png' },
    ])
    const resources = await listDeploymentResources('d1')
    expect(resources).toHaveLength(1)
    const metadata = await getDeploymentResourceMetadata('d1', 'forms/a.bpmn20.xml')
    expect(metadata.id).toBe('forms/a.bpmn20.xml')
    expect(calls[0].url).toBe('/service/repository/deployments/d1/resources')
    expect(calls[1].url).toBe('/service/repository/deployments/d1/resources/forms%2Fa.bpmn20.xml')
    await expect(getDeploymentResourceText('d1', 'a.bpmn20.xml')).resolves.toBe('<definitions/>')
    const blob = await getDeploymentResourceBlob('d1', 'logo.png')
    expect(blob).toBeInstanceOf(Blob)
  })

  it('生成编码后的资源路径', () => {
    expect(deploymentResourcePath('d1', 'a b/c.xml')).toBe('/service/repository/deployments/d1/resources/a%20b%2Fc.xml')
    expect(deploymentResourceDataPath('d1', 'a b/c.xml')).toBe('/service/repository/deployments/d1/resourcedata/a%20b%2Fc.xml')
  })
})

describe('流程定义接口', () => {
  it('读取定义列表并仅发送服务端支持的过滤字段', async () => {
    const calls = mockFetch([{ path: '/service/repository/process-definitions', body: pageOf([{ id: 'def1', key: 'leave', version: 1 }]) }])
    const page = await listProcessDefinitions({ key: 'leave', version: 2, startableByUser: 'alice', tenantId: 't1', start: 5, size: 10 })
    expect(page.data[0].key).toBe('leave')
    expect(calls[0].url).toBe('/service/repository/process-definitions?key=leave&version=2&startableByUser=alice&tenantId=t1&start=5&size=10')
  })

  it('读取全部定义用于本地分组', async () => {
    const calls = mockFetch([{ path: '/service/repository/process-definitions', body: pageOf([{ id: 'def1', key: 'leave', version: 1 }]) }])
    await expect(listAllProcessDefinitions('t1')).resolves.toHaveLength(1)
    expect(calls[0].url).toBe('/service/repository/process-definitions?tenantId=t1&start=0&size=1000')
  })

  it('全部定义空响应归一为空数组', async () => {
    mockFetch([{ path: '/service/repository/process-definitions', body: {} }])
    await expect(listAllProcessDefinitions()).resolves.toEqual([])
  })

  it('读取定义详情、XML、图片与原始资源', async () => {
    const calls = mockFetch([
      { path: '/service/repository/process-definitions/def1/xml', text: '<xml/>', contentType: 'application/xml' },
      { path: '/service/repository/process-definitions/def1/image', text: 'png', contentType: 'image/png' },
      { path: '/service/repository/process-definitions/def1/resourcedata', text: 'raw', contentType: 'application/octet-stream' },
      { path: '/service/repository/process-definitions/def1', body: { id: 'def1', key: 'leave', version: 1 } },
    ])
    await expect(getProcessDefinition('def1')).resolves.toMatchObject({ key: 'leave' })
    await expect(getProcessDefinitionXML('def1')).resolves.toBe('<xml/>')
    await expect(getProcessDefinitionImage('def1')).resolves.toBeInstanceOf(Blob)
    await expect(getDefinitionResourceBlob('def1')).resolves.toBeInstanceOf(Blob)
    expect(calls[0].url).toBe('/service/repository/process-definitions/def1')
    expect(definitionResourceDataPath('def1')).toBe('/service/repository/process-definitions/def1/resourcedata')
  })

  it('读取启动表单并归一空响应', async () => {
    mockFetch([
      { path: '/service/repository/process-definitions/def1/start-form', body: { formKey: 'leaveForm', formProperties: [{ id: 'days', type: 'long' }] } },
    ])
    await expect(getProcessDefinitionStartForm('def1')).resolves.toMatchObject({
      formKey: 'leaveForm',
      formProperties: [{ id: 'days' }],
    })
    vi.unstubAllGlobals()
    mockFetch([{ path: '/service/repository/process-definitions/def1/start-form', body: {} }])
    await expect(getProcessDefinitionStartForm('def1')).resolves.toEqual({ formKey: undefined, formProperties: [] })
  })

  it('身份链接列表、添加与删除', async () => {
    const calls = mockFetch([
      { path: '/service/repository/process-definitions/def1/identitylinks/users/alice', method: 'DELETE', status: 204 },
      { path: '/service/repository/process-definitions/def1/identitylinks', method: 'POST', body: { id: 'il1', userId: 'alice' } },
      { path: '/service/repository/process-definitions/def1/identitylinks', body: [{ id: 'il1', userId: 'alice' }] },
    ])
    await expect(listDefinitionIdentityLinks('def1')).resolves.toHaveLength(1)
    await expect(addDefinitionIdentityLink('def1', 'alice')).resolves.toMatchObject({ userId: 'alice' })
    await deleteDefinitionIdentityLink('def1', 'users', 'alice')
    expect(calls[0].url).toBe('/service/repository/process-definitions/def1/identitylinks')
    expect(calls[1].method).toBe('POST')
    expect(calls[1].body).toEqual({ userId: 'alice' })
    expect(calls[2].url).toBe('/service/repository/process-definitions/def1/identitylinks/users/alice')
    expect(definitionIdentityLinkPath('def1', 'groups', 'g 1')).toBe('/service/repository/process-definitions/def1/identitylinks/groups/g%201')
  })

  it('添加组身份链接只提交 groupId', async () => {
    const calls = mockFetch([{ path: '/service/repository/process-definitions/def1/identitylinks', method: 'POST', body: { id: 'il2' } }])
    await addDefinitionIdentityLink('def1', undefined, 'g1')
    expect(calls[0].body).toEqual({ groupId: 'g1' })
  })

  it('挂起或激活定义发送 PUT 动作', async () => {
    const calls = mockFetch([{ path: '/service/repository/process-definitions/def1', method: 'PUT', status: 204 }])
    await updateProcessDefinition('def1', { action: 'suspend', includeProcessInstances: true })
    expect(calls[0].method).toBe('PUT')
    expect(calls[0].body).toEqual({ action: 'suspend', includeProcessInstances: true })
  })

  it('启动实例提交定义与变量', async () => {
    const calls = mockFetch([{ path: '/service/runtime/process-instances', method: 'POST', body: { id: 'pi-1' } }])
    const instance = await startProcessInstance({ processDefinitionId: 'def1', businessKey: 'B-1', variables: { days: 2 } })
    expect(instance.id).toBe('pi-1')
    expect(calls[0].body).toEqual({ processDefinitionId: 'def1', businessKey: 'B-1', variables: { days: 2 } })
  })
})

describe('资源预览类型', () => {
  it('按媒体类型与扩展名分类', () => {
    expect(classifyResource('a.bpmn20.xml')).toBe('xml')
    expect(classifyResource('diagram.svg', 'image/svg+xml')).toBe('image')
    expect(classifyResource('logo.png')).toBe('image')
    expect(classifyResource('form.form')).toBe('json')
    expect(classifyResource('data.json', 'application/json')).toBe('json')
    expect(classifyResource('readme.txt')).toBe('text')
    expect(classifyResource('payload.bin', 'application/octet-stream')).toBe('binary')
  })
})

describe('错误映射', () => {
  it('所有接口都保留服务端错误码与原文', async () => {
    const routes = [
      { path: /^\/service\//, method: 'GET', status: 500, body: { code: 'ENGINE-1', message: '仓库故障', exception: 'stack' } },
      { path: /^\/service\//, method: 'POST', status: 500, body: { code: 'ENGINE-1', message: '仓库故障', exception: 'stack' } },
      { path: /^\/service\//, method: 'PUT', status: 500, body: { code: 'ENGINE-1', message: '仓库故障', exception: 'stack' } },
      { path: /^\/service\//, method: 'DELETE', status: 500, body: { code: 'ENGINE-1', message: '仓库故障', exception: 'stack' } },
    ]
    mockFetch(routes)
    const calls: Array<() => Promise<unknown>> = [
      () => listDeployments(),
      () => getDeployment('d1'),
      () => createDeployment([new File(['x'], 'a.bpmn')]),
      () => deleteDeployment('d1'),
      () => listDeploymentResources('d1'),
      () => getDeploymentResourceMetadata('d1', 'a.bpmn'),
      () => getDeploymentResourceText('d1', 'a.bpmn'),
      () => getDeploymentResourceBlob('d1', 'a.bpmn'),
      () => listProcessDefinitions(),
      () => listAllProcessDefinitions(),
      () => getProcessDefinition('def1'),
      () => getProcessDefinitionXML('def1'),
      () => getProcessDefinitionImage('def1'),
      () => getProcessDefinitionStartForm('def1'),
      () => listDefinitionIdentityLinks('def1'),
      () => addDefinitionIdentityLink('def1', 'alice'),
      () => deleteDefinitionIdentityLink('def1', 'users', 'alice'),
      () => updateProcessDefinition('def1', { action: 'suspend' }),
      () => startProcessInstance({ processDefinitionId: 'def1' }),
      () => getDefinitionResourceBlob('def1'),
    ]
    for (const call of calls) {
      await expect(call()).rejects.toMatchObject({ status: 500, code: 'ENGINE-1', message: '仓库故障' })
    }
  })
})
