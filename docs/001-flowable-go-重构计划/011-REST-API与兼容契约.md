# 011 REST API 与兼容契约

> 文档编号：011  
> 依赖：003 代码结构、008 任务、010 历史  
> 目标读者：后端开发、前端、集成方、QA

## 1. 兼容目标与基线

Flowable 8.x 提供官方 Swagger/OpenAPI 规范，作为 REST 契约的事实标准：

| 规范 | 端点路径数 | 方法数（约） | 本期目标 |
| --- | --- | --- | --- |
| 流程（process） | 119 | 184 | 100% |
| CMMN | 73 | — | T2 |
| DMN | 13 | — | T2 |
| 应用（app） | 10 | — | T3 |
| 事件注册（eventregistry） | 15 | — | T2 |

本文聚焦**流程 REST**。兼容性分为：

- **路径兼容**：URL、路径参数、查询参数一致。
- **结构兼容**：请求/响应 JSON 字段名、类型、嵌套一致。
- **语义兼容**：分页、排序、错误码、变量表达、时间格式一致。
- **行为兼容**：相同请求产生相同引擎状态与副作用。

## 2. 基础约定

### 2.1 基址

- 默认基址：`/service`（与 Flowable REST 一致），可配置。
- 健康检查：`/healthz`、`/readyz`（不在 `/service` 下）。
- 指标：`/metrics`（Prometheus）。
- OpenAPI：`/service/openapi.json`（扩展，便于调试）。

### 2.2 方法与内容类型

| 场景 | Content-Type | 说明 |
| --- | --- | --- |
| JSON 请求/响应 | `application/json` | UTF-8 |
| 部署上传 | `multipart/form-data` | 文件字段 `file`，可多个 |
| 二进制下载 | `application/octet-stream` | 资源/附件/图片 |
| 图片 | `image/png`、`image/svg+xml` | 流程图 |
| 表单提交 | `application/x-www-form-urlencoded` 或 JSON | 兼容旧客户端 |

### 2.3 认证

| 方式 | 说明 | 阶段 |
| --- | --- | --- |
| HTTP Basic | 用户名/密码，Base64 | P0 |
| Bearer Token | JWT 或 IDM Token | P1 |
| OAuth2 Resource Server | JWT 校验（JWKS） | P2 |
| mTLS | 网关层 | P3 |

- 未认证返回 `401` + `WWW-Authenticate: Basic realm="flowable"`。
- 无权限返回 `403`。
- 认证上下文注入 `authenticatedUserId`，引擎不得信任请求体中的用户字段。
- 管理端点要求管理员角色。

### 2.4 租户

- 请求参数 `tenantId` 优先；未提供时使用认证用户的租户；仍为空则默认租户。
- 扩展响应头 `X-Tenant-Id` 回显实际租户。
- 租户不匹配返回 `404`（不泄露其他租户资源存在性）。

### 2.5 幂等

- 可选 `Idempotency-Key` 请求头，用于启动流程、创建任务等 POST。
- 服务端缓存 24 小时的 key → 响应；重复请求返回首次结果。
- 未提供 key 时按普通请求处理；重试由客户端负责。

## 3. 端点清单

### 3.1 仓库（Repository）

| 方法 | 路径 | 作用 |
| --- | --- | --- |
| GET | `/repository/deployments` | 部署列表 |
| POST | `/repository/deployments` | 上传部署（multipart） |
| GET | `/repository/deployments/{id}` | 部署详情 |
| DELETE | `/repository/deployments/{id}` | 删除部署（可级联） |
| GET | `/repository/deployments/{id}/resources` | 资源列表 |
| GET | `/repository/deployments/{id}/resources/{name}` | 资源内容 |
| GET | `/repository/process-definitions` | 流程定义列表 |
| GET | `/repository/process-definitions/{id}` | 流程定义详情 |
| PUT | `/repository/process-definitions/{id}` | 更新分类/挂起 |
| GET | `/repository/process-definitions/{id}/xml` | BPMN XML |
| GET | `/repository/process-definitions/{id}/diagram` | 流程图 |
| GET | `/repository/process-definitions/{id}/start-form` | 启动表单 |
| GET | `/repository/process-definitions/{id}/identitylinks` | 候选发起人 |
| GET | `/repository/models` | 模型列表 |
| POST | `/repository/models` | 创建模型 |
| GET | `/repository/models/{id}/source` | 模型源文件 |
| PUT | `/repository/models/{id}/source` | 保存模型源文件 |
| GET | `/repository/process-definitions/{id}/model` | 定义对应模型 |

