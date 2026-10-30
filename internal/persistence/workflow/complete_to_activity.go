package workflow

import "context"

// CompleteToActivityRepository 是 workflow_complete_to_activity（直送记录）的仓储。
type CompleteToActivityRepository struct {
	baseRepo
}

// NewCompleteToActivityRepository 创建 workflow_complete_to_activity 仓储。
// @param store 仓储入口
// @return 仓储
func NewCompleteToActivityRepository(store *Store) CompleteToActivityRepository {
	return CompleteToActivityRepository{baseRepo{store: store, def: tableCompleteToActivities}}
}

// ListByProcessInstance 查询实例的直送记录（撤回策略纳入"下一节点集合"）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 ID
// @return 行列表
// @return 查询错误
func (r CompleteToActivityRepository) ListByProcessInstance(ctx context.Context, processInstanceID string) ([]Row, error) {
	return r.list(ctx, "process_instance_id = "+r.store.ph(1), []any{processInstanceID}, "create_time")
}

// ListByTask 查询任务的直送记录。
// @param ctx 操作上下文
// @param taskID 任务 ID
// @return 行列表
// @return 查询错误
func (r CompleteToActivityRepository) ListByTask(ctx context.Context, taskID string) ([]Row, error) {
	return r.list(ctx, "task_id = "+r.store.ph(1), []any{taskID}, "")
}
