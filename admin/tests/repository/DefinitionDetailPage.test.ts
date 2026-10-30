import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount } from '@vue/test-utils'
import type { VueWrapper } from '@vue/test-utils'
import DefinitionDetailPage from '@/pages/repository/DefinitionDetailPage.vue'
import { mockFetch, settle } from '../helpers'
import { findButton, mountPage } from './testUtils'

enableAutoUnmount(afterEach)
afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

// 构造定义详情路由。
// @param options 覆盖项
// @return 模拟路由
function routes(options: { suspended?: boolean; imageStatus?: number; detailStatus?: number; startStatus?: number } = {}) {
  return [
    { path: '/service/repository/process-definitions/def1/xml', text: '<definitions><process id="leave"/></definitions>', contentType: 'application/xml' },
    { path: '/service/repository/process-definitions/def1/image', status: options.imageStatus ?? 200, text: options.imageStatus ? '{"message":"无图"}' : 'png', contentType: options.imageStatus ? 'application/json' : 'image/png' },
    { path: '/service/repository/process-definitions/def1/start-form', body: { formKey: 'leaveForm', formProperties: [{ id: 'days', name: '请假天数', type: 'long', required: true }] } },
    { path: '/service/repository/process-definitions/def1/identitylinks', method: 'POST', body: { id: 'il9', userId: 'carol' } },
    { path: '/service/repository/process-definitions/def1/identitylinks/users/carol', method: 'DELETE', status: 204 },
    { path: '/service/repository/process-definitions/def1/identitylinks', body: [{ id: 'il1', userId: 'alice' }, { id: 'il2', groupId: 'g1' }] },
    { path: '/service/repository/process-definitions/def1/resourcedata', text: 'raw', contentType: 'application/octet-stream' },
    { path: '/service/runtime/process-instances', method: 'POST', status: options.startStatus ?? 201, body: options.startStatus ? { code: 'ENGINE-1', message: '挂起状态不能启动' } : { id: 'pi-1', processDefinitionId: 'def1' } },
    { path: '/service/repository/process-definitions/def1', method: 'PUT', status: 204 },
    { path: '/service/repository/process-definitions/def1', status: options.detailStatus ?? 200, body: options.detailStatus ? { code: 'ENGINE-0404', message: '流程定义不存在' } : { id: 'def1', key: 'leave', name: '请假流程', version: 3, deploymentId: 'd1', resourceName: 'leave.bpmn20.xml', tenantId: 't1', suspended: Boolean(options.suspended) } },
  ]
}

// 点击标签页。
// @param wrapper 挂载结果
// @param label 标签文本
// @return 无
function clickTab(wrapper: VueWrapper, label: string): void {
  const root = wrapper.element as HTMLElement
  const tabs = Array.from(root.querySelectorAll<HTMLElement>('.n-tabs-tab'))
  const tab = tabs.find((item) => (item.textContent ?? '').trim() === label)
  if (!tab) throw new Error(`未找到标签：${label}`)
  tab.dispatchEvent(new MouseEvent('click', { bubbles: true }))
}

