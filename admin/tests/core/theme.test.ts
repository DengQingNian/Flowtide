import { afterEach, describe, expect, it } from 'vitest'
import { nextTick } from 'vue'
import { applyTheme, cycleTheme, isDark, setThemeMode, themeMode, themeModeLabel } from '@/lib/theme'

afterEach(() => {
  setThemeMode('system')
  localStorage.clear()
  delete document.documentElement.dataset.theme
})

describe('主题状态', () => {
  it('默认跟随系统（测试环境为浅色）', async () => {
    await nextTick()
    expect(themeMode.value).toBe('system')
    expect(isDark.value).toBe(false)
    expect(document.documentElement.dataset.theme).toBe('light')
  })

  it('切换浅色与深色并写入存储', async () => {
    setThemeMode('dark')
    await nextTick()
    expect(isDark.value).toBe(true)
    expect(document.documentElement.dataset.theme).toBe('dark')
    expect(localStorage.getItem('flowtide-admin-theme')).toBe('dark')
    setThemeMode('light')
    await nextTick()
    expect(isDark.value).toBe(false)
    expect(document.documentElement.dataset.theme).toBe('light')
  })

  it('循环切换三种模式', () => {
    setThemeMode('light')
    expect(cycleTheme()).toBe('dark')
    expect(cycleTheme()).toBe('system')
    expect(cycleTheme()).toBe('light')
  })

  it('直接应用主题并支持标签', () => {
    applyTheme(true, 'dark')
    expect(document.documentElement.dataset.theme).toBe('dark')
    expect(themeModeLabel('light')).toBe('浅色')
    expect(themeModeLabel('dark')).toBe('深色')
    expect(themeModeLabel('system')).toBe('跟随系统')
  })
})