> 仓库组共 22 个资源类、约 33 个方法；上表列出核心路径，完整清单以官方 OpenAPI 为准。

### 3.2 运行时（Runtime）

| 方法 | 路径 | 作用 |
| --- | --- | --- |
| POST | `/runtime/process-instances` | 启动流程实例 |
| GET | `/runtime/process-instances` | 实例列表 |
| GET | `/runtime/process-instances/{id}` | 实例详情 |
| DELETE | `/runtime/process-instances/{id}` | 删除实例 |
| PUT | `/runtime/process-instances/{id}` | 更新名称/业务键 |
| POST | `/runtime/process-instances/{id}/variables` | 创建变量 |
| PUT | `/runtime/process-instances/{id}/variables/{name}` | 更新变量 |
| DELETE | `/runtime/process-instances/{id}/variables/{name}` | 删除变量 |
| GET | `/runtime/process-instances/{id}/diagram` | 运行态流程图 |
| GET | `/runtime/executions` | 执行列表 |
| GET | `/runtime/executions/{id}` | 执行详情 |
| PUT | `/runtime/executions/{id}` | 执行动作（signal 等） |
| GET | `/runtime/executions/{id}/variables` | 执行变量 |
| GET | `/runtime/tasks` | 任务列表 |
| POST | `/runtime/tasks` | 创建任务 |
| GET | `/runtime/tasks/{id}` | 任务详情 |
| PUT | `/runtime/tasks/{id}` | 更新任务属性 |
| POST | `/runtime/tasks/{id}` | 任务动作（claim/complete/delegate/resolve） |
| DELETE | `/runtime/tasks/{id}` | 删除任务 |
| GET | `/runtime/tasks/{id}/variables` | 任务变量 |
| POST | `/runtime/tasks/{id}/variables` | 创建任务变量 |
| GET | `/runtime/tasks/{id}/identitylinks` | 身份链接 |
| POST | `/runtime/tasks/{id}/identitylinks` | 添加候选人 |
| GET | `/runtime/tasks/{id}/comments` | 评论列表 |
| POST | `/runtime/tasks/{id}/comments` | 添加评论 |
| GET | `/runtime/tasks/{id}/attachments` | 附件列表 |
| POST | `/runtime/tasks/{id}/attachments` | 添加附件 |
| GET | `/runtime/tasks/{id}/subtasks` | 子任务 |
| GET | `/runtime/tasks/{id}/form` | 任务表单 |
| POST | `/runtime/tasks/{id}/form` | 提交任务表单 |
| POST | `/runtime/signals` | 广播信号 |
| GET | `/runtime/event-subscriptions` | 事件订阅查询 |
| GET | `/runtime/activity-instances` | 活动实例查询 |

> 运行时分 49 个资源类、约 80 个方法；任务与变量是重点兼容区域。

### 3.3 历史（History）

| 方法 | 路径 | 作用 |
| --- | --- | --- |
| GET | `/history/historic-process-instances` | 历史实例列表 |
| GET | `/history/historic-process-instances/{id}` | 历史实例详情 |
| DELETE | `/history/historic-process-instances/{id}` | 删除历史实例 |
| GET | `/history/historic-process-instances/{id}/variables` | 历史变量 |
| GET | `/history/historic-process-instances/{id}/identitylinks` | 历史身份链接 |
| GET | `/history/historic-process-instances/{id}/comments` | 历史评论 |
| GET | `/history/historic-activity-instances` | 历史活动 |
| GET | `/history/historic-task-instances` | 历史任务 |
| GET | `/history/historic-task-instances/{id}` | 历史任务详情 |
| GET | `/history/historic-task-instances/{id}/form` | 历史任务表单 |
| GET | `/history/historic-variable-instances` | 历史变量 |
| GET | `/history/historic-detail` | 历史明细 |
| GET | `/history/historic-task-log-entries` | 任务日志 |
| GET | `/history/historic-detail/{id}/data` | 明细二进制 |
| GET | `/history/historic-variable-instances/{id}/data` | 变量二进制 |

