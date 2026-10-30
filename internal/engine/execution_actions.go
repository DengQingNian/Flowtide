package engine

import (
	"context"

	"github.com/DengQingNian/Flowtide/api"
)

// MessageEventReceivedForExecution 向指定执行投递消息。
// @param ctx 上下文
// @param executionID 执行 ID
// @param messageName 消息名
// @param variables 变量
// @return 投递错误
func (e *Engine) MessageEventReceivedForExecution(ctx context.Context, executionID, messageName string, variables []api.Variable) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		return nil, e.deliverToExecution(cc, executionID, "message", messageName, variables)
	})
	return err
}

// SignalEventReceivedForExecution 向指定执行投递信号。
// @param ctx 上下文
// @param executionID 执行 ID
// @param signalName 信号名
// @param variables 变量
// @return 投递错误
func (e *Engine) SignalEventReceivedForExecution(ctx context.Context, executionID, signalName string, variables []api.Variable) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		return nil, e.deliverToExecution(cc, executionID, "signal", signalName, variables)
	})
	return err
}

// deliverToExecution 向执行上的匹配订阅投递事件。
// @param cc 命令上下文
// @param executionID 执行 ID
// @param eventType 事件类型
// @param eventName 事件名
// @param variables 变量
// @return 投递错误
func (e *Engine) deliverToExecution(cc *commandContext, executionID, eventType, eventName string, variables []api.Variable) error {
	execution, ok := e.findExecution(executionID)
	if !ok {
		return api.NewError("ENGINE-0404", "执行不存在", api.ErrNotFound)
	}
	instance, ok := e.findInstance(execution.ProcessInstanceID)
	if !ok {
		return api.NewError("ENGINE-0404", "流程实例不存在", api.ErrNotFound)
	}
	if err := e.allowTenant(cc.ctx, instance.TenantID); err != nil {
		return err
	}
	if instance.Ended {
		return api.NewError("ENGINE-0675", "流程实例已结束", api.ErrConflict)
	}
	var matched *subscriptionState
	for _, subscription := range e.subscriptionsFor(eventType, eventName) {
		if subscription.ExecutionID == executionID {
			matched = subscription
			break
		}
	}
	if matched == nil {
		return api.NewError("ENGINE-0676", "执行上的事件订阅不存在", api.ErrNotFound)
	}
	if err := e.triggerSubscription(cc, matched, variablesToMap(variables)); err != nil {
		return err
	}
	return cc.agenda.run(cc)
}
