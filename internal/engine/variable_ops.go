package engine

import (
	"context"
	"strings"

	"github.com/flowable-go/flowable/api"
)

// GetExecutionVariable 返回执行可见变量。
// @param ctx 上下文
// @param executionID 执行 ID
// @param name 变量名
// @return 变量值
// @return 查询错误
func (e *Engine) GetExecutionVariable(ctx context.Context, executionID, name string) (any, error) {
	result, err := e.read(func() (any, error) {
		execution, ok := e.findExecution(executionID)
		if !ok {
			return nil, api.NewError("ENGINE-0405", "执行不存在", api.ErrNotFound)
		}
		if err := e.allowInstance(ctx, execution.ProcessInstanceID); err != nil {
			return nil, err
		}
		if strings.TrimSpace(name) == "" {
			return nil, api.NewError("ENGINE-1103", "变量名不能为空", api.ErrBadRequest)
		}
		value, exists := e.variablesOf(execution)[name]
		if !exists {
			return nil, api.NewError("ENGINE-1104", "变量不存在", api.ErrNotFound)
		}
		return value, nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// SetExecutionVariable 设置执行局部变量。
// @param ctx 上下文
// @param executionID 执行 ID
// @param name 变量名
// @param value 变量值
// @return 设置错误
func (e *Engine) SetExecutionVariable(ctx context.Context, executionID, name string, value any) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		execution, ok := e.findExecution(executionID)
		if !ok {
			return nil, api.NewError("ENGINE-0405", "执行不存在", api.ErrNotFound)
		}
		if err := e.allowInstance(cc.ctx, execution.ProcessInstanceID); err != nil {
			return nil, err
		}
		if strings.TrimSpace(name) == "" {
			return nil, api.NewError("ENGINE-1103", "变量名不能为空", api.ErrBadRequest)
		}
		if execution.Variables == nil {
			execution.Variables = make(map[string]any)
		}
		execution.Variables[name] = value
		e.recordHistoricVariable(execution, name, value)
		// 变量变化后立即求值条件事件
		if err := e.evaluateConditionalSubscriptions(cc, execution.ProcessInstanceID); err != nil {
			return nil, err
		}
		return nil, cc.agenda.run(cc)
	})
	return err
}

// DeleteExecutionVariable 删除执行局部变量。
// @param ctx 上下文
// @param executionID 执行 ID
// @param name 变量名
// @return 删除错误
func (e *Engine) DeleteExecutionVariable(ctx context.Context, executionID, name string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		execution, ok := e.findExecution(executionID)
		if !ok {
			return nil, api.NewError("ENGINE-0405", "执行不存在", api.ErrNotFound)
		}
		if err := e.allowInstance(cc.ctx, execution.ProcessInstanceID); err != nil {
			return nil, err
		}
		if _, exists := execution.Variables[name]; !exists {
			return nil, api.NewError("ENGINE-1104", "变量不存在", api.ErrNotFound)
		}
		delete(execution.Variables, name)
		return nil, nil
	})
	return err
}

// DeleteAllExecutionVariables 清空执行局部变量。
// @param ctx 上下文
// @param executionID 执行 ID
// @return 删除错误
func (e *Engine) DeleteAllExecutionVariables(ctx context.Context, executionID string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		execution, ok := e.findExecution(executionID)
		if !ok {
			return nil, api.NewError("ENGINE-0405", "执行不存在", api.ErrNotFound)
		}
		if err := e.allowInstance(cc.ctx, execution.ProcessInstanceID); err != nil {
			return nil, err
		}
		execution.Variables = make(map[string]any)
		return nil, nil
	})
	return err
}

