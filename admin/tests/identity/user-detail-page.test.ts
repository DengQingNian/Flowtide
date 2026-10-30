// 用户详情页测试：资料、角色、组、信息、头像与密码面板的加载与操作。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import type { VueWrapper } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { NCheckboxGroup, NInput, NSelect, NTabs } from 'naive-ui'
import UserDetailPage from '@/pages/identity/UserDetailPage.vue'
import UserForm from '@/pages/identity/components/UserForm.vue'
import UserRolesPanel from '@/pages/identity/components/UserRolesPanel.vue'
import UserGroupsPanel from '@/pages/identity/components/UserGroupsPanel.vue'
import UserInfoPanel from '@/pages/identity/components/UserInfoPanel.vue'
import UserPicturePanel from '@/pages/identity/components/UserPicturePanel.vue'
import UserPasswordPanel from '@/pages/identity/components/UserPasswordPanel.vue'
import ErrorBlock from '@/components/ErrorBlock.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import { authNotice } from '@/stores/ui'
import { mockFetch, pageOf, settle } from '../helpers'

/** 用户数据 */
const USER = {
  id: 'alice',
  firstName: 'Alice',
  lastName: 'Zhang',
  displayName: '张三',
  email: 'alice@example.com',
  tenantId: 't1',
}

/** 角色数据 */
const ROLES = pageOf([
  { id: 'admin', name: '管理员', permissions: ['read', 'write', 'admin'] },
  { id: 'reader', name: '只读', permissions: ['read'] },
])

// 创建身份路由。
// @return 路由实例
function makeRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/identity/users', name: 'users', component: { template: '<div />' } },
      { path: '/identity/users/:id', name: 'user-detail', component: { template: '<div />' } },
    ],
  })
}

