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
			if e.allowTenant(ctx, historic.TenantID) != nil {
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
			if e.allowHistoricResource(ctx, historic.ProcessInstanceID, "") != nil {
				continue
			}
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
			if e.allowTenant(ctx, historic.TenantID) != nil {
				continue
			}
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
			if e.allowHistoricResource(ctx, historic.ProcessInstanceID, historic.TaskID) != nil {
				continue
			}
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

// ListHistoricIdentityLinks 返回历史身份链接。
// @param ctx 上下文
// @param taskID 任务 ID
// @return 链接列表
// @return 查询错误
func (e *Engine) ListHistoricIdentityLinks(ctx context.Context, taskID string) ([]api.IdentityLink, error) {
	result, err := e.read(func() (any, error) {
		out := make([]api.IdentityLink, 0)
		for _, link := range e.state.HistoricLinks {
			if e.allowHistoricResource(ctx, link.ProcessInstanceID, link.TaskID) != nil {
				continue
			}
			if taskID != "" && link.TaskID != taskID {
				continue
			}
			out = append(out, *link)
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].CreateTime.Equal(out[j].CreateTime) {
				return out[i].ID < out[j].ID
			}
			return out[i].CreateTime.Before(out[j].CreateTime)
		})
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]api.IdentityLink), nil
}

// ListHistoricProcessIdentityLinks 返回流程实例的历史身份链接。
// @param ctx 上下文
// @param processInstanceID 实例 ID
// @return 链接列表
// @return 查询错误
func (e *Engine) ListHistoricProcessIdentityLinks(ctx context.Context, processInstanceID string) ([]api.IdentityLink, error) {
	result, err := e.read(func() (any, error) {
		out := make([]api.IdentityLink, 0)
		for _, link := range e.state.HistoricLinks {
			if e.allowHistoricResource(ctx, link.ProcessInstanceID, link.TaskID) != nil {
				continue
			}
			if link.ProcessInstanceID != processInstanceID {
				continue
			}
			out = append(out, *link)
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].CreateTime.Equal(out[j].CreateTime) {
				return out[i].ID < out[j].ID
			}
			return out[i].CreateTime.Before(out[j].CreateTime)
		})
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]api.IdentityLink), nil
}

// ListHistoricDetails 返回历史变量明细。
// @param ctx 上下文
// @param processInstanceID 实例 ID
// @return 明细列表
// @return 查询错误
func (e *Engine) ListHistoricDetails(ctx context.Context, processInstanceID string) ([]api.HistoricDetail, error) {
	result, err := e.read(func() (any, error) {
		out := make([]api.HistoricDetail, 0)
		for _, detail := range e.state.HistoricDetails {
			if e.allowHistoricResource(ctx, detail.ProcessInstanceID, detail.TaskID) != nil {
				continue
			}
			if processInstanceID != "" && detail.ProcessInstanceID != processInstanceID {
				continue
			}
			out = append(out, *detail)
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Time.Equal(out[j].Time) {
				return out[i].ID < out[j].ID
			}
			return out[i].Time.Before(out[j].Time)
		})
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]api.HistoricDetail), nil
}

// ListHistoricTaskLogs 返回任务生命周期日志。
// @param ctx 上下文
// @param taskID 任务 ID
// @return 日志列表
// @return 查询错误
func (e *Engine) ListHistoricTaskLogs(ctx context.Context, taskID string) ([]api.HistoricTaskLog, error) {
	result, err := e.read(func() (any, error) {
		out := make([]api.HistoricTaskLog, 0)
		for _, log := range e.state.TaskLogs {
			if e.allowHistoricResource(ctx, log.ProcessInstanceID, log.TaskID) != nil {
				continue
			}
			if taskID != "" && log.TaskID != taskID {
				continue
			}
			out = append(out, *log)
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Time.Equal(out[j].Time) {
				return out[i].ID < out[j].ID
			}
			return out[i].Time.Before(out[j].Time)
		})
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]api.HistoricTaskLog), nil
}

// GetHistoricActivityInstance 查询历史活动。
// @param ctx 上下文
// @param activityID 历史活动 ID
// @return 历史活动
// @return 查询错误
func (e *Engine) GetHistoricActivityInstance(ctx context.Context, activityID string) (api.HistoricActivityInstance, error) {
	result, err := e.read(func() (any, error) {
		activity, ok := e.state.HistoricActivities[activityID]
		if !ok {
			return nil, api.NewError("ENGINE-0404", "历史活动不存在", api.ErrNotFound)
		}
		if err := e.allowHistoricResource(ctx, activity.ProcessInstanceID, ""); err != nil {
			return nil, err
		}
		return *activity, nil
	})
	if err != nil {
		return api.HistoricActivityInstance{}, err
	}
	return result.(api.HistoricActivityInstance), nil
}

// GetHistoricVariableInstance 查询历史变量。
// @param ctx 上下文
// @param variableID 历史变量 ID
// @return 历史变量
// @return 查询错误
func (e *Engine) GetHistoricVariableInstance(ctx context.Context, variableID string) (api.HistoricVariableInstance, error) {
	result, err := e.read(func() (any, error) {
		for _, variable := range e.state.HistoricVariables {
			if variable.ID == variableID {
				if err := e.allowHistoricResource(ctx, variable.ProcessInstanceID, variable.TaskID); err != nil {
					return nil, err
				}
				return *variable, nil
			}
		}
		return nil, api.NewError("ENGINE-0404", "历史变量不存在", api.ErrNotFound)
	})
	if err != nil {
		return api.HistoricVariableInstance{}, err
	}
	return result.(api.HistoricVariableInstance), nil
}

// DeleteHistoricVariableInstance 删除历史变量。
// @param ctx 上下文
// @param variableID 历史变量 ID
// @return 删除错误
func (e *Engine) DeleteHistoricVariableInstance(ctx context.Context, variableID string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		for key, variable := range e.state.HistoricVariables {
			if variable.ID == variableID {
				delete(e.state.HistoricVariables, key)
				return nil, nil
			}
		}
		return nil, api.NewError("ENGINE-0404", "历史变量不存在", api.ErrNotFound)
	})
	return err
}
