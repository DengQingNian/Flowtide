// 用户列表页测试：加载、空态、错误、筛选、新建、编辑、删除与分页。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { NInput } from 'naive-ui'
import UsersPage from '@/pages/identity/UsersPage.vue'
import DataTable from '@/components/DataTable.vue'
import ErrorBlock from '@/components/ErrorBlock.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import UserForm from '@/pages/identity/components/UserForm.vue'
import FilterBar from '@/components/FilterBar.vue'
import { authNotice } from '@/stores/ui'
import { mockFetch, pageOf, settle } from '../helpers'

// 创建带身份路由的内存路由。
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

// 在指定路径挂载页面。
// @param path 初始路径
// @return 包装器与路由
async function mountPage(path = '/identity/users') {
  const router = makeRouter()
  await router.push(path)
  await router.isReady()
  const wrapper = mount(UsersPage, { global: { plugins: [router] } })
  await settle()
  return { wrapper, router }
}

// 点击文本匹配的按钮。
// @param wrapper 包装器
// @param text 按钮文本
// @return 无
async function clickButton(wrapper: ReturnType<typeof mount>, text: string): Promise<void> {
  const button = wrapper.findAll('button').find((item) => item.text().includes(text))
  if (!button) throw new Error(`未找到按钮：${text}`)
  await button.trigger('click')
  await settle()
}

const USERS = [
  { id: 'alice', firstName: 'Alice', lastName: 'Zhang', displayName: '张三', email: 'alice@example.com', tenantId: 't1' },
  { id: 'bob', firstName: 'Bob', displayName: '李四', email: 'bob@example.com' },
]

beforeEach(() => {
  authNotice.value = 'none'
  vi.unstubAllGlobals()
})

