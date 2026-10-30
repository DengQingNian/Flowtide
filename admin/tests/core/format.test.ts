import { describe, expect, it } from 'vitest'
import {
  formatBytes,
  formatDate,
  formatDateTime,
  formatDurationMs,
  formatDurationSeconds,
  formatJson,
  formatNumber,
  formatValue,
  shortId,
  textOrDash,
  toDate,
} from '@/lib/format'

describe('时间格式化', () => {
  it('格式化本地时间与空值', () => {
    const date = new Date(2026, 9, 30, 8, 5, 9)
    expect(formatDateTime(date)).toBe('2026-10-30 08:05:09')
    expect(formatDate(date)).toBe('2026-10-30')
    expect(formatDateTime('')).toBe('—')
    expect(formatDateTime('not-a-date')).toBe('—')
    expect(toDate(null)).toBeNull()
    expect(toDate('2026-10-30T00:00:00Z')).toBeInstanceOf(Date)
  })

  it('格式化时长', () => {
    expect(formatDurationMs(undefined)).toBe('—')
    expect(formatDurationMs(500)).toBe('500 毫秒')
    expect(formatDurationMs(1000)).toBe('1秒')
    expect(formatDurationMs(90061000)).toBe('1天 1小时 1分 1秒')
    expect(formatDurationSeconds(125)).toBe('2分 5秒')
    expect(formatDurationSeconds(null)).toBe('—')
  })
})

describe('数值与文本格式化', () => {
  it('格式化字节', () => {
    expect(formatBytes(null)).toBe('—')
    expect(formatBytes(512)).toBe('512 B')
    expect(formatBytes(1536)).toBe('1.5 KB')
    expect(formatBytes(200 * 1024)).toBe('200 KB')
  })

  it('格式化数字与 ID', () => {
    expect(formatNumber(null)).toBe('0')
    expect(formatNumber(1234567)).toMatch(/1[,，]?234[,，]?567/)
    expect(shortId('')).toBe('—')
    expect(shortId('abc')).toBe('abc')
    expect(shortId('123456789012345')).toBe('12345678…2345')
    expect(textOrDash('')).toBe('—')
    expect(textOrDash(0)).toBe('0')
    expect(textOrDash(false)).toBe('false')
  })

  it('格式化变量值与 JSON', () => {
    expect(formatValue(null)).toBe('null')
    expect(formatValue(true)).toBe('true')
    expect(formatValue({ a: 1 })).toBe('{"a":1}')
    expect(formatValue('x'.repeat(20), 5)).toBe('xxxxx…')
    expect(formatJson({ a: 1 })).toBe('{\n  "a": 1\n}')
    const circular: Record<string, unknown> = {}
    circular.self = circular
    expect(formatJson(circular)).toBe('[object Object]')
  })
})
