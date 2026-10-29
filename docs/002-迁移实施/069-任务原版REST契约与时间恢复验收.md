# 069 任务原版 REST 契约与时间恢复验收

## 1. 范围与证据

固定 Java 提交 `f6017e32a4bdd3e59a8881f58b0ee181652d1d9e`，通过原版四个任务控制器、异常处理器、真实 H2 引擎与 Spring 7.0.7 MockMvc 生成 `testdata/compatibility/java/004-task-rest.json`。26 个独立场景覆盖下列八个官方方法：

| 方法 | 路径（前缀 `/service`） | 已验收场景 |
| --- | --- | --- |
| GET | `/runtime/tasks` | 默认排序、指定排序分页、零大小、越界、负值回退、名称与 unassigned 筛选 |
| POST | `/runtime/tasks` | 独立任务创建、完整 DTO 和创建副作用 |
| GET | `/runtime/tasks/{id}` | 父子任务完整 DTO、资源不存在错误 |
| PUT | `/runtime/tasks/{id}` | 显式 null 清空、零优先级、更新后重新读取 |
| DELETE | `/runtime/tasks/{id}` | 删除独立任务及实际剩余任务集合 |
| POST | `/runtime/tasks/{id}` | 签收、同用户重复签收、冲突、取消签收、委派、处理委派、完成 |
| GET | `/runtime/tasks/{id}/subtasks` | 子任务数组与父资源链接 |
| POST | `/query/tasks` | 名称筛选、URL 分页补充、请求体分页优先及忽略 URL 过滤条件 |

Go 回放比较完整响应 JSON、数组顺序、显式空值、状态码、动作后读回结果和公共服务任务状态。仅归一化动态标识；创建时不能全局替换短数字 ID，否则会误改日期字符串。原版源码及运行器未修改控制器方法，两个独立 Java 执行的完整 JSON 一致。来源及控制器字节摘要见 `task-provenance.json`，制品摘要由 `make contract` 校验。

按用户要求，Java 生成器只留本地，不纳入提交。干净检出能够回放黄金样本；重新生成需取得本地运行器并核对来源摘要。

## 2. HTTP 契约修复

- 任务响应显式输出空值、资源绝对 URL、执行标识、挂起状态和签收时间；委派状态按原版输出 `pending/resolved`。
- 默认变量列表为数组；普通任务响应不再直接序列化引擎内部的合并变量映射及候选集合。任务变量和候选身份仍通过对应资源接口读取。
- 创建返回 201，读取和更新返回 200，删除返回 204。任务动作成功返回 **200 空正文**，替代此前的 204；目标处理人读取官方 `assignee`，保留既有 `userId` 请求扩展。
- 原版资源不存在和签收冲突的 `message/exception` 按原值输出；其他错误仍由公共处理器返回，不将所有冲突伪装为签收冲突。
- 默认排序为 `id/asc`，分页 `size` 为实际返回条数。`size=0` 返回空页；超出末尾保留总数；负 start/size 按原版回退默认值。
- POST 查询的正文分页优先，URL 仅补充缺失分页字段；URL 过滤条件不参与 POST 查询。GET 与 POST 共用过滤、排序及响应转换。
- 固定 Java 控制器仅判断 `unassigned` 是否非空，故 **false 也选择未签收任务**，与 OpenAPI 中的描述不同。当前按原版实际行为处理，错误类型仍先校验，不能让非法布尔值借此绕过检查。

这些响应调整会影响依赖旧 Go DTO 或 204 动作状态的调用方，客户端应使用原版任务契约。其余方法不因本阶段八个方法通过而自动获得兼容结论。

## 3. 任务状态与查询修复

原版允许同一处理人再次签收，并刷新签收时间；其他处理人签收返回冲突。未签收任务也可以委派；已有所有者在委派时保留，仅所有者为空时继承当前处理人。处理委派后保留所有者；无所有者的委派任务恢复为未签收状态。Go 内部 API 继续用大写枚举，REST 单独转换为原版小写。

任务列表原有引擎单页上限为一千条。REST 查询现在在筛选前读取全部页，防止后续匹配任务被静默丢弃；大页输出不再被单次引擎读取上限截断。`TestTaskQueryTenantAndLargePage` 验证第 1001 条任务的筛选和大页结果，并验证独立任务优先按自身租户过滤。候选用户仍由引擎结合身份组关系筛选，不能再次仅按直接候选人缩小结果。

## 4. 时间与恢复修复

原版 Java 日期为毫秒精度。参考时钟输入故意包含纳秒，原版实际响应为 UTC 毫秒；Go 任务响应现在统一为同样精度及 UTC 时区。签收时间在命令中按 UTC 毫秒保存，经 `ACT_RU_TASK.CLAIM_TIME_` 恢复，不依赖历史级别。任务 DTO 的执行标识及实例挂起状态也在恢复后可读。

真实 PostgreSQL 验收进一步复现：本地 `+08:00` 时间直接绑定到无时区列后，以 UTC 读取会漂移八小时。行 SQL 参数统一把 `time.Time` 和可空时间转成 UTC，插入、更新使用同一编码；不改变调用方对象，空指针绑定 SQL NULL。该修复同时覆盖其他行表的时间参数。

`TestSQLTimestampUTC` 和 `TestSQLNullableTimestampUTC` 先复现正、负时区及可空日期问题。双库集成比较任务创建时间的绝对时刻、UTC 毫秒签收时间及带 `+08:00` 到期时间，关闭引擎后从标准行恢复再核对。`TestTaskClaimMetadataRestore` 另验证关闭历史时仍可恢复签收元数据，调用方修改返回指针不会污染状态。

此修复保证后续 SQL 参数正确编码，不自动修复旧数据库中已经丢失时区信息的错误时间；存量异常数据需结合备份及实际写入时区另行核对。

## 5. 回归与边界

```bash
go test ./internal/rest -run 'TestJavaTaskRESTReference|TestTaskQueryTenantAndLargePage' -count=1
go test ./internal/engine -run TestTaskClaimMetadataRestore -count=1
go test ./internal/persistence/flowable -run 'TestSQLTimestampUTC|TestSQLNullableTimestampUTC' -count=1
make check
make race
make test-mysql
make test-postgres
make docs-check
```

真实数据库任务需要 `.env` 或环境配置，只使用隔离临时库。当前共六个异步变量方法和八个任务方法已有原版 HTTP 场景回放，覆盖 40 个原始场景。任务的全部字段组合、变量包含选项、批量更新及其他官方方法仍需完整 Java 契约对照；M6 保持进行中。查询仍使用内存过滤和全量读取，不构成 SQL 查询下推、容量或集群验收。
