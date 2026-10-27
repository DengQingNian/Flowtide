# 005 BPMN 模型、XML 解析与部署校验

> 文档编号：005  
> 依赖：003 代码结构、004 数据模型  
> 目标读者：引擎开发、BPMN 产品

## 1. 目标与范围

本层负责把 `.bpmn20.xml` / `.bpmn` 文件转换为可执行的 Go 模型，并在部署前完成校验。兼容目标：

- 能解析 Flowable 8.x 能解析的 BPMN 2.0 文件（含 `flowable:` 扩展）。
- 能导出结构等价、可被 Flowable 再次解析的 XML（用于模型 API 与迁移校验）。
- 对不支持的 BPMN 元素给出**部署期**明确错误，而不是运行期崩溃。
- 解析结果不可变、可缓存、可并发读取。

非目标：

- 不实现 XSD 全量校验（Go 生态缺少无 CGO 的 XSD 1.0 验证器）；以结构化校验 + 白名单替代。
- 不实现可视化建模器的编辑能力，仅解析与导出 DI。

## 2. 模型分层

~~~mermaid
classDiagram
    class BpmnModel {
        +Processes []*Process
        +Pools []*Pool
        +Lanes []*Lane
        +Signals []*Signal
        +Messages []*Message
        +Errors []*Error
        +Escalations []*Escalation
        +Interfaces []*Interface
        +DataStores []*DataStore
        +Imports []*Import
        +Diagram *BpmnDiagram
    }
    class Process {
        +ID string
        +Name string
        +Executable bool
        +FlowElements map[string]FlowElement
        +ExecutionListeners []Listener
        +EventListeners []EventListener
        +Documentation string
    }
    class FlowElement {
        <<interface>>
        +GetID() string
        +GetName() string
        +GetDocumentation() string
    }
    class FlowNode {
        +IncomingFlows []*SequenceFlow
        +OutgoingFlows []*SequenceFlow
    }
    class Activity {
        +LoopCharacteristics *MultiInstanceLoopCharacteristics
        +BoundaryEvents []*BoundaryEvent
        +AsyncBefore bool
        +AsyncAfter bool
    }
    class SequenceFlow {
        +SourceRef string
        +TargetRef string
        +ConditionExpression string
        +DefaultFlow bool
    }
    BpmnModel --> Process
    Process --> FlowElement
    FlowElement <|-- FlowNode
    FlowNode <|-- Activity
    FlowNode <|-- Gateway
    FlowNode <|-- Event
    Activity <|-- Task
    Activity <|-- SubProcess
    Activity <|-- CallActivity
~~~

设计原则：

1. **以 `BpmnModel` 为聚合根**，包含多个 `Process`（BPMN 文件可含多个流程）。
2. `Process.FlowElements` 使用 `map[string]FlowElement`，同时保留**有序切片**用于导出与遍历稳定性。
3. 接口用 Go 小接口：`FlowElement`、`FlowNode`、`ActivityBehaviorProvider` 分离。
4. 所有字段导出，便于序列化；通过 `Validate()` 保证不变量，不提供 setter。
5. 不把运行时状态放进模型；行为对象由工厂根据模型构建，独立缓存。

## 3. Go 结构体设计

~~~go
// BpmnModel 是 BPMN 文件的根模型。
type BpmnModel struct {
    Processes   []*Process          // 流程列表
    Pools       []*Pool             // 协作池
    Lanes       []*Lane             // 泳道
    Signals     []*Signal           // 全局信号
    Messages    []*Message          // 全局消息
    Errors      []*Error            // 全局错误
    Escalations []*Escalation       // 全局升级
    Interfaces  []*Interface        // 接口定义
    DataStores  []*DataStore        // 数据存储
    Imports     []*Import           // 导入
    Diagram     *BpmnDiagram        // DI 图形信息
}

// Process 表示一个可执行或不可执行的流程。
type Process struct {
    ID                 string
    Name               string
    Documentation      string
    Executable         bool
    FlowElements       map[string]FlowElement // 按 ID 索引
    OrderedElements    []FlowElement          // 保持文档顺序
    ExecutionListeners []*FlowableListener
    EventListeners     []*EventListener
    ExtensionElements  []*ExtensionElement
}

