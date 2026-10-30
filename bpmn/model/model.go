// Package model 定义 BPMN 2.0 的 Go 模型。
// 模型只承载结构与属性，不包含运行时状态。
package model

// BpmnModel 是 BPMN 文件的根模型。
type BpmnModel struct {
	Processes []*Process // 流程列表
	Messages  []*Message // 全局消息
	Signals   []*Signal  // 全局信号
	Errors    []*Error   // 全局错误
}

// Error 表示 BPMN 错误定义。
type Error struct {
	ID        string // 错误 ID
	ErrorCode string // 错误码
}

// Message 表示全局消息定义。
type Message struct {
	ID   string // 消息 ID
	Name string // 消息名
}

// Signal 表示全局信号定义。
type Signal struct {
	ID   string // 信号 ID
	Name string // 信号名
}

// Process 表示一个可执行流程。
type Process struct {
	ID                     string                        // 流程 ID
	Name                   string                        // 流程名称
	Documentation          string                        // 文档说明
	Executable             bool                          // 是否可执行
	CandidateStarterUsers  []string                      // 候选发起人用户
	CandidateStarterGroups []string                      // 候选发起人组
	ExecutionListeners     []ExecutionListenerDefinition // 流程级执行监听器
	Elements               map[string]FlowElement        // 按 ID 索引的元素
	Order                  []string                      // 元素在文档中的顺序
	Extensions             *ElementExtensions            // 流程级业务扩展属性（nil 表示无扩展）
}

// ExecutionListenerDefinition 是 BPMN 执行监听器定义。
type ExecutionListenerDefinition struct {
	Event              string // 事件：start/end
	ClassName          string // 类名（仅作为注册名兼容标记）
	DelegateExpression string // 委托表达式，作为注册名
	Expression         string // 表达式，暂不支持
}

// FlowElement 是所有 BPMN 元素的公共接口。
type FlowElement interface {
	// GetID 返回元素 ID。
	// @return 元素 ID
	GetID() string
	// GetName 返回元素名称。
	// @return 元素名称
	GetName() string
	// ElementType 返回元素类型。
	// @return 元素类型
	ElementType() string
}

// BaseElement 提供元素的通用字段。
type BaseElement struct {
	ID            string // 元素 ID
	Name          string // 元素名称
	Documentation string // 文档说明
}

// ExtensionField 是扩展元素中的单个字段值（对应 BPMN 扩展 <field name="x"> 的文本值）。
type ExtensionField struct {
	Name  string // 字段名（field 的 name 属性）
	Value string // 字段值（string/value/expression 子元素的文本）
}

// ElementExtensions 保留元素上的业务扩展属性与扩展字段。
// 二次封装（workflow）依赖 stepCode/weightType/executorList/direction 等非引擎语义
// 属性；解析器把"带命名空间前缀的属性"与"解析器未消费的未知属性"统一落到
// Attributes（键为去命名空间前缀的本地名），扩展元素中的 field 落到 Fields。
type ElementExtensions struct {
	Attributes map[string]string // 扩展属性：本地名 → 值
	Fields     []ExtensionField  // 扩展字段（<extensionElements> 内 field 元素）
}

// Attr 返回扩展属性值。
// @param local 去命名空间前缀的本地属性名
// @return 属性值，不存在时为空串
func (e *ElementExtensions) Attr(local string) string {
	if e == nil {
		return ""
	}
	return e.Attributes[local]
}

// Field 返回扩展字段值。
// @param name 字段名
// @return 字段值，不存在时为空串
func (e *ElementExtensions) Field(name string) string {
	if e == nil {
		return ""
	}
	for i := range e.Fields {
		if e.Fields[i].Name == name {
			return e.Fields[i].Value
		}
	}
	return ""
}

// GetID 返回元素 ID。
// @return 元素 ID
func (b *BaseElement) GetID() string { return b.ID }

// GetName 返回元素名称。
// @return 元素名称
func (b *BaseElement) GetName() string { return b.Name }

