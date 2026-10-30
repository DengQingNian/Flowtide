package workflow

import "context"

// TaskConsignersRepository 是 workflow_task_consigners（被委托人）的仓储。
type TaskConsignersRepository struct {
	baseRepo
}

// NewTaskConsignersRepository 创建 workflow_task_consigners 仓储。
// @param store 仓储入口
// @return 仓储
func NewTaskConsignersRepository(store *Store) TaskConsignersRepository {
	return TaskConsignersRepository{baseRepo{store: store, def: tableTaskConsigners}}
}

// ListByDelegate 查询委托规则的被委托人列表。
// @param ctx 操作上下文
// @param delegateID 委托规则 ID
// @return 行列表
// @return 查询错误
func (r TaskConsignersRepository) ListByDelegate(ctx context.Context, delegateID string) ([]Row, error) {
	return r.list(ctx, "delegate_id = "+r.store.ph(1), []any{delegateID}, "create_time")
}

// DeleteByDelegate 删除委托规则的全部被委托人（规则重建时先删后增）。
// @param ctx 操作上下文
// @param delegateID 委托规则 ID
// @return 影响行数
// @return 删除错误
func (r TaskConsignersRepository) DeleteByDelegate(ctx context.Context, delegateID string) (int64, error) {
	return r.deleteWhere(ctx, "delegate_id = "+r.store.ph(1), []any{delegateID})
}

// TaskConsignObjRepository 是 workflow_task_consign_obj（委托对象）的仓储。
type TaskConsignObjRepository struct {
	baseRepo
}

// NewTaskConsignObjRepository 创建 workflow_task_consign_obj 仓储。
// @param store 仓储入口
// @return 仓储
func NewTaskConsignObjRepository(store *Store) TaskConsignObjRepository {
	return TaskConsignObjRepository{baseRepo{store: store, def: tableTaskConsignObjs}}
}

// ListByDelegate 查询委托规则的委托对象列表。
// @param ctx 操作上下文
// @param delegateID 委托规则 ID
// @return 行列表
// @return 查询错误
func (r TaskConsignObjRepository) ListByDelegate(ctx context.Context, delegateID string) ([]Row, error) {
	return r.list(ctx, "delegate_id = "+r.store.ph(1), []any{delegateID}, "create_time")
}

// DeleteByDelegate 删除委托规则的全部委托对象（规则重建时先删后增）。
// @param ctx 操作上下文
// @param delegateID 委托规则 ID
// @return 影响行数
// @return 删除错误
func (r TaskConsignObjRepository) DeleteByDelegate(ctx context.Context, delegateID string) (int64, error) {
	return r.deleteWhere(ctx, "delegate_id = "+r.store.ph(1), []any{delegateID})
}
