---
title: 排障手册
description: 端口、数据库锁、作业、认证、状态文件与迁移工具的常见故障定位顺序。
order: 50
section: operations
status: prototype
since: v0.1.0
tags: [运维, 排障, FAQ]
---

## 启动失败

| 现象 | 排查 |
| --- | --- |
| 端口占用 | <code>ss -ltnp</code> 查 8080，改 <code>-addr</code> |
| 状态文件无法创建 | 父目录是否存在、权限、磁盘空间 |
| 数据库连接失败 | <code>.env</code>/DSN、网络、账号、库是否存在 |
| Schema 版本不符 | 先执行 <code>-apply</code>，检查 <code>ACT_GE_PROPERTY</code> |
| 行仓储锁获取失败 | 是否有另一个服务实例在运行 |
| TLS 参数错误 | 证书/私钥/CA 是否成对，客户端认证模式是否合法 |

## 请求失败

| 状态 | 常见原因 |
| --- | --- |
| 401 | 未带凭据、Token 过期、Basic 密码错误 |
| 403 | 跨租户、RBAC 权限不足 |
| 404 | 路径缺少 <code>/service</code>、资源不存在 |
| 400 | 请求体字段/类型错误、任务已结束 |
| 409 | 并发冲突、状态不允许 |
| 429 | 触发限流 |
| 500 | 引擎内部错误，查看日志与错误码 |

## 流程不推进

1. 查询实例与执行树，确认当前活动。
2. 用户任务是否已创建、assignee/候选人是否正确。
3. 服务任务委托是否已注册；未注册通常报 <code>ENGINE-0510</code>。
4. 作业是否到期、后台扫描是否开启（<code>-jobs-interval</code>）。
5. 作业是否进入死信；查看异常栈并重试。
6. 事件订阅是否存在，消息/信号名称是否匹配。
7. 条件表达式是否求值为 false 且无默认流。

## 数据不一致

- JSON 状态文件只允许单进程访问。
- 行仓储与快照不能混用。
- 多执行者共享数据库会分叉；立即停止一个写者。
- 外部副作用不随内存回滚，检查业务幂等与补偿。
- 用 <code>-verify-state</code> 校验离线状态文件。

## 迁移工具

| 现象 | 原因 |
| --- | --- |
| 退出码 2 | 报告存在阻断，按问题码整改 |
| 退出码 1 | 参数/连接/扫描/输出失败 |
| 报告无法覆盖 | 目标已存在；工具不覆盖已有报告 |
| 预检通过但转换失败 | 复杂作用域/作业/订阅/变量类型阻断 |
| 静默检查不稳定 | 源库仍有写入或后台作业 |

## 收集诊断信息

~~~bash
./bin/flowtide-server -h
curl -sS http://localhost:8080/healthz
curl -sS http://localhost:8080/readyz
curl -sS http://localhost:8080/metrics | head
curl -sS http://localhost:8080/service/management/engine
tail -n 200 server.log
~~~

提交问题时附：版本/提交、部署模式、数据库版本、复现步骤、错误码与脱敏日志。不要附真实变量值、Token、证书私钥或连接串。

## 相关阅读

- [安装](/docs/getting-started/installation)
- [存储模式与容量](/docs/operations/storage)
- [已知问题](/docs/compatibility/known-issues)
