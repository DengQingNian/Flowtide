# 066 官方 REST 方法与安全行为验收

> 后续行为更新：068 已实现持久异步变量；069 的原版 HTTP 验收将任务动作成功状态修正为 200 空正文。下文保留本阶段原始验收记录，当前状态以 068、069 和能力矩阵为准。

验收日期：2026-10-29。对应进度总览 M6。

## 方法级验收

`TestOfficialMethodAcceptance` 直接读取固定 OpenAPI，逐一对 184 个官方方法构造请求，检查实际 `ServeMux` 路由、未认证 401、只读用户写入 403、管理与身份域权限、管理员空输入不出现 500。每个方法使用独立引擎，接口不能用同一空响应占位替代正常处理器。

验收发现并修复 8 个 `POST /query/*` 被误判为写权限的问题。查询使用 POST 传输过滤条件，要求 `read` 权限；管理及身份域继续要求 `admin`。

请求体现在要求恰好一个 JSON 值。拼接 JSON、尾部垃圾、`null`、超限及畸形输入返回 400；查询保持空体兼容。创建任务仍允许未知字段，其他原有严格接口保持字段白名单。

## Java 样本 HTTP 回放

`TestJavaReferenceHTTPPlayback` 消费固定 Java 提交的 JDBC 黄金行及公共服务结果，经正式 `migration.Convert` 和引擎状态恢复后，通过真实 HTTP 处理链验证：

- approve 与 review 两个等待点的任务字段、租户、分页数量；
- 以原处理人执行 HTTP complete，返回 204；
- 完成后的当前任务集合、历史任务处理人及结束状态；
- 历史实例业务键、租户、结束时间及历史变量数量；
- 已完成的 Java 存量历史仅恢复后仍可查询。

这属于 Java 公共服务结果到 Go HTTP 行为的业务对照，不是全量 Java REST 原始响应逐字段回放；不把两者混称。六个 `variables-async` 继续列为同步替代差异。

## 标准服务安全配置

```bash
./bin/flowable-server -security-config /受限目录/security.json
```

配置示例（示例密码仅供本地验收）：

```json
{
  "basicUsers": {"alice": "本地验收密码"},
  "userRoles": {"alice": ["reader"]},
  "rolePermissions": {"reader": ["read"]},
  "tenantBindings": {"alice": "tenant-a"},
  "auditCapacity": 1000,
  "metrics": true
}
```

其他字段为 `bearerToken`、`identityAuth`、`jwt` 和 `tenantAdministrators`。`jwt` 支持 `jwksUrl`、`issuer`、`audience`、`hs256Secret`、`rolesClaim`、`tenantClaim`。配置启动前校验，错误不输出内容；建议文件权限 0600，禁止提交实际密钥。原命令行 Basic/Bearer 参数在配置文件后生效。

mTLS 证书经 TLS 验证后，CN 可使用 `userRoles` 显式映射角色。O/OU 仅决定租户，不授予权限；没有角色映射的证书在启用 RBAC 后仍被拒绝。CRL/OCSP 不在本轮范围。

## 可重复执行

```bash
go test ./internal/rest -run 'TestOfficialMethodAcceptance|TestJSONRequestBoundary|TestJavaReferenceHTTPPlayback|TestClientCertificateIdentityRBAC' -count=1
go test ./cmd/flowable-server -run TestSecurityConfig -count=1
make check
```
