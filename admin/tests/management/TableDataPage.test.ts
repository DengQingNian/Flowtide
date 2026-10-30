// 表数据页测试：列渲染、单元格查看、排序、分页、二进制提示与错误。
import { afterEach, describe, expect, it, vi } from 'vitest'
import { mockFetch, pageOf, settle } from '../helpers'
import TableDataPage from '@/pages/management/TableDataPage.vue'
import DataTable from '@/components/DataTable.vue'
import { buttonByText, mountPage } from './pageUtils'

afterEach(() => {
  vi.unstubAllGlobals()
})

/** 列元数据 */
const META = {
  tableName: 'ACT_RU_JOBS',
  columnNames: ['ID_', 'NAME_', 'BYTES_'],
  columnTypes: ['varchar(64)', 'varchar(255)', 'bytea'],
}

// 构造成功加载所需路由。
// @param extra 额外路由
// @return 路由列表
function routes(extra: { method?: string; path: string | RegExp; status?: number; body?: unknown }[] = []) {
  return [
    ...extra,
    { method: 'GET', path: '/service/management/tables/ACT_RU_JOBS/columns', body: META },
    {
      method: 'GET',
      path: /^\/service\/management\/tables\/ACT_RU_JOBS\/data\?/,
      body: pageOf([{ ID_: 'row-1', NAME_: 'alice', BYTES_: 'aGVsbG8=' }]),
    },
  ] as never
}

describe('TableDataPage', () => {
  it('渲染列、行与二进制提示', async () => {
    mockFetch(routes())
    const { wrapper } = await mountPage(TableDataPage, '/management/tables/ACT_RU_JOBS')
    await settle()
    expect(wrapper.text()).toContain('ACT_RU_JOBS')
    expect(wrapper.text()).toContain('row-1')
    expect(wrapper.text()).toContain('alice')
    expect(wrapper.text()).toContain('二进制数据')
    expect(wrapper.text()).toContain('1 个二进制列')
  })

  it('点击单元格查看完整值并可复制', async () => {
    mockFetch(routes())
    const { wrapper } = await mountPage(TableDataPage, '/management/tables/ACT_RU_JOBS')
    await settle()
    await buttonByText(wrapper, 'alice')!.trigger('click')
    await settle()
    expect(wrapper.text()).toContain('单元格 NAME_')
    expect(wrapper.text()).toContain('varchar(255)')
    const copies = wrapper.findAllComponents({ name: 'CopyText' })
    expect(copies.length).toBeGreaterThan(0)
  })

  it('应用排序后请求降序参数', async () => {
    const calls = mockFetch(routes())
    const { wrapper, router } = await mountPage(TableDataPage, '/management/tables/ACT_RU_JOBS')
    await settle()
    const selects = wrapper.findAll('select')
    await selects[0].setValue('NAME_')
    await selects[1].setValue('desc')
    await buttonByText(wrapper, '查询')!.trigger('click')
    await settle()
    expect(calls.some((call) => call.url.includes('orderDescendingColumn=NAME_'))).toBe(true)
    expect(router.currentRoute.value.query.sort).toBe('NAME_')
    expect(router.currentRoute.value.query.order).toBe('desc')
  })

  it('翻页请求下一页偏移', async () => {
    const calls = mockFetch(routes())
    const { wrapper } = await mountPage(TableDataPage, '/management/tables/ACT_RU_JOBS')
    await settle()
    wrapper.findComponent(DataTable).vm.$emit('update:page', 2)
    await settle()
    expect(calls.some((call) => call.url.includes('/data?start=50&size=50'))).toBe(true)
  })

  it('从 URL 还原排序与分页', async () => {
    const calls = mockFetch(routes())
    const { wrapper } = await mountPage(TableDataPage, '/management/tables/ACT_RU_JOBS?page=3&size=20&sort=NAME_&order=desc')
    await settle()
    expect(calls.some((call) => call.url.includes('start=40') && call.url.includes('size=20'))).toBe(true)
    expect(calls.some((call) => call.url.includes('orderDescendingColumn=NAME_'))).toBe(true)
    const selects = wrapper.findAll('select')
    expect((selects[0].element as HTMLSelectElement).value).toBe('NAME_')
  })

  it('空数据展示空态', async () => {
    mockFetch(routes([{ method: 'GET', path: /^\/service\/management\/tables\/ACT_RU_JOBS\/data\?/, body: pageOf([]) }]))
    const { wrapper } = await mountPage(TableDataPage, '/management/tables/ACT_RU_JOBS')
    await settle()
    expect(wrapper.text()).toContain('表 ACT_RU_JOBS 暂无数据')
  })

  it('列元数据错误展示 ErrorBlock', async () => {
    mockFetch([{ method: 'GET', path: '/service/management/tables/ACT_RU_JOBS/columns', status: 500, body: { code: 'ENGINE-0918', message: '表不存在' } }])
    const { wrapper } = await mountPage(TableDataPage, '/management/tables/ACT_RU_JOBS')
    await settle()
    expect(wrapper.text()).toContain('表不存在')
  })
})
