---
title: 持久化与数据库进度
description: 内存、JSON 状态文件、MySQL 快照与行仓储四种形态，以及单执行者、锁和恢复边界。
order: 30
section: progress
status: prototype
since: v0.1.0
tags: [持久化, MySQL, PostgreSQL, 行仓储]
---

## 四种状态形态

| 形态 | 启用方式 | 写入粒度 | 适用场景 | 状态 |
| --- | --- | --- | --- | --- |
| 内存 | 默认 | 无持久化 | 单测、演示 | 已验证 |
| JSON 状态文件 | <code>-state ./data/engine.json</code> | 全量原子快照 | 开发、小规模单进程 | 已验证 |
| SQL 快照 | <code>-storage mysql</code> | 全量快照写入 | 单执行者数据库部署 | 原型可用 |
| 行仓储 | <code>-storage mysql-rows</code> | 命令后增量行写入 | 单执行者、需要 SQL 可查 | 原型可用 |

快照与行仓储互斥：引擎只允许一个权威状态源。行仓储模式启动时从 <code>ACT_*</code> 行恢复，命令完成后增量写入；关闭时释放独占锁。

## Schema 与行映射

- 复用 Flowable 8.1.0.3 的 41 张 <code>ACT_*</code> 表，MySQL 与 PostgreSQL 各一份 DDL。
- <code>REV_</code> 乐观锁、主键、外键顺序、事务工作单元与行差异写入已实现。
- 变量、部署资源、附件等二进制内容写入 <code>ACT_GE_BYTEARRAY</code>。
- 仓库、运行时、作业/订阅/死信、历史/评论/附件/身份链接/变量明细/任务日志均已有行映射。
- <code>make test-mysql</code> 在 MySQL 8.0.35、<code>make test-postgres</code> 在 PostgreSQL 16.3 的临时库上验证。

## 单执行者约束

行仓储模式使用数据库独占锁，锁名与方言相关；同一状态源同时只允许一个服务进程。连接丢失后引擎拒绝提交，不提供故障转移或“外部副作用恰好一次”。

::: danger 禁止多写者
没有集群租约、作业抢占、缓存失效或跨节点协调。共享同一个数据库同时启动多个写者会造成状态分叉。灰度只能按独立数据库、租户或固定实例集合划分。
:::

## 查询下推进度

| 查询 | 当前行为 | 状态 |
| --- | --- | --- |
| 默认租户运行任务列表 | 直接列、流程定义 Key、候选人、排序与分页下推 SQL | 已验证（MySQL） |
| 实例查询 | 内存扫描后过滤 | 未下推 |
| 历史查询 | 内存扫描后过滤 | 未下推 |
| 多租户任务查询 | 仅默认租户下推 | 未实现 |
| 变量过滤 | 内存过滤 | 未下推 |

任务查询下推只改变执行位置，不改变结果契约；内存路径与 SQL 路径由一致性测试对照。详见 [存储模式](/docs/operations/storage)。

## 恢复与校验

- JSON 状态文件原子写入，启动时恢复；失败时回滚。
- 行仓储启动时从行表恢复，行是唯一权威来源。
- <code>flowable-migrate -verify-state</code> 在隔离内存引擎中独立恢复状态文件并输出实体计数，不连接数据库。
- 没有自动备份、PITR 或在线迁移；生产备份仍依赖数据库自身能力。

## 未完成项

1. 实例、历史与多租户查询下推。
2. 集群租约、作业抢占、故障转移与幂等。
3. 异步历史写入与定期清理。
4. 真实数据库上的容量基线与长稳测试。
5. 从仓库外模块配置行仓储：<code>flowable.Config</code> 字段引用内部包类型，外部模块目前只能使用内存或 <code>StatePath</code>。

## 相关阅读

- [存储模式与容量](/docs/operations/storage)
- [备份、恢复与状态校验](/docs/operations/backup)
- [SDK 持久化](/docs/sdk/persistence)