describe('DefinitionDetailPage', () => {
  it('渲染基本信息、图片与 XML', async () => {
    mockFetch(routes())
    const { wrapper } = await mountPage(DefinitionDetailPage, '/repository/definitions/def1')
    await settle()
    await settle()
    expect(wrapper.text()).toContain('请假流程')
    expect(wrapper.text()).toContain('leave')
    expect(wrapper.text()).toContain('v3')
    expect(wrapper.text()).toContain('leave.bpmn20.xml')
    expect(wrapper.find('img').exists()).toBe(true)
    expect(wrapper.text()).toContain('请假流程')
  })

  it('图片 404 时降级展示 XML', async () => {
    mockFetch(routes({ imageStatus: 404 }))
    const { wrapper } = await mountPage(DefinitionDetailPage, '/repository/definitions/def1')
    await settle()
    await settle()
    expect(wrapper.text()).toContain('图形不可用')
    expect(wrapper.text()).toContain('<definitions>')
  })

  it('挂起定义并刷新状态', async () => {
    const calls = mockFetch(routes())
    const { wrapper } = await mountPage(DefinitionDetailPage, '/repository/definitions/def1')
    await settle()
    findButton('挂起', wrapper.element).click()
    await settle()
    findButton('Confirm').click()
    await settle()
    await settle()
    const put = calls.find((call) => call.method === 'PUT')
    expect(put?.body).toEqual({ action: 'suspend', includeProcessInstances: false })
    expect(wrapper.text()).toContain('流程定义已挂起')
  })

  it('已挂起定义展示提示并禁用启动', async () => {
    mockFetch(routes({ suspended: true }))
    const { wrapper } = await mountPage(DefinitionDetailPage, '/repository/definitions/def1')
    await settle()
    expect(wrapper.text()).toContain('已挂起')
    expect(wrapper.text()).toContain('激活')
    const root = wrapper.element as HTMLElement
    const start = Array.from(root.querySelectorAll<HTMLButtonElement>('button')).find((button) => (button.textContent ?? '').trim() === '启动实例') as HTMLButtonElement
    expect(start.disabled).toBe(true)
  })

  it('启动实例成功后跳转实例详情', async () => {
    mockFetch(routes())
    const { router } = await mountPage(DefinitionDetailPage, '/repository/definitions/def1')
    await settle()
    findButton('启动实例').click()
    await settle()
    findButton('启动').click()
    await settle()
    await settle()
    expect(router.currentRoute.value.fullPath).toBe('/runtime/instances/pi-1')
  })

  it('启动失败保留弹窗并展示服务端原文', async () => {
    mockFetch(routes({ startStatus: 500 }))
    const { wrapper } = await mountPage(DefinitionDetailPage, '/repository/definitions/def1')
    await settle()
    findButton('启动实例').click()
    await settle()
    findButton('启动').click()
    await settle()
    await settle()
    expect(wrapper.text()).toContain('挂起状态不能启动')
  })

  it('启动表单 Tab 渲染字段并提交', async () => {
    const calls = mockFetch(routes())
    const { wrapper } = await mountPage(DefinitionDetailPage, '/repository/definitions/def1')
    await settle()
    clickTab(wrapper, '启动表单')
    await settle()
    expect(wrapper.text()).toContain('请假天数')
    const input = wrapper.find('.n-input-number input')
    await input.setValue('2')
    findButton('按表单启动实例', wrapper.element).click()
    await settle()
    await settle()
    const post = calls.find((call) => call.method === 'POST' && call.url === '/service/runtime/process-instances')
    expect(post).toBeTruthy()
  })

  it('身份链接 Tab 展示候选发起人并可添加', async () => {
    const calls = mockFetch(routes())
    const { wrapper } = await mountPage(DefinitionDetailPage, '/repository/definitions/def1')
    await settle()
    clickTab(wrapper, '身份链接')
    await settle()
    await settle()
    expect(wrapper.text()).toContain('候选用户')
    expect(wrapper.text()).toContain('alice')
    expect(wrapper.text()).toContain('候选组')
    const identityInput = wrapper.findAll('input').find((item) => item.attributes('placeholder') === '用户 ID 或组 ID')
    await identityInput?.setValue('carol')
    findButton('添加', wrapper.element).click()
    await settle()
    const post = calls.find((call) => call.method === 'POST' && call.url.includes('/identitylinks'))
    expect(post?.body).toEqual({ userId: 'carol' })
  })

  it('迁移入口展示接口提示', async () => {
    mockFetch(routes())
    await mountPage(DefinitionDetailPage, '/repository/definitions/def1')
    await settle()
    findButton('迁移').click()
    await settle()
    expect(document.body.textContent).toContain('/migrate')
    expect(document.body.textContent).toContain('activityMappings')
  })

  it('定义不存在时展示 ErrorBlock', async () => {
    mockFetch(routes({ detailStatus: 404 }))
    const { wrapper } = await mountPage(DefinitionDetailPage, '/repository/definitions/def1')
    await settle()
    expect(wrapper.text()).toContain('流程定义不存在')
  })

  it('下载原始资源', async () => {
    const calls = mockFetch(routes())
    const { wrapper } = await mountPage(DefinitionDetailPage, '/repository/definitions/def1')
    await settle()
    findButton('下载原始资源', wrapper.element).click()
    await settle()
    expect(calls.some((call) => call.url === '/service/repository/process-definitions/def1/resourcedata')).toBe(true)
  })
})