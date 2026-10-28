---
title: 存储模式与容量
description: 内存、JSON、SQL 快照与行仓储的配置、锁、容量基线和选择建议。
order: 10
section: operations
status: prototype
since: v0.1.0
tags: [运维, 存储, 容量, MySQL]
---

## 选择存储

| 场景 | 推荐 | 原因 |
| --- | --- | --- |
| 单元测试/演示 | 内存 | 无外部依赖 |
| 本地开发/小规模单进程 | JSON 状态文件 | 简单、可恢复 |
| 单执行者数据库部署 | MySQL 行仓储 | SQL 可查、增量写 |
| 快速数据库验证 | MySQL 快照 | 接入简单、全量写 |
| 多节点/高可用 | 暂不支持 | 无集群租约 |

## 启动参数

| 参数 | 说明 |
| --- | --- |
| <code>-state</code> | JSON 状态文件路径 |
| <code>-storage mysql</code> | SQL 快照模式 |
| <code>-storage mysql-rows</code> | 行仓储模式 |
| <code>-env</code> | MySQL 连接配置文件 |
| <code>-history</code> | 历史级别 |
| <code>-jobs-interval</code> | 定时器扫描间隔，0 关闭 |
| <code>-tenant</code> | 默认租户 |

~~~bash
./bin/flowable-server -storage mysql-rows -env .env -jobs-interval 1s
~~~

行仓储与快照互斥；同时配置 <code>-state</code> 与数据库模式会被拒绝。

## 锁与单执行者

行仓储模式在专用连接上持有数据库独占锁。锁名按方言与 Schema 生成；第二个服务进程启动时无法获取锁，应直接失败。关闭顺序为：先停止作业、再保存/提交、最后释放锁与连接池。

::: danger 禁止共享
不要用同一个数据库启动多个 <code>mysql-rows</code> 实例来“提高吞吐”。当前没有作业仲裁与跨进程协调，会造成重复执行与状态分叉。
:::

## 容量基线

2026-10-28 在本机 QEMU（4 vCPU、内存状态）上的首轮结果：

| 场景 | 耗时 | 内存/次 | 分配/次 |
| --- | ---: | ---: | ---: |
| 启动并完成用户任务 | 23.43 ms | 7.61 MB | 42,381 |
| 完成任务 | 15.01 ms | 6.57 MB | 33,058 |
| 1000 任务分页查询 100 条 | 5.24 ms | 1.90 MB | 4,730 |
| 1000 行全量导出 | 274.57 ms | 62.74 MB | 854,263 |
| 1000 行单行变化增量 | 2.97 ms | 0.78 MB | 16,013 |

> 这是本机观察值，不是生产容量承诺。命令开销主要来自状态复制与历史写入；全量快照是当前最大单点。

## 查询下推

默认租户的运行任务列表已按直接列、流程定义 Key、候选人与排序/分页下推 SQL。实例、历史、多租户与变量过滤仍可能退化为内存扫描。数据量大时应避免高频管理查询。

## 备份与恢复

数据库模式必须使用数据库自身的备份/PITR；JSON 模式需备份状态文件。引擎没有内建自动备份。详见 [备份、恢复与状态校验](/docs/operations/backup)。

## 相关阅读

- [持久化架构](/docs/architecture/persistence)
- [持久化与数据库进度](/docs/progress/persistence)
- [SDK 持久化](/docs/sdk/persistence)
