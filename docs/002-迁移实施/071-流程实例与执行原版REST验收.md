# 071 流程实例与执行原版 REST 验收

## 1. 范围与来源

固定 Java 提交 `f6017e32a4bdd3e59a8881f58b0ee181652d1d9e`，使用原版六个流程实例及执行控制器、异常处理器、真实 H2 引擎与 Spring 7.0.7 MockMvc，生成 `testdata/compatibility/java/005-runtime-rest.json`。42 个独立场景覆盖十个官方方法：

| 方法 | 路径（前缀 `/service`） | 验收内容 |
| --- | --- | --- |
| GET | `/runtime/process-instances` | 完整对象、默认排序、有效与非法排序、分页、零数量、负数回退和挂起过滤 |
| POST | `/runtime/process-instances` | 按定义键、定义标识、消息启动，精确变量及返回变量，选择器互斥和未知定义 |
| GET | `/runtime/process-instances/{id}` | 完整对象、定义说明、启动人与不存在错误 |
| PUT | `/runtime/process-instances/{id}` | 属性更新、空更新、挂起、激活、冲突、签收、取消签收和非法动作 |
| DELETE | `/runtime/process-instances/{id}` | 空正文 204、删除后存活实例和任务数量 |
| POST | `/query/process-instances` | URL 补充分页、正文分页优先、未知字段和排序白名单 |
| GET | `/runtime/executions` | 实例根和活动子执行、默认排序、按实例过滤 |
| GET | `/runtime/executions/{id}` | 根和子执行完整对象、父资源 URL 和不存在错误 |
| PUT | `/runtime/executions/{id}` | 接收任务触发、变量及活动推进、非法动作 |
| POST | `/query/executions` | 活动、父执行过滤和零数量分页 |

两个独立 Java 执行生成的文件字节完全一致。`runtime-provenance.json` 保存六个控制器字节摘要、本地运行器源码摘要及制品摘要；基线清单和 `make contract` 校验黄金样本及来源文件。原版控制器没有修改。按用户要求，Java 生成器和重生成脚本只留本地，提交黄金样本、来源记录与 Go 验收代码。

Go 测试逐字段比较完整响应、显式空值、状态码、存活实例的后续 GET、精确变量值和实际任务数量。两端均注入单调二十位标识，替换动态标识后仍严格核对分页及执行数组顺序。唯一集合排序规范化是实例 DTO 的 `variables`：按作用域和名称比较成员，避免依赖 Java Map 的哈希桶遍历顺序；原始黄金样本保持原样，变量值内部的 JSON 数组也不重新排序。

## 2. 实例与执行响应

实例响应输出原版资源链接、显式空值、UTC 毫秒时间和变量数组，不再直接序列化内部 Go DTO。GET、列表和启动补查定义名称及说明；PUT 的属性更新和动作按原版保持这两个字段为空。流程定义说明写入 `ACT_RE_PROCDEF.DESCRIPTION_`，旧 Go 状态缺失说明时可从保存的 BPMN 模型读取。

实例更新支持名称、业务键、业务状态及到期时间，成功返回 200 和完整对象。挂起或激活成功也返回 200；重复请求返回 409。签收记录实例签收人、UTC 毫秒签收时间和 assignee 身份链接，取消签收同步清除相应字段。实例属性与签收链接在同一命令事务内提交；冲突、取消、跨租户和持久化失败不能留下部分更新。

固定原版的实例重复签收抛出通用异常，实际 HTTP 为 **500**，异常说明为已签收；本实现按原版输出该特定异常。核心 API 仍返回 `api.ErrConflict`，不把其他冲突改成 500。此行为与任务签收冲突的 409 不同。

执行响应按原版输出活动、父执行、上级执行、所属实例、租户及挂起状态，不把内部 active、waiting 等控制字段混入官方 DTO。已结束的实例和执行从运行时 HTTP 资源消失，历史通过历史接口读取；核心 SDK 查询语义保持独立。

