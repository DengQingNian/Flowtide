---
title: 备份、恢复与状态校验
description: 备份 JSON 状态与数据库、用 -verify-state 校验状态文件，以及恢复演练的边界。
order: 20
section: operations
status: prototype
since: v0.1.0
tags: [运维, 备份, 恢复, 校验]
---

## JSON 状态文件

- 状态文件包含业务值、变量与历史，按部署要求限制权限。
- 写入为原子替换；备份应在服务停止或低写入窗口进行。
- 不要只备份文件而不验证可恢复性。
- 状态文件不能与行仓储/快照模式混用。

## 数据库

行仓储与 SQL 快照的权威数据都在数据库。使用数据库原生备份：

- MySQL：物理/逻辑备份 + binlog（如需要 PITR）。
- PostgreSQL：基础备份 + WAL 归档。
- 备份必须包含全部 <code>ACT_*</code> 表与相关 Schema。
- 恢复后检查引擎属性中的 Schema 版本与初始化标记。

## 状态文件恢复校验

<code>-verify-state</code> 在隔离内存引擎中按服务相同入口恢复文件，输出计数与摘要，不修改状态文件：

~~~bash
./bin/flowable-migrate -verify-state -state prepared-state.json
./bin/flowable-migrate -verify-state -state prepared-state.json -out verify-report.json
./bin/flowable-migrate -verify-state -state prepared-state.json -timeout 30s
~~~

报告字段：

| 字段 | 含义 |
| --- | --- |
| <code>format</code> | 固定 <code>flowable-go-state</code> |
| <code>version</code> | 状态格式版本 |
| <code>bytes</code> / <code>sha256</code> | 文件字节数与摘要 |
| <code>deployments</code> / <code>definitions</code> | 部署与定义数量 |
| <code>runningInstances</code> / <code>endedInstances</code> | 运行/结束实例 |
| <code>tasks</code> | 当前任务数 |
| <code>historicProcessInstances</code> | 历史实例容器 |
| <code>historicTaskInstances</code> / <code>historicVariables</code> | 历史任务/变量 |
| <code>sequenceFlowHistory</code> | 顺序流历史标记 |
| <code>limitations</code> | 固定边界说明 |

退出码：0 恢复成功，1 文件/解析/恢复/输出失败。

::: warning 校验通过不等于数据一致
它只证明状态文件可被当前引擎恢复，不证明与源库数量一致，也不替代停写、备份或回退演练。
:::

## 恢复演练

1. 在隔离环境恢复备份。
2. 启动服务并检查 <code>/readyz</code> 与 <code>/service/management/engine</code>。
3. 验证关键实例、任务与历史。
4. 记录恢复耗时、数据损失窗口与人工步骤。
5. 将演练结果与版本/提交绑定存档。

## 相关阅读

- [存储模式与容量](/docs/operations/storage)
- [停写切换](/docs/migration/cutover)
- [回退与验证](/docs/migration/rollback)
