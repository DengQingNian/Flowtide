import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { NButton, NInput, NInputNumber, NSelect } from 'naive-ui'
import VariablesTab from '@/pages/runtime/components/instance/VariablesTab.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import { mockFetch, settle } from '../../helpers'
import type { MockRoute } from '../../helpers'

const vars = [
  { name: 'amount', type: 'long', value: 100, scope: 'global' },
  { name: 'note', type: 'string', value: 'hello', scope: 'local', transient: true },
]

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
  document.body.innerHTML = ''
})

// 按文本查找 Naive 按钮。
// @param wrapper 包装器
// @param text 文本
// @return 按钮组件
function nbutton(wrapper: ReturnType<typeof mount>, text: string) {
  const found = wrapper.findAllComponents(NButton).find((item) => item.text() === text)
  if (!found) throw new Error(`未找到按钮：${text}`)
  return found
}

// 构造默认路由。
// @param overrides 覆盖项
// @return 路由数组
function routes(): MockRoute[] {
  return [
    { path: '/service/runtime/process-instances/pi-1/variables/note/data', text: '{"amount":100}', contentType: 'application/octet-stream' },
    { path: '/service/runtime/process-instances/pi-1/variables/amount', method: 'PUT', status: 204 },
    { path: '/service/runtime/process-instances/pi-1/variables/amount', method: 'DELETE', status: 204 },
    { path: '/service/runtime/process-instances/pi-1/variables', method: 'POST', status: 201, body: { name: 'added', value: 1 } },
    { path: '/service/runtime/process-instances/pi-1/variables', body: vars },
  ]
}

describe('VariablesTab', () => {
  it('加载变量、作用域与瞬态提示', async () => {
    mockFetch(routes())
    const wrapper = mount(VariablesTab, { props: { instanceId: 'pi-1' } })
    await settle()
    expect(wrapper.text()).toContain('amount')
    expect(wrapper.text()).toContain('100')
    expect(wrapper.text()).toContain('瞬态变量')
    expect(wrapper.text()).toContain('note')
  })

  it('按名称过滤变量', async () => {
    mockFetch(routes())
    const wrapper = mount(VariablesTab, { props: { instanceId: 'pi-1' } })
    await settle()
    const input = wrapper.find('input[placeholder="按变量名搜索"]')
    await input.setValue('note')
    expect(wrapper.text()).toContain('note')
    expect(wrapper.text()).not.toContain('amount')
  })

  it('新增变量后刷新列表并提示', async () => {
    const calls = mockFetch(routes())
    const wrapper = mount(VariablesTab, { props: { instanceId: 'pi-1' } })
    await settle()
    await nbutton(wrapper, '新增变量').trigger('click')
    const nameInput = wrapper.findAllComponents(NInput).find((item) => item.props('placeholder') === '例如 orderId')!
    const valueInput = wrapper.findAllComponents(NInput).find((item) => item.props('placeholder') === '字符串值')!
    await nameInput.vm.$emit('update:value', 'added')
    await valueInput.vm.$emit('update:value', 'x')
    await nbutton(wrapper, '保存').trigger('click')
    await settle()
    const post = calls.find((call) => call.method === 'POST')
    expect(post?.body).toMatchObject({ name: 'added', type: 'string', value: 'x' })
    expect(wrapper.text()).toContain('已新增变量 added')
  })

  it('编辑变量按名称提交新值', async () => {
    const calls = mockFetch(routes())
    const wrapper = mount(VariablesTab, { props: { instanceId: 'pi-1' } })
    await settle()
    const edit = wrapper.findAll('button').find((item) => item.text() === '编辑')!
    await edit.trigger('click')
    const number = wrapper.findComponent(NInputNumber)
    await number.vm.$emit('update:value', 200)
    await nbutton(wrapper, '保存').trigger('click')
    await settle()
    const put = calls.find((call) => call.method === 'PUT')
    expect(put?.url).toBe('/service/runtime/process-instances/pi-1/variables/amount')
    expect(put?.body).toEqual({ name: 'amount', type: 'long', value: 200 })
    expect(wrapper.text()).toContain('已更新变量 amount')
  })

  it('删除变量需要确认后提交', async () => {
    const calls = mockFetch(routes())
    const wrapper = mount(VariablesTab, { props: { instanceId: 'pi-1' } })
    await settle()
    const remove = wrapper.findAll('button').find((item) => item.text() === '删除')!
    await remove.trigger('click')
    wrapper.findAllComponents(ConfirmButton)[0].vm.$emit('confirm')
    await settle()
    const del = calls.find((call) => call.method === 'DELETE')
    expect(del?.url).toBe('/service/runtime/process-instances/pi-1/variables/amount')
    expect(wrapper.text()).toContain('已删除变量 amount')
  })

  it('选择变量查看并下载原始数据', async () => {
    const createObjectURL = vi.fn(() => 'blob:mock')
    Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: createObjectURL })
    const calls = mockFetch(routes())
    const wrapper = mount(VariablesTab, { props: { instanceId: 'pi-1' } })
    await settle()
    await wrapper.findComponent(NSelect).vm.$emit('update:value', 'note')
    await nbutton(wrapper, '查看原始数据').trigger('click')
    await settle()
    expect(calls.some((call) => call.url.endsWith('/variables/note/data'))).toBe(true)
    expect(document.body.textContent).toContain('"amount":100')
    const download = wrapper.findAllComponents(NButton).find((item) => item.text() === '下载')!
    await download.trigger('click')
    expect(createObjectURL).toHaveBeenCalled()
  })

  it('加载失败展示 ErrorBlock 并可重试', async () => {
    mockFetch([
      { path: '/service/runtime/process-instances/pi-1/variables', status: 500, body: { message: '变量读取失败' } },
    ])
    const wrapper = mount(VariablesTab, { props: { instanceId: 'pi-1' } })
    await settle()
    expect(wrapper.text()).toContain('变量读取失败')
    expect(wrapper.find('button[title="重试"]').exists()).toBe(true)
  })

  it('写入失败保留页面并展示原文', async () => {
    mockFetch([
      { path: '/service/runtime/process-instances/pi-1/variables', method: 'POST', status: 409, body: { message: '变量已存在' } },
      { path: '/service/runtime/process-instances/pi-1/variables', body: vars },
    ])
    const wrapper = mount(VariablesTab, { props: { instanceId: 'pi-1' } })
    await settle()
    await nbutton(wrapper, '新增变量').trigger('click')
    const nameInput = wrapper.findAllComponents(NInput).find((item) => item.props('placeholder') === '例如 orderId')!
    await nameInput.vm.$emit('update:value', 'amount')
    await nbutton(wrapper, '保存').trigger('click')
    await settle()
    expect(wrapper.text()).toContain('变量已存在')
    expect(wrapper.text()).toContain('amount')
  })
})