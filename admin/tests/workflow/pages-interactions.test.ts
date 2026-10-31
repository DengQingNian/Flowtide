// 工作流页面交互补充测试：分页/筛选/编辑回填/启停/删除/流程图成功路径等处理器覆盖。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { NInput, NSelect, NSwitch } from 'naive-ui'
import CategoryPage from '@/pages/workflow/CategoryPage.vue'
import ModelPage from '@/pages/workflow/ModelPage.vue'
import TemplateConfigPage from '@/pages/workflow/TemplateConfigPage.vue'
import TodoPage from '@/pages/workflow/TodoPage.vue'
import DonePage from '@/pages/workflow/DonePage.vue'
import InstancePage from '@/pages/workflow/InstancePage.vue'
import EntrustPage from '@/pages/workflow/EntrustPage.vue'
import RemindPage from '@/pages/workflow/RemindPage.vue'
import ExceptionPage from '@/pages/workflow/ExceptionPage.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import DataTable from '@/components/DataTable.vue'
import FilterBar from '@/components/FilterBar.vue'
import DiagramPanel from '@/components/DiagramPanel.vue'
import FormModal from '@/pages/workflow/components/FormModal.vue'
import { authNotice } from '@/stores/ui'
import { mockFetch, settle } from '../helpers'

// r 构造 R 成功信封。
// @param data 业务数据
// @return 信封对象
function r(data: unknown) {
  return { code: '200', msg: 'ok', data, success: true }
}

// ok 构造成功响应路由。
// @param path 路径（前缀或正则）
// @param method HTTP 方法
// @return 模拟路由
function okRoute(path: string | RegExp, method?: string) {
  return { path, method, body: r(null) }
}

// mountPage 挂载页面（teleport 桩）并等待首次加载。
// @param component 页面组件
// @return 包装器
async function mountPage(component: unknown): Promise<VueWrapper> {
  const wrapper = mount(component, { global: { stubs: { teleport: true } } } as never)
  await settle()
  return wrapper
}

// clickButton 点击包含指定文本的按钮。
// @param wrapper 包装器
// @param text 按钮文本
// @return 无
async function clickButton(wrapper: VueWrapper, text: string): Promise<void> {
  const button = wrapper.findAll('button').find((item) => item.text().includes(text))
  if (!button) throw new Error(`未找到按钮：${text}`)
  await button.trigger('click')
  await settle()
}

// changePage 触发表格翻页。
// @param wrapper 包装器
// @param page 页码
// @return 无
async function changePage(wrapper: VueWrapper, page: number): Promise<void> {
  wrapper.findComponent(DataTable).vm.$emit('update:page', page)
  await settle()
}

// changePageSize 触发每页条数变化。
// @param wrapper 包装器
// @param size 条数
// @return 无
async function changePageSize(wrapper: VueWrapper, size: number): Promise<void> {
  wrapper.findComponent(DataTable).vm.$emit('update:page-size', size)
  await settle()
}

// findModalByTitle 按标题在多个 FormModal 中定位目标弹窗（页面可能同时挂两个表单弹窗）。
// @param wrapper 包装器
// @param titlePart 标题片段
// @return 目标弹窗包装器
function findModalByTitle(wrapper: VueWrapper, titlePart: string): VueWrapper {
  const modal = wrapper
    .findAllComponents(FormModal)
    .find((item) => String(item.props('title')).includes(titlePart))
  if (!modal) throw new Error(`未找到弹窗：${titlePart}`)
  return modal as VueWrapper
}

beforeEach(() => {
  authNotice.value = 'none'
  vi.unstubAllGlobals()
})

