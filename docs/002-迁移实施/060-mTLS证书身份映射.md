# 060 mTLS 客户端证书身份映射

> 文档编号：002-060
> 更新日期：2026-10-28
> 对应计划：011 REST API 与兼容契约
> 前置文档：[057 双向 TLS 认证](057-双向TLS认证.md)

## 1. 背景与目标

057 已经在 `cmd/flowable-server` 实现双向 TLS：通过 `-tls-client-ca` 与 `-tls-client-auth` 让服务端在握手阶段要求并校验客户端证书。但证书主体没有进入 `internal/rest` 的认证与租户上下文，业务层仍只认 Basic、静态 Bearer 与 JWT，纯 mTLS 客户端只能以匿名方式访问"未配置认证即放行"的端点。

本期在 `internal/rest` 增加客户端证书到身份/租户的映射能力：

- 新增选项 `WithClientCertificateIdentity()`；
- 仅在 `/service/` 路径、请求未携带 `Authorization` 头、TLS 已完成且证书链已验证时生效；
- 身份主体取证书 `Subject.CommonName`，租户取 `Subject.Organization` 第一项或 `Subject.OrganizationalUnit` 第一项；
- 不映射角色，不把证书中的任意字段直接当作角色；
- 不改变 Basic / Bearer / JWT 的原有优先级，HTTP 凭据失败时绝不回退到证书。

本期同时由同一提交的 `cmd/flowable-server` 完成主程序接线（见 2.2），`internal/rest` 仍是证书身份的唯一解析与授权入口。

## 2. 启用方式

### 2.1 REST 选项

```go
handler := rest.New(app,
    rest.WithClientCertificateIdentity(),
)
```

该选项只打开"允许用客户端证书认证"的开关，不会自动开启 TLS，也不会修改 `tls.Config`。TLS 层的 `ClientAuth`、`ClientCAs` 仍由服务端启动参数决定。

### 2.2 主程序接线

`cmd/flowable-server` 新增布尔参数 `-tls-client-identity`，语义为"使用已校验的客户端证书建立 REST 身份"。主程序在启动时校验以下前提，任一不满足即以退出码 1 拒绝启动：

- 必须同时配置 `-tls-cert` 与 `-tls-key`（启用本地 TLS）；
- 必须配置 `-tls-client-ca`；
- `-tls-client-auth` 必须为 `require-verify`；
- 通过 `buildTLSConfig` 的参数、CA 与最低版本校验。

满足前提后，主程序把 `rest.WithClientCertificateIdentity()` 加入 REST 选项。启动示例：

```bash
HOME=/root go run ./cmd/flowable-server \
  -addr :8443 \
  -tls-cert server.crt \
  -tls-key server.key \
  -tls-client-ca client-ca.pem \
  -tls-client-auth require-verify \
  -tls-client-identity
```

主程序强制 `require-verify`：只有该模式会生成 `VerifiedChains`，证书主体才可被映射为身份；`request` 或 `require-any` 下证书链未验证，启动阶段直接拒绝。

### 2.3 仅打开选项但不启用 TLS 的结果

若直接调用 `WithClientCertificateIdentity()` 而不启用 TLS（例如 TLS 在反向代理终止，且代理没有把已验证证书通过别的机制传给后端），`r.TLS` 为空，所有 `/service/` 请求都会返回 401。这是有意的失败关闭（fail closed）行为；`cmd/flowable-server` 已通过启动校验避免这种误配。

## 3. 证书主体语义

| 身份字段 | 来源 | 规则 |
| --- | --- | --- |
| Subject | `Subject.CommonName` | 去掉两侧空白后必须非空，否则认证失败（401）。 |
| Tenant | `Subject.Organization[0]`，为空时取 `Subject.OrganizationalUnit[0]` | 逐项去空白；O 第一项为空白视为未提供并回退 OU；O 与 OU 均为空时租户为空字符串。 |
| Roles | 无 | 证书身份恒为空角色集合，不读取 CN/O/OU/SAN 作为角色。 |

示例：

```
CN=alice, O=tenant-a, OU=team-a  ->  Subject=alice, Tenant=tenant-a
CN=bob,   O=(空),    OU=tenant-ou ->  Subject=bob,   Tenant=tenant-ou
CN=carol                         ->  Subject=carol, Tenant=""
CN=(空),  O=tenant-a             ->  认证失败（401）
```

