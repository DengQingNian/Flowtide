---
title: 持久化架构
description: 状态快照、SQL 快照与行仓储的边界，工作单元、乐观锁、恢复与单执行者锁。
order: 30
section: architecture
status: prototype
since: v0.1.0
tags: [持久化, 工作单元, 行仓储, 恢复]
---

## 状态模型

引擎的可变状态包括部署、流程定义、运行实例、执行树、任务、变量、作业、订阅、批次与历史容器。状态层有两种表达：

1. **快照**：整棵状态树序列化为 JSON 或 SQL 行，适合小规模与开发。
2. **行仓储**：把状态映射为 <code>ACT_*</code> 行，数据库是权威来源，适合需要 SQL 可查的场景。

两者互斥。行仓储模式下 <code>StatePath</code> 或 <code>StateStore</code> 必须为空；否则构造引擎时直接拒绝。

## 快照模式

~~~text
Engine state ──serialize──► JSON / SQL snapshot ──atomic write──► storage
                             ▲
                             └── recovery on startup
~~~

- JSON 文件使用同目录临时文件 + 原子替换，失败保留旧版本。
- SQL 快照把所有状态写入一张/一组桥接行，依靠事务提交。
- 全量写入是 O(state)，实例多时开销明显。

## 行仓储模式

~~~text
Engine command
   │
   ├─ 读取当前行（启动恢复或缓存）
   ├─ 内存执行
   ├─ 计算行差异
   └─ 事务写入 ACT_*（REV_ 乐观锁）
~~~

- 启动时从 41 张表恢复状态，行是唯一权威来源。
- 命令后计算增量行操作；删除、更新、插入按外键顺序执行。
- <code>REV_</code> 用于乐观锁；版本冲突使命令失败。
- 二进制内容进入 <code>ACT_GE_BYTEARRAY</code>。
- 连接持有独占锁，第二个写者被拒绝。

## 表族

| 表族 | 示例 | 内容 |
| --- | --- | --- |
| 通用 | <code>ACT_GE_PROPERTY</code>、<code>ACT_GE_BYTEARRAY</code> | 引擎属性、资源/变量字节 |
| 仓库 | <code>ACT_RE_DEPLOYMENT</code>、<code>ACT_RE_PROCDEF</code> | 部署与流程定义 |
| 运行时 | <code>ACT_RU_EXECUTION</code>、<code>ACT_RU_TASK</code> | 实例、执行、任务 |
| 作业 | <code>ACT_RU_JOB</code>、<code>ACT_RU_TIMER_JOB</code> | 异步与定时作业 |
| 历史 | <code>ACT_HI_PROCINST</code>、<code>ACT_HI_TASKINST</code> | 实例/活动/任务/变量历史 |
| 身份 | <code>ACT_ID_USER</code>、<code>ACT_ID_GROUP</code> | 用户、组、成员关系 |

## 恢复与校验

- JSON 状态：启动时反序列化并恢复索引；版本不合法时拒绝。
- 行仓储：启动时读取行并重建内存状态；行缺失或外键不一致时失败。
- <code>-verify-state</code>：在隔离内存引擎中按同一恢复入口重放状态文件，输出实体计数与摘要，不写状态。
- 没有自动迁移旧状态格式的机制；跨版本升级需重新生成或转换。

## 已知限制

- 快照写入全量、单执行者，无法支撑大数据量与高写入。
- 行仓储仍需要在内存中维护完整状态，启动恢复是 O(数据量)。
- 查询下推只覆盖默认租户任务列表。
- 无异步历史、无定期清理、无 PITR。
- 无集群租约、作业抢占与故障转移。

## 相关阅读

- [持久化与数据库进度](/docs/progress/persistence)
- [存储模式与容量](/docs/operations/storage)
- [备份、恢复与状态校验](/docs/operations/backup)
