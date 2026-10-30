// 批处理详情页测试：摘要、分片详情、文档、删除与错误处理。
import { afterEach, describe, expect, it, vi } from 'vitest'
import { mockFetch, pageOf, settle } from '../helpers'
import BatchDetailPage from '@/pages/management/BatchDetailPage.vue'
import DataTable from '@/components/DataTable.vue'
import { buttonByText, confirmByLabel, mountPage } from './pageUtils'

afterEach(() => {
  vi.unstubAllGlobals()
})

// 构造批次。
// @return 批次对象
function batch() {
  return {
    id: 'batch-1',
    type: 'process-instance-migration',
    status: 'completed',
    total: 2,
    completed: 1,
    startTime: '2026-10-30T12:00:00Z',
    endTime: '2026-10-30T12:05:00Z',
    tenantId: 'tenant-a',
  }
}

// 构造分片。
// @return 分片对象
function part() {
  return {
    id: 'part-1',
    batchId: 'batch-1',
    type: 'process-instance-migration',
    processInstanceId: 'pi-1',
    status: 'completed',
    startTime: '2026-10-30T12:00:00Z',
    endTime: '2026-10-30T12:01:00Z',
  }
}

// 构造成功加载所需路由。
// @param extra 额外路由
// @return 路由列表
function routes(extra: { method?: string; path: string | RegExp; status?: number; body?: unknown; text?: string }[] = []) {
  return [
    ...extra,
    { method: 'GET', path: '/service/management/batches/batch-1/batch-document', body: { target: 'def-1' } },
    { method: 'GET', path: '/service/management/batches/batch-1/summary', body: { id: 'batch-1', batchId: 'batch-1', total: 2, completed: 1, failed: 1 } },
    { method: 'GET', path: /^\/service\/management\/batches\/batch-1\/batch-parts\?/, body: pageOf([part()]) },
    { method: 'GET', path: '/service/management/batch-parts/part-1/batch-part-document', body: { instance: 'pi-1' } },
    { method: 'GET', path: '/service/management/batch-parts/part-1', body: part() },
    { method: 'GET', path: '/service/management/batches/batch-1', body: batch() },
  ] as never
}

describe('BatchDetailPage', () => {
  it('渲染摘要、文档与分片列表', async () => {
    mockFetch(routes())
    const { wrapper } = await mountPage(BatchDetailPage, '/management/batches/batch-1')
    await settle()
    expect(wrapper.text()).toContain('process-instance-migration')
    expect(wrapper.text()).toContain('target')
    expect(wrapper.text()).toContain('part-1')
    expect(wrapper.findAll('a').some((link) => link.attributes('href') === '/runtime/instances/pi-1')).toBe(true)
  })

  it('查看分片详情与分片文档', async () => {
    const calls = mockFetch(routes())
    const { wrapper } = await mountPage(BatchDetailPage, '/management/batches/batch-1')
    await settle()
    await buttonByText(wrapper, '查看')!.trigger('click')
    await settle()
    expect(wrapper.text()).toContain('分片详情')
    expect(wrapper.text()).toContain('pi-1')
    expect(wrapper.text()).toContain('instance')
    expect(calls.some((call) => call.url === '/service/management/batch-parts/part-1')).toBe(true)
  })

  it('分片翻页请求下一页偏移', async () => {
    const calls = mockFetch(routes([{ method: 'GET', path: /^\/service\/management\/batches\/batch-1\/batch-parts\?start=50&size=50$/, body: pageOf([], 80) }]))
    const { wrapper } = await mountPage(BatchDetailPage, '/management/batches/batch-1')
    await settle()
    wrapper.findComponent(DataTable).vm.$emit('update:page', 2)
    await settle()
    expect(calls.some((call) => call.url.includes('/batch-parts?start=50&size=50'))).toBe(true)
  })

  it('删除批次后返回列表', async () => {
    const calls = mockFetch(routes([{ method: 'DELETE', path: '/service/management/batches/batch-1', status: 204 }]))
    const { wrapper, router } = await mountPage(BatchDetailPage, '/management/batches/batch-1')
    await settle()
    confirmByLabel(wrapper, '删除批次')!.vm.$emit('confirm')
    await settle()
    expect(calls.some((call) => call.method === 'DELETE')).toBe(true)
    expect(router.currentRoute.value.path).toBe('/management/batches')
  })

  it('删除失败保留页面并展示原文', async () => {
    mockFetch(routes([{ method: 'DELETE', path: '/service/management/batches/batch-1', status: 403, body: { code: 'REST-0403', message: '无权限' } }]))
    const { wrapper, router } = await mountPage(BatchDetailPage, '/management/batches/batch-1')
    await settle()
    confirmByLabel(wrapper, '删除批次')!.vm.$emit('confirm')
    await settle()
    expect(wrapper.text()).toContain('无权限')
    expect(router.currentRoute.value.path).toBe('/management/batches/batch-1')
  })

  it('详情接口错误展示 ErrorBlock', async () => {
    mockFetch([{ method: 'GET', path: '/service/management/batches/batch-1', status: 500, body: { code: 'ENGINE-0700', message: '批次不存在' } }])
    const { wrapper } = await mountPage(BatchDetailPage, '/management/batches/batch-1')
    await settle()
    expect(wrapper.text()).toContain('批次不存在')
  })

  it('没有批次文档时展示空态', async () => {
    mockFetch(routes([{ method: 'GET', path: '/service/management/batches/batch-1/batch-document', status: 204 }]))
    const { wrapper } = await mountPage(BatchDetailPage, '/management/batches/batch-1')
    await settle()
    expect(wrapper.text()).toContain('没有批次文档')
  })
})
