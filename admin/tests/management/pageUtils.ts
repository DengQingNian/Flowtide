// 管理台页面测试辅助：内存路由与按钮查找。
import { createMemoryHistory, createRouter, type Router } from 'vue-router'
import { mount, type VueWrapper } from '@vue/test-utils'
import type { Component } from 'vue'
import ConfirmButton from '@/components/ConfirmButton.vue'

/** 测试路由占位组件 */
const Blank = { template: '<div />' }

// 创建覆盖运维域页面的内存路由。
// @param path 初始路径
// @return 已就绪的路由
export async function makeRouter(path: string): Promise<Router> {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/management/jobs/:bucket', name: 'jobs', component: Blank },
      { path: '/management/jobs/:bucket/:id', name: 'job-detail', component: Blank },
      { path: '/management/batches', name: 'batches', component: Blank },
      { path: '/management/batches/:id', name: 'batch-detail', component: Blank },
      { path: '/management/tables', name: 'tables', component: Blank },
      { path: '/management/tables/:name', name: 'table-data', component: Blank },
      { path: '/management/engine', name: 'engine', component: Blank },
      { path: '/management/audit', name: 'audit', component: Blank },
      { path: '/runtime/instances/:id', name: 'instance-detail', component: Blank },
      { path: '/repository/definitions/:id', name: 'definition-detail', component: Blank },
    ],
  })
  await router.push(path)
  await router.isReady()
  return router
}

// 挂载页面并注入内存路由。
// @param page 页面组件
// @param path 初始路径
// @return 包装器与路由
export async function mountPage(page: Component, path: string): Promise<{ wrapper: VueWrapper; router: Router }> {
  const router = await makeRouter(path)
  const wrapper = mount(page, { global: { plugins: [router] } })
  return { wrapper, router }
}

// 按文本查找按钮。
// @param wrapper 包装器
// @param text 按钮文本
// @return 按钮包装器或 undefined
export function buttonByText(wrapper: VueWrapper, text: string) {
  return wrapper.findAll('button').find((button) => button.text().replace(/\s+/g, ' ').includes(text))
}

// 按标签查找二次确认按钮组件。
// @param wrapper 包装器
// @param label 按钮标签
// @return 组件包装器或 undefined
export function confirmByLabel(wrapper: VueWrapper, label: string): VueWrapper | undefined {
  return wrapper.findAllComponents(ConfirmButton).find((item) => item.props('label') === label)
}
