# Flowtide 管理台 REST 接口契约速查

> 依据 `internal/rest` 路由与 handler 实现整理；仓库内 `/service` 方法端点共 **242** 个（GET 125 / POST 53 / PUT 26 / DELETE 38）。路径参数写作 `{id}`，正文只记录代码事实。

## 0. 通用约定

### 0.1 认证、租户、操作人

- 只有 `/service/` 经过认证、RBAC、限流与审计；`/healthz`、`/readyz`、`/metrics`、`/adminpage/` 不鉴权。
- 未配置认证时全部开放。401 带 `WWW-Authenticate: Basic realm="flowable"`，体 `{"message":"未认证","code":"REST-0401"}`；403 `{"message":"无权限","code":"REST-0403"}`；429 `{"message":"请求过多","code":"REST-0429"}`。
- RBAC：`/management/*`、`/identity/*` 需 admin；GET/HEAD 与 `POST /query/*` 需 read；其余写操作需 write；静态 Bearer 视为全权限。
- 租户取 `X-Tenant-Id` 头或 `?tenantId=`；JWT 的 tenant 声明优先，跨租户返回 403。管理台不主动注入。
- 操作人优先取 `X-User-Id`，其次 Basic 用户名；任务动作还可用 body `userId`/`assignee`。

### 0.2 分页信封（两种）

- 通用 `writePage`：`{data,total,start,size}`，**无** sort/order；`size` 回显请求值（缺省 10）。
- 运行时/查询/历史/身份/模型/批次：`{data,total,start,size,sort,order}`（模型未指定 sort 时省略 sort/order，批次无这两个字段）。其中实例、任务、执行、身份、历史列表的 `size` 回显**当前页实际条数**，不是请求值。
- 参数 `start`（偏移，缺省 0）、`size`（缺省 10，通用上限 1000）。`/query/*`、任务、实例、执行、身份、历史查询允许请求 `size>1000`。
- 批处理另支持 `page`（0 基）+`size`，显式 `start` 优先。

```json
{"data":[],"total":242,"start":0,"size":50,"sort":"id","order":"asc"}
```

### 0.3 POST /service/query/* 请求体

10 个端点：`process-instances`、`executions`、`tasks`、`activity-instances`、`variable-instances`、`historic-process-instances`、`historic-task-instances`、`historic-activity-instances`、`historic-variable-instances`、`historic-detail`。

- 扁平 JSON 对象；过滤字段与 `start/size/sort/order` 同级；未知字段忽略；空 body 等同无过滤。
- 时间支持 RFC3339 / RFC3339Nano / `2006-01-02T15:04:05`。
- 变量过滤：`variables:[{name,value,operation}]`，operation 缺省 `equals`，另支持 `notequals/like/greaterthan/greaterthanorequal/lessthan/lessthanorequal`。
- 布尔/整数类型错误返回 400（REST-0124 / REST-0121）。POST 查询只从 URL 补充 `start/size/sort/order`，URL 过滤参数被忽略。

```json
{"processDefinitionKey":"leave","finished":false,"startedAfter":"2026-10-01T00:00:00Z",
 "variables":[{"name":"amount","operation":"greaterthan","value":100}],
 "start":0,"size":50,"sort":"startTime","order":"desc"}
```

### 0.4 错误体与状态码

- 统一体：`{code?,message,exception?}`。`message` 必有；业务码 `ENGINE-*/REST-*` 在 `code`；底层异常在 `exception`。
- 400 参数/业务校验、401、403、404、409 冲突、429、500 引擎内部、503 引擎关闭。
- 兼容层另有一类 **无 code** 的 `{message,exception}`：任务 404/409、运行时 400/404/409/500、身份 400/404/409、异步变量 400/409，`message` 为英文短句（如 `Conflict`）。
- 404 常见：已结束实例/执行按不存在；作业桶互斥（在 jobs 桶查 timer/suspended 作业返回 404）；历史任务详情不存在。

### 0.5 非 JSON 响应

| 端点 | Content-Type / 行为 |
| --- | --- |
| `GET .../process-definitions/{id}/xml` | `application/xml; charset=utf-8` |
| `GET .../process-definitions/{id}/image`、`.../process-instances/{id}/diagram` | `image/png`；请求头 Accept 含 `image/svg+xml` 时返回 SVG |
| `GET .../process-definitions/{id}/resourcedata`、`/deployments/{id}/resourcedata/{resourceName}` | `application/octet-stream` + `Content-Disposition: attachment`；部署资源带 ETag/Last-Modified，可 304 |
| `GET /deployments/{id}/resources`、`/resources/{name...}` | JSON 元数据 + ETag，可 304；无分页 |
| `GET/PUT models/{id}/source`、`source-extra` | `application/octet-stream` |
| `GET .../variables/{name}/data`、`/runtime/variable-instances/{id}/data` | `application/octet-stream` 原始变量值 |
| `GET /tasks/{id}/attachments/{attachmentId}/content` | 附件 `type`（非 MIME 时 octet-stream）+ attachment |
| `GET /identity/users/{id}/picture` | 图片 MIME，无头像 404 |
| `GET /management/*/{id}/exception-stacktrace` | `text/plain`；无异常 404 |
| `GET /management/batches/{id}/batch-document`、`batch-parts/{id}/batch-part-document` | `application/json`，原样文档字节 |
| `GET /metrics` | `text/plain; version=0.0.4` |
| `GET /healthz`、`/readyz` | JSON |

