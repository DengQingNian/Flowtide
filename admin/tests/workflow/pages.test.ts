// 工作流九大页面冒烟测试：列表三态（成功/失败/501 降级）、弹窗增改删与关键行操作。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { NInput } from 'naive-ui'
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
import EmptyState from '@/components/EmptyState.vue'
import { authNotice } from '@/stores/ui'
import { mockFetch, settle } from '../helpers'

// r 构造 R 成功信封。
// @param data 业务数据
// @return 信封对象
function r(data: unknown) {
  return { code: '200', msg: 'ok', data, success: true }
}

// notImplemented 构造 501 失败信封。
// @return 信封对象
function notImplemented() {
  return { code: '500', ErrorMessage: '接口未实现', success: false }
}

// mountPage 挂载页面并等待首次加载完成。
// teleport 桩让 n-modal/n-drawer 内容留在包装器 DOM 内，便于断言与交互。
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

// setInput 向第 index 个 NInput 写入值。
// @param wrapper 包装器
// @param index 输入框序号
// @param value 值
// @return 无
async function setInput(wrapper: VueWrapper, index: number, value: string): Promise<void> {
  const inputs = wrapper.findAllComponents(NInput)
  inputs[index].vm.$emit('update:value', value)
  await settle()
}

beforeEach(() => {
  authNotice.value = 'none'
  vi.unstubAllGlobals()
})

