import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  authNotice,
  checkService,
  clearAuthNotice,
  markAuthError,
  serviceDetail,
  serviceStatus,
  sidebarCollapsed,
  toggleSidebar,
  withGlobalLoading,
  changeTheme,
  darkMode,
  currentTheme,
  globalLoading,
} from '@/stores/ui'
import { ApiError } from '@/api/client'
import { mockFetch } from '../helpers'

afterEach(() => {
  vi.unstubAllGlobals()
  clearAuthNotice()
  serviceStatus.value = 'unknown'
  serviceDetail.value = ''
  localStorage.clear()
})

describe('认证提示', () => {
  it('401 与 403 分别标记', () => {
    markAuthError(new ApiError(401, '未认证'))
    expect(authNotice.value).toBe('unauthorized')
    markAuthError(new ApiError(403, '无权限'))
    expect(authNotice.value).toBe('forbidden')
    markAuthError(new Error('其他'))
    expect(authNotice.value).toBe('forbidden')
    clearAuthNotice()
    expect(authNotice.value).toBe('none')
  })
})

describe('侧栏折叠', () => {
  it('切换并持久化', () => {
    const initial = sidebarCollapsed.value
    toggleSidebar()
    expect(sidebarCollapsed.value).toBe(!initial)
    expect(localStorage.getItem('flowtide-admin-sidebar')).toBe(sidebarCollapsed.value ? '1' : '0')
    toggleSidebar()
    expect(sidebarCollapsed.value).toBe(initial)
  })
})

describe('健康检查', () => {
  it('healthz 与 readyz 都通过时为 ok', async () => {
    mockFetch([
      { path: '/healthz', body: { status: 'UP' } },
      { path: '/readyz', body: { status: 'UP' } },
    ])
    await expect(checkService()).resolves.toBe(true)
    expect(serviceStatus.value).toBe('ok')
  })

  it('任一失败时为 degraded 并记录状态码', async () => {
    mockFetch([
      { path: '/healthz', body: { status: 'UP' } },
      { path: '/readyz', status: 503, body: { status: 'DOWN' } },
    ])
    await expect(checkService()).resolves.toBe(false)
    expect(serviceStatus.value).toBe('degraded')
    expect(serviceDetail.value).toContain('readyz=503')
  })

  it('网络异常时为 degraded', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => {
      throw new Error('offline')
    }))
    await expect(checkService()).resolves.toBe(false)
    expect(serviceStatus.value).toBe('degraded')
    expect(serviceDetail.value).toBe('无法连接服务端')
  })
})

describe('全局加载与主题', () => {
  it('包装异步任务并维护计数', async () => {
    const result = await withGlobalLoading(async () => {
      expect(globalLoading.value).toBe(1)
      return 'ok'
    })
    expect(result).toBe('ok')
    expect(globalLoading.value).toBe(0)
  })

  it('失败时也释放计数', async () => {
    await expect(withGlobalLoading(async () => {
      throw new Error('x')
    })).rejects.toThrow('x')
    expect(globalLoading.value).toBe(0)
  })

  it('通过 store 切换主题', () => {
    changeTheme('dark')
    expect(currentTheme.value).toBe('dark')
    expect(darkMode.value).toBe(true)
    changeTheme('system')
    expect(darkMode.value).toBe(false)
  })
})