## 3. 启动、变量和查询

启动请求要求 `processDefinitionId`、`processDefinitionKey`、`message` 三者恰好设置一个。定义标识不能与显式租户标识组合；未知定义键返回原版 400。消息启动只在当前租户的最新定义中匹配顶层消息开始，不误投递到运行时等待事件或事件子流程。歧义消息被拒绝，不能部分创建实例。

消息选择回归同时发现部署版本使用全局 Key 缓存，导致租户间版本相互累计。新版本从当前租户的标准定义计算，两个租户的同名定义独立递增；旧快照与旧行表继续可读。

启动和执行动作按声明类型解析变量，长整数不经过浮点数，日期保留时间类型。整组普通及瞬态变量完成校验后才执行命令，非法日期不能留下实例、变量更新或活动推进。既有对象形式变量扩展仍保留，不将该扩展宣称为原版契约。

运行时查询共用 GET 与 POST 的过滤、排序、分页和响应转换。实例默认按 `id/asc`，执行默认按 `processInstanceId/asc`；`size` 为实际返回条数。零数量返回空页，负数回退默认值，极大分页数先比较剩余条数以避免整数溢出。

实例允许排序字段为 `processDefinitionId`、`processDefinitionKey`、`id`、`startTime`、`tenantId`、`businessKey`；执行允许 `processDefinitionId`、`processDefinitionKey`、`processInstanceId`、`tenantId`。原版不允许实例按 name 或执行按 activityId 排序，非法字段按原版异常正文返回 400。

POST 正文分页优先，URL 仅补充缺失分页项，不补充过滤项。实例 POST 正文使用 `processBusinessKey`、`processInstanceName` 等原版字段，GET 的 businessKey/name 不是正文别名。样本包含正文未知 businessKey 被忽略的真实原版场景。

## 4. 标准行与重启恢复

实例名称、启动人、业务状态、到期时间、签收时间、签收人、父调用执行及回调/关联字段由标准根行保存。行映射补齐原版 DDL 已有的 `CLAIM_TIME_` 与 `CLAIMED_BY_`，无需修改固定 DDL 或数据库版本。

新 Go 续跑控制增加 `standardInstanceMetadata` 标记，只携带实例定位信息及内部执行控制，不再用元数据副本覆盖标准根行。缺少该标记的旧控制格式仍按旧语义恢复，以兼容此前把属性放在控制扩展中的状态。旧单根状态的导入与继续执行回归继续通过。

`TestProcessMetadataRows` 在关闭历史时保存、恢复全部实例属性，并直接改写标准根行名称，验证带控制扩展和仅标准行两种恢复都以标准行为准。`TestProcessUpdateAtomicity` 验证冲突、租户、取消、保存失败回滚和可空时间副本。双库真实集成在关闭首个引擎后创建新引擎，从行表恢复，再以直接 SQL 查询核对字段和绝对时间，避免扩展副本掩盖标准行缺失。

## 5. 验证与边界

```bash
go test ./internal/rest -run 'TestJavaRuntimeRESTReference|TestRuntimeTypedVariablesAtomicity' -count=1
go test ./internal/engine -run 'TestProcessMetadataRows|TestProcessUpdateAtomicity|TestMessageStartSelection' -count=1
make check
make race
make test-mysql
make test-postgres
make docs-check
```

真实数据库任务只使用隔离临时库。累计六个异步变量、八个任务、十个实例/执行方法具备原版 HTTP 场景回放，共 24 个方法、82 个原始场景；184 个方法的基础路由与安全矩阵已通过，**完整字段组合及其余方法的 Java 行为验收仍未完成，M6 保持进行中**。

本阶段不宣称所有包含变量选项、空字符串与 null 组合、消息租户请求组合、并发动作竞争、复杂调用流程、级联历史和其他查询字段均已完成原版对照。运行时查询仍以全量读取和内存过滤为主，不构成 SQL 查询下推、容量或集群验收。