describe('流程分类页交互', () => {
  const listRoute = () => ({
    path: /^\/workflow\/category\/list/,
    body: r([{ id: 'c1', categoryName: '行政', categoryCode: 'admin', parentId: null, enabled: true, visible: true, remark: '备注A' }]),
  })

  it('编辑回填并提交编辑接口', async () => {
    const calls = mockFetch([listRoute(), okRoute('/workflow/category/edit', 'POST')])
    const wrapper = await mountPage(CategoryPage)
    await clickButton(wrapper, '编辑')
    const modal = wrapper.findComponent(FormModal)
    const inputs = modal.findAllComponents(NInput)
    inputs[0].vm.$emit('update:value', '行政修订')
    await settle()
    await clickButton(modal, '保存')
    expect(calls.find((call) => call.url === '/workflow/category/edit')?.body).toMatchObject({ id: 'c1', categoryName: '行政修订' })
  })

  it('添加子级预填父分类；必填校验拦截', async () => {
    mockFetch([listRoute()])
    const wrapper = await mountPage(CategoryPage)
    await clickButton(wrapper, '添加子级')
    const modal = wrapper.findComponent(FormModal)
    await clickButton(modal, '保存')
    expect(modal.text()).toContain('分类名称与分类代号为必填项')
  })

  it('刷新按钮重新拉取列表', async () => {
    const calls = mockFetch([listRoute()])
    const wrapper = await mountPage(CategoryPage)
    await clickButton(wrapper, '刷新')
    expect(calls.filter((call) => call.url === '/workflow/category/list').length).toBe(2)
  })
})

