# 055 REST 官方缺失端点补齐

> 文档编号：002-055
> 更新日期：2026-10-28
> 对应计划：011 REST API 与兼容契约

## 1. 背景

docs/003-迁移审计/004-REST端点对照.md 曾列出 12 个状态为“缺失；迁移验收阻断”的官方端点。本期按官方 Swagger 与 Flowable 7 REST 资源类补齐路由与行为，并同步修正部署资源响应模式、作业桶语义与契约生成器的路径规范化。

生成结果：官方方法端点 184，实现方法端点 242；官方缺失数为 **0**。

## 2. 端点与语义

| # | 方法与路径 | 成功 | 主要错误 | 实现 |
| --- | --- | --- | --- | --- |
| 1 | DELETE /service/management/batches/{batchId} | 204 | 不存在/跨租户 404 | handleDeleteBatch + Engine.DeleteBatch |
| 2 | DELETE /service/management/deadletter-jobs/{jobId} | 204 | 非死信或不存在 404 | handleDeleteDeadLetterJob + Engine.DeleteDeadLetterJob |
| 3 | DELETE /service/management/suspended-jobs/{jobId} | 204 | 非挂起或不存在 404 | handleDeleteSuspendedJob + Engine.DeleteSuspendedJob |
| 4 | DELETE /service/management/timer-jobs/{jobId} | 204 | 非定时器或不存在 404 | handleDeleteTimerJob + Engine.DeleteTimerJob |
| 5 | GET /service/runtime/process-instances/{id}/identitylinks/users/{identityId}/{type} | 200 RestIdentityLink | 实例或链接不存在 404 | handleGetInstanceUserIdentityLink |
| 6 | GET /service/management/batch-parts/{id}/batch-part-document | 200 application/json | 分片不存在或无文档 404 | handleGetBatchPartDocument + Engine.GetBatchPartDocument |
| 7 | GET /service/management/batches/{id}/batch-document | 200 application/json | 批次不存在或无文档 404 | handleGetBatchDocument + Engine.GetBatchDocument |
| 8 | GET /service/repository/deployments/{id}/resourcedata/{resourceName} | 200 原始字节 | 部署或资源不存在 404 | handleGetDeploymentResourceData |
| 9 | GET /service/repository/deployments/{id}/resources/** | 200 DeploymentResourceResponse | 部署或资源不存在 404 | handleGetDeploymentResourceMetadata |
| 10 | DELETE /service/runtime/process-instances/{id}/identitylinks/users/{identityId}/{type} | 204 | 实例或链接不存在 404 | handleDeleteInstanceUserIdentityLink |
| 11 | POST /service/management/deadletter-jobs/{jobId} | 204 | 不存在 404；不支持动作 400 | handleDeadLetterJobAction |
| 12 | POST /service/management/timer-jobs/{jobId} | 204 | 不存在 404；不支持动作或非法日期 400 | handleTimerJobAction |

### 2.1 作业桶

jobState 新增 Executable 标记，api.JobInfo 同步暴露可选 executable 字段。

- 可执行桶：Jobs 中非定时器、非挂起作业。
- 定时器桶：Jobs 中类型含 timer 且未移动、未挂起。
- 挂起桶：Jobs 中所属流程实例处于挂起状态。
- 死信桶：DeadLetters。

四个删除端点严格校验桶，跨桶一律 404。DELETE /service/management/jobs/{id}、GET /service/management/jobs 与 GET /service/management/jobs/{id} 也收紧为可执行桶，避免把定时器/挂起作业误判为可执行作业。

### 2.2 作业动作

- 定时器 move：标记 Executable=true，到期时间置为当前时间，保留原处理器类型。
- 定时器 reschedule：接受 dueDate（RFC3339）；为兼容早期调用同时接受 date 别名。缺少到期时间或格式非法返回 400。
- 死信 move：移入可执行桶并重置重试次数、异常与到期时间。
- 死信 moveToHistoryJob：**不支持**，返回 400。原因：当前引擎没有历史作业模型（HistoryJob 表/实体），伪造成功会产生不可执行的历史作业。
- 其他 action 返回 400。

### 2.3 批次文档

- 批次与分片 DTO 新增可选 Document []byte（json 标签 document, omitempty），旧快照缺少该字段时保持兼容。
- BatchMigrateProcessInstances 创建批次时写入批次文档（批次元数据 + 目标定义 + 实例列表）与分片文档（分片元数据）。
- 文档为创建时快照，分片完成状态变化不重写文档。
- 无文档（例如更旧快照）返回 404，与 Flowable“批次没有文档”语义一致。
- 删除批次会级联移除分片与文档。

### 2.4 部署资源

- /resources 列表与 /resources/** 单条均返回 DeploymentResourceResponse：id（资源名）、url、contentUrl、mediaType、type。
- url 指向 /resources/{URL 编码资源名}，contentUrl 指向 /resourcedata/{URL 编码资源名}，均为绝对地址。
- mediaType 按 Flowable DefaultContentTypeResolver 默认映射：xml/bpmn/cmmn/dmn → text/xml，app/event/form/channel → application/json，png → image/png，txt → text/plain，其余 application/octet-stream。
- type：BPMN 资源为 processDefinition；png/jpg/jpeg/gif/svg 为 processImage；其余 resource。注：当前 Java RestResponseFactory 只显式产生前两种，processImage 依据官方枚举与 ResourceNameUtil.DIAGRAM_SUFFIXES 补齐。
- /resourcedata/{resourceName} 返回原始字节，设置同一 mediaType 与 Content-Disposition: attachment。
- 资源名含斜杠时必须 URL 编码（forms%2Fprocess.bpmn20.xml），原始斜杠按官方约定不会命中 {resourceName} 单段变量。

### 2.5 身份链接

- 新增字面量 users 路由，避免与 {family} 通配在契约规范化后无法匹配官方键。
- GET 返回官方 RestIdentityLink：url、user、type（组链接走既有 {family} 扩展路由）。
- DELETE 成功 204；不存在 404；非法 family 仍由既有路由返回 400。

## 3. 实现位置

- 路由注册：internal/rest/server.go
- 新增处理器与资源元数据：internal/rest/missing_endpoints.go
- 作业桶过滤：internal/rest/jobs_extra.go、internal/rest/server.go
- 引擎作业桶：internal/engine/job_admin.go、internal/engine/state.go
- 引擎批次文档与删除：internal/engine/batch.go
- API DTO/接口：api/types.go、api/engine.go
- 契约规范化：internal/quality/routes.go
- 测试：internal/rest/missing_endpoints_test.go、internal/rest/deployment_resource_test.go、internal/rest/jobs_extra_test.go、internal/engine/job_bucket_test.go、internal/engine/batch_document_test.go、internal/quality/baseline_test.go

## 4. 契约规范化

normalizeRoutePath 只按完整路径段处理：

- 单段变量 {id} / {name} → {}
- 跨段通配 ** / {name...} → {**}
- 普通字面量段不变

这样官方 /resources/** 能与实现 {name...} 匹配，又不会让 /resources/{name} 误匹配跨段通配。新增单测覆盖该精度。

## 5. 验证

    HOME=/root go test -count=1 ./internal/rest ./internal/engine ./internal/quality
    HOME=/root go run ./tools/contract -write
    HOME=/root go run ./tools/contract
    HOME=/root go run ./tools/quality -doccheck
    HOME=/root go test -count=1 -coverprofile=/tmp/rest.cover ./internal/rest
    HOME=/root go test -count=1 -coverprofile=/tmp/engine.cover ./internal/engine
    HOME=/root go test -count=1 -coverprofile=/tmp/quality.cover ./internal/quality

本期结果：

- 三个包测试通过。
- 契约生成/校验通过，官方缺失数 0。
- 覆盖率：internal/rest 90.5%，internal/engine 91.5%，internal/quality 98.7%。
- 中文函数注释门禁通过。

## 6. 边界与残留问题

1. moveToHistoryJob 未实现，明确返回 400；需要先引入历史作业模型与异步历史执行器。
2. 契约只证明路由存在，未做与 Java 的逐字段响应比对；例如批次 JSON 响应现在会带可选 document 字段（扩展字段，旧客户端应忽略）。
3. 批次/分片文档为创建时快照，不随批次完成或分片状态更新；与 Flowable 文档字段的完整生命周期仍有差异。
4. processImage 依据官方枚举补充，当前 Java 工厂未显式产生该值，属于契约枚举对齐而非逐行复刻。
5. reschedule 的 date 别名是兼容扩展；官方 Java 请求字段为 dueDate。
6. 部署资源名只支持 URL 编码斜杠形式；未编码斜杠按官方语义 404。
7. 作业桶依赖内存状态中的流程实例挂起标记；跨重启快照恢复后语义一致，但并发挂起/删除竞态未做额外隔离。
8. 未修改 cmd/flowtide-server 与 internal/engine/dmn*。