只接受 `r.TLS.VerifiedChains` 中已验证链的叶子证书，不直接信任 `PeerCertificates`。因此即使 TLS 模式为 `request` / `require-any`（不校验证书链），或者反向代理只做了"请求证书"而未验证，自签或未受信证书也不会被当作身份，应用层返回 401。

## 4. 认证优先级与不回退规则

开启选项后，`/service/` 路径的认证顺序保持不变：

1. 静态 Bearer Token（`WithBearerToken`）；
2. JWT Bearer（`WithJWT`，RS256/JWKS 或 HS256）；
3. Basic Auth（静态用户或 `WithIdentityAuth` 身份库）；
4. 仅当以上 HTTP 凭据完全不存在（`Authorization` 头为空）时，才尝试客户端证书。

关键约束：

- 只要请求带有 `Authorization` 头（无论 Basic 还是 Bearer），就不会使用证书认证；
- Basic 密码错误、Bearer 令牌错误或 JWT 校验失败时直接返回 401，不会回退到证书，避免"凭据失败但证书有效"绕过密码/令牌校验；
- 证书认证失败（无证书、未验证链、CN 为空）且无其他凭据时返回 401；
- 非 `/service/` 路径（如 `/healthz`、`/metrics`、`/readyz`）不受该选项影响，仍保持公开。

注意：当 TLS 使用 `require-verify`（`tls.RequireAndVerifyClientCert`）时，未提供客户端证书会在握手阶段被拒绝，HTTP 层不会产生 401；应用层的 401 出现在"TLS 未要求证书"或"TLS 由反向代理终止"的场景。两种情况下都不会建立身份。

## 5. 租户隔离

证书租户写入 `jwtIdentity.Tenant`，后续中间件与现有逻辑完全一致：

- 请求未显式指定租户（无 `X-Tenant-Id`、无 `tenantId` 查询参数）时，使用证书租户；
- 显式选择的租户与证书租户不同时返回 403（`REST-0403`）；
- 证书租户为空时，客户端显式选择任意非空租户都会 403；
- `WithTenantAdministrators` 仍按身份 `Subject`（即证书 CN）判断是否允许跨租户，行为与其他身份一致。

租户隔离只保证"客户端不能选择与证书主体不同的租户"，不校验租户在引擎中是否存在，也不改变各端点原有的租户过滤逻辑。

## 6. RBAC 未映射

证书身份的角色集合恒为空，因此：

- 未配置 `rolePermissions` 时，证书身份放行（即使同时启用了 `WithIdentityAuth`）；
- 配置了 `rolePermissions` 时，证书身份没有任何角色可映射，访问 `/service/` 端点返回 403；
- 即使证书 CN 与某个 Basic/身份库用户名相同，也不会套用该用户的角色，避免"证书字段隐式提权"。

当前不支持"证书主体到角色"的映射。若生产环境需要 RBAC，推荐让网关同时注入 JWT（由 JWT 角色授权），或后续单独设计证书属性到角色的显式映射配置。

## 7. 实现说明

新增 `internal/rest/client_cert_identity.go`：

| 函数 | 职责 |
| --- | --- |
| `WithClientCertificateIdentity` | 返回打开 `Server.clientCertIdentity` 的选项。 |
| `clientCertificateIdentity` | 校验选项、路径、TLS 状态与 `VerifiedChains`，从叶子证书构造 `jwtIdentity`；CN 为空时失败。 |
| `clientCertificateTenant` | 按 O 优先、OU 兜底的顺序推导租户，逐项去空白。 |

对既有文件的改动：

- `internal/rest/server.go`：`Server` 新增 `clientCertIdentity` 字段；`authenticate` 在该选项开启时启用认证中间件；`permitted` 增加"证书身份无角色"分支（未配置 `rolePermissions` 放行，否则拒绝）。
- `internal/rest/identity_context.go`：`authenticateIdentity` 在静态 Bearer、JWT、Basic 均未通过且 `Authorization` 头为空时，最后尝试证书认证。

证书解析逻辑没有放入 `jwt.go`，两者互不依赖。