> 历史组 27 个资源类；查询参数组合最多，需要完整实现过滤与排序。

### 3.4 管理（Management）

| 方法 | 路径 | 作用 |
| --- | --- | --- |
| GET | `/management/engine` | 引擎信息（版本、数据库） |
| GET | `/management/properties` | 引擎属性 |
| GET | `/management/tables` | 表列表 |
| GET | `/management/tables/{name}` | 表结构 |
| GET | `/management/tables/{name}/data` | 表数据 |
| GET | `/management/jobs` | 异步作业 |
| POST | `/management/jobs/{id}` | 执行/移动/改重试 |
| DELETE | `/management/jobs/{id}` | 删除作业 |
| GET | `/management/jobs/{id}/exception-stacktrace` | 异常栈 |
| GET | `/management/timer-jobs` | 定时器作业 |
| GET | `/management/suspended-jobs` | 挂起作业 |
| GET | `/management/deadletter-jobs` | 死信作业 |
| GET | `/management/history-jobs` | 历史作业 |
| GET | `/management/batches` | 批处理列表 |
| GET | `/management/batches/{id}` | 批处理详情 |
| GET | `/management/batch-parts` | 分片列表 |

> 管理组 21 个资源类；表数据端点仅管理员可用，默认脱敏大字段。

### 3.5 身份与表单

| 方法 | 路径 | 作用 |
| --- | --- | --- |
| GET/POST | `/identity/users` | 用户列表/创建 |
| GET/PUT/DELETE | `/identity/users/{id}` | 用户详情/更新/删除 |
| GET/POST | `/identity/users/{id}/info` | 用户信息 |
| GET/PUT/DELETE | `/identity/users/{id}/info/{key}` | 单条信息 |
| GET | `/identity/users/{id}/picture` | 头像 |
| GET/POST | `/identity/groups` | 组列表/创建 |
| GET/PUT/DELETE | `/identity/groups/{id}` | 组详情/更新/删除 |
| POST | `/identity/groups/{id}/members` | 添加成员 |
| DELETE | `/identity/groups/{id}/members/{userId}` | 移除成员 |
| GET | `/form/form-data` | 获取表单数据 |
| POST | `/form/form-data` | 提交表单 |

## 4. DTO 规范

### 4.1 分页

请求：

| 参数 | 类型 | 默认 | 说明 |
| --- | --- | --- | --- |
| `start` | int | 0 | 起始偏移 |
| `size` | int | 10 | 每页数量，最大 1000 |
| `sort` | string | — | 排序字段，白名单 |
| `order` | string | `asc` | `asc`/`desc` |

响应：

~~~json
{
  "data": [],
  "total": 0,
  "start": 0,
  "size": 10,
  "sort": "createTime",
  "order": "desc"
}
~~~

### 4.2 变量

~~~json
{
  "name": "amount",
  "type": "long",
  "value": 100,
  "scope": "global"
}
~~~

| 字段 | 说明 |
| --- | --- |
| `name` | 变量名 |
| `type` | string/long/double/boolean/date/json/bytes/null 等 |
| `value` | 值；二进制用 `valueUrl` |
| `valueUrl` | 二进制下载地址 |
| `scope` | `global`/`local`；写入缺省 global |
| `valueUnresolvable` | 值不可读取时为 true |

规则：

