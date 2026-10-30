---
title: 持久化与数据库进度
description: 内存、JSON 状态文件、MySQL 快照及 MySQL/PostgreSQL 行仓储五种形态，以及单执行者、锁和恢复边界。
order: 30
section: progress
status: prototype
since: v0.1.0
tags: [持久化, MySQL, PostgreSQL, 行仓储]
---

## 五种状态形态

| 形态 | 启用方式 | 写入粒度 | 适用场景 | 状态 |
| --- | --- | --- | --- | --- |
| 内存 | 默认 | 无持久化 | 单测、演示 | 已验证 |
| JSON 状态文件 | <code>-state ./data/engine.json</code> | 全量原子快照 | 开发、小规模单进程 | 已验证 |
| SQL 快照 | <code>-storage mysql</code> | 全量快照写入 | 单执行者数据库部署 | 原型可用 |
| MySQL 行仓储 | <code>-storage mysql-rows</code> | 命令后增量行写入 | 单执行者、需要 SQL 可查 | 真实库验收通过 |
| PostgreSQL 行仓储 | <code>-storage postgres-rows</code> | 同一持锁会话内的命令事务 | 单执行者、需要 SQL 可查 | 真实库验收通过 |

快照与行仓储互斥：引擎只允许一个权威状态源。行仓储模式启动时从 <code>ACT_*</code> 行恢复，命令完成后增量写入；关闭时释放独占锁。

## Schema 与行映射

- 复用 Flowable 8.1.0.3 的 41 张 <code>ACT_*</code> 表，MySQL 与 PostgreSQL 各一份 DDL。
- <code>REV_</code> 乐观锁、主键、外键顺序、事务工作单元与行差异写入已实现。
- 变量、部署资源、附件等二进制内容写入 <code>ACT_GE_BYTEARRAY</code>。
- 仓库、运行时、作业/订阅/死信、历史/评论/附件/身份链接/变量明细/任务日志及五张身份标准表均已有行映射；成员表使用复合主键，身份资料以标准列为真实来源，Go 安全扩展存于独立控制行。
- <code>make test-mysql</code> 在 MySQL 8.0.35、<code>make test-postgres</code> 在 PostgreSQL 16.3 的临时库上验证。

## 单执行者约束

行仓储模式使用数据库独占锁，锁名与方言相关；同一状态源同时只允许一个服务进程。连接丢失后引擎拒绝提交，不提供故障转移或“外部副作用恰好一次”。

::: danger 禁止多写者
没有集群租约、作业抢占、缓存失效或跨节点协调。共享同一个数据库同时启动多个写者会造成状态分叉。灰度只能按独立数据库、租户或固定实例集合划分。
:::

## 查询下推进度

| 查询 | 当前行为 | 状态 |
| --- | --- | --- |
| 运行任务列表 | 直接列、流程定义 Key、候选人、租户条件、排序与分页下推 SQL | 已验证（MySQL，默认租户）；租户条件已编码 |
| 实例查询 | 等值列、流程定义 Key、挂起状态、租户条件、开始时间与分页下推 SQL | 双库验收（MySQL 8.0.35 / PG 16.3） |
| 历史实例查询 | 等值列、流程定义 Key、结束状态、租户条件、开始/结束时间与分页下推 SQL | 双库验收（MySQL 8.0.35 / PG 16.3） |
| 历史任务查询 | 等值列、流程定义 Key、结束状态、租户条件、开始/结束时间与分页下推 SQL | 双库验收（MySQL 8.0.35 / PG 16.3） |
| 历史活动查询 | 等值列、流程定义 Key、结束状态、租户条件、开始/结束时间与分页下推 SQL | 双库验收（MySQL 8.0.35 / PG 16.3） |
| 历史变量查询 | 等值列与分页下推 SQL；表无租户列，租户作用域回退内存 | 双库验收（MySQL 8.0.35 / PG 16.3） |
| 变量过滤 | 内存过滤 | 未下推 |

任务、实例与历史查询下推只改变执行位置，不改变结果契约；内存路径与 SQL 路径由一致性测试对照。详见 [存储模式](/docs/operations/storage) 与 [075 实例与历史查询下推](https://github.com/flowable-go/flowable/blob/master/docs/002-迁移实施/075-实例与历史查询下推.md)、[078 历史活动与历史变量查询下推](https://github.com/flowable-go/flowable/blob/master/docs/002-迁移实施/078-历史活动与历史变量查询下推.md)。

## 恢复与校验

- JSON 状态文件原子写入，启动时恢复；失败时回滚。
- 行仓储启动时从行表恢复，行是唯一权威来源。
- <code>flowable-migrate -verify-state</code> 在隔离内存引擎中独立恢复状态文件并输出实体计数，不连接数据库。
- verify-state 配合 verify-source 可只读核对源库与准备状态的行数及内容指纹。
- 没有自动备份、PITR 或在线迁移；生产备份仍依赖数据库自身能力。

## 未完成项

1. 查询下推的双库真实执行验收；运行时变量实例（ACT_RU_VARIABLE）仍由内存合成，未下推。
2. 集群租约、作业抢占、故障转移与幂等。
3. 异步历史写入与定期清理。
4. 真实数据库上的容量基线与长稳测试。
5. 从仓库外模块配置行仓储：<code>flowable.Config</code> 字段引用内部包类型，外部模块目前只能使用内存或 <code>StatePath</code>。

## 相关阅读

- [存储模式与容量](/docs/operations/storage)
- [备份、恢复与状态校验](/docs/operations/backup)
- [SDK 持久化](/docs/sdk/persistence)
