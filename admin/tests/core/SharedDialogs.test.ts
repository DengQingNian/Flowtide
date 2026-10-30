import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { NButton, NInput, NInputNumber, NSelect, NSwitch } from 'naive-ui'
import AuthNotice from '@/components/AuthNotice.vue'
import ServiceBanner from '@/components/ServiceBanner.vue'
import StartInstanceDialog from '@/components/StartInstanceDialog.vue'
import VariableEditor from '@/components/VariableEditor.vue'
import VariableTable from '@/components/VariableTable.vue'
import TaskForm from '@/components/TaskForm.vue'
import { authNotice, clearAuthNotice, serviceDetail, serviceStatus } from '@/stores/ui'
import { mockFetch, settle } from '../helpers'

afterEach(() => {
  vi.unstubAllGlobals()
  clearAuthNotice()
  serviceStatus.value = 'unknown'
  serviceDetail.value = ''
  document.body.innerHTML = ''
})

// 按文本查找 Naive 按钮。
// @param wrapper 包装器
// @param text 文本
// @return 按钮组件
function nbutton(wrapper: ReturnType<typeof mount>, text: string) {
  const found = wrapper.findAllComponents(NButton).find((item) => item.text().includes(text))
  if (!found) throw new Error(`未找到按钮：${text}`)
  return found
}

describe('AuthNotice', () => {
  it('401 展示认证说明并可关闭', async () => {
    authNotice.value = 'unauthorized'
    const wrapper = mount(AuthNotice, { attachTo: document.body })
    expect(wrapper.text()).toContain('本期管理台未提供登录入口')
    await wrapper.find('button').trigger('click')
    expect(authNotice.value).toBe('none')
    wrapper.unmount()
  })

  it('403 展示权限说明', () => {
    authNotice.value = 'forbidden'
    const wrapper = mount(AuthNotice)
    expect(wrapper.text()).toContain('没有该操作的权限')
  })

  it('无提示时不渲染', () => {
    expect(mount(AuthNotice).text()).toBe('')
  })
})

describe('ServiceBanner', () => {
  it('健康时隐藏，异常时展示并支持重试', async () => {
    serviceStatus.value = 'ok'
    const hidden = mount(ServiceBanner)
    expect(hidden.text()).toBe('')
    serviceStatus.value = 'degraded'
    serviceDetail.value = 'readyz=503'
    mockFetch([
      { path: '/healthz', body: { status: 'UP' } },
      { path: '/readyz', body: { status: 'UP' } },
    ])
    const wrapper = mount(ServiceBanner, { attachTo: document.body })
    expect(wrapper.text()).toContain('readyz=503')
    await wrapper.find('button[title="重试"]').trigger('click')
    await settle()
    expect(serviceStatus.value).toBe('ok')
    wrapper.unmount()
  })
})

describe('StartInstanceDialog', () => {
  const definitions = [{ id: 'def-1', key: 'leave', name: '请假', version: 2 }]

  it('提交合法变量并携带定义与业务 Key', async () => {
    const wrapper = mount(StartInstanceDialog, { props: { show: true, definitions }, attachTo: document.body })
    const inputs = wrapper.findAllComponents(NInput)
    await inputs[0].vm.$emit('update:value', 'B-1')
    await inputs[1].vm.$emit('update:value', '请假实例')
    await inputs[2].vm.$emit('update:value', '{"days":2}')
    await nbutton(wrapper, '启动').trigger('click')
    expect(wrapper.emitted('submit')?.[0]?.[0]).toEqual({
      processDefinitionId: 'def-1',
      businessKey: 'B-1',
      name: '请假实例',
      variables: { days: 2 },
    })
  })

  it('JSON 非法与变量非对象时给出错误', async () => {
    const wrapper = mount(StartInstanceDialog, { props: { show: true, definitions }, attachTo: document.body })
    const variableInput = wrapper.findAllComponents(NInput)[2]
    await variableInput.vm.$emit('update:value', '{bad')
    await nbutton(wrapper, '启动').trigger('click')
    expect(document.body.textContent).toContain('变量 JSON 格式非法')
    await variableInput.vm.$emit('update:value', '[1,2]')
    await nbutton(wrapper, '启动').trigger('click')
    expect(document.body.textContent).toContain('变量必须是 JSON 对象')
  })

  it('无定义且无 Key 时提示选择定义，支持关闭', async () => {
    const wrapper = mount(StartInstanceDialog, { props: { show: true }, attachTo: document.body })
    await nbutton(wrapper, '启动').trigger('click')
    expect(document.body.textContent).toContain('请选择流程定义')
    await nbutton(wrapper, '取消').trigger('click')
    expect(wrapper.emitted('update:show')?.[0]).toEqual([false])
  })

  it('仅提供定义 Key 时可启动', async () => {
    const wrapper = mount(StartInstanceDialog, { props: { show: true, initialDefinitionKey: 'leave' }, attachTo: document.body })
    await nbutton(wrapper, '启动').trigger('click')
    expect(wrapper.emitted('submit')?.[0]?.[0]).toMatchObject({ processDefinitionKey: 'leave', variables: {} })
  })
})

