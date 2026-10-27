package engine

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/bpmn/model"
	"github.com/flowable-go/flowable/internal/expr"
)

// StartProcessInstance 启动流程实例。
// @param ctx 上下文
// @param req 启动请求
// @return 流程实例
// @return 启动错误
func (e *Engine) StartProcessInstance(ctx context.Context, req api.StartRequest) (api.ProcessInstance, error) {
	result, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		return e.startProcess(cc, req)
	})
	if err != nil {
		return api.ProcessInstance{}, err
	}
	return result.(api.ProcessInstance), nil
}

// startProcess 在命令内启动流程实例。
// @param cc 命令上下文
// @param req 启动请求
// @return 流程实例 DTO
// @return 启动错误
func (e *Engine) startProcess(cc *commandContext, req api.StartRequest) (api.ProcessInstance, error) {
	var definition *definitionState
	var ok bool
	if req.ProcessDefinitionID != "" {
		definition, ok = e.findDefinition(req.ProcessDefinitionID)
	} else if req.ProcessDefinitionKey != "" {
		definition, ok = e.findLatestDefinition(req.ProcessDefinitionKey)
	} else {
		return api.ProcessInstance{}, api.NewError("ENGINE-0101", "必须指定流程定义 Key 或 ID", api.ErrBadRequest)
	}
	if !ok {
		return api.ProcessInstance{}, api.NewError("ENGINE-0404", "流程定义不存在", api.ErrNotFound)
	}
	if definition.Info.Suspended {
		return api.ProcessInstance{}, api.NewError("ENGINE-0102", "流程定义已挂起", api.ErrConflict)
	}
	if len(definition.Model.Processes) == 0 {
		return api.ProcessInstance{}, api.NewError("ENGINE-0103", "流程定义不包含流程", nil)
	}
	process := processOfDefinition(definition)
	if process == nil {
		return api.ProcessInstance{}, api.NewError("ENGINE-0103", "流程定义不包含流程", nil)
	}
	startEvent := firstStartEvent(process)
	if startEvent == nil {
		return api.ProcessInstance{}, api.NewError("ENGINE-0104", "流程缺少开始事件", nil)
	}

	instanceID := e.newID()
	executionID := e.newID()
	variables := variablesToMap(req.Variables)
	instance := &instanceState{
		ID:                   instanceID,
		ProcessDefinitionID:  definition.Info.ID,
		ProcessDefinitionKey: definition.Info.Key,
		BusinessKey:          req.BusinessKey,
		Name:                 req.Name,
		TenantID:             e.tenantOf(req.TenantID),
		StartTime:            cc.now,
		RootExecutionID:      executionID,
	}
	execution := &executionState{
		ID:                executionID,
		ProcessInstanceID: instanceID,
		DefinitionID:      definition.Info.ID,
		ActivityID:        startEvent.ID,
		Active:            true,
		Scope:             true,
		Variables:         variables,
	}
	e.state.Instances[instanceID] = instance
	e.state.Executions[executionID] = execution
	e.recordProcessStart(cc, instance)

	cc.agenda.schedule(&continueOperation{executionID: executionID, activityID: startEvent.ID})
	if err := cc.agenda.run(cc); err != nil {
		return api.ProcessInstance{}, err
	}
	return e.toProcessInstance(instance), nil
}

// firstStartEvent 返回第一个开始事件。
// @param process 流程
// @return 开始事件，不存在时为 nil
func firstStartEvent(process *model.Process) *model.StartEvent {
	events := process.StartEvents()
	if len(events) == 0 {
		return nil
	}
	return events[0]
}

// variablesToMap 将变量列表转换为映射。
// @param variables 变量列表
// @return 变量映射
func variablesToMap(variables []api.Variable) map[string]any {
	out := make(map[string]any, len(variables))
	for _, variable := range variables {
		out[variable.Name] = variable.Value
	}
	return out
}

// continueOperation 进入一个流程节点。
type continueOperation struct {
	executionID string // 执行 ID
	activityID  string // 活动 ID
}

