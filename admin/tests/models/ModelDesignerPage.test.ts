import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { createMemoryHistory, createRouter, type Router } from 'vue-router'
import { defineComponent, h, nextTick } from 'vue'
import ModelDesignerPage from '@/pages/repository/ModelDesignerPage.vue'
import { buildMinimalBpmn } from '@/design/minimalBpmn'
import { downloadText } from '@/lib/download'
import { mockFetch, pageOf, settle } from '../helpers'

vi.mock('@/lib/download', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/download')>()
  return { ...actual, downloadText: vi.fn() }
})

const EmptyPage = defineComponent({ name: 'EmptyPage', template: '<div />' })

const model = {
  id: 'm-1',
  name: '请假流程',
  key: 'leave',
  category: 'hr',
  version: 1,
  lastUpdateTime: '2026-10-30T10:00:00Z',
}

/** 设计器桩状态与方法 */
interface DesignerState {
  xml: string
  importResult: boolean
  selectResult: boolean
  imported: string[]
}

// 创建设计器桩，暴露页面依赖的命令并记录调用。
// @return 组件、状态与方法
function createDesignerStub() {
  const state: DesignerState = { xml: buildMinimalBpmn('leave', '请假流程'), importResult: true, selectResult: true, imported: [] }
  const methods = {
    getXml: vi.fn(async () => state.xml),
    importXml: vi.fn(async (xml: string) => {
      state.imported.push(xml)
      if (state.importResult) state.xml = xml
      return state.importResult
    }),
    fitViewport: vi.fn(),
    zoomIn: vi.fn(),
    zoomOut: vi.fn(),
    undo: vi.fn(),
    redo: vi.fn(),
    selectElement: vi.fn(() => state.selectResult),
  }
  const component = defineComponent({
    name: 'BpmnDesigner',
    props: {
      xml: { type: String, default: '' },
      modelKey: { type: String, default: '' },
      readonly: { type: Boolean, default: false },
    },
    emits: ['ready', 'dirty', 'error', 'warnings'],
    setup(_props, { expose }) {
      expose(methods)
      return () => h('div', { class: 'designer-stub' })
    },
  })
  return { component, state, methods }
}

// 创建包含设计器与列表路由的测试路由。
// @return 路由
function makeRouter(component = EmptyPage): Router {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/repository/models', name: 'models', component: EmptyPage },
      { path: '/repository/models/:id/design', name: 'model-designer', component },
    ],
  })
}

// 挂载设计器页并等待首屏加载。
// @param stub 设计器桩
// @param source 模型源码
// @return 包装器、路由与状态
async function mountPage(stub: ReturnType<typeof createDesignerStub>, source = buildMinimalBpmn('leave', '请假流程')) {
  const calls = mockFetch([
    {
      path: '/service/repository/models/m-1/source',
      text: source,
      contentType: 'application/octet-stream',
    },
    { path: '/service/repository/models/m-1/deployment', method: 'POST', status: 201, body: { id: 'd-2', name: '请假流程' } },
    {
      path: '/service/repository/models/m-1/source',
      method: 'PUT',
      status: 204,
    },
    { path: '/service/repository/models/m-1', body: model },
  ])
  const router = makeRouter()
  await router.push('/repository/models/m-1/design')
  await router.isReady()
  const wrapper = mount(ModelDesignerPage, {
    global: { plugins: [router], stubs: { BpmnDesigner: stub.component } },
  })
  await flush()
  return { wrapper, router, calls, stub }
}

// 等待异步请求与文件读取完成。
// @return 无
async function flush(): Promise<void> {
  await settle()
  await settle()
}

// 读取 File 内容，兼容 jsdom 的 Blob 实现。
// @param file 文件
// @return 文本
function readFile(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result ?? ''))
    reader.onerror = () => reject(reader.error)
    reader.readAsText(file)
  })
}

// 触发隐藏文件输入的 change 事件。
// @param wrapper 页面包装器
// @param file 文件
// @return 无
async function selectFile(wrapper: VueWrapper, file: File): Promise<void> {
  const input = wrapper.find('[data-testid="import-file"]')
  Object.defineProperty(input.element, 'files', { value: [file], configurable: true })
  await input.trigger('change')
  await flush()
}

