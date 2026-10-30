package workflow

import "context"

// BusinessSystemRepository 是 workflow_business_system（业务系统注册，替代 auth 表）的仓储。
type BusinessSystemRepository struct {
	baseRepo
}

// NewBusinessSystemRepository 创建 workflow_business_system 仓储。
// @param store 仓储入口
// @return 仓储
func NewBusinessSystemRepository(store *Store) BusinessSystemRepository {
	return BusinessSystemRepository{baseRepo{store: store, def: tableBusinessSystems}}
}

// GetBySymbol 按系统代号查询注册信息（system_symbol 唯一）。
// @param ctx 操作上下文
// @param systemSymbol 系统代号（通常即租户）
// @return 行数据
// @return 是否存在
// @return 查询错误
func (r BusinessSystemRepository) GetBySymbol(ctx context.Context, systemSymbol string) (Row, bool, error) {
	return r.getFirst(ctx, "system_symbol = "+r.store.ph(1), []any{systemSymbol}, "")
}

// ListAll 查询全部业务系统注册。
// @param ctx 操作上下文
// @return 行列表
// @return 查询错误
func (r BusinessSystemRepository) ListAll(ctx context.Context) ([]Row, error) {
	return r.list(ctx, "1 = 1", nil, "system_symbol")
}