// run 执行进入节点逻辑。
// @param cc 命令上下文
// @return 执行错误
func (op *continueOperation) run(cc *commandContext) error {
	return cc.engine.continueAt(cc, op.executionID, op.activityID)
}

// continueAt 让执行进入指定活动。
// @param cc 命令上下文
// @param executionID 执行 ID
// @param activityID 活动 ID
// @return 执行错误
func (e *Engine) continueAt(cc *commandContext, executionID, activityID string) error {
	execution, ok := e.findExecution(executionID)
	if !ok || execution.Ended || execution.Waiting {
		return nil
	}
	instance, ok := e.findInstance(execution.ProcessInstanceID)
	if !ok || instance.Ended {
		return nil
	}
	definition, ok := e.findDefinition(execution.DefinitionID)
	if !ok {
		return api.NewError("ENGINE-0404", "流程定义不存在", api.ErrNotFound)
	}
	process := processOfDefinition(definition)
	if process == nil {
		return api.NewError("ENGINE-0501", "流程定义不包含流程", nil)
	}
	element, ok := process.Elements[activityID]
	if !ok {
		return api.NewError("ENGINE-0501", "活动不存在: "+activityID, api.ErrNotFound)
	}
	execution.ActivityID = activityID
	execution.CurrentHistActivityID = e.recordActivityStart(cc, execution, element)
	switch node := element.(type) {
	case *model.StartEvent:
		e.recordActivityEnd(cc, execution.CurrentHistActivityID)
		execution.CurrentHistActivityID = ""
		return e.scheduleTakeOutgoing(cc, execution, node)
	case *model.EndEvent:
		e.recordActivityEnd(cc, execution.CurrentHistActivityID)
		execution.CurrentHistActivityID = ""
		e.endExecution(cc, execution, node.Terminate)
		return nil
	case *model.UserTask:
		return e.createUserTask(cc, execution, node)
	case *model.ServiceTask:
		if err := e.executeServiceTask(cc, execution, node); err != nil {
			return err
		}
		e.recordActivityEnd(cc, execution.CurrentHistActivityID)
		execution.CurrentHistActivityID = ""
		return e.scheduleTakeOutgoing(cc, execution, node)
	case *model.ScriptTask:
		return api.NewError("ENGINE-0701", "脚本任务暂未实现", nil)
	case *model.ManualTask:
		e.recordActivityEnd(cc, execution.CurrentHistActivityID)
		execution.CurrentHistActivityID = ""
		return e.scheduleTakeOutgoing(cc, execution, node)
	case *model.ExclusiveGateway:
		e.recordActivityEnd(cc, execution.CurrentHistActivityID)
		execution.CurrentHistActivityID = ""
		return e.scheduleTakeOutgoingExclusive(cc, execution, node)
	case *model.ParallelGateway:
		e.recordActivityEnd(cc, execution.CurrentHistActivityID)
		execution.CurrentHistActivityID = ""
		return e.handleParallelGateway(cc, execution, node)
	default:
		return api.NewError("ENGINE-0502", fmt.Sprintf("不支持的元素类型: %T", element), nil)
	}
}

// takeOutgoingOperation 处理节点出线。
type takeOutgoingOperation struct {
	executionID string // 执行 ID
	activityID  string // 活动 ID
	exclusive   bool   // 是否排他选择
}

// run 执行出线处理。
// @param cc 命令上下文
// @return 执行错误
func (op *takeOutgoingOperation) run(cc *commandContext) error {
	return cc.engine.takeOutgoing(cc, op.executionID, op.activityID, op.exclusive)
}

// scheduleTakeOutgoing 调度普通出线处理。
// @param cc 命令上下文
// @param execution 执行
// @param node 流程节点
// @return 错误
func (e *Engine) scheduleTakeOutgoing(cc *commandContext, execution *executionState, node model.FlowNode) error {
	cc.agenda.schedule(&takeOutgoingOperation{executionID: execution.ID, activityID: node.GetID()})
	return nil
}

