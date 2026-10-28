import { ref } from 'vue'

export const paletteOpen = ref(false)

// openPalette 打开搜索面板。
// @return 无
export function openPalette(): void {
  paletteOpen.value = true
}

// closePalette 关闭搜索面板。
// @return 无
export function closePalette(): void {
  paletteOpen.value = false
}
