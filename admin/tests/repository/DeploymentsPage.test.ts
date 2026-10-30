import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount } from '@vue/test-utils'
import DeploymentsPage from '@/pages/repository/DeploymentsPage.vue'
import { mockFetch, pageOf, settle } from '../helpers'
import { findButton, mountPage } from './testUtils'

enableAutoUnmount(afterEach)
afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

// 构造部署列表路由。
// @param overrides 覆盖项
// @return 模拟路由
function routes(overrides: { deployments?: unknown; definitions?: unknown; deleteStatus?: number; postStatus?: number } = {}) {
  const deployments = 'deployments' in overrides ? overrides.deployments : pageOf([
    { id: 'd1', name: '请假流程部署', tenantId: 't1', deployTime: '2026-10-30T03:00:00Z' },
    { id: 'd2', name: '报销流程部署', tenantId: 't2', deployTime: '2026-10-29T03:00:00Z' },
  ])
  const definitions = 'definitions' in overrides ? overrides.definitions : pageOf([
    { id: 'def1', key: 'leave', name: '请假', version: 1, deploymentId: 'd1', tenantId: 't1' },
    { id: 'def2', key: 'expense', name: '报销', version: 2, deploymentId: 'd2', tenantId: 't2' },
  ])
  return [
    { path: '/service/repository/deployments/d1/resources', body: [{ id: 'leave.bpmn20.xml' }, { id: 'diagram.png' }] },
    { path: '/service/repository/deployments/d2/resources', body: [] },
    { path: '/service/repository/process-definitions', body: definitions },
    { path: '/service/repository/deployments', body: deployments },
    { path: '/service/repository/deployments/d1', method: 'DELETE', status: overrides.deleteStatus ?? 204, body: { message: '删除失败' } },
    { path: '/service/repository/deployments', method: 'POST', status: overrides.postStatus ?? 201, body: overrides.postStatus ? { message: '上传失败' } : { id: 'd9', name: '新部署' } },
  ]
}

describe('DeploymentsPage', () => {
  it('渲染部署、关联 Key 与资源数', async () => {
    const calls = mockFetch(routes())
    const { wrapper } = await mountPage(DeploymentsPage, '/repository/deployments')
    await settle()
    await settle()
    expect(wrapper.text()).toContain('请假流程部署')
    expect(wrapper.text()).toContain('报销流程部署')
    expect(wrapper.text()).toContain('leave')
    expect(wrapper.text()).toContain('expense')
    expect(calls.some((call) => call.url === '/service/repository/deployments/d1/resources')).toBe(true)
    expect(wrapper.text()).toContain('2026-10-30')
  })

  it('空数据展示空态', async () => {
    mockFetch(routes({ deployments: pageOf([]), definitions: pageOf([]) }))
    const { wrapper } = await mountPage(DeploymentsPage, '/repository/deployments')
    await settle()
    expect(wrapper.text()).toContain('还没有部署')
  })

  it('接口错误展示 ErrorBlock 并重试', async () => {
    const calls = mockFetch([{ path: '/service/repository/deployments', status: 500, body: { code: 'ENGINE-1', message: '仓库故障' } }])
    const { wrapper } = await mountPage(DeploymentsPage, '/repository/deployments')
    await settle()
    expect(wrapper.text()).toContain('仓库故障')
    expect(wrapper.text()).toContain('ENGINE-1')
    expect(calls.length).toBeGreaterThan(1)
  })

  it('按路由 query 还原名称与租户筛选', async () => {
    mockFetch(routes())
    const { wrapper } = await mountPage(DeploymentsPage, '/repository/deployments?nameLike=请假&tenantId=t1')
    await settle()
    expect(wrapper.text()).toContain('请假流程部署')
    expect(wrapper.text()).not.toContain('报销流程部署')
  })

  it('查询按钮写入路由并重置页码', async () => {
    mockFetch(routes())
    const { wrapper, router } = await mountPage(DeploymentsPage, '/repository/deployments?page=2')
    await settle()
    const inputs = wrapper.findAll('input')
    await inputs[0].setValue('报销')
    await inputs[1].setValue('t2')
    findButton('查询', wrapper.element).click()
    await settle()
    expect(router.currentRoute.value.query.nameLike).toBe('报销')
    expect(router.currentRoute.value.query.tenantId).toBe('t2')
    expect(router.currentRoute.value.query.page).toBeUndefined()
    expect(wrapper.text()).toContain('报销流程部署')
    expect(wrapper.text()).not.toContain('请假流程部署')
  })

  it('上传部署成功后跳转详情', async () => {
    const calls = mockFetch(routes())
    const { router } = await mountPage(DeploymentsPage, '/repository/deployments')
    await settle()
    findButton('上传部署').click()
    await settle()
    const input = document.querySelector('input[type=file]') as HTMLInputElement
    const file = new File(['<definitions/>'], 'leave.bpmn20.xml', { type: 'application/xml' })
    Object.defineProperty(input, 'files', { value: [file], configurable: true })
    input.dispatchEvent(new Event('change'))
    await settle()
    findButton('上传并部署').click()
    await settle()
    await settle()
    const post = calls.find((call) => call.method === 'POST')
    expect(post).toBeTruthy()
    expect(router.currentRoute.value.fullPath).toBe('/repository/deployments/d9')
  })

  it('未选择文件时上传给出校验提示', async () => {
    mockFetch(routes())
    await mountPage(DeploymentsPage, '/repository/deployments')
    await settle()
    findButton('上传部署').click()
    await settle()
    findButton('上传并部署').click()
    await settle()
    expect(document.body.textContent).toContain('请选择至少一个')
  })

  it('上传失败时保留弹窗并展示服务端原文', async () => {
    mockFetch(routes({ postStatus: 500 }))
    await mountPage(DeploymentsPage, '/repository/deployments')
    await settle()
    findButton('上传部署').click()
    await settle()
    const input = document.querySelector('input[type=file]') as HTMLInputElement
    Object.defineProperty(input, 'files', { value: [new File(['x'], 'a.bpmn')], configurable: true })
    input.dispatchEvent(new Event('change'))
    await settle()
    findButton('上传并部署').click()
    await settle()
    await settle()
    expect(document.body.textContent).toContain('上传失败')
    expect(document.body.textContent).toContain('上传部署')
  })

  it('删除失败展示 ErrorBlock', async () => {
    mockFetch(routes({ deleteStatus: 500 }))
    const { wrapper } = await mountPage(DeploymentsPage, '/repository/deployments')
    await settle()
    findButton('删除', wrapper.element).click()
    await settle()
    findButton('Confirm').click()
    await settle()
    await settle()
    expect(wrapper.text()).toContain('删除失败')
  })
})