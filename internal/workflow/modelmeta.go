package workflow

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/bpmn/model"
)

// ExecutorConfig 描述 executorList 扩展属性中的单个执行者配置。
// 兼容 Java 设计器写入的 JSON 数组形态与逗号分隔的简单串。
type ExecutorConfig struct {
	IdentityID   string  `json:"identityId,omitempty"`   // 身份 Id（person 类型）
	ExecutorType string  `json:"executorType,omitempty"` // 执行者类型：person/org/role
	OrgID        string  `json:"orgId,omitempty"`        // 组织 Id（org 类型）
	RoleID       string  `json:"roleId,omitempty"`       // 角色 Id（role 类型）
	AgreeWeight  float64 `json:"agreeWeight,omitempty"`  // 通过权重（权重会签）
	RejectWeight float64 `json:"rejectWeight,omitempty"` // 拒绝权重（权重会签）
}

// MultiInstanceMeta 是节点的多实例（会签）配置摘要。
type MultiInstanceMeta struct {
	Sequential          bool   // 是否串行
	Collection          string // 集合表达式
	ElementVariable     string // 元素变量名
	CompletionCondition string // 完成条件表达式
}

// UserTaskMeta 是用户任务节点的模型元数据摘要，供执行人重写/推演/会签消费。
type UserTaskMeta struct {
	ActivityID               string             // 节点 ID（= TaskDefinitionKey）
	ActivityName             string             // 节点名称
	Assignee                 string             // 处理人表达式（原样）
	CandidateUsers           []string           // 候选用户表达式
	CandidateGroups          []string           // 候选组表达式
	StepCode                 string             // 步骤代号扩展属性
	ExecutorType             string             // 执行类型扩展属性（person/org/role）
	ExecutorNumType          string             // 执行数量类型扩展属性（single/multi）
	ExecutorListRaw          string             // executorList 原始串（JSON 数组或逗号分隔）
	WeightType               string             // 会签权重类型：average/custom
	WeightThreshold          string             // 权重阈值（表达式或数值）
	WeightListRaw            string             // weightList 原始串（按身份的权重 JSON）
	Documentation            string             // 文档说明
	MultiInstance            *MultiInstanceMeta // 多实例配置，非会签节点为 nil
	ExecutionListenerClasses []string           // 节点执行监听器注册名（部署校验用）
}

// ModelMetaService 读取部署定义的 BPMN 模型元数据。
// 全部经 api.RepositoryService().GetBpmnModel 走引擎内存权威模型，不自行解析 XML。
type ModelMetaService struct {
	engine api.Engine // 引擎门面
}

// NewModelMetaService 构造模型元数据服务。
// @param engine 引擎门面
// @return 模型元数据服务
func NewModelMetaService(engine api.Engine) *ModelMetaService {
	return &ModelMetaService{engine: engine}
}

// UserTaskMeta 返回指定定义上用户任务节点的元数据。
// @param ctx 上下文
// @param definitionID 流程定义 ID
// @param activityID 节点 ID（TaskDefinitionKey）
// @return 元数据
// @return 节点不存在或不是用户任务时返回错误
func (s *ModelMetaService) UserTaskMeta(ctx context.Context, definitionID, activityID string) (*UserTaskMeta, error) {
	process, err := s.processOf(ctx, definitionID)
	if err != nil {
		return nil, err
	}
	element, ok := findFlowElement(process, activityID)
	if !ok {
		return nil, api.NewError("WORKFLOW-1404", "流程定义不包含节点: "+activityID, api.ErrNotFound)
	}
	task, ok := element.(*model.UserTask)
	if !ok {
		return nil, api.NewError("WORKFLOW-1405", "节点不是用户任务: "+activityID, api.ErrBadRequest)
	}
	return userTaskMetaOf(task), nil
}

// FirstUserTask 返回从开始事件可达的第一个用户任务。
// 穿无条件网关；遇条件网关无法静态判定时报错（应改用推演器）。
// @param ctx 上下文
// @param definitionID 流程定义 ID
// @return 元数据
// @return 错误
func (s *ModelMetaService) FirstUserTask(ctx context.Context, definitionID string) (*UserTaskMeta, error) {
	process, err := s.processOf(ctx, definitionID)
	if err != nil {
		return nil, err
	}
	starts := process.StartEvents()
	if len(starts) == 0 {
		return nil, api.NewError("WORKFLOW-1406", "流程定义缺少开始事件", api.ErrNotFound)
	}
	visited := map[string]bool{}
	for _, start := range starts {
		meta := firstUserTaskFrom(process, start.GetID(), visited)
		if meta != nil {
			return meta, nil
		}
	}
	return nil, api.NewError("WORKFLOW-1407", "开始事件之后未找到用户任务", api.ErrNotFound)
}

