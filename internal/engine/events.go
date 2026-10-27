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
		subscriptions := e.subscriptionsFor("message", messageName)
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
		subscriptions := e.subscriptionsFor("signal", signalName)
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