### 0.6 multipart 字段名

| 端点 | 文件字段 | 普通字段 / 查询 |
| --- | --- | --- |
| `POST /repository/deployments` | **任意字段名**，资源名取 filename（空则字段名） | `deploymentName`、`tenantId` 可在 query 或 form；`enableDuplicateFiltering=true` 时同租户同内容返回 200 已有部署，否则 201 |
| `PUT /repository/models/{id}/source`、`source-extra` | **任意字段名**，取第一个文件 | 无 |
| `POST /tasks/{id}/attachments` | **任意字段名** | `name`、`description`、`type` |
| `PUT /identity/users/{id}/picture` | **任意字段名**，取第一个文件 | 无（MIME 取文件 part 的 Content-Type） |
| `POST /process-instances/{id}/variables`、`PUT .../variables/{name}` | **任意字段名** | 集合 POST 需 `name`；单变量 PUT 用路径 name |
| 异步变量（`variables-async`） | **任意字段名** | 集合用 body `name`；单变量用路径 name；可带 `scope` |

上传上限 64MB（`MaxUploadSize`）。非 multipart 的部署请求直接把 body 当资源，资源名取 `?resourceName`（缺省 `deployment.bpmn20.xml`）。

### 0.7 变量与时间

- 写入变量支持数组 `[{name,type?,value,scope?}]` 或对象 `{name:value}`；`type` 留空由引擎推断，`scope` 为 global/local。
- 实例/任务/执行的变量集合端点返回 `[{name,type?,value}]`，二进制 value 为 base64。
- 实例 DTO、历史查询 DTO 中变量为 `{name,scope,type,value,valueUrl?}`；二进制为 `type=binary,value=null,valueUrl=<data 端点>`。
- 运行时/任务 DTO 时间显式 UTC 毫秒；部署、作业、审计等为 Go 默认 RFC3339（可能含纳秒）。前端统一按 ISO 解析。

## 1. 仪表盘 / 引擎

| 方法 | 路径 | 关键参数 | 响应关键字段 | 备注 |
| --- | --- | --- | --- | --- |
| GET | `/service/management/engine` | 无 | `name,version,historyLevel,closed` | 无 schemaVersion/tenantId/storageMode |
| GET | `/service/management/engine-properties`、`/management/properties` | 无 | `map[string]string`：schema.version、engine.version、historyLevel、tenantId、自定义属性 | 内置属性只读 |
| POST | `/service/management/engine-properties` | `{name,value}` | 201 `{name,value}` | 内置属性 409；空名 400 |
| PUT | `/service/management/engine-properties/{name}` | `{name?,value}` | 200 `{name,value}` | name 与路径不一致 400；内置 400 |
| DELETE | `/service/management/engine-properties/{name}` | 无 | 204 | 不存在 404，内置 400 |

仪表盘计数没有聚合端点：分别取 `deployments`、`process-definitions`、`process-instances`、`tasks`、`deadletter-jobs` 的 `total`（`size=1` 即可）。健康与指标见 §9。

## 2. 部署

| 方法 | 路径 | 关键参数 | 响应关键字段 | 备注 |
| --- | --- | --- | --- | --- |
| GET | `/service/repository/deployments` | `start,size,tenantId` | `data[] {id,name,tenantId,deployTime}` | 仅此三个过滤参数；**无 nameLike、无 sort、无资源数** |
| POST | `/service/repository/deployments` | multipart 任意文件；`deploymentName,tenantId,enableDuplicateFiltering` | 201 `Deployment`；重复且开启过滤时 200 | 单文件可直传 body + `resourceName` |
| GET | `/service/repository/deployments/{id}` | 无 | `{id,name,tenantId,deployTime}` | 404 |
| DELETE | `/service/repository/deployments/{id}` | `cascade=true` | 204 | 默认不级联 |
| GET | `.../{id}/resources` | 无 | `[{id,url,contentUrl,mediaType,type}]` | 数组，无分页；ETag/304 |
| GET | `.../{id}/resources/{name...}` | 路径可含 `/` | 单个资源元数据 | JSON；ETag/304 |
| GET | `.../{id}/resourcedata/{resourceName}` | 无 | 二进制 | attachment；ETag/304 |

## 3. 流程定义 / 模型

**定义列表**：`GET /service/repository/process-definitions` 过滤参数仅 `key,version,startableByUser,tenantId,start,size`；无 sort、无 `suspended`、无 `latestVersion`。响应字段 `id,key,name,description,version,deploymentId,resourceName,tenantId,suspended`（无部署时间/分类）。

