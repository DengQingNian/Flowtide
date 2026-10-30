package api

import "context"

// ListenerExecution 是扩展执行监听器可见的执行上下文快照。
// 对位 Flowable DelegateExecution 的只读面：变量为监听器触发时刻的可见快照，
// 监听器内的状态改写必须经 ListenerControl（改写快照无效）。
type ListenerExecution struct {
	ExecutionID         string         // 执行 ID
	ActivityID          string         // 当前活动 ID（活动结束监听器下为离开的节点 ID）
	ProcessInstanceID   string         // 流程实例 ID
	ProcessDefinitionID string         // 流程定义 ID
	TenantID            string         // 租户
	MIChild             bool           // 是否多实例子执行（会签监听器区分根执行与实例执行用）
	Variables           map[string]any // 执行可见变量快照（含作用域链继承值）
}

// ListenerControl 监听器控制接口，提供给执行监听器在命令内同步改写引擎状态、
// 登记命令后延迟动作。对位 api.HookControl 模式：命令内改写在监听器返回错误时随命令整体回滚。
type ListenerControl interface {
	// SetVariable 设置流程实例级变量（写入最近已存在作用域，语义与引擎 Execution.SetVariable 一致）。
	// @param name 变量名
	// @param value 变量值
	// @return 错误（实例不存在等）
	SetVariable(name string, value any) error
	// SetVariableLocal 设置当前执行作用域变量。
	// @param name 变量名
	// @param value 变量值
	// @return 错误（执行不存在等）
	SetVariableLocal(name string, value any) error
	// Defer 登记命令提交成功后顺序执行的延迟动作（新命令上下文，锁外执行）。
	// 命令失败回滚时不执行；动作 panic 或返回错误仅记录日志，不影响已完成的命令。
	// @param action 延迟动作，入参为原命令上下文
	Defer(action func(ctx context.Context) error)
}

// ExecutionListenerHook 是扩展执行监听器（控制接口版）。
// 在引擎命令事务内同步执行，返回错误使整个命令回滚（与 BPMN 执行监听器语义一致）。
// @param ctx 命令上下文（与所在引擎命令共用，命令失败会整体回滚）
// @param execution 执行上下文快照
// @param ctl 监听器控制（命令内变量改写 + 命令后延迟动作）
// @return 错误（非空则当前引擎命令失败并整体回滚）
type ExecutionListenerHook func(ctx context.Context, execution ListenerExecution, ctl ListenerControl) error

// ExecutionListenerRegistrar 是支持注册扩展执行监听器的引擎能力（由 *engine.Engine 实现）。
// workflow 门面按 BPMN flowable:executionListener 的 class 名分发注册。
type ExecutionListenerRegistrar interface {
	// RegisterExecutionListenerHook 注册扩展执行监听器。
	// @param name 监听器名（BPMN class/delegateExpression 注册名）
	// @param hook 监听器钩子
	RegisterExecutionListenerHook(name string, hook ExecutionListenerHook)
}
