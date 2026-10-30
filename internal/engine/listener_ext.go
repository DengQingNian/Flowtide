package engine

import (
	"context"

	"github.com/DengQingNian/Flowtide/api"
)

// RegisterExecutionListenerHook 注册扩展执行监听器（api.ExecutionListenerHook 控制接口版）。
// 与 RegisterExecutionListener 共用监听器调用点：BPMN class/delegateExpression 名相同时
// 扩展监听器优先生效。监听器在命令事务内同步执行，返回错误使命令整体回滚。
// @param name 监听器名（BPMN class/delegateExpression 注册名）
// @param hook 监听器钩子
func (e *Engine) RegisterExecutionListenerHook(name string, hook api.ExecutionListenerHook) {
	e.mu.Lock()
	defer e.mu.Unlock()
	// 空钩子等价于注销，避免调用点残留无效条目
	if hook == nil {
		delete(e.executionListenerHooks, name)
		return
	}
	e.executionListenerHooks[name] = hook
}

// invokeExecutionListenerHooks 在监听器调用点查找并执行扩展监听器。
// 由 invokeExecutionListeners 在命令上下文内调用，仅本文件与该调用点使用。
// @param cc 命令上下文
// @param execution 执行上下文
// @param name 监听器注册名
// @return 是否命中扩展监听器（命中false时调用点应回退原生监听器表）
// @return 执行错误
func (e *Engine) invokeExecutionListenerHooks(cc *commandContext, execution *Execution, name string) (bool, error) {
	hook, ok := e.executionListenerHooks[name]
	if !ok {
		return false, nil
	}
	ctl := &listenerControl{engine: e, state: execution.state, cc: cc}
	if err := hook(cc.ctx, e.listenerExecutionOf(execution), ctl); err != nil {
		return true, err
	}
	return true, nil
}

// listenerExecutionOf 构造扩展监听器可见的执行上下文快照。
// @param execution 引擎执行上下文
// @return 快照
func (e *Engine) listenerExecutionOf(execution *Execution) api.ListenerExecution {
	state := execution.state
	instance := e.state.Instances[state.ProcessInstanceID]
	snapshot := api.ListenerExecution{
		ExecutionID:         state.ID,
		ActivityID:          execution.ActivityID(),
		ProcessInstanceID:   state.ProcessInstanceID,
		ProcessDefinitionID: state.DefinitionID,
		MIChild:             state.MIChild,
		Variables:           e.variablesOf(state),
	}
	if instance != nil {
		snapshot.TenantID = instance.TenantID
	}
	return snapshot
}

// listenerControl 是 api.ListenerControl 的引擎实现，
// 直接操作当前命令内的引擎内存状态（改写随命令回滚）。
type listenerControl struct {
	engine *Engine         // 引擎
	state  *executionState // 监听器所属执行状态
	cc     *commandContext // 当前命令上下文
}

// SetVariable 设置流程实例级变量（写入最近已存在变量作用域，否则根执行）。
// @param name 变量名
// @param value 变量值
// @return 错误（执行不存在等）
func (c *listenerControl) SetVariable(name string, value any) error {
	if _, ok := c.engine.findExecution(c.state.ID); !ok {
		return api.NewError("ENGINE-0405", "执行不存在", api.ErrNotFound)
	}
	c.engine.setVariable(c.state, name, value)
	return nil
}

// SetVariableLocal 设置当前执行作用域变量。
// @param name 变量名
// @param value 变量值
// @return 错误（执行不存在等）
func (c *listenerControl) SetVariableLocal(name string, value any) error {
	if _, ok := c.engine.findExecution(c.state.ID); !ok {
		return api.NewError("ENGINE-0405", "执行不存在", api.ErrNotFound)
	}
	c.engine.setVariableLocal(c.state, name, value)
	return nil
}

// Defer 登记命令提交成功后顺序执行的延迟动作；命令失败回滚时不执行。
// @param action 延迟动作，入参为原命令上下文
func (c *listenerControl) Defer(action func(ctx context.Context) error) {
	// 空动作直接忽略，避免延迟队列出现无意义条目
	if action == nil {
		return
	}
	c.cc.deferred = append(c.cc.deferred, action)
}
