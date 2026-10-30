import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { createMemoryHistory, createRouter, type Router } from 'vue-router'
import { defineComponent } from 'vue'
import ModelsPage from '@/pages/repository/ModelsPage.vue'
import FilterBar from '@/components/FilterBar.vue'
import DataTable from '@/components/DataTable.vue'
import { mockFetch, pageOf, settle } from '../helpers'

const EmptyPage = defineComponent({ name: 'EmptyPage', template: '<div />' })

/** 确认按钮桩：直接触发 confirm，避免依赖弹层定位 */
const ConfirmButtonStub = defineComponent({
  name: 'ConfirmButton',
  props: {
    label: { type: String, default: '删除' },
    loading: { type: Boolean, default: false },
    title: { type: String, default: '' },
    description: { type: String, default: '' },
  },
  emits: ['confirm'],
  template: '<button class="confirm-stub" type="button" @click="$emit(\'confirm\')">{{ label }}</button>',
})

const model = {
  id: 'm-1',
  name: '请假流程',
  key: 'leave',
  category: 'hr',
  version: 3,
  deploymentId: 'd-1',
  lastUpdateTime: '2026-10-30T10:00:00Z',
}

// 创建测试路由。
// @return 路由实例
function makeRouter(): Router {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/repository/models', name: 'models', component: EmptyPage },
      { path: '/repository/models/:id/design', name: 'model-designer', component: EmptyPage },
    ],
  })
}

// 挂载模型列表页并等待首屏请求完成。
// @param path 初始路径
// @return 包装器与路由
async function mountPage(path = '/repository/models'): Promise<{ wrapper: VueWrapper; router: Router }> {
  const router = makeRouter()
  await router.push(path)
  await router.isReady()
  const wrapper = mount(ModelsPage, {
    global: {
      plugins: [router],
      stubs: { teleport: true, ConfirmButton: ConfirmButtonStub },
    },
  })
  await settle()
  return { wrapper, router }
}

// 按文本查找按钮。
// @param wrapper 包装器
// @param text 按钮文本
// @return 元素或 undefined
function buttonByText(wrapper: VueWrapper, text: string) {
  return wrapper.findAll('button').find((item) => item.text().trim() === text)
}

// 读取 DataTable 的当前分页属性。
// @param wrapper 页面包装器
// @return 分页属性
function tableProps(wrapper: VueWrapper): { total: number; page: number; pageSize: number } {
  return wrapper.findComponent(DataTable).props() as { total: number; page: number; pageSize: number }
}