// scheduleTakeOutgoingExclusive 调度排他网关出线处理。
// @param cc 命令上下文
// @param execution 执行
// @param node 网关
// @return 错误
func (e *Engine) scheduleTakeOutgoingExclusive(cc *commandContext, execution *executionState, node model.FlowNode) error {
	cc.agenda.schedule(&takeOutgoingOperation{executionID: execution.ID, activityID: node.GetID(), exclusive: true})
	return nil
}

// takeOutgoing 计算并执行出线。
// @param cc 命令上下文
// @param executionID 执行 ID
// @param activityID 活动 ID
// @param exclusive 是否排他
// @return 执行错误
func (e *Engine) takeOutgoing(cc *commandContext, executionID, activityID string, exclusive bool) error {
	execution, ok := e.findExecution(executionID)
	if !ok || execution.Ended {
		return nil
	}
	instance, ok := e.findInstance(execution.ProcessInstanceID)
	if !ok || instance.Ended {
		return nil
	}
	definition, ok := e.findDefinition(execution.DefinitionID)
	if !ok {
		return api.NewError("ENGINE-0404", "流程定义不存在", api.ErrNotFound)
	}
	process := processOfDefinition(definition)
	if process == nil {
		return api.NewError("ENGINE-0501", "流程定义不包含流程", nil)
	}
	node, ok := process.GetFlowNode(activityID)
	if !ok {
		return api.NewError("ENGINE-0501", "流程节点不存在: "+activityID, api.ErrNotFound)
	}
	flows := node.GetOutgoing()
	if len(flows) == 0 {
		e.endExecution(cc, execution, false)
		return nil
	}
	var selected []*model.SequenceFlow
	if exclusive {
		flow, err := e.selectExclusive(execution, flows)
		if err != nil {
			return err
		}
		selected = []*model.SequenceFlow{flow}
	} else {
		var err error
		selected, err = e.selectFlows(execution, flows)
		if err != nil {
			return err
		}
	}
	if len(selected) == 0 {
		return api.NewError("ENGINE-0503", "没有满足条件的出线: "+activityID, nil)
	}
	return e.dispatchFlows(cc, execution, selected)
}

// selectExclusive 选择排他网关的唯一出线。
// @param execution 执行
// @param flows 出线
// @return 选中的顺序流
// @return 选择错误
func (e *Engine) selectExclusive(execution *executionState, flows []*model.SequenceFlow) (*model.SequenceFlow, error) {
	var defaultFlow *model.SequenceFlow
	for _, flow := range flows {
		if flow.IsDefault {
			defaultFlow = flow
			continue
		}
		if strings.TrimSpace(flow.Condition) == "" {
			return flow, nil
		}
		matched, err := expr.EvalBool(flow.Condition, e.variablesOf(execution))
		if err != nil {
			return nil, api.NewError("ENGINE-0504", "条件表达式错误", err)
		}
		if matched {
			return flow, nil
		}
	}
	if defaultFlow != nil {
		return defaultFlow, nil
	}
	return nil, api.NewError("ENGINE-0503", "排他网关没有匹配的出线", nil)
}

// selectFlows 选择普通节点或并行网关的出线集合。
// @param execution 执行
// @param flows 出线
// @return 选中的顺序流
// @return 选择错误
func (e *Engine) selectFlows(execution *executionState, flows []*model.SequenceFlow) ([]*model.SequenceFlow, error) {
	var defaultFlow *model.SequenceFlow
	selected := make([]*model.SequenceFlow, 0, len(flows))
	for _, flow := range flows {
		if flow.IsDefault {
			defaultFlow = flow
			continue
		}
		if strings.TrimSpace(flow.Condition) == "" {
			selected = append(selected, flow)
			continue
		}
		matched, err := expr.EvalBool(flow.Condition, e.variablesOf(execution))
		if err != nil {
			return nil, api.NewError("ENGINE-0504", "条件表达式错误", err)
		}
		if matched {
			selected = append(selected, flow)
		}
	}
	if len(selected) == 0 && defaultFlow != nil {
		selected = append(selected, defaultFlow)
	}
	return selected, nil
}