// FlowNodeBase 提供流程节点的公共字段与方法。
type FlowNodeBase struct {
	BaseElement
	incoming           []*SequenceFlow               // 入线
	outgoing           []*SequenceFlow               // 出线
	BoundaryEvents     []*BoundaryEvent              // 依附的边界事件
	AsyncBefore        bool                          // 进入前异步
	AsyncAfter         bool                          // 完成后异步
	Exclusive          bool                          // 是否独占作业
	ForCompensation    bool                          // 是否补偿处理器
	ExecutionListeners []ExecutionListenerDefinition // 节点级执行监听器
	Extensions         *ElementExtensions            // 业务扩展属性（nil 表示无扩展）
}

// GetIncoming 返回入线。
// @return 入线列表
func (n *FlowNodeBase) GetIncoming() []*SequenceFlow { return n.incoming }

// GetOutgoing 返回出线。
// @return 出线列表
func (n *FlowNodeBase) GetOutgoing() []*SequenceFlow { return n.outgoing }

// AddIncoming 添加入线。
// @param flow 顺序流
func (n *FlowNodeBase) AddIncoming(flow *SequenceFlow) { n.incoming = append(n.incoming, flow) }

// AddOutgoing 添加出线。
// @param flow 顺序流
func (n *FlowNodeBase) AddOutgoing(flow *SequenceFlow) { n.outgoing = append(n.outgoing, flow) }

// IsAsyncBefore 返回是否异步进入。
// @return 是否异步进入
func (n *FlowNodeBase) IsAsyncBefore() bool { return n.AsyncBefore }

// IsAsyncAfter 返回是否异步离开。
// @return 是否异步离开
func (n *FlowNodeBase) IsAsyncAfter() bool { return n.AsyncAfter }

// IsExclusive 返回是否独占作业。
// @return 是否独占
func (n *FlowNodeBase) IsExclusive() bool { return n.Exclusive }

// AddBoundaryEvent 添加边界事件。
// @param event 边界事件
func (n *FlowNodeBase) AddBoundaryEvent(event *BoundaryEvent) {
	n.BoundaryEvents = append(n.BoundaryEvents, event)
}

// GetBoundaryEvents 返回边界事件。
// @return 边界事件列表
func (n *FlowNodeBase) GetBoundaryEvents() []*BoundaryEvent { return n.BoundaryEvents }

// GetExecutionListeners 返回节点级执行监听器定义。
// @return 执行监听器列表
func (n *FlowNodeBase) GetExecutionListeners() []ExecutionListenerDefinition {
	return n.ExecutionListeners
}

// FlowNode 是可被令牌经过的节点。
type FlowNode interface {
	FlowElement
	// GetIncoming 返回入线。
	// @return 入线列表
	GetIncoming() []*SequenceFlow
	// GetOutgoing 返回出线。
	// @return 出线列表
	GetOutgoing() []*SequenceFlow
	// AddIncoming 添加入线。
	// @param flow 顺序流
	AddIncoming(flow *SequenceFlow)
	// AddOutgoing 添加出线。
	// @param flow 顺序流
	AddOutgoing(flow *SequenceFlow)
	// AddBoundaryEvent 添加边界事件。
	// @param event 边界事件
	AddBoundaryEvent(event *BoundaryEvent)
	// GetBoundaryEvents 返回边界事件。
	// @return 边界事件列表
	GetBoundaryEvents() []*BoundaryEvent
	// IsAsyncBefore 返回是否异步进入。
	// @return 是否异步进入
	IsAsyncBefore() bool
	// IsAsyncAfter 返回是否异步离开。
	// @return 是否异步离开
	IsAsyncAfter() bool
	// IsExclusive 返回是否独占作业。
	// @return 是否独占
	IsExclusive() bool
}

// FormValue 是表单枚举值。
type FormValue struct {
	ID   string // 值 ID
	Name string // 显示名
}

// FormProperty 是 BPMN 表单属性。
type FormProperty struct {
	ID         string       // 字段 ID
	Name       string       // 显示名
	Type       string       // 类型
	Required   bool         // 是否必填
	Readable   bool         // 是否可读
	Writable   bool         // 是否可写
	Value      string       // 默认值
	EnumValues []*FormValue // 枚举值
}