// 挂载详情页。
// @param path 初始路径
// @return 包装器与路由
async function mountPage(path = '/identity/users/alice') {
  const router = makeRouter()
  await router.push(path)
  await router.isReady()
  const wrapper = mount(UserDetailPage, { global: { plugins: [router] } })
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

// 切换 Tab。
// @param wrapper 页面包装器
// @param name Tab 名
// @return 无
async function switchTab(wrapper: VueWrapper, name: string): Promise<void> {
  wrapper.findComponent(NTabs).vm.$emit('update:value', name)
  await settle()
  await settle()
}

// 构造默认成功路由。
// @return 路由列表
function baseRoutes() {
  return [
    { path: /^\/service\/identity\/users\/alice$/, body: USER },
    { path: /^\/service\/identity\/roles\?/, body: ROLES },
  ]
}

beforeEach(() => {
  authNotice.value = 'none'
  vi.unstubAllGlobals()
})

describe('用户详情页', () => {
  it('加载并渲染用户资料，提示密码不回显', async () => {
    const calls = mockFetch(baseRoutes())
    const { wrapper } = await mountPage()
    expect(wrapper.text()).toContain('张三')
    expect(wrapper.text()).toContain('alice@example.com')
    expect(wrapper.text()).toContain('服务端不会返回密码')
    expect(calls[0].url).toBe('/service/identity/users/alice')
    expect(wrapper.findComponent(UserRolesPanel).exists()).toBe(true)
  })

  it('用户不存在时展示错误并可返回列表', async () => {
    mockFetch([{ path: '/service/identity/users/alice', status: 404, body: { message: "Could not find a user with id 'alice'." } }])
    const { wrapper } = await mountPage()
    expect(wrapper.findComponent(ErrorBlock).text()).toContain("Could not find a user with id 'alice'.")
    const back = wrapper.findAll('button').find((item) => item.text().includes('返回用户列表'))
    expect(back).toBeTruthy()
  })

  it('编辑资料成功', async () => {
    const calls = mockFetch([...baseRoutes(), { path: '/service/identity/users/alice', method: 'PUT', body: USER }])
    const { wrapper } = await mountPage()
    await clickButton(wrapper, '编辑资料')
    const form = wrapper.findComponent(UserForm)
    form.findAllComponents(NInput)[1].vm.$emit('update:value', '新显示名')
    await settle()
    await clickButton(form, '保存资料')
    const put = calls.find((call) => call.method === 'PUT')
    expect(put?.body).toMatchObject({ displayName: '新显示名' })
    expect(wrapper.text()).toContain('用户资料已更新。')
  })

  it('编辑资料失败时保留表单并标记认证错误', async () => {
    mockFetch([
      ...baseRoutes(),
      { path: '/service/identity/users/alice', method: 'PUT', status: 401, body: { message: '未认证', code: 'REST-0401' } },
    ])
    const { wrapper } = await mountPage()
    await clickButton(wrapper, '编辑资料')
    await clickButton(wrapper.findComponent(UserForm), '保存资料')
    expect(wrapper.findComponent(UserForm).text()).toContain('未认证')
    expect(authNotice.value).toBe('unauthorized')
  })

  it('删除用户成功后返回列表', async () => {
    const calls = mockFetch([...baseRoutes(), { path: '/service/identity/users/alice', method: 'DELETE', status: 204 }])
    const { wrapper, router } = await mountPage()
    wrapper.findAllComponents(ConfirmButton)[0].vm.$emit('confirm')
    await settle()
    await settle()
    expect(calls.some((call) => call.method === 'DELETE')).toBe(true)
    expect(router.currentRoute.value.path).toBe('/identity/users')
  })

  it('删除用户失败时停留并展示原文', async () => {
    mockFetch([
      ...baseRoutes(),
      { path: '/service/identity/users/alice', method: 'DELETE', status: 403, body: { message: '无权限', code: 'REST-0403' } },
    ])
    const { wrapper } = await mountPage()
    wrapper.findAllComponents(ConfirmButton)[0].vm.$emit('confirm')
    await settle()
    expect(wrapper.text()).toContain('无权限')
    expect(authNotice.value).toBe('forbidden')
  })

  it('角色分配保存成功', async () => {
    const calls = mockFetch([
      ...baseRoutes(),
      { path: '/service/identity/users/alice/roles', method: 'PUT', status: 204 },
    ])
    const { wrapper } = await mountPage()
    const panel = wrapper.findComponent(UserRolesPanel)
    panel.findComponent(NCheckboxGroup).vm.$emit('update:value', ['admin'])
    await settle()
    await clickButton(panel, '保存角色')
    const put = calls.find((call) => call.method === 'PUT')
    expect(put?.body).toEqual({ roles: ['admin'] })
    expect(panel.text()).toContain('已设置角色：admin')
    expect(panel.text()).toContain('服务端未提供“查询用户角色”的 GET 接口')
  })

  it('角色分配失败时原文展示并标记认证错误', async () => {
    mockFetch([
      ...baseRoutes(),
      { path: '/service/identity/users/alice/roles', method: 'PUT', status: 403, body: { message: '无权限', code: 'REST-0403' } },
    ])
    const { wrapper } = await mountPage()
    const panel = wrapper.findComponent(UserRolesPanel)
    await clickButton(panel, '保存角色')
    expect(panel.findComponent(ErrorBlock).text()).toContain('无权限')
    expect(authNotice.value).toBe('forbidden')
  })

  it('角色列表加载失败时展示错误且可重试', async () => {
    const calls = mockFetch([
      { path: /^\/service\/identity\/users\/alice$/, body: USER },
      { path: /^\/service\/identity\/roles\?/, status: 500, body: { message: '角色库不可用' } },
    ])
    const { wrapper } = await mountPage()
    expect(wrapper.findComponent(UserRolesPanel).text()).toContain('角色库不可用')
    await wrapper.findComponent(UserRolesPanel).findComponent(ErrorBlock).find('button[title="重试"]').trigger('click')
    await settle()
    expect(calls.filter((call) => call.url.includes('/identity/roles'))).toHaveLength(2)
  })

  it('切换所属组 Tab 后加载、加入与移除组', async () => {
    const calls = mockFetch([
      ...baseRoutes(),
      { path: '/service/identity/users/alice/groups', body: [{ id: 'g1', name: '审批组', type: 'assignment' }] },
      { path: /^\/service\/identity\/groups\?/, body: pageOf([{ id: 'g1', name: '审批组' }, { id: 'g2', name: '测试组' }]) },
      { path: '/service/identity/users/alice/groups', method: 'POST', status: 204 },
      { path: '/service/identity/users/alice/groups/g1', method: 'DELETE', status: 204 },
    ])
    const { wrapper } = await mountPage()
    await switchTab(wrapper, 'groups')
    const panel = wrapper.findComponent(UserGroupsPanel)
    expect(panel.text()).toContain('审批组')
    panel.findComponent(NSelect).vm.$emit('update:value', 'g2')
    await settle()
    await clickButton(panel, '加入组')
    expect(calls.some((call) => call.method === 'POST' && (call.body as { groupId?: string }).groupId === 'g2')).toBe(true)
    expect(panel.text()).toContain('已加入组 g2')
    panel.findAllComponents(ConfirmButton)[0].vm.$emit('confirm')
    await settle()
    expect(calls.some((call) => call.method === 'DELETE' && call.url.endsWith('/groups/g1'))).toBe(true)
    expect(panel.text()).toContain('已移出组 g1')
  })

  it('所属组加载失败时展示错误', async () => {
    mockFetch([
      ...baseRoutes(),
      { path: '/service/identity/users/alice/groups', status: 500, body: { message: '成员库不可用' } },
      { path: /^\/service\/identity\/groups\?/, body: pageOf([]) },
    ])
    const { wrapper } = await mountPage()
    await switchTab(wrapper, 'groups')
    expect(wrapper.findComponent(UserGroupsPanel).text()).toContain('成员库不可用')
  })

  it('用户信息 Tab 加载、新增、编辑与删除', async () => {
    const calls = mockFetch([
      ...baseRoutes(),
      { path: /^\/service\/identity\/users\/alice\/info$/, body: [{ key: 'dept' }] },
      { path: '/service/identity/users/alice/info/dept', body: { key: 'dept', value: '研发' } },
      { path: '/service/identity/users/alice/info', method: 'POST', status: 201, body: { key: 'title', value: '工程师' } },
      { path: '/service/identity/users/alice/info/dept', method: 'PUT', body: { key: 'dept', value: '测试' } },
      { path: '/service/identity/users/alice/info/dept', method: 'DELETE', status: 204 },
    ])
    const { wrapper } = await mountPage()
    await switchTab(wrapper, 'info')
    const panel = wrapper.findComponent(UserInfoPanel)
    expect(panel.text()).toContain('研发')

    const inputs = panel.findAllComponents(NInput)
    inputs[0].vm.$emit('update:value', 'title')
    inputs[1].vm.$emit('update:value', '工程师')
    await settle()
    await clickButton(panel, '新增')
    expect(calls.some((call) => call.method === 'POST' && call.url.endsWith('/info'))).toBe(true)
    expect(panel.text()).toContain('已新增信息 title')

    await clickButton(panel, '编辑')
    panel.findAllComponents(NInput).at(-1)?.vm.$emit('update:value', '测试')
    await settle()
    await clickButton(panel, '保存')
    expect(calls.some((call) => call.method === 'PUT' && call.url.endsWith('/info/dept'))).toBe(true)

    panel.findAllComponents(ConfirmButton)[0].vm.$emit('confirm')
    await settle()
    expect(calls.some((call) => call.method === 'DELETE' && call.url.endsWith('/info/dept'))).toBe(true)
  })

  it('用户信息新增校验空键并展示加载错误', async () => {
    mockFetch([
      ...baseRoutes(),
      { path: /^\/service\/identity\/users\/alice\/info$/, status: 500, body: { message: '信息库不可用' } },
    ])
    const { wrapper } = await mountPage()
    await switchTab(wrapper, 'info')
    const panel = wrapper.findComponent(UserInfoPanel)
    expect(panel.text()).toContain('信息库不可用')
    await clickButton(panel, '新增')
    expect(panel.text()).toContain('信息键不能为空。')
  })

  it('头像 Tab 无头像时回退首字母，上传成功后刷新', async () => {
    let pictureCalls = 0
    const calls = mockFetch([
      ...baseRoutes(),
      {
        path: '/service/identity/users/alice/picture',
        status: 404,
        body: { message: "The user with id 'alice' does not have a picture." },
      },
    ])
    const { wrapper } = await mountPage()
    await switchTab(wrapper, 'picture')
    const panel = wrapper.findComponent(UserPicturePanel)
    expect(panel.text()).toContain('未设置头像')
    expect(panel.text()).toContain('张')

    // 上传使用独立路由，避免与首次 404 读取冲突。
    vi.unstubAllGlobals()
    mockFetch([{ path: '/service/identity/users/alice/picture', method: 'PUT', status: 204 }])
    const input = panel.find('input[data-test="picture-input"]')
    const file = new File(['avatar'], 'avatar.png', { type: 'image/png' })
    Object.defineProperty(input.element, 'files', { value: [file], configurable: true })
    await input.trigger('change')
    await settle()
    await settle()
    pictureCalls += calls.length
    expect(pictureCalls).toBeGreaterThan(0)
    expect(panel.text()).toContain('头像已更新')
  })

  it('头像读取成功时展示图片预览', async () => {
    mockFetch([
      ...baseRoutes(),
      { path: '/service/identity/users/alice/picture', text: 'PNGDATA', contentType: 'image/png' },
    ])
    const { wrapper } = await mountPage()
    await switchTab(wrapper, 'picture')
    const panel = wrapper.findComponent(UserPicturePanel)
    expect(panel.text()).toContain('当前使用上传的头像。')
  })

  it('头像上传失败时展示原文', async () => {
    mockFetch([
      ...baseRoutes(),
      { path: '/service/identity/users/alice/picture', status: 404, body: { message: 'no picture' } },
      { path: '/service/identity/users/alice/picture', method: 'PUT', status: 400, body: { message: '缺少上传文件' } },
    ])
    const { wrapper } = await mountPage()
    await switchTab(wrapper, 'picture')
    const panel = wrapper.findComponent(UserPicturePanel)
    const input = panel.find('input[data-test="picture-input"]')
    Object.defineProperty(input.element, 'files', { value: [new File(['x'], 'x.png', { type: 'image/png' })], configurable: true })
    await input.trigger('change')
    await settle()
    expect(panel.text()).toContain('缺少上传文件')
  })

  it('头像读取失败（非 404）时展示 ErrorBlock', async () => {
    mockFetch([
      ...baseRoutes(),
      { path: '/service/identity/users/alice/picture', status: 500, body: { message: '图片服务不可用' } },
    ])
    const { wrapper } = await mountPage()
    await switchTab(wrapper, 'picture')
    expect(wrapper.findComponent(UserPicturePanel).text()).toContain('图片服务不可用')
  })

  it('密码重置校验一致性并提交成功', async () => {
    const calls = mockFetch([
      ...baseRoutes(),
      { path: '/service/identity/users/alice/picture', status: 404, body: { message: 'no picture' } },
      { path: '/service/identity/users/alice/password', method: 'PUT', status: 204 },
    ])
    const { wrapper } = await mountPage()
    await switchTab(wrapper, 'picture')
    const panel = wrapper.findComponent(UserPasswordPanel)
    const inputs = panel.findAllComponents(NInput)
    inputs[0].vm.$emit('update:value', 'new-secret')
    inputs[1].vm.$emit('update:value', 'other')
    await settle()
    await clickButton(panel, '重置密码')
    expect(panel.text()).toContain('两次输入的密码不一致。')

    inputs[1].vm.$emit('update:value', 'new-secret')
    await settle()
    await clickButton(panel, '重置密码')
    const put = calls.find((call) => call.method === 'PUT' && call.url.endsWith('/password'))
    expect(put?.body).toEqual({ newPassword: 'new-secret' })
    expect(panel.text()).toContain('密码已重置')
  })

  it('密码为空时提示且不提交', async () => {
    mockFetch([...baseRoutes(), { path: '/service/identity/users/alice/picture', status: 404, body: { message: 'no picture' } }])
    const { wrapper } = await mountPage()
    await switchTab(wrapper, 'picture')
    const panel = wrapper.findComponent(UserPasswordPanel)
    await clickButton(panel, '重置密码')
    expect(panel.text()).toContain('新密码不能为空。')
  })

  it('密码重置失败时原文展示并标记认证错误', async () => {
    mockFetch([
      ...baseRoutes(),
      { path: '/service/identity/users/alice/picture', status: 404, body: { message: 'no picture' } },
      { path: '/service/identity/users/alice/password', method: 'PUT', status: 403, body: { message: '无权限', code: 'REST-0403' } },
    ])
    const { wrapper } = await mountPage()
    await switchTab(wrapper, 'picture')
    const panel = wrapper.findComponent(UserPasswordPanel)
    const inputs = panel.findAllComponents(NInput)
    inputs[0].vm.$emit('update:value', 'x')
    inputs[1].vm.$emit('update:value', 'x')
    await settle()
    await clickButton(panel, '重置密码')
    expect(panel.text()).toContain('无权限')
    expect(authNotice.value).toBe('forbidden')
  })
})
