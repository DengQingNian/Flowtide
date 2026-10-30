import { afterEach, describe, expect, it, vi } from 'vitest'
import { NInput } from 'naive-ui'
import HistoricVariablesPage from '@/pages/history/HistoricVariablesPage.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import { authNotice } from '@/stores/ui'
import { mockFetch, pageOf, settle } from '../helpers'
import { clickButton, mountAt } from './support'

/** 历史变量样本 */
const VARIABLE = {
  id: 'v1',
  processInstanceId: 'pi-1',
  taskId: 't1',
  executionId: 'e1',
  variable: { name: 'amount', scope: 'local', type: 'long', value: 100 },
}
/** 历史明细样本 */
const DETAIL = {
  id: 'd1',
  detailType: 'VariableUpdate',
  processInstanceId: 'pi-1',
  taskId: 't1',
  executionId: 'e1',
  revision: 2,
  time: '2026-10-30T03:00:00Z',
  variable: { name: 'amount', scope: 'local', type: 'long', value: 100 },
}

afterEach(() => {
  vi.unstubAllGlobals()
  authNotice.value = 'none'
  document.body.innerHTML = ''
})

describe('历史变量与明细', () => {
  it('加载变量实例并按筛选查询', async () => {
    const calls = mockFetch([{ path: '/service/query/historic-variable-instances', method: 'POST', body: pageOf([VARIABLE]) }])
    const { wrapper } = await mountAt(HistoricVariablesPage, '/history/variables')
    expect(wrapper.text()).toContain('amount')
    expect(wrapper.text()).toContain('任务局部')
    expect(wrapper.text()).toContain('共 1 条')
    expect(calls[0].body).toMatchObject({ start: 0, size: 50, sort: 'name', order: 'asc' })

    wrapper.findAllComponents(NInput)[0].vm.$emit('update:value', 'pi-1')
    await clickButton(wrapper, '查询')
    expect(calls[1].body).toMatchObject({ processInstanceId: 'pi-1', start: 0 })
    wrapper.unmount()
  })

  it('空数据与接口错误', async () => {
    mockFetch([{ path: '/service/query/historic-variable-instances', method: 'POST', body: pageOf([]) }])
    const empty = await mountAt(HistoricVariablesPage, '/history/variables')
    expect(empty.wrapper.text()).toContain('暂无历史变量')
    empty.wrapper.unmount()

    mockFetch([
      { path: '/service/query/historic-variable-instances', method: 'POST', status: 403, body: { message: '无权限查看变量' } },
    ])
    const failed = await mountAt(HistoricVariablesPage, '/history/variables')
    expect(failed.wrapper.text()).toContain('无权限查看变量')
    expect(authNotice.value).toBe('forbidden')
    await failed.wrapper.find('button[title="重试"]').trigger('click')
    await settle()
    failed.wrapper.unmount()
  })

  it('切换到变量明细页签并渲染明细', async () => {
    const calls = mockFetch([
      { path: '/service/query/historic-variable-instances', method: 'POST', body: pageOf([VARIABLE]) },
      { path: '/service/query/historic-detail', method: 'POST', body: pageOf([DETAIL]) },
    ])
    const { wrapper, router } = await mountAt(HistoricVariablesPage, '/history/variables')
    await clickButton(wrapper, '变量明细')
    expect(wrapper.text()).toContain('记录时间')
    expect(wrapper.text()).toContain('d1')
    expect(wrapper.text()).toContain('amount')
    expect(calls[1].body).toMatchObject({ start: 0, size: 50, sort: 'time', order: 'desc' })
    expect(router.currentRoute.value.query.tab).toBe('details')
    wrapper.unmount()
  })

  it('明细空态与错误提示', async () => {
    mockFetch([
      { path: '/service/query/historic-variable-instances', method: 'POST', body: pageOf([]) },
      { path: '/service/query/historic-detail', method: 'POST', body: pageOf([]) },
    ])
    const empty = await mountAt(HistoricVariablesPage, '/history/variables?tab=details')
    expect(empty.wrapper.text()).toContain('暂无历史明细')
    empty.wrapper.unmount()

    mockFetch([
      { path: '/service/query/historic-variable-instances', method: 'POST', body: pageOf([]) },
      { path: '/service/query/historic-detail', method: 'POST', status: 500, body: { message: '明细查询失败' } },
    ])
    const failed = await mountAt(HistoricVariablesPage, '/history/variables?tab=details')
    expect(failed.wrapper.text()).toContain('明细查询失败')
    failed.wrapper.unmount()
  })

  it('删除变量成功与失败', async () => {
    const calls = mockFetch([
      { path: '/service/history/historic-variable-instances/v1', method: 'DELETE', status: 204 },
      { path: '/service/query/historic-variable-instances', method: 'POST', body: pageOf([VARIABLE]) },
    ])
    const ok = await mountAt(HistoricVariablesPage, '/history/variables')
    ok.wrapper.findComponent(ConfirmButton).vm.$emit('confirm')
    await settle()
    expect(calls.some((call) => call.method === 'DELETE' && call.url.endsWith('/v1'))).toBe(true)
    ok.wrapper.unmount()

    mockFetch([
      { path: '/service/history/historic-variable-instances/v1', method: 'DELETE', status: 422, body: { message: '变量删除失败' } },
      { path: '/service/query/historic-variable-instances', method: 'POST', body: pageOf([VARIABLE]) },
    ])
    const failed = await mountAt(HistoricVariablesPage, '/history/variables')
    failed.wrapper.findComponent(ConfirmButton).vm.$emit('confirm')
    await settle()
    expect(failed.wrapper.text()).toContain('变量删除失败')
    failed.wrapper.unmount()
  })

  it('删除明细成功', async () => {
    const calls = mockFetch([
      { path: '/service/history/historic-detail/d1', method: 'DELETE', status: 204 },
      { path: '/service/query/historic-variable-instances', method: 'POST', body: pageOf([VARIABLE]) },
      { path: '/service/query/historic-detail', method: 'POST', body: pageOf([DETAIL]) },
    ])
    const { wrapper } = await mountAt(HistoricVariablesPage, '/history/variables?tab=details')
    wrapper.findComponent(ConfirmButton).vm.$emit('confirm')
    await settle()
    expect(calls.some((call) => call.method === 'DELETE' && call.url.endsWith('/d1'))).toBe(true)
    wrapper.unmount()
  })

  it('查看变量与明细原始数据', async () => {
    mockFetch([
      { path: '/service/history/historic-variable-instances/v1/data', text: 'variable-raw', contentType: 'application/octet-stream' },
      { path: '/service/history/historic-detail/d1/data', text: 'detail-raw', contentType: 'application/octet-stream' },
      { path: '/service/query/historic-variable-instances', method: 'POST', body: pageOf([VARIABLE]) },
      { path: '/service/query/historic-detail', method: 'POST', body: pageOf([DETAIL]) },
    ])
    const { wrapper } = await mountAt(HistoricVariablesPage, '/history/variables')
    await clickButton(wrapper, '原始')
    await settle()
    expect(document.body.textContent).toContain('variable-raw')
    // 关闭弹窗后切换明细再查看原始数据
    const close = Array.from(document.body.querySelectorAll('button')).find((button) => button.textContent?.trim() === '关闭')
    close?.click()
    await settle()
    await clickButton(wrapper, '变量明细')
    await clickButton(wrapper, '原始')
    await settle()
    expect(document.body.textContent).toContain('detail-raw')
    wrapper.unmount()
  })

  it('分页与重置写回路由', async () => {
    const calls = mockFetch([{ path: '/service/query/historic-variable-instances', method: 'POST', body: pageOf([VARIABLE], 120) }])
    const { wrapper, router } = await mountAt(HistoricVariablesPage, '/history/variables?page=2&size=50&name=amount')
    expect(calls[0].body).toMatchObject({ start: 50, name: 'amount' })
    await clickButton(wrapper, '重置')
    expect(calls[1].body).not.toHaveProperty('name')
    expect(router.currentRoute.value.query.name).toBeUndefined()
    wrapper.unmount()
  })
})
