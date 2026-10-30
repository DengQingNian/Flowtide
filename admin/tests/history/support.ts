// 历史域测试辅助：按路径挂载页面组件并复用内存路由。
import { mount, type VueWrapper } from '@vue/test-utils'
import { createMemoryHistory, createRouter, type RouteRecordRaw } from 'vue-router'
import type { Component } from 'vue'
import { settle } from '../helpers'

// 基础路由表：覆盖历史页面跳转与跨域链接目标。
// @return 路由记录
function baseRoutes(): RouteRecordRaw[] {
  const stub = { template: '<div />' }
  return [
    { path: '/history/processes', name: 'historic-processes', component: stub },
    { path: '/history/processes/:id', name: 'historic-process-detail', component: stub },
    { path: '/history/tasks', name: 'historic-tasks', component: stub },
    { path: '/history/tasks/:id', name: 'historic-task-detail', component: stub },
    { path: '/history/activities', name: 'historic-activities', component: stub },
    { path: '/history/variables', name: 'historic-variables', component: stub },
    { path: '/repository/definitions/:id', name: 'definition-detail', component: stub },
    { path: '/runtime/instances/:id', name: 'instance-detail', component: stub },
  ]
}

// 在指定路径挂载页面组件。
// @param component 页面组件
// @param path 初始路径（可含 query）
// @return 包装器与路由实例
export async function mountAt(
  component: Component,
  path: string,
): Promise<{ wrapper: VueWrapper; router: ReturnType<typeof createRouter> }> {
  const router = createRouter({ history: createMemoryHistory(), routes: baseRoutes() })
  await router.push(path)
  await router.isReady()
  const wrapper = mount(component, { global: { plugins: [router] } })
  await settle()
  return { wrapper, router }
}

// 按文本查找按钮。
// @param wrapper 包装器
// @param text 按钮文本
// @return 按钮包装器或 undefined
export function buttonByText(wrapper: VueWrapper, text: string): ReturnType<VueWrapper['find']> | undefined {
  return wrapper.findAll('button').find((button) => button.text().trim() === text)
}

// 点击指定文本的按钮。
// @param wrapper 包装器
// @param text 按钮文本
// @return 无
export async function clickButton(wrapper: VueWrapper, text: string): Promise<void> {
  const button = buttonByText(wrapper, text)
  if (!button) throw new Error(`未找到按钮：${text}。当前按钮：${wrapper.findAll('button').map((item) => item.text().trim()).join('/')}`)
  await button.trigger('click')
  await settle()
}
