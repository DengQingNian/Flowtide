---
title: 架构总览
description: 从 REST、服务接口、引擎、表达式、持久化到 ACT Schema 的分层与依赖方向。
order: 10
section: architecture
status: prototype
since: v0.1.0
tags: [架构, 分层, 模块]
---

## 分层

~~~text
┌──────────────────────────────────────────────┐
│ cmd/flowable-server · cmd/flowable-migrate   │
├──────────────────────────────────────────────┤
│ internal/rest        HTTP 路由、认证、指标   │
├──────────────────────────────────────────────┤
│ api                  公共类型与服务接口      │
│ flowable             公共构造入口与类型别名  │
├──────────────────────────────────────────────┤
│ internal/engine      PVM、任务、作业、历史   │
│ internal/expr        UEL 兼容表达式子集      │
│ internal/bpmn        XML 解析、模型、图形    │
├──────────────────────────────────────────────┤
│ internal/persistence 状态、行仓储、SQL、Schema│
│ internal/auth        Basic/JWT/RBAC/密码     │
│ internal/tenant      租户上下文               │
└──────────────────────────────────────────────┘
                        │
                        ▼
                  MySQL / PostgreSQL
                  Flowable ACT_* Schema
~~~

依赖方向自上而下。业务代码不依赖 <code>internal/rest</code>；REST 只负责 DTO、认证与 HTTP 语义，流程语义在引擎内完成。

## 关键决策

| 领域 | 决策 | 代价 |
| --- | --- | --- |
| 数据库 | 复用 Flowable <code>ACT_*</code> Schema | 需跟随官方 DDL 变化 |
| 持久化 | <code>database/sql</code> + 显式仓储 + 工作单元，不用 ORM | 手写映射量大 |
| 执行模型 | 移植 Command + Agenda + 执行树 | 状态复制成本高 |
| 表达式 | 自研 UEL 兼容子集，白名单方法 | 不覆盖完整脚本 |
| 并发 | 一命令一事务一 goroutine；作业扫描单 worker | 无集群抢占 |
| REST | chi 风格路由 + 手写 Handler + 契约清单 | 需要逐项 Java 对照 |
| 前端文档 | Vue 3 + Naive UI 静态站 | 与引擎独立构建 |

## 一次请求发生了什么

以“完成任务”为例：

1. <code>internal/rest</code> 解析路径、认证、租户与请求体。
2. 调用 <code>api.TaskService.Complete</code>。
3. 引擎进入命令临界区，建立命令上下文与 Agenda。
4. 执行树推进：结束用户任务、写入历史、计算后续节点。
5. 命令提交时统一持久化：内存状态、JSON 快照或行仓储增量。
6. REST 将结果映射为状态码与 JSON；错误映射为统一错误体。

## 包结构

| 路径 | 职责 |
| --- | --- |
| <code>api</code> | 公共类型、服务接口、错误码 |
| <code>flowable</code> | <code>New</code> 与公共类型别名 |
| <code>bpmn/model</code> | 对外暴露的 BPMN 模型类型 |
| <code>internal/bpmn/xml</code> | 安全 XML 解析与模型校验 |
| <code>internal/bpmn/diagram</code> | BPMN DI 与 SVG/PNG 渲染 |
| <code>internal/engine</code> | 运行时、任务、事件、作业、历史 |
| <code>internal/expr</code> | 表达式解析与求值 |
| <code>internal/persistence/state</code> | 状态快照与文件存储 |
| <code>internal/persistence/flowable</code> | ACT 行映射与差异 |
| <code>internal/persistence/sqlstate</code> | SQL 快照桥接 |
| <code>internal/persistence/mysql/postgres</code> | 方言、锁与连接 |
| <code>internal/rest</code> | HTTP 层与兼容端点 |
| <code>internal/migration</code> | 预检、转换、静默检查、校验 |
| <code>internal/auth</code> | 认证、RBAC、PBKDF2、JWT |
| <code>internal/quality</code> | 质量门禁辅助 |

## 明确不做的分层

- 不做 Java 类级二方库兼容、Spring/CDI/OSGi/JMX。
- 不内置流程设计器、表单设计器或 DMN 编辑器。
- 不提供跨进程委托加载；委托是 Go 函数。
- 不把 REST 层当作兼容性证明；兼容性由行为对照与契约测试决定。

## 相关阅读

- [运行时与执行树](/docs/architecture/runtime)
- [持久化架构](/docs/architecture/persistence)
- [模块与代码结构](/docs/architecture/modules)
