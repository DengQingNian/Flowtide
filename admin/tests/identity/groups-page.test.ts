// 组列表页测试：加载、空态、错误、筛选、新建、编辑、删除、成员增删与分页。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import type { VueWrapper } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { NInput, NSelect } from 'naive-ui'
import GroupsPage from '@/pages/identity/GroupsPage.vue'
import GroupForm from '@/pages/identity/components/GroupForm.vue'
import GroupMembersPanel from '@/pages/identity/components/GroupMembersPanel.vue'
import DataTable from '@/components/DataTable.vue'
import ErrorBlock from '@/components/ErrorBlock.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import FilterBar from '@/components/FilterBar.vue'
import { authNotice } from '@/stores/ui'
import { mockFetch, pageOf, settle } from '../helpers'

/** 组数据 */
const GROUPS = [
  { id: 'g1', name: '审批组', type: 'assignment' },
  { id: 'g2', name: '测试组', type: '' },
]

// 创建路由。
// @return 路由实例
function makeRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/identity/groups', name: 'groups', component: { template: '<div />' } }],
  })
}

// 挂载组页面。
// @param path 初始路径
// @return 包装器与路由
async function mountPage(path = '/identity/groups') {
  const router = makeRouter()
  await router.push(path)
  await router.isReady()
  const wrapper = mount(GroupsPage, { global: { plugins: [router] } })
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

describe('组列表页', () => {
  it('加载并渲染组列表', async () => {
    const calls = mockFetch([{ path: /^\/service\/identity\/groups\?/, body: pageOf(GROUPS, 2) }])
    const { wrapper } = await mountPage()
    expect(wrapper.text()).toContain('审批组')
    expect(wrapper.text()).toContain('测试组')
    expect(calls[0].url).toContain('start=0')
    expect(calls[0].url).toContain('size=50')
  })

  it('无数据时展示空态', async () => {
    mockFetch([{ path: /^\/service\/identity\/groups\?/, body: pageOf([]) }])
    const { wrapper } = await mountPage()
    expect(wrapper.text()).toContain('暂无组')
  })

  it('接口错误时展示 ErrorBlock 并可重试', async () => {
    const calls = mockFetch([
      { path: /^\/service\/identity\/groups\?/, status: 500, body: { message: '组库不可用' } },
    ])
    const { wrapper } = await mountPage()
    expect(wrapper.findComponent(ErrorBlock).text()).toContain('组库不可用')
    await wrapper.findComponent(ErrorBlock).find('button[title="重试"]').trigger('click')
    await settle()
    expect(calls.length).toBeGreaterThanOrEqual(2)
  })

  it('从路由还原筛选并在搜索/重置时更新 query', async () => {
    const calls = mockFetch([{ path: /^\/service\/identity\/groups\?/, body: pageOf([]) }])
    const { wrapper, router } = await mountPage('/identity/groups?id=g1&name=%E5%AE%A1&type=assignment&page=2&size=20')
    expect(calls[0].url).toContain('id=g1')
    expect(calls[0].url).toContain('nameLike=%E5%AE%A1')
    expect(calls[0].url).toContain('type=assignment')
    expect(calls[0].url).toContain('start=20')

    const inputs = wrapper.findAllComponents(NInput)
    inputs[1].vm.$emit('update:value', '测试')
    await settle()
    await wrapper.findComponent(FilterBar).vm.$emit('search')
    await settle()
    expect(router.currentRoute.value.query.name).toBe('测试')

    await wrapper.findComponent(FilterBar).vm.$emit('reset')
    await settle()
    expect(router.currentRoute.value.query).toEqual({})
  })

  it('新建组成功并提交字段', async () => {
    const calls = mockFetch([
      { path: /^\/service\/identity\/groups\?/, body: pageOf(GROUPS, 2) },
      { path: '/service/identity/groups', method: 'POST', status: 201, body: { id: 'g3' } },
    ])
    const { wrapper } = await mountPage()
    await clickButton(wrapper, '新建组')
    const form = wrapper.findComponent(GroupForm)
    form.findAllComponents(NInput)[0].vm.$emit('update:value', 'g3')
    form.findAllComponents(NInput)[1].vm.$emit('update:value', '财务组')
    form.findAllComponents(NInput)[2].vm.$emit('update:value', 'assignment')
    await settle()
    await clickButton(form, '创建组')
    const post = calls.find((call) => call.method === 'POST')
    expect(post?.body).toEqual({ id: 'g3', name: '财务组', type: 'assignment' })
    expect(wrapper.text()).toContain('已创建组 g3')
  })

  it('新建组失败时保留表单并原文展示', async () => {
    mockFetch([
      { path: /^\/service\/identity\/groups\?/, body: pageOf([]) },
      { path: '/service/identity/groups', method: 'POST', status: 409, body: { message: "A group with id 'g3' already exists." } },
    ])
    const { wrapper } = await mountPage()
    await clickButton(wrapper, '新建组')
    const form = wrapper.findComponent(GroupForm)
    form.findAllComponents(NInput)[0].vm.$emit('update:value', 'g3')
    await settle()
    await clickButton(form, '创建组')
    expect(wrapper.findComponent(GroupForm).exists()).toBe(true)
    expect(wrapper.findComponent(GroupForm).text()).toContain("A group with id 'g3' already exists.")
  })

  it('新建组校验空 ID', async () => {
    mockFetch([{ path: /^\/service\/identity\/groups\?/, body: pageOf([]) }])
    const { wrapper } = await mountPage()
    await clickButton(wrapper, '新建组')
    await clickButton(wrapper.findComponent(GroupForm), '创建组')
    expect(wrapper.findComponent(GroupForm).text()).toContain('组 ID 不能为空。')
  })

  it('编辑组成功并提交 PUT', async () => {
    const calls = mockFetch([
      { path: /^\/service\/identity\/groups\?/, body: pageOf(GROUPS, 2) },
      { path: '/service/identity/groups/g1', method: 'PUT', body: { id: 'g1', name: '新名称' } },
    ])
    const { wrapper } = await mountPage()
    await clickButton(wrapper, '编辑')
    const form = wrapper.findComponent(GroupForm)
    expect(form.findAllComponents(NInput)[0].props('disabled')).toBe(true)
    form.findAllComponents(NInput)[1].vm.$emit('update:value', '新名称')
    await settle()
    await clickButton(form, '保存组')
    const put = calls.find((call) => call.method === 'PUT')
    expect(put?.url).toBe('/service/identity/groups/g1')
    expect(put?.body).toMatchObject({ name: '新名称' })
    expect(wrapper.text()).toContain('已更新组 g1')
  })

  it('编辑组失败时标记认证错误', async () => {
    mockFetch([
      { path: /^\/service\/identity\/groups\?/, body: pageOf(GROUPS, 2) },
      { path: '/service/identity/groups/g1', method: 'PUT', status: 403, body: { message: '无权限', code: 'REST-0403' } },
    ])
    const { wrapper } = await mountPage()
    await clickButton(wrapper, '编辑')
    await clickButton(wrapper.findComponent(GroupForm), '保存组')
    expect(wrapper.findComponent(GroupForm).text()).toContain('无权限')
    expect(authNotice.value).toBe('forbidden')
  })

  it('删除组成功并收起成员面板', async () => {
    const calls = mockFetch([
      { path: /^\/service\/identity\/groups\?/, body: pageOf(GROUPS, 2) },
      { path: '/service/identity/groups/g1/members', body: [] },
      { path: /^\/service\/identity\/users\?/, body: pageOf([]) },
      { path: '/service/identity/groups/g1', method: 'DELETE', status: 204 },
    ])
    const { wrapper } = await mountPage()
    await clickButton(wrapper, '成员')
    expect(wrapper.findComponent(GroupMembersPanel).exists()).toBe(true)
    wrapper.findAllComponents(ConfirmButton)[0].vm.$emit('confirm')
    await settle()
    expect(calls.some((call) => call.method === 'DELETE' && call.url === '/service/identity/groups/g1')).toBe(true)
    expect(wrapper.findComponent(GroupMembersPanel).exists()).toBe(false)
  })

  it('删除组失败时保留列表并原文展示', async () => {
    mockFetch([
      { path: /^\/service\/identity\/groups\?/, body: pageOf(GROUPS, 2) },
      { path: '/service/identity/groups/g1', method: 'DELETE', status: 400, body: { message: '组仍被引用' } },
    ])
    const { wrapper } = await mountPage()
    wrapper.findAllComponents(ConfirmButton)[0].vm.$emit('confirm')
    await settle()
    expect(wrapper.text()).toContain('组仍被引用')
    expect(wrapper.text()).toContain('审批组')
  })

  it('成员面板加载、选择用户加入与移除', async () => {
    const calls = mockFetch([
      { path: /^\/service\/identity\/groups\?/, body: pageOf(GROUPS, 2) },
      { path: '/service/identity/groups/g1/members', body: [{ id: 'alice', displayName: '张三', email: 'alice@example.com' }] },
      { path: /^\/service\/identity\/users\?/, body: pageOf([{ id: 'alice', displayName: '张三' }, { id: 'bob', displayName: '李四' }]) },
      { path: '/service/identity/groups/g1/members', method: 'POST', status: 201, body: { userId: 'bob', groupId: 'g1' } },
      { path: '/service/identity/groups/g1/members/alice', method: 'DELETE', status: 204 },
    ])
    const { wrapper } = await mountPage()
    await clickButton(wrapper, '成员')
    const panel = wrapper.findComponent(GroupMembersPanel)
    expect(panel.text()).toContain('张三')
    expect(panel.text()).toContain('组成员：审批组')
    panel.findComponent(NSelect).vm.$emit('update:value', 'bob')
    await settle()
    await clickButton(panel, '添加成员')
    expect(calls.some((call) => call.method === 'POST' && (call.body as { userId?: string }).userId === 'bob')).toBe(true)
    expect(panel.text()).toContain('已添加成员 bob')
    panel.findAllComponents(ConfirmButton)[0].vm.$emit('confirm')
    await settle()
    expect(calls.some((call) => call.method === 'DELETE' && call.url.endsWith('/members/alice'))).toBe(true)
    expect(panel.text()).toContain('已移除成员 alice')
    await clickButton(panel, '关闭')
    expect(wrapper.findComponent(GroupMembersPanel).exists()).toBe(false)
  })

  it('成员为空时展示空态', async () => {
    mockFetch([
      { path: /^\/service\/identity\/groups\?/, body: pageOf(GROUPS, 2) },
      { path: '/service/identity/groups/g1/members', body: [] },
      { path: /^\/service\/identity\/users\?/, body: pageOf([]) },
    ])
    const { wrapper } = await mountPage()
    await clickButton(wrapper, '成员')
    expect(wrapper.findComponent(GroupMembersPanel).text()).toContain('组内暂无成员')
  })

  it('成员接口失败时展示错误', async () => {
    mockFetch([
      { path: /^\/service\/identity\/groups\?/, body: pageOf(GROUPS, 2) },
      { path: '/service/identity/groups/g1/members', status: 500, body: { message: '成员库不可用' } },
      { path: /^\/service\/identity\/users\?/, body: pageOf([]) },
    ])
    const { wrapper } = await mountPage()
    await clickButton(wrapper, '成员')
    expect(wrapper.findComponent(GroupMembersPanel).text()).toContain('成员库不可用')
  })

  it('翻页与切换每页条数写入 query', async () => {
    const calls = mockFetch([{ path: /^\/service\/identity\/groups\?/, body: pageOf(GROUPS, 120) }])
    const { wrapper, router } = await mountPage()
    wrapper.findComponent(DataTable).vm.$emit('update:page', 2)
    await settle()
    expect(router.currentRoute.value.query.page).toBe('2')
    expect(calls[calls.length - 1].url).toContain('start=50')
    wrapper.findComponent(DataTable).vm.$emit('update:pageSize', 10)
    await settle()
    expect(router.currentRoute.value.query.size).toBe('10')
  })

  it('切换成员按钮可收起面板', async () => {
    mockFetch([
      { path: /^\/service\/identity\/groups\?/, body: pageOf(GROUPS, 2) },
      { path: '/service/identity/groups/g1/members', body: [] },
      { path: /^\/service\/identity\/users\?/, body: pageOf([]) },
    ])
    const { wrapper } = await mountPage()
    await clickButton(wrapper, '成员')
    expect(wrapper.findComponent(GroupMembersPanel).exists()).toBe(true)
    const memberButtons = wrapper.findAll('button').filter((item) => item.text().trim() === '成员')
    await memberButtons[0].trigger('click')
    await settle()
    expect(wrapper.findComponent(GroupMembersPanel).exists()).toBe(false)
  })
})
