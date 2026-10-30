package workflow

import "context"

// HisNextActivityRepository 是 workflow_his_next_activity（下一步历史，双写落库）的仓储。
type HisNextActivityRepository struct {
	baseRepo
}

// NewHisNextActivityRepository 创建 workflow_his_next_activity 仓储。
// @param store 仓储入口
// @return 仓储
func NewHisNextActivityRepository(store *Store) HisNextActivityRepository {
	return HisNextActivityRepository{baseRepo{store: store, def: tableHisNextActivities}}
}

// ListByProcessInstance 查询实例的全部下一步历史（轨迹回放）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 ID
// @return 行列表
// @return 查询错误
func (r HisNextActivityRepository) ListByProcessInstance(ctx context.Context, processInstanceID string) ([]Row, error) {
	return r.list(ctx, "process_instance_id = "+r.store.ph(1), []any{processInstanceID}, "version")
}

// ListByActivityByVersionDesc 查询实例下指定节点的历史记录（版本号降序，便于取最新版本）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 ID
// @param nextActivityID 下一活动 ID
// @return 行列表
// @return 查询错误
func (r HisNextActivityRepository) ListByActivityByVersionDesc(ctx context.Context, processInstanceID, nextActivityID string) ([]Row, error) {
	where := "process_instance_id = " + r.store.ph(1) + " AND next_activity_id = " + r.store.ph(2)
	return r.list(ctx, where, []any{processInstanceID, nextActivityID}, "version DESC")
}

// MarkUsed 标记历史记录已消耗并回写任务 ID。
// @param ctx 操作上下文
// @param id 记录 ID
// @param taskID 任务 ID
// @return 影响行数
// @return 更新错误
func (r HisNextActivityRepository) MarkUsed(ctx context.Context, id, taskID string) (int64, error) {
	return r.updateWhere(ctx, Row{"used": 1, "task_id": taskID}, "id = "+r.store.ph(1)+" AND used = 0", []any{id})
}

// DeleteByProcessInstanceAndActivities 按实例与活动集合删除历史记录（退回/撤回规则清理）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 ID
// @param nextActivityIDs 下一活动 ID 集合
// @return 影响行数
// @return 删除错误
func (r HisNextActivityRepository) DeleteByProcessInstanceAndActivities(ctx context.Context, processInstanceID string, nextActivityIDs []string) (int64, error) {
	// 空集合无需删除，直接返回避免生成非法 IN 子句
	if len(nextActivityIDs) == 0 {
		return 0, nil
	}
	in, inArgs := r.store.buildInClause("next_activity_id", 2, nextActivityIDs)
	where := "process_instance_id = " + r.store.ph(1) + " AND " + in
	args := append([]any{processInstanceID}, inArgs...)
	return r.deleteWhere(ctx, where, args)
}
