import { expect, test } from 'playwright/test'
import { buildBpmn } from './utils'

// 管理台冒烟：页面可打开、导航可用、无控制台错误。
test.describe.serial('管理台冒烟', () => {
  test('仪表盘与左侧导航', async ({ page }) => {
    const errors: string[] = []
    page.on('console', (message) => {
      if (message.type() === 'error') errors.push(message.text())
    })
    page.on('pageerror', (error) => errors.push(`PAGEERROR ${error.message}`))

    await page.goto('/adminpage/#/dashboard')
    await expect(page.locator('.app-header__title')).toHaveText('仪表盘')
    await expect(page.locator('.side-nav')).toBeVisible()
    await expect(page.getByText('Flowtide 管理台')).toBeVisible()
    await expect(page.getByText('引擎', { exact: true }).first()).toBeVisible()

    await page.locator('.side-nav__link', { hasText: '部署' }).click()
    await expect(page.locator('.app-header__title')).toHaveText('部署')

    await page.locator('.side-nav__link', { hasText: '流程模型' }).click()
    await expect(page.locator('.app-header__title')).toHaveText('流程模型')

    expect(errors).toEqual([])
  })

  test('静态资源缺失返回友好页面', async ({ request }) => {
    const response = await request.get('/adminpage/missing-resource.js')
    expect(response.status()).toBe(404)
    expect(response.headers()['content-type']).toContain('text/html')
    expect(await response.text()).toContain('管理台资源不存在')
  })

  test('设计器加载模型并渲染画布与属性面板', async ({ page, request }) => {
    const key = `e2e_design_${Date.now()}`
    const created = await request.post('/service/repository/models', {
      data: { name: 'E2E 设计器模型', key, category: 'e2e' },
    })
    expect(created.status()).toBe(201)
    const model = (await created.json()) as { id: string }
    const xml = buildBpmn({ key, name: 'E2E 设计器模型', taskName: '审批' })
    const source = await request.put(`/service/repository/models/${model.id}/source`, {
      multipart: { file: { name: `${key}.bpmn20.xml`, mimeType: 'application/xml', buffer: Buffer.from(xml) } },
    })
    expect(source.status()).toBeLessThan(300)

    await page.goto(`/adminpage/#/repository/models/${model.id}/design`)
    await expect(page.locator('.djs-container')).toBeVisible({ timeout: 20000 })
    await expect(page.locator('.djs-element').first()).toBeVisible()
    await expect(page.locator('.bpmn-designer__panel')).toBeVisible()
    await expect(page.getByRole('button', { name: /保存/ }).first()).toBeVisible()
    await expect(page.getByRole('button', { name: /部署/ }).first()).toBeVisible()
  })

  test('部署 → 启动 → 办理 → 历史闭环', async ({ page, request }) => {
    const key = `e2e_flow_${Date.now()}`
    const businessKey = `BK-${key}`
    const xml = buildBpmn({ key, name: 'E2E 闭环流程', taskName: 'E2E 审批', assignee: 'alice' })
    const deployment = await request.post('/service/repository/deployments?deploymentName=' + encodeURIComponent('E2E 闭环部署'), {
      multipart: { file: { name: `${key}.bpmn20.xml`, mimeType: 'application/xml', buffer: Buffer.from(xml) } },
    })
    expect(deployment.status()).toBeLessThan(300)
    const definitions = await (await request.get(`/service/repository/process-definitions?key=${key}`)).json()
    expect(definitions.total).toBe(1)
    const definition = definitions.data[0] as { id: string }

    await page.goto(`/adminpage/#/repository/definitions/${definition.id}`)
    await expect(page.getByText('E2E 闭环流程').first()).toBeVisible()
    await page.getByRole('button', { name: '启动实例', exact: true }).click()
    await page.getByPlaceholder('可选，用于关联业务单据').fill(businessKey)
    await page.getByRole('button', { name: '启动', exact: true }).click()
    await expect(page).toHaveURL(/\/runtime\/instances\//)

    const instanceId = page.url().split('/runtime/instances/')[1]
    const tasks = await (await request.get(`/service/runtime/tasks?processInstanceId=${instanceId}&assignee=alice`)).json()
    expect(tasks.total).toBe(1)
    const taskId = tasks.data[0].id as string

    await page.goto(`/adminpage/#/runtime/tasks/${taskId}`)
    await expect(page.getByText('变量 JSON 完成')).toBeVisible()
    await page.getByPlaceholder('{"approved": true}').fill('{"approved":true}')
    await page.getByRole('button', { name: '完成任务', exact: true }).click()
    await expect(page).toHaveURL(/\/runtime\/tasks$/)

    await page.goto('/adminpage/#/history/processes')
    await expect(page.getByText(businessKey)).toBeVisible()
  })
  test('ZIP 多流程部署与资源预览', async ({ page }) => {
    await page.goto('/adminpage/#/repository/deployments')
    await page.getByRole('button', { name: '上传部署' }).click()
    await page.locator('input[type=file]').setInputFiles('e2e/fixtures/two-processes.zip')
    await page.getByPlaceholder('可选，缺省为 deployment').fill('E2E ZIP 部署')
    await page.getByRole('button', { name: '上传并部署' }).click()
    await expect(page).toHaveURL(/\/repository\/deployments\//)
    await expect(page.getByText('ZIP 流程一').first()).toBeVisible()
    await expect(page.getByText('ZIP 流程二').first()).toBeVisible()
    await page.getByRole('button', { name: '预览' }).first().click()
    await expect(page.getByText('one.bpmn20.xml').first()).toBeVisible()
  })

})