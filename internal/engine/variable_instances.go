package engine

import (
	"context"
	"sort"
	"strings"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/tenant"
)

// ListVariableInstances 返回运行时变量实例。
// @param ctx 上下文
// @param filter 查询条件
// @return 变量实例列表
// @return 查询错误
func (e *Engine) ListVariableInstances(ctx context.Context, filter api.VariableInstanceFilter) ([]api.VariableInstance, error) {
	result, err := e.read(func() (any, error) {
		out := e.variableInstancesLocked(e.tenantOf(tenant.From(ctx)), filter)
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]api.VariableInstance), nil
}

// GetVariableInstance 查询运行时变量实例。
// @param ctx 上下文
// @param variableID 变量实例 ID
// @return 变量实例
// @return 查询错误
func (e *Engine) GetVariableInstance(ctx context.Context, variableID string) (api.VariableInstance, error) {
	result, err := e.read(func() (any, error) {
		for _, item := range e.variableInstancesLocked(e.tenantOf(tenant.From(ctx)), api.VariableInstanceFilter{}) {
			if item.ID == variableID {
				return item, nil
			}
		}
		return nil, api.NewError("ENGINE-1601", "变量实例不存在", api.ErrNotFound)
	})
	if err != nil {
		return api.VariableInstance{}, err
	}
	return result.(api.VariableInstance), nil
}

// variableInstancesLocked 在锁内收集变量实例。
// @param requestedTenant 请求租户
// @param filter 查询条件
// @return 变量实例列表
func (e *Engine) variableInstancesLocked(requestedTenant string, filter api.VariableInstanceFilter) []api.VariableInstance {
	out := make([]api.VariableInstance, 0)
	if filter.TaskID == "" {
		for _, execution := range e.state.Executions {
			if execution.Ended {
				continue
			}
			instance, ok := e.findInstance(execution.ProcessInstanceID)
			if !ok || instance.Ended {
				continue
			}
			if requestedTenant != instance.TenantID {
				continue
			}
			if filter.ProcessInstanceID != "" && execution.ProcessInstanceID != filter.ProcessInstanceID {
				continue
			}
			if filter.ExecutionID != "" && execution.ID != filter.ExecutionID {
				continue
			}
			for name, value := range execution.Variables {
				if !matchVariableName(name, filter) {
					continue
				}
				out = append(out, api.VariableInstance{
					ID:                variableID(execution.ID, "", name),
					Name:              name,
					Type:              variableTypeName(value),
					Value:             value,
					ProcessInstanceID: execution.ProcessInstanceID,
					ExecutionID:       execution.ID,
					Scope:             "global",
				})
			}
		}
	}
	for _, task := range e.state.Tasks {
		instance, hasInstance := e.findInstance(task.Task.ProcessInstanceID)
		if hasInstance && instance.Ended {
			continue
		}
		tenantID := task.Task.TenantID
		if hasInstance {
			tenantID = instance.TenantID
		}
		if requestedTenant != tenantID {
			continue
		}
		if filter.ProcessInstanceID != "" && task.Task.ProcessInstanceID != filter.ProcessInstanceID {
			continue
		}
		if filter.TaskID != "" && task.Task.ID != filter.TaskID {
			continue
		}
		if filter.ExecutionID != "" && task.ExecutionID != filter.ExecutionID {
			continue
		}
		for name, value := range task.LocalVariables {
			if !matchVariableName(name, filter) {
				continue
			}
			out = append(out, api.VariableInstance{
				ID:                variableID(task.ExecutionID, task.Task.ID, name),
				Name:              name,
				Type:              variableTypeName(value),
				Value:             value,
				ProcessInstanceID: task.Task.ProcessInstanceID,
				ExecutionID:       task.ExecutionID,
				TaskID:            task.Task.ID,
				Scope:             "local",
			})
		}
	}
	return out
}

// matchVariableName 判断变量名是否满足条件。
// @param name 变量名
// @param filter 查询条件
// @return 是否匹配
func matchVariableName(name string, filter api.VariableInstanceFilter) bool {
	if filter.Name != "" && name != filter.Name {
		return false
	}
	if filter.NameLike != "" && !strings.Contains(strings.ToLower(name), strings.ToLower(strings.Trim(filter.NameLike, "%"))) {
		return false
	}
	return true
}
