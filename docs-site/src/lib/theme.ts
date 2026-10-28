import { computed, ref, watch } from 'vue'

const storageKey = 'flowable-go-docs-theme'
const preferred = typeof window !== 'undefined' && window.matchMedia('(prefers-color-scheme: dark)').matches
const stored = typeof window !== 'undefined' ? window.localStorage.getItem(storageKey) : null
const isDark = ref(stored ? stored === 'dark' : preferred)
const isReady = ref(false)

// applyTheme 把主题写到根元素与本地存储。
// @param dark 是否暗色
// @return 无
function applyTheme(dark: boolean): void {
  if (typeof document === 'undefined') return
  document.documentElement.dataset.theme = dark ? 'dark' : 'light'
  document.documentElement.style.colorScheme = dark ? 'dark' : 'light'
  window.localStorage.setItem(storageKey, dark ? 'dark' : 'light')
}

watch(
  isDark,
  (value) => {
    if (isReady.value) applyTheme(value)
  },
  { immediate: true },
)

// initTheme 在挂载时应用一次主题。
// @return 无
export function initTheme(): void {
  isReady.value = true
  applyTheme(isDark.value)
}

// toggleTheme 切换亮/暗主题。
// @return 无
export function toggleTheme(): void {
  isDark.value = !isDark.value
}

export const themeState = {
  isDark,
  isDarkMode: computed(() => isDark.value),
}
