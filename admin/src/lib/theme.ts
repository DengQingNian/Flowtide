// 主题状态：浅色、深色与跟随系统，持久化到 localStorage。
import { computed, ref, watch } from 'vue'

/** 主题模式 */
export type ThemeMode = 'light' | 'dark' | 'system'

const storageKey = 'flowtide-admin-theme'

// 读取系统深色偏好。
// @return 是否深色
function systemPrefersDark(): boolean {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return false
  return window.matchMedia('(prefers-color-scheme: dark)').matches
}

// 读取本地保存的主题模式。
// @return 合法主题模式，默认 system
function readThemeMode(): ThemeMode {
  if (typeof window === 'undefined') return 'system'
  let stored: string | null = null
  try {
    stored = window.localStorage.getItem(storageKey)
  } catch {
    stored = null
  }
  return stored === 'light' || stored === 'dark' || stored === 'system' ? stored : 'system'
}

const systemDark = ref(systemPrefersDark())

// 监听系统主题变化，仅在跟随系统时生效。
// @return 无
function watchSystemTheme(): void {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return
  const media = window.matchMedia('(prefers-color-scheme: dark)')
  if (typeof media.addEventListener === 'function') {
    media.addEventListener('change', (event) => {
      systemDark.value = event.matches
    })
  }
}

/** 当前主题模式 */
export const themeMode = ref<ThemeMode>(readThemeMode())
/** 当前是否深色 */
export const isDark = computed(() => (themeMode.value === 'system' ? systemDark.value : themeMode.value === 'dark'))

// 把主题写入根元素与本地存储。
// @param dark 是否深色
// @param mode 主题模式
// @return 无
export function applyTheme(dark: boolean, mode: ThemeMode = themeMode.value): void {
  if (typeof document === 'undefined') return
  document.documentElement.dataset.theme = dark ? 'dark' : 'light'
  document.documentElement.style.colorScheme = dark ? 'dark' : 'light'
  if (typeof window !== 'undefined') {
    try {
      window.localStorage.setItem(storageKey, mode)
    } catch {
      // 隐私模式或测试环境可能禁用存储，主题仍在当前会话生效
    }
  }
}

watch(
  [isDark, themeMode],
  ([dark, mode]) => applyTheme(dark, mode),
  { immediate: true },
)

// 设置主题模式。
// @param mode 主题模式
// @return 无
export function setThemeMode(mode: ThemeMode): void {
  themeMode.value = mode
}

// 在浅色、深色、跟随系统之间循环切换。
// @return 切换后的模式
export function cycleTheme(): ThemeMode {
  const next: ThemeMode = themeMode.value === 'light' ? 'dark' : themeMode.value === 'dark' ? 'system' : 'light'
  setThemeMode(next)
  return next
}

// 初始化系统主题监听。
// @return 无
export function initTheme(): void {
  watchSystemTheme()
  applyTheme(isDark.value)
}

// 主题模式的中文标签。
// @param mode 主题模式
// @return 中文标签
export function themeModeLabel(mode: ThemeMode): string {
  return mode === 'light' ? '浅色' : mode === 'dark' ? '深色' : '跟随系统'
}