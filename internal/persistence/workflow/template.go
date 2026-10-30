package workflow

import "context"

// TemplateRepository 是 workflow_template（设计器模板）的仓储。
type TemplateRepository struct {
	baseRepo
}

// NewTemplateRepository 创建 workflow_template 仓储。
// @param store 仓储入口
// @return 仓储
func NewTemplateRepository(store *Store) TemplateRepository {
	return TemplateRepository{baseRepo{store: store, def: tableTemplates}}
}

// ListByCategory 查询分类下的设计器模板。
// @param ctx 操作上下文
// @param categoryID 分类 ID
// @return 行列表
// @return 查询错误
func (r TemplateRepository) ListByCategory(ctx context.Context, categoryID string) ([]Row, error) {
	return r.list(ctx, "category_id = "+r.store.ph(1), []any{categoryID}, "name")
}

// ListDefault 查询默认模板。
// @param ctx 操作上下文
// @return 行列表
// @return 查询错误
func (r TemplateRepository) ListDefault(ctx context.Context) ([]Row, error) {
	return r.list(ctx, "is_default = 1 AND is_visible = 1", nil, "name")
}
