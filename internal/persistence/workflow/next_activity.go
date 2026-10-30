package workflow

import "context"

// NextActivityRepository 是 workflow_next_activity（下一步节点及处理人，预生成-消耗核心记账表）的仓储。
type NextActivityRepository struct {
	baseRepo
}

// NewNextActivityRepository 创建 workflow_next_activity 仓储。
// @param store 仓储入口
// @return 仓储
func NewNextActivityRepository(store *Store) NextActivityRepository {
	return NextActivityRepository{baseRepo{store: store, def: tableNextActivities}}
}

// ListUnusedByActivity 查询实例下指定节点尚未消耗（used=0）的下一步记录。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 ID
// @param nextActivityID 下一活动 ID
// @return 行列表
// @return 查询错误
func (r NextActivityRepository) ListUnusedByActivity(ctx context.Context, processInstanceID, nextActivityID string) ([]Row, error) {
	where := "process_instance_id = " + r.store.ph(1) + " AND next_activity_id = " + r.store.ph(2) + " AND used = 0"
	return r.list(ctx, where, []any{processInstanceID, nextActivityID}, "version")
}

// ListUnusedByProcessInstance 查询实例下全部未消耗记录（执行人重写钩子读取）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 ID
// @return 行列表
// @return 查询错误
func (r NextActivityRepository) ListUnusedByProcessInstance(ctx context.Context, processInstanceID string) ([]Row, error) {
	return r.list(ctx, "process_instance_id = "+r.store.ph(1)+" AND used = 0", []any{processInstanceID}, "version")
}

// MarkUsed 标记记录已消耗并回写任务 ID（仅未消耗记录生效，防并发重复消耗）。
// @param ctx 操作上下文
// @param id 记录 ID
// @param taskID 任务 ID（→ACT_RU_TASK.ID_）
// @return 影响行数
// @return 更新错误
func (r NextActivityRepository) MarkUsed(ctx context.Context, id, taskID string) (int64, error) {
	// WHERE 相对占位符：used = $1
	return r.updateWhere(ctx, Row{"used": 1, "task_id": taskID}, "id = "+r.store.ph(1)+" AND used = 0", []any{id})
}

// ListByPreTask 查询以上一任务为来源的下一步记录（退回/撤回清理入口）。
// @param ctx 操作上下文
// @param preTaskID 上一任务 ID
// @return 行列表
// @return 查询错误
func (r NextActivityRepository) ListByPreTask(ctx context.Context, preTaskID string) ([]Row, error) {
	return r.list(ctx, "pre_task_id = "+r.store.ph(1), []any{preTaskID}, "version")
}

// ListByTask 查询已挂到任务上的记录。
// @param ctx 操作上下文
// @param taskID 任务 ID
// @return 行列表
// @return 查询错误
func (r NextActivityRepository) ListByTask(ctx context.Context, taskID string) ([]Row, error) {
	return r.list(ctx, "task_id = "+r.store.ph(1), []any{taskID}, "")
}

// DeleteByProcessInstance 删除实例的全部未消耗预生成记录。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 ID
// @return 影响行数
// @return 删除错误
func (r NextActivityRepository) DeleteByProcessInstance(ctx context.Context, processInstanceID string) (int64, error) {
	return r.deleteWhere(ctx, "process_instance_id = "+r.store.ph(1), []any{processInstanceID})
}

// DeleteByProcessInstanceAndActivities 删除实例下指定活动集合的记录（退回/转办清理）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 ID
// @param nextActivityIDs 下一活动 ID 集合
// @return 影响行数
// @return 删除错误
func (r NextActivityRepository) DeleteByProcessInstanceAndActivities(ctx context.Context, processInstanceID string, nextActivityIDs []string) (int64, error) {
	// 空集合无需删除，直接返回避免生成非法 IN 子句
	if len(nextActivityIDs) == 0 {
		return 0, nil
	}
	in, inArgs := r.store.buildInClause("next_activity_id", 2, nextActivityIDs)
	where := "process_instance_id = " + r.store.ph(1) + " AND " + in
	args := append([]any{processInstanceID}, inArgs...)
	return r.deleteWhere(ctx, where, args)
}

// InsertDual 双写运行态与历史表（同 ID 同内容），需在 WithTx 中调用以保证原子性。
// @param ctx 操作上下文
// @param row 下一步记录（两表共用同一行数据）
// @return 双写错误
func (r NextActivityRepository) InsertDual(ctx context.Context, row Row) error {
	// 双写是"预生成-消耗"模型的记账基础，事务保证两表一致
	if err := r.Insert(ctx, row); err != nil {
		return err
	}
	history := NewHisNextActivityRepository(r.store)
	return history.Insert(ctx, row)
}
