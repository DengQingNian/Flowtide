// 管理台 REST 客户端：同源请求、统一错误体解析与超时控制。
// 不注入认证头、租户头，是否携带凭据由浏览器或前置代理决定。

// ApiError 表示一次失败的 REST 调用。
export class ApiError extends Error {
  /** HTTP 状态码，网络错误或超时为 0 */
  readonly status: number
  /** 服务端业务错误码，如 REST-0404 */
  readonly code: string
  /** 服务端异常信息，通常为堆栈或技术细节 */
  readonly exception: string

  // 构造 API 错误。
  // @param status HTTP 状态码
  // @param message 可读错误信息
  // @param code 业务错误码
  // @param exception 服务端异常信息
  // @return 无
  constructor(status: number, message: string, code = '', exception = '') {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.exception = exception
  }
}

/** 认证错误监听器 */
export type AuthErrorListener = (status: 401 | 403, error: ApiError) => void

const authErrorListeners = new Set<AuthErrorListener>()

// 订阅认证错误；返回取消订阅函数。
// @param listener 监听器
// @return 取消订阅函数
export function onAuthError(listener: AuthErrorListener): () => void {
  authErrorListeners.add(listener)
  return () => authErrorListeners.delete(listener)
}

// 通知认证错误监听器；监听器异常不影响请求结果。
// @param error API 错误
// @return 无
function notifyAuthError(error: ApiError): void {
  if (error.status !== 401 && error.status !== 403) return
  for (const listener of authErrorListeners) {
    try {
      listener(error.status, error)
    } catch {
      // 监听器异常不应覆盖原始请求错误
    }
  }
}

// 请求选项。
export interface RequestOptions {
  /** HTTP 方法 */
  method?: string
  /** 查询参数，自动跳过 undefined/null */
  query?: Record<string, unknown>
  /** 原始请求体 */
  body?: BodyInit | null
  /** JSON 请求体，自动序列化并设置 Content-Type */
  json?: unknown
  /** 额外请求头 */
  headers?: HeadersInit
  /** 外部取消信号 */
  signal?: AbortSignal
  /** 超时毫秒数，默认 20 秒 */
  timeoutMs?: number
}

// 分页响应信封。
export interface Page<T> {
  data: T[]
  total: number
  start: number
  size: number
  sort?: string
  order?: string
}

// 分页查询参数。
export interface PageQuery {
  start?: number
  size?: number
  sort?: string
  order?: 'asc' | 'desc' | string
  [key: string]: unknown
}

// buildQuery 将对象编码为查询字符串。
// @param query 查询参数
// @return 以 ? 开头的查询串，无参数时返回空串
export function buildQuery(query?: Record<string, unknown>): string {
  if (!query) return ''
  const parts: string[] = []
  for (const [key, value] of Object.entries(query)) {
    if (value === undefined || value === null) continue
    if (Array.isArray(value)) {
      if (value.length === 0) continue
      parts.push(`${encodeURIComponent(key)}=${encodeURIComponent(value.join(','))}`)
      continue
    }
    if (typeof value === 'object') {
      parts.push(`${encodeURIComponent(key)}=${encodeURIComponent(JSON.stringify(value))}`)
      continue
    }
    parts.push(`${encodeURIComponent(key)}=${encodeURIComponent(String(value))}`)
  }
  return parts.length > 0 ? `?${parts.join('&')}` : ''
}

// parseErrorBody 从响应中提取 Flowtide 错误体。
// @param response 失败响应
// @return 错误码、信息与异常
async function parseErrorBody(response: Response): Promise<{ code: string; message: string; exception: string }> {
  const fallback = `请求失败（HTTP ${response.status}）`
  try {
    const text = await response.text()
    if (!text) return { code: '', message: fallback, exception: '' }
    // ErrorMessage 兼容 workflow 门面 R 信封的大写字段名（api.R）
    const body = JSON.parse(text) as { code?: string; message?: string; ErrorMessage?: string; exception?: string }
    return {
      code: body.code ?? '',
      message: body.message || body.ErrorMessage || fallback,
      exception: body.exception ?? '',
    }
  } catch {
    return { code: '', message: fallback, exception: '' }
  }
}

