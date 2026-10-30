import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/api/client'
import {
  createModel,
  deleteModel,
  deployModel,
  getModel,
  getModelSource,
  getModelSourceExtra,
  listModels,
  listProcessDefinitionsByKey,
  pickLatestDefinition,
  saveModelSource,
  saveModelSourceExtra,
  sourceFileName,
  updateModel,
} from '@/api/models'
import { mockFetch, pageOf } from '../helpers'

// 用 FileReader 读取 File 内容，兼容 jsdom 的 Blob 实现。
// @param file 文件
// @return 文本内容
function readFile(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result ?? ''))
    reader.onerror = () => reject(reader.error)
    reader.readAsText(file)
  })
}

const model = {
  id: 'm-1',
  name: '请假流程',
  key: 'leave',
  category: 'hr',
  version: 1,
  deploymentId: 'd-1',
  lastUpdateTime: '2026-10-30T10:00:00Z',
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('模型列表与元信息', () => {
  it('按筛选与分页参数查询列表', async () => {
    const calls = mockFetch([{ path: '/service/repository/models', body: pageOf([model], 1) }])
    const result = await listModels({
      nameLike: '请假',
      key: 'leave',
      category: 'hr',
      deployed: false,
      start: 50,
      size: 50,
      sort: 'lastUpdateTime',
      order: 'desc',
    })
    expect(result.total).toBe(1)
    const url = new URL(calls[0].url, 'http://localhost')
    expect(calls[0].method).toBe('GET')
    expect(url.pathname).toBe('/service/repository/models')
    expect(url.searchParams.get('nameLike')).toBe('请假')
    expect(url.searchParams.get('key')).toBe('leave')
    expect(url.searchParams.get('category')).toBe('hr')
    expect(url.searchParams.get('deployed')).toBe('false')
    expect(url.searchParams.get('start')).toBe('50')
    expect(url.searchParams.get('size')).toBe('50')
    expect(url.searchParams.get('sort')).toBe('lastUpdateTime')
    expect(url.searchParams.get('order')).toBe('desc')
  })

  it('获取单个模型并对 ID 编码', async () => {
    const calls = mockFetch([{ path: '/service/repository/models/', body: model }])
    const result = await getModel('m 1')
    expect(result.key).toBe('leave')
    expect(calls[0].url).toBe('/service/repository/models/m%201')
  })

  it('创建模型使用 POST 并提交字段', async () => {
    const calls = mockFetch([{ path: '/service/repository/models', method: 'POST', status: 201, body: model }])
    await createModel({ name: '请假流程', key: 'leave', category: 'hr', metaInfo: '{}' })
    expect(calls[0].method).toBe('POST')
    expect(calls[0].body).toEqual({ name: '请假流程', key: 'leave', category: 'hr', metaInfo: '{}' })
  })

  it('Key 冲突保留服务端错误原文', async () => {
    mockFetch([
      {
        path: '/service/repository/models',
        method: 'POST',
        status: 409,
        body: { code: 'REPO-0409', message: '模型 Key 已存在', exception: 'duplicate' },
      },
    ])
    const error = await createModel({ name: 'x', key: 'leave' }).catch((cause: unknown) => cause)
    expect(error).toBeInstanceOf(ApiError)
    expect((error as ApiError).status).toBe(409)
    expect((error as ApiError).message).toBe('模型 Key 已存在')
    expect((error as ApiError).code).toBe('REPO-0409')
  })

  it('更新模型使用 PUT 并支持清空字段', async () => {
    const calls = mockFetch([{ path: '/service/repository/models/m-1', method: 'PUT', body: model }])
    await updateModel('m-1', { name: '新名称', category: null, metaInfo: '' })
    expect(calls[0].method).toBe('PUT')
    expect(calls[0].body).toEqual({ name: '新名称', category: null, metaInfo: '' })
  })

  it('删除模型使用 DELETE', async () => {
    const calls = mockFetch([{ path: '/service/repository/models/m-1', method: 'DELETE', status: 204 }])
    await expect(deleteModel('m-1')).resolves.toBeUndefined()
    expect(calls[0].method).toBe('DELETE')
  })
})

describe('模型源码', () => {
  it('把 octet-stream 响应转为文本', async () => {
    const calls = mockFetch([
      {
        path: '/service/repository/models/m-1/source',
        text: '<bpmn:definitions/>',
        contentType: 'application/octet-stream',
      },
    ])
    await expect(getModelSource('m-1')).resolves.toBe('<bpmn:definitions/>')
    expect(calls[0].url).toBe('/service/repository/models/m-1/source')
  })

  it('读取扩展源码', async () => {
    mockFetch([
      {
        path: '/service/repository/models/m-1/source-extra',
        text: '{"extra":true}',
        contentType: 'application/octet-stream',
      },
    ])
    await expect(getModelSourceExtra('m-1')).resolves.toBe('{"extra":true}')
  })

  it('保存源码使用 multipart 的 file 字段与有意义的文件名', async () => {
    const calls = mockFetch([{ path: '/service/repository/models/m-1/source', method: 'PUT', status: 204 }])
    await saveModelSource('m-1', '<xml/>', 'leave.bpmn20.xml')
    expect(calls[0].method).toBe('PUT')
    const form = calls[0].body as FormData
    expect(form).toBeInstanceOf(FormData)
    const file = form.get('file') as File
    expect(file.name).toBe('leave.bpmn20.xml')
    expect(await readFile(file)).toBe('<xml/>')
  })

  it('保存扩展源码使用默认文件名', async () => {
    const calls = mockFetch([{ path: '/service/repository/models/m-1/source-extra', method: 'PUT', status: 204 }])
    await saveModelSourceExtra('m-1', '{}')
    const file = (calls[0].body as FormData).get('file') as File
    expect(file.name).toBe('source-extra.bpmn')
  })

  it('源码上传失败时抛出 ApiError', async () => {
    mockFetch([
      {
        path: '/service/repository/models/m-1/source',
        method: 'PUT',
        status: 400,
        body: { code: 'REST-0129', message: '缺少上传文件' },
      },
    ])
    await expect(saveModelSource('m-1', '', 'x.bpmn')).rejects.toMatchObject({ status: 400, message: '缺少上传文件' })
  })
})

describe('模型部署与定义版本', () => {
  it('部署模型并返回部署结果', async () => {
    const calls = mockFetch([
      {
        path: '/service/repository/models/m-1/deployment',
        method: 'POST',
        status: 201,
        body: { id: 'd-1', name: '请假流程', tenantId: '', deployTime: '2026-10-30T10:00:00Z' },
      },
    ])
    const result = await deployModel('m-1')
    expect(calls[0].method).toBe('POST')
    expect(result.id).toBe('d-1')
  })

  it('部署失败保留服务端错误信息', async () => {
    mockFetch([
      {
        path: '/service/repository/models/m-1/deployment',
        method: 'POST',
        status: 400,
        body: { code: 'REST-0151', message: '模型源码为空，无法部署', exception: 'empty source' },
      },
    ])
    const error = await deployModel('m-1').catch((cause: unknown) => cause)
    expect(error).toBeInstanceOf(ApiError)
    expect((error as ApiError).message).toBe('模型源码为空，无法部署')
    expect((error as ApiError).exception).toBe('empty source')
  })

  it('按 Key 查询流程定义并取最新版本', async () => {
    const calls = mockFetch([
      {
        path: '/service/repository/process-definitions',
        body: pageOf([
          { id: 'leave:1', key: 'leave', version: 1 },
          { id: 'leave:3', key: 'leave', version: 3 },
          { id: 'leave:2', key: 'leave', version: 2 },
        ]),
      },
    ])
    const page = await listProcessDefinitionsByKey('leave')
    expect(calls[0].url).toContain('key=leave')
    expect(calls[0].url).toContain('size=1000')
    expect(pickLatestDefinition(page.data)?.version).toBe(3)
    expect(pickLatestDefinition([])).toBeNull()
  })
})

describe('辅助函数', () => {
  it('源码文件名优先使用 Key，缺失时回退模型 ID', () => {
    expect(sourceFileName({ id: 'm-1', key: ' leave ' })).toBe('leave.bpmn20.xml')
    expect(sourceFileName({ id: 'm-1', key: '' })).toBe('model-m-1.bpmn20.xml')
  })
})