// ListUserTaskMetas 返回定义上全部用户任务元数据（按文档顺序，含子流程内节点）。
// @param ctx 上下文
// @param definitionID 流程定义 ID
// @return 元数据列表
// @return 错误
func (s *ModelMetaService) ListUserTaskMetas(ctx context.Context, definitionID string) ([]*UserTaskMeta, error) {
	process, err := s.processOf(ctx, definitionID)
	if err != nil {
		return nil, err
	}
	metas := make([]*UserTaskMeta, 0)
	collectUserTaskMetas(process.Elements, process.Order, &metas)
	return metas, nil
}

// collectUserTaskMetas 递归收集容器内全部用户任务元数据（保持文档顺序）。
// @param elements 容器元素表
// @param order 容器元素顺序
// @param metas 收集目标
func collectUserTaskMetas(elements map[string]model.FlowElement, order []string, metas *[]*UserTaskMeta) {
	for _, id := range order {
		switch element := elements[id].(type) {
		case *model.UserTask:
			*metas = append(*metas, userTaskMetaOf(element))
		case *model.SubProcess:
			collectUserTaskMetas(element.Elements, element.Order, metas)
		}
	}
}

// NodeExtensions 返回任意节点（含连线）的原始扩展属性。
// @param ctx 上下文
// @param definitionID 流程定义 ID
// @param elementID 元素 ID
// @return 扩展集合，无扩展时为 nil
// @return 元素不存在时返回错误
func (s *ModelMetaService) NodeExtensions(ctx context.Context, definitionID, elementID string) (*model.ElementExtensions, error) {
	process, err := s.processOf(ctx, definitionID)
	if err != nil {
		return nil, err
	}
	element, ok := findFlowElement(process, elementID)
	if !ok {
		return nil, api.NewError("WORKFLOW-1404", "流程定义不包含元素: "+elementID, api.ErrNotFound)
	}
	switch typed := element.(type) {
	case *model.SequenceFlow:
		return typed.Extensions, nil
	case interface{ GetID() string }:
		// 流程节点：扩展挂在 FlowNodeBase 上
		return extensionsOfNode(element), nil
	default:
		return nil, nil
	}
}

// processOf 查询定义对应的第一个流程。
// @param ctx 上下文
// @param definitionID 流程定义 ID
// @return 流程
// @return 错误
func (s *ModelMetaService) processOf(ctx context.Context, definitionID string) (*model.Process, error) {
	bpmn, err := s.engine.RepositoryService().GetBpmnModel(ctx, definitionID)
	if err != nil {
		return nil, err
	}
	if len(bpmn.Processes) == 0 {
		return nil, api.NewError("WORKFLOW-1404", "流程定义不包含流程", api.ErrNotFound)
	}
	return bpmn.Processes[0], nil
}

// userTaskMetaOf 从模型节点提取元数据摘要。
// @param task 用户任务节点
// @return 元数据
func userTaskMetaOf(task *model.UserTask) *UserTaskMeta {
	extensions := task.Extensions
	meta := &UserTaskMeta{
		ActivityID:      task.ID,
		ActivityName:    task.Name,
		Assignee:        task.Assignee,
		CandidateUsers:  append([]string(nil), task.CandidateUsers...),
		CandidateGroups: append([]string(nil), task.CandidateGroups...),
		Documentation:   task.Documentation,
	}
	if extensions != nil {
		meta.StepCode = extensions.Attr("stepCode")
		meta.ExecutorType = extensions.Attr("executorType")
		meta.ExecutorNumType = extensions.Attr("executorNumType")
		meta.ExecutorListRaw = extensions.Attr("executorList")
		meta.WeightType = extensions.Attr("weightType")
		meta.WeightThreshold = extensions.Attr("weightThreshold")
		meta.WeightListRaw = extensions.Attr("weightList")
	}
	if task.LoopCharacteristics != nil {
		meta.MultiInstance = &MultiInstanceMeta{
			Sequential:          task.LoopCharacteristics.Sequential,
			Collection:          task.LoopCharacteristics.Collection,
			ElementVariable:     task.LoopCharacteristics.ElementVariable,
			CompletionCondition: task.LoopCharacteristics.CompletionCondition,
		}
	}
	// 执行监听器注册名（class 或 delegateExpression），供部署强校验比对
	for _, listener := range task.ExecutionListeners {
		name := listener.ClassName
		if name == "" {
			name = listener.DelegateExpression
		}
		if name != "" {
			meta.ExecutionListenerClasses = append(meta.ExecutionListenerClasses, name)
		}
	}
	return meta
}

