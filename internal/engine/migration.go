package engine

import (
	"context"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/bpmn/model"
)

// MigrateProcessInstance 将运行中实例迁移到目标流程定义。
// @param ctx 上下文
// @param req 迁移请求
// @return 迁移错误
func (e *Engine) MigrateProcessInstance(ctx context.Context, req api.MigrationRequest) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		instance, ok := e.findInstance(req.ProcessInstanceID)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "流程实例不存在", api.ErrNotFound)
		}
		if err := e.allowTenant(ctx, instance.TenantID); err != nil {
			return nil, err
		}
		if instance.Ended {
			return nil, api.NewError("ENGINE-0640", "已结束实例不能迁移", api.ErrConflict)
		}
		target, ok := e.findDefinition(req.TargetProcessDefinitionID)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "目标流程定义不存在", api.ErrNotFound)
		}
		if err := e.allowTenant(ctx, target.Info.TenantID); err != nil {
			return nil, err
		}
		if target.Info.Suspended {
			return nil, api.NewError("ENGINE-0641", "目标流程定义已挂起", api.ErrConflict)
		}
		if instance.TenantID != target.Info.TenantID {
			return nil, api.NewError("ENGINE-0642", "不允许跨租户迁移", api.ErrForbidden)
		}
		targetProcess := processOfDefinition(target)
		if targetProcess == nil {
			return nil, api.NewError("ENGINE-0643", "目标流程定义不包含流程", nil)
		}
		// 活动定时器或订阅难以安全重映射，迁移前要求清理
		for _, job := range e.state.Jobs {
			if job.ProcessInstanceID == instance.ID {
				return nil, api.NewError("ENGINE-0644", "实例存在活动作业，暂不支持迁移", api.ErrConflict)
			}
		}
		for _, subscription := range e.state.Subscriptions {
			if subscription.ProcessInstanceID == instance.ID {
				return nil, api.NewError("ENGINE-0645", "实例存在事件订阅，暂不支持迁移", api.ErrConflict)
			}
		}
		sourceDefinition, sourceExists := e.findDefinition(instance.ProcessDefinitionID)
		if !sourceExists {
			return nil, api.NewError("ENGINE-0404", "源流程定义不存在", api.ErrNotFound)
		}
		sourceProcess := processOfDefinition(sourceDefinition)
		if sourceProcess == nil {
			return nil, api.NewError("ENGINE-0643", "源流程定义不包含流程", nil)
		}
		executions := make([]*executionState, 0)
		for _, execution := range e.state.Executions {
			if execution.ProcessInstanceID == instance.ID && !execution.Ended {
				executions = append(executions, execution)
			}
		}
		if len(executions) == 0 {
			return nil, api.NewError("ENGINE-0646", "实例没有可迁移的执行", api.ErrConflict)
		}
		for _, execution := range executions {
			sourceActivityID := execution.ActivityID
			if sourceActivityID == "" {
				continue
			}
			targetActivityID := sourceActivityID
			if mapped, exists := req.ActivityMappings[sourceActivityID]; exists {
				targetActivityID = mapped
			}
			sourceElement, sourceOK := findElement(sourceProcess, sourceActivityID)
			targetElement, targetOK := findElement(targetProcess, targetActivityID)
			if !sourceOK || !targetOK {
				return nil, api.NewError("ENGINE-0647", "活动不存在或目标活动缺失: "+sourceActivityID, api.ErrBadRequest)
			}
			if sourceElement.ElementType() != targetElement.ElementType() {
				return nil, api.NewError("ENGINE-0648", "活动类型不兼容: "+sourceActivityID, api.ErrBadRequest)
			}
			execution.ActivityID = targetActivityID
			execution.DefinitionID = target.Info.ID
		}
		for _, task := range e.state.Tasks {
			if task.Task.ProcessInstanceID != instance.ID {
				continue
			}
			targetActivityID := task.ActivityID
			if mapped, exists := req.ActivityMappings[task.ActivityID]; exists {
				targetActivityID = mapped
			}
			targetElement, exists := findElement(targetProcess, targetActivityID)
			if !exists {
				return nil, api.NewError("ENGINE-0647", "目标任务活动不存在: "+task.ActivityID, api.ErrBadRequest)
			}
			userTask, isUserTask := targetElement.(*model.UserTask)
			if !isUserTask {
				return nil, api.NewError("ENGINE-0649", "目标任务活动类型不兼容", api.ErrBadRequest)
			}
			task.ActivityID = targetActivityID
			task.Task.ProcessDefinitionID = target.Info.ID
			task.Task.TaskDefinitionKey = targetActivityID
			if userTask.Name != "" {
				task.Task.Name = userTask.Name
			}
		}
		instance.ProcessDefinitionID = target.Info.ID
		instance.ProcessDefinitionKey = target.Info.Key
		return nil, nil
	})
	return err
}
