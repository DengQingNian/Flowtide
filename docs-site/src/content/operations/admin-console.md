---
title: 管理台
description: 单二进制内嵌的管理台：页面结构、工作流九页、认证边界与开发构建方式。
order: 15
section: operations
status: prototype
since: v0.1.0
tags: [管理台, 运维, workflow]
---

## 访问方式

管理台随服务端二进制内嵌，构建后访问 <code>/adminpage/#/</code>：

~~~bash
make build
./bin/flowtide-server -addr :8080 -history audit -state ./data/engine.json
# 浏览器打开 http://localhost:8080/adminpage/#/
~~~

路由使用 hash 模式，静态资源由 <code>/adminpage/</code> 提供，页面通过同源 <code>/service/**</code> 与 <code>/workflow/**</code> 取数，不注入认证头与租户头。

## 页面结构

| 分组 | 页面 |
| --- | --- |
| 概览 | 仪表盘 |
| 仓库 | 部署、部署详情、流程定义、定义详情、流程模型、BPMN 设计器 |
| 运行态 | 流程实例、实例详情、任务、任务详情 |
| 运维 | 作业、作业详情、批处理、批处理详情、表数据、表数据浏览、引擎属性、审计日志 |
| 历史 | 历史流程、历史任务、历史活动、历史变量 |
| 身份 | 用户、用户详情、组、角色 |
| 工作流 | 流程分类、流程模型、流程配置、待办任务、已办任务、流程实例、委托配置、催办配置、流程异常 |
| 系统 | 指标、关于 |

## 工作流九页

九页数据来自 workflow 业务门面，与 <code>/workflow/**</code> 共用同一套服务：

- **流程分类**：树表展示，支持添加子级。
- **流程模型**：模型列表与部署按钮，部署走 <code>/process/deploy</code>。
- **流程配置**：三个 Tab——步骤管理、流程模板与 XML 预览、流程规则占位。
- **待办 / 已办**：先选身份，再按分页取任务；办理弹窗填写意见与变量 JSON。
- **流程实例**：按业务键查状态，支持终止、挂起、激活、删除，高亮抽屉内嵌流程图。
- **委托配置**：委托规则的启停切换。
- **催办配置**：催办规则维护。
- **流程异常**：异常记录标记已处理。

保留 501 的端点一律降级为空态（“接口接线中”），不阻塞其他页面。

## 认证与安全

管理台本身不做登录、会话与权限控制，也不存储凭据：

- 未启用 REST 认证时，页面与接口都是开放的；只应部署在内网或前置反向代理之后。
- 启用 Basic、Bearer、JWT/JWKS 或 mTLS 后，数据请求返回 401/403，页面给出说明而不是静默失败。
- 浏览器凭据由反向代理注入 Cookie 或前端统一加头；管理台客户端不读取本地存储的令牌。

需要按人审计时，在前置代理层记录操作者，并保留服务端的审计日志端点输出。

## 开发与构建

~~~bash
# 整体构建：先构建管理台再内嵌进服务端
make build

# 仅构建管理台并复制进嵌入目录
make admin-build

# 前端开发：5174 端口，代理 /service、/workflow、/healthz、/readyz、/metrics 到 8080
cd admin && npm run dev

# 单元测试与端到端冒烟
make admin-test
make admin-e2e
~~~

开发模式要求本机 8080 已有服务端在运行；代理目标写死在 <code>admin/vite.config.ts</code>，换端口需要同步修改。

## 已知边界

- 页面状态不持久化，刷新回到分组首页。
- 设计器只覆盖项目已实现的建模能力，不追求 Flowable Modeler 的全部交互。
- 依赖待办清单的扫描类功能在 SQL 行仓储模式下无待办数据源，页面操作不受影响，但后台自动流转不生效。
- 管理台不提供升级、备份、迁移入口，这些仍通过 CLI 完成。

## 相关阅读

- [workflow 业务门面](/docs/sdk/workflow)
- [运维安全](/docs/operations/security)
- [存储模式](/docs/operations/storage)
- [宿主嵌入 SDK](/docs/sdk/embedding)
