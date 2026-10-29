---
title: REST 与安全进度
description: 242 个方法端点的覆盖情况、认证方式、mTLS 身份映射与尚未完成的权限审计。
order: 40
section: progress
status: prototype
since: v0.1.0
tags: [REST, 认证, mTLS, 租户]
---

## 路由覆盖

| 指标 | 数值 | 含义 |
| --- | ---: | --- |
| 官方方法端点 | 184 | 以官方流程 OpenAPI 为基线 |
| 实现方法端点 | 242 | 含扩展端点 |
| 官方缺失 | 0 | 路径与方法均已注册 |
| 扩展端点 | 58 | 不计入官方覆盖 |
| variables-async | 6 | 持久异步与原版 HTTP 场景通过（单执行者） |
| 任务方法原版回放 | 8 | 26 个完整响应及副作用场景通过 |
| 实例与执行原版回放 | 10 | 42 个完整响应及副作用场景通过 |
| 身份方法原版回放 | 19 | 52 个完整响应、二进制及副作用场景通过 |

路径存在只说明路由已编码。请求体字段、响应模式、分页、错误码、副作用和并发行为仍需要逐项 Java 对照。对照表与可筛选清单见 [REST 浏览器](/rest-explorer)。

## 本轮行为验收

- 184 个官方方法逐项检查真实路由、401、只读角色写入 403、管理域权限及管理员空输入处理。
- POST 查询接口使用 read 权限；拼接 JSON、尾部垃圾和 null 返回 400。
- Java 黄金样本 approve/review/completed 通过 Go HTTP 链路核对任务字段、历史状态、变量数量和完成副作用；这不是全量 Java REST 原始响应逐字段验收。
- 六个异步变量方法经原版 Java 控制器 14 个 HTTP 场景逐字段回放，覆盖排队、副作用、冲突、名称不匹配、对象误传及空数组；双库重启后执行通过。实例集合成功 201，执行集合及单变量成功 204，均为空正文。
- 八个任务方法完成 26 个原版 HTTP 场景回放，覆盖完整 DTO、显式空值、UTC 毫秒、分页及优先级、签收/委派/处理和错误正文。动作成功统一为 200 空正文，目标处理人使用 assignee。
- 十个流程实例及执行方法完成 42 个原版 HTTP 场景回放，覆盖完整 DTO、启动选择器、消息启动、类型变量、分页、属性更新、签收、挂起/激活和实际任务副作用。实例动作成功为 200；原版重复实例签收实际为 500，核心 API 仍保留冲突语义。
- 实例元数据保存于标准根行，双库重启后可恢复启动人、业务状态、到期及签收时间；旧 Go 控制格式仍可读。版本号改为按租户和定义键递增。
- 十九个身份方法完成 52 个原版场景，覆盖完整对象、查询、局部更新、成员冲突、信息键与空值、头像二进制及文件媒体类型；五张身份标准行在双库行仓储重启后恢复，账户类型信息仍拒绝导入。
- 审计记录认证后的主体、有效租户及实际终态响应码，失败 Basic 凭据不能冒充操作人。

## 分域规模

| 业务域 | 端点 | 官方 | 扩展 |
| --- | ---: | ---: | ---: |
| runtime | 96 | 71 | 25 |
| management | 41 | 32 | 9 |
| repository | 34 | 31 | 3 |
| identity | 30 | 19 | 11 |
| history | 29 | 21 | 8 |
| query | 10 | 8 | 2 |
| form | 2 | 2 | 0 |

## 认证能力

| 方式 | 实现 | 当前接入 |
| --- | --- | --- |
| Basic Auth | 静态用户或身份库 PBKDF2 | 服务端 <code>-basic-auth user:pass</code> |
| 静态 Bearer | 固定 Token | 服务端 <code>-bearer-token</code> |
| JWT / JWKS | RS256/HS256、<code>iss/aud/exp</code>、JWKS 缓存 | 通过 <code>rest.WithJWT</code> 嵌入 |
| mTLS | 客户端证书校验、CN/O/OU 身份映射 | 服务端 TLS 参数 |
| RBAC | read/write/admin 端点权限 | 通过 <code>rest.WithRBAC</code> 嵌入 |
| 租户绑定 | 用户/证书到租户、越权拒绝 | 通过选项嵌入 |
| 限流 | 令牌桶 | 服务端 <code>-rate-limit</code> |

::: info 标准二进制安全接入
标准 <code>flowable-server -security-config /受限目录/security.json</code> 已支持 JWT/JWKS、RBAC、身份库认证、租户绑定、内存审计及 HTTP 指标。配置文件支持 basicUsers、bearerToken、identityAuth、jwt、userRoles、rolePermissions、tenantBindings、tenantAdministrators、auditCapacity、metrics；解析失败不回显密钥。原 Basic/Bearer flag 在配置文件之后生效。
:::

## mTLS 与证书身份

<code>-tls-client-ca</code> 设置客户端 CA，<code>-tls-client-auth</code> 支持 <code>none/request/require-any/require-verify</code>。<code>-tls-client-identity</code> 只在 <code>require-verify</code> 下开启，且仅对 <code>/service/</code> 路径、未携带 <code>Authorization</code> 头的请求生效。

主体映射规则：CN 作为用户，O 作为租户，O 为空时回退 OU。证书 CN 可通过 userRoles 显式映射 RBAC 角色；没有映射时拒绝访问受控端点。O/OU 只绑定租户，CRL/OCSP 未实现。

## 已实现的安全边界

- 未认证请求返回 401，越权租户返回 403。
- 认证优先级：Basic/Bearer 失败不会回退到证书身份。
- 租户头 <code>X-Tenant-Id</code> 与查询参数 <code>tenantId</code> 受绑定租户约束。
- 审计日志、HTTP/引擎指标、健康检查与限流已实现。
- 部署资源支持 ETag/Last-Modified 与条件请求 304。

## 未完成项

- 全域资源级权限组合与跨租户负向回放审计。
- 更复杂的资源级权限组合与身份治理审计。
- CRL/OCSP、密钥轮换与令牌吊销。
- 其余方法的完整原版 Java DTO、分页、空值、错误与级联副作用回放。
- 外部持久审计存储与安全配置的在线重载。

## 相关阅读

- [REST 约定](/docs/rest/conventions)
- [认证与租户](/docs/rest/auth)
- [REST 覆盖与差异](/docs/compatibility/rest)
