---
title: 引擎配置
description: Config 字段、默认值、状态路径、时钟与 ID 注入，以及仓库外模块的可配置边界。
order: 20
section: sdk
status: verified
since: v0.1.0
tags: [SDK, 配置, 持久化]
---

## 默认值

<code>flowtide.New(flowtide.Config{})</code> 使用内存状态、<code>audit</code> 历史、真实时钟与 UUIDv7 ID 生成器。需要持久化或调整历史级别时，再设置对应字段。

~~~go
app, err := flowtide.New(flowtide.Config{
	HistoryLevel: "audit",
	StatePath:    "./data/engine.json",
	TenantID:     "default",
})
~~~

## 字段

| 字段 | 类型 | 默认 | 说明 |
| --- | --- | --- | --- |
| <code>StartupContext</code> | context.Context | 空 | 仅用于初始化与恢复，完成后不保留 |
| <code>HistoryLevel</code> | string | <code>audit</code> | none/instance/task/activity/audit/full |
| <code>Clock</code> | internal 接口 | 真实时钟 | 测试可注入假时钟 |
| <code>IDs</code> | internal 接口 | UUIDv7 | 测试可注入确定性 ID |
| <code>MaxAgendaSteps</code> | int | 10000 | 单命令最大议程步数 |
| <code>TenantID</code> | string | 空 | 默认租户 |
| <code>StatePath</code> | string | 空 | JSON 状态文件 |
| <code>StateStore</code> | internal 接口 | nil | 自定义状态存储 |
| <code>RowRepository</code> | internal 指针 | nil | 行仓储 |
| <code>LoadRows</code> | bool | false | 启动时从行仓储恢复 |
| <code>LoadTenant</code> | string | 空 | 恢复租户 |
| <code>RecordSequenceFlowHistory</code> | bool | false | 迁移续跑时按 Java audit 记录顺序流 |

## 三种常用组合

### 内存

~~~go
app, err := flowtide.New(flowtide.Config{})
~~~

适合单测与演示；进程退出即丢失。

### JSON 状态文件

~~~go
app, err := flowtide.New(flowtide.Config{StatePath: "./data/engine.json"})
~~~

单进程、全量原子快照。父目录必须存在；文件包含业务数据，权限由进程 umask 决定。

### 行仓储

行仓储字段引用 <code>internal/persistence/flowable.Repository</code>，仓库外模块无法直接构造。需要数据库的生产接入使用 <code>NewEmbedded</code>：接入方只提供 <code>*sql.DB</code>，行仓储、写者锁与恢复由 SDK 装配。独立服务对应 <code>-storage mysql-rows</code>。

## 仓库外模块限制

<code>flowtide.Config</code> 是 <code>internal/engine.Config</code> 的类型别名，字段类型包含 <code>internal/clock</code>、<code>internal/idgen</code>、<code>internal/persistence/state</code> 与 <code>internal/persistence/flowable</code>。因此：

- 仓库外可以设置 <code>HistoryLevel</code>、<code>TenantID</code>、<code>StatePath</code>、<code>MaxAgendaSteps</code>、<code>LoadTenant</code>、<code>LoadRows</code> 与 <code>RecordSequenceFlowHistory</code>。
- 仓库外不能命名 <code>Clock</code>、<code>IDs</code>、<code>StateStore</code>、<code>RowRepository</code> 的类型来注入实现。
- 需要数据库、workflow 门面或 HTTP 处理器时使用 [宿主嵌入 SDK](/docs/sdk/embedding) 的 <code>NewEmbedded</code>。

::: warning 不要依赖别名
类型别名可能随重构调整。外部模块应只使用稳定字段，并在升级时重新编译验证。
:::

## 历史级别

| 级别 | 记录范围 | 当前状态 |
| --- | --- | --- |
| none | 不记录 | 支持 |
| instance | 实例 | 支持 |
| task | 实例 + 任务 | 支持 |
| activity | 实例 + 活动 | 支持 |
| audit | 活动 + 任务 + 变量主路径 | 当前默认 |
| full | 完整明细 | 数据结构存在，异步/清理未实现 |

## 相关阅读

- [SDK 持久化](/docs/sdk/persistence)
- [配置参考](/docs/reference/config)
- [SDK 限制](/docs/sdk/limits)
