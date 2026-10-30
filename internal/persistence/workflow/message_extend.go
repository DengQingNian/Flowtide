package workflow

import "context"

// MessageExtendRepository 是 workflow_task_message_extend（待办扩展，待办中心唯一数据源）的仓储。
type MessageExtendRepository struct {
	baseRepo
}

// NewMessageExtendRepository 创建 workflow_task_message_extend 仓储。
// @param store 仓储入口
// @return 仓储
func NewMessageExtendRepository(store *Store) MessageExtendRepository {
	return MessageExtendRepository{baseRepo{store: store, def: tableMessageExtends}}
}

// GetByTask 按任务 ID 查询待办扩展（与 ACT_RU_TASK 1:1）。
// @param ctx 操作上下文
// @param taskID 任务 ID
// @return 行数据
// @return 是否存在
// @return 查询错误
func (r MessageExtendRepository) GetByTask(ctx context.Context, taskID string) (Row, bool, error) {
	return r.getFirst(ctx, "task_id = "+r.store.ph(1), []any{taskID}, "")
}

// ListByProcessInstance 查询实例下全部待办扩展。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 ID
// @return 行列表
// @return 查询错误
func (r MessageExtendRepository) ListByProcessInstance(ctx context.Context, processInstanceID string) ([]Row, error) {
	return r.list(ctx, "process_instance_id = "+r.store.ph(1), []any{processInstanceID}, "create_time")
}

// ListByBusiness 按业务单据与租户查询待办扩展。
// @param ctx 操作上下文
// @param businessID 业务单据 ID
// @param tenantID 租户
// @return 行列表
// @return 查询错误
func (r MessageExtendRepository) ListByBusiness(ctx context.Context, businessID, tenantID string) ([]Row, error) {
	where := "business_id = " + r.store.ph(1) + " AND tenant_id = " + r.store.ph(2)
	return r.list(ctx, where, []any{businessID, tenantID}, "create_time")
}

// ListByOwner 按处理人查询待办扩展。
// @param ctx 操作上下文
// @param ownerID 处理人身份 ID
// @return 行列表
// @return 查询错误
func (r MessageExtendRepository) ListByOwner(ctx context.Context, ownerID string) ([]Row, error) {
	return r.list(ctx, "owner_id = "+r.store.ph(1), []any{ownerID}, "create_time")
}

// DeleteByTask 删除任务的待办扩展（办结流转时先删后增）。
// @param ctx 操作上下文
// @param taskID 任务 ID
// @return 影响行数
// @return 删除错误
func (r MessageExtendRepository) DeleteByTask(ctx context.Context, taskID string) (int64, error) {
	return r.deleteWhere(ctx, "task_id = "+r.store.ph(1), []any{taskID})
}

// DeleteByProcessInstanceAndSteps 删除实例下指定步骤代号的待办扩展（退回/撤回清理）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 ID
// @param stepCodes 步骤代号集合
// @return 影响行数
// @return 删除错误
func (r MessageExtendRepository) DeleteByProcessInstanceAndSteps(ctx context.Context, processInstanceID string, stepCodes []string) (int64, error) {
	// 空集合无需删除，直接返回避免生成非法 IN 子句
	if len(stepCodes) == 0 {
		return 0, nil
	}
	in, inArgs := r.store.buildInClause("task_step_code", 2, stepCodes)
	where := "process_instance_id = " + r.store.ph(1) + " AND " + in
	args := append([]any{processInstanceID}, inArgs...)
	return r.deleteWhere(ctx, where, args)
}

// MarkRead 标记待办已读并回写阅读时间。
// @param ctx 操作上下文
// @param taskID 任务 ID
// @param readTime 阅读时间
// @return 影响行数
// @return 更新错误
func (r MessageExtendRepository) MarkRead(ctx context.Context, taskID string, readTime any) (int64, error) {
	return r.updateWhere(ctx, Row{"is_read": 1, "read_time": readTime}, "task_id = "+r.store.ph(1), []any{taskID})
}
