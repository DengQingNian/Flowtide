---
title: TLS 与 mTLS 运维
description: 证书准备、服务端参数、客户端认证模式、身份映射与当前安全边界。
order: 30
section: operations
status: prototype
since: v0.1.0
tags: [运维, TLS, mTLS, 证书]
---

## 证书准备

需要：

- 服务端证书与私钥（PEM）。
- 客户端 CA 证书（可含多个）。
- 客户端证书与私钥（双向认证时）。
- 证书 SAN 与访问地址匹配。

~~~bash
openssl req -x509 -newkey rsa:4096 -nodes \
  -keyout server.key -out server.crt -days 365 \
  -subj "/CN=localhost"
~~~

## 服务端参数

| 参数 | 说明 |
| --- | --- |
| <code>-tls-cert</code> | 服务端证书 |
| <code>-tls-key</code> | 服务端私钥 |
| <code>-tls-client-ca</code> | 客户端 CA；设置后启用校验 |
| <code>-tls-client-auth</code> | none/request/require-any/require-verify |
| <code>-tls-min-version</code> | 1.2 或 1.3 |
| <code>-tls-client-identity</code> | 用证书建立 REST 身份 |

推荐配置：

~~~bash
./bin/flowable-server -addr :8443 \
  -tls-cert server.crt -tls-key server.key \
  -tls-client-ca client-ca.pem \
  -tls-client-auth require-verify \
  -tls-client-identity
~~~

## 参数校验

- 证书与私钥必须同时提供。
- <code>-tls-client-ca</code> 可隐式启用客户端校验。
- <code>-tls-client-identity</code> 要求 <code>require-verify</code>。
- 非法组合在启动前失败，不会带病监听。
- 不给 <code>-tls-cert</code> 时保持明文监听。

## 身份映射

- CN → 用户。
- O → 租户；O 为空时使用 OU。
- 仅在 <code>/service/</code>、未携带 <code>Authorization</code> 头且证书链已验证时生效。
- Basic/Bearer 优先，失败不回退证书。
- 证书 CN 可以通过 userRoles 显式映射 RBAC 角色，O/OU 不授予权限。

## 客户端调用

~~~bash
curl --cacert ca.pem \
  --cert client.crt --key client.key \
  https://localhost:8443/service/management/engine
~~~

## 限制

- 无 CRL/OCSP。
- 无自动证书轮换。
- 无自动证书到角色同步；使用安全配置文件维护明确的主体映射。
- 未配置认证时端点开放，只适合开发。
- JWT/RBAC、身份库、审计与指标已通过 security-config 接入标准 CLI；没有在线配置重载。

## 相关阅读

- [认证与租户](/docs/rest/auth)
- [REST 与安全进度](/docs/progress/rest-security)
- [已知问题](/docs/compatibility/known-issues)