// StartEvent 表示开始事件。
type StartEvent struct {
	FlowNodeBase
	FormKey        string           // 启动表单 Key
	Initiator      string           // 发起人变量名
	Timer          *TimerDefinition // 定时启动配置
	MessageRef     string           // 消息引用
	SignalRef      string           // 信号引用
	Interrupting   bool             // 是否中断作用域
	FormProperties []*FormProperty  // 启动表单字段
}

// ElementType 返回元素类型。
// @return 元素类型
func (e *StartEvent) ElementType() string { return "startEvent" }

// EndEvent 表示结束事件。
type EndEvent struct {
	FlowNodeBase
	Terminate bool   // 是否终止整个流程实例
	Cancel    bool   // 是否取消事务子流程
	ErrorRef  string // 错误引用
}

// ElementType 返回元素类型。
// @return 元素类型
func (e *EndEvent) ElementType() string { return "endEvent" }

// MultiInstanceLoopCharacteristics 表示多实例配置。
type MultiInstanceLoopCharacteristics struct {
	Sequential          bool   // 是否串行
	Collection          string // 集合表达式
	ElementVariable     string // 元素变量名
	LoopCardinality     string // 实例数量表达式
	CompletionCondition string // 完成条件
}

// UserTask 表示用户任务。
type UserTask struct {
	FlowNodeBase
	LoopCharacteristics *MultiInstanceLoopCharacteristics // 多实例配置
	Assignee            string                            // 处理人表达式
	CandidateUsers      []string                          // 候选用户
	CandidateGroups     []string                          // 候选组
	DueDate             string                            // 到期时间表达式
	Priority            string                            // 优先级表达式
	FormKey             string                            // 表单 Key
	Category            string                            // 分类
	FormProperties      []*FormProperty                   // 表单字段
	TaskListeners       []TaskListenerDefinition          // 任务监听器
}

// TaskListenerDefinition 是 BPMN 任务监听器定义。
type TaskListenerDefinition struct {
	Event              string // 事件：create/complete/delete/assignment
	ClassName          string // 类名（兼容标记）
	DelegateExpression string // 委托名
	Expression         string // 表达式
}

// ElementType 返回元素类型。
// @return 元素类型
func (t *UserTask) ElementType() string { return "userTask" }

// ServiceTask 表示服务任务。
type ServiceTask struct {
	FlowNodeBase
	LoopCharacteristics *MultiInstanceLoopCharacteristics // 多实例配置
	ImplementationType  string                            // 实现类型：delegateExpression/expression/class
	DelegateExpression  string                            // 委托表达式
	Expression          string                            // 表达式
	ClassName           string                            // 类名（Java 兼容标记）
	ResultVariable      string                            // 结果变量名
}

// ElementType 返回元素类型。
// @return 元素类型
func (t *ServiceTask) ElementType() string { return "serviceTask" }

// BusinessRuleTask 表示业务规则任务，进入时求值部署内的 DMN 决策表。
type BusinessRuleTask struct {
	FlowNodeBase
	DecisionRef    string // 决策引用：decision id 或 decision table id
	ResultVariable string // 结果变量名，为空时按输出列写入多个变量
}

// ElementType 返回元素类型。
// @return 元素类型
func (t *BusinessRuleTask) ElementType() string { return "businessRuleTask" }

// ScriptTask 表示脚本任务。
type ScriptTask struct {
	FlowNodeBase
	ScriptFormat   string // 脚本格式
	Script         string // 脚本内容
	ResultVariable string // 结果变量名
}

// ElementType 返回元素类型。
// @return 元素类型
func (t *ScriptTask) ElementType() string { return "scriptTask" }

// ManualTask 表示手动任务。
type ManualTask struct {
	FlowNodeBase
}

// ElementType 返回元素类型。
// @return 元素类型
func (t *ManualTask) ElementType() string { return "manualTask" }

// ExclusiveGateway 表示排他网关。
type ExclusiveGateway struct {
	FlowNodeBase
	DefaultFlow string // 默认流出线 ID
}

// ElementType 返回元素类型。
// @return 元素类型
func (g *ExclusiveGateway) ElementType() string { return "exclusiveGateway" }