describe('VariableEditor', () => {
  // 挂载变量编辑器。
  // @param variable 已有变量
  // @return 包装器
  function mountEditor(variable: Record<string, unknown> | null = null) {
    return mount(VariableEditor, { props: { show: false, variable: variable as never }, attachTo: document.body })
  }

  it('新增字符串变量并校验空名称', async () => {
    const wrapper = mountEditor()
    await wrapper.setProps({ show: true })
    await nbutton(wrapper, '保存').trigger('click')
    expect(document.body.textContent).toContain('变量名不能为空')
    const inputs = wrapper.findAllComponents(NInput)
    await inputs[0].vm.$emit('update:value', 'note')
    await inputs[1].vm.$emit('update:value', 'hello')
    await nbutton(wrapper, '保存').trigger('click')
    expect(wrapper.emitted('save')?.[0]?.[0]).toEqual({ name: 'note', type: 'string', value: 'hello', scope: undefined })
    expect(wrapper.emitted('update:show')?.[0]).toEqual([false])
  })

  it('JSON 非法时报错，合法 JSON 可保存', async () => {
    const wrapper = mountEditor()
    await wrapper.setProps({ show: true })
    await wrapper.findComponent(NSelect).vm.$emit('update:value', 'json')
    await wrapper.findAllComponents(NInput)[0].vm.$emit('update:value', 'payload')
    const valueInput = wrapper.findAllComponents(NInput)[1]
    await valueInput.vm.$emit('update:value', '{bad')
    await nbutton(wrapper, '保存').trigger('click')
    expect(document.body.textContent).toContain('JSON 格式非法')
    await valueInput.vm.$emit('update:value', '{"a":1}')
    await nbutton(wrapper, '保存').trigger('click')
    expect(wrapper.emitted('save')?.[0]?.[0]).toMatchObject({ value: { a: 1 } })
  })

  it('数字类型取整、布尔与日期类型可保存', async () => {
    const wrapper = mountEditor()
    await wrapper.setProps({ show: true })
    await wrapper.findAllComponents(NInput)[0].vm.$emit('update:value', 'n')
    await wrapper.findComponent(NSelect).vm.$emit('update:value', 'long')
    await wrapper.findAllComponents(NInputNumber)[0].vm.$emit('update:value', 3.9)
    await nbutton(wrapper, '保存').trigger('click')
    expect(wrapper.emitted('save')?.[0]?.[0]).toMatchObject({ value: 3 })

    await wrapper.findComponent(NSelect).vm.$emit('update:value', 'double')
    await wrapper.findAllComponents(NInputNumber)[0].vm.$emit('update:value', 3.9)
    await nbutton(wrapper, '保存').trigger('click')
    expect(wrapper.emitted('save')?.[1]?.[0]).toMatchObject({ value: 3.9 })

    await wrapper.findComponent(NSelect).vm.$emit('update:value', 'boolean')
    await wrapper.findComponent(NSwitch).vm.$emit('update:value', true)
    await nbutton(wrapper, '保存').trigger('click')
    expect(wrapper.emitted('save')?.[2]?.[0]).toMatchObject({ value: true })

    await wrapper.findComponent(NSelect).vm.$emit('update:value', 'date')
    await wrapper.findAllComponents(NInput)[1].vm.$emit('update:value', '2026-10-30')
    await nbutton(wrapper, '保存').trigger('click')
    expect(wrapper.emitted('save')?.[3]?.[0]).toMatchObject({ value: '2026-10-30' })
  })

  it('编辑已有变量时回填类型与值并关闭', async () => {
    const wrapper = mountEditor({ name: 'amount', type: 'long', value: 5, scope: 'global' })
    await wrapper.setProps({ show: true })
    expect((wrapper.findAllComponents(NInput)[0].props('value') as string)).toBe('amount')
    await nbutton(wrapper, '保存').trigger('click')
    expect(wrapper.emitted('save')?.[0]?.[0]).toMatchObject({ name: 'amount', type: 'long', value: 5 })
    await nbutton(wrapper, '取消').trigger('click')
    expect(wrapper.emitted('update:show')?.[0]).toEqual([false])
  })
})

