# 045 JWT 与 JWKS 认证

> 文档编号：002-045  
> 更新日期：2026-10-28  
> 对应计划：011 REST API 与兼容契约；008 任务、身份与表单

## 1. 背景

Flowable REST 服务此前支持 Basic Auth、静态 Bearer Token 与身份库认证，但尚未实现 OAuth2/OIDC 资源服务器能力。迁移计划中的 P2 项要求支持 JWT 校验（JWKS），以便接入企业统一身份平台。

本期在 `internal/rest` 内新增 JWT/JWKS Bearer 认证：

- 支持 RS256（从 JWKS 按 `kid` 取 RSA 公钥）；
- 支持 HS256（预共享密钥，作为内部服务或测试的兼容方式）；
- 与现有静态 Bearer、Basic Auth 并存；
- JWT 角色接入既有 `rolePermissions` RBAC；
- 校验失败统一返回 401，不泄漏内部错误细节。

## 2. 配置

`JWTConfig` 位于 `internal/rest/jwt.go`：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `JWKSURL` | `string` | JWKS 端点；非空时启用 RS256。 |
| `Issuer` | `string` | 非空时校验 `iss`，必须完全一致。 |
| `Audience` | `string` | 非空时校验 `aud`，令牌的 `aud` 可为字符串或数组。 |
| `HS256Secret` | `string` | 非空时启用 HS256；为空时拒绝 HS256 令牌。 |
| `RolesClaim` | `string` | 角色声明名，空值默认 `roles`；支持字符串数组或空格分隔字符串。 |
| `TenantClaim` | `string` | 可选租户声明名；当前只解析，不参与授权与租户隔离。 |

通过 `WithJWT` 选项启用：

```go
server := rest.New(engine,
    rest.WithJWT(rest.JWTConfig{
        JWKSURL:     "https://idp.example.com/.well-known/jwks.json",
        Issuer:      "https://idp.example.com/",
        Audience:    "flowable-rest",
        RolesClaim:  "roles",
        TenantClaim: "tenant_id",
    }),
    rest.WithRBAC(nil, map[string][]string{
        "flowable-read":  {"read"},
        "flowable-write": {"write"},
        "flowable-admin": {"admin"},
    }),
)
```

`JWKSURL` 与 `HS256Secret` 可以同时配置，实际使用哪种算法由 JWT 头部的 `alg` 决定。两者都为空时仍会启用认证，但所有 Bearer 令牌都会 fail-closed 返回 401。

## 3. 令牌校验

请求头格式：

```http
Authorization: Bearer <jwt>
```

- 认证方案大小写不敏感（`Bearer`、`bearer` 均可），令牌两侧空白会被去除。
- JWT 必须为 `header.payload.signature` 三段，使用无填充 base64url 编码。
- 只接受 `RS256` 与 `HS256`；`none`、`HS512` 等其他算法一律拒绝。
- RS256 必须提供 `kid`，并按 `kid` 从 JWKS 取公钥；`kid` 缺失或未知均拒绝。
- HS256 使用 `HS256Secret` 做 HMAC-SHA256 校验，并在比较时使用常量时间比较。
- `exp` 存在时必须晚于当前时间；`nbf` 存在时不得晚于当前时间。
- 配置了 `Issuer` / `Audience` 时必须匹配；`sub` 必须为非空字符串。
- 角色声明支持字符串数组或空格分隔字符串；其他类型视为非法令牌。
- `TenantClaim` 配置后仅解析到身份对象，当前不写请求上下文，也不做租户强制隔离。

校验失败时 `authorized` 返回 false，由统一认证中间件输出：

```json
{"message":"未认证","code":"REST-0401"}
```

不会返回签名、JWKS 地址、kid 或解析错误等细节。JWKS 网络错误、非 200、响应非法、响应超限同样按未认证处理。

## 4. JWKS 缓存与防放大

每个 `Server` 持有独立的公钥缓存：

- 成功拉取后缓存 `kid -> *rsa.PublicKey`，默认有效期 5 分钟；
- 缓存有效且命中 `kid` 时直接复用，不再访问 JWKS；
- 未命中 `kid` 时最多重新拉取一次；
- 距上次拉取不足 1 秒时不再重复拉取，避免未知 `kid` 造成请求放大；
- 拉取失败也记录时间，限制故障期间的重试频率；
- 单次 JWKS 响应限制 1 MiB，HTTP 超时 5 秒；
- 解析时忽略非 RSA、非签名用途、非 RS256、缺少 `kid` 或 `n`/`e` 非法的条目。