- 写入时允许省略 `type`，由服务端推断。
- `date` 使用 ISO-8601，可带时区。
- `json` 的 `value` 为 JSON 对象/数组，不是字符串。
- `bytes` 读取返回 `valueUrl`，不内联 Base64。
- 变量名大小写敏感。

### 4.3 时间

- 输出：ISO-8601 带偏移，毫秒精度，例如 `2026-10-28T01:20:32.123+08:00`。
- 输入：接受带偏移或 UTC 的 ISO-8601；无时区按 UTC 解释。
- 数据库统一 UTC；REST 按请求时区（默认引擎时区）输出。

### 4.4 错误

~~~json
{
  "message": "Bad request",
  "exception": "Process definition with key 'x' not found",
  "code": "ENGINE-0404",
  "requestId": "01J..."
}
~~~

- 兼容字段：`message`、`exception`。
- 扩展字段：`code`、`requestId`、`details`（校验错误数组）。
- 生产环境可关闭 `exception` 详情，仅返回 `requestId`。

### 4.5 二进制

- 响应头 `Content-Disposition` 指定文件名。
- 大文件支持 `Range`（附件/资源）。
- 上传限制默认 50MB，可配置。

## 5. 查询参数

| 类别 | 示例 | 说明 |
| --- | --- | --- |
| 精确匹配 | `processDefinitionKey=order` | 等值 |
| 模糊匹配 | `nameLike=%25order%25` | URL 编码 `%` |
| 大小写不敏感 | `nameLikeIgnoreCase=order` | 显式参数 |
| 时间范围 | `startedAfter=...&startedBefore=...` | ISO-8601 |
| 布尔 | `finished=true` | true/false |
| 排序 | `sort=createTime&order=desc` | 白名单 |
| 包含 | `includeProcessVariables=true` | 关联加载 |

规则：

- 未知参数忽略还是报错？为兼容，**忽略未知参数**，但记录 Debug 日志。
- 非法枚举/时间格式返回 400。
- `%' 通配符需 URL 编码为 `%25'。
- 所有查询自动附加租户过滤。

## 6. 写操作与状态码

| 操作 | 成功码 | 说明 |
| --- | --- | --- |
| GET | 200 | 查询 |
| POST 创建 | 201 | 返回 `Location` |
| POST 动作 | 200/204 | claim/complete 等 |
| PUT | 200 | 更新后返回资源 |
| DELETE | 204 | 无 body |
| 参数错误 | 400 | 含字段错误 |
| 未认证 | 401 | `WWW-Authenticate` |
| 无权限 | 403 | — |
| 未找到 | 404 | — |
| 冲突 | 409 | 乐观锁/重复签收 |
| 限流 | 429 | `Retry-After` |
| 服务不可用 | 503 | 数据库/作业执行器未就绪 |

任务动作 body：

~~~json
{
  "action": "complete",
  "variables": [
    {"name": "approved", "type": "boolean", "value": true}
  ]
}
~~~

支持动作：`claim`、`unclaim`、`complete`、`delegate`、`resolve`。

## 7. 实现架构

~~~mermaid
flowchart LR
    R[chi Router] --> M1[RequestID]
    M1 --> M2[Recover]
    M2 --> M3[Logging]
    M3 --> M4[Auth]
    M4 --> M5[Tenant]
    M5 --> M6[RateLimit]
    M6 --> H[Handler]
    H --> S[Engine Service]
    H --> V[DTO 校验]
    S --> C[Command]
~~~

- 路由按资源分组，每组一个 `Mount`。
- Handler 只做：解析参数 → 校验 → 调用服务 → 转换 DTO。
- 业务逻辑不得写在 Handler；错误统一由 `ErrorMapper` 转换。
- 请求上下文传递 `requestId`、`tenant`、`user`。
- 超时：读 30s、写 60s；长操作走异步作业。

## 8. 代码生成与契约测试

### 8.1 代码生成

- 源规范：Flowable 官方 Swagger 2.0。
- 构建期用 `swagger2openapi` 转为 OpenAPI 3，再用 `oapi-codegen` 生成：
  - 请求/响应 DTO（仅类型）
  - 路径常量
  - 参数校验桩
