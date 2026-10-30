// 审计日志页测试：渲染、本地筛选、分页、空态与错误。
import { afterEach, describe, expect, it, vi } from 'vitest'
import { mockFetch, pageOf, settle } from '../helpers'
import AuditPage from '@/pages/management/AuditPage.vue'
import DataTable from '@/components/DataTable.vue'
import { buttonByText, mountPage } from './pageUtils'

afterEach(() => {
  vi.unstubAllGlobals()
})

// 构造审计事件。
// @param overrides 覆盖字段
// @return 审计事件
function event(overrides: Record<string, unknown> = {}) {
  return {
    id: 1,
    time: '2026-10-30T12:00:00Z',
    userId: 'alice',
    tenantId: 'tenant-a',
    method: 'GET',
    path: '/service/runtime/tasks',
    status: 200,
    durationMillis: 12,
    ...overrides,
  }
}

describe('AuditPage', () => {
  it('渲染时间、主体、方法、路径、状态与耗时', async () => {
    mockFetch([
      {
        method: 'GET',
        path: /^\/service\/management\/audit-logs\?/,
        body: pageOf([event(), event({ id: 2, method: 'POST', path: '/service/runtime/tasks', status: 500, durationMillis: 1500 })]),
      },
    ])
    const { wrapper } = await mountPage(AuditPage, '/management/audit')
    await settle()
    expect(wrapper.text()).toContain('alice')
    expect(wrapper.text()).toContain('/service/runtime/tasks')
    expect(wrapper.text()).toContain('500')
    expect(wrapper.text()).toContain('1秒')
    expect(wrapper.text()).toContain('已加载最近 2 条')
  })

  it('按方法、路径、状态与主体本地筛选并写入 URL', async () => {
    mockFetch([
      {
        method: 'GET',
        path: /^\/service\/management\/audit-logs\?/,
        body: pageOf([
          event({ id: 1, method: 'GET', path: '/service/runtime/tasks', status: 200, userId: 'alice' }),
          event({ id: 2, method: 'POST', path: '/service/management/jobs/run', status: 500, userId: 'bob' }),
        ]),
      },
    ])
    const { wrapper, router } = await mountPage(AuditPage, '/management/audit')
    await settle()
    await wrapper.find('select').setValue('POST')
    await wrapper.findAll('input')[0].setValue('jobs')
    await wrapper.findAll('input')[1].setValue('500')
    await wrapper.findAll('input')[2].setValue('bob')
    await buttonByText(wrapper, '查询')!.trigger('click')
    await settle()
    expect(wrapper.text()).toContain('/service/management/jobs/run')
    expect(wrapper.text()).not.toContain('/service/runtime/tasks')
    expect(router.currentRoute.value.query.method).toBe('POST')
    expect(router.currentRoute.value.query.path).toBe('jobs')
  })

  it('翻页只切换本地页码并写入 URL', async () => {
    const events = Array.from({ length: 30 }, (_, index) => event({ id: index + 1, path: `/service/item/${index + 1}` }))
    mockFetch([{ method: 'GET', path: /^\/service\/management\/audit-logs\?/, body: pageOf(events) }])
    const { wrapper, router } = await mountPage(AuditPage, '/management/audit?size=10')
    await settle()
    wrapper.findComponent(DataTable).vm.$emit('update:page', 2)
    await settle()
    expect(router.currentRoute.value.query.page).toBe('2')
    expect(wrapper.text()).toContain('/service/item/11')
  })

  it('从 URL 还原筛选与分页', async () => {
    const events = Array.from({ length: 30 }, (_, index) => event({ id: index + 1, path: `/service/item/${index + 1}` }))
    mockFetch([{ method: 'GET', path: /^\/service\/management\/audit-logs\?/, body: pageOf(events) }])
    const { wrapper } = await mountPage(AuditPage, '/management/audit?method=GET&path=item&page=2&size=10')
    await settle()
    expect(wrapper.text()).toContain('/service/item/11')
  })

  it('空审计展示空态', async () => {
    mockFetch([{ method: 'GET', path: /^\/service\/management\/audit-logs\?/, body: pageOf([]) }])
    const { wrapper } = await mountPage(AuditPage, '/management/audit')
    await settle()
    expect(wrapper.text()).toContain('没有匹配的审计记录')
  })

  it('接口错误展示 ErrorBlock', async () => {
    mockFetch([{ method: 'GET', path: /^\/service\/management\/audit-logs\?/, status: 500, body: { code: 'ENGINE-5000', message: '审计服务异常' } }])
    const { wrapper } = await mountPage(AuditPage, '/management/audit')
    await settle()
    expect(wrapper.text()).toContain('审计服务异常')
  })
})
