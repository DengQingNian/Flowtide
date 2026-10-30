package workflow

import "context"

// DelegateTaskRepository 是 workflow_delegate_task（委托镜像待办）的仓储。
type DelegateTaskRepository struct {
	baseRepo
}

// NewDelegateTaskRepository 创建 workflow_delegate_task 仓储。
// @param store 仓储入口
// @return 仓储
func NewDelegateTaskRepository(store *Store) DelegateTaskRepository {
	return DelegateTaskRepository{baseRepo{store: store, def: tableDelegateTasks}}
}

// GetByTask 按任务 ID 查询镜像待办。
// @param ctx 操作上下文
// @param taskID 任务 ID
// @return 行数据
// @return 是否存在
// @return 查询错误
func (r DelegateTaskRepository) GetByTask(ctx context.Context, taskID string) (Row, bool, error) {
	return r.getFirst(ctx, "task_id = "+r.store.ph(1), []any{taskID}, "")
}

// ListByEntrust 查询被委托人的镜像待办列表。
// @param ctx 操作上下文
// @param entrustID 被委托人 ID
// @return 行列表
// @return 查询错误
func (r DelegateTaskRepository) ListByEntrust(ctx context.Context, entrustID string) ([]Row, error) {
	return r.list(ctx, "entrust_id = "+r.store.ph(1), []any{entrustID}, "create_time")
}

// ListByProcessInstance 查询实例下全部镜像待办。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 ID
// @return 行列表
// @return 查询错误
func (r DelegateTaskRepository) ListByProcessInstance(ctx context.Context, processInstanceID string) ([]Row, error) {
	return r.list(ctx, "process_instance_id = "+r.store.ph(1), []any{processInstanceID}, "create_time")
}

// DeleteByProcessInstance 删除实例下全部镜像待办（实例删除级联）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 ID
// @return 影响行数
// @return 删除错误
func (r DelegateTaskRepository) DeleteByProcessInstance(ctx context.Context, processInstanceID string) (int64, error) {
	return r.deleteWhere(ctx, "process_instance_id = "+r.store.ph(1), []any{processInstanceID})
}