// extensionsOfNode 读取流程节点上的扩展属性。
// FlowNodeBase 以值嵌入各节点结构，这里经反射取嵌入基座的 Extensions 字段。
// @param element 流程元素
// @return 扩展集合，无扩展时为 nil
func extensionsOfNode(element model.FlowElement) *model.ElementExtensions {
	value := reflect.Indirect(reflect.ValueOf(element))
	if value.Kind() != reflect.Struct {
		return nil
	}
	base := value.FieldByName("FlowNodeBase")
	if !base.IsValid() {
		return nil
	}
	extensions := base.FieldByName("Extensions")
	if !extensions.IsValid() || extensions.IsNil() {
		return nil
	}
	if pointer, ok := extensions.Interface().(*model.ElementExtensions); ok {
		return pointer
	}
	return nil
}

// findFlowElement 在流程（含子流程）中按 ID 查找元素。
// @param process 流程
// @param id 元素 ID
// @return 元素
// @return 是否找到
func findFlowElement(process *model.Process, id string) (model.FlowElement, bool) {
	if element, ok := process.Elements[id]; ok {
		return element, true
	}
	for _, element := range process.Elements {
		if sub, ok := element.(*model.SubProcess); ok {
			if found, ok := findFlowElementInSub(sub, id); ok {
				return found, true
			}
		}
	}
	return nil, false
}

// findFlowElementInSub 在子流程（含嵌套）中按 ID 查找元素。
// @param sub 子流程
// @param id 元素 ID
// @return 元素
// @return 是否找到
func findFlowElementInSub(sub *model.SubProcess, id string) (model.FlowElement, bool) {
	if element, ok := sub.Elements[id]; ok {
		return element, true
	}
	for _, element := range sub.Elements {
		if nested, ok := element.(*model.SubProcess); ok {
			if found, ok := findFlowElementInSub(nested, id); ok {
				return found, true
			}
		}
	}
	return nil, false
}

// firstUserTaskFrom 从指定节点出发沿出线寻找第一个用户任务。
// 穿并行/排他无条件网关与中间事件；条件网关按出线逐条尝试（无法判定时跳过该分支）。
// @param process 流程
// @param nodeID 起始节点 ID
// @param visited 访问标记（防环）
// @return 首个用户任务元数据，找不到为 nil
func firstUserTaskFrom(process *model.Process, nodeID string, visited map[string]bool) *UserTaskMeta {
	// 防止环形图导致的无限递归
	if visited[nodeID] {
		return nil
	}
	visited[nodeID] = true
	element, ok := findFlowElement(process, nodeID)
	if !ok {
		return nil
	}
	node, ok := element.(model.FlowNode)
	if !ok {
		return nil
	}
	if task, ok := node.(*model.UserTask); ok {
		return userTaskMetaOf(task)
	}
	// 子流程：先找子流程内部第一个用户任务
	if sub, ok := node.(*model.SubProcess); ok {
		for _, id := range sub.Order {
			if inner, ok := sub.Elements[id].(*model.UserTask); ok {
				return userTaskMetaOf(inner)
			}
		}
	}
	for _, flow := range node.GetOutgoing() {
		if meta := firstUserTaskFrom(process, flow.TargetRef, visited); meta != nil {
			return meta
		}
	}
	return nil
}

// ParseExecutorConfigs 解析 executorList 原始串。
// 支持 JSON 数组形态（对象含 identityId/executorType/orgId/roleId/权重）与逗号分隔
// 的身份 Id 串（退化为 person 类型配置）。
// @param raw 原始串
// @return 执行者配置列表，空串返回 nil
func ParseExecutorConfigs(raw string) []ExecutorConfig {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var configs []ExecutorConfig
		if err := json.Unmarshal([]byte(trimmed), &configs); err == nil {
			return configs
		}
		// JSON 数组形态解析失败时回退逗号拆分，保证兼容手写属性
	}
	configs := make([]ExecutorConfig, 0)
	for _, part := range strings.Split(trimmed, ",") {
		if id := strings.TrimSpace(part); id != "" {
			configs = append(configs, ExecutorConfig{IdentityID: id, ExecutorType: "person"})
		}
	}
	return configs
}

// ParseWeightList 解析 weightList 原始串（JSON 数组，按身份配置权重）。
// @param raw 原始串
// @return 执行者权重配置列表，空串或解析失败返回 nil
func ParseWeightList(raw string) []ExecutorConfig {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || !strings.HasPrefix(trimmed, "[") {
		return nil
	}
	var configs []ExecutorConfig
	if err := json.Unmarshal([]byte(trimmed), &configs); err != nil {
		return nil
	}
	return configs
}
