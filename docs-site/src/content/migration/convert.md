---
title: 离线状态转换
description: 用 -convert 把简单用户任务执行树转换为 Go 状态文件，以及 Java H2 对照和阻断边界。
order: 30
section: migration
status: prototype
since: v0.1.0
tags: [迁移, 转换, 状态文件]
---

## 作用

<code>-convert</code> 只读扫描源库，把**简单用户任务执行树**转换为 Go 状态文件。它不写源库、不执行停写，也不生成反向转换。

## 命令

~~~bash
# 先做预检；存在阻断时不会生成状态文件
./bin/flowable-migrate -preflight -dialect mysql -env .env -out preflight.json

# 目标路径必须不存在
./bin/flowable-migrate -convert -dialect mysql -env .env -out prepared-state.json

# 用同一状态文件启动服务
./bin/flowable-server -state prepared-state.json
~~~

PostgreSQL 源库沿用 <code>FLOWABLE_POSTGRES_DSN</code>。

## 转换范围

| 对象 | 处理 |
| --- | --- |
| 部署与流程定义 | 保留 Key、版本、资源 |
| 运行中实例 | 仅简单用户任务执行树 |
| 执行与任务 | 保留活动/任务 Key 与变量 |
| 历史 | 保留 Java audit 列，补记顺序流标记 |
| 变量 | 支持受控类型；<code>serializable</code> 阻断 |
| 作业/订阅 | 阻断，不转换 |
| 复杂作用域 | 多实例、并发、调用、事件子流程等阻断 |

## 产物

- 新状态文件，格式为 <code>flowable-go-state</code>。
- 文件包含业务值，权限 <code>0600</code>，不得提交。
- 转换过程会做内存恢复，但不产出独立校验报告；建议再用 <code>-verify-state</code> 校验。

## Java 对照

仓库内 Java H2 真实样本的 <code>approve → review → completed</code> 路径已通过转换与续跑对照。这只覆盖简单用户任务，不代表多实例、作业、订阅或生产数据。

## 边界

- 不是通用存量转换器。
- 不处理 Java 委托/监听器/脚本的语义改写。
- 不保证源库与状态数量一致，需在停写快照上人工核对。
- 转换成功不等于可切换，更不等于可回退。

## 相关阅读

- [状态恢复校验](/docs/operations/backup)
- [停写切换](/docs/migration/cutover)
- [迁移限制与 FAQ](/docs/migration/limits)