// FlowElement 是所有流程元素的公共接口。
type FlowElement interface {
    // GetID 返回元素 ID。
    // @return 元素 ID
    GetID() string
    // GetName 返回元素名称。
    // @return 元素名称
    GetName() string
    // GetDocumentation 返回文档说明。
    // @return 文档说明
    GetDocumentation() string
}

// BaseElement 提供通用字段。
type BaseElement struct {
    ID                string
    Name              string
    Documentation     string
    ExtensionElements []*ExtensionElement
    Attributes        map[string]string // 保留未知属性
}
~~~

具体元素（节选）：

~~~go
// UserTask 表示用户任务。
type UserTask struct {
    Task
    Assignee         string
    CandidateUsers   []string
    CandidateGroups  []string
    DueDate          string
    Priority         string
    Category         string
    FormKey          string
    FormProperties   []*FormProperty
    TaskListeners    []*FlowableListener
    CustomProperties map[string]string
}

// ServiceTask 表示服务任务。
type ServiceTask struct {
    Task
    Implementation     string // class / delegateExpression / expression / http
    ImplementationType string
    ClassName          string
    Expression         string
    DelegateExpression string
    ResultVariable     string
    StoreResultAsTransient bool
    FieldExtensions    []*FieldExtension
    SkipExpression     string
}

// SequenceFlow 表示顺序流。
type SequenceFlow struct {
    BaseElement
    SourceRef           string
    TargetRef           string
    ConditionExpression string
    DefaultFlow         bool
}
~~~

## 4. 解析流水线

~~~mermaid
flowchart TD
    A[读取资源] --> B{类型判断}
    B -- BPMN XML --> C[安全解码为通用节点树]
    B -- ZIP/BAR --> D[解压并逐个资源处理]
    B -- 非 BPMN --> E[存入 ACT_GE_BYTEARRAY]
    C --> F[命名空间归一化]
    F --> G[按元素转换器构建模型]
    G --> H[解析扩展元素/属性]
    H --> I[解析 BPMN DI]
    I --> J[结构化校验]
    J -->|错误| K[部署失败，返回错误列表]
    J -->|通过| L[去重与版本分配]
    L --> M[写部署/流程定义/资源]
    M --> N[创建定时器/事件订阅]
    N --> O[更新缓存]
~~~

### 4.1 安全解码

Go 标准库 `encoding/xml` 不解析外部实体，天然规避 XXE；仍需防护：

| 风险 | 措施 |
| --- | --- |
| Zip Bomb | 解压后总大小 ≤ 100MB，单文件 ≤ 20MB，压缩比 > 100 拒绝 |
| Zip Slip | 规范化路径，拒绝 `..`、绝对路径 |
| 超深嵌套 | 节点深度 ≤ 200 |
| 超大属性 | 单属性 ≤ 64KB |
| DTD/实体 | 检测到 `<!DOCTYPE` 直接拒绝 |
| 非 UTF-8 | 仅接受 UTF-8，带 BOM 时剥离 |

### 4.2 通用节点树

为保留未知扩展元素与顺序，先解码为中间树：

~~~go
// XMLNode 是保留命名空间与顺序的通用 XML 节点。
type XMLNode struct {
    Namespace string            // 命名空间 URI
    Local     string            // 本地名
    Attrs     map[string]string // 属性，key 为 "namespace local"
    Children  []*XMLNode        // 子节点
    Text      string            // 文本内容
    Order     int               // 文档顺序
}

// Decode 将 XML 解码为通用节点树。
// @param r 输入流
// @param limit 最大字节数
// @return 根节点
// @return 解析错误
func Decode(r io.Reader, limit int64) (*XMLNode, error) { /* ... */ }
~~~

然后由转换器注册表按 `(namespace, local)` 分派：

