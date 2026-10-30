package workflow

import "context"

// TaskDelegateRepository 是 workflow_task_delegate（委托规则主表）的仓储。
// condition 为保留字列（MySQL 反引号 / PostgreSQL 双引号），vaild_from/vaild_to 保留 Java 拼写。
type TaskDelegateRepository struct {
	baseRepo
}

// NewTaskDelegateRepository 创建 workflow_task_delegate 仓储。
// @param store 仓储入口
// @return 仓储
func NewTaskDelegateRepository(store *Store) TaskDelegateRepository {
	return TaskDelegateRepository{baseRepo{store: store, def: tableTaskDelegates}}
}

// ListByOwner 查询委托人启用的委托规则（命中优先级由服务层按范围类型计算）。
// @param ctx 操作上下文
// @param ownerID 委托人 ID
// @return 行列表
// @return 查询错误
func (r TaskDelegateRepository) ListByOwner(ctx context.Context, ownerID string) ([]Row, error) {
	return r.list(ctx, "owner_id = "+r.store.ph(1)+" AND enable = 1", []any{ownerID}, "create_time")
}

// ListBySourceCode 按来源租户/系统查询委托规则。
// @param ctx 操作上下文
// @param sourceCode 来源代号
// @return 行列表
// @return 查询错误
func (r TaskDelegateRepository) ListBySourceCode(ctx context.Context, sourceCode string) ([]Row, error) {
	return r.list(ctx, "source_code = "+r.store.ph(1)+" AND enable = 1", []any{sourceCode}, "create_time")
}

// Retire 按主键退休委托规则（retire=1，退休委托语义）。
// @param ctx 操作上下文
// @param id 委托规则 ID
// @return 影响行数
// @return 更新错误
func (r TaskDelegateRepository) Retire(ctx context.Context, id string) (int64, error) {
	return r.updateWhere(ctx, Row{"retire": 1, "enable": 0}, "id = "+r.store.ph(1), []any{id})
}
