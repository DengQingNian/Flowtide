package engine

import (
	"context"
	"sort"

	"github.com/DengQingNian/Flowtide/api"
)

// ListExecutions 返回指定实例的执行树。
// @param ctx 上下文
// @param processInstanceID 流程实例 ID
// @return 执行列表
// @return 查询错误
func (e *Engine) ListExecutions(ctx context.Context, processInstanceID string) ([]api.ExecutionInfo, error) {
	result, err := e.read(func() (any, error) {
		out := make([]api.ExecutionInfo, 0)
		for _, execution := range e.state.Executions {
			if e.allowInstance(ctx, execution.ProcessInstanceID) != nil {
				continue
			}
			if processInstanceID != "" && execution.ProcessInstanceID != processInstanceID {
				continue
			}
			out = append(out, e.toExecutionInfo(execution))
		}
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]api.ExecutionInfo), nil
}

// GetExecution 查询执行。
// @param ctx 上下文
// @param executionID 执行 ID
// @return 执行信息
// @return 查询错误
func (e *Engine) GetExecution(ctx context.Context, executionID string) (api.ExecutionInfo, error) {
	result, err := e.read(func() (any, error) {
		execution, ok := e.findExecution(executionID)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "执行不存在", api.ErrNotFound)
		}
		if err := e.allowInstance(ctx, execution.ProcessInstanceID); err != nil {
			return nil, err
		}
		return e.toExecutionInfo(execution), nil
	})
	if err != nil {
		return api.ExecutionInfo{}, err
	}
	return result.(api.ExecutionInfo), nil
}

// GetHistoricProcessInstance 查询历史流程实例。
// @param ctx 上下文
// @param processInstanceID 实例 ID
// @return 历史实例
// @return 查询错误
func (e *Engine) GetHistoricProcessInstance(ctx context.Context, processInstanceID string) (api.HistoricProcessInstance, error) {
	result, err := e.read(func() (any, error) {
		historic, ok := e.state.HistoricProcesses[processInstanceID]
		if !ok {
			return nil, api.NewError("ENGINE-0404", "历史流程实例不存在", api.ErrNotFound)
		}
		if err := e.allowTenant(ctx, historic.TenantID); err != nil {
			return nil, err
		}
		return *historic, nil
	})
	if err != nil {
		return api.HistoricProcessInstance{}, err
	}
	return result.(api.HistoricProcessInstance), nil
}

// GetHistoricTaskInstance 查询历史任务。
// @param ctx 上下文
// @param taskID 任务 ID
// @return 历史任务
// @return 查询错误
func (e *Engine) GetHistoricTaskInstance(ctx context.Context, taskID string) (api.HistoricTaskInstance, error) {
	result, err := e.read(func() (any, error) {
		historic, ok := e.state.HistoricTasks[taskID]
		if !ok {
			return nil, api.NewError("ENGINE-0404", "历史任务不存在", api.ErrNotFound)
		}
		if err := e.allowHistoricResource(ctx, historic.ProcessInstanceID, historic.ID); err != nil {
			return nil, err
		}
		return *historic, nil
	})
	if err != nil {
		return api.HistoricTaskInstance{}, err
	}
	return result.(api.HistoricTaskInstance), nil
}

// toExecutionInfo 将执行状态转换为 DTO。
// @param execution 执行
// @return 执行信息
func (e *Engine) toExecutionInfo(execution *executionState) api.ExecutionInfo {
	info := api.ExecutionInfo{
		ID:                  execution.ID,
		ProcessInstanceID:   execution.ProcessInstanceID,
		ProcessDefinitionID: execution.DefinitionID,
		ParentID:            execution.ParentID,
		ActivityID:          execution.ActivityID,
		Active:              execution.Active,
		Ended:               execution.Ended,
		Waiting:             execution.Waiting,
	}
	// 租户及挂起状态以实例为准，上级调用引用仅属于实例根。
	if instance, ok := e.state.Instances[execution.ProcessInstanceID]; ok {
		info.ProcessDefinitionKey = instance.ProcessDefinitionKey
		info.TenantID = instance.TenantID
		info.Suspended = instance.Suspended
		if instance.RootExecutionID == execution.ID {
			info.SuperExecutionID = instance.SuperExecutionID
		}
	}
	return info
}

// GetExecutionVariables 返回执行可见变量。
// @param ctx 上下文
// @param executionID 执行 ID
// @return 变量快照
// @return 查询错误
func (e *Engine) GetExecutionVariables(ctx context.Context, executionID string) (map[string]any, error) {
	result, err := e.read(func() (any, error) {
		execution, ok := e.findExecution(executionID)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "执行不存在", api.ErrNotFound)
		}
		if err := e.allowInstance(ctx, execution.ProcessInstanceID); err != nil {
			return nil, err
		}
		return e.variablesOf(execution), nil
	})
	if err != nil {
		return nil, err
	}
	return result.(map[string]any), nil
}

// findActiveExecutionsByActivityIDs 按实例与活动 ID 集合查找未结束的执行（G8）。
// @param processInstanceID 流程实例 ID
// @param activityIDs 活动 ID 集合
// @return 执行列表（按 ID 排序）
func (e *Engine) findActiveExecutionsByActivityIDs(processInstanceID string, activityIDs map[string]bool) []*executionState {
	out := make([]*executionState, 0)
	for _, execution := range e.state.Executions {
		if execution.ProcessInstanceID != processInstanceID || execution.Ended {
			continue
		}
		if !activityIDs[execution.ActivityID] {
			continue
		}
		out = append(out, execution)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// getParentExecutionIDsByActivityID 按活动 ID 求承载执行的父执行 ID 集合（G8 多实例归并）。
// 对位 Java FlowUtils.getParentExecutionIdsByActivityId：多实例活动只取多实例根的父执行。
// @param executions 执行列表
// @param activityID 活动 ID
// @return 父执行 ID 集合，无命中时为 nil
func (e *Engine) getParentExecutionIDsByActivityID(executions []*executionState, activityID string) map[string]bool {
	matches := make([]*executionState, 0)
	for _, execution := range executions {
		if execution.ActivityID == activityID {
			matches = append(matches, execution)
		}
	}
	if len(matches) == 0 {
		return nil
	}
	// 多实例：只保留根执行的父执行，避免把子实例当独立分支
	roots := make([]*executionState, 0)
	for _, execution := range matches {
		if execution.MultiInstanceRoot {
			roots = append(roots, execution)
		}
	}
	if len(roots) > 0 {
		matches = roots
	}
	parents := map[string]bool{}
	for _, execution := range matches {
		if execution.ParentID != "" {
			parents[execution.ParentID] = true
		}
	}
	return parents
}