缓存使用互斥锁保护，JWKS 拉取期间会阻塞同一服务的并发密钥查询，避免击穿。

## 5. 权限语义

### 5.1 JWT 身份

JWT 通过后：

- `user = sub`；
- `roles = RolesClaim` 声明的角色；
- 使用既有的 `rolePermissions` 做 RBAC 映射。

端点权限规则保持不变：

- `/service/management/`、`/service/identity/` 需要 `admin`；
- `GET`、`HEAD` 需要 `read`；
- 其他方法需要 `write`；
- 角色拥有 `admin` 或对应权限即放行。

JWT 角色只查 `rolePermissions`，不再查询身份库中的用户/角色扩展，避免把外部令牌与本地用户体系隐式混合。

### 5.2 未配置 RBAC

`rolePermissions` 为空时保持既有放行语义：

- 普通 JWT 请求直接放行；
- 若同时启用 `WithIdentityAuth`，JWT 分支同样按“未配置 RBAC 即放行”处理，Basic/身份库路径行为不变。

### 5.3 静态 Bearer 与 Basic

- 静态 Bearer Token 仍按服务账号处理，通过常量时间比较后拥有全部权限（不受 RBAC 限制）。
- 静态 Bearer 优先于 JWT 判断，便于运维令牌与外部 JWT 并存。
- Basic Auth 的认证与 RBAC 逻辑完全保持不变。
- 静态 token 不匹配时继续尝试 JWT；JWT 也不通过时才返回 401。

## 6. 限制

1. 只支持 RS256 与 HS256，不支持 ES256、EdDSA、PS256 等算法。
2. JWKS 只解析 RSA 公钥，不支持 EC、OKP 等密钥类型。
3. 未实现 JWT 吊销、令牌自省（introspection）、刷新令牌与时钟偏移容忍；`exp`、`nbf` 使用精确时间比较。
4. `exp` 缺失的令牌会被接受（若 `nbf`、`iss`、`aud` 等满足）。生产环境应在签发端强制 `exp`，并配合短有效期。
5. `TenantClaim` 仅解析，不参与授权、不覆盖 `X-Tenant-Id` 或 `tenantId` 查询参数。
6. 公钥轮换依赖 5 分钟 TTL 与未知 `kid` 刷新；同 `kid` 下密钥内容变更最多可能延迟 5 分钟生效。
7. JWKS 拉取在互斥锁内执行，JWKS 故障时单次请求可能等待至 5 秒超时。
8. 未实现按 `jti`、IP、客户端证书等维度的额外校验。

## 7. 测试

新增 `internal/rest/jwt_test.go`，不依赖第三方库，测试内生成 RSA 密钥并启动 `httptest` JWKS 服务。覆盖：

- RS256 有效令牌、JWKS 缓存命中、RBAC 允许/拒绝、管理员角色；
- `exp` 过期、`nbf` 未生效、`iss`/`aud` 错误、`aud` 数组、缺失或非法声明；
- 未知 `kid`、错误签名、篡改载荷、缺失 `kid`、非法算法与非法格式；
- HS256 有效/错误密钥、自定义角色声明、空格分隔角色、租户声明；
- JWKS 网络错误、非 200、非法 JSON、超大响应、读取失败、取消上下文；
- 无令牌 401、静态 Bearer 兼容、Basic Auth 兼容、未配置 RBAC 放行。

执行：

```bash
GOCACHE=/root/gocache-sub-jwt go test ./... -count=1 -coverprofile=coverage.out
go tool cover -func=coverage.out | grep -E 'jwt.go|total:'
```

本期结果：

- `go test ./...` 通过；
- `internal/rest` 覆盖率 90.8%；
- `internal/rest/jwt.go` 语句覆盖率 100%。

## 8. 建议配置

1. 生产优先使用 RS256 + HTTPS JWKS；HS256 仅用于内部服务或测试，且密钥应定期轮换。
2. 始终配置 `Issuer` 与 `Audience`，防止跨环境令牌复用。
3. 显式配置 `WithRBAC`，不要把“未配置 `rolePermissions` 即放行”作为生产默认。
4. 签发端使用 5～15 分钟的短 `exp`（并设置 `nbf`），在网关层做撤销与限流。
5. 为 JWKS 端点配置网络白名单、超时与监控；IDP 轮换密钥时保留旧公钥一段时间。
6. 静态 `WithBearerToken` 仅用于运维兜底，并与 JWT 使用不同的权限边界。
7. 多租户场景当前应继续通过 `X-Tenant-Id` 或查询参数明确租户；待租户强制隔离实现后再启用 `TenantClaim` 联动。
