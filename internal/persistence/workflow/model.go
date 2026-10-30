package workflow

import "context"

// ModelRepository 是 workflow_model（流程模型/分类主数据）的仓储。
type ModelRepository struct {
	baseRepo
}

// NewModelRepository 创建 workflow_model 仓储。
// @param store 仓储入口
// @return 仓储
func NewModelRepository(store *Store) ModelRepository {
	return ModelRepository{baseRepo{store: store, def: tableModels}}
}

// GetByModelCode 按模型编码（=流程定义 key）与租户查询模型（分类页与模型页共用）。
// @param ctx 操作上下文
// @param modelCode 模型编码
// @param tenantID 租户
// @return 行数据
// @return 是否存在
// @return 查询错误
func (r ModelRepository) GetByModelCode(ctx context.Context, modelCode, tenantID string) (Row, bool, error) {
	where := "model_code = " + r.store.ph(1) + " AND tenant_id = " + r.store.ph(2)
	return r.getFirst(ctx, where, []any{modelCode, tenantID}, "")
}

// ListByParent 查询指定父节点下的子模型/子分类。
// @param ctx 操作上下文
// @param parentID 父节点 ID
// @return 行列表
// @return 查询错误
func (r ModelRepository) ListByParent(ctx context.Context, parentID string) ([]Row, error) {
	return r.list(ctx, "parent_id = "+r.store.ph(1), []any{parentID}, "sort")
}

// ListByTenant 查询租户下全部模型与分类。
// @param ctx 操作上下文
// @param tenantID 租户
// @return 行列表
// @return 查询错误
func (r ModelRepository) ListByTenant(ctx context.Context, tenantID string) ([]Row, error) {
	return r.list(ctx, "tenant_id = "+r.store.ph(1), []any{tenantID}, "sort")
}

// UpdateLastDeployTime 回写模型最近部署时间。
// @param ctx 操作上下文
// @param id 模型 ID
// @param deployTime 部署时间
// @return 影响行数
// @return 更新错误
func (r ModelRepository) UpdateLastDeployTime(ctx context.Context, id string, deployTime any) (int64, error) {
	return r.UpdateByPK(ctx, id, Row{"last_deploy_time": deployTime})
}
