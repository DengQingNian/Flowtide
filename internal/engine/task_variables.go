package engine

import (
	"context"

	"github.com/flowable-go/flowable/api"
)

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
		values := make(map[string]any)
		if execution, exists := e.findExecution(task.ExecutionID); exists {
			for name, value := range e.variablesOf(execution) {
				values[name] = value
			}
		}
		for name, value := range task.LocalVariables {
			values[name] = value
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
		if name == "" {
			return nil, api.NewError("ENGINE-1103", "变量名不能为空", api.ErrBadRequest)
		}
		task.LocalVariables[name] = value
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
		delete(task.LocalVariables, name)
		return nil, nil
	})
	return err
}
