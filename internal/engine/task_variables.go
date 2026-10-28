package engine

import (
	"context"

	"github.com/flowable-go/flowable/api"
)

// GetTaskVariable 返回任务变量。
// @param ctx 上下文
// @param taskID 任务 ID
// @param name 变量名
// @return 变量值
// @return 查询错误
func (e *Engine) GetTaskVariable(ctx context.Context, taskID, name string) (any, error) {
	result, err := e.read(func() (any, error) {
		task, ok := e.findTask(taskID)
		if !ok {
			return nil, api.NewError("ENGINE-0805", "任务不存在", api.ErrNotFound)
		}
		if err := e.allowTask(ctx, task.Task.ID); err != nil {
			return nil, err
		}
		if value, exists := task.LocalVariables[name]; exists {
			return value, nil
		}
		if execution, exists := e.findExecution(task.ExecutionID); exists {
			if value, ok := e.getVariable(execution, name); ok {
				return value, nil
			}
		}
		return nil, api.NewError("ENGINE-0406", "变量不存在: "+name, api.ErrNotFound)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// GetTaskVariables 返回任务可见变量。
// @param ctx 上下文
// @param taskID 任务 ID
// @return 变量快照
// @return 查询错误
func (e *Engine) GetTaskVariables(ctx context.Context, taskID string) (map[string]any, error) {
	result, err := e.read(func() (any, error) {
		task, ok := e.findTask(taskID)
		if !ok {
			return nil, api.NewError("ENGINE-0805", "任务不存在", api.ErrNotFound)
		}
		if err := e.allowTask(ctx, task.Task.ID); err != nil {
			return nil, err
		}
		values := make(map[string]any)
		if execution, exists := e.findExecution(task.ExecutionID); exists {
			for name, value := range e.variablesOf(execution) {
				values[name] = copyValue(value)
			}
		}
		for name, value := range task.LocalVariables {
			values[name] = copyValue(value)
		}
		return values, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(map[string]any), nil
}

// SetTaskVariable 设置任务局部变量。
// @param ctx 上下文
// @param taskID 任务 ID
// @param name 变量名
// @param value 变量值
// @return 设置错误
func (e *Engine) SetTaskVariable(ctx context.Context, taskID, name string, value any) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		task, ok := e.findTask(taskID)
		if !ok {
			return nil, api.NewError("ENGINE-0805", "任务不存在", api.ErrNotFound)
		}
		if err := e.allowTask(ctx, task.Task.ID); err != nil {
			return nil, err
		}
		if name == "" {
			return nil, api.NewError("ENGINE-1103", "变量名不能为空", api.ErrBadRequest)
		}
		task.LocalVariables[name] = copyValue(value)
		// 流程任务与独立任务均保存局部变量历史，不能只追加明细而遗漏当前历史值。
		e.recordTaskHistoricVariable(task, name, value, cc.now)
		if task.ExecutionID != "" {
			// 任务局部变量参与条件求值，变化后立即触发
			if err := e.evaluateConditionalSubscriptions(cc, task.Task.ProcessInstanceID); err != nil {
				return nil, err
			}
			return nil, cc.agenda.run(cc)
		}
		return nil, nil
	})
	return err
}

// DeleteAllTaskVariables 清空任务局部变量。
// @param ctx 上下文
// @param taskID 任务 ID
// @return 删除错误
func (e *Engine) DeleteAllTaskVariables(ctx context.Context, taskID string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		task, ok := e.findTask(taskID)
		if !ok {
			return nil, api.NewError("ENGINE-0805", "任务不存在", api.ErrNotFound)
		}
		if err := e.allowTask(cc.ctx, task.Task.ID); err != nil {
			return nil, err
		}
		task.LocalVariables = make(map[string]any)
		return nil, nil
	})
	return err
}

// DeleteTaskVariable 删除任务局部变量。
// @param ctx 上下文
// @param taskID 任务 ID
// @param name 变量名
// @return 删除错误
func (e *Engine) DeleteTaskVariable(ctx context.Context, taskID, name string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		task, ok := e.findTask(taskID)
		if !ok {
			return nil, api.NewError("ENGINE-0805", "任务不存在", api.ErrNotFound)
		}
		if err := e.allowTask(ctx, task.Task.ID); err != nil {
			return nil, err
		}
		delete(task.LocalVariables, name)
		return nil, nil
	})
	return err
}