- Handler 手写，保留 Flowable 的语义与错误行为。
- 生成代码不纳入覆盖率统计。

### 8.2 契约测试

| 测试 | 方法 |
| --- | --- |
| 路径覆盖 | 解析 OpenAPI，断言每个方法都有 Handler |
| 响应结构 | 用 `kin-openapi` 校验响应 JSON |
| 请求校验 | 非法 body/参数返回 400 |
| 兼容回放 | 录制 Java 版请求/响应，Go 版回放对比（忽略时间/ID） |
| 错误码 | 逐条映射表测试 |
| 分页/排序 | 边界值、空结果、最大 size |
| 二进制 | Content-Type/Disposition/Range |

### 8.3 兼容回放

~~~mermaid
sequenceDiagram
    participant J as Java 引擎
    participant R as 录制器
    participant G as Go 引擎
    participant D as 差异报告
    J->>R: 请求 + 响应 + DB 快照
    R->>G: 回放请求
    G->>D: 响应 + DB 快照
    R->>D: 对比（忽略 ID/时间）
~~~

- 忽略字段：ID、时间戳、持续时间、异常栈地址。
- 允许差异：字段顺序、空白、可选字段缺失；必须记录白名单。
- 差异报告纳入 CI 制品，逐条收敛。

## 9. 分阶段交付

| 阶段 | 端点范围 | 目标 |
| --- | --- | --- |
| M8.1 | 部署、流程定义、实例启动/查询/删除 | 可跑通 Demo |
| M8.2 | 任务查询/动作/变量/身份链接 | 待办闭环 |
| M8.3 | 执行、活动实例、信号、事件订阅 | 运行时完整 |
| M8.4 | 历史查询、管理作业/表、批次 | 运维可用 |
| M8.5 | 身份、表单、模型、图片 | 全量 119 |

## 10. 性能与限流

| 指标 | 目标 |
| --- | --- |
| 列表查询 p95 | ≤ 50ms（10 万行） |
| 详情查询 p95 | ≤ 20ms |
| 启动实例 p95 | ≤ 80ms |
| 完成任务 p95 | ≤ 40ms |
| 错误率 | < 0.1% |

- 单 IP/租户令牌桶限流，默认 100 QPS，可配置。
- 大查询强制分页；`size > 1000` 返回 400。
- 慢请求（> 1s）记录日志并采样追踪。
- 连接池与数据库超时联动，避免请求堆积。

## 11. 安全

- 所有输入做类型/长度/枚举校验；SQL 参数化。
- 部署上传校验文件类型、大小、Zip 结构。
- 禁止通过 `sort` 注入列名。
- 管理端点二次鉴权。
- CORS 白名单，默认关闭。
- 响应头：`X-Content-Type-Options: nosniff`、`X-Frame-Options: DENY`。
- 不在错误响应中泄露 SQL、堆栈（生产）。

## 12. 测试要点

- 每个端点：成功、缺参、非法参、未找到、无权限、冲突。
- 分页：start/size 边界、排序稳定性、总数正确。
- 变量：各类型读写、二进制、作用域、批量。
- 任务动作：状态机非法转移返回 400/409。
- 部署：多文件、ZIP、重复部署、失败回滚。
- 历史：过滤组合、时间范围、includeVariables。
- 契约：OpenAPI 全路径覆盖、响应 schema 校验。
- 兼容：Java 回放差异在阈值内。
- 性能：基准与限流。

## 13. 验收清单

- [ ] 流程 REST 119 个路径全部实现，方法覆盖 100%。
- [ ] 响应字段与官方 OpenAPI 一致，契约测试通过。
- [ ] 分页/排序/错误码/变量格式兼容。
- [ ] 认证、租户、权限在全部端点生效。
- [ ] 二进制端点支持 Content-Disposition 与 Range。
- [ ] Manage 端点仅管理员可用。
- [ ] 与 Java 版回放差异有白名单且逐条收敛。
- [ ] 性能指标达标，限流生效。
