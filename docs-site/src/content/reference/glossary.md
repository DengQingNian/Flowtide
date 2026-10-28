---
title: 术语表
description: 部署、执行、任务、作业、历史、租户、委托等术语的中英文对照与本站用法。
order: 40
section: reference
status: verified
since: v0.1.0
tags: [参考, 术语]
---

## 核心术语

| 中文 | 英文 | 说明 |
| --- | --- | --- |
| 部署 | deployment | 一组资源的发布 |
| 流程定义 | process definition | 可执行 BPMN 的版本 |
| 流程实例 | process instance | 一次运行 |
| 执行 | execution | 实例内的令牌/作用域 |
| 任务 | task | 用户任务或独立任务 |
| 作业 | job | 异步/定时作业 |
| 死信 | dead letter | 重试耗尽的作业 |
| 租户 | tenant | 隔离标识 |
| 历史级别 | history level | none/instance/task/activity/audit/full |
| 委托 | delegate | 服务任务的 Go 实现 |
| 监听器 | listener | 任务/执行回调 |
| 补偿 | compensation | 逆序补偿处理 |
| 行仓储 | row repository | 以 ACT_* 行为权威状态 |
| 快照 | snapshot | 全量状态序列化 |
| 独占锁 | exclusive lock | 单执行者数据库锁 |
| 静默检查 | quiescence check | 观察窗口前后快照比较 |
| 预检 | preflight | 只读扫描存量库 |
| 反向转换 | reverse conversion | Go 状态回写 Java 兼容数据 |
| 契约测试 | contract test | 对照官方 OpenAPI/Java 行为 |

## 状态词

| 状态 | 含义 |
| --- | --- |
| 已验证 | 当前提交可复现 |
| 原型可用 | 主路径可用，有限制 |
| 部分支持 | 语义子集或边界不完整 |
| 规划中 | 未实现 |
| 已编码 | 代码存在，不代表兼容 |

## 命名约定

- 路由使用小写短横线：<code>/process-definitions</code>。
- Go 包使用短小写：<code>internal/engine</code>。
- 文档使用三位流水号：<code>docs/004-文档站/001-文档站规划.md</code>。
- 错误码使用大写前缀 + 四位数字。

## 相关阅读

- [核心概念](/docs/getting-started/concepts)
- [架构总览](/docs/architecture/overview)
- [文档索引](/docs/reference/docs-index)
