package engine

import (
	"context"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/bpmn/model"
)

// executionMigration 是执行迁移目标。
type executionMigration struct {
	execution        *executionState // 执行
	targetActivityID string          // 目标活动 ID
}

// taskMigration 是任务迁移目标。
type taskMigration struct {
	task             *taskState // 任务
	targetActivityID string     // 目标活动 ID
	name             string     // 目标名称
}

// migrationPlan 是迁移校验后的变更计划。
type migrationPlan struct {
	instance   *instanceState       // 实例
	target     *definitionState     // 目标定义
	executions []executionMigration // 执行变更
	tasks      []taskMigration      // 任务变更
}

// MigrateProcessInstance 将运行中实例迁移到目标流程定义。
// @param ctx 上下文
// @param req 迁移请求
// @return 迁移错误
func (e *Engine) MigrateProcessInstance(ctx context.Context, req api.MigrationRequest) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		plan, messages, err := e.prepareMigration(cc.ctx, req)
		if err != nil {
			return nil, err
		}
		if len(messages) > 0 {
			return nil, api.NewError("ENGINE-0650", messages[0], api.ErrBadRequest)
		}
		e.applyMigration(plan)
		return nil, nil
	})
	return err
}

// ValidateMigration 校验实例迁移，不修改状态。
// @param ctx 上下文
// @param req 迁移请求
// @return 校验消息，空列表表示可迁移
// @return 查询错误
func (e *Engine) ValidateMigration(ctx context.Context, req api.MigrationRequest) ([]string, error) {
	result, err := e.read(func() (any, error) {
		_, messages, prepareErr := e.prepareMigration(ctx, req)
		if prepareErr != nil {
			return nil, prepareErr
		}
		return messages, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]string), nil
}

// prepareMigration 校验迁移并生成变更计划。
// @param ctx 上下文
// @param req 迁移请求
// @return 变更计划
// @return 校验消息
// @return 结构性错误
func (e *Engine) prepareMigration(ctx context.Context, req api.MigrationRequest) (*migrationPlan, []string, error) {
	instance, ok := e.findInstance(req.ProcessInstanceID)
	if !ok {
		return nil, nil, api.NewError("ENGINE-0404", "流程实例不存在", api.ErrNotFound)
	}
	if err := e.allowTenant(ctx, instance.TenantID); err != nil {
		return nil, nil, err
	}
	if instance.Ended {
		return nil, nil, api.NewError("ENGINE-0640", "已结束实例不能迁移", api.ErrConflict)
	}
	target, ok := e.findDefinition(req.TargetProcessDefinitionID)
	if !ok {
		return nil, nil, api.NewError("ENGINE-0404", "目标流程定义不存在", api.ErrNotFound)
	}
	if err := e.allowTenant(ctx, target.Info.TenantID); err != nil {
		return nil, nil, err
	}
	if target.Info.Suspended {
		return nil, nil, api.NewError("ENGINE-0641", "目标流程定义已挂起", api.ErrConflict)
	}
	if instance.TenantID != target.Info.TenantID {
		return nil, nil, api.NewError("ENGINE-0642", "不允许跨租户迁移", api.ErrForbidden)
	}
	targetProcess := processOfDefinition(target)
	if targetProcess == nil {
		return nil, nil, api.NewError("ENGINE-0643", "目标流程定义不包含流程", nil)
	}
	sourceDefinition, sourceExists := e.findDefinition(instance.ProcessDefinitionID)
	if !sourceExists {
		return nil, nil, api.NewError("ENGINE-0404", "源流程定义不存在", api.ErrNotFound)
	}
	sourceProcess := processOfDefinition(sourceDefinition)
	if sourceProcess == nil {
		return nil, nil, api.NewError("ENGINE-0643", "源流程定义不包含流程", nil)
	}

	messages := make([]string, 0)
	// 活动定时器或订阅难以安全重映射，迁移前要求清理
	for _, job := range e.state.Jobs {
		if job.ProcessInstanceID == instance.ID {
			messages = append(messages, "实例存在活动作业，暂不支持迁移")
			break
		}
	}
	for _, subscription := range e.state.Subscriptions {
		if subscription.ProcessInstanceID == instance.ID {
			messages = append(messages, "实例存在事件订阅，暂不支持迁移")
			break
		}
	}

	plan := &migrationPlan{instance: instance, target: target, executions: make([]executionMigration, 0), tasks: make([]taskMigration, 0)}
	executions := make([]*executionState, 0)
	for _, execution := range e.state.Executions {
		if execution.ProcessInstanceID == instance.ID && !execution.Ended {
			executions = append(executions, execution)
		}
	}
	if len(executions) == 0 {
		messages = append(messages, "实例没有可迁移的执行")
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
			messages = append(messages, "活动不存在或目标活动缺失: "+sourceActivityID)
			continue
		}
		if sourceElement.ElementType() != targetElement.ElementType() {
			messages = append(messages, "活动类型不兼容: "+sourceActivityID)
			continue
		}
		plan.executions = append(plan.executions, executionMigration{execution: execution, targetActivityID: targetActivityID})
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
			messages = append(messages, "目标任务活动不存在: "+task.ActivityID)
			continue
		}
		userTask, isUserTask := targetElement.(*model.UserTask)
		if !isUserTask {
			messages = append(messages, "目标任务活动类型不兼容")
			continue
		}
		plan.tasks = append(plan.tasks, taskMigration{task: task, targetActivityID: targetActivityID, name: userTask.Name})
	}
	return plan, messages, nil
}

// applyMigration 应用迁移计划。
// @param plan 变更计划
func (e *Engine) applyMigration(plan *migrationPlan) {
	for _, item := range plan.executions {
		item.execution.ActivityID = item.targetActivityID
		item.execution.DefinitionID = plan.target.Info.ID
	}
	for _, item := range plan.tasks {
		item.task.ActivityID = item.targetActivityID
		item.task.Task.ProcessDefinitionID = plan.target.Info.ID
		item.task.Task.TaskDefinitionKey = item.targetActivityID
		if item.name != "" {
			item.task.Task.Name = item.name
		}
	}
	plan.instance.ProcessDefinitionID = plan.target.Info.ID
	plan.instance.ProcessDefinitionKey = plan.target.Info.Key
}