| 方法 | 路径 | 关键参数 | 响应关键字段 | 备注 |
| --- | --- | --- | --- | --- |
| GET | `/service/repository/process-definitions` | 见上 | `data[] ProcessDefinition` | 通用分页信封 |
| GET | `.../process-definitions/{id}` | 无 | `ProcessDefinition` | 404 |
| PUT | `.../process-definitions/{id}` | `{action:"suspend"\|"activate",includeProcessInstances}` | 204 | 其他 action 400 |
| GET | `.../{id}/xml` | 无 | XML | 非 JSON |
| GET | `.../{id}/image` | Accept 可选 SVG | PNG/SVG | 无 DI 时解析失败走错误体 |
| GET | `.../{id}/model` | 无 | `{id,name,processes[],messages[],signals[],errors[]}` | 元素含 id/name/type/sourceRef/targetRef/assignee/formKey/候选 |
| GET | `.../{id}/start-form` | 无 | `{formKey,formProperties[]}` | 启动表单 |
| GET | `.../{id}/resourcedata` | 无 | 二进制 | attachment |
| GET | `.../{id}/decisions`、`.../decision-tables`、`.../form-definitions` | 无 | 数组，空为 `[]` | DMN 能力缺失时 500 |
| GET/POST | `.../{id}/identitylinks` | POST `{type,userId,groupId}` | 数组 / 201 `IdentityLink` | 候选发起人 |
| GET/DELETE | `.../{id}/identitylinks/{family}/{identityId}` | family=users/groups | 单条 / 204 | |
| POST | `.../{id}/migrate` | `{processInstanceIds[],activityMappings{}}` | 204 | 逐个迁移，失败即中断 |
| POST | `.../{id}/batch-migrate` | 同上 | 201 `Batch` | 失败记录在分片 |

**模型列表**：`GET /service/repository/models` 参数 `id,category,categoryLike,categoryNotEquals,name,nameLike,key,deploymentId,tenantId,tenantIdLike,deployed(true/false),withoutTenantId,latestVersion,version,sort,order,start,size`；`sort` 支持 id/category/createTime/key/lastUpdateTime/name/version/tenantId。响应 `{id,name,key,category,version,metaInfo,deploymentId,tenantId,createTime,lastUpdateTime}`。

| 方法 | 路径 | 关键参数 | 响应关键字段 | 备注 |
| --- | --- | --- | --- | --- |
| POST | `/service/repository/models` | `{name,key,category,metaInfo,tenantId,version}` | 201 `Model` | **无重复 Key 校验**，同名 Key 可重复创建 |
| GET/PUT/DELETE | `/service/repository/models/{id}` | PUT 局部字段，null 清空 | `Model` / 204 | |
| GET | `.../{id}/source`、`source-extra` | 无 | octet-stream | 模型 XML / 扩展源码 |
| PUT | `.../{id}/source`、`source-extra` | multipart 任意文件字段 | 204 | 缺文件 400 |
| POST | `.../{id}/deployment` | 无 | 201 `Deployment` | 资源名 `{key}.bpmn20.xml`，空源码 400 |

## 4. 实例

**列表**：
- `GET /service/runtime/process-instances`：`id,processDefinitionId,processDefinitionKey,processDefinitionKeyIn,businessKey,businessKeyLike,name,nameLike,suspended,startedAfter,startedBefore,includeProcessVariables,tenantId,start,size,sort,order`。`sort` 白名单 `id,processDefinitionId,processDefinitionKey,startTime,tenantId,businessKey`，默认 `id asc`。GET **不识别** `processInstanceId` 别名，只认 `id`。
- `POST /service/query/process-instances`：官方字段 `processInstanceId,processInstanceName,processInstanceNameLike,processBusinessKey,processBusinessKeyLike` 会映射为内部字段；另支持 `processDefinitionId/Key,finished,suspended,startedAfter/Before,tenantId,includeProcessVariables,variables,start,size,sort,order`。POST 中的 `id/name/businessKey` 会被删除，必须用官方别名。

```json
{"id":"pi-1","processDefinitionId":"leave:1:1","processDefinitionName":"请假","businessKey":"B-1",
 "startUserId":"alice","startTime":"2026-10-30T03:00:00Z","ended":false,"suspended":false,
 "activityId":"usertask1","tenantId":"","variables":[]}
```