// Association 表示关联，用于补偿处理器绑定。
type Association struct {
	BaseElement
	SourceRef string // 源元素 ID
	TargetRef string // 目标元素 ID
}

// ElementType 返回元素类型。
// @return 元素类型
func (a *Association) ElementType() string { return "association" }

// IntermediateThrowEvent 表示中间抛出事件。
type IntermediateThrowEvent struct {
	FlowNodeBase
	Compensate bool   // 是否抛出补偿
	LinkName   string // 链接事件名称
}

// ElementType 返回元素类型。
// @return 元素类型
func (e *IntermediateThrowEvent) ElementType() string { return "intermediateThrowEvent" }

// EventGateway 表示事件网关。
type EventGateway struct {
	FlowNodeBase
}

// ElementType 返回元素类型。
// @return 元素类型
func (g *EventGateway) ElementType() string { return "eventGateway" }

// InclusiveGateway 表示包容网关。
type InclusiveGateway struct {
	FlowNodeBase
	DefaultFlow string // 默认流出线 ID
}

// ElementType 返回元素类型。
// @return 元素类型
func (g *InclusiveGateway) ElementType() string { return "inclusiveGateway" }

// ParallelGateway 表示并行网关。
type ParallelGateway struct {
	FlowNodeBase
}

// ElementType 返回元素类型。
// @return 元素类型
func (g *ParallelGateway) ElementType() string { return "parallelGateway" }

// IOParameter 表示调用活动的入参或出参映射。
type IOParameter struct {
	Source           string // 源变量
	Target           string // 目标变量
	SourceExpression string // 源表达式
}

// CallActivity 表示调用活动。
type CallActivity struct {
	FlowNodeBase
	CalledElement    string         // 被调用流程 Key 或表达式
	InheritVariables bool           // 是否继承父变量
	BusinessKey      string         // 子实例业务键表达式
	NameExpression   string         // 子实例名称表达式
	InParameters     []*IOParameter // 入参映射
	OutParameters    []*IOParameter // 出参映射
}

// ElementType 返回元素类型。
// @return 元素类型
func (c *CallActivity) ElementType() string { return "callActivity" }

// BoundaryEvent 表示边界事件。
type BoundaryEvent struct {
	FlowNodeBase
	AttachedToRef         string           // 依附活动 ID
	CancelActivity        bool             // 是否中断依附活动
	Timer                 *TimerDefinition // 定时器定义
	MessageRef            string           // 消息引用
	SignalRef             string           // 信号引用
	ErrorRef              string           // 错误引用
	Cancel                bool             // 是否取消边界
	Compensate            bool             // 是否补偿边界
	Condition             string           // 条件表达式
	CompensationHandlerID string           // 补偿处理器 ID
}

// ElementType 返回元素类型。
// @return 元素类型
func (e *BoundaryEvent) ElementType() string { return "boundaryEvent" }

// ReceiveTask 表示接收任务。
type ReceiveTask struct {
	FlowNodeBase
	MessageRef string // 消息引用
	SignalRef  string // 信号引用
}

// ElementType 返回元素类型。
// @return 元素类型
func (t *ReceiveTask) ElementType() string { return "receiveTask" }

// SendTask 表示发送任务。
type SendTask struct {
	FlowNodeBase
	MessageRef string // 消息引用
	SignalRef  string // 信号引用
}

// ElementType 返回元素类型。
// @return 元素类型
func (t *SendTask) ElementType() string { return "sendTask" }

// IntermediateCatchEvent 表示中间捕获事件。
type IntermediateCatchEvent struct {
	FlowNodeBase
	MessageRef string           // 消息引用
	SignalRef  string           // 信号引用
	Timer      *TimerDefinition // 定时器定义
	LinkName   string           // 链接事件名称
	Condition  string           // 条件表达式
}

// ElementType 返回元素类型。
// @return 元素类型
func (e *IntermediateCatchEvent) ElementType() string { return "intermediateCatchEvent" }

