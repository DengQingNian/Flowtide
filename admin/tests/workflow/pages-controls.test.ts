// 工作流页面控件交互补测：弹窗全字段控件、刷新/回车触发、抽屉与弹窗关闭、终止/挂起操作。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { NDrawer, NInput, NInputNumber, NModal, NSelect, NSwitch, NTreeSelect } from 'naive-ui'
import CategoryPage from '@/pages/workflow/CategoryPage.vue'
import ModelPage from '@/pages/workflow/ModelPage.vue'
import TemplateConfigPage from '@/pages/workflow/TemplateConfigPage.vue'
import TodoPage from '@/pages/workflow/TodoPage.vue'
import DonePage from '@/pages/workflow/DonePage.vue'
import InstancePage from '@/pages/workflow/InstancePage.vue'
import EntrustPage from '@/pages/workflow/EntrustPage.vue'
import RemindPage from '@/pages/workflow/RemindPage.vue'
import ExceptionPage from '@/pages/workflow/ExceptionPage.vue'
import ApprovalDialog from '@/pages/workflow/components/ApprovalDialog.vue'
import FormModal from '@/pages/workflow/components/FormModal.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import DataTable from '@/components/DataTable.vue'
import FilterBar from '@/components/FilterBar.vue'
import { authNotice } from '@/stores/ui'
import { mockFetch, settle } from '../helpers'

// r 构造 R 成功信封。
// @param data 业务数据
// @return 信封对象
function r(data: unknown) {
  return { code: '200', msg: 'ok', data, success: true }
}