~~~go
// ElementConverter 将 XML 节点转换为模型元素。
type ElementConverter interface {
    // ElementName 返回支持的本地名。
    // @return 元素名
    ElementName() string
    // Convert 执行转换。
    // @param ctx 转换上下文
    // @param node XML 节点
    // @return 模型元素
    // @return 转换错误
    Convert(ctx *ConvertContext, node *XMLNode) (any, error)
}
~~~

对应 Flowable 的 37 个 `*XMLConverter` 与 46 个 `child` 转换器；Go 版合并为**按元素类型注册**的转换器，减少类爆炸。

### 4.3 命名空间

支持的前缀与 URI：

| 前缀 | URI | 说明 |
| --- | --- | --- |
| `bpmn`/`bpmn2` | `http://www.omg.org/spec/BPMN/20100524/MODEL` | 标准模型 |
| `bpmndi` | `http://www.omg.org/spec/BPMN/20100524/DI` | 图形信息 |
| `omgdc` | `http://www.omg.org/spec/DD/20100524/DC` | 形状 |
| `omgdi` | `http://www.omg.org/spec/DD/20100524/DI` | 连线 |
| `flowable` | `http://flowable.org/bpmn` | Flowable 扩展 |
| `activiti` | `http://activiti.org/bpmn` | 兼容别名 |

> 不依赖前缀，始终按 URI 判断；同时兼容缺失命名空间的旧文件（按 `bpmn` 处理并记录 Warn）。

## 5. 扩展元素与属性映射

### 5.1 `flowable:` 属性

| 属性 | 适用元素 | 模型字段 | 说明 |
| --- | --- | --- | --- |
| `assignee` | userTask | `UserTask.Assignee` | 支持表达式 |
| `candidateUsers` | userTask | `CandidateUsers` | 逗号分隔 |
| `candidateGroups` | userTask | `CandidateGroups` | 逗号分隔 |
| `owner` | userTask | `Owner` | 任务所有者 |
| `dueDate` | userTask | `DueDate` | ISO-8601 或表达式 |
| `priority` | userTask | `Priority` | 整数或表达式 |
| `category` | userTask | `Category` | 分类 |
| `formKey` | userTask/startEvent | `FormKey` | 表单标识 |
| `formFieldValidation` | userTask | `FormFieldValidation` | 表单校验 |
| `async` | activity | `AsyncBefore` | 兼容旧写法 |
| `asyncBefore` | activity | `AsyncBefore` | 异步续跑 |
| `asyncAfter` | activity | `AsyncAfter` | 异步离开 |
| `exclusive` | activity | `Exclusive` | 是否独占作业 |
| `failedJobRetryTimeCycle` | activity | `FailedJobRetryCycle` | 重试周期 |
| `isForCompensation` | activity | `ForCompensation` | 补偿处理器 |
| `class` | serviceTask | `ClassName` | Java 类（迁移告警） |
| `delegateExpression` | serviceTask | `DelegateExpression` | 委托表达式 |
| `expression` | serviceTask | `Expression` | 表达式 |
| `resultVariable` | serviceTask | `ResultVariable` | 结果变量名 |
| `storeResultVariableAsTransient` | serviceTask | `StoreResultAsTransient` | 瞬态结果 |
| `skipExpression` | activity | `SkipExpression` | 跳过条件 |
| `collection` | multiInstance | `Collection` | 集合表达式 |
| `elementVariable` | multiInstance | `ElementVariable` | 元素变量名 |
| `completionCondition` | multiInstance | `CompletionCondition` | 完成条件 |
| `calledElement` | callActivity | `CalledElement` | 被调用流程 |
| `inheritVariables` | callActivity | `InheritVariables` | 是否继承变量 |
| `sameDeployment` | callActivity | `SameDeployment` | 同部署优先 |
| `fallbackToDefaultTenant` | callActivity | `FallbackToDefaultTenant` | 租户回退 |
| `businessKey` | callActivity | `BusinessKey` | 业务键表达式 |
| `processInstanceName` | callActivity | `ProcessInstanceName` | 实例名表达式 |
| `initiator` | startEvent | `Initiator` | 发起人变量 |
| `triggerable` | startEvent | `Triggerable` | 可触发启动 |
| `autoStoreVariables` | scriptTask | `AutoStoreVariables` | 脚本变量回写 |
| `parallel` | multiInstance | `Parallel` | 并行/串行 |

