package engine

import (
	"context"
	"fmt"
	"log"

	"github.com/DengQingNian/Flowtide/api"
)

// 任务钩子事件类型，与 BPMN 任务监听器的 create/complete 事件相互独立。
const (
	hookEventCreated   = "created"   // 任务创建后触发
	hookEventCompleted = "completed" // 任务完成时触发
)

// AddTaskCreatedHook 注册任务创建全局钩子（TASK_CREATED）。
// 钩子按注册顺序在创建用户任务的命令内同步执行，任一钩子失败使命令失败回滚。
// @param hook 钩子函数
func (e *Engine) AddTaskCreatedHook(hook api.TaskHook) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.taskCreatedHooks = append(e.taskCreatedHooks, hook)
}

// AddTaskCompletedHook 注册任务完成全局钩子（TASK_COMPLETED）。
// 钩子按注册顺序在完成任务的命令内同步执行（历史结束行落库之前），任一钩子失败使命令失败回滚。
// @param hook 钩子函数
func (e *Engine) AddTaskCompletedHook(hook api.TaskHook) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.taskCompletedHooks = append(e.taskCompletedHooks, hook)
}

// invokeTaskHooks 按注册顺序执行任务生命周期全局钩子。
// 钩子拿到任务快照与命令级控制句柄；快照反映钩子执行前的状态，改写必须经控制句柄。
// @param cc 命令上下文
// @param task 任务
// @param event 事件：created/completed
// @return 执行错误（非空则当前命令失败回滚）
func (e *Engine) invokeTaskHooks(cc *commandContext, task *taskState, event string) error {
	hooks := e.taskCreatedHooks
	code := "ENGINE-1944"
	name := "创建"
	if event == hookEventCompleted {
		hooks = e.taskCompletedHooks
		code = "ENGINE-1945"
		name = "完成"
	}
	if len(hooks) == 0 {
		return nil
	}
	// 快照携带任务可见变量，供钩子读取流程上下文（与任务监听器快照语义一致）
	var variables map[string]any
	if execution, ok := e.findExecution(task.ExecutionID); ok {
		variables = e.variablesOf(execution)
	}
	snapshot := task.Task
	snapshot.Variables = variables
	ctl := &hookControl{engine: e, cc: cc, instanceID: task.Task.ProcessInstanceID}
	for _, hook := range hooks {
		if err := hook(cc.ctx, &snapshot, ctl); err != nil {
			// 钩子失败使命令失败回滚，与监听器错误路径一致
			return api.NewError(code, "任务"+name+"钩子执行失败", err)
		}
	}
	return nil
}

// hookControl 是 api.HookControl 的引擎实现，直接操作当前命令内的引擎内存状态。
type hookControl struct {
	engine     *Engine         // 引擎
	cc         *commandContext // 当前命令上下文
	instanceID string          // 钩子所属任务的流程实例 ID
}

// SetAssignee 命令内改写任务办理人。
// @param taskID 任务 ID
// @param userID 用户 ID，空串表示清空办理人
// @return 错误（任务不存在等）
func (c *hookControl) SetAssignee(taskID, userID string) error {
	return c.engine.setTaskAssigneeInCommand(c.cc, taskID, userID)
}

// SetCandidateUsers 命令内整体重写任务候选用户。
// @param taskID 任务 ID
// @param ids 候选用户 ID 列表，空列表表示清空
// @return 错误（任务不存在等）
func (c *hookControl) SetCandidateUsers(taskID string, ids []string) error {
	return c.engine.setTaskCandidatesInCommand(c.cc, taskID, ids, true)
}

// SetCandidateGroups 命令内整体重写任务候选组。
// @param taskID 任务 ID
// @param ids 候选组 ID 列表，空列表表示清空
// @return 错误（任务不存在等）
func (c *hookControl) SetCandidateGroups(taskID string, ids []string) error {
	return c.engine.setTaskCandidatesInCommand(c.cc, taskID, ids, false)
}

// SetVariable 设置流程实例级变量，写入实例根执行作用域。
// @param name 变量名
// @param value 变量值
// @return 错误（实例不存在等）
func (c *hookControl) SetVariable(name string, value any) error {
	return c.engine.setInstanceVariableInCommand(c.cc, c.instanceID, name, value)
}

// Defer 登记命令提交成功后顺序执行的延迟动作；命令失败回滚时不执行。
// @param action 延迟动作，入参为原命令上下文
func (c *hookControl) Defer(action func(ctx context.Context) error) {
	// 空动作直接忽略，避免延迟队列出现无意义条目
	if action == nil {
		return
	}
	c.cc.deferred = append(c.cc.deferred, action)
}

