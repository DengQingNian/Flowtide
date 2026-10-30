package workflow

import "context"

// ReviewTaskRepository 是 workflow_review_task（审阅/知会影子任务）的仓储。
type ReviewTaskRepository struct {
	baseRepo
}

// NewReviewTaskRepository 创建 workflow_review_task 仓储。
// @param store 仓储入口
// @return 仓储
func NewReviewTaskRepository(store *Store) ReviewTaskRepository {
	return ReviewTaskRepository{baseRepo{store: store, def: tableReviewTasks}}
}

// GetByTask 按任务 ID 查询审阅任务。
// @param ctx 操作上下文
// @param taskID 任务 ID
// @return 行数据
// @return 是否存在
// @return 查询错误
func (r ReviewTaskRepository) GetByTask(ctx context.Context, taskID string) (Row, bool, error) {
	return r.getFirst(ctx, "task_id = "+r.store.ph(1), []any{taskID}, "")
}

// ListByProcessInstance 查询实例的审阅任务。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 ID
// @return 行列表
// @return 查询错误
func (r ReviewTaskRepository) ListByProcessInstance(ctx context.Context, processInstanceID string) ([]Row, error) {
	return r.list(ctx, "process_instance_id = "+r.store.ph(1), []any{processInstanceID}, "create_time")
}

// ListPendingByReviewer 查询审阅人的未审任务（status=0）。
// @param ctx 操作上下文
// @param reviewer 审阅人 ID
// @return 行列表
// @return 查询错误
func (r ReviewTaskRepository) ListPendingByReviewer(ctx context.Context, reviewer string) ([]Row, error) {
	return r.list(ctx, "reviewer = "+r.store.ph(1)+" AND status = 0", []any{reviewer}, "start_time")
}

// MarkReviewed 完成审阅：置已审状态、回写意见与审阅时间。
// @param ctx 操作上下文
// @param id 审阅任务 ID
// @param opinion 审阅意见
// @param reviewTime 审阅时间
// @return 影响行数
// @return 更新错误
func (r ReviewTaskRepository) MarkReviewed(ctx context.Context, id, opinion string, reviewTime any) (int64, error) {
	return r.updateWhere(ctx, Row{"status": 1, "opinion": opinion, "review_time": reviewTime}, "id = "+r.store.ph(1), []any{id})
}
