// 运维域辅助函数测试：属性分类、表分组、二进制判断与审计筛选。
import { describe, expect, it } from 'vitest'
import { filterAuditEvents, groupTables, isBinaryColumn, isSystemProperty, JOB_BUCKET_LABELS, JOB_BUCKET_ORDER } from '@/pages/management/components/managementUtils'
import type { AuditEvent, TableInfo } from '@/api/management'

// 构造表信息。
// @param name 表名
// @param count 行数
// @return 表信息
function table(name: string, count: number): TableInfo {
  return { name, count }
}

// 构造审计事件。
// @param overrides 覆盖字段
// @return 审计事件
function audit(overrides: Partial<AuditEvent> = {}): AuditEvent {
  return {
    id: 1,
    time: '2026-10-30T12:00:00Z',
    method: 'GET',
    path: '/service/runtime/tasks',
    status: 200,
    durationMillis: 12,
    userId: 'alice',
    tenantId: 'tenant-a',
    ...overrides,
  }
}

describe('managementUtils', () => {
  it('作业桶文案与顺序固定', () => {
    expect(JOB_BUCKET_ORDER).toEqual(['executable', 'timer', 'suspended', 'deadletter'])
    expect(JOB_BUCKET_LABELS.deadletter).toBe('死信')
  })

  it('识别内置只读属性', () => {
    expect(isSystemProperty('schema.version')).toBe(true)
    expect(isSystemProperty('flowtide.schema.checksum')).toBe(true)
    expect(isSystemProperty('historyLevel')).toBe(true)
    expect(isSystemProperty('feature.flag')).toBe(false)
  })

  it('按前缀分组并汇总行数', () => {
    const groups = groupTables([
      table('ACT_RE_MODEL', 2),
      table('ACT_RU_JOBS', 3),
      table('ACT_HI_TASKINST', 5),
      table('ACT_ID_USER', 7),
      table('ACT_GE_PROPERTY', 11),
    ])
    expect(groups.map((group) => group.key)).toEqual(['repository', 'runtime', 'history', 'identity', 'other'])
    expect(groups[0].tables.map((item) => item.name)).toEqual(['ACT_RE_MODEL'])
    expect(groups[1].totalCount).toBe(3)
    expect(groups[4].tables.map((item) => item.name)).toEqual(['ACT_GE_PROPERTY'])
    expect(groups[4].totalCount).toBe(11)
  })

  it('空表清单返回固定空分组', () => {
    const groups = groupTables([])
    expect(groups).toHaveLength(5)
    expect(groups.every((group) => group.tables.length === 0)).toBe(true)
  })

  it('识别二进制列类型', () => {
    expect(isBinaryColumn('bytea')).toBe(true)
    expect(isBinaryColumn('LONGBLOB')).toBe(true)
    expect(isBinaryColumn('varbinary(64)')).toBe(true)
    expect(isBinaryColumn('varchar(255)')).toBe(false)
    expect(isBinaryColumn(undefined)).toBe(false)
  })

  it('无筛选时返回原数组', () => {
    const events = [audit()]
    expect(filterAuditEvents(events, {})).toBe(events)
  })

  it('按方法、路径、状态与主体筛选', () => {
    const events = [
      audit({ id: 1, method: 'GET', path: '/service/runtime/tasks', status: 200, userId: 'alice' }),
      audit({ id: 2, method: 'POST', path: '/service/runtime/tasks', status: 500, userId: 'bob' }),
      audit({ id: 3, method: 'GET', path: '/service/management/audit-logs', status: 403, userId: 'alice' }),
    ]
    expect(filterAuditEvents(events, { method: 'post' }).map((event) => event.id)).toEqual([2])
    expect(filterAuditEvents(events, { path: 'AUDIT' }).map((event) => event.id)).toEqual([3])
    expect(filterAuditEvents(events, { status: '500' }).map((event) => event.id)).toEqual([2])
    expect(filterAuditEvents(events, { userId: 'ALI' }).map((event) => event.id)).toEqual([1, 3])
    expect(filterAuditEvents(events, { method: 'GET', userId: 'alice', status: '403' }).map((event) => event.id)).toEqual([3])
  })

  it('缺失主体时按空串处理', () => {
    const events = [audit({ userId: undefined })]
    expect(filterAuditEvents(events, { userId: 'x' })).toHaveLength(0)
  })
})