### 5.2 扩展元素

| 扩展元素 | 用途 | 处理 |
| --- | --- | --- |
| `flowable:executionListener` | 执行监听器 | 解析 class/expression/delegateExpression + event |
| `flowable:taskListener` | 任务监听器 | create/assignment/complete/delete/update/timeout |
| `flowable:eventListener` | 流程级事件监听器 | 事件类型 + class/expression |
| `flowable:field` | 字段注入 | name/string/expression |
| `flowable:formProperty` | 表单属性 | id/name/type/required/readable/writable |
| `flowable:formField` | 动态表单字段 | 与 formKey 配合 |
| `flowable:in` / `flowable:out` | 调用活动参数映射 | source/target/sourceExpression |
| `flowable:variableAggregation` | 多实例变量聚合 | target/expression |
| `flowable:potentialStarter` | 流程候选发起人 | 写入 identity link |
| `flowable:httpRequestHandler` | HTTP 任务 | URL/method/headers/body |
| `flowable:customProperty` | 自定义属性 | 保留到模型 |

未知扩展元素：保留在 `ExtensionElements` 中并序列化回 XML，不参与执行；记录 Debug 日志。

## 6. 元素覆盖矩阵

优先级：P0 = MVP 必须；P1 = 完整版；P2 = 增强；P3 = 二期多引擎。

| 类别 | 元素 | 优先级 | 执行语义 |
| --- | --- | --- | --- |
| 开始事件 | none | P0 | 直接推进 |
| | timer | P0 | 部署时创建定时器作业 |
| | message | P1 | 创建消息订阅 |
| | signal | P1 | 创建信号订阅 |
| | error | P1 | 仅事件子流程 |
| | escalation | P1 | 仅事件子流程 |
| | conditional | P1 | 启动时求值 |
| 结束事件 | none | P0 | 结束执行 |
| | terminate | P0 | 终止整个流程实例 |
| | error | P0 | 抛出 BPMN Error |
| | escalation | P1 | 抛出升级 |
| | cancel | P1 | 取消事务子流程 |
| | compensation | P1 | 触发补偿 |
| 中间捕获 | timer | P0 | 等待定时器 |
| | message | P1 | 等待消息 |
| | signal | P1 | 等待信号 |
| | conditional | P1 | 条件满足时触发 |
| | escalation | P1 | 捕获升级 |
| 中间抛出 | none | P0 | 继续 |
| | signal | P1 | 广播信号 |
| | escalation | P1 | 抛出升级 |
| | compensation | P1 | 触发补偿 |
| 网关 | exclusive | P0 | 条件/默认流 |
| | parallel | P0 | 分叉/汇聚 |
| | inclusive | P1 | 多条件分叉/动态汇聚 |
| | eventBased | P1 | 事件竞争 |
| | complex | P3 | 参考实现未提供行为，先拒绝部署 |
| 任务 | userTask | P0 | 创建任务，等待完成 |
| | serviceTask | P0 | 委托/表达式/HTTP |
| | scriptTask | P0 | 内嵌脚本 |
| | manualTask | P0 | 自动通过 |
| | receiveTask | P1 | 等待消息 |
| | sendTask | P1 | 发送消息/信号 |
| | businessRuleTask | P2 | DMN 决策 |
| | externalWorkerTask | P2 | 外部工作项 |
| | caseServiceTask | P3 | CMMN 集成 |
| | formAwareServiceTask | P3 | 表单服务 |
| 子流程 | embedded subProcess | P0 | 作用域执行 |
| | event subProcess | P1 | 事件触发 |
| | transaction | P2 | 事务子流程 + 补偿 |
| | adHoc subProcess | P2 | 动态子流程 |
| | callActivity | P1 | 调用其他流程 |
| 多实例 | parallel | P0 | 并发执行 |
| | sequential | P0 | 串行执行 |
| 边界事件 | timer | P0 | 中断/非中断 |
| | error | P0 | 捕获错误 |
| | signal | P1 | 捕获信号 |
| | message | P1 | 捕获消息 |
| | escalation | P1 | 捕获升级 |
| | compensation | P1 | 补偿边界 |
| | conditional | P1 | 条件边界 |
| | cancel | P1 | 事务取消边界 |
| | variableListener | P2 | 变量变化 |
| 其他 | sequenceFlow | P0 | 条件与默认流 |
| | lane/pool | P1 | 仅模型与 DI |
| | dataObject | P2 | 变量映射 |
| | textAnnotation/association | P1 | 仅模型与 DI |
| | messageFlow | P2 | 仅 DI/事件注册 |
| | variableListener | P2 | 变量监听 |

