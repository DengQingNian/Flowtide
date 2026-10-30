---
title: 配置参考
description: flowtide-server 命令行参数、Config 字段、默认值与配置边界。
order: 10
section: reference
status: verified
since: v0.1.0
tags: [参考, 配置, CLI]
---

## 命令行参数

| 参数 | 默认 | 说明 |
| --- | --- | --- |
| <code>-addr</code> | <code>:8080</code> | HTTP 监听地址 |
| <code>-history</code> | <code>audit</code> | 历史级别 |
| <code>-tenant</code> | 空 | 默认租户 |
| <code>-state</code> | 空 | JSON 状态文件；为空时内存 |
| <code>-storage</code> | 空 | <code>mysql</code> 快照或 <code>mysql-rows</code> 行仓储 |
| <code>-env</code> | <code>.env</code> | MySQL 配置文件路径 |
| <code>-jobs-interval</code> | <code>1s</code> | 定时器扫描间隔；0 关闭 |
| <code>-basic-auth</code> | 空 | <code>user:pass</code> |
| <code>-bearer-token</code> | 空 | 静态 Bearer Token |
| <code>-rate-limit</code> | 0 | 每秒请求数；0 关闭 |
| <code>-rate-burst</code> | 0 | 令牌桶容量 |
| <code>-tls-cert</code> | 空 | 服务端证书 |
| <code>-tls-key</code> | 空 | 服务端私钥 |
| <code>-tls-client-ca</code> | 空 | 客户端 CA |
| <code>-tls-client-auth</code> | <code>none</code> | none/request/require-any/require-verify |
| <code>-tls-min-version</code> | <code>1.2</code> | 1.2 或 1.3 |
| <code>-tls-client-identity</code> | false | 用证书建立身份 |

## 配置文件

<code>-env</code> 指向 MySQL 开发配置，键包括：

| 键 | 说明 |
| --- | --- |
| <code>db_mysql_host</code> | 主机 |
| <code>db_mysql_port</code> | 端口 |
| <code>db_mysql_user</code> | 用户 |
| <code>db_mysql_pwd</code> | 密码 |
| <code>db_mysql_db_name</code> | 数据库 |

PostgreSQL 迁移工具从 <code>FLOWABLE_POSTGRES_DSN</code> 读取连接。仓库内的 <code>.env</code> 只用于开发，不应提交真实凭据。

## Config 字段

| 字段 | 说明 |
| --- | --- |
| <code>HistoryLevel</code> | 历史级别 |
| <code>Clock</code> | 时钟注入（内部类型） |
| <code>IDs</code> | ID 生成器（内部类型） |
| <code>MaxAgendaSteps</code> | 单命令最大议程步数 |
| <code>TenantID</code> | 默认租户 |
| <code>StatePath</code> | JSON 状态文件 |
| <code>StateStore</code> | 自定义存储（内部类型） |
| <code>RowRepository</code> | 行仓储（内部类型） |
| <code>LoadRows</code> | 启动恢复行 |
| <code>LoadTenant</code> | 恢复租户 |
| <code>RecordSequenceFlowHistory</code> | 顺序流历史标记 |

仓库外模块只能稳定使用字符串/布尔字段；内部类型字段无法从外部命名。详见 [引擎配置](/docs/sdk/config)。

## 配置组合约束

- <code>-storage</code> 与 <code>-state</code> 不能混用。
- 行仓储与快照互斥。
- TLS 证书与私钥必须同时提供。
- 客户端身份要求 <code>require-verify</code>。
- 限流只在 rate 与 burst 都大于 0 时生效。
- Basic 格式必须是 <code>user:pass</code>，否则启动失败。

## 相关阅读

- [安装](/docs/getting-started/installation)
- [存储模式与容量](/docs/operations/storage)
- [TLS 与 mTLS 运维](/docs/operations/security)
