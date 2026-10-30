// 批处理列表页测试：渲染、筛选、分页与错误/空态。
import { afterEach, describe, expect, it, vi } from 'vitest'
import { mockFetch, pageOf, settle } from '../helpers'
import BatchesPage from '@/pages/management/BatchesPage.vue'
import DataTable from '@/components/DataTable.vue'
import { buttonByText, mountPage } from './pageUtils'

afterEach(() => {
  vi.unstubAllGlobals()
})

// 构造批次。
// @param overrides 覆盖字段
// @return 批次对象
function batch(overrides: Record<string, unknown> = {}) {
  return {
    id: 'batch-1',
    type: 'process-instance-migration',
    status: 'completed',
    total: 10,
    completed: 9,
    startTime: '2026-10-30T12:00:00Z',
    endTime: '2026-10-30T12:05:00Z',
    tenantId: 'tenant-a',
    ...overrides,
  }
}

describe('BatchesPage', () => {
  it('渲染批次类型、状态与分片进度', async () => {
    mockFetch([
      { method: 'GET', path: /^\/service\/management\/batches\?/, body: pageOf([batch(), batch({ id: 'batch-2', completed: 1, total: 4, endTime: null })]) },
    ])
    const { wrapper } = await mountPage(BatchesPage, '/management/batches')
    await settle()
    expect(wrapper.text()).toContain('process-instance-migration')
    expect(wrapper.text()).toContain('已完成')
    expect(wrapper.text()).toContain('90%')
    expect(wrapper.findAll('a').some((link) => link.attributes('href') === '/management/batches/batch-1')).toBe(true)
  })

  it('筛选条件写入 URL 并传递到接口', async () => {
    const calls = mockFetch([{ method: 'GET', path: /^\/service\/management\/batches\?/, body: pageOf([]) }])
    const { wrapper, router } = await mountPage(BatchesPage, '/management/batches')
    await settle()
    await wrapper.find('input[placeholder="如 process-instance-migration"]').setValue('migration')
    await wrapper.find('select').setValue('failed')
    await wrapper.find('input[placeholder="租户 ID"]').setValue('tenant-a')
    await buttonByText(wrapper, '查询')!.trigger('click')
    await settle()
    const url = calls[calls.length - 1].url
    expect(url).toContain('type=migration')
    expect(url).toContain('status=failed')
    expect(url).toContain('tenantId=tenant-a')
    expect(router.currentRoute.value.query.type).toBe('migration')
  })

  it('从 URL 还原筛选条件', async () => {
    const calls = mockFetch([{ method: 'GET', path: /^\/service\/management\/batches\?/, body: pageOf([]) }])
    const { wrapper } = await mountPage(BatchesPage, '/management/batches?type=migration&status=failed&tenantId=t1&page=2&size=20')
    await settle()
    expect(calls[0].url).toContain('type=migration')
    expect(calls[0].url).toContain('start=20')
    expect(calls[0].url).toContain('size=20')
    expect((wrapper.find('input[placeholder="如 process-instance-migration"]').element as HTMLInputElement).value).toBe('migration')
  })

  it('翻页时请求下一页偏移', async () => {
    const calls = mockFetch([{ method: 'GET', path: /^\/service\/management\/batches\?/, body: pageOf([], 120) }])
    const { wrapper } = await mountPage(BatchesPage, '/management/batches')
    await settle()
    wrapper.findComponent(DataTable).vm.$emit('update:page', 2)
    await settle()
    expect(calls[calls.length - 1].url).toContain('start=50')
  })

  it('重置筛选恢复默认条件', async () => {
    mockFetch([{ method: 'GET', path: /^\/service\/management\/batches\?/, body: pageOf([]) }])
    const { wrapper, router } = await mountPage(BatchesPage, '/management/batches?type=x&status=failed')
    await settle()
    await buttonByText(wrapper, '重置')!.trigger('click')
    await settle()
    expect(router.currentRoute.value.query.type).toBeUndefined()
    expect((wrapper.find('input[placeholder="如 process-instance-migration"]').element as HTMLInputElement).value).toBe('')
  })

  it('接口错误展示 ErrorBlock', async () => {
    mockFetch([{ method: 'GET', path: /^\/service\/management\/batches\?/, status: 500, body: { code: 'ENGINE-5000', message: '批次服务异常' } }])
    const { wrapper } = await mountPage(BatchesPage, '/management/batches')
    await settle()
    expect(wrapper.text()).toContain('批次服务异常')
  })

  it('空数据展示空态', async () => {
    mockFetch([{ method: 'GET', path: /^\/service\/management\/batches\?/, body: pageOf([]) }])
    const { wrapper } = await mountPage(BatchesPage, '/management/batches')
    await settle()
    expect(wrapper.text()).toContain('没有匹配的批处理作业')
  })
})
