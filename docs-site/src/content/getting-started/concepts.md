---
title: 核心概念
description: 了解部署、流程定义、实例、执行与任务的关系，以及作业、历史和变量的作用。
order: 40
section: getting-started
status: verified
since: v0.1.0
tags: [概念, BPMN, 执行树]
---

## 核心概念

| 概念 | 含义 | REST 入口 | SDK 入口 |
| --- | --- | --- | --- |
| 部署 | 一次发布的资源集合 | <code>/service/repository/deployments</code> | <code>RepositoryService.Deploy</code> |
| 流程定义 | 可执行 BPMN 的版本 | <code>/service/repository/process-definitions</code> | <code>RepositoryService.ListProcessDefinitions</code> |
| 流程实例 | 流程定义的一次运行 | <code>/service/runtime/process-instances</code> | <code>RuntimeService.StartProcessInstance</code> |
| 执行 | 实例内的令牌/作用域节点 | <code>/service/runtime/executions</code> | <code>RuntimeService.ListExecutions</code> |
| 任务 | 等待人工处理的工作项 | <code>/service/runtime/tasks</code> | <code>TaskService.ListTasks</code> |
| 作业 | 异步或定时执行的工作 | <code>/service/management/jobs</code> | <code>ManagementService.ListJobs</code> |
| 历史 | 已发生事实的审计记录 | <code>/service/history/...</code> | <code>HistoryService...</code> |
| 变量 | 流程与任务的数据 | <code>.../variables</code> | <code>RuntimeService.SetVariable</code> |

## 生命周期

~~~text
部署 ──► 流程定义 v1 ──► 启动实例
                             │
                             ▼
                        执行树展开
                        │        │
                     用户任务   服务任务
                        │        │
                     完成/签收  委托执行
                        │        │
                        └──► 结束
                             │
                             ▼
                        历史记录
~~~

## 部署与版本

一次部署可以包含多个资源：BPMN XML、表单 <code>.form</code>、DMN 等。相同 Key 的再次部署默认创建新版本；启用重复过滤时，资源完全相同则复用已有部署。启动可以按 Key（使用最新版本）或按定义 ID（固定版本）。

## 实例与执行

流程实例是业务运行，执行是实例内的令牌。并行网关、多实例和子流程会生成多个执行；变量沿作用域链查找。实例级变量在整棵执行树可见，执行局部变量只在其作用域可见，瞬态变量只在当前命令内可见且不持久化。

## 任务

用户任务由 BPMN 的 <code>userTask</code> 创建，可设置 assignee、候选用户/组、表单 Key、监听器与到期时间。任务动作包括签收、完成、委派与 resolve。独立任务（不依赖流程实例）也受支持，用于临时待办。

## 作业

<code>asyncBefore/asyncAfter</code>、定时器与重试会生成作业。作业由后台扫描器按间隔执行；失败按退避重试，耗尽后进入死信。作业需要持久化才能在重启后继续。

## 历史

历史级别从 <code>none</code> 到 <code>full</code>。当前主路径使用 <code>audit</code>：记录流程实例、活动、任务与变量历史。异步历史与定期清理未实现。

## 变量

支持的变量类型包括 null、string、longString、boolean、short、integer、long、double、date、json、longJson 与 bytes。<code>serializable</code> 与未知类型会被拒绝。变量作用域与类型详见 [变量类型](/docs/reference/variables)。

## 与 Java Flowable 的对照

| Java 概念 | Go 对应 | 差异 |
| --- | --- | --- |
| ProcessEngine | <code>flowtide.Engine</code> | 单进程构造，无 Spring |
| RuntimeService | <code>api.RuntimeService</code> | 方法子集，参数使用 Go 结构体 |
| JavaDelegate | <code>flowtide.Delegate</code> | 进程内函数注册，无类加载 |
| ACT_* Schema | 相同表名与主要列 | 部分语义与写入时机待对照 |
| REST /service | 相同前缀与主要路径 | 响应模式待逐项验收 |

## 下一步

- [架构总览](/docs/architecture/overview)
- [BPMN 元素支持](/docs/compatibility/bpmn)
- [REST 约定](/docs/rest/conventions)
