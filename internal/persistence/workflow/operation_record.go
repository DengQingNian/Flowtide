package workflow

import "context"

// OperationRecordRepository 是 workflow_operation_record（操作记录/意见留痕）的仓储。
type OperationRecordRepository struct {
	baseRepo
}

// NewOperationRecordRepository 创建 workflow_operation_record 仓储。
// @param store 仓储入口
// @return 仓储
func NewOperationRecordRepository(store *Store) OperationRecordRepository {
	return OperationRecordRepository{baseRepo{store: store, def: tableOperationRecords}}
}

// ListByTask 查询任务的操作记录。
// @param ctx 操作上下文
// @param taskID 任务 ID
// @return 行列表
// @return 查询错误
func (r OperationRecordRepository) ListByTask(ctx context.Context, taskID string) ([]Row, error) {
	return r.list(ctx, "task_id = "+r.store.ph(1), []any{taskID}, "create_time")
}

// ListByBusiness 查询业务单据的操作记录。
// @param ctx 操作上下文
// @param businessID 业务单据 ID
// @return 行列表
// @return 查询错误
func (r OperationRecordRepository) ListByBusiness(ctx context.Context, businessID string) ([]Row, error) {
	return r.list(ctx, "business_id = "+r.store.ph(1), []any{businessID}, "create_time")
}

// ListByProcessInstance 查询实例的操作记录。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 ID
// @return 行列表
// @return 查询错误
func (r OperationRecordRepository) ListByProcessInstance(ctx context.Context, processInstanceID string) ([]Row, error) {
	return r.list(ctx, "process_instance_id = "+r.store.ph(1), []any{processInstanceID}, "create_time")
}