| 方法 | 路径 | 关键参数 | 响应关键字段 | 备注 |
| --- | --- | --- | --- | --- |
| GET/POST | `/runtime/process-instances`、`/query/process-instances` | 见上 | 分页 `data[]` 实例 DTO | 实例 DTO **无 processDefinitionKey**；有 definitionName |
| POST | `/service/runtime/process-instances` | `processDefinitionKey\|processDefinitionId\|message` 三选一；`businessKey,name,tenantId,variables,transientVariables,returnVariables` | 201 实例 DTO | 201；同时给多个选择器 400；ID+tenantId 400 |
| GET | `/runtime/process-instances/{id}` | 无 | 实例 DTO | 已结束 404 |
| PUT | `/runtime/process-instances/{id}` | `action=suspend/activate/claim/unclaim`，或 `name,businessKey,businessStatus,dueDate` | 200 实例 DTO | 动作与属性互斥 |
| DELETE | `/runtime/process-instances/{id}` | `deleteReason` | 204 | 已结束 404 |
| POST | `/runtime/process-instances/delete` | `{processInstanceIds[],deleteReason}` | 204 | 批量逐个删 |
| GET | `.../{id}/diagram` | Accept 可选 SVG | PNG/SVG，当前活动高亮 | 非 JSON |
| GET | `.../{id}/tasks` | `start,size` | 分页 `data[]` **裸 Task 实体** | 含 candidateUsers/Groups，无 url 字段 |
| GET/PUT/DELETE | `.../{id}/variables` | PUT 数组/对象；DELETE 清空 | 200 变量数组 / 204 | |
| POST | `.../{id}/variables` | 数组/对象/multipart | 201 变量 | |
| GET/PUT/DELETE | `.../{id}/variables/{name}` | GET 返回 `{name,value}`；PUT body `{value}` 或 multipart | 200/204 | |
| GET | `.../{id}/variables/{name}/data` | 无 | 二进制 | |
| POST/PUT | `.../{id}/variables-async` | 数组；POST 仅新建，PUT 可更新 | 201（集合 POST）/204 | 异步入队，冲突 409 |
| PUT | `.../{id}/variables-async/{name}` | 单变量 `{name,type?,value}` | 204 | 变量不存在/重名 400/409 |
| POST | `.../{id}/comments`；GET/DELETE 同前缀 | POST `{message,userId}` | 201 Comment / 分页数组 / 204 | |
| GET/POST/DELETE | `.../{id}/attachments` | POST 仅 JSON 外链附件 | 分页 / 201 / 204 | 实例附件不支持 multipart |
| GET/POST/DELETE | `.../{id}/identitylinks...` | POST `{type,userId,groupId}`；单条查/删需 family+identityId(+type) | 数组 / 单条 / 204 | |
| POST | `.../{id}/change-state` | `{startActivityIds[]}` | 204 | 空数组 400 |
| POST | `.../{id}/migrate` | `{targetProcessDefinitionId,activityMappings{}}` | 204 | |
| POST | `.../{id}/validate-migration` | 同上 | 200 `{validationMessages}` | |
| POST | `.../{id}/inject` | `{injectionType:"task",id?,name,assignee?,taskId?}` | 200 Task 实体 | |
| POST | `.../{id}/evaluate-conditions` | 无 | 204 | 已结束/挂起 409 |

**执行**：`GET /service/runtime/executions` 与 `POST /service/query/executions` 共用过滤 `id,processInstanceId,processDefinitionId,processDefinitionKey,activityId,parentId,tenantId,ended,active,waiting,suspended`；`variables` 仅 POST。GET 只返回未结束执行。`sort` 白名单 `processDefinitionId,processDefinitionKey,processInstanceId,tenantId`，默认 `processInstanceId`。

| 方法 | 路径 | 关键参数 | 响应关键字段 | 备注 |
| --- | --- | --- | --- | --- |
| GET/POST | `/runtime/executions`、`/query/executions` | 见上；`start,size,sort,order` | 分页执行 DTO | `size` 回显实际条数 |
| GET | `/runtime/executions/{id}` | 无 | `{id,url,activityId,suspended,tenantId,parentId/Url,superExecutionId/Url,processInstanceId/Url}` | 已结束 404 |
| PUT | `/runtime/executions/{id}` | `action=trigger/signal/signal_event_received/message` + `variables,transientVariables,signalName,messageName` | 200 执行 DTO 或 204 | 执行已结束返回 204 |
| PUT | `/runtime/executions` | 仅 `action=signal_event_received` + `signalName,variables` | 204 | 其他 action 400 |
| GET | `.../{id}/activities` | 无 | `["activityId"]` | **实例没有此端点**，只有执行有 |
| GET | `.../{id}/variables` | 无 | 变量数组 | |
| POST/PUT | `.../{id}/variables` | 数组 | 201/200 | |
| GET/PUT/DELETE | `.../{id}/variables/{name}` | PUT `{value,type?}` | 200/204 | |
| GET | `.../{id}/variables/{name}/data` | 无 | 二进制 | |
| POST/PUT | `.../{id}/variables-async`、`PUT .../{name}` | 同实例异步变量 | 204（执行侧集合 POST 也是 204） | |
| POST | `.../{id}/change-state` | `{startActivityIds[]}` | 204 | |
| POST | `/runtime/messages`、`/runtime/signals` | `{messageName,signalName,variables}` | 200 `{triggered:n}` | 触发消息/广播信号 |
| GET | `/runtime/event-subscriptions`、`/runtime/event-subscriptions/{id}` | `processInstanceId,id,eventType,eventName,activityId,executionId,processDefinitionId,createdAfter,createdBefore,withoutProcessInstanceId,withoutProcessDefinitionId,withoutTenantId,sort,order,start,size` | 分页 EventSubscription / 单条 | 无租户过滤 |
| GET/POST | `/runtime/activity-instances`、`/query/activity-instances` | GET/body：`id,processInstanceId,processDefinitionId,activityId,activityType,activityName,activityNameLike,taskAssignee/assignee,sort,order,start,size` | 分页活动实例 | |
| GET/POST | `/runtime/variable-instances`、`/query/variable-instances` | `processInstanceId,executionId,taskId,variableName/name,variableNameLike/nameLike,sort,order,start,size`；`GET .../{id}/data` 返回二进制 | 分页变量实例 / 二进制 | |

## 5. 任务 / 表单

