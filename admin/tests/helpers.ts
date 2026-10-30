// 测试辅助：fetch 路由桩、响应构造与异步刷新。
import { vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'

/** 模拟路由 */
export interface MockRoute {
  /** HTTP 方法，默认 GET */
  method?: string
  /** 路径前缀或正则 */
  path: string | RegExp
  /** 状态码 */
  status?: number
  /** JSON 响应体 */
  body?: unknown
  /** 纯文本响应体 */
  text?: string
  /** 内容类型 */
  contentType?: string
}

/** 已记录的请求 */
export interface RecordedRequest {
  url: string
  method: string
  body: unknown
}

// 判断请求是否匹配路由。
// @param route 模拟路由
// @param url 请求地址
// @param method 请求方法
// @return 是否匹配
function matches(route: MockRoute, url: string, method: string): boolean {
  const routeMethod = (route.method ?? 'GET').toUpperCase()
  if (routeMethod !== method) return false
  return typeof route.path === 'string' ? url.startsWith(route.path) : route.path.test(url)
}

// 安装 fetch 桩并返回请求记录。
// @param routes 模拟路由列表
// @return 请求记录数组
export function mockFetch(routes: MockRoute[]): RecordedRequest[] {
  const calls: RecordedRequest[] = []
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input)
    const method = (init?.method ?? 'GET').toUpperCase()
    let body: unknown = init?.body ?? null
    if (typeof body === 'string' && body.startsWith('{')) {
      try {
        body = JSON.parse(body)
      } catch {
        body = init?.body
      }
    }
    calls.push({ url, method, body })
    const route = routes.find((item) => matches(item, url, method))
    if (!route) {
      return new Response(JSON.stringify({ code: 'REST-0404', message: `未模拟的请求：${method} ${url}` }), {
        status: 404,
        headers: { 'Content-Type': 'application/json' },
      })
    }
    const status = route.status ?? 200
    if (route.text !== undefined) {
      return new Response(route.text, { status, headers: { 'Content-Type': route.contentType ?? 'text/plain' } })
    }
    if (status === 204) return new Response(null, { status })
    return new Response(JSON.stringify(route.body ?? {}), {
      status,
      headers: { 'Content-Type': route.contentType ?? 'application/json' },
    })
  })
  vi.stubGlobal('fetch', fetchMock)
  return calls
}

// 等待所有微任务与 Vue 更新完成。
// @return 无
export async function settle(): Promise<void> {
  await flushPromises()
  await new Promise((resolve) => setTimeout(resolve, 0))
  await flushPromises()
}

// 构造分页响应。
// @param data 数据
// @param total 总数
// @return 分页信封
export function pageOf<T>(data: T[], total = data.length): { data: T[]; total: number; start: number; size: number } {
  return { data, total, start: 0, size: total }
}
