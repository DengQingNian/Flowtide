package workflow

import "context"

// RemindRepository 是 workflow_remind（催办配置主表）的仓储。查询统一过滤 deleted = 0。
type RemindRepository struct {
	baseRepo
}

// NewRemindRepository 创建 workflow_remind 仓储。
// @param store 仓储入口
// @return 仓储
func NewRemindRepository(store *Store) RemindRepository {
	return RemindRepository{baseRepo{store: store, def: tableReminds}}
}

// ListByModel 查询模型的催办配置。
// @param ctx 操作上下文
// @param modelID 模型 ID（pid → workflow_model.id）
// @return 行列表
// @return 查询错误
func (r RemindRepository) ListByModel(ctx context.Context, modelID string) ([]Row, error) {
	return r.list(ctx, "pid = "+r.store.ph(1)+" AND deleted = 0", []any{modelID}, "create_time")
}

// MarkSent 标记催办配置已发送。
// @param ctx 操作上下文
// @param id 催办配置 ID
// @return 影响行数
// @return 更新错误
func (r RemindRepository) MarkSent(ctx context.Context, id string) (int64, error) {
	return r.updateWhere(ctx, Row{"sent": 1}, "id = "+r.store.ph(1), []any{id})
}

// RemindTargetRepository 是 workflow_remind_target（催办对象）的仓储。查询统一过滤 deleted = 0。
type RemindTargetRepository struct {
	baseRepo
}

// NewRemindTargetRepository 创建 workflow_remind_target 仓储。
// @param store 仓储入口
// @return 仓储
func NewRemindTargetRepository(store *Store) RemindTargetRepository {
	return RemindTargetRepository{baseRepo{store: store, def: tableRemindTargets}}
}

// ListByRemind 查询催办配置的对象列表。
// @param ctx 操作上下文
// @param remindID 催办配置 ID（pid → workflow_remind.id）
// @return 行列表
// @return 查询错误
func (r RemindTargetRepository) ListByRemind(ctx context.Context, remindID string) ([]Row, error) {
	return r.list(ctx, "pid = "+r.store.ph(1)+" AND deleted = 0", []any{remindID}, "create_time")
}

// DeleteByRemind 删除催办配置的全部对象（配置重建时先删后增）。
// @param ctx 操作上下文
// @param remindID 催办配置 ID
// @return 影响行数
// @return 删除错误
func (r RemindTargetRepository) DeleteByRemind(ctx context.Context, remindID string) (int64, error) {
	return r.deleteWhere(ctx, "pid = "+r.store.ph(1), []any{remindID})
}

// MarkSent 标记催办对象已发送。
// @param ctx 操作上下文
// @param id 催办对象 ID
// @return 影响行数
// @return 更新错误
func (r RemindTargetRepository) MarkSent(ctx context.Context, id string) (int64, error) {
	return r.updateWhere(ctx, Row{"sent": 1}, "id = "+r.store.ph(1), []any{id})
}
