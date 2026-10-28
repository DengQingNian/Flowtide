package engine

import (
	"context"
	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/bpmn/model"
	"sort"
)

// MessageEventReceived 投递消息，触发一个匹配订阅。
// @param ctx 上下文
// @param messageName 消息名
// @param variables 变量
// @return 触发数量
// @return 投递错误
func (e *Engine) MessageEventReceived(ctx context.Context, messageName string, variables []api.Variable) (int, error) {
	result, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		subscriptions := e.tenantSubscriptions(cc.ctx, "message", messageName)
		if len(subscriptions) == 0 {
			return 0, nil
		}
		// 消息是点对点投递，只触发第一个订阅
		if err := e.triggerSubscription(cc, subscriptions[0], variablesToMap(variables)); err != nil {
			return 0, err
		}
		if err := cc.agenda.run(cc); err != nil {
			return 0, err
		}
		return 1, nil
	})
	if err != nil {
		return 0, err
	}
	return result.(int), nil
}

// SignalEventReceived 广播信号，触发全部匹配订阅。
// @param ctx 上下文
// @param signalName 信号名
// @param variables 变量
// @return 触发数量
// @return 投递错误
func (e *Engine) SignalEventReceived(ctx context.Context, signalName string, variables []api.Variable) (int, error) {
	result, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		subscriptions := e.tenantSubscriptions(cc.ctx, "signal", signalName)
		if len(subscriptions) == 0 {
			return 0, nil
		}
		triggered := 0
		for _, subscription := range subscriptions {
			if err := e.triggerSubscription(cc, subscription, variablesToMap(variables)); err != nil {
				return 0, err
			}
			triggered++
		}
		if err := cc.agenda.run(cc); err != nil {
			return 0, err
		}
		return triggered, nil
	})
	if err != nil {
		return 0, err
	}
	return result.(int), nil
}