// listRoute 构造列表查询成功路由。
// @param path 路径前缀
// @param rows 行数据
// @return 模拟路由
function listRoute(path: string | RegExp, rows: unknown[]) {
  return { path, body: r({ list: rows, total: rows.length }) }
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

// pressEnter 触发第 index 个输入框的回车事件。
// @param wrapper 包装器
// @param index 输入框序号
// @return 无
async function pressEnter(wrapper: VueWrapper, index: number): Promise<void> {
  const input = wrapper.findAll('input')[index]
  await input.trigger('keyup', { key: 'enter' })
  await settle()
}

// driveModalControls 对弹窗内全部控件依次触发更新事件（覆盖各 v-model 编译回调）。
// @param modal 弹窗包装器
// @return 无
async function driveModalControls(modal: VueWrapper): Promise<void> {
  modal.findAllComponents(NTreeSelect).forEach((item) => item.vm.$emit('update:value', 'c1'))
  modal.findAllComponents(NInputNumber).forEach((item) => item.vm.$emit('update:value', 5))
  modal.findAllComponents(NSwitch).forEach((item) => item.vm.$emit('update:checked', false))
  modal.findAllComponents(NSelect).forEach((item) => item.vm.$emit('update:value', 'minute'))
  const inputs = modal.findAllComponents(NInput)
  inputs.forEach((item, index) => item.vm.$emit('update:value', `v${index}`))
  await settle()
}

beforeEach(() => {
  authNotice.value = 'none'
  vi.unstubAllGlobals()
})

describe('弹窗控件与关闭路径', () => {
  it('分类弹窗：全控件更新、取消关闭', async () => {
    mockFetch([
      listRoute(/^\/workflow\/category\/list/, [{ id: 'c1', categoryName: '行政', categoryCode: 'admin' }]),
    ])
    const wrapper = await mountPage(CategoryPage)
    await clickButton(wrapper, '新增分类')
    const modal = wrapper.findAllComponents(FormModal)[0]
    await driveModalControls(modal)
    await clickButton(modal, '取消')
    expect(wrapper.findAllComponents(FormModal)[0].props('show')).toBe(false)
  })

  it('模型弹窗：树选择/开关/排序/备注更新后提交', async () => {
    const calls = mockFetch([
      listRoute(/^\/workflow\/model\/list/, [{ id: 'm1', modelName: '请假', modelCode: 'leave' }]),
      listRoute(/^\/workflow\/category\/list/, [{ id: 'c1', categoryName: '行政', categoryCode: 'admin' }]),
      { path: '/workflow/model/add', method: 'POST', body: r(null) },
    ])
    const wrapper = await mountPage(ModelPage)
    await clickButton(wrapper, '新增模型')
    const modal = wrapper.findAllComponents(FormModal)[0]
    await driveModalControls(modal)
    await clickButton(modal, '保存')
    expect(calls.find((call) => call.url === '/workflow/model/add')?.body).toMatchObject({ modelName: 'v0', sort: 5 })
    await clickButton(wrapper, '刷新')
  })

  it('步骤弹窗：退回开关/可退步骤/URL/排序更新后提交', async () => {
    const calls = mockFetch([
      listRoute(/^\/workflow\/model\/list/, [{ id: 'm1', modelName: '请假', modelCode: 'leave' }]),
      listRoute(/^\/workflow\/step\/list/, [{ id: 's1', stepName: '审批', stepCode: 'audit' }]),
      listRoute(/^\/workflow\/template\/list/, []),
      { path: '/workflow/step/add', method: 'POST', body: r(null) },
    ])
    const wrapper = await mountPage(TemplateConfigPage)
    await clickButton(wrapper, '新增步骤')
    const modal = wrapper.findAllComponents(FormModal)[0]
    await driveModalControls(modal)
    await clickButton(modal, '保存')
    const body = calls.find((call) => call.url === '/workflow/step/add')?.body as Record<string, unknown>
    expect(body).toMatchObject({ stepName: 'v0', stepCode: 'v1', backStepCodes: 'v3', url: 'v4', allowReturn: false })
    // 两处刷新按钮（步骤 Tab 与模板 Tab）
    await clickButton(wrapper, '刷新')
    const tabs = wrapper.findAll('.n-tabs-tab')
    await tabs[1].trigger('click')
    await settle()
    await clickButton(wrapper, '刷新')
  })

  it('模板弹窗：备注更新；XML 预览弹窗可关闭', async () => {
    mockFetch([
      listRoute(/^\/workflow\/model\/list/, [{ id: 'm1', modelName: '请假', modelCode: 'leave' }]),
      listRoute(/^\/workflow\/step\/list/, []),
      listRoute(/^\/workflow\/template\/list/, [{ id: 't1', templateName: '默认模板', templateCode: 'default', flowXml: '<definitions/>' }]),
      { path: '/workflow/template/add', method: 'POST', body: r(null) },
    ])
    const wrapper = await mountPage(TemplateConfigPage)
    const tabs = wrapper.findAll('.n-tabs-tab')
    await tabs[1].trigger('click')
    await settle()
    await clickButton(wrapper, '新增模板')
    const modal = findTemplateModal(wrapper)
    await driveModalControls(modal)
    await clickButton(modal, '保存')
    // XML 预览开关与关闭
    await clickButton(wrapper, 'XML 预览')
    const previewModal = wrapper.findAllComponents(NModal).at(-1)!
    previewModal.vm.$emit('update:show', false)
    await settle()
  })

  it('催办弹窗：预警/延时控件更新后提交', async () => {
    const calls = mockFetch([
      listRoute(/^\/workflow\/remind\/getPageList/, [{ id: 'r1', systemSymbol: 'crm' }]),
      { path: '/workflow/remind/add', method: 'POST', body: r(null) },
    ])
    const wrapper = await mountPage(RemindPage)
    await clickButton(wrapper, '新增催办')
    const modal = wrapper.findAllComponents(FormModal)[0]
    await driveModalControls(modal)
    await clickButton(modal, '保存')
    const body = calls.find((call) => call.url === '/workflow/remind/add')?.body as Record<string, unknown>
    expect(body).toMatchObject({ systemSymbol: 'v0', ahead: 5, delay: 5 })
    await clickButton(wrapper, '刷新')
    await wrapper.findComponent(FilterBar).vm.$emit('reset')
    await settle()
  })

  it('委托弹窗：条件/时限/系统/备注更新；删除与翻页', async () => {
    const calls = mockFetch([
      listRoute(/^\/workflow\/delegate\/list/, [{ id: 'd1', ownerId: 'u1', trusteeId: 'u2', enabled: true }]),
      { path: '/workflow/delegate/add', method: 'POST', body: r(null) },
      { path: /^\/workflow\/delegate\/delete\//, method: 'DELETE', body: r(null) },
    ])
    const wrapper = await mountPage(EntrustPage)
    await clickButton(wrapper, '新增委托')
    const modal = wrapper.findAllComponents(FormModal)[0]
    await driveModalControls(modal)
    await clickButton(modal, '保存')
    expect(calls.find((call) => call.url === '/workflow/delegate/add')?.body).toMatchObject({ ownerId: 'v0', trusteeId: 'v1' })
    // 行操作：编辑(NButton)、启停(NButton)、删除(ConfirmButton)
    wrapper.findAllComponents(ConfirmButton)[0].vm.$emit('confirm')
    await settle()
    expect(calls.find((call) => call.url.startsWith('/workflow/delegate/delete/'))?.url).toBe('/workflow/delegate/delete/d1')
    await wrapper.findComponent(FilterBar).vm.$emit('reset')
    await settle()
    // 条数变化经 DataTable 转发到页面 changePageSize
    wrapper.findComponent(DataTable).vm.$emit('update:page-size', 10)
    await settle()
  })

  it('审批弹窗：非法 JSON 与非对象 JSON 报错、取消关闭', async () => {
    mockFetch([listRoute(/^\/workflow\/process\/task\/all\/todoList/, [{ id: 't1', title: '审批A' }])])
    const wrapper = await mountPage(TodoPage)
    await clickButton(wrapper, '办理')
    const dialog = wrapper.findComponent(ApprovalDialog)
    const inputs = dialog.findAllComponents(NInput)
    inputs[0].vm.$emit('update:value', '同意')
    inputs[1].vm.$emit('update:value', 'not-json')
    await settle()
    await clickButton(dialog, '提交')
    expect(dialog.text()).toContain('变量 JSON 解析失败')
    inputs[1].vm.$emit('update:value', '[1,2]')
    await settle()
    await clickButton(dialog, '提交')
    expect(dialog.text()).toContain('变量 JSON 需为对象')
    // 取消按钮关闭弹窗
    await clickButton(dialog, '取消')
    expect(dialog.props('show')).toBe(false)
  })
})

// findTemplateModal 按标题定位模板弹窗（配置页同时挂两个表单弹窗）。
// @param wrapper 页面包装器
// @return 模板弹窗
function findTemplateModal(wrapper: VueWrapper): VueWrapper {
  const modal = wrapper
    .findAllComponents(FormModal)
    .find((item) => String(item.props('title')).includes('模板'))
  if (!modal) throw new Error('未找到模板弹窗')
  return modal as VueWrapper
}

describe('刷新与回车触发', () => {
  it('待办：刷新按钮与身份输入回车', async () => {
    const calls = mockFetch([listRoute(/^\/workflow\/process\/task\/all\/todoList/, [{ id: 't1', title: 'A' }])])
    const wrapper = await mountPage(TodoPage)
    await clickButton(wrapper, '刷新')
    await pressEnter(wrapper, 0)
    expect(calls.filter((call) => call.url.includes('todoList')).length).toBeGreaterThanOrEqual(3)
  })

  it('已办：刷新按钮、回车与查询', async () => {
    const calls = mockFetch([listRoute(/^\/workflow\/process\/task\/all\/doneList/, [{ id: 'd1', title: 'A' }])])
    const wrapper = await mountPage(DonePage)
    await clickButton(wrapper, '刷新')
    await pressEnter(wrapper, 0)
    await wrapper.findComponent(FilterBar).vm.$emit('search')
    await settle()
    expect(calls.filter((call) => call.url.includes('doneList')).length).toBeGreaterThanOrEqual(4)
  })

  it('实例：刷新、回车、终止与挂起', async () => {
    const calls = mockFetch([
      listRoute(/^\/workflow\/process\/instance\/all\/list/, [{ id: 'pi-1', title: '订单', status: 1 }]),
      { path: /^\/workflow\/process\/stop\/processInstance\//, method: 'POST', body: r(null) },
      { path: /^\/workflow\/process\/suspend\/processInstance\//, method: 'POST', body: r(null) },
    ])
    const wrapper = await mountPage(InstancePage)
    await clickButton(wrapper, '刷新')
    await pressEnter(wrapper, 0)
    // 状态 1（运行中）→ 行操作顺序：终止(0)、挂起(1)、删除(2)
    wrapper.findAllComponents(ConfirmButton)[0].vm.$emit('confirm')
    await settle()
    expect(calls.find((call) => call.url.startsWith('/workflow/process/stop/processInstance/'))?.url).toBe('/workflow/process/stop/processInstance/pi-1')
    wrapper.findAllComponents(ConfirmButton)[1].vm.$emit('confirm')
    await settle()
    expect(calls.find((call) => call.url.startsWith('/workflow/process/suspend/processInstance/'))?.url).toBe('/workflow/process/suspend/processInstance/pi-1')
    // 抽屉 v-model:show 关闭回调
    const drawer = wrapper.findComponent(NDrawer)
    if (drawer.exists()) {
      drawer.vm.$emit('update:show', false)
      await settle()
    }
  })

  it('异常：刷新与条数变化', async () => {
    const calls = mockFetch([listRoute(/^\/workflow\/exception\/list/, [{ id: 'e1', title: 'A' }])])
    const wrapper = await mountPage(ExceptionPage)
    await clickButton(wrapper, '刷新')
    wrapper.findComponent(DataTable).vm.$emit('update:page-size', 10)
    await settle()
    expect(calls.filter((call) => call.url.includes('exception/list')).length).toBeGreaterThanOrEqual(3)
  })
})
