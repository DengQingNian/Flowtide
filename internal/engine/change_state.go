package engine

import (
	"context"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/bpmn/model"
)

// ChangeActivityState 将执行移动到新的活动（仅支持单目标活动）。
// @param ctx 上下文
// @param executionID 执行 ID
// @param activityIDs 目标活动 ID 列表
// @return 迁移错误
func (e *Engine) ChangeActivityState(ctx context.Context, executionID string, activityIDs []string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		execution, ok := e.findExecution(executionID)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "执行不存在", api.ErrNotFound)
		}
		if execution.Ended {
			return nil, api.NewError("ENGINE-0660", "执行已结束", api.ErrConflict)
		}
		instance, ok := e.findInstance(execution.ProcessInstanceID)
		if !ok || instance.Ended {
			return nil, api.NewError("ENGINE-0660", "流程实例已结束", api.ErrConflict)
		}
		if err := e.allowTenant(cc.ctx, instance.TenantID); err != nil {
			return nil, err
		}
		if instance.Suspended {
			return nil, api.NewError("ENGINE-0661", "流程实例已挂起", api.ErrConflict)
		}
		if len(activityIDs) == 0 {
			return nil, api.NewError("ENGINE-0662", "目标活动不能为空", api.ErrBadRequest)
		}
		// 当前实现只支持单目标；多目标会引入并发作用域，留待后续扩展
		if len(activityIDs) > 1 {
			return nil, api.NewError("ENGINE-0663", "暂不支持多目标活动迁移", api.ErrBadRequest)
		}
		definition, ok := e.findDefinition(execution.DefinitionID)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "流程定义不存在", api.ErrNotFound)
		}
		process := processOfDefinition(definition)
		if process == nil {
			return nil, api.NewError("ENGINE-0501", "流程定义不包含流程", nil)
		}
		targetID := activityIDs[0]
		element, ok := findElement(process, targetID)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "目标活动不存在: "+targetID, api.ErrNotFound)
		}
		if _, isNode := element.(model.FlowNode); !isNode {
			return nil, api.NewError("ENGINE-0664", "目标活动不是流程节点", api.ErrBadRequest)
		}

		// 取消当前活动上的任务、边界定时器与事件订阅
		e.cancelExecutionTasks(cc, execution.ID, "change-state")
		e.cancelBoundaryTimers(execution.ID)
		for id, subscription := range e.state.Subscriptions {
			if subscription.ExecutionID == execution.ID {
				delete(e.state.Subscriptions, id)
			}
		}
		e.recordActivityEnd(cc, execution.CurrentHistActivityID)
		execution.CurrentHistActivityID = ""
		execution.Waiting = false
		execution.Active = true
		execution.ActivityID = targetID
		cc.agenda.schedule(&continueOperation{executionID: execution.ID, activityID: targetID})
		return nil, cc.agenda.run(cc)
	})
	return err
}

// ChangeProcessInstanceState 将实例根执行移动到新的活动。
// @param ctx 上下文
// @param processInstanceID 实例 ID
// @param activityIDs 目标活动 ID 列表
// @return 迁移错误
func (e *Engine) ChangeProcessInstanceState(ctx context.Context, processInstanceID string, activityIDs []string) error {
	if _, err := e.GetProcessInstance(ctx, processInstanceID); err != nil {
		return err
	}
	executions, err := e.ListExecutions(ctx, processInstanceID)
	if err != nil {
		return err
	}
	rootID := ""
	for _, execution := range executions {
		if execution.ParentID == "" && !execution.Ended {
			rootID = execution.ID
			break
		}
	}
	if rootID == "" {
		return api.NewError("ENGINE-0660", "实例没有可迁移的根执行", api.ErrConflict)
	}
	return e.ChangeActivityState(ctx, rootID, activityIDs)
}

// cancelExecutionTasks 取消执行上的运行中任务。
// @param cc 命令上下文
// @param executionID 执行 ID
// @param reason 取消原因
func (e *Engine) cancelExecutionTasks(cc *commandContext, executionID, reason string) {
	for id, task := range e.state.Tasks {
		if task.ExecutionID != executionID {
			continue
		}
		e.recordTaskLog(cc, task, "delete", "", reason)
		e.recordTaskEnd(cc, task)
		if historic, ok := e.state.HistoricTasks[task.HistoricTaskID]; ok {
			historic.DeleteReason = reason
		}
		delete(e.state.Tasks, id)
	}
}