beforeEach(() => {
  vi.mocked(downloadText).mockClear()
})

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('设计器页加载', () => {
  it('加载模型与源码并渲染标题', async () => {
    const stub = createDesignerStub()
    const { wrapper, calls } = await mountPage(stub)
    expect(calls[0].url).toBe('/service/repository/models/m-1')
    expect(calls[1].url).toBe('/service/repository/models/m-1/source')
    expect(wrapper.text()).toContain('设计：请假流程')
    expect(wrapper.text()).toContain('Key：leave')
    expect(wrapper.text()).toContain('版本 v1')
    expect(wrapper.find('.designer-stub').exists()).toBe(true)
    expect(wrapper.find('[data-testid="designer-loading"]').exists()).toBe(false)
  })

  it('模型缺少源码时载入最小 BPMN 模板并提示', async () => {
    const stub = createDesignerStub()
    mockFetch([
      {
        path: '/service/repository/models/m-1/source',
        status: 404,
        body: { code: 'REST-0404', message: '模型源码不存在' },
      },
      { path: '/service/repository/models/m-1', body: model },
    ])
    const router = makeRouter()
    await router.push('/repository/models/m-1/design')
    const wrapper = mount(ModelDesignerPage, { global: { plugins: [router], stubs: { BpmnDesigner: stub.component } } })
    await flush()
    expect(wrapper.text()).toContain('暂无源码')
    const designer = wrapper.findComponent(stub.component)
    expect(designer.props('xml')).toContain('id="leave"')
    expect(designer.props('xml')).toContain('startEvent')
  })

  it('加载失败展示 ErrorBlock', async () => {
    const stub = createDesignerStub()
    mockFetch([
      { path: '/service/repository/models/m-1', status: 500, body: { code: 'REST-0500', message: '模型不存在' } },
    ])
    const router = makeRouter()
    await router.push('/repository/models/m-1/design')
    const wrapper = mount(ModelDesignerPage, { global: { plugins: [router], stubs: { BpmnDesigner: stub.component } } })
    await flush()
    expect(wrapper.text()).toContain('模型不存在')
    expect(wrapper.find('.designer-stub').exists()).toBe(false)
  })
})

describe('保存与校验', () => {
  it('保存成功保留画布并清除未保存状态', async () => {
    const stub = createDesignerStub()
    const { wrapper, calls } = await mountPage(stub)
    stub.state.xml = buildMinimalBpmn('leave', '修改后的流程')
    wrapper.findComponent(stub.component).vm.$emit('dirty')
    await nextTick()
    expect(wrapper.find('[data-testid="dirty-indicator"]').exists()).toBe(true)
    await wrapper.find('[data-testid="save"]').trigger('click')
    await flush()
    expect(wrapper.find('[data-testid="dirty-indicator"]').exists()).toBe(false)
    expect(stub.methods.importXml).not.toHaveBeenCalled()
    const put = calls.find((item) => item.method === 'PUT')
    expect(put?.url).toBe('/service/repository/models/m-1/source')
    const file = (put?.body as FormData).get('file') as File
    expect(file.name).toBe('leave.bpmn20.xml')
    expect(await readFile(file)).toContain('修改后的流程')
    expect(wrapper.text()).toContain('已保存模型源码')
  })

  it('校验不通过时阻止保存并展示可点击问题', async () => {
    const stub = createDesignerStub()
    const { wrapper, calls } = await mountPage(stub)
    stub.state.xml = buildMinimalBpmn('leave', '坏流程').replace(/<bpmn:startEvent[^>]*\/>/, '')
    await wrapper.find('[data-testid="save"]').trigger('click')
    await flush()
    expect(calls.some((item) => item.method === 'PUT')).toBe(false)
    expect(wrapper.text()).toContain('至少需要一个开始事件')
    const issueButton = wrapper.findAll('button').find((item) => item.text().includes('至少需要一个开始事件'))
    expect(issueButton).toBeTruthy()
    await issueButton!.trigger('click')
    expect(stub.methods.selectElement).toHaveBeenCalled()
    expect(wrapper.text()).toContain('已定位元素')
  })

  it('问题元素无法定位时提示未找到', async () => {
    const stub = createDesignerStub()
    const { wrapper } = await mountPage(stub)
    stub.state.xml = buildMinimalBpmn('leave', '坏流程').replace(/<bpmn:startEvent[^>]*\/>/, '')
    await wrapper.find('[data-testid="save"]').trigger('click')
    await flush()
    stub.state.selectResult = false
    const issueButton = wrapper.findAll('button').find((item) => item.text().includes('至少需要一个开始事件'))
    await issueButton!.trigger('click')
    expect(wrapper.text()).toContain('未找到元素')
  })

  it('画布 XML 为空时不执行保存', async () => {
    const stub = createDesignerStub()
    const { wrapper, calls } = await mountPage(stub)
    stub.state.xml = ''
    await wrapper.find('[data-testid="save"]').trigger('click')
    await flush()
    expect(calls.some((item) => item.method === 'PUT')).toBe(false)
    expect(wrapper.text()).toContain('无法读取画布 XML')
  })

  it('校验按钮展示通过或警告结果', async () => {
    const stub = createDesignerStub()
    const { wrapper } = await mountPage(stub)
    await wrapper.find('[data-testid="validate"]').trigger('click')
    await flush()
    expect(wrapper.text()).toContain('校验通过')
  })
})