**列表**：`GET /service/runtime/tasks` 与 `POST /service/query/tasks` 共用字段 `id/taskId,name,nameLike,description,descriptionLike,assignee,assigneeLike,owner,ownerLike,candidateUser,candidateGroup,candidateGroupIn,involvedUser,processDefinitionId,processDefinitionKey,processInstanceId,processInstanceBusinessKey,taskDefinitionKey,priority,minimumPriority,maximumPriority,createdAfter/Before,dueAfter/Before,unassigned,finished,tenantId,delegationState,start,size,sort,order`；`variables` 仅 POST。`sort` 支持 id/name/description/assignee/owner/priority/createTime/dueDate，默认 id。GET 传任意 `unassigned`（含 false）都会只返回未签收任务；`finished=true` 返回空（该端点只含运行任务）。

```json
{"id":"t-1","name":"审批","assignee":null,"owner":null,"priority":50,"createTime":"2026-10-30T03:00:00Z",
 "dueDate":null,"processInstanceId":"pi-1","processDefinitionId":"leave:1:1",
 "taskDefinitionKey":"usertask1","delegationState":null,"variables":[]}
```

| 方法 | 路径 | 关键参数 | 响应关键字段 | 备注 |
| --- | --- | --- | --- | --- |
| GET/POST | `/runtime/tasks`、`/query/tasks` | 见上 | 分页任务 DTO | `size` 回显实际条数；任务 DTO 无 processDefinitionKey/实例名称 |
| POST | `/runtime/tasks` | `{name,description,assignee,owner,priority,dueDate,category,formKey,parentTaskId,tenantId}` | 201 任务 DTO | 独立任务 |
| GET | `/runtime/tasks/{id}` | 无 | 任务 DTO | 不存在返回 404 英文体 |
| PUT | `/runtime/tasks/{id}` | 局部 `name,description,assignee,owner,priority,dueDate,category,formKey`；null 清空 | 200 任务 DTO | 只更新提交字段 |
| DELETE | `/runtime/tasks/{id}` | `cascadeHistory,deleteReason` | 204 | |
| POST | `/runtime/tasks/{id}` | `action=claim/unclaim/delegate/resolve/complete`；`assignee,userId,variables,localVariables,transientVariables` | **200 空体** | 非 204；claim 冲突 409 英文体 |
| PUT | `/runtime/tasks` | `{taskIds[], ...任务字段}` | 200 分页信封包裹更新后任务 | 批量，逐条失败即中断 |
| GET | `.../{id}/subtasks` | 无 | 任务 DTO 数组 | 无分页 |
| GET | `.../{id}/form` | 无 | `{formKey,formProperties[]}` | 无 formKey 时 400 `任务未定义表单` |
| GET | `.../{id}/events` | `action/eventType,userId,start,size` | 事件数组 | 传 start/size 才切片，仍是数组 |
| GET/DELETE | `.../{id}/events/{eventId}` | eventId 必须整数 | 事件 / 204 | |
| GET | `.../{id}/variables`、`.../variables/{name}` | 无 | 变量数组 / `{name,value}` | |
| POST | `.../{id}/variables` | 变量对象 | 201 变量 | |
| PUT/DELETE | `.../{id}/variables/{name}` | PUT `{value}` | 204 | |
| GET | `.../{id}/variables/{name}/data` | 无 | 二进制 | |
| GET/POST/DELETE | `.../{id}/comments` | POST `{message,userId}` | 分页数组 / 201 / 204 | |
| GET/POST/DELETE | `.../{id}/attachments` | POST multipart（`name,description,type`+文件）或 JSON 外链 | 分页 / 201 / 204 | |
| GET | `.../{id}/attachments/{attachmentId}/content` | 无 | 二进制 | |
| GET/POST/DELETE | `.../{id}/identitylinks...` | 同实例；另有 `GET .../identitylinks/{family}` | 数组 / 单条 / 204 | |

**表单**：`GET /service/form/form-data?taskId=` 或 `?processDefinitionId=`；`POST /service/form/form-data` body `{taskId,values:{}}`，成功 204；缺 taskId 400。

## 6. 作业

四类列表参数只有 `start,size`，**无过滤/排序**；`history-jobs` 恒返回空数组。

| 方法 | 路径 | 关键参数 | 响应关键字段 | 备注 |
| --- | --- | --- | --- | --- |
| GET | `/service/management/jobs`、`/timer-jobs`、`/suspended-jobs`、`/deadletter-jobs`、`/history-jobs` | `start,size` | `data[] {id,type,processInstanceId,executionId,activityId,retries,exception,dueDate,executable}` | 作业桶互斥 |
| GET | `.../jobs/{id}`、`.../timer-jobs/{id}`、`.../suspended-jobs/{id}`、`.../deadletter-jobs/{id}` | 无 | `JobInfo` | 错桶 404；history-jobs/{id} 恒 404 |
| GET | `.../{id}/exception-stacktrace`（四类） | 无 | text/plain | 无异常 404 |
| POST | `.../jobs/{id}` | `{action:"moveToExecutable"\|"setRetries",retries}` | 204 | move 仅对死信有效 |
| POST | `.../deadletter-jobs/{id}` | `{action:"move"\|"moveToHistoryJob",retries}` | 204 | moveToHistoryJob 恒 400（无历史作业模型） |
| POST | `.../timer-jobs/{id}` | `{action:"move"\|"reschedule",dueDate\|date}` | 204 | reschedule 必须给时间 |
| DELETE | `.../jobs/{id}`、`timer-jobs/{id}`、`suspended-jobs/{id}`、`deadletter-jobs/{id}` | 无 | 204 | 无 suspended 的 POST 动作端点 |
| POST | `/service/management/jobs/run` | 无 | 200 `{executed:n}` | 立即执行到期定时器 |

