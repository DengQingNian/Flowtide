// 通用格式化工具：时间、时长、字节、数字与短 ID。

// pad2 将数字补足两位。
// @param value 数字
// @return 两位字符串
function pad2(value: number): string {
  return value < 10 ? `0${value}` : String(value)
}

// toDate 将后端时间值转换为 Date。
// @param value 时间值
// @return 合法日期或 null
export function toDate(value: string | number | Date | null | undefined): Date | null {
  if (value === null || value === undefined || value === '') return null
  const date = value instanceof Date ? value : new Date(value)
  return Number.isNaN(date.getTime()) ? null : date
}

// formatDateTime 格式化日期时间为 YYYY-MM-DD HH:mm:ss。
// @param value 时间值
// @return 本地时间字符串，空值返回 —
export function formatDateTime(value: string | number | Date | null | undefined): string {
  const date = toDate(value)
  if (!date) return '—'
  return `${date.getFullYear()}-${pad2(date.getMonth() + 1)}-${pad2(date.getDate())} ${pad2(date.getHours())}:${pad2(date.getMinutes())}:${pad2(date.getSeconds())}`
}

// formatDate 格式化日期为 YYYY-MM-DD。
// @param value 时间值
// @return 日期字符串，空值返回 —
export function formatDate(value: string | number | Date | null | undefined): string {
  const date = toDate(value)
  if (!date) return '—'
  return `${date.getFullYear()}-${pad2(date.getMonth() + 1)}-${pad2(date.getDate())}`
}

// formatDurationMs 将毫秒格式化为中文时长。
// @param value 毫秒数
// @return 如 1天 2小时 3分 4秒，空值返回 —
export function formatDurationMs(value: number | null | undefined): string {
  if (value === null || value === undefined || !Number.isFinite(value)) return '—'
  if (value < 1000) return `${Math.max(0, Math.round(value))} 毫秒`
  const totalSeconds = Math.floor(value / 1000)
  const days = Math.floor(totalSeconds / 86400)
  const hours = Math.floor((totalSeconds % 86400) / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60
  const parts: string[] = []
  if (days > 0) parts.push(`${days}天`)
  if (hours > 0) parts.push(`${hours}小时`)
  if (minutes > 0) parts.push(`${minutes}分`)
  if (seconds > 0 || parts.length === 0) parts.push(`${seconds}秒`)
  return parts.join(' ')
}

// formatDurationSeconds 将秒格式化为中文时长。
// @param value 秒数
// @return 中文时长，空值返回 —
export function formatDurationSeconds(value: number | null | undefined): string {
  if (value === null || value === undefined) return '—'
  return formatDurationMs(value * 1000)
}

// formatBytes 将字节数格式化为可读大小。
// @param value 字节数
// @return 如 1.5 MB，空值返回 —
export function formatBytes(value: number | null | undefined): string {
  if (value === null || value === undefined || !Number.isFinite(value)) return '—'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let size = Math.max(0, value)
  let index = 0
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024
    index += 1
  }
  const digits = index === 0 ? 0 : size >= 100 ? 0 : 1
  return `${size.toFixed(digits)} ${units[index]}`
}

// formatNumber 格式化整数，添加千分位。
// @param value 数字
// @return 格式化字符串，空值返回 0
export function formatNumber(value: number | null | undefined): string {
  if (value === null || value === undefined || !Number.isFinite(value)) return '0'
  return new Intl.NumberFormat('zh-CN').format(value)
}

// shortId 截断长 ID 便于表格展示。
// @param value 原始 ID
// @param head 头部保留长度
// @param tail 尾部保留长度
// @return 截断后的 ID
export function shortId(value: string | null | undefined, head = 8, tail = 4): string {
  if (!value) return '—'
  if (value.length <= head + tail + 1) return value
  return `${value.slice(0, head)}…${value.slice(-tail)}`
}

// textOrDash 空值显示为短横线。
// @param value 文本
// @return 原文本或 —
export function textOrDash(value: string | number | boolean | null | undefined): string {
  if (value === null || value === undefined || value === '') return '—'
  return String(value)
}

// formatValue 将变量值格式化为单行文本。
// @param value 任意变量值
// @param maxLength 最大长度
// @return 展示文本
export function formatValue(value: unknown, maxLength = 120): string {
  if (value === null || value === undefined) return 'null'
  let text: string
  if (typeof value === 'string') text = value
  else if (typeof value === 'object') {
    try {
      text = JSON.stringify(value)
    } catch {
      text = String(value)
    }
  } else text = String(value)
  if (text.length > maxLength) return `${text.slice(0, maxLength)}…`
  return text
}

// formatJson 美化 JSON 文本。
// @param value 任意值
// @return 格式化 JSON，失败时退化为 String
export function formatJson(value: unknown): string {
  try {
    return JSON.stringify(value, null, 2) ?? String(value)
  } catch {
    return String(value)
  }
}