describe('部署', () => {
  it('存在未保存修改时先保存再部署并展示定义版本', async () => {
    const stub = createDesignerStub()
    const { wrapper } = await mountPage(stub)
    stub.state.xml = buildMinimalBpmn('leave', '待部署流程')
    wrapper.findComponent(stub.component).vm.$emit('dirty')
    await nextTick()
    // 追加部署后的定义查询与模型刷新
    const deployCalls = mockFetch([
      { path: '/service/repository/models/m-1/source', method: 'PUT', status: 204 },
      { path: '/service/repository/models/m-1/deployment', method: 'POST', status: 201, body: { id: 'd-2', name: '请假流程' } },
      {
        path: '/service/repository/process-definitions',
        body: pageOf([
          { id: 'leave:1', key: 'leave', version: 1 },
          { id: 'leave:2', key: 'leave', version: 2 },
        ]),
      },
      { path: '/service/repository/models/m-1', body: model },
    ])
    await wrapper.find('[data-testid="deploy"]').trigger('click')
    await flush()
    const methods = deployCalls.map((item) => item.method)
    expect(methods.indexOf('PUT')).toBeLessThan(methods.indexOf('POST'))
    const result = wrapper.find('[data-testid="deployment-result"]')
    expect(result.text()).toContain('d-2')
    expect(result.text()).toContain('v2')
    expect(wrapper.text()).toContain('部署成功')
  })

  it('无未保存修改时直接部署', async () => {
    const stub = createDesignerStub()
    const { wrapper, calls } = await mountPage(stub)
    await wrapper.find('[data-testid="deploy"]').trigger('click')
    await flush()
    expect(calls.some((item) => item.method === 'PUT')).toBe(false)
    expect(calls.some((item) => item.method === 'POST')).toBe(true)
  })

  it('部署失败展示服务端原文', async () => {
    const stub = createDesignerStub()
    const { wrapper } = await mountPage(stub)
    mockFetch([
      {
        path: '/service/repository/models/m-1/deployment',
        method: 'POST',
        status: 400,
        body: { code: 'REST-0151', message: '模型源码为空，无法部署', exception: 'empty' },
      },
      { path: '/service/repository/models/m-1', body: model },
    ])
    await wrapper.find('[data-testid="deploy"]').trigger('click')
    await flush()
    expect(wrapper.text()).toContain('模型源码为空，无法部署')
    expect(wrapper.text()).toContain('REST-0151')
  })
})