## 7. 部署校验

校验在**解析后、写库前**执行，错误一次性收集并返回列表（不是遇到第一个就停）。

### 7.1 校验规则分类

| 类别 | 规则示例 | 错误码 |
| --- | --- | --- |
| 结构 | 流程必须有 ID；至少一个开始事件；元素 ID 唯一 | `BPMN-1001` |
| 连线 | 顺序流 source/target 必须存在且为 FlowNode | `BPMN-1002` |
| 网关 | 排他网关默认流必须是出线；条件不能为空 | `BPMN-1003` |
| 边界 | attachedToRef 必须存在且为 Activity；取消边界仅限事务子流程 | `BPMN-1004` |
| 事件 | 开始事件最多一个 none；定时器必须有 timeDate/timeDuration/timeCycle | `BPMN-1005` |
| 子流程 | 事件子流程开始事件必须是 error/message/signal/timer/escalation/conditional | `BPMN-1006` |
| 调用活动 | calledElement 非空；不能调用自身 | `BPMN-1007` |
| 服务任务 | class/delegateExpression/expression 三选一 | `BPMN-1008` |
| 多实例 | collection 与 loopCardinality 二选一 | `BPMN-1009` |
| 监听器 | class/expression/delegateExpression 三选一 | `BPMN-1010` |
| 引用 | message/signal/error/escalation 引用必须存在 | `BPMN-1011` |
| 图形 | DI 引用的元素必须存在；形状坐标非负 | `BPMN-1012` |
| 安全 | 禁止 `flowable:class`（可配置为警告） | `BPMN-1013` |
| 兼容 | 使用未支持元素（如 complexGateway） | `BPMN-1014` |

### 7.2 错误结构

~~~go
// ValidationError 表示一条部署校验错误。
type ValidationError struct {
    Code       string // 错误码，如 BPMN-1001
    ElementID  string // 相关元素 ID
    ElementType string // 元素类型
    Message    string // 中文说明
    Severity   string // error / warning
}
~~~

- `error` 阻止部署；`warning` 记录并继续。
- 错误信息包含资源名、行号（解析阶段记录）、元素 ID，便于定位。
- REST 返回 `400`，body 包含 `errors` 数组。

### 7.3 校验器实现

~~~go
// Validator 是单个校验规则。
type Validator interface {
    // Name 返回规则名。
    // @return 规则名
    Name() string
    // Validate 执行校验。
    // @param ctx 校验上下文
    // @return 错误列表
    Validate(ctx *ValidationContext) []ValidationError
}
~~~

校验器注册表按顺序执行；每条规则独立、无副作用、可单测。对应 Flowable 的 21 个 `*Validator`，Go 版可合并为约 15 个。

## 8. 部署流程

~~~mermaid
sequenceDiagram
    participant C as 调用方
    participant R as RepositoryService
    participant P as 解析器
    participant V as 校验器
    participant DB as 数据库
    participant Cache as 缓存

    C->>R: Deploy(name, resources)
    R->>P: 解析每个资源
    P-->>R: BpmnModel 列表
    R->>V: 校验
    V-->>R: 错误列表
    alt 有 error
        R-->>C: 400 + errors
    else 通过
        R->>DB: 开启事务 + 获取 (key,tenant) 锁
        R->>DB: 计算版本号 + 去重判断
        R->>DB: 写 deployment / procddef / bytearray
        R->>DB: 创建定时器与事件订阅
        R->>Cache: 失效并预热
        R-->>C: 部署结果
    end
