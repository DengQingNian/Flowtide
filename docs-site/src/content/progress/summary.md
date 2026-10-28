---
title: 重构进度总览
description: 截至提交 2925fc7 的规模、阶段、已完成主线与尚未跨过的门槛。
order: 10
section: progress
status: prototype
since: v0.1.0
tags: [进度, 重构, 基线]
---

## 一句话结论

截至提交 <code>2925fc7</code>（2026-10-29），项目已经完成 **BPMN 内核与 REST 路由层的主体实现**，可以在内存、JSON 状态文件或 MySQL 单执行者模式下跑通从部署、启动、用户任务到历史查询的闭环；但**尚未通过 Java 行为对照、停写切换、回退和集群验收**，当前定位仍是“新建库原型”。

::: warning 状态口径
本页的“完成”指代码与回归测试已落地，不等同于生产可用。能力状态以 [能力矩阵](/compatibility) 的四列证据为准：已编码、真实数据库、Java 对照、发布状态。
:::

## 当前快照

| 指标 | 数值 | 说明 |
| --- | ---: | --- |
| Git 提交 | 177 | 当前分支 <code>master</code> |
| Go 源文件 | 392 | 含测试与内部包 |
| Go 代码行 | 82,634 | 不含文档站与生成物 |
| 测试/基准函数 | 965 | 以 <code>*_test.go</code> 中声明统计 |
| 语句覆盖率 | 90.86% | <code>coverage.out</code> 采样，15,904 / 17,504 |
| REST 方法端点 | 242 | 官方 184 + 扩展 58 |
| ACT 表 | 41 | Flowable 8.1.0.3 双库 DDL |
| 中文文档 | 84 篇 | 设计、实施与审计 |
| 可执行文件 | 2 | <code>flowable-server</code>、<code>flowable-migrate</code> |

> 这些数字由仓库脚本生成，用于描述当前提交，不作为性能或生产容量承诺。

## 阶段进展

| 阶段 | 内容 | 状态 | 证据入口 |
| --- | --- | --- | --- |
| M0 | 兼容基线、DDL、OpenAPI 与端点清单 | 完成 | [兼容性总览](/compatibility) |
| M1 | BPMN 模型、XML 安全解析、部署与版本 | 完成 | [内核进度](/docs/progress/kernel) |
| M2 | 命令、事务回滚、Agenda、执行树 | 完成 | [运行时架构](/docs/architecture/runtime) |
| M3 | 用户任务、身份、表单、监听器 | 完成 | [任务与身份](/docs/progress/kernel) |
| M4 | 作业、定时器、异步续跑、死信 | 完成 | [持久化与作业](/docs/progress/persistence) |
| M5 | 历史、审计、变量明细 | 主体完成 | [质量门禁](/docs/progress/quality) |
| M6 | 官方 REST 方法级路由补齐 | 路由完成，行为待验收 | [REST 与安全](/docs/progress/rest-security) |
| M7 | 真实数据库、行仓储、迁移工具 | 单执行者原型 | [迁移工具链](/docs/progress/migration) |
| M8 | 集群、租约、故障转移、生产容量 | 未开始 | [差距与下一步](/docs/progress/gaps) |

## 已完成的三条主线

### 1. BPMN 执行内核

解析、部署、执行树、Agenda 与主流 BPMN 元素已经形成可运行闭环：用户/服务/脚本/手动任务，排他/并行/包容/事件网关，子流程、多实例、调用活动、消息/信号、定时器、错误、补偿、事务子流程与 Ad-hoc 子流程均有实现与回归测试。详见 [内核与 BPMN 进度](/docs/progress/kernel)。

### 2. 持久化与数据库

状态层支持内存、JSON 文件、MySQL SQL 快照与 MySQL 行仓储四种形态。41 张 <code>ACT_*</code> 表的 DDL 与行映射已落地，行仓储模式使用数据库独占锁限制单执行者，并在启动时从行表恢复。任务查询已经下推到 SQL，但实例/历史查询、租户下推与集群租约仍未实现。

### 3. 兼容与迁移工具

官方 184 个流程 REST 方法端点均已有路由，另有 58 个扩展端点。迁移工具链已具备只读预检、离线状态转换、停写静默检查与状态文件恢复校验四个独立能力；它们各自有明确边界，不能相互推导为“可切换”。

## 尚未跨过的门槛

1. **Java 行为对照未完成**：端点存在不等于请求体、响应模式、分页、错误码与副作用兼容。
2. **停写切换与回退未演练**：预检通过只代表当前分析未发现阻断。
3. **集群能力缺失**：没有跨节点作业抢占、租约或故障转移，禁止多执行者共享状态源。
4. **异步与外部工作项缺失**：<code>variables-async</code> 为同步替代实现，没有持久异步命令。
5. **脚本能力受限**：仅表达式子集，不支持 Groovy/Starlark 与完整 JavaScript 语句。
6. **多引擎未开始**：CMMN、完整 DMN 引擎、Form/App/Content 引擎与 Helm 部署仍在路线图中。

## 如何阅读这套进度文档

- 先看 [能力矩阵](/compatibility)，确认一项能力处于哪一列证据。
- 需要判断“能不能用”时，看 [已知问题](/docs/compatibility/known-issues) 与 [迁移限制](/docs/migration/limits)。
- 需要复现实验时，从 [安装](/docs/getting-started/installation) 与 [REST 快速开始](/docs/getting-started/quickstart-rest) 入手。
- 需要理解实现取舍时，读 [架构总览](/docs/architecture/overview) 与仓库 <code>docs/001</code> 设计文档。