describe('导入与导出', () => {
  it('导入合法 BPMN 后标记未保存并校验', async () => {
    const stub = createDesignerStub()
    const { wrapper } = await mountPage(stub)
    const xml = buildMinimalBpmn('leave', '导入流程')
    await selectFile(wrapper, new File([xml], 'import.bpmn', { type: 'application/xml' }))
    expect(stub.methods.importXml).toHaveBeenCalledWith(xml)
    expect(wrapper.find('[data-testid="dirty-indicator"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('已导入 import.bpmn')
  })

  it('拒绝超过 5MB 的文件', async () => {
    const stub = createDesignerStub()
    const { wrapper } = await mountPage(stub)
    const file = new File(['x'], 'big.bpmn', { type: 'application/xml' })
    Object.defineProperty(file, 'size', { value: 5 * 1024 * 1024 + 1 })
    await selectFile(wrapper, file)
    expect(stub.methods.importXml).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('超过 5MB')
  })

  it('解析失败时回退到导入前画布并提示', async () => {
    const stub = createDesignerStub()
    const { wrapper } = await mountPage(stub)
    const previous = stub.state.xml
    stub.state.importResult = false
    await selectFile(wrapper, new File(['<broken'], 'bad.bpmn', { type: 'application/xml' }))
    expect(stub.methods.importXml).toHaveBeenNthCalledWith(1, '<broken')
    expect(stub.methods.importXml).toHaveBeenNthCalledWith(2, previous)
    expect(wrapper.find('[data-testid="dirty-indicator"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('已保留导入前的画布')
  })

  it('设计器错误与解析警告以列表展示并可定位', async () => {
    const stub = createDesignerStub()
    const { wrapper } = await mountPage(stub)
    const designer = wrapper.findComponent(stub.component)
    designer.vm.$emit('error', 'XML 语法错误')
    await nextTick()
    expect(wrapper.text()).toContain('XML 语法错误')
    designer.vm.$emit('warnings', [{ message: '未知元素', element: { id: 'Task_1' } }])
    await nextTick()
    expect(wrapper.text()).toContain('未知元素')
    expect(wrapper.find('[data-testid="validation-list"]').text()).toContain('Task_1')
    const issue = wrapper.findAll('button').find((item) => item.text().includes('未知元素'))
    await issue!.trigger('click')
    expect(stub.methods.selectElement).toHaveBeenCalledWith('Task_1')
  })

  it('导出 .bpmn 与 .bpmn20.xml 使用模型 Key 命名', async () => {
    const stub = createDesignerStub()
    const { wrapper } = await mountPage(stub)
    await wrapper.find('[data-testid="export-bpmn"]').trigger('click')
    await flush()
    expect(downloadText).toHaveBeenLastCalledWith('leave.bpmn', stub.state.xml, 'application/xml;charset=utf-8')
    await wrapper.find('[data-testid="export-bpmn20"]').trigger('click')
    await flush()
    expect(downloadText).toHaveBeenLastCalledWith('leave.bpmn20.xml', stub.state.xml, 'application/xml;charset=utf-8')
    expect(wrapper.text()).toContain('已导出 leave.bpmn20.xml')
  })

  it('撤销/重做/缩放/适应窗口调用设计器命令', async () => {
    const stub = createDesignerStub()
    const { wrapper } = await mountPage(stub)
    for (const id of ['undo', 'redo', 'zoom-in', 'zoom-out', 'fit']) {
      await wrapper.find(`[data-testid="${id}"]`).trigger('click')
    }
    expect(stub.methods.undo).toHaveBeenCalled()
    expect(stub.methods.redo).toHaveBeenCalled()
    expect(stub.methods.zoomIn).toHaveBeenCalled()
    expect(stub.methods.zoomOut).toHaveBeenCalled()
    expect(stub.methods.fitViewport).toHaveBeenCalled()
  })
})

describe('离开页面提示', () => {
  it('存在未保存修改时阻止路由离开，确认后放行', async () => {
    const stub = createDesignerStub()
    mockFetch([
      { path: '/service/repository/models/m-1/source', text: buildMinimalBpmn('leave', '请假'), contentType: 'application/octet-stream' },
      { path: '/service/repository/models/m-1', body: model },
    ])
    const router = makeRouter(ModelDesignerPage)
    await router.push('/repository/models/m-1/design')
    await router.isReady()
    const wrapper = mount({ template: '<router-view />' }, { global: { plugins: [router], stubs: { BpmnDesigner: stub.component } } })
    await flush()
    const page = wrapper.findComponent(ModelDesignerPage)
    page.findComponent(stub.component).vm.$emit('dirty')
    await nextTick()
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    await router.push('/repository/models')
    expect(router.currentRoute.value.name).toBe('model-designer')
    confirm.mockReturnValue(true)
    await router.push('/repository/models')
    expect(router.currentRoute.value.name).toBe('models')
  })
})
