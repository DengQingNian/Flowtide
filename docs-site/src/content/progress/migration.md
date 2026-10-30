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
| 新建库原型 | MySQL/PostgreSQL 行服务、独占、恢复及断线回滚证据 | 仅限单执行者范围 |
| 既有库只读预检 | 保守分析子集已实现 | 否，预检无阻断不等于可切换 |
| 停写切换续跑 | 双库临时库的 Java 黄金行转换、核对、行模式续跑及重启通过；生产停写未完成 | 否，不能替代生产切换 |
| 可回退迁移 | 双库源快照还原与指纹核对通过；反向转换及 Java 增量接管未完成 | 否，不能宣称 Java 可接管 Go 增量 |

每种迁移状态都需要单独验收。初始化 Schema 或在空库部署后，仍需使用存量数据验证预检、转换、续跑和回退。

## 工具矩阵

| 命令 | 作用 | 是否连接源库 | 主要产物 | 退出码 |
| --- | --- | --- | --- | --- |
| <code>-preflight</code> | 只读扫描存量库并输出阻断报告 | 是，只读 | JSON 报告 | 0/2/1 |
| <code>-convert</code> | 将简单用户任务执行树转换为 Go 状态文件 | 是，只读 | 新状态文件 | 0/1 |
| <code>-cutover-check</code> | 观察窗口前后两次快照比较 | 是，只读 | 稳定性报告 | 0/2/1 |
| <code>-verify-state</code> | 隔离内存引擎恢复状态并统计实体 | 否 | 校验报告 | 0/1 |
| <code>-verify-state -verify-source</code> | 重新转换只读源快照，与状态逐表核对数量及内容指纹 | 是，只读 | 核对报告 | 0/2/1 |

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
- 单独 verify-state 不证明源库与状态一致；增加 verify-source 后核对规范行投影的数量与内容，仍不替代停写或回退。

## 推荐执行顺序

~~~bash
# 1. 只读预检
./bin/flowtide-migrate -preflight -dialect mysql -env .env -out preflight.json

# 2. 预检无阻断后准备离线状态
./bin/flowtide-migrate -convert -dialect mysql -env .env -out prepared-state.json

# 3. 独立恢复校验
./bin/flowtide-migrate -verify-state -state prepared-state.json -out verify-report.json

# 4. 在停写的源快照上核对准备状态
./bin/flowtide-migrate -verify-state -verify-source -dialect mysql -env .env -state prepared-state.json -out reconcile.json

# 5. 停写窗口内检查静默
./bin/flowtide-migrate -cutover-check -dialect mysql -env .env -quiesce-window 30s -out cutover.json
~~~

::: danger 不能省略的步骤
双库临时库已验证 Java 黄金行预检、转换、逐表核对、Go 行模式续跑、重启及源快照还原。实际 Java 执行者停写、生产物理备份、反向转换与 Java 增量接管仍由迁移团队另行验收。
:::

## 相关阅读

- [迁移总览](/docs/migration)
- [存量库只读预检](/docs/migration/preflight)
- [停写切换与回退](/docs/migration/cutover)