describe('VariableTable', () => {
  const variables = [
    { name: 'amount', type: 'long', value: 100, scope: 'global' },
    { name: 'raw', type: 'binary', value: 'AQID', scope: 'local', id: 'v1' },
  ]

  it('渲染变量、二进制提示与作用域', () => {
    const wrapper = mount(VariableTable, { props: { variables, showScope: true } })
    expect(wrapper.text()).toContain('amount')
    expect(wrapper.text()).toContain('二进制内容')
    expect(wrapper.text()).toContain('global')
    expect(wrapper.text()).toContain('local')
  })

  it('触发编辑、原始数据与删除事件', async () => {
    const wrapper = mount(VariableTable, { props: { variables } })
    const buttons = wrapper.findAll('button')
    await buttons.find((item) => item.text() === '编辑')!.trigger('click')
    await buttons.filter((item) => item.text() === '原始')[1].trigger('click')
    await buttons.find((item) => item.text() === '删除')!.trigger('click')
    expect(wrapper.emitted('edit')?.[0]?.[0]).toMatchObject({ name: 'amount' })
    expect(wrapper.emitted('raw')?.[0]?.[0]).toMatchObject({ name: 'raw' })
    expect(wrapper.emitted('remove')?.[0]?.[0]).toMatchObject({ name: 'amount' })
  })

  it('隐藏操作列并展示空态', () => {
    const wrapper = mount(VariableTable, { props: { variables: [], actions: false } })
    expect(wrapper.text()).toContain('暂无变量')
    expect(wrapper.findAll('button').some((item) => item.text() === '编辑')).toBe(false)
  })
})

describe('TaskForm', () => {
  const form = {
    formKey: 'f',
    formProperties: [
      { id: 'name', name: '姓名', type: 'string', required: true, readable: true, writable: true },
      { id: 'age', name: '年龄', type: 'long', required: false, readable: true, writable: true },
      { id: 'active', name: '启用', type: 'boolean', value: true, readable: true, writable: true },
      { id: 'role', name: '角色', type: 'enum', enumValues: [{ id: 'a', name: 'A' }], readable: true, writable: true },
      { id: 'note', name: '说明', type: 'string', readable: true, writable: false },
      { id: 'hidden', name: '隐藏', type: 'string', readable: false },
    ],
  }

  it('校验必填并转换字段值', async () => {
    const wrapper = mount(TaskForm, { props: { form } })
    await nbutton(wrapper, '提交').trigger('click')
    expect(wrapper.text()).toContain('为必填项')
    const inputs = wrapper.findAllComponents(NInput)
    await inputs[0].vm.$emit('update:value', '张三')
    await wrapper.findAllComponents(NInputNumber)[0].vm.$emit('update:value', 20)
    await wrapper.findAllComponents(NSelect)[0].vm.$emit('update:value', 'a')
    await nbutton(wrapper, '提交').trigger('click')
    expect(wrapper.emitted('submit')?.[0]?.[0]).toMatchObject({ name: '张三', age: 20, active: true, role: 'a' })
    expect(wrapper.emitted('submit')?.[0]?.[0]).not.toHaveProperty('hidden')
  })

  it('无字段时展示说明', () => {
    const wrapper = mount(TaskForm, { props: { form: { formProperties: [] } } })
    expect(wrapper.text()).toContain('未定义表单字段')
  })
})