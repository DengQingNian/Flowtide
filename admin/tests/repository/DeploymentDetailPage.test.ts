import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount } from '@vue/test-utils'
import DeploymentDetailPage from '@/pages/repository/DeploymentDetailPage.vue'
import { mockFetch, pageOf, settle } from '../helpers'
import { findButton, mountPage } from './testUtils'

enableAutoUnmount(afterEach)
afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

// 构造部署详情路由。
// @param options 覆盖项
// @return 模拟路由
function routes(options: { detailStatus?: number; resourceStatus?: number; deleteStatus?: number } = {}) {
  return [
    { path: '/service/repository/deployments/d1/resourcedata/leave.bpmn20.xml', text: '<definitions><process id="leave"/></definitions>', contentType: 'text/xml' },
    { path: '/service/repository/deployments/d1/resourcedata/diagram.png', text: 'png-bytes', contentType: 'image/png' },
    { path: '/service/repository/deployments/d1/resourcedata/data.bin', text: 'binary-bytes', contentType: 'application/octet-stream' },
    { path: '/service/repository/deployments/d1/resources', status: options.resourceStatus ?? 200, body: options.resourceStatus ? { message: '资源读取失败' } : [
      { id: 'leave.bpmn20.xml', mediaType: 'text/xml', type: 'processDefinition' },
      { id: 'diagram.png', mediaType: 'image/png', type: 'processImage' },
      { id: 'data.bin', mediaType: 'application/octet-stream', type: 'resource' },
    ] },
    { path: '/service/repository/process-definitions', body: pageOf([
      { id: 'def1', key: 'leave', name: '请假', version: 1, deploymentId: 'd1', tenantId: 't1' },
      { id: 'def2', key: 'other', name: '其他', version: 1, deploymentId: 'd2', tenantId: 't1' },
    ]) },
    { path: '/service/repository/deployments/d1', method: 'DELETE', status: options.deleteStatus ?? 204, body: { message: '删除失败' } },
    { path: '/service/repository/deployments/d1', status: options.detailStatus ?? 200, body: options.detailStatus ? { message: '部署不存在' } : { id: 'd1', name: '请假流程部署', tenantId: 't1', deployTime: '2026-10-30T03:00:00Z' } },
  ]
}

describe('DeploymentDetailPage', () => {
  it('渲染基本信息、资源清单与关联定义', async () => {
    mockFetch(routes())
    const { wrapper } = await mountPage(DeploymentDetailPage, '/repository/deployments/d1')
    await settle()
    await settle()
    expect(wrapper.text()).toContain('请假流程部署')
    expect(wrapper.text()).toContain('d1')
    expect(wrapper.text()).toContain('leave.bpmn20.xml')
    expect(wrapper.text()).toContain('diagram.png')
    expect(wrapper.text()).toContain('请假')
    expect(wrapper.text()).not.toContain('其他')
  })

  it('默认预览 XML 资源', async () => {
    mockFetch(routes())
    const { wrapper } = await mountPage(DeploymentDetailPage, '/repository/deployments/d1')
    await settle()
    await settle()
    expect(wrapper.text()).toContain('<definitions>')
  })

  it('切换图片与二进制资源时展示对应预览', async () => {
    mockFetch(routes())
    const { wrapper } = await mountPage(DeploymentDetailPage, '/repository/deployments/d1')
    await settle()
    await settle()
    const root = wrapper.element as HTMLElement
    const previews = Array.from(root.querySelectorAll<HTMLButtonElement>('button')).filter((button) => (button.textContent ?? '').trim() === '预览')
    previews[1].click()
    await settle()
    expect(wrapper.find('img').exists()).toBe(true)
    previews[2].click()
    await settle()
    expect(wrapper.text()).toContain('二进制内容')
  })

  it('部署不存在时展示 ErrorBlock', async () => {
    mockFetch(routes({ detailStatus: 404 }))
    const { wrapper } = await mountPage(DeploymentDetailPage, '/repository/deployments/d1')
    await settle()
    expect(wrapper.text()).toContain('部署不存在')
    expect(wrapper.find('button[title="重试"]').exists()).toBe(true)
  })

  it('资源读取失败时保留基本信息并给出重试', async () => {
    mockFetch(routes({ resourceStatus: 500 }))
    const { wrapper } = await mountPage(DeploymentDetailPage, '/repository/deployments/d1')
    await settle()
    await settle()
    expect(wrapper.text()).toContain('请假流程部署')
    expect(wrapper.text()).toContain('资源读取失败')
  })

  it('删除部署成功后返回列表', async () => {
    const calls = mockFetch(routes())
    const { router } = await mountPage(DeploymentDetailPage, '/repository/deployments/d1')
    await settle()
    findButton('删除部署').click()
    await settle()
    findButton('Confirm').click()
    await settle()
    await settle()
    const deletion = calls.find((call) => call.method === 'DELETE')
    expect(deletion?.url).toBe('/service/repository/deployments/d1?cascade=true')
    expect(router.currentRoute.value.fullPath).toBe('/repository/deployments')
  })

  it('删除失败时展示服务端原文', async () => {
    mockFetch(routes({ deleteStatus: 500 }))
    const { wrapper } = await mountPage(DeploymentDetailPage, '/repository/deployments/d1')
    await settle()
    findButton('删除部署').click()
    await settle()
    findButton('Confirm').click()
    await settle()
    await settle()
    expect(wrapper.text()).toContain('删除失败')
  })
})