// request 发送同源请求并按 Content-Type 解析响应。
// @param path /service 或 / 开头的路径
// @param options 请求选项
// @return 解析后的 JSON、文本或二进制
export async function request<T = unknown>(path: string, options: RequestOptions = {}): Promise<T> {
  const { method = 'GET', query, body, json, headers, signal, timeoutMs = 20000 } = options
  const url = `${path}${buildQuery(query)}`
  const controller = new AbortController()
  let timedOut = false
  const timer = setTimeout(() => {
    timedOut = true
    controller.abort()
  }, timeoutMs)
  // 外部取消信号与内部超时信号合并，任一触发都中止请求。
  const onAbort = () => controller.abort()
  if (signal) {
    if (signal.aborted) controller.abort()
    else signal.addEventListener('abort', onAbort, { once: true })
  }
  const finalHeaders = new Headers(headers)
  let finalBody = body
  if (json !== undefined) {
    finalBody = JSON.stringify(json)
    finalHeaders.set('Content-Type', 'application/json')
  }
  if (method !== 'GET' && method !== 'HEAD') finalHeaders.set('Accept', 'application/json, text/plain, */*')
  try {
    const response = await fetch(url, {
      method,
      headers: finalHeaders,
      body: finalBody,
      signal: controller.signal,
      credentials: 'same-origin',
    })
    if (!response.ok) {
      const parsed = await parseErrorBody(response)
      const apiError = new ApiError(response.status, parsed.message, parsed.code, parsed.exception)
      notifyAuthError(apiError)
      throw apiError
    }
    if (response.status === 204 || response.status === 205) return undefined as T
    const contentType = response.headers.get('Content-Type') ?? ''
    if (contentType.includes('application/json')) {
      const text = await response.text()
      return (text ? JSON.parse(text) : undefined) as T
    }
    if (contentType.startsWith('text/') || contentType.includes('xml') || contentType.includes('javascript')) {
      return (await response.text()) as T
    }
    return (await response.blob()) as T
  } catch (error) {
    if (error instanceof ApiError) throw error
    if (controller.signal.aborted) {
      if (timedOut) throw new ApiError(0, `请求超时（${Math.round(timeoutMs / 1000)} 秒），请检查服务状态`, 'TIMEOUT')
      throw new ApiError(0, '请求已取消', 'ABORTED')
    }
    const message = error instanceof Error ? error.message : String(error)
    throw new ApiError(0, `网络请求失败：${message}`, 'NETWORK')
  } finally {
    clearTimeout(timer)
    if (signal) signal.removeEventListener('abort', onAbort)
  }
}

// getJSON 发送 GET 请求并解析 JSON。
// @param path 请求路径
// @param query 查询参数
// @param signal 取消信号
// @return 解析后的 JSON
export function getJSON<T>(path: string, query?: Record<string, unknown>, signal?: AbortSignal): Promise<T> {
  return request<T>(path, { query, signal })
}

// postJSON 发送 POST JSON 请求。
// @param path 请求路径
// @param json 请求体
// @param query 查询参数
// @return 解析后的 JSON
export function postJSON<T>(path: string, json?: unknown, query?: Record<string, unknown>): Promise<T> {
  return request<T>(path, { method: 'POST', json, query })
}

// putJSON 发送 PUT JSON 请求。
// @param path 请求路径
// @param json 请求体
// @param query 查询参数
// @return 解析后的 JSON
export function putJSON<T>(path: string, json?: unknown, query?: Record<string, unknown>): Promise<T> {
  return request<T>(path, { method: 'PUT', json, query })
}

// patchJSON 发送 PATCH JSON 请求。
// @param path 请求路径
// @param json 请求体
// @return 解析后的 JSON
export function patchJSON<T>(path: string, json?: unknown): Promise<T> {
  return request<T>(path, { method: 'PATCH', json })
}

// deleteJSON 发送 DELETE 请求。
// @param path 请求路径
// @param query 查询参数
// @return 解析后的 JSON
export function deleteJSON<T = void>(path: string, query?: Record<string, unknown>): Promise<T> {
  return request<T>(path, { method: 'DELETE', query })
}

// getText 发送 GET 请求并返回纯文本。
// @param path 请求路径
// @param query 查询参数
// @param signal 取消信号
// @return 文本内容
export function getText(path: string, query?: Record<string, unknown>, signal?: AbortSignal): Promise<string> {
  return request<string>(path, { query, signal })
}

// getBlob 发送 GET 请求并返回二进制。
// @param path 请求路径
// @param query 查询参数
// @param signal 取消信号
// @return 二进制内容
export function getBlob(path: string, query?: Record<string, unknown>, signal?: AbortSignal): Promise<Blob> {
  return request<Blob>(path, { query, signal })
}

// postForm 发送 multipart 表单请求。
// @param path 请求路径
// @param form 表单数据
// @param signal 取消信号
// @return 解析后的 JSON
export function postForm<T>(path: string, form: FormData, signal?: AbortSignal): Promise<T> {
  return request<T>(path, { method: 'POST', body: form, signal, timeoutMs: 60000 })
}

// putForm 发送 multipart 表单 PUT 请求。
// @param path 请求路径
// @param form 表单数据
// @param signal 取消信号
// @return 解析后的 JSON
export function putForm<T>(path: string, form: FormData, signal?: AbortSignal): Promise<T> {
  return request<T>(path, { method: 'PUT', body: form, signal, timeoutMs: 60000 })
}

// errorMessage 提取任意错误的可读信息。
// @param error 错误对象
// @return 可读信息
export function errorMessage(error: unknown): string {
  if (error instanceof ApiError) return error.message
  if (error instanceof Error) return error.message
  return String(error)
}

// isAuthError 判断是否为未认证或未授权错误。
// @param error 错误对象
// @return 是否为 401/403
export function isAuthError(error: unknown): boolean {
  return error instanceof ApiError && (error.status === 401 || error.status === 403)
}