## 7. 批处理

| 方法 | 路径 | 关键参数 | 响应关键字段 | 备注 |
| --- | --- | --- | --- | --- |
| GET | `/service/management/batches` | `type,status,tenantId,page,size,start` | 分页 `Batch` | 无 sort；`page` 0 基 |
| GET | `.../batches/{id}` | 无 | `Batch` | |
| GET | `.../{id}/batch-parts` | `page,size,start` | 分页 `BatchPart` | |
| GET | `.../{id}/summary` | 无 | `{id,batchId,type,status,total,completed,failed,inProgress,queued,startTime,endTime,tenantId}` | 计数由分片重算 |
| GET | `.../{id}/batch-document` | 无 | 原始 JSON 文档 | 非包装 |
| DELETE | `.../batches/{id}` | 无 | 204 | |
| GET | `/management/batch-parts/{id}` | 无 | `BatchPart` | |
| GET | `.../batch-parts/{id}/batch-part-document` | 无 | 原始 JSON 文档 | |

```json
{"id":"b-1","batchId":"b-1","type":"migration","status":"completed",
 "total":10,"completed":9,"failed":1,"inProgress":0,"queued":0}
```

## 8. 表数据

| 方法 | 路径 | 关键参数 | 响应关键字段 | 备注 |
| --- | --- | --- | --- | --- |
| GET | `/service/management/tables` | 无 | `[{name,count,url}]` | 全部 ACT_* 表，按表名排序；不返回分组 |
| GET | `.../tables/{name}` | 表名大小写不敏感 | `{name,count,url}` | 不存在 404 |
| GET | `.../tables/{name}/columns` | 无 | `{tableName,columnNames[],columnTypes[]}` | **无 nullable/PK/FK** |
| GET | `.../tables/{name}/data` | `start,size,orderAscendingColumn,orderDescendingColumn` | 分页 `data[]` 行对象，列名大写 | 两个排序列同时给 400；无排序列时按 ID_ |

## 9. 审计 / 指标 / 健康

| 方法 | 路径 | 关键参数 | 响应关键字段 | 备注 |
| --- | --- | --- | --- | --- |
| GET | `/service/management/audit-logs` | `start,size` | `data[] {id,time,userId,tenantId,method,path,status,durationMillis,remoteAddr}` | 无任何筛选；未启用审计时恒空 |
| GET | `/healthz` | 无 | `{status:"UP"}` | 不鉴权，固定 200 |
| GET | `/readyz` | 无 | 200 `{status:"UP",engine:{...}}` / 503 `{status:"DOWN",engine:{...}}` | engine.closed=true 时 503 |
| GET | `/metrics` | 无 | Prometheus 文本：`flowable_http_requests_total`、`flowable_http_request_duration_seconds_*`、`flowable_http_requests_in_flight`、`flowable_deployments`、`flowable_process_definitions`、`flowable_process_instances_active`、`flowable_tasks_active`、`flowable_jobs_active`、`flowable_deadletter_jobs`、`flowable_historic_process_instances` | 非 JSON |

## 10. 历史

**GET 列表过滤很窄，复杂筛选必须用 POST /query**：
- `GET /history/historic-process-instances`：仅 `processDefinitionKey,businessKey,tenantId,ended,finished,start,size,sort,order`；**不支持 startedAfter 等时间过滤**。
- `GET /history/historic-task-instances`：仅 `processInstanceId,finished,start,size,sort,order`；**不支持 assignee 等过滤**。
- `GET /history/historic-activity-instances`：仅 `processInstanceId,start,size,sort,order`。
- `GET /history/historic-variable-instances`、`historic-detail`：仅 `processInstanceId,start,size,sort,order`。
- POST 对应 5 个 query 端点字段见 §0.3：支持时间区间、assignee、definitionKey、variables 等。

