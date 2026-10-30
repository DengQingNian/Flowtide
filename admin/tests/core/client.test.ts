import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  ApiError,
  buildQuery,
  deleteJSON,
  errorMessage,
  getBlob,
  getJSON,
  getText,
  isAuthError,
  onAuthError,
  patchJSON,
  postForm,
  postJSON,
  putForm,
  putJSON,
  request,
} from '@/api/client'
import { mockFetch } from '../helpers'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('buildQuery', () => {
  it('编码普通值与数组，跳过空值', () => {
    expect(buildQuery()).toBe('')
    expect(buildQuery({ a: 1, b: 'x y', c: undefined, d: null })).toBe('?a=1&b=x%20y')
    expect(buildQuery({ ids: ['a', 'b'] })).toBe('?ids=a%2Cb')
    expect(buildQuery({ filter: { a: 1 } })).toBe('?filter=%7B%22a%22%3A1%7D')
    expect(buildQuery({ empty: [], flag: false, zero: 0 })).toBe('?flag=false&zero=0')
  })
})

describe('request', () => {
  it('解析 JSON 并带上查询参数', async () => {
    const calls = mockFetch([{ path: '/service/things', body: { data: [1] } }])
    const result = await getJSON('/service/things', { start: 0 })
    expect(result).toEqual({ data: [1] })
    expect(calls[0].url).toBe('/service/things?start=0')
  })

  it('204 返回 undefined', async () => {
    mockFetch([{ path: '/service/things/1', method: 'DELETE', status: 204 }])
    await expect(deleteJSON('/service/things/1')).resolves.toBeUndefined()
  })

  it('解析文本与二进制', async () => {
    mockFetch([
      { path: '/service/text', text: 'hello', contentType: 'text/plain' },
      { path: '/service/blob', text: 'raw', contentType: 'application/octet-stream' },
    ])
    await expect(getText('/service/text')).resolves.toBe('hello')
    const blob = await getBlob('/service/blob')
    expect(blob).toBeInstanceOf(Blob)
  })

  it('把错误体映射为 ApiError 并保留 code/exception', async () => {
    mockFetch([{ path: '/service/fail', status: 409, body: { code: 'ENGINE-1', message: '冲突', exception: 'stack' } }])
    const error = await request('/service/fail').catch((cause: unknown) => cause)
    expect(error).toBeInstanceOf(ApiError)
    expect((error as ApiError).status).toBe(409)
    expect((error as ApiError).code).toBe('ENGINE-1')
    expect((error as ApiError).message).toBe('冲突')
    expect((error as ApiError).exception).toBe('stack')
  })

  it('非 JSON 错误回退为状态码提示', async () => {
    mockFetch([{ path: '/service/fail', status: 500, text: 'boom', contentType: 'text/plain' }])
    await expect(getJSON('/service/fail')).rejects.toMatchObject({ status: 500, message: '请求失败（HTTP 500）' })
  })

  it('空错误体也给出可读提示', async () => {
    mockFetch([{ path: '/service/fail', status: 502, text: '' }])
    await expect(getJSON('/service/fail')).rejects.toMatchObject({ message: '请求失败（HTTP 502）' })
  })

  it('网络错误包装为 ApiError', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => {
      throw new Error('offline')
    }))
    await expect(getJSON('/service/x')).rejects.toMatchObject({ status: 0, code: 'NETWORK' })
  })

  it('超时中止请求', async () => {
    vi.useFakeTimers()
    vi.stubGlobal('fetch', vi.fn((_url: string, init: RequestInit) => new Promise((_resolve, reject) => {
      init.signal?.addEventListener('abort', () => reject(Object.assign(new Error('aborted'), { name: 'AbortError' })))
    })))
    const promise = request('/service/slow', { timeoutMs: 10 }).catch((cause: unknown) => cause)
    await vi.advanceTimersByTimeAsync(20)
    const error = await promise
    expect(error).toMatchObject({ status: 0, code: 'TIMEOUT' })
    vi.useRealTimers()
  })

  it('外部取消返回 ABORTED', async () => {
    vi.stubGlobal('fetch', vi.fn((_url: string, init: RequestInit) => new Promise((_resolve, reject) => {
      init.signal?.addEventListener('abort', () => reject(Object.assign(new Error('aborted'), { name: 'AbortError' })))
    })))
    const controller = new AbortController()
    const promise = request('/service/slow', { signal: controller.signal }).catch((cause: unknown) => cause)
    controller.abort()
    await expect(promise).resolves.toMatchObject({ code: 'ABORTED' })
  })
})

describe('请求便捷函数', () => {
  it('发送 JSON 与表单请求', async () => {
    const calls = mockFetch([
      { path: '/service/post', method: 'POST', body: { ok: true } },
      { path: '/service/put', method: 'PUT', body: { ok: true } },
      { path: '/service/patch', method: 'PATCH', body: { ok: true } },
      { path: '/service/form', method: 'POST', body: { ok: true } },
      { path: '/service/form-put', method: 'PUT', body: { ok: true } },
    ])
    await postJSON('/service/post', { a: 1 })
    await putJSON('/service/put', { b: 2 })
    await patchJSON('/service/patch', { c: 3 })
    const form = new FormData()
    form.append('file', new Blob(['x']), 'a.bpmn')
    await postForm('/service/form', form)
    await putForm('/service/form-put', form)
    expect(calls.map((call) => call.method)).toEqual(['POST', 'PUT', 'PATCH', 'POST', 'PUT'])
    expect(calls[0].body).toEqual({ a: 1 })
    expect(calls[3].url).toBe('/service/form')
  })
})

describe('错误工具', () => {
  it('提取错误信息并判断认证错误', () => {
    expect(errorMessage(new ApiError(401, '未认证'))).toBe('未认证')
    expect(errorMessage(new Error('x'))).toBe('x')
    expect(errorMessage('y')).toBe('y')
    expect(isAuthError(new ApiError(401, '未认证'))).toBe(true)
    expect(isAuthError(new ApiError(403, '无权限'))).toBe(true)
    expect(isAuthError(new ApiError(500, '失败'))).toBe(false)
    expect(isAuthError(null)).toBe(false)
  })
})

describe('认证错误订阅', () => {
  it('401/403 通知监听器并可取消订阅', async () => {
    mockFetch([
      { path: '/service/auth', status: 401, body: { message: '未认证' } },
      { path: '/service/forbidden', status: 403, body: { message: '无权限' } },
    ])
    const listener = vi.fn()
    const unsubscribe = onAuthError(listener)
    await expect(getJSON('/service/auth')).rejects.toMatchObject({ status: 401 })
    expect(listener).toHaveBeenCalledWith(401, expect.any(ApiError))
    await expect(getJSON('/service/forbidden')).rejects.toMatchObject({ status: 403 })
    expect(listener).toHaveBeenCalledWith(403, expect.any(ApiError))
    unsubscribe()
    await expect(getJSON('/service/auth')).rejects.toMatchObject({ status: 401 })
    expect(listener).toHaveBeenCalledTimes(2)
  })

  it('普通错误不触发认证监听', async () => {
    mockFetch([{ path: '/service/fail', status: 500, body: { message: '失败' } }])
    const listener = vi.fn()
    const unsubscribe = onAuthError(listener)
    await expect(getJSON('/service/fail')).rejects.toMatchObject({ status: 500 })
    expect(listener).not.toHaveBeenCalled()
    unsubscribe()
  })
})