describe('流程模型页交互', () => {
  const routes = () => [
    { path: /^\/workflow\/model\/list/, body: r({ list: [{ id: 'm1', modelName: '请假', modelCode: 'leave', enabled: true, tenantId: 'crm', remark: 'R' }], total: 1 }) },
    { path: /^\/workflow\/category\/list/, body: r([{ id: 'c1', categoryName: '行政', categoryCode: 'admin' }]) },
  ]

  it('搜索与重置走筛选参数', async () => {
    const calls = mockFetch(routes())
    const wrapper = await mountPage(ModelPage)
    const inputs = wrapper.findAllComponents(NInput)
    inputs[0].vm.$emit('update:value', '请假')
    await settle()
    await wrapper.findComponent(FilterBar).vm.$emit('search')
    await settle()
    expect(calls.at(-1)?.url).toContain('modelName=')
    await wrapper.findComponent(FilterBar).vm.$emit('reset')
    await settle()
    expect(calls.at(-1)?.url).not.toContain('modelName=')
  })

  it('翻页与条数变化触发重新加载', async () => {
    const calls = mockFetch(routes())
    const wrapper = await mountPage(ModelPage)
    await changePage(wrapper, 2)
    expect(calls.at(-1)?.url).toContain('pageNum=2')
    await changePageSize(wrapper, 10)
    expect(calls.at(-1)?.url).toContain('pageSize=10')
  })

  it('新增与编辑弹窗提交', async () => {
    const calls = mockFetch([...routes(), okRoute('/workflow/model/add', 'POST'), okRoute('/workflow/model/edit', 'POST')])
    const wrapper = await mountPage(ModelPage)
    await clickButton(wrapper, '新增模型')
    let modal = wrapper.findComponent(FormModal)
    const inputs = modal.findAllComponents(NInput)
    inputs[0].vm.$emit('update:value', '报销')
    inputs[1].vm.$emit('update:value', 'expense')
    await settle()
    await clickButton(modal, '保存')
    expect(calls.find((call) => call.url === '/workflow/model/add')?.body).toMatchObject({ modelName: '报销', modelCode: 'expense' })

    await clickButton(wrapper, '编辑')
    modal = wrapper.findComponent(FormModal)
    await clickButton(modal, '保存')
    expect(calls.find((call) => call.url === '/workflow/model/edit')?.body).toMatchObject({ id: 'm1' })
  })

  it('删除模型调用 delete/{ids}', async () => {
    const calls = mockFetch([...routes(), okRoute(/^\/workflow\/model\/delete\//, 'DELETE')])
    const wrapper = await mountPage(ModelPage)
    // 行操作顺序：部署(0)、删除(1)
    wrapper.findAllComponents(ConfirmButton)[1].vm.$emit('confirm')
    await settle()
    expect(calls.find((call) => call.url.startsWith('/workflow/model/delete/'))?.url).toBe('/workflow/model/delete/m1')
  })
})

describe('流程配置页交互', () => {
  const routes = () => [
    { path: /^\/workflow\/model\/list/, body: r({ list: [{ id: 'm1', modelName: '请假', modelCode: 'leave' }], total: 1 }) },
    { path: /^\/workflow\/step\/list/, body: r([{ id: 's1', stepName: '审批', stepCode: 'audit', sort: 2, allow_return: true, back_step_codes: 'draft', url: '/a' }]) },
    { path: /^\/workflow\/template\/list/, body: r([{ id: 't1', templateName: '默认模板', templateCode: 'default', remark: 'R', updateTime: '2026-10-30 10:00:00' }]) },
  ]

  it('编辑步骤回填下划线字段并提交', async () => {
    const calls = mockFetch([...routes(), okRoute('/workflow/step/edit', 'POST')])
    const wrapper = await mountPage(TemplateConfigPage)
    await clickButton(wrapper, '编辑')
    const modal = findModalByTitle(wrapper, '步骤')
    // 回填断言：allow_return 为 true 时开关打开
    expect(modal.findAllComponents(NSwitch)[0].props('value')).toBe(true)
    await clickButton(modal, '保存')
    expect(calls.find((call) => call.url === '/workflow/step/edit')?.body).toMatchObject({ id: 's1', stepName: '审批', backStepCodes: 'draft' })
  })

  it('切换模型重新加载步骤；空名校验拦截', async () => {
    const calls = mockFetch([...routes()])
    const wrapper = await mountPage(TemplateConfigPage)
    await clickButton(wrapper, '新增步骤')
    const modal = wrapper.findComponent(FormModal)
    await clickButton(modal, '保存')
    expect(modal.text()).toContain('步骤名称与步骤代号为必填项')
    // 切换模型触发步骤列表重查
    const select = wrapper.findComponent(NSelect)
    select.vm.$emit('update:value', null)
    await settle()
    expect(calls.filter((call) => call.url.startsWith('/workflow/step/list')).length).toBeGreaterThanOrEqual(2)
  })

  it('删除步骤与模板增改删', async () => {
    const calls = mockFetch([
      ...routes(),
      okRoute(/^\/workflow\/step\/delete\//, 'DELETE'),
      okRoute('/workflow/template/add', 'POST'),
      okRoute('/workflow/template/edit', 'POST'),
      okRoute(/^\/workflow\/template\/delete\//, 'DELETE'),
    ])
    const wrapper = await mountPage(TemplateConfigPage)
    // 删除步骤
    wrapper.findAllComponents(ConfirmButton)[0].vm.$emit('confirm')
    await settle()
    expect(calls.find((call) => call.url.startsWith('/workflow/step/delete/'))?.url).toBe('/workflow/step/delete/s1')
    // 模板操作在第二个 Tab 内（非激活 Tab 不渲染，先切换）
    const tabs = wrapper.findAll('.n-tabs-tab')
    await tabs[1].trigger('click')
    await settle()
    await clickButton(wrapper, '新增模板')
    let modal = findModalByTitle(wrapper, '模板')
    const inputs = modal.findAllComponents(NInput)
    inputs[0].vm.$emit('update:value', '模板B')
    inputs[1].vm.$emit('update:value', 'tpl-b')
    await settle()
    await clickButton(modal, '保存')
    expect(calls.find((call) => call.url === '/workflow/template/add')?.body).toMatchObject({ templateName: '模板B' })
    // 编辑模板
    await clickButton(wrapper, '编辑')
    modal = findModalByTitle(wrapper, '模板')
    await clickButton(modal, '保存')
    expect(calls.find((call) => call.url === '/workflow/template/edit')?.body).toMatchObject({ id: 't1' })
    // 删除模板
    wrapper.findAllComponents(ConfirmButton).at(-1)!.vm.$emit('confirm')
    await settle()
    expect(calls.find((call) => call.url.startsWith('/workflow/template/delete/'))?.url).toBe('/workflow/template/delete/t1')
  })
})

describe('待办/已办页交互', () => {
  it('待办：搜索、重置与翻页', async () => {
    const calls = mockFetch([
      { path: /^\/workflow\/process\/task\/all\/todoList/, body: r({ list: [{ id: 't1', title: '审批A' }], total: 1 }) },
    ])
    const wrapper = await mountPage(TodoPage)
    await wrapper.findComponent(FilterBar).vm.$emit('search')
    await settle()
    await wrapper.findComponent(FilterBar).vm.$emit('reset')
    await settle()
    expect(calls.at(-1)?.url).toContain('identityId=u1')
    await changePage(wrapper, 2)
    expect(calls.at(-1)?.url).toContain('pageNum=2')
    await changePageSize(wrapper, 10)
    expect(calls.at(-1)?.url).toContain('pageSize=10')
  })

  it('已办：搜索、重置与翻页', async () => {
    const calls = mockFetch([
      { path: /^\/workflow\/process\/task\/all\/doneList/, body: r({ list: [{ id: 'd1', title: '已办A' }], total: 1 }) },
    ])
    const wrapper = await mountPage(DonePage)
    await wrapper.findComponent(FilterBar).vm.$emit('reset')
    await settle()
    await changePage(wrapper, 3)
    expect(calls.at(-1)?.url).toContain('pageNum=3')
    await changePageSize(wrapper, 50)
    expect(calls.at(-1)?.url).toContain('pageSize=50')
  })
})

describe('流程实例页交互', () => {
  const routes = () => [
    { path: /^\/workflow\/process\/instance\/all\/list/, body: r({ list: [{ id: 'pi-1', title: '订单', businessKey: 'B1', status: 2 }], total: 1 }) },
  ]

  it('挂起/激活/删除行操作命中对应端点', async () => {
    const calls = mockFetch([
      ...routes(),
      okRoute(/^\/workflow\/process\/activate\/processInstance\//, 'POST'),
      okRoute(/^\/workflow\/process\/instance\/all\/delete\//, 'DELETE'),
    ])
    const wrapper = await mountPage(InstancePage)
    // 行操作顺序：终止(0)、激活(1，状态为挂起)、删除(2)
    wrapper.findAllComponents(ConfirmButton)[1].vm.$emit('confirm')
    await settle()
    expect(calls.find((call) => call.url.startsWith('/workflow/process/activate/processInstance/'))?.url).toBe('/workflow/process/activate/processInstance/pi-1')
    wrapper.findAllComponents(ConfirmButton)[2].vm.$emit('confirm')
    await settle()
    expect(calls.find((call) => call.url.startsWith('/workflow/process/instance/all/delete/'))?.url).toBe('/workflow/process/instance/all/delete/pi-1')
  })

  it('翻页携带业务键筛选', async () => {
    const calls = mockFetch(routes())
    const wrapper = await mountPage(InstancePage)
    await changePage(wrapper, 2)
    await changePageSize(wrapper, 10)
    expect(calls.at(-1)?.url).toContain('pageSize=10')
    await wrapper.findComponent(FilterBar).vm.$emit('reset')
    await settle()
    expect(calls.at(-1)?.url).not.toContain('businessKey=')
  })

  it('流程图成功路径：高亮数据渲染 DiagramPanel', async () => {
    mockFetch([
      ...routes(),
      {
        path: /^\/workflow\/process\/highlight\/img/,
        body: { code: '200', success: true, data: { bpmnXml: '<definitions/>', completed: ['a1'], active: ['a2'] } },
      },
    ])
    const wrapper = await mountPage(InstancePage)
    await clickButton(wrapper, '流程图')
    expect(wrapper.findComponent(DiagramPanel).exists()).toBe(true)
    expect(wrapper.findComponent(DiagramPanel).props('xml')).toBe('<definitions/>')
  })

  it('高亮无 XML 时回退实例态 XML 门面', async () => {
    mockFetch([
      ...routes(),
      { path: /^\/workflow\/process\/highlight\/img/, body: { code: '200', success: true, data: { completed: ['a1'] } } },
      { path: /^\/workflow\/process\/getHisModelJson\//, body: { code: '200', success: true, data: '<definitions/>' } },
    ])
    const wrapper = await mountPage(InstancePage)
    await clickButton(wrapper, '流程图')
    expect(wrapper.findComponent(DiagramPanel).props('xml')).toBe('<definitions/>')
  })
})

describe('委托配置页交互', () => {
  const routes = () => [
    { path: /^\/workflow\/delegate\/list/, body: r([{ id: 'd1', ownerId: 'u1', trusteeId: 'u2', enabled: false, condition: 'ALL', systemSymbol: 'crm', remark: '备注', startTime: '2026-10-01 00:00:00', endTime: '2026-10-31 23:59:59' }]) },
  ]

  it('编辑回填并提交；新增校验相同身份', async () => {
    const calls = mockFetch([...routes(), okRoute('/workflow/delegate/edit', 'POST')])
    const wrapper = await mountPage(EntrustPage)
    await clickButton(wrapper, '编辑')
    const modal = wrapper.findComponent(FormModal)
    await clickButton(modal, '保存')
    expect(calls.find((call) => call.url === '/workflow/delegate/edit')?.body).toMatchObject({ id: 'd1', trusteeId: 'u2' })

    await clickButton(wrapper, '新增委托')
    const modal2 = wrapper.findComponent(FormModal)
    const inputs = modal2.findAllComponents(NInput)
    inputs[0].vm.$emit('update:value', 'u1')
    inputs[1].vm.$emit('update:value', 'u1')
    await settle()
    await clickButton(modal2, '保存')
    expect(modal2.text()).toContain('受托人不能与委托人相同')
  })

  it('翻页与搜索', async () => {
    const calls = mockFetch(routes())
    const wrapper = await mountPage(EntrustPage)
    await wrapper.findComponent(FilterBar).vm.$emit('search')
    await settle()
    expect(calls.at(-1)?.url).toContain('pageNum=1')
    await changePage(wrapper, 2)
    expect(calls.at(-1)?.url).toContain('pageNum=2')
  })
})

describe('催办配置页交互', () => {
  const routes = () => [
    { path: /^\/workflow\/remind\/getPageList/, body: r({ list: [{ id: 'r1', systemSymbol: 'crm', stepCodes: 's1,s2', delayEnabled: true, delay: 5, createTime: '2026-10-30 10:00:00' }], total: 1 }) },
  ]

  it('编辑回填并提交编辑接口', async () => {
    const calls = mockFetch([...routes(), okRoute('/workflow/remind/edit', 'POST')])
    const wrapper = await mountPage(RemindPage)
    await clickButton(wrapper, '编辑')
    const modal = wrapper.findComponent(FormModal)
    await clickButton(modal, '保存')
    expect(calls.find((call) => call.url === '/workflow/remind/edit')?.body).toMatchObject({ id: 'r1', systemSymbol: 'crm', stepCodes: 's1,s2' })
  })

  it('删除调用 remind/delete/{id}', async () => {
    const calls = mockFetch([...routes(), okRoute(/^\/workflow\/remind\/delete\//, 'DELETE')])
    const wrapper = await mountPage(RemindPage)
    wrapper.findAllComponents(ConfirmButton)[0].vm.$emit('confirm')
    await settle()
    expect(calls.find((call) => call.url.startsWith('/workflow/remind/delete/'))?.url).toBe('/workflow/remind/delete/r1')
  })

  it('搜索携带系统代号并支持翻页', async () => {
    const calls = mockFetch(routes())
    const wrapper = await mountPage(RemindPage)
    const inputs = wrapper.findAllComponents(NInput)
    inputs[0].vm.$emit('update:value', 'oa')
    await settle()
    await wrapper.findComponent(FilterBar).vm.$emit('search')
    await settle()
    expect(calls.at(-1)?.url).toContain('systemSymbol=oa')
    await changePageSize(wrapper, 10)
    expect(calls.at(-1)?.url).toContain('pageSize=10')
  })
})

describe('流程异常页交互', () => {
  const routes = () => [
    { path: /^\/workflow\/exception\/list/, body: r([{ id: 'e1', title: '网关异常', exceptionType: 'ENGINE', errorMessage: 'boom', handleStatus: 'handled' }]) },
  ]

  it('搜索与翻页', async () => {
    const calls = mockFetch(routes())
    const wrapper = await mountPage(ExceptionPage)
    const inputs = wrapper.findAllComponents(NInput)
    inputs[0].vm.$emit('update:value', 'pi-9')
    await settle()
    await wrapper.findComponent(FilterBar).vm.$emit('search')
    await settle()
    expect(calls.at(-1)?.url).toContain('processInstanceId=pi-9')
    await changePage(wrapper, 2)
    expect(calls.at(-1)?.url).toContain('pageNum=2')
    await wrapper.findComponent(FilterBar).vm.$emit('reset')
    await settle()
    expect(calls.at(-1)?.url).not.toContain('processInstanceId=')
  })

  it('标记已处理 501 时展示降级文案', async () => {
    mockFetch([
      ...routes(),
      { path: /^\/workflow\/exception\/delete\//, method: 'DELETE', status: 501, body: { code: '500', ErrorMessage: '接口未实现', success: false } },
    ])
    const wrapper = await mountPage(ExceptionPage)
    wrapper.findAllComponents(ConfirmButton)[0].vm.$emit('confirm')
    await settle()
    expect(wrapper.text()).toContain('异常处理接口接线中（后端 501）')
  })
})