| 方法 | 路径 | 关键参数 | 响应关键字段 | 备注 |
| --- | --- | --- | --- | --- |
| GET/POST | `/history/historic-process-instances`、`/query/historic-process-instances` | 见上 | 分页历史实例 DTO | DTO 有 definitionId/Name，**无 processDefinitionKey** |
| GET | `.../historic-process-instances/{id}` | 无 | 历史实例 DTO | 404 英文体 |
| GET | `.../{id}/variables` | `start,size` | 分页**裸 HistoricVariableInstance**：id/name/type/value/createTime | 与 query 的嵌套 variable 形状不同 |
| GET | `.../{id}/variables/{name}/data` | 无 | 二进制 | |
| GET/POST | `.../{id}/comments` | POST `{message,userId}` | 评论 DTO 数组 / 201 | |
| GET/DELETE | `.../{id}/comments/{commentId}` | 无 | 单条 / 204 | |
| GET | `.../{id}/identitylinks` | 无 | 身份链接 DTO 数组 | 无分页 |
| DELETE | `.../historic-process-instances/{id}` | 无 | 204 | 运行中实例返回 500 文本 |
| POST | `.../historic-process-instances/delete` | `{processInstanceIds[],deleteReason}` | 204 | |
| GET/POST | `/history/historic-task-instances`、`/query/historic-task-instances` | 见上 | 分页历史任务 DTO | |
| GET | `.../historic-task-instances/{id}` | 无 | 历史任务 DTO | |
| GET | `.../{id}/identitylinks` | 无 | 数组 | |
| GET | `.../{id}/form` | 无 | **恒 400** | 有 formKey 返回 `Form engine is not initialized` |
| GET | `.../{id}/variables/{name}/data` | 无 | 二进制 | **没有 `/variables` 列表端点** |
| DELETE | `.../historic-task-instances/{id}` | 无 | 204 | |
| GET/POST | `/history/historic-activity-instances`、`/query/historic-activity-instances` | 见上 | 分页活动 DTO | |
| GET | `.../historic-activity-instances/{id}` | 无 | 活动 DTO | |
| GET/POST | `/history/historic-variable-instances`、`/query/historic-variable-instances` | 见上 | 分页变量 DTO `{id,processInstanceId,taskId,variable:{name,scope,type,value,valueUrl?}}` | |
| GET | `.../{id}`、`.../{id}/data`、DELETE `.../{id}` | 无 | 变量 DTO / 二进制 / 204 | |
| GET/POST | `/history/historic-detail`、`/query/historic-detail` | 见上 | 分页明细 DTO `{id,detailType,time,variable{...}}` | |
| GET/DELETE | `.../historic-detail/{id}/data`、`.../{id}` | 无 | 二进制 / 204 | |
| GET | `/history/historic-task-log-entries` | `taskId,processInstanceId,userId,type,action,from,to,start,size,sort,order` | 分页日志 DTO `{logNumber,type,userId,timeStamp,data}` | 事件名 create/complete 映射为 USER_TASK_* |
| DELETE | `.../historic-task-log-entries/{id}` | 数字 ID | 204 | |
| GET | `/history/historic-identitylinks` | `taskId,start,size` | 分页链接 DTO | |

```json
{"id":"pi-1","processDefinitionId":"leave:1:1","processDefinitionName":"请假","businessKey":"B-1",
 "startUserId":"alice","startTime":"2026-10-30T03:00:00Z","endTime":"2026-10-30T04:00:00Z",
 "durationInMillis":3600000,"deleteReason":null,"state":"completed"}
```

## 11. 身份

**用户列表**：`GET /service/identity/users` 参数 `id,firstName,lastName,displayName,email,memberOfGroup,tenantId,start,size,sort,order`；名称/邮箱类字段同时支持对应 `xxxLike`（%/_ 通配）。`sort` 白名单 id/firstName/lastName/displayName/email。响应 `{id,firstName,lastName,displayName,email,url,pictureUrl,tenantId?}`，**不回显 roles/disabled**。

| 方法 | 路径 | 关键参数 | 响应关键字段 | 备注 |
| --- | --- | --- | --- | --- |
| GET | `/service/identity/users` | 见上 | 分页用户 | `size` 回显实际条数 |
| POST | `/service/identity/users` | `{id,firstName,lastName,displayName,email,password,roles?,tenantId?}` | 201 用户 | 可写 roles，但响应不返回 |
| GET | `/users/{id}` | 无 | 用户 | |
| PUT | `/users/{id}` | 局部 `firstName,lastName,displayName,email,password` | 200 用户 | **不接受 roles**，角色走 PUT roles |
| PUT | `/users/{id}/roles` | `{roles:[]}` | 204 | 全量替换；**没有 GET roles** |
| DELETE | `/users/{id}` | 无 | 204 | |
| GET/POST | `/users/{id}/groups` | POST `{groupId}` | 数组 `Group` / 204 | |
| DELETE | `/users/{id}/groups/{groupId}` | 无 | 204 | |
| GET | `/users/{id}/info` | 无 | `[{key,url}]`（不含值） | 需逐个 GET |
| POST | `/users/{id}/info` | `{key,value}` | 201 `{key,value,url}` | 重复 409 |
| GET/PUT/DELETE | `/users/{id}/info/{key}` | PUT `{key?,value}` | 200 / 204 | |
| GET/PUT | `/users/{id}/picture` | PUT multipart 任意文件 | 图片 / 204 | 无头像 404 |
| PUT | `/users/{id}/password` | `{oldPassword,newPassword}` | 204 | old 为空即管理员重置 |
| GET/POST | `/identity/roles` | POST `{id,name,permissions[],tenantId?}` | 分页 / 201 Role | 仅 `start,size`，无过滤排序 |
| GET/PUT/DELETE | `/identity/roles/{id}` | PUT 全量 Role | Role / 204 | |
| GET | `/identity/groups` | `id,name,type,nameLike,member,start,size,sort,order` | 分页 `{id,name,type,url}` | 无 tenantId 过滤/回显 |
| POST | `/identity/groups` | `{id,name,type,tenantId?}` | 201 Group | 重复 409 |
| GET/PUT/DELETE | `/identity/groups/{id}` | PUT 局部 name/type | Group / 204 | |
| GET/POST | `/groups/{id}/members` | POST `{userId}` | **裸 User 实体数组** / 201 | 成员数组含 roles 等实体字段 |
| DELETE | `/groups/{id}/members/{userId}` | 无 | 204 | 非成员 404 |

