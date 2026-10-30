package workflow

import "context"

// TaskExceptionRepository 是 workflow_task_exception（任务异常落库）的仓储。
// handle_status：0 未处理、1 已处理（Go 侧新增的异常处理状态列）。
type TaskExceptionRepository struct {
	baseRepo
}

// NewTaskExceptionRepository 创建 workflow_task_exception 仓储。
// @param store 仓储入口
// @return 仓储
func NewTaskExceptionRepository(store *Store) TaskExceptionRepository {
	return TaskExceptionRepository{baseRepo{store: store, def: tableTaskExceptions}}
}

// ListUnhandled 查询未处理异常（handle_status=0，管理台重发与僵尸监控数据源）。
// @param ctx 操作上下文
// @return 行列表
// @return 查询错误
func (r TaskExceptionRepository) ListUnhandled(ctx context.Context) ([]Row, error) {
	return r.list(ctx, "handle_status = 0", nil, "create_time")
}

// ListByProcessInstance 查询实例的异常记录。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 ID
// @return 行列表
// @return 查询错误
func (r TaskExceptionRepository) ListByProcessInstance(ctx context.Context, processInstanceID string) ([]Row, error) {
	return r.list(ctx, "process_instance_id = "+r.store.ph(1), []any{processInstanceID}, "create_time")
}

// ListByTask 查询任务的异常记录。
// @param ctx 操作上下文
// @param taskID 任务 ID
// @return 行列表
// @return 查询错误
func (r TaskExceptionRepository) ListByTask(ctx context.Context, taskID string) ([]Row, error) {
	return r.list(ctx, "task_id = "+r.store.ph(1), []any{taskID}, "create_time")
}

// MarkHandled 标记异常已处理（管理台重发成功后调用）。
// @param ctx 操作上下文
// @param id 异常记录 ID
// @return 影响行数
// @return 更新错误
func (r TaskExceptionRepository) MarkHandled(ctx context.Context, id string) (int64, error) {
	return r.updateWhere(ctx, Row{"handle_status": 1}, "id = "+r.store.ph(1), []any{id})
}