// dispatchFlows 沿一条或多条出线推进。
// @param cc 命令上下文
// @param execution 当前执行
// @param flows 选中的出线
// @return 执行错误
func (e *Engine) dispatchFlows(cc *commandContext, execution *executionState, flows []*model.SequenceFlow) error {
	if len(flows) == 1 {
		target := flows[0].TargetRef
		execution.ActivityID = target
		cc.agenda.schedule(&continueOperation{executionID: execution.ID, activityID: target})
		return nil
	}
	// 多条出线：父执行等待，创建并发子执行
	execution.Active = false
	execution.PendingBranches = len(flows)
	for _, flow := range flows {
		child := &executionState{
			ID:                e.newID(),
			ProcessInstanceID: execution.ProcessInstanceID,
			DefinitionID:      execution.DefinitionID,
			ParentID:          execution.ID,
			ActivityID:        flow.TargetRef,
			Active:            true,
			Concurrent:        true,
			Variables:         make(map[string]any),
		}
		e.state.Executions[child.ID] = child
		cc.agenda.schedule(&continueOperation{executionID: child.ID, activityID: flow.TargetRef})
	}
	return nil
}

// handleParallelGateway 处理并行网关分叉与汇聚。
// @param cc 命令上下文
// @param execution 执行
// @param gateway 并行网关
// @return 执行错误
func (e *Engine) handleParallelGateway(cc *commandContext, execution *executionState, gateway *model.ParallelGateway) error {
	if len(gateway.GetIncoming()) > 1 && execution.ParentID != "" {
		return e.joinParallel(cc, execution)
	}
	return e.scheduleTakeOutgoing(cc, execution, gateway)
}

// joinParallel 执行并行汇聚。
// @param cc 命令上下文
// @param execution 到达网关的子执行
// @return 执行错误
func (e *Engine) joinParallel(cc *commandContext, execution *executionState) error {
	execution.Ended = true
	execution.Active = false
	parent, ok := e.findExecution(execution.ParentID)
	if !ok {
		return nil
	}
	parent.PendingBranches--
	if parent.PendingBranches > 0 {
		return nil
	}
	parent.Active = true
	parent.Waiting = false
	// 父执行直接从网关继续，不重复记录网关进入历史
	cc.agenda.schedule(&takeOutgoingOperation{executionID: parent.ID, activityID: execution.ActivityID})
	return nil
}

// endExecution 结束执行。
// @param cc 命令上下文
// @param execution 执行
// @param terminate 是否终止整个实例
func (e *Engine) endExecution(cc *commandContext, execution *executionState, terminate bool) {
	instance, ok := e.findInstance(execution.ProcessInstanceID)
	if !ok {
		return
	}
	if terminate {
		e.endProcessInstance(cc, instance)
		return
	}
	execution.Ended = true
	execution.Active = false
	execution.Waiting = false
	if execution.ID == instance.RootExecutionID {
		e.endProcessInstance(cc, instance)
		return
	}
	if execution.ParentID != "" {
		parent, parentOK := e.findExecution(execution.ParentID)
		if parentOK && parent.PendingBranches > 0 {
			parent.PendingBranches--
			if parent.PendingBranches == 0 {
				parent.Active = true
				if parent.ID == instance.RootExecutionID {
					e.endProcessInstance(cc, instance)
				}
			}
		}
	}
}

// endProcessInstance 结束流程实例。
// @param cc 命令上下文
// @param instance 实例
func (e *Engine) endProcessInstance(cc *commandContext, instance *instanceState) {
	if instance.Ended {
		return
	}
	instance.Ended = true
	instance.EndTime = cloneTime(&cc.now)
	for _, execution := range e.state.Executions {
		if execution.ProcessInstanceID == instance.ID {
			execution.Ended = true
			execution.Active = false
		}
	}
	for id, task := range e.state.Tasks {
		if task.Task.ProcessInstanceID != instance.ID {
			continue
		}
		e.recordTaskEnd(cc, task)
		delete(e.state.Tasks, id)
	}
	e.recordProcessEnd(cc, instance)
}

