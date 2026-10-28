---
title: 认证与租户
description: Basic、Bearer、JWT/JWKS、mTLS、RBAC 与租户绑定的启用方式和优先级。
order: 30
section: rest
status: prototype
since: v0.1.0
tags: [REST, 认证, JWT, mTLS, 租户]
---

## 认证方式

| 方式 | 凭据 | 启用 | 适用 |
| --- | --- | --- | --- |
| Basic | <code>user:pass</code> | <code>-basic-auth</code> 或 <code>WithBasicAuth</code> | 开发、简单集成 |
| 静态 Bearer | 固定 Token | <code>-bearer-token</code> 或 <code>WithBearerToken</code> | 服务间调用 |
| JWT/JWKS | Bearer JWT | <code>rest.WithJWT</code> | 与现有 IdP 集成 |
| mTLS | 客户端证书 | TLS 参数 | 双向认证 |
| 身份库 | 用户/密码 | <code>WithIdentityAuth</code> | 内置身份目录 |

未配置任何认证时端点开放，只适合开发。配置认证后，<code>/service/</code> 路径要求凭据；健康检查与指标是否开放取决于部署配置。

## Basic

~~~bash
./bin/flowable-server -addr :8080 -basic-auth alice:secret
curl -u alice:secret http://localhost:8080/service/management/engine
~~~

启用身份库后，密码使用 PBKDF2 校验；否则使用静态映射。

## JWT / JWKS

<code>rest.JWTConfig</code> 支持：

- <code>JWKSURL</code>：按 <code>kid</code> 获取 RSA 公钥并缓存。
- <code>HS256Secret</code>：共享密钥。
- <code>Issuer</code> / <code>Audience</code>：校验 <code>iss</code>/<code>aud</code>。
- <code>TenantClaim</code>：从声明中提取租户。
- <code>exp</code> 校验与 RSA/HS256 算法限制。

::: warning 标准二进制未暴露 JWT 参数
JWT、RBAC、身份库认证、审计与 HTTP 指标目前通过 <code>rest.New</code> 选项接入，命令行没有对应 flag。使用这些能力需要自行嵌入服务端。
:::

## mTLS

~~~bash
./bin/flowable-server -addr :8443 \
  -tls-cert server.crt -tls-key server.key \
  -tls-client-ca client-ca.pem \
  -tls-client-auth require-verify \
  -tls-client-identity
~~~

参数约束：

- <code>-tls-cert</code> 与 <code>-tls-key</code> 必须同时提供。
- <code>-tls-client-ca</code> 可隐式启用客户端证书校验。
- <code>-tls-client-auth</code> 取值 <code>none/request/require-any/require-verify</code>。
- <code>-tls-client-identity</code> 要求 <code>require-verify</code>。
- <code>-tls-min-version</code> 支持 1.2/1.3。

证书主体映射：CN → 用户，O → 租户，O 为空回退 OU。仅在 <code>/service/</code>、无 <code>Authorization</code> 头且证书链已验证时生效。

## 优先级与不回退

1. <code>Authorization</code> 中的 Basic/Bearer 优先。
2. Basic/Bearer 失败直接 401，不回退证书身份。
3. 无 <code>Authorization</code> 时才尝试证书身份。
4. 租户来自认证绑定；请求头/参数不能越权覆盖。

## RBAC

<code>WithRBAC(userRoles, rolePermissions)</code> 按端点所需权限校验：

- <code>GET/HEAD</code> 需要 <code>read</code>。
- 写操作需要 <code>write</code>。
- <code>/service/management/</code> 与 <code>/service/identity/</code> 需要 <code>admin</code>。

证书身份暂不映射角色，因此 mTLS + RBAC 的组合需要额外接入。

## 相关阅读

- [REST 与安全进度](/docs/progress/rest-security)
- [TLS 与 mTLS 运维](/docs/operations/security)
- [REST 约定](/docs/rest/conventions)