describe('用户列表页', () => {
  it('加载并渲染用户、头像首字母与分页参数', async () => {
    const calls = mockFetch([{ path: /^\/service\/identity\/users\?/, body: pageOf(USERS, 2) }])
    const { wrapper } = await mountPage()
    expect(wrapper.text()).toContain('张三')
    expect(wrapper.text()).toContain('李四')
    expect(wrapper.text()).toContain('alice@example.com')
    expect(wrapper.findAllComponents(UserForm)).toHaveLength(0)
    expect(calls[0].url).toContain('start=0')
    expect(calls[0].url).toContain('size=50')
    expect(calls[0].url).toContain('sort=id')
  })

  it('无数据时展示空态说明', async () => {
    mockFetch([{ path: /^\/service\/identity\/users\?/, body: pageOf([]) }])
    const { wrapper } = await mountPage()
    expect(wrapper.text()).toContain('暂无用户')
  })

  it('接口错误时展示 ErrorBlock 并可重试', async () => {
    const calls = mockFetch([
      { path: /^\/service\/identity\/users\?/, status: 500, body: { message: '身份库不可用', code: 'ENGINE-5000' } },
    ])
    const { wrapper } = await mountPage()
    expect(wrapper.findComponent(ErrorBlock).text()).toContain('身份库不可用')
    await wrapper.findComponent(ErrorBlock).find('button[title="重试"]').trigger('click')
    await settle()
    expect(calls.length).toBeGreaterThanOrEqual(2)
  })

  it('从路由还原筛选与分页，并在搜索/重置时更新 query', async () => {
    const calls = mockFetch([{ path: /^\/service\/identity\/users\?/, body: pageOf([]) }])
    const { wrapper, router } = await mountPage('/identity/users?id=alice&name=Ali&email=a%40x.com&page=2&size=20')
    expect(calls[0].url).toContain('id=alice')
    expect(calls[0].url).toContain('displayNameLike=Ali')
    expect(calls[0].url).toContain('emailLike=a%40x.com')
    expect(calls[0].url).toContain('start=20')
    expect(calls[0].url).toContain('size=20')

    const inputs = wrapper.findAllComponents(NInput)
    inputs[0].vm.$emit('update:value', 'bob')
    inputs[1].vm.$emit('update:value', 'B')
    inputs[2].vm.$emit('update:value', 'b@x.com')
    await settle()
    await wrapper.findComponent(FilterBar).vm.$emit('search')
    await settle()
    expect(router.currentRoute.value.query).toMatchObject({ id: 'bob', name: 'B', email: 'b@x.com' })
    expect(calls[calls.length - 1].url).toContain('start=0')

    await wrapper.findComponent(FilterBar).vm.$emit('reset')
    await settle()
    expect(router.currentRoute.value.query).toEqual({})
    expect(calls[calls.length - 1].url).toContain('size=50')
  })

  it('新建用户成功并提交表单字段', async () => {
    const calls = mockFetch([
      { path: /^\/service\/identity\/users\?/, body: pageOf(USERS, 2) },
      { path: '/service/identity/users', method: 'POST', status: 201, body: { id: 'carol' } },
    ])
    const { wrapper } = await mountPage()
    await clickButton(wrapper, '新建用户')
    expect(wrapper.findComponent(UserForm).exists()).toBe(true)
    const form = wrapper.findComponent(UserForm)
    const inputs = form.findAllComponents(NInput)
    inputs[0].vm.$emit('update:value', 'carol')
    inputs[1].vm.$emit('update:value', '王五')
    inputs[2].vm.$emit('update:value', 'Carol')
    inputs[3].vm.$emit('update:value', 'Wang')
    inputs[4].vm.$emit('update:value', 'carol@example.com')
    inputs[5].vm.$emit('update:value', 'secret')
    await settle()
    await clickButton(form, '创建用户')
    const post = calls.find((call) => call.method === 'POST')
    expect(post?.body).toMatchObject({ id: 'carol', displayName: '王五', password: 'secret' })
    expect(wrapper.text()).toContain('已创建用户 carol')
    expect(wrapper.findComponent(UserForm).exists()).toBe(false)
  })

  it('新建用户失败时保留表单并原文展示错误', async () => {
    mockFetch([
      { path: /^\/service\/identity\/users\?/, body: pageOf([]) },
      { path: '/service/identity/users', method: 'POST', status: 409, body: { message: "A user with id 'carol' already exists." } },
    ])
    const { wrapper } = await mountPage()
    await clickButton(wrapper, '新建用户')
    const form = wrapper.findComponent(UserForm)
    form.findAllComponents(NInput)[0].vm.$emit('update:value', 'carol')
    await settle()
    await clickButton(form, '创建用户')
    expect(wrapper.findComponent(UserForm).exists()).toBe(true)
    expect(wrapper.findComponent(UserForm).findComponent(ErrorBlock).text()).toContain("A user with id 'carol' already exists.")
    expect(wrapper.findComponent(UserForm).findAllComponents(NInput)[0].props('value')).toBe('carol')
  })

  it('新建时校验必填 ID', async () => {
    mockFetch([{ path: /^\/service\/identity\/users\?/, body: pageOf([]) }])
    const { wrapper } = await mountPage()
    await clickButton(wrapper, '新建用户')
    await clickButton(wrapper.findComponent(UserForm), '创建用户')
    expect(wrapper.findComponent(UserForm).text()).toContain('用户 ID 不能为空。')
  })

  it('编辑用户成功并提交 PUT', async () => {
    const calls = mockFetch([
      { path: /^\/service\/identity\/users\?/, body: pageOf(USERS, 2) },
      { path: '/service/identity/users/alice', method: 'PUT', body: { id: 'alice', displayName: '新名字' } },
    ])
    const { wrapper } = await mountPage()
    await clickButton(wrapper, '编辑')
    const form = wrapper.findComponent(UserForm)
    expect(form.findComponent(NInput).props('disabled')).toBe(true)
    form.findAllComponents(NInput)[1].vm.$emit('update:value', '新名字')
    await settle()
    await clickButton(form, '保存资料')
    const put = calls.find((call) => call.method === 'PUT')
    expect(put?.url).toBe('/service/identity/users/alice')
    expect(put?.body).toMatchObject({ displayName: '新名字' })
    expect(wrapper.text()).toContain('已更新用户 alice')
  })

  it('编辑失败时保留表单并标记认证错误', async () => {
    mockFetch([
      { path: /^\/service\/identity\/users\?/, body: pageOf(USERS, 2) },
      { path: '/service/identity/users/alice', method: 'PUT', status: 403, body: { message: '无权限', code: 'REST-0403' } },
    ])
    const { wrapper } = await mountPage()
    await clickButton(wrapper, '编辑')
    await clickButton(wrapper.findComponent(UserForm), '保存资料')
    expect(wrapper.findComponent(UserForm).text()).toContain('无权限')
    expect(authNotice.value).toBe('forbidden')
  })

  it('删除用户成功并刷新列表', async () => {
    const calls = mockFetch([
      { path: /^\/service\/identity\/users\?/, body: pageOf(USERS, 2) },
      { path: '/service/identity/users/alice', method: 'DELETE', status: 204 },
    ])
    const { wrapper } = await mountPage()
    const confirm = wrapper.findAllComponents(ConfirmButton)[0]
    expect(confirm.props('description')).toContain('alice')
    confirm.vm.$emit('confirm')
    await settle()
    expect(calls.some((call) => call.method === 'DELETE' && call.url === '/service/identity/users/alice')).toBe(true)
    expect(wrapper.text()).toContain('已删除用户 alice')
  })

  it('删除失败时保留列表并展示原文', async () => {
    mockFetch([
      { path: /^\/service\/identity\/users\?/, body: pageOf(USERS, 2) },
      { path: '/service/identity/users/alice', method: 'DELETE', status: 401, body: { message: '未认证', code: 'REST-0401' } },
    ])
    const { wrapper } = await mountPage()
    wrapper.findAllComponents(ConfirmButton)[0].vm.$emit('confirm')
    await settle()
    expect(wrapper.text()).toContain('未认证')
    expect(wrapper.text()).toContain('张三')
    expect(authNotice.value).toBe('unauthorized')
  })

  it('翻页与切换每页条数写入 query 并重新加载', async () => {
    const calls = mockFetch([{ path: /^\/service\/identity\/users\?/, body: pageOf(USERS, 120) }])
    const { wrapper, router } = await mountPage()
    wrapper.findComponent(DataTable).vm.$emit('update:page', 2)
    await settle()
    expect(router.currentRoute.value.query.page).toBe('2')
    expect(calls[calls.length - 1].url).toContain('start=50')
    wrapper.findComponent(DataTable).vm.$emit('update:pageSize', 20)
    await settle()
    expect(router.currentRoute.value.query).toMatchObject({ size: '20' })
    expect(calls[calls.length - 1].url).toContain('start=0')
  })

  it('取消表单不发起请求', async () => {
    const calls = mockFetch([{ path: /^\/service\/identity\/users\?/, body: pageOf([]) }])
    const { wrapper } = await mountPage()
    await clickButton(wrapper, '新建用户')
    await clickButton(wrapper.findComponent(UserForm), '取消')
    expect(wrapper.findComponent(UserForm).exists()).toBe(false)
    expect(calls.filter((call) => call.method === 'POST')).toHaveLength(0)
  })
})