## 12. 系统

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/` | 302 跳 `/adminpage/` |
| GET | `/adminpage` | 301 跳 `/adminpage/` |
| GET | `/adminpage/` | 管理台静态资源（hash 路由） |
| GET | `/healthz`、`/readyz`、`/metrics` | 见 §9；不在 `/service` 下 |

没有运行期“端点目录”接口；242 个端点清单是构建期产物。BASIC/JWT 等凭据由浏览器或反向代理提供，管理台不注入。

## 13. 实现与 docs 计划不一致 / 缺失

1. **实例 `.../activities` 不存在**：计划 §9 实例详情列了 `.../activities`，实际只有 `GET /runtime/executions/{id}/activities`。
2. **历史任务变量列表缺失**：计划列 `GET /history/historic-task-instances/{id}/variables`，代码只有 `.../variables/{name}/data`。
3. **部署能力不足**：计划要求 `nameLike`、排序、资源数、分类；列表只支持 `start,size,tenantId`，响应无资源数，部署请求也无分类字段。
4. **定义列表过滤不足**：计划要求 `latestVersion/suspended`；代码只有 `key,version,startableByUser,tenantId`，且无 sort。
5. **引擎信息字段不足**：计划要从 `/management/engine` 取 schemaVersion、tenant、存储模式；实际只有 `name,version,historyLevel,closed`，其余在 engine-properties。
6. **审计无筛选**：计划 FR-MGMT-05 要求筛选；实际只有 `start,size`。
7. **表结构元数据缺列**：计划要求可空/主键/外键；实际仅 `columnNames/columnTypes`。
8. **历史 GET 过滤远少于计划**：`startedAfter/finished` 等组合仅在 POST `/query/historic-*` 可用；GET 历史实例不支持 startedAfter。
9. **表单能力受限**：任务无 formKey 时 `/tasks/{id}/form` 返回 400；历史任务表单恒 400（Form engine is not initialized）。
10. **身份 DTO 不回显角色**：用户列表/详情无 roles/disabled，且没有 GET user roles；只有 PUT 全量设置。
11. **分页信封不一致**：计划 §7.2 说列表统一 `{data,total,start,size,sort,order}`；部署、定义、作业、表、审计、评论、附件等使用 `{data,total,start,size}`，无 sort/order。
12. **`size` 语义不一致**：通用列表回显请求 size；实例/任务/执行/身份/历史回显当前页实际条数。
13. **作业动作不统一**：计划写 `POST .../{id}` 覆盖四类；suspended-jobs 没有 POST 动作端点。
14. **实例 DTO 无 processDefinitionKey**：列表按 key 过滤，但响应只有 processDefinitionId/Name；历史 DTO 同样无 key。
15. **实例 GET 不支持官方别名**：`processInstanceId` 只在 POST query 生效；GET 必须用 `id`。
16. **模型 Key 不校验重复**：计划 FR-MODEL-02 要求 Key 重复冲突；`POST /repository/models` 未检查 Key，重复创建会成功。
17. **计划中的 242 端点总数属实**；无代码缺失的官方 184 端点路由（以 docs-site facts.json 对照）。

## 14. 前端调用建议

- **GET 列表**：部署、定义、模型（带 sort）、作业四桶、批处理、表、审计、身份、简单历史首屏。参数用 `start/size`；管理台统一 `size=50`。
- **POST /query**：实例、任务、执行、活动实例/变量实例、5 类历史。凡涉及时间区间、assignee、variables、多条件组合或 `size>1000`，一律 POST；GET 历史只保留首屏简单筛选。
- **计数**：用列表 `total` + `size=1`；不要依赖返回的 `size` 做页大小（实例/任务/历史会回显实际条数）。
- **非 JSON**：XML/SVG/PNG/二进制用 `fetch` + `blob()`；图优先请求 `Accept: image/svg+xml`，失败降级 PNG，再失败展示 XML。
- **FormData 字段**：部署/模型源码/头像/二进制变量都用**任意文件字段名**（推荐 `file`）；附件再加 `name/description/type`；单变量 PUT 的变量名走路径，集合 POST 走 `name`。
- **任务完成**：`POST /runtime/tasks/{id}` body `{action:"complete",variables:[...]}`；成功是 **200 空体**，不要当 204。
- **错误处理**：先读 `code`，无 code 时展示 `message` 与可复制 `exception`；401 不跳转，403 保留页面，404 给返回列表入口。
- **认证/租户**：不注入 `Authorization` 与 `X-Tenant-Id`；依赖同源 Cookie/代理或服务端默认租户。

## 15. 端点覆盖

| 域 | 数量 | 域 | 数量 |
| --- | ---: | --- | ---: |
| runtime | 96 | management | 41 |
| repository | 34 | identity | 30 |
| history | 29 | query | 10 |
| form | 2 | **合计** | **242** |