// 等待弹窗与异步保存完成。
// @return 无
async function flush(): Promise<void> {
  await settle()
  await settle()
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('模型列表页', () => {
  it('加载并渲染模型字段', async () => {
    const calls = mockFetch([{ path: '/service/repository/models', body: pageOf([model], 1) }])
    const { wrapper } = await mountPage()
    expect(calls[0].url).toContain('start=0')
    expect(calls[0].url).toContain('size=50')
    expect(calls[0].url).toContain('sort=lastUpdateTime')
    expect(calls[0].url).toContain('order=desc')
    const text = wrapper.text()
    expect(text).toContain('请假流程')
    expect(text).toContain('leave')
    expect(text).toContain('hr')
    expect(text).toContain('v3')
    expect(text).toContain('已部署')
    expect(text).toContain('2026-10-30')
    expect(tableProps(wrapper).total).toBe(1)
  })

  it('从路由 query 还原筛选与分页', async () => {
    const calls = mockFetch([{ path: '/service/repository/models', body: pageOf([model], 30) }])
    const { wrapper } = await mountPage('/repository/models?nameLike=%E8%AF%B7%E5%81%87&deployed=true&start=50&size=20')
    const url = new URL(calls[0].url, 'http://localhost')
    expect(url.searchParams.get('nameLike')).toBe('请假')
    expect(url.searchParams.get('deployed')).toBe('true')
    expect(url.searchParams.get('start')).toBe('50')
    expect(url.searchParams.get('size')).toBe('20')
    expect(tableProps(wrapper).page).toBe(3)
    expect(tableProps(wrapper).pageSize).toBe(20)
  })

  it('空数据显示空态', async () => {
    mockFetch([{ path: '/service/repository/models', body: pageOf([]) }])
    const { wrapper } = await mountPage()
    expect(wrapper.text()).toContain('暂无流程模型')
  })

  it('接口错误展示 ErrorBlock 原文并可重试', async () => {
    const calls = mockFetch([
      {
        path: '/service/repository/models',
        status: 500,
        body: { code: 'REST-0500', message: '查询模型失败', exception: 'stack' },
      },
      { path: '/service/repository/models', body: pageOf([model]) },
    ])
    const { wrapper } = await mountPage()
    expect(wrapper.text()).toContain('查询模型失败')
    expect(wrapper.text()).toContain('REST-0500')
    await wrapper.findComponent(DataTable).vm.$emit('retry')
    await flush()
    expect(calls.length).toBe(2)
  })

  it('输入名称后查询写入路由并刷新', async () => {
    const calls = mockFetch([{ path: '/service/repository/models', body: pageOf([model]) }])
    const { wrapper, router } = await mountPage()
    await wrapper.find('input[placeholder="按名称模糊搜索"]').setValue('请假')
    await wrapper.findComponent(FilterBar).vm.$emit('search')
    await flush()
    expect(calls.length).toBe(2)
    expect(decodeURIComponent(calls[1].url)).toContain('nameLike=请假')
    expect(router.currentRoute.value.query.nameLike).toBe('请假')
    expect(router.currentRoute.value.query.start).toBeUndefined()
  })

  it('重置清空筛选与路由 query', async () => {
    const calls = mockFetch([{ path: '/service/repository/models', body: pageOf([model]) }])
    const { wrapper, router } = await mountPage('/repository/models?nameLike=leave&key=leave')
    expect(calls[0].url).toContain('nameLike=leave')
    await wrapper.findComponent(FilterBar).vm.$emit('reset')
    await flush()
    expect(router.currentRoute.value.query).toEqual({})
    expect(calls[calls.length - 1].url).not.toContain('nameLike')
    expect(calls[calls.length - 1].url).not.toContain('key=')
  })

  it('翻页与切换每页条数更新请求', async () => {
    const calls = mockFetch([{ path: '/service/repository/models', body: pageOf([model], 120) }])
    const { wrapper, router } = await mountPage()
    await wrapper.findComponent(DataTable).vm.$emit('update:page', 2)
    await flush()
    expect(calls[calls.length - 1].url).toContain('start=50')
    expect(router.currentRoute.value.query.start).toBe('50')
    await wrapper.findComponent(DataTable).vm.$emit('update:page-size', 20)
    await flush()
    const last = calls[calls.length - 1].url
    expect(last).toContain('start=0')
    expect(last).toContain('size=20')
    expect(router.currentRoute.value.query.size).toBe('20')
  })

  it('新建模型后写入最小 BPMN 并跳转设计器', async () => {
    const calls = mockFetch([
      { path: '/service/repository/models', method: 'POST', status: 201, body: model },
      { path: '/service/repository/models/m-1/source', method: 'PUT', status: 204 },
      { path: '/service/repository/models', body: pageOf([]) },
    ])
    const { wrapper, router } = await mountPage()
    await wrapper.find('[data-testid="create-model"]').trigger('click')
    await flush()
    await wrapper.find('input[placeholder="例如：请假流程"]').setValue('请假流程')
    await wrapper.find('input[placeholder="例如：leave"]').setValue('leave')
    await wrapper.find('input[placeholder="例如：hr"]').setValue('hr')
    const create = buttonByText(wrapper, '创建')
    expect(create).toBeTruthy()
    await create!.trigger('click')
    await flush()
    const post = calls.find((item) => item.method === 'POST')
    expect(post?.body).toEqual({ name: '请假流程', key: 'leave', category: 'hr', metaInfo: '' })
    const put = calls.find((item) => item.method === 'PUT')
    expect(put?.url).toContain('/models/m-1/source')
    const file = (put?.body as FormData).get('file') as File
    expect(file.name).toBe('leave.bpmn20.xml')
    expect(router.currentRoute.value.name).toBe('model-designer')
    expect(router.currentRoute.value.params.id).toBe('m-1')
  })

  it('Key 冲突时保留弹窗并展示服务端错误', async () => {
    const calls = mockFetch([
      { path: '/service/repository/models', method: 'POST', status: 409, body: { code: 'REPO-0409', message: '模型 Key 已存在' } },
      { path: '/service/repository/models', body: pageOf([]) },
    ])
    const { wrapper } = await mountPage()
    await wrapper.find('[data-testid="create-model"]').trigger('click')
    await flush()
    await wrapper.find('input[placeholder="例如：请假流程"]').setValue('另一个流程')
    await wrapper.find('input[placeholder="例如：leave"]').setValue('leave')
    await buttonByText(wrapper, '创建')!.trigger('click')
    await flush()
    expect(calls.some((item) => item.method === 'PUT')).toBe(false)
    expect(wrapper.text()).toContain('模型 Key 已存在')
    expect(wrapper.find('input[placeholder="例如：leave"]').exists()).toBe(true)
  })

  it('新建后源码写入失败时关闭弹窗并提示从列表进入', async () => {
    const calls = mockFetch([
      { path: '/service/repository/models', method: 'POST', status: 201, body: model },
      {
        path: '/service/repository/models/m-1/source',
        method: 'PUT',
        status: 500,
        body: { code: 'REST-0500', message: '磁盘不可写' },
      },
      { path: '/service/repository/models', body: pageOf([model]) },
    ])
    const { wrapper } = await mountPage()
    await wrapper.find('[data-testid="create-model"]').trigger('click')
    await flush()
    await wrapper.find('input[placeholder="例如：请假流程"]').setValue('请假流程')
    await wrapper.find('input[placeholder="例如：leave"]').setValue('leave')
    await buttonByText(wrapper, '创建')!.trigger('click')
    await flush()
    expect(wrapper.text()).toContain('最小源码写入失败')
    expect(calls.filter((item) => item.method === 'GET').length).toBeGreaterThanOrEqual(2)
    expect(wrapper.find('input[placeholder="例如：leave"]').exists()).toBe(false)
  })

  it('编辑元信息提交 PUT 并刷新列表', async () => {
    const calls = mockFetch([
      { path: '/service/repository/models/m-1', method: 'PUT', body: { ...model, name: '改名流程' } },
      { path: '/service/repository/models', body: pageOf([model]) },
    ])
    const { wrapper } = await mountPage()
    await buttonByText(wrapper, '编辑')!.trigger('click')
    await flush()
    const nameInput = wrapper.find('input[placeholder="例如：请假流程"]')
    await nameInput.setValue('改名流程')
    await buttonByText(wrapper, '保存')!.trigger('click')
    await flush()
    const put = calls.find((item) => item.method === 'PUT')
    expect(put?.body).toMatchObject({ name: '改名流程', category: 'hr' })
    expect(wrapper.text()).toContain('已更新模型')
  })

  it('删除模型二次确认后调用 DELETE', async () => {
    const calls = mockFetch([
      { path: '/service/repository/models/m-1', method: 'DELETE', status: 204 },
      { path: '/service/repository/models', body: pageOf([model]) },
    ])
    const { wrapper } = await mountPage()
    await wrapper.find('.confirm-stub').trigger('click')
    await flush()
    const remove = calls.find((item) => item.method === 'DELETE')
    expect(remove?.url).toBe('/service/repository/models/m-1')
    expect(wrapper.text()).toContain('已删除模型')
  })

  it('删除失败展示服务端错误', async () => {
    mockFetch([
      { path: '/service/repository/models/m-1', method: 'DELETE', status: 403, body: { code: 'AUTH-0403', message: '没有删除权限' } },
      { path: '/service/repository/models', body: pageOf([model]) },
    ])
    const { wrapper } = await mountPage()
    await wrapper.find('.confirm-stub').trigger('click')
    await flush()
    expect(wrapper.text()).toContain('没有删除权限')
  })
})
