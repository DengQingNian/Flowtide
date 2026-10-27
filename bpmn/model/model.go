// Package model 定义 BPMN 2.0 的 Go 模型。
// 模型只承载结构与属性，不包含运行时状态。
package model

// BpmnModel 是 BPMN 文件的根模型。
type BpmnModel struct {
	Processes []*Process // 流程列表
}

// Process 表示一个可执行流程。
type Process struct {
	ID            string                 // 流程 ID
	Name          string                 // 流程名称
	Documentation string                 // 文档说明
	Executable    bool                   // 是否可执行
	Elements      map[string]FlowElement // 按 ID 索引的元素
	Order         []string               // 元素在文档中的顺序
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

// GetID 返回元素 ID。
// @return 元素 ID
func (b *BaseElement) GetID() string { return b.ID }

// GetName 返回元素名称。
// @return 元素名称
func (b *BaseElement) GetName() string { return b.Name }

// FlowNodeBase 提供流程节点的公共字段与方法。
type FlowNodeBase struct {
	BaseElement
	incoming []*SequenceFlow // 入线
	outgoing []*SequenceFlow // 出线
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
}

// StartEvent 表示开始事件。
type StartEvent struct {
	FlowNodeBase
	FormKey   string           // 启动表单 Key
	Initiator string           // 发起人变量名
	Timer     *TimerDefinition // 定时启动配置
}

// ElementType 返回元素类型。
// @return 元素类型
func (e *StartEvent) ElementType() string { return "startEvent" }

// EndEvent 表示结束事件。
type EndEvent struct {
	FlowNodeBase
	Terminate bool // 是否终止整个流程实例
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

// ParallelGateway 表示并行网关。
type ParallelGateway struct {
	FlowNodeBase
}

// ElementType 返回元素类型。
// @return 元素类型
func (g *ParallelGateway) ElementType() string { return "parallelGateway" }

// SubProcess 表示嵌入式子流程。
type SubProcess struct {
	FlowNodeBase
	Elements         map[string]FlowElement // 子元素
	Order            []string               // 子元素顺序
	TriggeredByEvent bool                   // 是否事件子流程
}

// ElementType 返回元素类型。
// @return 元素类型
func (s *SubProcess) ElementType() string { return "subProcess" }

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
	SourceRef string // 源节点 ID
	TargetRef string // 目标节点 ID
	Condition string // 条件表达式
	IsDefault bool   // 是否默认流
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
