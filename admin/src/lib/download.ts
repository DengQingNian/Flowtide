// 浏览器下载工具：文本、二进制与文件名清理。

// sanitizeFileName 清理文件名中的非法字符。
// @param name 原始文件名
// @return 安全文件名
export function sanitizeFileName(name: string): string {
  const cleaned = name.replace(/[\\/:*?"<>|\u0000-\u001f]/g, '_').trim()
  return cleaned || 'download'
}

// downloadBlob 触发浏览器下载。
// @param filename 文件名
// @param blob 内容
// @return 无
export function downloadBlob(filename: string, blob: Blob): void {
  if (typeof document === 'undefined') return
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = sanitizeFileName(filename)
  anchor.rel = 'noopener'
  document.body.appendChild(anchor)
  anchor.click()
  document.body.removeChild(anchor)
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

// downloadText 下载文本文件。
// @param filename 文件名
// @param text 文本内容
// @param mime MIME 类型
// @return 无
export function downloadText(filename: string, text: string, mime = 'text/plain;charset=utf-8'): void {
  downloadBlob(filename, new Blob([text], { type: mime }))
}

// downloadJson 下载 JSON 文件。
// @param filename 文件名
// @param value 任意可序列化值
// @return 无
export function downloadJson(filename: string, value: unknown): void {
  downloadText(filename, JSON.stringify(value, null, 2), 'application/json;charset=utf-8')
}

// extensionOf 返回文件扩展名（小写，不含点）。
// @param name 文件名
// @return 扩展名或空串
export function extensionOf(name: string): string {
  const index = name.lastIndexOf('.')
  return index >= 0 ? name.slice(index + 1).toLowerCase() : ''
}
