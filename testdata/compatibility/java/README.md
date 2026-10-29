# Java 引擎真实数据样本

本目录的 JSON 直接运行固定提交的 Flowable Java 引擎生成。原始实体来自 JDBC，业务期望来自 Java 公共服务或原版 REST 控制器；没有使用 Go 引擎生成或推导这些结果。按本地交付约束，Java 生成器与构建脚本仅用于本地验收，不纳入版本库；干净检出不能直接重生成 Java 样本。Go 普通测试消费固定制品，不依赖 Java 环境。

参考源码提交为 `f6017e32a4bdd3e59a8881f58b0ee181652d1d9e`，在 JDK 17 下从该提交编译引擎和依赖模块，再使用 H2 内存数据库生成数据。构建及样本来源摘要见 `provenance.json`。制品中的身份与业务数据均为生成器创建的固定测试数据。

## 数据和业务期望

`001-user-task-audit.json` 的顶层 `baseline` 与 `schemaVersion` 绑定 Java 提交和实际 Schema。`scenarios` 分别保存 `approve`、`review` 两个等待点：

- `before`：JDBC 读取的实际 `ACT_*` 表计数和完整原始列值。
- `beforeExpected`：完成任务之前，从 Java 公共服务读取的任务、候选人、流程变量、任务局部变量与 audit 历史。
- `expected`：Java 完成当前任务之后，从公共服务读取的对应结果。
- `completed` 与 `completedExpected`：两个用户任务全部完成后的实体和公共服务结果。

流程为 `start → approve → review → end`，使用 `tenant-a`、业务键、候选用户/组、签收、任务局部变量与 audit 历史。流程变量包含中文字符串、`9007199254740993` 整数、布尔、字节、日期、JSON；完成审批时更新同名变量，用于发现恢复后的历史重复或遗漏。

字节使用 `{"$bytes":"Base64"}` 标记，公共服务返回的日期使用 `{"$date":"RFC3339"}` 标记，JDBC 时间列使用 RFC3339 字符串。消费端必须精确读取整数，禁止先转成 `float64`。固定时钟减少无关时间差异，ID 保留真实引擎生成值。

## 异步变量参考制品

- `002-async-variables.json`：真实 Java 全局/局部异步服务调用。入队时变量和 full 明细保持原值，执行持久作业后核对大整数、日期、字节、中文文本和明细数。
- `003-async-variable-rest.json`：原版四个变量控制器、异常处理器与 Spring 7.0.7 MockMvc 的实际请求响应。14 个场景覆盖六种官方方法、创建冲突、单变量名称不匹配、集合误传对象和空数组，同时记录变量及作业副作用。
- `async-provenance.json`：新增样本、未提交的生成器源码及脚本摘要；样本元数据另保存原版配置类或控制器类的 SHA-256，便于识别不同构建。

成功响应为空正文；实例变量集合为 201，执行变量集合和单变量为 204。执行集合的原版实际状态与 OpenAPI 标记不同，不能仅从规范推导期望。

## 本地重生成记录

本轮使用 JDK 17、Maven 和固定提交的无修改 Java 工作区，从源码构建 REST reactor，再通过 Maven 解析实际类路径，不用其他发布包替代原版类。002、003 各自两次独立 Java 执行的完整 JSON 一致。

```bash
go test ./internal/engine -run TestJavaAsyncVariableReference -count=1
go test ./internal/rest -run TestJavaAsyncRESTReference -count=1
go test ./internal/rest -run TestJavaTaskRESTReference -count=1
make contract
```

上面命令只回放固定样本。重生成需先取得本地生成器及构建脚本，核对 `async-provenance.json` 中的摘要，并准备固定 Java 源码和依赖。脚本校验提交且拒绝源码修改，输出目录必须不存在，两个 Java 运行器都成功后才发布；需要 Maven 代理时由调用者提供受限配置，不提交凭据。生成只使用独立 H2 内存数据库，不读取 `.env` 或连接业务库。新旧制品必须人工审核并更新 `baseline.json`，不能用 Go 响应重写 Java 期望。

## 任务 HTTP 参考制品

`004-task-rest.json` 保存八个官方任务方法的 26 个原版 HTTP 场景，包括完整 DTO、显式空值、资源 URL、UTC 毫秒时间、分页及其优先级、父子任务、签收冲突、重复签收、委派和完成副作用。`task-provenance.json` 保存原版四个控制器的字节摘要和本地运行器源码摘要；两次独立 Java 执行的完整 JSON 一致。该运行器同样只留本地。

Go 回归入口为 `TestJavaTaskRESTReference`，另有双库创建、签收、到期时间恢复和查询大页测试。完整场景及当前边界见实施记录 069。任务动作按原版返回 200 空正文；分页 `size` 是实际条数。固定原版的 `unassigned=false` 也选择未签收任务，与 OpenAPI 描述不同。

## 证据边界

001 已作为双库临时库转换、续跑及源快照还原的输入；002、003 的 Go 异步实现也已通过双库重启恢复后执行。各样本仅证明其中列明的场景，不自动证明复杂 BPMN、全量 REST、生产切换、反向接管或集群兼容。详细边界见实施记录 067、068。

`005-runtime-rest.json` 保存十个流程实例及执行方法的 42 个原版 HTTP 场景，覆盖完整对象、启动选择器、消息、精确变量、分页、属性更新、签收和挂起/激活。`runtime-provenance.json` 记录六个控制器及本地运行器摘要，两次独立 Java 运行生成字节一致。两端注入单调标识以保留排序比较；仅实例 DTO 的变量集合按作用域和名称规范化，原始样本及其他数组顺序保持不变。生成器及重生成脚本按用户要求只留本地。
