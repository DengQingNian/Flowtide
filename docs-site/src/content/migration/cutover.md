---
title: 停写切换与静默检查
description: 停写窗口、-cutover-check 的内容指纹、退出码，以及切换前后必须完成的人工步骤。
order: 40
section: migration
status: prototype
since: v0.1.0
tags: [迁移, 停写, 静默检查, 切换]
---

## 停写检查做什么

<code>-cutover-check</code> 在观察窗口前后各做一次独立只读快照，比较表集合、行数、内容指纹、Schema 版本与作业计数。两次扫描各自使用 <code>ReadOnly=true</code>、<code>RepeatableRead</code> 事务并回滚，不复用同一事务。

## 命令

~~~bash
# MySQL；只读检查，不初始化源库
./bin/flowtide-migrate -cutover-check -dialect mysql -env .env -quiesce-window 30s -out cutover.json

# PostgreSQL
./bin/flowtide-migrate -cutover-check -dialect postgres -timeout 5m -quiesce-window 60s
~~~

参数约束：

- 与 <code>-preflight</code>、<code>-convert</code>、<code>-apply</code>、<code>-tables</code> 互斥。
- 不接受 <code>-tenant</code>。
- <code>-quiesce-window</code> 必须为正。
- <code>-out</code> 不能覆盖已有文件、配置文件或其链接。

## 退出码

| 退出码 | 含义 | 动作 |
| ---: | --- | --- |
| 0 | 两次快照一致 | 可继续准备，但仍需停写、备份、数量核对与回退演练 |
| 2 | 检测到变化 | 停止切换，定位写入来源并重新观察 |
| 1 | 参数/连接/扫描/输出失败 | 修复后重跑，不用旧报告 |

## 报告字段

<code>dialect</code>、<code>schemaVersion</code>、<code>windowSeconds</code>、<code>stable</code>、<code>scanTimes</code>、<code>tables</code>（前后行数、指纹、是否变化）、<code>changedTables</code>、<code>jobCounts</code>、<code>limitations</code>。

内容指纹对列名、类型标记、长度前缀和值编码后做 SHA-256，行/列顺序与时区不影响；不同表示（字符串“1”、字节 0x31、数字 1）会产生不同指纹。未知表只比较行数，同数改值无法发现。

## 切换前检查清单

- [ ] 业务写入入口已关闭，Java 与 Go 后台作业均已停止。
- [ ] 已完成数据库备份，并在隔离环境验证恢复。
- [ ] 预检报告无阻断，且报告基于停写快照。
- [ ] 转换状态文件已生成并通过 <code>-verify-state</code>。
- [ ] 源库、状态文件与目标库的实体数量已核对。
- [ ] Java 续跑对照样本通过。
- [ ] 只启用一个执行者；数据库独占锁可获取。
- [ ] 监控、告警、回退触发条件与负责人就绪。
- [ ] 回退窗口与人工停止条件已书面确认。

## 切换后

1. 观察健康、任务积压、作业、错误码与数据库锁。
2. 验证关键流程闭环与历史写入。
3. 保留 Java 库只读快照与备份，直到回退窗口结束。
4. 如果出现状态分叉或锁异常，立即停止写入并按回退手册处理。

::: warning stable=true 的含义
它只证明观察窗口内未发现变化，不证明数据可转换、可切换或可回退。不要把它当作切换放行开关。
:::

## 相关阅读

- [回退与验证](/docs/migration/rollback)
- [状态恢复校验](/docs/operations/backup)
- [迁移限制与 FAQ](/docs/migration/limits)