describe('流程分类页', () => {
  it('加载分类并组装树形展示', async () => {
    const calls = mockFetch([
      {
        path: /^\/workflow\/category\/list/,
        body: r([{ id: 'c1', categoryName: '行政', categoryCode: 'admin', parentId: null, visible: true, enabled: true }, { id: 'c2', categoryName: '人事', categoryCode: 'hr', parentId: 'c1' }]),
      },
    ])
    const wrapper = await mountPage(CategoryPage)
    // 子分类默认折叠，断言根分类与表格渲染即可
    expect(wrapper.text()).toContain('行政')
    expect(wrapper.text()).toContain('admin')
    expect(calls[0].url).toBe('/workflow/category/list')
  })

  it('501 时空态降级展示接线提示', async () => {
    mockFetch([{ path: /^\/workflow\/category\/list/, status: 501, body: notImplemented() }])
    const wrapper = await mountPage(CategoryPage)
    expect(wrapper.text()).toContain('分类列表接口接线中（后端 501）')
  })

  it('新增分类提交字段并刷新', async () => {
    const calls = mockFetch([
      { path: /^\/workflow\/category\/list/, body: r([]) },
      { path: '/workflow/category/add', method: 'POST', body: r(null) },
    ])
    const wrapper = await mountPage(CategoryPage)
    await clickButton(wrapper, '新增分类')
    const modal = wrapper.findComponent(FormModal)
    await setInput(modal, 0, '财务分类')
    await setInput(modal, 1, 'fin')
    await clickButton(modal, '保存')
    const post = calls.find((call) => call.method === 'POST')
    expect(post?.body).toMatchObject({ categoryName: '财务分类', categoryCode: 'fin' })
  })

  it('删除分类调用 delete/{ids}', async () => {
    const calls = mockFetch([
      { path: /^\/workflow\/category\/list/, body: r([{ id: 'c1', categoryName: '行政', categoryCode: 'admin' }]) },
      { path: /^\/workflow\/category\/delete\//, method: 'DELETE', body: r(null) },
    ])
    const wrapper = await mountPage(CategoryPage)
    wrapper.findAllComponents(ConfirmButton)[0].vm.$emit('confirm')
    await settle()
    const del = calls.find((call) => call.url.startsWith('/workflow/category/delete/'))
    expect(del?.url).toBe('/workflow/category/delete/c1')
  })
})

describe('流程模型页', () => {
  it('加载模型列表并展示部署时间', async () => {
    mockFetch([
      { path: /^\/workflow\/model\/list/, body: r({ list: [{ id: 'm1', modelName: '请假', modelCode: 'leave', enabled: true, lastDeployTime: '2026-10-30 10:00:00' }], total: 1 }) },
      { path: /^\/workflow\/category\/list/, body: r([]) },
    ])
    const wrapper = await mountPage(ModelPage)
    expect(wrapper.text()).toContain('请假')
    expect(wrapper.text()).toContain('leave')
  })

  it('部署按钮调用 process/deploy 并提示结果', async () => {
    const calls = mockFetch([
      { path: /^\/workflow\/model\/list/, body: r({ list: [{ id: 'm1', modelName: '请假', modelCode: 'leave' }], total: 1 }) },
      { path: /^\/workflow\/category\/list/, body: r([]) },
      { path: '/workflow/process/deploy', method: 'POST', body: { code: '200', msg: '部署完成', data: null, success: true } },
    ])
    const wrapper = await mountPage(ModelPage)
    wrapper.findAllComponents(ConfirmButton)[0].vm.$emit('confirm')
    await settle()
    const post = calls.find((call) => call.url.startsWith('/workflow/process/deploy'))
    expect(post?.body).toEqual({ modelId: 'm1', tenantId: '' })
    expect(wrapper.text()).toContain('部署完成')
  })

  it('部署 501 时展示降级文案', async () => {
    mockFetch([
      { path: /^\/workflow\/model\/list/, body: r({ list: [{ id: 'm1', modelName: '请假', modelCode: 'leave' }], total: 1 }) },
      { path: /^\/workflow\/category\/list/, body: r([]) },
      { path: '/workflow/process/deploy', method: 'POST', status: 501, body: notImplemented() },
    ])
    const wrapper = await mountPage(ModelPage)
    wrapper.findAllComponents(ConfirmButton)[0].vm.$emit('confirm')
    await settle()
    expect(wrapper.text()).toContain('部署接口接线中（后端 501）')
  })
})

describe('流程配置页', () => {
  const stepRoutes = () => [
    { path: /^\/workflow\/model\/list/, body: r({ list: [{ id: 'm1', modelName: '请假', modelCode: 'leave' }], total: 1 }) },
    { path: /^\/workflow\/step\/list/, body: r([{ id: 's1', stepName: '审批', stepCode: 'audit', sort: 1, allow_return: true, back_step_codes: 'draft', url: '/a' }]) },
    { path: /^\/workflow\/template\/list/, body: r([{ id: 't1', templateName: '默认模板', templateCode: 'default', flowXml: '<definitions/>' }]) },
  ]

  it('步骤管理：默认加载模型与步骤列表', async () => {
    const calls = mockFetch(stepRoutes())
    const wrapper = await mountPage(TemplateConfigPage)
    expect(wrapper.text()).toContain('审批')
    expect(calls.find((call) => call.url.startsWith('/workflow/step/list'))?.url).toContain('modelId=m1')
  })

  it('流程模板：切换 Tab 展示列表并打开 XML 预览', async () => {
    mockFetch(stepRoutes())
    const wrapper = await mountPage(TemplateConfigPage)
    const tabs = wrapper.findAll('.n-tabs-tab')
    await tabs[1].trigger('click')
    await settle()
    expect(wrapper.text()).toContain('默认模板')
    await clickButton(wrapper, 'XML 预览')
    expect(wrapper.text()).toContain('流程模板 XML')
  })

  it('流程规则：占位空态', async () => {
    mockFetch(stepRoutes())
    const wrapper = await mountPage(TemplateConfigPage)
    const tabs = wrapper.findAll('.n-tabs-tab')
    await tabs[2].trigger('click')
    await settle()
    expect(wrapper.findComponent(EmptyState).text()).toContain('流程规则待开放')
  })

  it('新增步骤提交核心字段', async () => {
    const calls = mockFetch([
      ...stepRoutes(),
      { path: '/workflow/step/add', method: 'POST', body: r(null) },
    ])
    const wrapper = await mountPage(TemplateConfigPage)
    await settle()
    await clickButton(wrapper, '新增步骤')
    const modal = wrapper.findComponent(FormModal)
    await setInput(modal, 0, '复核')
    await setInput(modal, 1, 'review')
    await clickButton(modal, '保存')
    const post = calls.find((call) => call.url === '/workflow/step/add')
    expect(post?.body).toMatchObject({ modelId: 'm1', stepName: '复核', stepCode: 'review' })
  })
})

describe('待办任务页', () => {
  it('加载待办列表（默认身份 u1）', async () => {
    const calls = mockFetch([
      { path: /^\/workflow\/process\/task\/all\/todoList/, body: r({ list: [{ id: 't1', title: '请假审批', processName: '请假流程', stepName: '审批', senderName: '张三', createTime: '2026-10-30 09:00:00' }], total: 1 }) },
    ])
    const wrapper = await mountPage(TodoPage)
    expect(wrapper.text()).toContain('请假审批')
    expect(calls[0].url).toContain('identityId=u1')
  })

  it('办理弹窗提交意见与变量', async () => {
    const calls = mockFetch([
      { path: /^\/workflow\/process\/task\/all\/todoList/, body: r({ list: [{ id: 't1', title: '请假审批', processInstanceId: 'pi-1' }], total: 1 }) },
      { path: '/workflow/process/complete/task', method: 'POST', body: r(null) },
    ])
    const wrapper = await mountPage(TodoPage)
    await clickButton(wrapper, '办理')
    const dialog = wrapper.findComponent(ApprovalDialog)
    const inputs = dialog.findAllComponents(NInput)
    inputs[0].vm.$emit('update:value', '同意')
    inputs[1].vm.$emit('update:value', '{"days": 3}')
    await settle()
    await clickButton(dialog, '提交')
    const post = calls.find((call) => call.url === '/workflow/process/complete/task')
    expect(post?.body).toMatchObject({ taskId: 't1', message: '同意', variables: { days: 3 } })
    expect(wrapper.text()).toContain('已办结')
  })

  it('办理接口 501 时弹窗展示降级文案', async () => {
    mockFetch([
      { path: /^\/workflow\/process\/task\/all\/todoList/, body: r({ list: [{ id: 't1', title: '请假审批' }], total: 1 }) },
      { path: '/workflow/process/complete/task', method: 'POST', status: 501, body: notImplemented() },
    ])
    const wrapper = await mountPage(TodoPage)
    await clickButton(wrapper, '办理')
    const dialog = wrapper.findComponent(ApprovalDialog)
    dialog.findAllComponents(NInput)[0].vm.$emit('update:value', '同意')
    await settle()
    await clickButton(dialog, '提交')
    expect(dialog.text()).toContain('办理接口接线中（后端 501）')
  })

  it('审批意见为空时本地校验拦截', async () => {
    mockFetch([{ path: /^\/workflow\/process\/task\/all\/todoList/, body: r({ list: [{ id: 't1', title: '请假审批' }], total: 1 }) }])
    const wrapper = await mountPage(TodoPage)
    await clickButton(wrapper, '办理')
    const dialog = wrapper.findComponent(ApprovalDialog)
    await clickButton(dialog, '提交')
    expect(dialog.text()).toContain('审批意见为必填项')
  })
})

describe('已办任务页', () => {
  it('加载已办列表并展示完成时间列', async () => {
    const calls = mockFetch([
      { path: /^\/workflow\/process\/task\/all\/doneList/, body: r({ list: [{ id: 'd1', title: '已审批', endTime: '2026-10-30 11:00:00' }], total: 1 }) },
    ])
    const wrapper = await mountPage(DonePage)
    expect(wrapper.text()).toContain('已审批')
    expect(wrapper.text()).toContain('完成时间')
    expect(calls[0].url).toContain('identityId=u1')
  })

  it('501 时空态降级', async () => {
    mockFetch([{ path: /^\/workflow\/process\/task\/all\/doneList/, status: 501, body: notImplemented() }])
    const wrapper = await mountPage(DonePage)
    expect(wrapper.text()).toContain('已办列表接口接线中（后端 501）')
  })
})

describe('流程实例页', () => {
  it('实例列表 501 时空态提示"实例分页接口接线中"', async () => {
    mockFetch([{ path: /^\/workflow\/process\/instance\/all\/list/, status: 501, body: notImplemented() }])
    const wrapper = await mountPage(InstancePage)
    expect(wrapper.text()).toContain('实例分页接口接线中（后端 501）')
  })

  it('加载实例列表并渲染行操作', async () => {
    mockFetch([
      { path: /^\/workflow\/process\/instance\/all\/list/, body: r({ list: [{ id: 'pi-1', title: '订单审批', businessKey: 'B001', status: 1 }], total: 1 }) },
    ])
    const wrapper = await mountPage(InstancePage)
    expect(wrapper.text()).toContain('订单审批')
    expect(wrapper.findAllComponents(ConfirmButton).length).toBeGreaterThanOrEqual(3)
  })

  it('按业务键查询状态：成功展示结果，501 展示降级提示', async () => {
    const calls = mockFetch([
      { path: /^\/workflow\/process\/instance\/all\/list/, body: r({ list: [], total: 0 }) },
      { path: /^\/workflow\/process\/public\/processes\/id/, method: 'POST', body: r({ processInstanceId: 'pi-9', status: 1 }) },
    ])
    const wrapper = await mountPage(InstancePage)
    await setInput(wrapper, 1, 'B001')
    await clickButton(wrapper, '查询状态')
    const post = calls.find((call) => call.url.startsWith('/workflow/process/public/processes/id'))
    expect(post?.url).toContain('businessKey=B001')
    expect(wrapper.text()).toContain('pi-9')

    mockFetch([{ path: /^\/workflow\/process\/public\/processes\/id/, method: 'POST', status: 501, body: notImplemented() }])
    await clickButton(wrapper, '查询状态')
    await settle()
    expect(wrapper.text()).toContain('按业务键查询状态接口接线中（后端 501）')
  })

  it('流程图抽屉在高亮接口 501 时降级空态', async () => {
    mockFetch([
      { path: /^\/workflow\/process\/instance\/all\/list/, body: r({ list: [{ id: 'pi-1', title: '订单审批' }], total: 1 }) },
      { path: /^\/workflow\/process\/highlight\/img/, status: 501, body: notImplemented() },
    ])
    const wrapper = await mountPage(InstancePage)
    await clickButton(wrapper, '流程图')
    expect(wrapper.text()).toContain('流程图高亮接口接线中（后端 501）')
  })
})

describe('委托配置页', () => {
  it('加载委托列表并支持启停切换', async () => {
    const calls = mockFetch([
      { path: /^\/workflow\/delegate\/list/, body: r([{ id: 'd1', ownerId: 'u1', trusteeId: 'u2', enabled: true, systemSymbol: 'crm', remark: '出差' }]) },
      { path: '/workflow/delegate/edit', method: 'POST', body: r(null) },
    ])
    const wrapper = await mountPage(EntrustPage)
    expect(wrapper.text()).toContain('u2')
    await clickButton(wrapper, '停用')
    const edit = calls.find((call) => call.url === '/workflow/delegate/edit')
    expect(edit?.body).toMatchObject({ id: 'd1', enabled: false })
  })

  it('新增委托校验受托人必填', async () => {
    mockFetch([{ path: /^\/workflow\/delegate\/list/, body: r([]) }])
    const wrapper = await mountPage(EntrustPage)
    await clickButton(wrapper, '新增委托')
    const modal = wrapper.findComponent(FormModal)
    await clickButton(modal, '保存')
    expect(modal.text()).toContain('委托人与受托人为必填项')
  })

  it('501 时空态降级', async () => {
    mockFetch([{ path: /^\/workflow\/delegate\/list/, status: 501, body: notImplemented() }])
    const wrapper = await mountPage(EntrustPage)
    expect(wrapper.text()).toContain('委托列表接口接线中（后端 501）')
  })
})

describe('催办配置页', () => {
  it('加载催办列表并提交新增', async () => {
    const calls = mockFetch([
      { path: /^\/workflow\/remind\/getPageList/, body: r({ list: [{ id: 'r1', systemSymbol: 'crm', aheadEnabled: true, ahead: 30 }], total: 1 }) },
      { path: '/workflow/remind/add', method: 'POST', body: r(null) },
    ])
    const wrapper = await mountPage(RemindPage)
    expect(wrapper.text()).toContain('提前 30')
    await clickButton(wrapper, '新增催办')
    const modal = wrapper.findComponent(FormModal)
    await setInput(modal, 0, 'oa')
    await clickButton(modal, '保存')
    const post = calls.find((call) => call.url === '/workflow/remind/add')
    expect(post?.body).toMatchObject({ systemSymbol: 'oa', aheadEnabled: true })
  })

  it('501 时空态降级', async () => {
    mockFetch([{ path: /^\/workflow\/remind\/getPageList/, status: 501, body: notImplemented() }])
    const wrapper = await mountPage(RemindPage)
    expect(wrapper.text()).toContain('催办列表接口接线中（后端 501）')
  })
})

describe('流程异常页', () => {
  it('加载异常列表并标记已处理', async () => {
    const calls = mockFetch([
      { path: /^\/workflow\/exception\/list/, body: r([{ id: 'e1', title: '网关异常', exceptionType: 'ENGINE', errorMessage: 'boom', handleStatus: 'pending' }]) },
      { path: /^\/workflow\/exception\/delete\//, method: 'DELETE', body: r(null) },
    ])
    const wrapper = await mountPage(ExceptionPage)
    expect(wrapper.text()).toContain('网关异常')
    wrapper.findAllComponents(ConfirmButton)[0].vm.$emit('confirm')
    await settle()
    expect(calls.find((call) => call.url.startsWith('/workflow/exception/delete/'))?.url).toBe('/workflow/exception/delete/e1')
    expect(wrapper.text()).toContain('异常已标记为处理')
  })

  it('列表失败时展示错误文案', async () => {
    mockFetch([{ path: /^\/workflow\/exception\/list/, status: 403, body: { code: 'AUTH', ErrorMessage: '无权限', success: false } }])
    const wrapper = await mountPage(ExceptionPage)
    expect(wrapper.text()).toContain('无权限')
  })
})

describe('列表数据源通用行为', () => {
  it('HTTP 失败时 DataTable 内不渲染数据且错误条可见', async () => {
    mockFetch([{ path: /^\/workflow\/exception\/list/, status: 500, body: { code: 'REST-0500', message: '内部错误' } }])
    const wrapper = await mountPage(ExceptionPage)
    expect(wrapper.findComponent(DataTable).exists()).toBe(true)
    expect(wrapper.text()).toContain('内部错误')
    // 重试按钮触发重新加载
    await wrapper.findComponent(FilterBar).vm.$emit('search')
    await settle()
    expect(wrapper.findAllComponents(DataTable).length).toBeGreaterThan(0)
  })
})