~~~

### 8.1 版本与去重

- 版本号 = 同 `(key, tenant)` 下最大版本 + 1，在行锁内计算。
- 去重（`enableDuplicateFiltering`）：比较资源字节的 SHA-256；相同则复用已有流程定义，不新增版本。
- 流程定义 ID 格式沿用 Flowable：`{key}:{version}:{uuid}`；调用方不得解析。
- 派生部署（`DERIVED_FROM_`）用于父子部署场景，首期只保留字段。

### 8.2 部署资源

| 资源 | 处理 |
| --- | --- |
| `.bpmn20.xml` / `.bpmn` | 解析为流程定义 |
| `.dmn` | T1 解析为决策定义 |
| `.cmmn` | T2 |
| `.form` / `.form.json` | T1 |
| `.png` / `.svg` | 作为流程图资源 |
| 其他 | 原样存入 `ACT_GE_BYTEARRAY` |

### 8.3 部署后动作

1. 创建流程级定时器开始作业（`ACT_RU_TIMER_JOB`）。
2. 创建消息/信号开始事件订阅（`ACT_RU_EVENT_SUBSCR`）。
3. 写入流程定义缓存。
4. 发布 `ENTITY_CREATED` / `ENTITY_INITIALIZED` 事件。
5. 若启用异步历史，写部署历史。

## 9. 模型导出与 DI

- `bpmn.Write(model, w)` 输出 BPMN 2.0 XML，保持元素顺序与扩展元素。
- 导出仅保证**语义等价**，不保证字节级一致；往返测试以"再解析后模型 DeepEqual"为准。
- DI 解析：`BPMNShape`（x/y/width/height）、`BPMNEdge`（waypoints）、`BPMNLabel`。
- 图片渲染在 009 之后的 `internal/image` 实现：SVG 直接生成，PNG 用纯 Go 绘制（圆角矩形、菱形、箭头、文本换行）。
- 渲染不参与运行时，失败不影响流程执行。

## 10. 缓存

- 缓存键：`definitionID`；值包含 `*BpmnModel`、行为映射、定时器声明、事件订阅声明、校验结果。
- 模型构建后**只读**；任何动态修改（DynamicBpmnService）通过复制-替换实现。
- 部署事务提交后失效；失败回滚不失效。
- 缓存大小默认 1024，LRU 淘汰；提供命中率指标。

## 11. 测试策略

| 层级 | 内容 | 数量级 |
| --- | --- | --- |
| 单元 | 每个转换器：正常/缺省/非法输入 | 200+ |
| 黄金 | Flowable 测试 BPMN 文件解析 + 模型快照 | 1,000+ |
| 往返 | 解析 → 导出 → 再解析，模型 DeepEqual | 500+ |
| 校验 | 每条规则至少 2 个反例 | 100+ |
| 模糊 | `FuzzParse` 随机字节/随机结构，5 分钟无 panic | 持续 |
| 兼容 | 用 Java 引擎解析同一文件，对比元素/属性/连线 | 100+ |
| 性能 | 1MB BPMN 解析 < 50ms；10,000 元素 < 500ms | 基准 |

关键断言：

- 未知扩展元素不丢失。
- 命名空间别名不影响解析。
- 非法 XML 返回结构化错误，不 panic。
- 部署校验错误包含元素 ID 与行号。
- 同一资源重复部署在去重开启时不产生新版本。

## 12. 验收清单

- [ ] 元素矩阵 P0/P1 全部可解析、可执行或有明确拒绝。
- [ ] `flowable:` 属性映射表 100% 覆盖。
- [ ] 部署校验错误可定位到资源、元素、行号。
- [ ] 往返测试通过率 ≥ 95%。
- [ ] 1,725 个参考 BPMN 文件中，解析成功率 ≥ 98%，失败项有分类报告。
- [ ] 解析器通过 5 分钟 fuzz 无 panic。
- [ ] 模型缓存并发安全，部署后旧实例不受影响。
