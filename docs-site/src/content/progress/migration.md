---
title: 迁移工具链进度
description: 预检、离线转换、停写静默检查与状态恢复校验的当前能力、退出码和边界。
order: 50
section: progress
status: prototype
since: v0.1.0
tags: [迁移, 预检, 停写, 回退]
---

## 四种迁移状态

| 状态 | 当前结果 | 是否可宣称通过 |
| --- | --- | --- |
| 新建库原型 | 双库本地证据 | 仅限新建库原型 |
| 既有库只读预检 | 保守分析子集已实现 | 否，预检无阻断不等于可切换 |
| 停写切换续跑 | 工具已具备，实际停写/备份/演练未完成 | 否 |
| 可回退迁移 | 反向转换与还原演练未完成 | 否 |

这四种状态分别取证，不能相互推导。初始化 Schema、读取相同表名、空库部署或 JSON 快照重启都不会让后三种状态自动通过。

## 工具矩阵

| 命令 | 作用 | 是否连接源库 | 主要产物 | 退出码 |
| --- | --- | --- | --- | --- |
| <code>-preflight</code> | 只读扫描存量库并输出阻断报告 | 是，只读 | JSON 报告 | 0/2/1 |
| <code>-convert</code> | 将简单用户任务执行树转换为 Go 状态文件 | 是，只读 | 新状态文件 | 0/1 |
| <code>-cutover-check</code> | 观察窗口前后两次快照比较 | 是，只读 | 稳定性报告 | 0/2/1 |
| <code>-verify-state</code> | 隔离内存引擎恢复状态并统计实体 | 否 | 校验报告 | 0/1 |

## 能力边界

### 预检

- 要求 14 张核心、事件订阅与 Java 作业表存在；非空未知表保守阻断。
- 检查变量类型、数值范围、外置字节、JSON、执行/任务归属与定义引用。
- Java 作业、订阅、委托、监听器、脚本与复杂作用域进入阻断报告。
- 全量内存扫描，未实现分页、流式分析与实例集合选择。
- 只支持无表名前缀的 MySQL/PostgreSQL <code>ACT_*</code> 布局。

### 离线转换

- 只覆盖简单用户任务执行树；复杂作用域、作业、订阅阻断。
- 输出只是新状态文件；不写源库，不执行停写。
- 状态文件权限 <code>0600</code>，包含业务值，不得提交。
- Java H2 真实样本的 approve/review/completed 路径已对照。

### 停写检查

- 两次独立只读事务，比较表集合、行数、内容指纹与作业计数。
- <code>stable=true</code> 只证明窗口内未观察到变化。
- 不替代停写、备份、数量核对与回退演练。
- 未知表只比较行数，无法发现同数改值。

### 状态恢复校验

- 使用与服务相同的恢复入口，在隔离内存引擎中恢复。
- 输出部署、定义、运行/结束实例、任务、历史实例/任务/变量计数。
- 不证明源库与状态数量一致，不替代停写或回退。

## 推荐执行顺序

~~~bash
# 1. 只读预检
./bin/flowable-migrate -preflight -dialect mysql -env .env -out preflight.json

# 2. 预检无阻断后准备离线状态
./bin/flowable-migrate -convert -dialect mysql -env .env -out prepared-state.json

# 3. 独立恢复校验
./bin/flowable-migrate -verify-state -state prepared-state.json -out verify-report.json

# 4. 停写窗口内检查静默
./bin/flowable-migrate -cutover-check -dialect mysql -env .env -quiesce-window 30s -out cutover.json
~~~

::: danger 不能省略的步骤
实际停写、备份、数量核对、Java 对照续跑与回退演练尚未自动化，必须由迁移团队按运行手册执行。工具通过不能替代这些人工控制点。
:::

## 相关阅读

- [迁移总览](/docs/migration)
- [存量库只读预检](/docs/migration/preflight)
- [停写切换与回退](/docs/migration/cutover)
