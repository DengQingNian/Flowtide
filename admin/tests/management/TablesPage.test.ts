// 表清单页测试：前缀分组、行数、空态与错误。
import { afterEach, describe, expect, it, vi } from 'vitest'
import { mockFetch, settle } from '../helpers'
import TablesPage from '@/pages/management/TablesPage.vue'
import { mountPage } from './pageUtils'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('TablesPage', () => {
  it('按仓库/运行/历史/身份/其他分组展示表与行数', async () => {
    mockFetch([
      {
        method: 'GET',
        path: '/service/management/tables',
        body: [
          { name: 'ACT_RE_MODEL', count: 2, url: '/service/management/tables/ACT_RE_MODEL' },
          { name: 'ACT_RU_JOBS', count: 3 },
          { name: 'ACT_HI_TASKINST', count: 4 },
          { name: 'ACT_ID_USER', count: 5 },
          { name: 'ACT_GE_PROPERTY', count: 6 },
        ],
      },
    ])
    const { wrapper } = await mountPage(TablesPage, '/management/tables')
    await settle()
    expect(wrapper.text()).toContain('仓库')
    expect(wrapper.text()).toContain('运行')
    expect(wrapper.text()).toContain('历史')
    expect(wrapper.text()).toContain('身份')
    expect(wrapper.text()).toContain('其他')
    expect(wrapper.text()).toContain('ACT_GE_PROPERTY')
    expect(wrapper.text()).toContain('5 张 ACT_* 表')
    expect(wrapper.findAll('a').some((link) => link.attributes('href') === '/management/tables/ACT_RE_MODEL')).toBe(true)
  })

  it('空清单展示空态', async () => {
    mockFetch([{ method: 'GET', path: '/service/management/tables', body: [] }])
    const { wrapper } = await mountPage(TablesPage, '/management/tables')
    await settle()
    expect(wrapper.text()).toContain('没有可展示的表')
  })

  it('接口错误展示 ErrorBlock', async () => {
    mockFetch([{ method: 'GET', path: '/service/management/tables', status: 500, body: { code: 'ENGINE-0918', message: '表元数据不可用' } }])
    const { wrapper } = await mountPage(TablesPage, '/management/tables')
    await settle()
    expect(wrapper.text()).toContain('表元数据不可用')
    expect(wrapper.text()).toContain('错误码：ENGINE-0918')
  })
})
