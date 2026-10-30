package workflow

import "context"

// HistoryExtendRepository 是 workflow_task_history_extend（已办扩展，ACT_HI_TASKINST 1:1）的仓储。
type HistoryExtendRepository struct {
	baseRepo
}

// NewHistoryExtendRepository 创建 workflow_task_history_extend 仓储。
// @param store 仓储入口
// @return 仓储
func NewHistoryExtendRepository(store *Store) HistoryExtendRepository {
	return HistoryExtendRepository{baseRepo{store: store, def: tableHistoryExtends}}
}

// GetByTask 按任务 ID 查询已办扩展。
// @param ctx 操作上下文
// @param taskID 任务 ID
// @return 行数据
// @return 是否存在
// @return 查询错误
func (r HistoryExtendRepository) GetByTask(ctx context.Context, taskID string) (Row, bool, error) {
	return r.getFirst(ctx, "task_id = "+r.store.ph(1), []any{taskID}, "")
}

// ListByProcessInstance 查询实例下全部已办扩展（按办结时间排序，聚合已办轨迹）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 ID
// @return 行列表
// @return 查询错误
func (r HistoryExtendRepository) ListByProcessInstance(ctx context.Context, processInstanceID string) ([]Row, error) {
	return r.list(ctx, "process_instance_id = "+r.store.ph(1), []any{processInstanceID}, "task_finish_time")
}

// ListByBusiness 按业务单据与租户查询已办扩展。
// @param ctx 操作上下文
// @param businessID 业务单据 ID
// @param tenantID 租户
// @return 行列表
// @return 查询错误
func (r HistoryExtendRepository) ListByBusiness(ctx context.Context, businessID, tenantID string) ([]Row, error) {
	where := "business_id = " + r.store.ph(1) + " AND tenant_id = " + r.store.ph(2)
	return r.list(ctx, where, []any{businessID, tenantID}, "task_finish_time")
}
