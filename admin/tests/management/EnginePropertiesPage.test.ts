// 引擎属性页测试：系统属性只读、新增、编辑、删除与错误提示。
import { afterEach, describe, expect, it, vi } from 'vitest'
import { mockFetch, settle } from '../helpers'
import EnginePropertiesPage from '@/pages/management/EnginePropertiesPage.vue'
import { buttonByText, confirmByLabel, mountPage } from './pageUtils'

afterEach(() => {
  vi.unstubAllGlobals()
})

/** 属性映射 */
const PROPERTIES = { 'schema.version': '8.1.0.3', 'engine.version': '0.1.0', 'feature.flag': 'on' }

describe('EnginePropertiesPage', () => {
  it('系统属性只读，自定义属性可编辑删除', async () => {
    mockFetch([{ method: 'GET', path: '/service/management/engine-properties', body: PROPERTIES }])
    const { wrapper } = await mountPage(EnginePropertiesPage, '/management/engine')
    await settle()
    expect(wrapper.text()).toContain('schema.version')
    expect(wrapper.text()).toContain('feature.flag')
    expect(wrapper.text()).toContain('系统属性')
    expect(wrapper.text()).toContain('只读')
    expect(wrapper.findAll('button').filter((button) => button.text().includes('编辑'))).toHaveLength(1)
  })

  it('新增自定义属性', async () => {
    const calls = mockFetch([
      { method: 'POST', path: '/service/management/engine-properties', status: 201, body: { name: 'feature.new', value: '1' } },
      { method: 'GET', path: '/service/management/engine-properties', body: PROPERTIES },
    ])
    const { wrapper } = await mountPage(EnginePropertiesPage, '/management/engine')
    await settle()
    const inputs = wrapper.findAll('input')
    await inputs[0].setValue('feature.new')
    await inputs[1].setValue('1')
    await buttonByText(wrapper, '新增属性')!.trigger('click')
    await settle()
    const post = calls.find((call) => call.method === 'POST')
    expect(post?.body).toEqual({ name: 'feature.new', value: '1' })
    expect(wrapper.text()).toContain('属性 feature.new 已创建')
  })

  it('空属性名被前端拒绝', async () => {
    const calls = mockFetch([{ method: 'GET', path: '/service/management/engine-properties', body: PROPERTIES }])
    const { wrapper } = await mountPage(EnginePropertiesPage, '/management/engine')
    await settle()
    await buttonByText(wrapper, '新增属性')!.trigger('click')
    await settle()
    expect(wrapper.text()).toContain('属性名不能为空')
    expect(calls.some((call) => call.method === 'POST')).toBe(false)
  })

  it('新增失败展示服务端原文并保留输入', async () => {
    mockFetch([
      { method: 'POST', path: '/service/management/engine-properties', status: 409, body: { code: 'ENGINE-1802', message: '属性已存在' } },
      { method: 'GET', path: '/service/management/engine-properties', body: PROPERTIES },
    ])
    const { wrapper } = await mountPage(EnginePropertiesPage, '/management/engine')
    await settle()
    const inputs = wrapper.findAll('input')
    await inputs[0].setValue('feature.flag')
    await inputs[1].setValue('off')
    await buttonByText(wrapper, '新增属性')!.trigger('click')
    await settle()
    expect(wrapper.text()).toContain('属性已存在')
    expect((inputs[0].element as HTMLInputElement).value).toBe('feature.flag')
  })

  it('编辑自定义属性并提交 PUT', async () => {
    const calls = mockFetch([
      { method: 'PUT', path: '/service/management/engine-properties/feature.flag', body: { name: 'feature.flag', value: 'off' } },
      { method: 'GET', path: '/service/management/engine-properties', body: PROPERTIES },
    ])
    const { wrapper } = await mountPage(EnginePropertiesPage, '/management/engine')
    await settle()
    await buttonByText(wrapper, '编辑')!.trigger('click')
    await settle()
    const inputs = wrapper.findAll('input')
    expect((inputs[0].element as HTMLInputElement).disabled).toBe(true)
    await inputs[1].setValue('off')
    await buttonByText(wrapper, '保存修改')!.trigger('click')
    await settle()
    expect(calls.find((call) => call.method === 'PUT')?.body).toEqual({ name: 'feature.flag', value: 'off' })
    expect(wrapper.text()).toContain('属性 feature.flag 已更新')
  })

  it('删除自定义属性', async () => {
    const calls = mockFetch([
      { method: 'DELETE', path: '/service/management/engine-properties/feature.flag', status: 204 },
      { method: 'GET', path: '/service/management/engine-properties', body: PROPERTIES },
    ])
    const { wrapper } = await mountPage(EnginePropertiesPage, '/management/engine')
    await settle()
    confirmByLabel(wrapper, '删除')!.vm.$emit('confirm')
    await settle()
    expect(calls.some((call) => call.method === 'DELETE')).toBe(true)
    expect(wrapper.text()).toContain('属性 feature.flag 已删除')
  })

  it('加载失败展示 ErrorBlock', async () => {
    mockFetch([{ method: 'GET', path: '/service/management/engine-properties', status: 500, body: { code: 'ENGINE-0002', message: '引擎已关闭' } }])
    const { wrapper } = await mountPage(EnginePropertiesPage, '/management/engine')
    await settle()
    expect(wrapper.text()).toContain('引擎已关闭')
  })
})