// DeleteAllInstanceVariables 清空流程实例变量。
// @param ctx 上下文
// @param processInstanceID 实例 ID
// @return 删除错误
func (e *Engine) DeleteAllInstanceVariables(ctx context.Context, processInstanceID string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		instance, ok := e.findInstance(processInstanceID)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "流程实例不存在", api.ErrNotFound)
		}
		if err := e.allowTenant(cc.ctx, instance.TenantID); err != nil {
			return nil, err
		}
		if root, exists := e.rootExecution(instance); exists {
			root.Variables = make(map[string]any)
		}
		return nil, nil
	})
	return err
}

// GetHistoricDetail 查询历史变量明细。
// @param ctx 上下文
// @param detailID 明细 ID
// @return 明细
// @return 查询错误
func (e *Engine) GetHistoricDetail(ctx context.Context, detailID string) (api.HistoricDetail, error) {
	result, err := e.read(func() (any, error) {
		detail, ok := e.state.HistoricDetails[detailID]
		if !ok {
			return nil, api.NewError("ENGINE-0404", "历史明细不存在", api.ErrNotFound)
		}
		if err := e.allowHistoricResource(ctx, detail.ProcessInstanceID, detail.TaskID); err != nil {
			return nil, err
		}
		return *detail, nil
	})
	if err != nil {
		return api.HistoricDetail{}, err
	}
	return result.(api.HistoricDetail), nil
}

// GetHistoricVariableByName 按实例与变量名查询历史变量。
// @param ctx 上下文
// @param processInstanceID 实例 ID
// @param name 变量名
// @return 历史变量
// @return 查询错误
func (e *Engine) GetHistoricVariableByName(ctx context.Context, processInstanceID, name string) (api.HistoricVariableInstance, error) {
	result, err := e.read(func() (any, error) {
		if err := e.allowHistoricResource(ctx, processInstanceID, ""); err != nil {
			return nil, err
		}
		return latestHistoricVariable(e.state.HistoricVariables, func(variable *api.HistoricVariableInstance) bool {
			return variable.ProcessInstanceID == processInstanceID && variable.TaskID == "" && variable.Name == name
		})
	})
	if err != nil {
		return api.HistoricVariableInstance{}, err
	}
	return result.(api.HistoricVariableInstance), nil
}

// GetHistoricTaskVariableByName 按任务与变量名查询历史变量。
// @param ctx 上下文
// @param taskID 任务 ID
// @param name 变量名
// @return 历史变量
// @return 查询错误
func (e *Engine) GetHistoricTaskVariableByName(ctx context.Context, taskID, name string) (api.HistoricVariableInstance, error) {
	result, err := e.read(func() (any, error) {
		if err := e.allowHistoricResource(ctx, "", taskID); err != nil {
			return nil, err
		}
		variable, findErr := latestHistoricVariable(e.state.HistoricVariables, func(variable *api.HistoricVariableInstance) bool {
			return variable.TaskID == taskID && variable.Name == name
		})
		if findErr != nil {
			if _, ok := e.state.HistoricTasks[taskID]; !ok {
				return nil, api.NewError("ENGINE-0404", "历史任务不存在", api.ErrNotFound)
			}
			return nil, findErr
		}
		return variable, nil
	})
	if err != nil {
		return api.HistoricVariableInstance{}, err
	}
	return result.(api.HistoricVariableInstance), nil
}

// latestHistoricVariable 返回满足条件的最新历史变量。
// @param variables 历史变量集合
// @param match 条件
// @return 历史变量
// @return 错误
func latestHistoricVariable(variables map[string]*api.HistoricVariableInstance, match func(*api.HistoricVariableInstance) bool) (api.HistoricVariableInstance, error) {
	var found *api.HistoricVariableInstance
	for _, variable := range variables {
		if !match(variable) {
			continue
		}
		if found == nil || variable.LastUpdatedTime.After(found.LastUpdatedTime) ||
			(variable.LastUpdatedTime.Equal(found.LastUpdatedTime) && variable.ID > found.ID) {
			found = variable
		}
	}
	if found == nil {
		return api.HistoricVariableInstance{}, api.NewError("ENGINE-1104", "变量不存在", api.ErrNotFound)
	}
	return *found, nil
}
