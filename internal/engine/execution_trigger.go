package engine

import (
	"context"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/bpmn/model"
)

// TriggerExecution 触发等待中的执行（无事件定义的接收任务/中间捕获事件）。
// @param ctx 上下文
// @param executionID 执行 ID
// @param variables 写入变量
// @return 触发错误
func (e *Engine) TriggerExecution(ctx context.Context, executionID string, variables []api.Variable) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		execution, ok := e.findExecution(executionID)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "执行不存在", api.ErrNotFound)
		}
		instance, ok := e.findInstance(execution.ProcessInstanceID)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "流程实例不存在", api.ErrNotFound)
		}
		if err := e.allowTenant(cc.ctx, instance.TenantID); err != nil {
			return nil, err
		}
		if instance.Ended {
			return nil, api.NewError("ENGINE-0680", "流程实例已结束", api.ErrConflict)
		}
		if instance.Suspended {
			return nil, api.NewError("ENGINE-0681", "流程实例已挂起", api.ErrConflict)
		}
		if !execution.Waiting {
			return nil, api.NewError("ENGINE-0682", "执行不处于等待状态", api.ErrConflict)
		}
		definition, ok := e.findDefinition(execution.DefinitionID)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "流程定义不存在", api.ErrNotFound)
		}
		process := processOfDefinition(definition)
		if process == nil {
			return nil, api.NewError("ENGINE-0501", "流程定义不包含流程", nil)
		}
		element, ok := findElement(process, execution.ActivityID)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "当前活动不存在", api.ErrNotFound)
		}
		switch node := element.(type) {
		case *model.ReceiveTask:
			if node.MessageRef != "" || node.SignalRef != "" {
				return nil, api.NewError("ENGINE-0683", "接收任务需通过消息或信号触发", api.ErrBadRequest)
			}
		case *model.IntermediateCatchEvent:
			if node.MessageRef != "" || node.SignalRef != "" || node.Timer != nil || node.Condition != "" || node.LinkName != "" {
				return nil, api.NewError("ENGINE-0683", "中间捕获事件需通过对应事件触发", api.ErrBadRequest)
			}
		default:
			return nil, api.NewError("ENGINE-0684", "当前活动不支持 trigger", api.ErrBadRequest)
		}
		for _, variable := range variables {
			e.setVariable(execution, variable.Name, variable.Value)
		}
		e.recordActivityEnd(cc, execution.CurrentHistActivityID)
		execution.CurrentHistActivityID = ""
		execution.Waiting = false
		cc.agenda.schedule(&takeOutgoingOperation{executionID: execution.ID, activityID: execution.ActivityID})
		return nil, cc.agenda.run(cc)
	})
	return err
}