// setTaskAssigneeInCommand 在当前命令内改写任务办理人，复用 UpdateTask 的状态同步路径。
// @param cc 命令上下文
// @param taskID 任务 ID
// @param userID 新办理人，空串表示清空
// @return 错误（任务不存在等）
func (e *Engine) setTaskAssigneeInCommand(cc *commandContext, taskID, userID string) error {
	task, ok := e.findTask(taskID)
	if !ok {
		return api.NewError("ENGINE-0805", "任务不存在", api.ErrNotFound)
	}
	previous := task.Task.Assignee
	if previous == userID {
		return nil
	}
	task.Task.Assignee = userID
	// 清空办理人时同步清除签收时间，与 UpdateTask 语义一致
	if userID == "" {
		task.Task.ClaimTime = nil
	}
	e.syncTaskAssigneeHistory(task)
	e.recordTaskIdentityLinks(cc, task)
	// 原版把处理人变化升级流程参与者并记录独立事件
	if userID != "" {
		e.promoteProcessParticipant(cc, task.Task.ProcessInstanceID, userID, cc.now)
	}
	e.recordTaskLog(cc, task, "USER_TASK_ASSIGNEE_CHANGED", actorFromContext(cc.ctx), assigneeChangedLogData(previous, userID))
	return nil
}

// setTaskCandidatesInCommand 在当前命令内整体重写任务候选用户或候选组。
// @param cc 命令上下文
// @param taskID 任务 ID
// @param ids 候选 ID 列表，空列表表示清空
// @param users true 重写候选用户，false 重写候选组
// @return 错误（任务不存在等）
func (e *Engine) setTaskCandidatesInCommand(cc *commandContext, taskID string, ids []string, users bool) error {
	task, ok := e.findTask(taskID)
	if !ok {
		return api.NewError("ENGINE-0805", "任务不存在", api.ErrNotFound)
	}
	// 整体重写语义：拷贝入参避免外部切片别名污染引擎状态，空列表归一为 nil
	values := append([]string(nil), ids...)
	if len(values) == 0 {
		values = nil
	}
	if users {
		task.CandidateUsers = values
		task.Task.CandidateUsers = values
	} else {
		task.CandidateGroups = values
		task.Task.CandidateGroups = values
	}
	// 重建历史身份链接，保持候选列表与历史行一致
	e.recordTaskIdentityLinks(cc, task)
	return nil
}

// setInstanceVariableInCommand 在当前命令内设置流程实例级变量（根执行作用域）。
// @param cc 命令上下文
// @param processInstanceID 实例 ID
// @param name 变量名
// @param value 变量值
// @return 错误（实例或根执行不存在、条件求值失败等）
func (e *Engine) setInstanceVariableInCommand(cc *commandContext, processInstanceID, name string, value any) error {
	instance, ok := e.findInstance(processInstanceID)
	if !ok {
		return api.NewError("ENGINE-0404", "流程实例不存在", api.ErrNotFound)
	}
	root, ok := e.rootExecution(instance)
	if !ok {
		return api.NewError("ENGINE-0405", "根执行不存在", nil)
	}
	e.setVariable(root, name, value)
	// 变量变化后排队求值条件事件，议程由所在命令统一驱动，此处不重入议程
	return e.evaluateConditionalSubscriptions(cc, processInstanceID)
}

// syncTaskAssigneeHistory 将任务当前办理人同步到历史任务行与活动历史行。
// @param task 任务
func (e *Engine) syncTaskAssigneeHistory(task *taskState) {
	if historic, exists := e.state.HistoricTasks[task.HistoricTaskID]; exists {
		historic.Assignee = task.Task.Assignee
	}
	// 原版同步更新活动历史行上的处理人
	for _, activity := range e.state.HistoricActivities {
		if activity.TaskID == task.Task.ID {
			activity.Assignee = task.Task.Assignee
		}
	}
}

// assigneeChangedLogData 构造办理人变更事件数据。
// @param previous 原办理人
// @param next 新办理人
// @return 事件 JSON 数据
func assigneeChangedLogData(previous, next string) string {
	return fmt.Sprintf(`{"newAssigneeId":%q,"previousAssigneeId":%s}`, next, jsonNullableString(previous))
}

// runDeferredActions 在命令成功提交并释放锁后，按登记顺序执行延迟动作。
// 动作 panic 转错误日志，不向调用方传播；动作失败仅记录日志，不影响已完成的命令。
// @param ctx 原命令上下文
// @param actions 延迟动作列表
func (e *Engine) runDeferredActions(ctx context.Context, actions []func(ctx context.Context) error) {
	for _, action := range actions {
		e.runDeferredAction(ctx, action)
	}
}

// runDeferredAction 执行单个延迟动作，隔离 panic 与错误。
// @param ctx 原命令上下文
// @param action 延迟动作
func (e *Engine) runDeferredAction(ctx context.Context, action func(ctx context.Context) error) {
	defer func() {
		// 延迟动作属于命令后联动，panic 不能拖垮引擎调用方进程
		if r := recover(); r != nil {
			log.Printf("[flowtide] 任务钩子延迟动作 panic: %v", r)
		}
	}()
	if err := action(ctx); err != nil {
		// P0 阶段延迟动作失败仅记录日志；P4 阶段由异常表（workflow_task_exception）接管
		log.Printf("[flowtide] 任务钩子延迟动作失败: %v", err)
	}
}
