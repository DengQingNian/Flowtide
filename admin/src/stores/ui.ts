// 全局 UI 状态：侧栏、服务健康、认证提示与全局加载计数。
import { computed, ref } from 'vue'
import { isDark, themeMode, setThemeMode } from '@/lib/theme'
import type { ThemeMode } from '@/lib/theme'
import { ApiError, onAuthError } from '@/api/client'

const sidebarKey = 'flowtide-admin-sidebar'

// 读取侧栏折叠状态。
// @return 是否折叠
function readSidebar(): boolean {
  if (typeof window === 'undefined') return false
  try {
    return window.localStorage.getItem(sidebarKey) === '1'
  } catch {
    return false
  }
}

/** 侧栏是否折叠 */
export const sidebarCollapsed = ref(readSidebar())

// 切换侧栏折叠状态并持久化。
// @return 无
export function toggleSidebar(): void {
  sidebarCollapsed.value = !sidebarCollapsed.value
  if (typeof window !== 'undefined') {
    try {
      window.localStorage.setItem(sidebarKey, sidebarCollapsed.value ? '1' : '0')
    } catch {
      // 存储不可用时仅保留内存状态
    }
  }
}

/** 认证提示类型 */
export type AuthNoticeKind = 'none' | 'unauthorized' | 'forbidden'

/** 认证提示状态 */
export const authNotice = ref<AuthNoticeKind>('none')

// 全局监听请求层的 401/403，确保即使页面自行吞掉错误也能给出提示。
onAuthError((status) => {
  authNotice.value = status === 401 ? 'unauthorized' : 'forbidden'
})

// 根据错误标记认证提示。
// @param error 请求错误
// @return 无
export function markAuthError(error: unknown): void {
  if (error instanceof ApiError && error.status === 401) authNotice.value = 'unauthorized'
  else if (error instanceof ApiError && error.status === 403) authNotice.value = 'forbidden'
}

// 清除认证提示。
// @return 无
export function clearAuthNotice(): void {
  authNotice.value = 'none'
}

/** 服务健康状态 */
export type ServiceStatus = 'unknown' | 'ok' | 'degraded'

/** 服务健康状态 */
export const serviceStatus = ref<ServiceStatus>('unknown')
/** 健康检查说明 */
export const serviceDetail = ref('')

// 检查 /healthz 与 /readyz，更新全局横幅。
// @return 是否全部健康
export async function checkService(): Promise<boolean> {
  try {
    const [health, ready] = await Promise.all([
      fetch('/healthz', { credentials: 'same-origin' }),
      fetch('/readyz', { credentials: 'same-origin' }),
    ])
    if (health.ok && ready.ok) {
      serviceStatus.value = 'ok'
      serviceDetail.value = ''
      return true
    }
    serviceStatus.value = 'degraded'
    serviceDetail.value = `healthz=${health.status}，readyz=${ready.status}`
    return false
  } catch {
    serviceStatus.value = 'degraded'
    serviceDetail.value = '无法连接服务端'
    return false
  }
}

/** 全局加载计数 */
export const globalLoading = ref(0)

// 包装异步任务并维护全局加载计数。
// @param task 异步任务
// @return 任务结果
export async function withGlobalLoading<T>(task: () => Promise<T>): Promise<T> {
  globalLoading.value += 1
  try {
    return await task()
  } finally {
    globalLoading.value -= 1
  }
}

/** 当前主题模式 */
export const currentTheme = computed(() => themeMode.value)

// 设置主题模式。
// @param mode 主题模式
// @return 无
export function changeTheme(mode: ThemeMode): void {
  setThemeMode(mode)
}

/** 是否深色 */
export const darkMode = computed(() => isDark.value)