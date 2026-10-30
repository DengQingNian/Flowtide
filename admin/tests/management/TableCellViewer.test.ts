import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import TableCellViewer from '@/pages/management/components/TableCellViewer.vue'

describe('TableCellViewer', () => {
  it('展示普通文本与列类型', () => {
    const wrapper = mount(TableCellViewer, { props: { column: 'NAME_', type: 'varchar', value: 'hello' } })
    expect(wrapper.text()).toContain('单元格 NAME_')
    expect(wrapper.text()).toContain('varchar')
    expect(wrapper.text()).toContain('hello')
  })

  it('对象值使用 JSON 查看器', () => {
    const wrapper = mount(TableCellViewer, { props: { column: 'DATA_', type: 'json', value: { a: 1 } } })
    expect(wrapper.find('pre').text()).toContain('"a": 1')
  })

  it('二进制列给出提示并展示原始文本', () => {
    const wrapper = mount(TableCellViewer, { props: { column: 'BYTES_', type: 'blob', value: 'AQID' } })
    expect(wrapper.text()).toContain('二进制列不可直接预览')
    expect(wrapper.find('pre').text()).toContain('AQID')
  })

  it('空值展示短横线并可关闭', async () => {
    const wrapper = mount(TableCellViewer, { props: { column: 'X_', value: null } })
    expect(wrapper.text()).toContain('—')
    await wrapper.find('button[title="关闭单元格查看"]').trigger('click')
    expect(wrapper.emitted('close')).toBeTruthy()
  })
})
