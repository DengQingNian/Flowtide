---
title: 重构进度总览
description: 查看 BPMN 内核、REST、数据库与迁移工具的实现情况，以及生产使用前仍需完成的验收。
order: 10
section: progress
status: prototype
since: v0.1.0
tags: [进度, 重构, 基线]
---

## 当前能做什么

截至 2026-10-29 的验收，Flowtide 已能在内存、JSON 状态文件或 MySQL/PostgreSQL 单执行者模式下部署流程、启动实例、处理用户任务并查询历史。BPMN 内核与 REST 路由层的主体实现已经完成。

Java 用户任务黄金样本的业务对照，以及双库临时库上的转换、续跑和源快照还原已通过。全量 Java REST、生产切换、反向接管和集群仍待验收。当前适合新建库原型和功能验证。

::: warning 如何理解完成状态
本页标记的完成范围是代码实现与回归测试。数据库、Java 对照和发布验收另行记录，使用前请查看 [能力矩阵](/compatibility) 中各列的结果。
:::

## 当前快照

| 指标 | 数值 | 说明 |
| --- | ---: | --- |
| 验收代码提交 | d1faeee | 当前分支 <code>master</code>，实现提交共 193 次 |
| Go 源文件 | 443 | 含测试与内部包 |
| Go 代码行 | 89,826 | 不含文档站与生成物 |
| 测试/基准函数 | 1024 | 以 <code>*_test.go</code> 中声明统计 |
| 全仓语句覆盖率 | 90.81% | <code>coverage.out</code> 采样，17,081 / 18,809；核心库门禁 91.88% |
| REST 方法端点 | 242 | 官方 184 + 扩展 58 |
| ACT 表 | 41 | Flowable 8.1.0.3 双库 DDL |
| 中文文档 | 93 篇 | 设计、实施与审计 |
| 可执行文件 | 2 | <code>flowtide-server</code>、<code>flowtide-migrate</code> |

> 上表保留提交 d1faeee 对应的验收统计。当前提交的代码规模和覆盖率请查看 [进度看板](/progress)。

## 阶段进展

| 阶段 | 内容 | 状态 | 证据入口 |
| --- | --- | --- | --- |
| M0 | 兼容基线、DDL、OpenAPI 与端点清单 | 完成 | [兼容性总览](/compatibility) |
| M1 | BPMN 模型、XML 安全解析、部署与版本 | 完成 | [内核进度](/docs/progress/kernel) |
| M2 | 命令、事务回滚、Agenda、执行树 | 完成 | [运行时架构](/docs/architecture/runtime) |
| M3 | 用户任务、身份、表单、监听器 | 完成 | [任务与身份](/docs/progress/kernel) |
| M4 | 作业、定时器、异步续跑、死信 | 完成 | [持久化与作业](/docs/progress/persistence) |
| M5 | 历史、审计、变量明细 | 实现与回归验收完成 | [质量门禁](/docs/progress/quality) |
| M6 | 官方 REST 方法级路由补齐 | 方法安全、69 个方法共 178 个原版 HTTP 场景通过；全量 REST 待验收 | [REST 与安全](/docs/progress/rest-security) |
| M7 | 真实数据库、行仓储、迁移工具 | 流程与身份标准行双库单执行者及迁移闭环通过 | [迁移工具链](/docs/progress/migration) |
| M8 | 集群、租约、故障转移、生产容量 | 未开始 | [差距与下一步](/docs/progress/gaps) |
| M9 | 生产切换、反向转换与 Java 增量接管 | 待生产验收 | [停写与回退](/docs/migration/cutover) |

## 已实现的功能

### 1. BPMN 执行内核

解析、部署、执行树与 Agenda 已能配合运行。用户/服务/脚本/手动任务，排他/并行/包容/事件网关，子流程、多实例、调用活动、消息/信号、定时器、错误、补偿、事务子流程与 Ad-hoc 子流程均有实现与回归测试。详见 [内核与 BPMN 进度](/docs/progress/kernel)。

### 2. 持久化与数据库

状态层支持内存、JSON 文件、MySQL SQL 快照、MySQL 行仓储与 PostgreSQL 行仓储五种形态。41 张 <code>ACT_*</code> 表的 DDL 已落地；当前行仓储映射其中 26 张流程与身份标准表，用户、组、成员、信息与头像随行仓储重启恢复，行仓储模式使用数据库独占锁限制单执行者，并在启动时从行表恢复。默认租户的任务、实例与历史实例查询及其租户条件已经下推到 SQL，但集群租约仍未实现。

### 3. 兼容与迁移工具

官方 184 个流程 REST 方法端点均已有路由，另有 58 个扩展端点。迁移工具链已具备只读预检、离线状态转换、停写静默检查与状态文件恢复校验四个独立能力；实际切换前，还需要完成停写、备份、数据核对与回退演练。

## 待验收与未实现的功能

1. **Java 行为对照未完成**：端点存在不等于请求体、响应模式、分页、错误码与副作用兼容。
2. **生产切换与回退待验收**。临时库演练已有结果，生产环境仍需完成停写、备份与回退验收。
3. **集群能力缺失**：没有跨节点作业抢占、租约或故障转移，禁止多执行者共享状态源。
4. **异步集群与外部工作项缺失**：变量异步作业的持久化、重试及恢复已通过；跨节点调度与外部工作项尚未实现。
5. **脚本能力受限**：仅表达式子集，不支持 Groovy/Starlark 与完整 JavaScript 语句。
6. **多引擎未开始**：CMMN、完整 DMN 引擎、Form/App/Content 引擎与 Helm 部署仍在路线图中。

## 如何阅读这套进度文档

- 先看 [能力矩阵](/compatibility)，核对所需能力的实现与验收结果。
- 评估是否适合接入时，看 [已知问题](/docs/compatibility/known-issues) 与 [迁移限制](/docs/migration/limits)。
- 需要复现实验时，从 [安装](/docs/getting-started/installation) 与 [REST 快速开始](/docs/getting-started/quickstart-rest) 入手。
- 需要理解实现取舍时，读 [架构总览](/docs/architecture/overview) 与仓库 <code>docs/001</code> 设计文档。
