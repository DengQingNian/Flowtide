import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import DataTable from '@/components/DataTable.vue'

const columns = [{ title: '名称', key: 'name' }]
const data = [
  { id: '1', name: 'A' },
  { id: '2', name: 'B' },
]

describe('DataTable', () => {
  it('渲染数据与总数', () => {
    const wrapper = mount(DataTable, { props: { columns, data, total: 2 } })
    expect(wrapper.text()).toContain('A')
    expect(wrapper.text()).toContain('B')
    expect(wrapper.text()).toContain('共 2 条')
  })

  it('空数据展示空态', () => {
    const wrapper = mount(DataTable, { props: { columns, data: [], total: 0, emptyDescription: '没有记录' } })
    expect(wrapper.text()).toContain('暂无数据')
    expect(wrapper.text()).toContain('没有记录')
  })

  it('错误时展示 ErrorBlock', () => {
    const wrapper = mount(DataTable, { props: { columns, data: [], error: new Error('失败') } })
    expect(wrapper.text()).toContain('失败')
  })

  it('分页与选择事件向上传播', async () => {
    const wrapper = mount(DataTable, { props: { columns, data, total: 100, page: 1, pageSize: 50 } })
    const vm = wrapper.vm as unknown as {
      updatePage: (page: number) => void
      updatePageSize: (size: number) => void
      updateChecked: (keys: string[]) => void
    }
    vm.updatePage(2)
    vm.updatePageSize(20)
    vm.updateChecked(['1'])
    await nextTick()
    expect(wrapper.emitted('update:page')?.[0]).toEqual([2])
    expect(wrapper.emitted('update:pageSize')?.[0]).toEqual([20])
    expect(wrapper.emitted('update:checkedRowKeys')?.[0]).toEqual([['1']])
  })
})