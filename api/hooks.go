package api

import "context"

// TaskHook 任务生命周期全局钩子，对标 Flowable 全局事件监听（TASK_CREATED/TASK_COMPLETED）。
// 在创建/完成任务的引擎命令内同步调用：命令持有引擎互斥锁，钩子内禁止再调用引擎公有 API
// （会重入死锁），需要再次驱动引擎的联动必须通过 ctl.Defer 延迟到命令提交后执行。
// @param ctx 命令上下文（与所在引擎命令共用，命令失败会整体回滚）
// @param task 任务快照（快照改写无效，改写任务状态必须经 ctl）
// @param ctl 钩子控制（命令内状态改写 + 命令后延迟动作）
// @return 错误（非空则当前引擎命令失败并整体回滚）
type TaskHook func(ctx context.Context, task *Task, ctl HookControl) error

// HookControl 钩子控制接口，提供给钩子在命令内同步改写引擎状态、登记命令后延迟动作。
// 各改写方法针对当前命令上下文生效：钩子返回错误时，命令回滚，全部改写一并失效。
type HookControl interface {
	// SetAssignee 命令内改写任务办理人（执行人重写入口）。
	// @param taskID 任务 ID
	// @param userID 用户 ID，空串表示清空办理人
	// @return 错误（任务不存在等）
	SetAssignee(taskID, userID string) error
	// SetCandidateUsers 命令内整体重写任务候选用户。
	// @param taskID 任务 ID
	// @param ids 候选用户 ID 列表，空列表表示清空
	// @return 错误（任务不存在等）
	SetCandidateUsers(taskID string, ids []string) error
	// SetCandidateGroups 命令内整体重写任务候选组。
	// @param taskID 任务 ID
	// @param ids 候选组 ID 列表，空列表表示清空
	// @return 错误（任务不存在等）
	SetCandidateGroups(taskID string, ids []string) error
	// SetVariable 设置流程实例级变量（写入实例根执行作用域）。
	// @param name 变量名
	// @param value 变量值
	// @return 错误（实例不存在等）
	SetVariable(name string, value any) error
	// Defer 登记命令提交成功后顺序执行的延迟动作（新命令上下文，锁外执行）。
	// 动作内可安全调用引擎公有 API（如 CompleteTask 驱动相邻同人自动办结）；
	// 命令失败回滚时不执行；动作 panic 或返回错误仅记录日志，不影响已完成的命令。
	// @param action 延迟动作，入参为原命令上下文
	Defer(action func(ctx context.Context) error)
}
