---
title: SDK 持久化
description: 内存、JSON 状态文件与数据库模式的差别，以及仓库外模块当前能配置到什么程度。
order: 50
section: sdk
status: prototype
since: v0.1.0
tags: [SDK, 持久化, 状态文件]
---

## 可选模式

| 模式 | SDK 可配置 | 写入 | 恢复 | 多进程 |
| --- | --- | --- | --- | --- |
| 内存 | 是 | 无 | 无 | 不适用 |
| JSON 状态文件 | 是 | 全量原子快照 | 启动恢复 | 禁止 |
| MySQL 快照 | 仓库外不可配置 | 全量 SQL 快照 | 启动恢复 | 禁止 |
| MySQL 行仓储 | 仓库外不可配置 | 命令后增量行 | 启动恢复 | 禁止 |

数据库模式当前通过 <code>cmd/flowable-server -storage mysql</code> 或 <code>mysql-rows</code> 使用。原因是 <code>Config</code> 中的 <code>StateStore</code> 与 <code>RowRepository</code> 引用内部包类型。

## JSON 状态文件

~~~go
app, err := flowable.New(flowable.Config{StatePath: "./data/engine.json"})
if err != nil {
	log.Fatal(err)
}
defer app.Close(ctx)
~~~

- 文件保存定义、实例、任务、作业、历史与身份数据。
- 写入使用临时文件 + 原子替换，失败保留旧文件。
- 启动时若文件存在则恢复，不存在则创建空状态。
- 文件包含业务值，权限应由部署环境限制；不要提交到 Git。
- 同一文件只能被一个引擎实例打开。

## 关闭与刷盘

<code>Close(ctx)</code> 会保存最终状态并释放资源。进程被强制杀死时，最后一次成功写入之后的数据可能丢失；需要更强保证时使用数据库模式并配合备份。

## 恢复校验

状态文件可用迁移工具独立校验：

~~~bash
./bin/flowable-migrate -verify-state -state ./data/engine.json
~~~

该命令在隔离内存引擎中恢复并输出实体计数，不修改状态文件，也不证明数据库与状态一致。

## 事务与外部副作用

引擎保证内存状态与持久化的命令级一致，但不保证外部副作用（HTTP、消息、文件）原子。委托中调用外部系统时，必须使用幂等键、补偿或本地消息表。

## 相关阅读

- [引擎配置](/docs/sdk/config)
- [持久化架构](/docs/architecture/persistence)
- [备份、恢复与状态校验](/docs/operations/backup)
