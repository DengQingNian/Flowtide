// 仓库域页面测试辅助：内存路由与挂载入口。
import { mount } from '@vue/test-utils'
import type { VueWrapper } from '@vue/test-utils'
import type { Component } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import type { Router } from 'vue-router'

// 创建覆盖仓库详情与实例详情的测试路由。
// @param initial 初始路径
// @return 路由实例
export function createRepositoryRouter(initial: string): Router {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/repository/deployments', component: { template: '<div />' } },
      { path: '/repository/deployments/:id', component: { template: '<div />' } },
      { path: '/repository/definitions', component: { template: '<div />' } },
      { path: '/repository/definitions/:id', component: { template: '<div />' } },
      { path: '/runtime/instances/:id', component: { template: '<div />' } },
    ],
  })
  void router.push(initial)
  return router
}

// 挂载页面并等待路由就绪。
// @param component 页面组件
// @param initial 初始路径
// @return 路由与挂载结果
export async function mountPage(component: Component, initial: string): Promise<{ router: Router; wrapper: VueWrapper }> {
  const router = createRepositoryRouter(initial)
  await router.isReady()
  const wrapper = mount(component, {
    global: { plugins: [router] },
    attachTo: document.body,
  })
  return { router, wrapper }
}

// 查找页面中文本匹配的按钮。
// @param text 按钮文本
// @param root 查找根节点
// @return 按钮元素
export function findButton(text: string, root: ParentNode = document.body): HTMLButtonElement {
  const buttons = Array.from(root.querySelectorAll('button'))
  const matched = buttons.find((button) => (button.textContent ?? '').trim() === text)
  if (!matched) throw new Error(`未找到按钮：${text}`)
  return matched as HTMLButtonElement
}