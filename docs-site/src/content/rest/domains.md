---
title: REST 分域端点
description: 按仓库、运行时、任务、历史、管理、身份、查询与表单域梳理主要端点。
order: 40
section: rest
status: prototype
since: v0.1.0
tags: [REST, 端点, 业务域]
---

## 仓库域 /repository

| 方法与路径 | 用途 |
| --- | --- |
| <code>GET/POST /repository/deployments</code> | 部署列表/创建 |
| <code>GET/DELETE /repository/deployments/{id}</code> | 部署详情/删除 |
| <code>GET /repository/deployments/{id}/resources</code> | 资源列表 |
| <code>GET /repository/deployments/{id}/resourcedata/{name}</code> | 资源内容 |
| <code>GET /repository/process-definitions</code> | 定义列表 |
| <code>GET /repository/process-definitions/{id}/xml</code> | BPMN XML |
| <code>GET /repository/process-definitions/{id}/image</code> | 定义流程图 |
| <code>GET /repository/process-definitions/{id}/model</code> | BPMN 模型 |
| <code>POST /repository/process-definitions/{id}/migrate</code> | 按定义迁移实例 |
| <code>POST /repository/process-definitions/{id}/batch-migrate</code> | 批量迁移实例 |
| <code>PUT /repository/process-definitions/{id}</code> | 挂起/激活 |
| <code>GET/POST /repository/models</code> | 模型仓库 |
| <code>GET/POST /repository/process-definitions/{id}/identitylinks</code> | 候选发起人 |

## 运行时域 /runtime

| 方法与路径 | 用途 |
| --- | --- |
| <code>POST /runtime/process-instances</code> | 启动实例 |
| <code>GET/DELETE /runtime/process-instances/{id}</code> | 查询/删除 |
| <code>PUT /runtime/process-instances/{id}</code> | 挂起/激活 |
| <code>GET /runtime/process-instances/{id}/variables</code> | 实例变量 |
| <code>GET /runtime/executions/{id}</code> | 执行树 |
| <code>GET/POST /runtime/tasks</code> | 任务查询/创建 |
| <code>POST /runtime/tasks/{id}</code> | action 形式的任务动作 |
| <code>GET/POST /runtime/tasks/{id}/variables</code> | 任务变量 |
| <code>POST /runtime/process-instances/{id}/migrate</code> | 实例迁移 |
| <code>POST /runtime/process-instances/{id}/validate-migration</code> | 迁移预检 |
| <code>POST /runtime/process-instances/{id}/change-state</code> | 实例活动迁移 |
| <code>POST /runtime/executions/{id}/change-state</code> | 执行活动迁移 |
| <code>GET /runtime/process-instances/{id}/diagram</code> | 运行态流程图 |
| <code>POST /runtime/signals</code> | 信号广播 |
| <code>PUT /runtime/executions/{id}</code> | 触发等待执行 |

## 历史域 /history

| 方法与路径 | 用途 |
| --- | --- |
| <code>GET /history/historic-process-instances</code> | 实例历史 |
| <code>GET /history/historic-process-instances/{id}</code> | 实例详情 |
| <code>GET /history/historic-activity-instances</code> | 活动历史 |
| <code>GET /history/historic-task-instances</code> | 任务历史 |
| <code>GET /history/historic-variable-instances</code> | 变量历史 |
| <code>GET /history/historic-detail</code> | 明细 |
| <code>GET /history/historic-task-log-entries</code> | 任务日志 |

## 管理域 /management

| 方法与路径 | 用途 |
| --- | --- |
| <code>GET /management/engine</code> | 引擎信息 |
| <code>GET /management/properties</code> | 引擎属性 |
| <code>GET /management/tables</code> | 表清单 |
| <code>GET /management/tables/{name}/data</code> | 表数据 |
| <code>GET /management/jobs</code> | 作业 |
| <code>GET /management/deadletter-jobs</code> | 死信 |
| <code>POST /management/jobs/run</code> | 立即执行到期作业 |
| <code>GET /management/batches</code> | 批次 |

## 身份与表单域 /identity /form

用户、组、角色、成员、用户信息键值与头像；启动/任务表单读取与提交。表单定义来自部署内的 <code>.form</code> 资源或 BPMN 的 <code>formKey</code>。

## 查询域 /query

<code>POST /query/process-instances</code>、<code>/query/tasks</code>、<code>/query/executions</code>、<code>/query/activity-instances</code>、<code>/query/variable-instances</code> 以及历史实例/任务/活动/变量/明细组合查询，支持过滤、排序与分页。

## 完整清单

REST 浏览器提供 242 个端点的搜索、方法与域筛选：[打开 REST 浏览器](/rest-explorer)。

## 相关阅读

- [REST 约定](/docs/rest/conventions)
- [REST 覆盖与差异](/docs/compatibility/rest)
- [REST 快速开始](/docs/getting-started/quickstart-rest)