// executeServiceTask 执行服务任务。
// @param cc 命令上下文
// @param execution 执行
// @param task 服务任务
// @return 执行错误
func (e *Engine) executeServiceTask(cc *commandContext, execution *executionState, task *model.ServiceTask) error {
	var delegateName string
	switch task.ImplementationType {
	case "delegateExpression":
		delegateName = resolveDelegateName(task.DelegateExpression, e.variablesOf(execution))
	case "expression":
		value, err := expr.Eval(task.Expression, e.variablesOf(execution))
		if err != nil {
			return api.NewError("ENGINE-0506", "服务任务表达式求值失败", err)
		}
		if name, ok := value.(string); ok {
			if _, exists := e.delegates[name]; exists {
				delegateName = name
			}
		}
		if delegateName == "" {
			if task.ResultVariable != "" {
				e.setVariable(execution, task.ResultVariable, value)
				return nil
			}
			return api.NewError("ENGINE-0507", "表达式未返回已注册委托", nil)
		}
	case "class":
		return api.NewError("ENGINE-0508", "不支持 Java 类委托，请迁移为 Go 委托", nil)
	default:
		return api.NewError("ENGINE-0509", "服务任务缺少实现方式", nil)
	}
	delegate, ok := e.delegates[delegateName]
	if !ok {
		return api.NewError("ENGINE-0510", "委托未注册: "+delegateName, api.ErrNotFound)
	}
	if err := delegate(cc.ctx, &Execution{engine: e, state: execution, instance: e.state.Instances[execution.ProcessInstanceID]}); err != nil {
		return api.NewError("ENGINE-0511", "服务任务执行失败: "+delegateName, err)
	}
	return nil
}

// toProcessInstance 将状态转换为 DTO。
// @param instance 实例状态
// @return 流程实例 DTO
func (e *Engine) toProcessInstance(instance *instanceState) api.ProcessInstance {
	dto := api.ProcessInstance{
		ID:                   instance.ID,
		ProcessDefinitionID:  instance.ProcessDefinitionID,
		ProcessDefinitionKey: instance.ProcessDefinitionKey,
		BusinessKey:          instance.BusinessKey,
		Name:                 instance.Name,
		TenantID:             instance.TenantID,
		StartTime:            instance.StartTime,
		EndTime:              cloneTime(instance.EndTime),
		Ended:                instance.Ended,
		Suspended:            instance.Suspended,
	}
	if root, ok := e.rootExecution(instance); ok {
		dto.Variables = e.variablesOf(root)
	}
	return dto
}

// GetProcessInstance 查询流程实例。
// @param ctx 上下文
// @param id 实例 ID
// @return 流程实例
// @return 查询错误
func (e *Engine) GetProcessInstance(ctx context.Context, id string) (api.ProcessInstance, error) {
	result, err := e.read(func() (any, error) {
		instance, ok := e.findInstance(id)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "流程实例不存在", api.ErrNotFound)
		}
		return e.toProcessInstance(instance), nil
	})
	if err != nil {
		return api.ProcessInstance{}, err
	}
	return result.(api.ProcessInstance), nil
}

// ListProcessInstances 返回运行中的流程实例。
// @param ctx 上下文
// @return 实例列表
// @return 查询错误
func (e *Engine) ListProcessInstances(ctx context.Context) ([]api.ProcessInstance, error) {
	result, err := e.read(func() (any, error) {
		out := make([]api.ProcessInstance, 0)
		for _, instance := range e.state.Instances {
			out = append(out, e.toProcessInstance(instance))
		}
		sortInstances(out)
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]api.ProcessInstance), nil
}

