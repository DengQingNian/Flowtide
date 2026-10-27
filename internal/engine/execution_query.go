package engine

import (
	"context"
	"sort"

	"github.com/flowable-go/flowable/api"
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
			if processInstanceID != "" && execution.ProcessInstanceID != processInstanceID {
				continue
			}
			out = append(out, toExecutionInfo(execution))
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
		return toExecutionInfo(execution), nil
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
func toExecutionInfo(execution *executionState) api.ExecutionInfo {
	return api.ExecutionInfo{
		ID:                  execution.ID,
		ProcessInstanceID:   execution.ProcessInstanceID,
		ProcessDefinitionID: execution.DefinitionID,
		ParentID:            execution.ParentID,
		ActivityID:          execution.ActivityID,
		Active:              execution.Active,
		Ended:               execution.Ended,
		Waiting:             execution.Waiting,
	}
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