## 8. 验证命令与结果

所有命令在仓库根目录执行，覆盖率文件写入 `/tmp`：

```bash
HOME=/root go test ./internal/rest -count=1
# ok github.com/flowable-go/flowable/internal/rest

HOME=/root go test ./internal/rest -count=1 -coverprofile=/tmp/rest_mtls_coverage.out
# ok github.com/flowable-go/flowable/internal/rest coverage: 90.6% of statements

HOME=/root go tool cover -func=/tmp/rest_mtls_coverage.out | grep client_cert
# WithClientCertificateIdentity  100.0%
# clientCertificateIdentity      100.0%
# clientCertificateTenant        100.0%
```

新增测试 `internal/rest/client_cert_identity_test.go`，使用 `crypto/x509` 现场生成 CA、含 `127.0.0.1` SAN 的服务端证书与客户端证书，基于 `httptest.NewUnstartedServer` + `TLS.ClientAuth=RequireAndVerifyClientCert` 启动真实 TLS，不访问外网。覆盖：

| 场景 | 期望 |
| --- | --- |
| 开启选项但请求无 HTTP 凭据、无 TLS | 401；`/healthz` 仍 200 |
| `RequireAndVerifyClientCert` 且无客户端证书 | TLS 握手失败，请求不进入 HTTP 层 |
| TLS 允许无证书（`RequestClientCert`） | 401 |
| 受信证书（CN=alice，O=tenant-a） | 200 |
| 受信证书 + `X-Tenant-Id: tenant-a` | 200 |
| 受信证书 + 其他租户 ID（含 OU） | 403 |
| 仅 OU 派生租户 | 同租户 200，其他租户 403 |
| 无 O/OU（空租户） + 显式租户 | 403 |
| 证书 CN 为空 | 401 |
| 自签/未受信证书 | `require-verify` 下 TLS 拒绝；`RequestClientCert` 下 401 |
| 有效 Basic + 证书 | 按 Basic 身份与租户绑定 |
| 非法 Basic + 有效证书 | 401（不回退） |
| 有效/非法 Bearer、JWT + 证书 | 按 Bearer/JWT 结果，非法时 401（不回退） |
| 配置 `rolePermissions` + 证书身份 | 403 |
| 未配置 `rolePermissions`（含启用身份库） + 证书身份 | 放行 |

## 9. 限制与后续工作

1. **CRL/OCSP 未实现**：只依赖 TLS 握手期的证书链、有效期与用途校验；证书吊销不检查，已吊销证书在有效期内仍可认证。
2. **RBAC 未映射**：证书身份没有角色，配置 `rolePermissions` 后只能返回 403，需要 JWT 或后续显式映射方案补充授权。
3. **仅 CN/O/OU**：不读取 SAN、Email、序列号等字段，也不支持多租户或前缀/正则匹配。
4. **仅 `/service/` 路径**：其他路径不解析证书身份；这是与现有认证中间件作用域保持一致。
5. **不校验租户存在性**：证书租户只用于与客户端选择的值比对和传递到引擎，不查询租户表。
6. **依赖 TLS 层验证结果**：若前置代理终止 TLS 且未把客户端证书转发给后端，后端 `r.TLS` 为空，选项不会生效（返回 401）。
7. **未实现证书热加载/轮换**：沿用 057 的限制，CA 与证书更新需要重启。

## 10. 变更文件

| 文件 | 变更 |
| --- | --- |
| `internal/rest/client_cert_identity.go` | 新增选项、证书身份解析与租户推导函数。 |
| `internal/rest/client_cert_identity_test.go` | 新增真实 TLS 集成测试与边界单元测试。 |
| `internal/rest/server.go` | 新增字段；认证中间件启用条件；`permitted` 的证书身份授权分支。 |
| `internal/rest/identity_context.go` | 认证顺序末尾增加证书回退（仅无 `Authorization` 时）。 |
| `cmd/flowable-server/main.go` | 新增 `-tls-client-identity` 参数、启动校验与 REST 选项接线。 |
| `cmd/flowable-server/tls.go` / `tls_test.go` | 新增 `validateClientCertIdentity` 及错误分支测试。 |
| `docs/002-迁移实施/060-mTLS证书身份映射.md` | 本文档。 |
