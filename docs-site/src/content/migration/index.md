---
title: 迁移总览与决策树
description: 从既有 Java Flowable 库迁移到 Go 的四类状态、工具顺序与不可省略的人工控制点。
order: 10
section: migration
status: prototype
since: v0.1.0
tags: [迁移, Java, 决策树]
---

## 先看结论

当前**没有通用、可回退的一键迁移**。工具链覆盖“只读预检、简单执行树离线转换、停写静默检查、状态恢复校验”，但停写、备份、数量核对、Java 续跑对照与回退演练仍需人工执行。

::: danger 不要这样切换
- 看到预检退出码 0 就启用 Go 接管。
- 让 Java 与 Go 同时写同一个库。
- 只保留旧镜像，不保留可恢复的数据备份。
- 用空库部署成功证明存量库兼容。
:::

## 四种迁移状态

| 状态 | 判定 | 当前 |
| --- | --- | --- |
| 新建库原型 | 新库 + 新流程 + 单执行者 | 已有双库本地证据 |
| 既有库只读预检 | 只读扫描、阻断报告 | 已实现保守子集 |
| 停写切换续跑 | 停写、备份、转换、数量核对、单执行者 | 工具就绪，演练未完成 |
| 可回退迁移 | Java 读取 Go 写入并续跑、反向转换/还原 | 未完成 |

四种状态不能相互推导。预检通过、转换成功、静默检查稳定都不等于可回退。

## 决策树

~~~text
存量 Java Flowable 库
   │
   ├─ 只想评估风险？ → -preflight 只读报告
   │
   ├─ 想验证简单流程能否转换？ → -preflight 通过后 -convert
   │        └─ -verify-state 独立恢复校验
   │
   ├─ 准备切换？ → 停写 → 备份 → -cutover-check → 数量核对
   │        └─ Java 续跑对照 → 单执行者启用 Go
   │
   └─ 要求可回退？ → 反向转换/备份还原演练 → 回退窗口与负责人
            └─ 未完成前不得宣称可回退
~~~

## 工具顺序

~~~bash
# 1. 只读预检；存在阻断时退出码 2
./bin/flowable-migrate -preflight -dialect mysql -env .env -out preflight.json

# 2. 预检无阻断后，准备离线状态文件
./bin/flowable-migrate -convert -dialect mysql -env .env -out prepared-state.json

# 3. 在隔离内存引擎中恢复校验
./bin/flowable-migrate -verify-state -state prepared-state.json -out verify-report.json

# 4. 停写窗口内做静默检查
./bin/flowable-migrate -cutover-check -dialect mysql -env .env -quiesce-window 30s -out cutover.json
~~~

## 人工控制点

| 控制点 | 产物 | 负责人 |
| --- | --- | --- |
| 停写确认 | 写入入口关闭、后台作业停止 | 运维/业务 |
| 备份与恢复演练 | 可恢复备份、恢复耗时记录 | DBA |
| 数量核对 | 源库与状态/目标库实体计数对照 | 迁移/QA |
| Java 续跑对照 | Java 读取 Go 写入的数据并推进 | 迁移/Java |
| 回退演练 | 还原步骤、窗口、停止条件 | 运维/负责人 |
| 切换窗口 | 变更单、监控、回滚触发条件 | 项目负责人 |

## 相关阅读

- [存量库只读预检](/docs/migration/preflight)
- [离线转换](/docs/migration/convert)
- [停写切换](/docs/migration/cutover)
- [回退与验证](/docs/migration/rollback)
- [迁移限制与 FAQ](/docs/migration/limits)
