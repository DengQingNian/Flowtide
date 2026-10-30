package engine

import (
	"strings"

	"github.com/DengQingNian/Flowtide/api"
)

// invokeTaskListeners 执行任务监听器。
// 仅支持 class/delegateExpression；expression 监听器返回明确错误。
// @param cc 命令上下文
// @param task 任务
// @param event 事件：create/complete
// @return 执行错误
func (e *Engine) invokeTaskListeners(cc *commandContext, task *taskState, event string) error {
	if len(task.Listeners) == 0 {
		return nil
	}
	var variables map[string]any
	if execution, ok := e.findExecution(task.ExecutionID); ok {
		variables = e.variablesOf(execution)
	}
	snapshot := task.Task
	snapshot.Variables = variables
	for _, listenerDef := range task.Listeners {
		if !strings.EqualFold(listenerDef.Event, event) {
			continue
		}
		if strings.TrimSpace(listenerDef.Expression) != "" {
			return api.NewError("ENGINE-1932", "暂不支持表达式任务监听器: "+listenerDef.Expression, api.ErrBadRequest)
		}
		name := strings.TrimSpace(listenerDef.DelegateExpression)
		if name == "" {
			name = strings.TrimSpace(listenerDef.ClassName)
		}
		if name == "" {
			return api.NewError("ENGINE-1930", "任务监听器缺少实现", api.ErrBadRequest)
		}
		listener, ok := e.taskListeners[name]
		if !ok {
			return api.NewError("ENGINE-1930", "任务监听器未注册: "+name, api.ErrBadRequest)
		}
		if err := listener(cc.ctx, &snapshot); err != nil {
			return api.NewError("ENGINE-1931", "任务监听器执行失败: "+name, err)
		}
	}
	return nil
}
