package engine

import (
	"context"
	"sort"

	"github.com/flowable-go/flowable/api"
)

// ListHistoricProcessInstances 返回历史流程实例。
// @param ctx 上下文
// @return 历史实例列表
// @return 查询错误
func (e *Engine) ListHistoricProcessInstances(ctx context.Context) ([]api.HistoricProcessInstance, error) {
	result, err := e.read(func() (any, error) {
		out := make([]api.HistoricProcessInstance, 0, len(e.state.HistoricProcesses))
		for _, historic := range e.state.HistoricProcesses {
			out = append(out, *historic)
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].StartTime.Equal(out[j].StartTime) {
				return out[i].ID < out[j].ID
			}
			return out[i].StartTime.Before(out[j].StartTime)
		})
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]api.HistoricProcessInstance), nil
}

// ListHistoricActivityInstances 返回历史活动实例。
// @param ctx 上下文
// @param processInstanceID 流程实例 ID
// @return 历史活动列表
// @return 查询错误
func (e *Engine) ListHistoricActivityInstances(ctx context.Context, processInstanceID string) ([]api.HistoricActivityInstance, error) {
	result, err := e.read(func() (any, error) {
		out := make([]api.HistoricActivityInstance, 0)
		for _, historic := range e.state.HistoricActivities {
			if processInstanceID != "" && historic.ProcessInstanceID != processInstanceID {
				continue
			}
			out = append(out, *historic)
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].StartTime.Equal(out[j].StartTime) {
				return out[i].ID < out[j].ID
			}
			return out[i].StartTime.Before(out[j].StartTime)
		})
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]api.HistoricActivityInstance), nil
}

// ListHistoricTaskInstances 返回历史任务实例。
// @param ctx 上下文
// @param processInstanceID 流程实例 ID
// @return 历史任务列表
// @return 查询错误
func (e *Engine) ListHistoricTaskInstances(ctx context.Context, processInstanceID string) ([]api.HistoricTaskInstance, error) {
	result, err := e.read(func() (any, error) {
		out := make([]api.HistoricTaskInstance, 0)
		for _, historic := range e.state.HistoricTasks {
			if processInstanceID != "" && historic.ProcessInstanceID != processInstanceID {
				continue
			}
			out = append(out, *historic)
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].StartTime.Equal(out[j].StartTime) {
				return out[i].ID < out[j].ID
			}
			return out[i].StartTime.Before(out[j].StartTime)
		})
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]api.HistoricTaskInstance), nil
}

// ListHistoricVariableInstances 返回历史变量。
// @param ctx 上下文
// @param processInstanceID 实例 ID
// @return 历史变量列表
// @return 查询错误
func (e *Engine) ListHistoricVariableInstances(ctx context.Context, processInstanceID string) ([]api.HistoricVariableInstance, error) {
	result, err := e.read(func() (any, error) {
		out := make([]api.HistoricVariableInstance, 0)
		for _, historic := range e.state.HistoricVariables {
			if processInstanceID != "" && historic.ProcessInstanceID != processInstanceID {
				continue
			}
			out = append(out, *historic)
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Name == out[j].Name {
				return out[i].ID < out[j].ID
			}
			return out[i].Name < out[j].Name
		})
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]api.HistoricVariableInstance), nil
}