// DeleteProcessInstance 删除流程实例。
// @param ctx 上下文
// @param id 实例 ID
// @param reason 删除原因
// @return 删除错误
func (e *Engine) DeleteProcessInstance(ctx context.Context, id, reason string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		instance, ok := e.findInstance(id)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "流程实例不存在", api.ErrNotFound)
		}
		instance.DeleteReason = reason
		for taskID, task := range e.state.Tasks {
			if task.Task.ProcessInstanceID != id {
				continue
			}
			e.recordTaskEnd(cc, task)
			delete(e.state.Tasks, taskID)
		}
		for _, execution := range e.state.Executions {
			if execution.ProcessInstanceID == id {
				execution.Ended = true
				execution.Active = false
			}
		}
		instance.Ended = true
		instance.EndTime = cloneTime(&cc.now)
		e.recordProcessEnd(cc, instance)
		return nil, nil
	})
	return err
}

// GetVariables 返回实例可见变量。
// @param ctx 上下文
// @param processInstanceID 实例 ID
// @return 变量快照
// @return 查询错误
func (e *Engine) GetVariables(ctx context.Context, processInstanceID string) (map[string]any, error) {
	result, err := e.read(func() (any, error) {
		instance, ok := e.findInstance(processInstanceID)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "流程实例不存在", api.ErrNotFound)
		}
		root, ok := e.rootExecution(instance)
		if !ok {
			return map[string]any{}, nil
		}
		return e.variablesOf(root), nil
	})
	if err != nil {
		return nil, err
	}
	return result.(map[string]any), nil
}

// SetVariable 设置流程实例变量。
// @param ctx 上下文
// @param processInstanceID 实例 ID
// @param name 变量名
// @param value 变量值
// @return 设置错误
func (e *Engine) SetVariable(ctx context.Context, processInstanceID, name string, value any) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		instance, ok := e.findInstance(processInstanceID)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "流程实例不存在", api.ErrNotFound)
		}
		root, ok := e.rootExecution(instance)
		if !ok {
			return nil, api.NewError("ENGINE-0405", "根执行不存在", nil)
		}
		e.setVariable(root, name, value)
		return nil, nil
	})
	return err
}

// evalInt 求值并转换为整数。
// @param raw 表达式
// @param vars 变量
// @return 整数
// @return 错误
func evalInt(raw string, vars map[string]any) (int, error) {
	value, err := expr.Eval(raw, vars)
	if err != nil {
		return 0, err
	}
	switch typed := value.(type) {
	case int:
		return typed, nil
	case int64:
		return int(typed), nil
	case float64:
		return int(typed), nil
	case string:
		return strconv.Atoi(typed)
	default:
		return 0, fmt.Errorf("无法转换为整数: %T", value)
	}
}

// parseDueDate 解析到期时间。
// @param raw 表达式
// @param vars 变量
// @return 到期时间，无法解析时为 nil
func parseDueDate(raw string, vars map[string]any) *time.Time {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	value, err := expr.EvalString(raw, vars)
	if err != nil || value == "" {
		return nil
	}
	layouts := []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"}
	for _, layout := range layouts {
		parsed, parseErr := time.Parse(layout, value)
		if parseErr == nil {
			return &parsed
		}
	}
	return nil
}

// processOfDefinition 返回流程定义对应的流程。
// @param definition 流程定义状态
// @return 流程
func processOfDefinition(definition *definitionState) *model.Process {
	if definition == nil || definition.Model == nil || len(definition.Model.Processes) == 0 {
		return nil
	}
	for _, process := range definition.Model.Processes {
		if process.ID == definition.Info.Key {
			return process
		}
	}
	return definition.Model.Processes[0]
}

// resolveDelegateName 解析委托表达式。
// 支持 ${beanName} 与直接名称；变量存在时优先使用变量值。
// @param raw 原始表达式
// @param vars 变量
// @return 委托名
func resolveDelegateName(raw string, vars map[string]any) string {
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "${") && strings.HasSuffix(trimmed, "}") {
		inner := strings.TrimSpace(trimmed[2 : len(trimmed)-1])
		if value, ok := vars[inner]; ok {
			if name, isString := value.(string); isString {
				return name
			}
		}
		return inner
	}
	return trimmed
}
