// 角色列表页测试：加载、空态、错误、新建、编辑、删除与内置角色错误展示。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import type { VueWrapper } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { NCheckboxGroup, NInput } from 'naive-ui'
import RolesPage from '@/pages/identity/RolesPage.vue'
import RoleForm from '@/pages/identity/components/RoleForm.vue'
import DataTable from '@/components/DataTable.vue'
import ErrorBlock from '@/components/ErrorBlock.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import StatusTag from '@/components/StatusTag.vue'
import { authNotice } from '@/stores/ui'
import { mockFetch, pageOf, settle } from '../helpers'

/** 角色数据 */
const ROLES = [
  { id: 'admin', name: '管理员', permissions: ['read', 'write', 'admin'] },
  { id: 'reader', name: '只读', permissions: ['read'] },
]

// 创建路由。
// @return 路由实例
function makeRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/identity/roles', name: 'roles', component: { template: '<div />' } }],
  })
}

// 挂载角色页面。
// @param path 初始路径
// @return 包装器与路由
async function mountPage(path = '/identity/roles') {
  const router = makeRouter()
  await router.push(path)
  await router.isReady()
  const wrapper = mount(RolesPage, { global: { plugins: [router] } })
  await settle()
  return { wrapper, router }
}

// 点击包含指定文本的按钮。
// @param wrapper 包装器
// @param text 按钮文本
// @return 无
async function clickButton(wrapper: VueWrapper, text: string): Promise<void> {
  const button = wrapper.findAll('button').find((item) => item.text().includes(text))
  if (!button) throw new Error(`未找到按钮：${text}`)
  await button.trigger('click')
  await settle()
}

beforeEach(() => {
  authNotice.value = 'none'
  vi.unstubAllGlobals()
})