// SubProcess 表示嵌入式子流程。
type SubProcess struct {
	FlowNodeBase
	Elements            map[string]FlowElement // 子元素
	Order               []string               // 子元素顺序
	TriggeredByEvent    bool                   // 是否事件子流程
	Transaction         bool                   // 是否事务子流程
	AdHoc               bool                   // 是否 Ad-hoc 子流程
	AdHocOrdering       string                 // Ad-hoc 执行顺序：Parallel（默认）或 Sequential
	CompletionCondition string                 // Ad-hoc 完成条件表达式
}

// ElementType 返回元素类型。
// @return 元素类型
func (s *SubProcess) ElementType() string {
	if s.Transaction {
		return "transaction"
	}
	return "subProcess"
}

// AddElement 添加子元素。
// @param element 流程元素
func (s *SubProcess) AddElement(element FlowElement) {
	if s.Elements == nil {
		s.Elements = make(map[string]FlowElement)
	}
	s.Elements[element.GetID()] = element
	s.Order = append(s.Order, element.GetID())
}

// GetFlowNode 按 ID 获取子流程节点。
// @param id 元素 ID
// @return 流程节点
// @return 是否存在
func (s *SubProcess) GetFlowNode(id string) (FlowNode, bool) {
	element, ok := s.Elements[id]
	if !ok {
		return nil, false
	}
	node, ok := element.(FlowNode)
	return node, ok
}

// GetSequenceFlow 按 ID 获取子流程顺序流。
// @param id 元素 ID
// @return 顺序流
// @return 是否存在
func (s *SubProcess) GetSequenceFlow(id string) (*SequenceFlow, bool) {
	element, ok := s.Elements[id]
	if !ok {
		return nil, false
	}
	flow, ok := element.(*SequenceFlow)
	return flow, ok
}

// StartEvents 返回子流程开始事件。
// @return 开始事件列表
func (s *SubProcess) StartEvents() []*StartEvent {
	events := make([]*StartEvent, 0)
	for _, id := range s.Order {
		if event, ok := s.Elements[id].(*StartEvent); ok {
			events = append(events, event)
		}
	}
	return events
}

// SequenceFlow 表示顺序流。
type SequenceFlow struct {
	BaseElement
	SourceRef  string             // 源节点 ID
	TargetRef  string             // 目标节点 ID
	Condition  string             // 条件表达式
	IsDefault  bool               // 是否默认流
	Extensions *ElementExtensions // 业务扩展属性（nil 表示无扩展）
}

// ElementType 返回元素类型。
// @return 元素类型
func (f *SequenceFlow) ElementType() string { return "sequenceFlow" }

// TimerDefinition 表示定时器定义。
type TimerDefinition struct {
	TimeDate     string // 绝对时间
	TimeDuration string // 持续时间
	TimeCycle    string // 重复周期
}

// AddElement 添加元素到流程。
// @param element 流程元素
func (p *Process) AddElement(element FlowElement) {
	if p.Elements == nil {
		p.Elements = make(map[string]FlowElement)
	}
	p.Elements[element.GetID()] = element
	p.Order = append(p.Order, element.GetID())
}

// GetFlowNode 按 ID 获取流程节点。
// @param id 元素 ID
// @return 流程节点
// @return 是否存在
func (p *Process) GetFlowNode(id string) (FlowNode, bool) {
	element, ok := p.Elements[id]
	if !ok {
		return nil, false
	}
	node, ok := element.(FlowNode)
	return node, ok
}

// GetSequenceFlow 按 ID 获取顺序流。
// @param id 元素 ID
// @return 顺序流
// @return 是否存在
func (p *Process) GetSequenceFlow(id string) (*SequenceFlow, bool) {
	element, ok := p.Elements[id]
	if !ok {
		return nil, false
	}
	flow, ok := element.(*SequenceFlow)
	return flow, ok
}

// StartEvents 返回全部开始事件，按文档顺序。
// @return 开始事件列表
func (p *Process) StartEvents() []*StartEvent {
	events := make([]*StartEvent, 0)
	for _, id := range p.Order {
		if event, ok := p.Elements[id].(*StartEvent); ok {
			events = append(events, event)
		}
	}
	return events
}