// subscriptionsFor 返回匹配的事件订阅，按创建时间排序。
// @param eventType 事件类型
// @param name 事件名
// @return 订阅列表
func (e *Engine) subscriptionsFor(eventType, name string) []*subscriptionState {
	out := make([]*subscriptionState, 0)
	for _, subscription := range e.state.Subscriptions {
		if subscription.EventType == eventType && subscription.EventName == name {
			out = append(out, subscription)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

// triggerSubscription 触发单个订阅。
// @param cc 命令上下文
// @param subscription 订阅
// @param variables 变量
// @return 触发错误
func (e *Engine) triggerSubscription(cc *commandContext, subscription *subscriptionState, variables map[string]any) error {
	if subscription.EventSubProcessID != "" {
		return e.triggerEventSubProcess(cc, subscription)
	}
	if subscription.GatewayGroup != "" {
		return e.triggerGatewaySubscription(cc, subscription, variables)
	}
	if subscription.BoundaryEventID != "" {
		return e.triggerBoundarySubscription(cc, subscription)
	}
	if subscription.StartEvent {
		_, err := e.startProcess(cc, api.StartRequest{
			ProcessDefinitionID: subscription.ProcessDefinitionID,
			TenantID:            subscription.TenantID,
			Variables:           mapToVariables(variables),
		})
		return err
	}
	execution, ok := e.findExecution(subscription.ExecutionID)
	if !ok || execution.Ended {
		delete(e.state.Subscriptions, subscription.ID)
		return nil
	}
	if instance, exists := e.findInstance(execution.ProcessInstanceID); exists && instance.Suspended {
		return nil
	}
	for name, value := range variables {
		e.setVariable(execution, name, value)
	}
	e.recordActivityEnd(cc, execution.CurrentHistActivityID)
	execution.CurrentHistActivityID = ""
	execution.Waiting = false
	delete(e.state.Subscriptions, subscription.ID)
	cc.agenda.schedule(&takeOutgoingOperation{executionID: execution.ID, activityID: subscription.ActivityID})
	return nil
}

// enterEventGateway 进入事件网关，为出线捕获事件创建竞争订阅。
// @param cc 命令上下文
// @param execution 执行
// @param gateway 事件网关
// @return 进入错误
func (e *Engine) enterEventGateway(cc *commandContext, execution *executionState, gateway *model.EventGateway) error {
	definition, ok := e.findDefinition(execution.DefinitionID)
	if !ok {
		return api.NewError("ENGINE-0404", "流程定义不存在", api.ErrNotFound)
	}
	process := processOfDefinition(definition)
	if process == nil {
		return api.NewError("ENGINE-0501", "流程定义不包含流程", nil)
	}
	group := execution.ID + ":" + gateway.ID
	created := 0
	for _, flow := range gateway.GetOutgoing() {
		element, exists := findElement(process, flow.TargetRef)
		if !exists {
			continue
		}
		catch, isCatch := element.(*model.IntermediateCatchEvent)
		if !isCatch {
			continue
		}
		eventType, name := e.eventTypeAndName(definition.Model, catch.MessageRef, catch.SignalRef)
		if eventType == "" {
			continue
		}
		subscription := &subscriptionState{
			ID:                  e.newID(),
			ProcessDefinitionID: definition.Info.ID,
			ProcessInstanceID:   execution.ProcessInstanceID,
			ExecutionID:         execution.ID,
			EventType:           eventType,
			EventName:           name,
			ActivityID:          catch.ID,
			GatewayGroup:        group,
			TenantID:            definition.Info.TenantID,
			CreatedAt:           cc.now,
		}
		e.state.Subscriptions[subscription.ID] = subscription
		created++
	}
	if created == 0 {
		return api.NewError("ENGINE-0630", "事件网关没有可等待的捕获事件", nil)
	}
	execution.Waiting = true
	execution.ActivityID = gateway.ID
	return nil
}

// triggerGatewaySubscription 触发事件网关的胜出分支。
// @param cc 命令上下文
// @param subscription 订阅
// @return 触发错误
// @param variables 变量映射
func (e *Engine) triggerGatewaySubscription(cc *commandContext, subscription *subscriptionState, variables map[string]any) error {
	// 取消同组其他竞争订阅
	for id, other := range e.state.Subscriptions {
		if other.GatewayGroup == subscription.GatewayGroup {
			delete(e.state.Subscriptions, id)
		}
	}
	execution, ok := e.findExecution(subscription.ExecutionID)
	if !ok || execution.Ended {
		return nil
	}
	for name, value := range variables {
		e.setVariable(execution, name, value)
	}
	execution.Waiting = false
	execution.ActivityID = subscription.ActivityID
	cc.agenda.schedule(&takeOutgoingOperation{executionID: execution.ID, activityID: subscription.ActivityID})
	return nil
}

// emitEvent 发送任务触发消息或信号。
// @param cc 命令上下文
// @param definition 流程定义
// @param messageRef 消息引用
// @param signalRef 信号引用
// @return 发送错误
func (e *Engine) emitEvent(cc *commandContext, definition *definitionState, messageRef, signalRef string) error {
	eventType, name := e.eventTypeAndName(definition.Model, messageRef, signalRef)
	if eventType == "" || name == "" {
		return api.NewError("ENGINE-0624", "发送任务缺少有效事件定义", nil)
	}
	subscriptions := e.subscriptionsFor(eventType, name)
	if len(subscriptions) == 0 {
		return nil
	}
	// 消息点对点，信号广播
	if eventType == "message" {
		return e.triggerSubscription(cc, subscriptions[0], map[string]any{})
	}
	for _, subscription := range subscriptions {
		if err := e.triggerSubscription(cc, subscription, map[string]any{}); err != nil {
			return err
		}
	}
	return nil
}

// triggerBoundarySubscription 触发边界消息/信号。
// @param cc 命令上下文
// @param subscription 订阅
// @return 触发错误
func (e *Engine) triggerBoundarySubscription(cc *commandContext, subscription *subscriptionState) error {
	return e.triggerBoundaryEvent(cc, subscription, false)
}

// triggerBoundaryEvent 触发边界事件。
// forceInterrupt 为 true 时忽略 cancelActivity 配置强制中断依附活动。
// 中断边界以及消息/信号边界触发后消费订阅；非中断条件边界保留订阅，
// 以便后续变量变化或显式求值再次触发。
// @param cc 命令上下文
// @param subscription 订阅
// @param forceInterrupt 是否强制中断
// @return 触发错误
func (e *Engine) triggerBoundaryEvent(cc *commandContext, subscription *subscriptionState, forceInterrupt bool) error {
	// 非中断条件订阅在触发后需要保留，其余订阅（含提前返回）一律消费
	keepSubscription := false
	defer func() {
		if !keepSubscription {
			delete(e.state.Subscriptions, subscription.ID)
		}
	}()
	execution, ok := e.findExecution(subscription.ExecutionID)
	if !ok || execution.Ended {
		return nil
	}
	instance, ok := e.findInstance(execution.ProcessInstanceID)
	if !ok || instance.Ended || instance.Suspended {
		return nil
	}
	definition, ok := e.findDefinition(execution.DefinitionID)
	if !ok {
		return nil
	}
	process := processOfDefinition(definition)
	if process == nil {
		return nil
	}
	element, ok := findElement(process, subscription.BoundaryEventID)
	if !ok {
		return nil
	}
	boundary, ok := element.(*model.BoundaryEvent)
	if !ok {
		return nil
	}
	if boundary.CancelActivity || forceInterrupt {
		for taskID, task := range e.state.Tasks {
			if task.ExecutionID != execution.ID {
				continue
			}
			e.recordTaskEnd(cc, task)
			delete(e.state.Tasks, taskID)
		}
		e.recordActivityEnd(cc, execution.CurrentHistActivityID)
		execution.CurrentHistActivityID = ""
		e.cancelBoundaryTimers(execution.ID)
		e.cancelBoundarySubscriptions(execution.ID)
		execution.Waiting = false
		execution.ActivityID = boundary.ID
		cc.agenda.schedule(&continueOperation{executionID: execution.ID, activityID: boundary.ID})
		return nil
	}
	// 非中断条件边界保留订阅，下次变量变化仍可触发；父执行继续等待原任务
	keepSubscription = subscription.EventType == conditionEventType
	child := &executionState{
		ID:                e.newID(),
		ProcessInstanceID: execution.ProcessInstanceID,
		DefinitionID:      execution.DefinitionID,
		ParentID:          execution.ID,
		ActivityID:        boundary.ID,
		Active:            true,
		Variables:         make(map[string]any),
	}
	e.state.Executions[child.ID] = child
	cc.agenda.schedule(&continueOperation{executionID: child.ID, activityID: boundary.ID})
	return nil
}

// cancelBoundarySubscriptions 取消执行上的边界消息/信号订阅。
// @param executionID 执行 ID
func (e *Engine) cancelBoundarySubscriptions(executionID string) {
	for id, subscription := range e.state.Subscriptions {
		if subscription.ExecutionID == executionID && subscription.BoundaryEventID != "" {
			delete(e.state.Subscriptions, id)
		}
	}
}

// createStartSubscriptions 为消息/信号开始事件创建订阅。
// @param cc 命令上下文
// @param definition 流程定义
// @param process 流程
func (e *Engine) createStartSubscriptions(cc *commandContext, definition *definitionState, process *model.Process) {
	for _, start := range process.StartEvents() {
		eventType, name := e.eventTypeAndName(definition.Model, start.MessageRef, start.SignalRef)
		if eventType == "" {
			continue
		}
		subscription := &subscriptionState{
			ID:                  e.newID(),
			ProcessDefinitionID: definition.Info.ID,
			EventType:           eventType,
			EventName:           name,
			ActivityID:          start.ID,
			StartEvent:          true,
			TenantID:            definition.Info.TenantID,
			CreatedAt:           cc.now,
		}
		e.state.Subscriptions[subscription.ID] = subscription
	}
}

// createExecutionSubscription 为等待中的执行创建订阅。
// @param cc 命令上下文
// @param execution 执行
// @param definition 流程定义
// @param messageRef 消息引用
// @param signalRef 信号引用
// @return 创建错误
func (e *Engine) createExecutionSubscription(cc *commandContext, execution *executionState, definition *definitionState, messageRef, signalRef string) error {
	eventType, name := e.eventTypeAndName(definition.Model, messageRef, signalRef)
	if eventType == "" {
		return api.NewError("ENGINE-0620", "事件缺少 messageRef 或 signalRef", nil)
	}
	if name == "" {
		return api.NewError("ENGINE-0621", "事件定义名称为空", nil)
	}
	subscription := &subscriptionState{
		ID:                  e.newID(),
		ProcessDefinitionID: definition.Info.ID,
		ProcessInstanceID:   execution.ProcessInstanceID,
		ExecutionID:         execution.ID,
		EventType:           eventType,
		EventName:           name,
		ActivityID:          execution.ActivityID,
		TenantID:            definition.Info.TenantID,
		CreatedAt:           cc.now,
	}
	e.state.Subscriptions[subscription.ID] = subscription
	return nil
}

// eventTypeAndName 解析事件类型与名称。
// @param bpmnModel 模型
// @param messageRef 消息引用
// @param signalRef 信号引用
// @return 事件类型
// @return 事件名称
func (e *Engine) eventTypeAndName(bpmnModel *model.BpmnModel, messageRef, signalRef string) (string, string) {
	if messageRef != "" {
		for _, message := range bpmnModel.Messages {
			if message.ID == messageRef {
				if message.Name != "" {
					return "message", message.Name
				}
				return "message", message.ID
			}
		}
		return "message", messageRef
	}
	if signalRef != "" {
		for _, signal := range bpmnModel.Signals {
			if signal.ID == signalRef {
				if signal.Name != "" {
					return "signal", signal.Name
				}
				return "signal", signal.ID
			}
		}
		return "signal", signalRef
	}
	return "", ""
}

// removeSubscriptionsForExecution 删除执行的事件订阅。
// @param executionID 执行 ID
func (e *Engine) removeSubscriptionsForExecution(executionID string) {
	for id, subscription := range e.state.Subscriptions {
		if subscription.ExecutionID == executionID {
			delete(e.state.Subscriptions, id)
		}
	}
}

// removeSubscriptionsForInstance 删除实例的事件订阅。
// @param instanceID 实例 ID
func (e *Engine) removeSubscriptionsForInstance(instanceID string) {
	for id, subscription := range e.state.Subscriptions {
		if subscription.ProcessInstanceID == instanceID {
			delete(e.state.Subscriptions, id)
		}
	}
}

// mapToVariables 将映射转换为变量列表。
// @param values 变量映射
// @return 变量列表
func mapToVariables(values map[string]any) []api.Variable {
	out := make([]api.Variable, 0, len(values))
	for name, value := range values {
		out = append(out, api.Variable{Name: name, Value: value})
	}
	return out
}

// tenantSubscriptions 返回当前租户可见的匹配事件订阅。
// @param ctx 上下文
// @param eventType 事件类型
// @param name 事件名
// @return 订阅列表
func (e *Engine) tenantSubscriptions(ctx context.Context, eventType, name string) []*subscriptionState {
	out := make([]*subscriptionState, 0)
	for _, subscription := range e.subscriptionsFor(eventType, name) {
		if e.allowTenant(ctx, e.subscriptionTenant(subscription)) == nil {
			out = append(out, subscription)
		}
	}
	return out
}