describe('角色列表页', () => {
  it('加载角色并按真实 permissions 字段展示权限', async () => {
    const calls = mockFetch([{ path: /^\/service\/identity\/roles\?/, body: pageOf(ROLES, 2) }])
    const { wrapper } = await mountPage()
    expect(wrapper.text()).toContain('管理员')
    expect(wrapper.text()).toContain('只读')
    const tags = wrapper.findAllComponents(StatusTag).map((item) => item.text())
    expect(tags).toContain('read')
    expect(tags).toContain('write')
    expect(tags).toContain('admin')
    expect(calls[0].url).toContain('start=0')
    expect(calls[0].url).toContain('size=50')
  })

  it('无数据时展示空态', async () => {
    mockFetch([{ path: /^\/service\/identity\/roles\?/, body: pageOf([]) }])
    const { wrapper } = await mountPage()
    expect(wrapper.text()).toContain('暂无角色')
  })

  it('接口错误时展示 ErrorBlock 并可重试', async () => {
    const calls = mockFetch([
      { path: /^\/service\/identity\/roles\?/, status: 500, body: { message: '角色库不可用' } },
    ])
    const { wrapper } = await mountPage()
    expect(wrapper.findComponent(ErrorBlock).text()).toContain('角色库不可用')
    await wrapper.findComponent(ErrorBlock).find('button[title="重试"]').trigger('click')
    await settle()
    expect(calls.length).toBeGreaterThanOrEqual(2)
  })

  it('从路由还原分页并响应翻页', async () => {
    const calls = mockFetch([{ path: /^\/service\/identity\/roles\?/, body: pageOf(ROLES, 120) }])
    const { wrapper, router } = await mountPage('/identity/roles?page=2&size=20')
    expect(calls[0].url).toContain('start=20')
    expect(calls[0].url).toContain('size=20')
    wrapper.findComponent(DataTable).vm.$emit('update:page', 3)
    await settle()
    expect(router.currentRoute.value.query.page).toBe('3')
    expect(calls[calls.length - 1].url).toContain('start=40')
    wrapper.findComponent(DataTable).vm.$emit('update:pageSize', 10)
    await settle()
    expect(router.currentRoute.value.query).toEqual({ size: '10' })
  })

  it('新建角色成功并提交权限', async () => {
    const calls = mockFetch([
      { path: /^\/service\/identity\/roles\?/, body: pageOf(ROLES, 2) },
      { path: '/service/identity/roles', method: 'POST', status: 201, body: { id: 'writer' } },
    ])
    const { wrapper } = await mountPage()
    await clickButton(wrapper, '新建角色')
    const form = wrapper.findComponent(RoleForm)
    const inputs = form.findAllComponents(NInput)
    inputs[0].vm.$emit('update:value', 'writer')
    inputs[1].vm.$emit('update:value', '写入者')
    form.findComponent(NCheckboxGroup).vm.$emit('update:value', ['read', 'write'])
    await settle()
    await clickButton(form, '创建角色')
    const post = calls.find((call) => call.method === 'POST')
    expect(post?.body).toEqual({ id: 'writer', name: '写入者', permissions: ['read', 'write'] })
    expect(wrapper.text()).toContain('已创建角色 writer')
  })

  it('新建角色失败时保留表单并原文展示', async () => {
    mockFetch([
      { path: /^\/service\/identity\/roles\?/, body: pageOf([]) },
      { path: '/service/identity/roles', method: 'POST', status: 409, body: { message: '角色已存在' } },
    ])
    const { wrapper } = await mountPage()
    await clickButton(wrapper, '新建角色')
    const form = wrapper.findComponent(RoleForm)
    form.findAllComponents(NInput)[0].vm.$emit('update:value', 'writer')
    await settle()
    await clickButton(form, '创建角色')
    expect(wrapper.findComponent(RoleForm).exists()).toBe(true)
    expect(wrapper.findComponent(RoleForm).text()).toContain('角色已存在')
  })

  it('新建角色校验空 ID', async () => {
    mockFetch([{ path: /^\/service\/identity\/roles\?/, body: pageOf([]) }])
    const { wrapper } = await mountPage()
    await clickButton(wrapper, '新建角色')
    await clickButton(wrapper.findComponent(RoleForm), '创建角色')
    expect(wrapper.findComponent(RoleForm).text()).toContain('角色 ID 不能为空。')
  })

  it('编辑角色为全量替换并提示额外权限', async () => {
    const calls = mockFetch([
      {
        path: /^\/service\/identity\/roles\?/,
        body: pageOf([{ id: 'reader', name: '只读', permissions: ['read', 'custom'] }]),
      },
      { path: '/service/identity/roles/reader', method: 'PUT', body: { id: 'reader', name: '新只读' } },
    ])
    const { wrapper } = await mountPage()
    await clickButton(wrapper, '编辑')
    const form = wrapper.findComponent(RoleForm)
    expect(form.text()).toContain('额外权限')
    expect(form.text()).toContain('custom')
    form.findAllComponents(NInput)[1].vm.$emit('update:value', '新只读')
    form.findComponent(NCheckboxGroup).vm.$emit('update:value', ['read'])
    await settle()
    await clickButton(form, '保存角色')
    const put = calls.find((call) => call.method === 'PUT')
    expect(put?.url).toBe('/service/identity/roles/reader')
    expect(put?.body).toEqual({ name: '新只读', permissions: ['read'] })
    expect(wrapper.text()).toContain('已更新角色 reader')
  })

  it('编辑失败时标记认证错误', async () => {
    mockFetch([
      { path: /^\/service\/identity\/roles\?/, body: pageOf(ROLES, 2) },
      { path: '/service/identity/roles/admin', method: 'PUT', status: 401, body: { message: '未认证', code: 'REST-0401' } },
    ])
    const { wrapper } = await mountPage()
    await clickButton(wrapper, '编辑')
    await clickButton(wrapper.findComponent(RoleForm), '保存角色')
    expect(wrapper.findComponent(RoleForm).text()).toContain('未认证')
    expect(authNotice.value).toBe('unauthorized')
  })

  it('删除角色成功并刷新列表', async () => {
    const calls = mockFetch([
      { path: /^\/service\/identity\/roles\?/, body: pageOf(ROLES, 2) },
      { path: '/service/identity/roles/admin', method: 'DELETE', status: 204 },
    ])
    const { wrapper } = await mountPage()
    wrapper.findAllComponents(ConfirmButton)[0].vm.$emit('confirm')
    await settle()
    expect(calls.some((call) => call.method === 'DELETE' && call.url === '/service/identity/roles/admin')).toBe(true)
    expect(wrapper.text()).toContain('已删除角色 admin')
  })

  it('内置角色删除失败时原样展示服务端错误且保留列表', async () => {
    mockFetch([
      { path: /^\/service\/identity\/roles\?/, body: pageOf(ROLES, 2) },
      {
        path: '/service/identity/roles/admin',
        method: 'DELETE',
        status: 400,
        body: { message: '内置角色不可删除', code: 'ENGINE-1007', exception: 'builtin role guard' },
      },
    ])
    const { wrapper } = await mountPage()
    wrapper.findAllComponents(ConfirmButton)[0].vm.$emit('confirm')
    await settle()
    expect(wrapper.text()).toContain('内置角色不可删除')
    expect(wrapper.text()).toContain('ENGINE-1007')
    expect(wrapper.text()).toContain('管理员')
    expect(authNotice.value).toBe('none')
  })

  it('删除角色触发 403 时标记认证错误', async () => {
    mockFetch([
      { path: /^\/service\/identity\/roles\?/, body: pageOf(ROLES, 2) },
      { path: '/service/identity/roles/admin', method: 'DELETE', status: 403, body: { message: '无权限', code: 'REST-0403' } },
    ])
    const { wrapper } = await mountPage()
    wrapper.findAllComponents(ConfirmButton)[0].vm.$emit('confirm')
    await settle()
    expect(wrapper.text()).toContain('无权限')
    expect(authNotice.value).toBe('forbidden')
  })

  it('取消表单不发起写请求', async () => {
    const calls = mockFetch([{ path: /^\/service\/identity\/roles\?/, body: pageOf([]) }])
    const { wrapper } = await mountPage()
    await clickButton(wrapper, '新建角色')
    await clickButton(wrapper.findComponent(RoleForm), '取消')
    expect(wrapper.findComponent(RoleForm).exists()).toBe(false)
    expect(calls.filter((call) => call.method === 'POST')).toHaveLength(0)
  })
})
