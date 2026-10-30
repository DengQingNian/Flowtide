package workflow

import "context"

// StepRepository 是 workflow_step（设计时步骤配置）的仓储。
type StepRepository struct {
	baseRepo
}

// NewStepRepository 创建 workflow_step 仓储。
// @param store 仓储入口
// @return 仓储
func NewStepRepository(store *Store) StepRepository {
	return StepRepository{baseRepo{store: store, def: tableSteps}}
}

// ListByModel 查询模型下全部步骤配置（按 sort 升序）。
// @param ctx 操作上下文
// @param modelID 模型 ID
// @return 行列表
// @return 查询错误
func (r StepRepository) ListByModel(ctx context.Context, modelID string) ([]Row, error) {
	return r.list(ctx, "model_id = "+r.store.ph(1), []any{modelID}, "sort")
}

// GetByModelAndStepCode 按步骤代号查询单个步骤配置。
// @param ctx 操作上下文
// @param modelID 模型 ID
// @param stepCode 步骤代号
// @return 行数据
// @return 是否存在
// @return 查询错误
func (r StepRepository) GetByModelAndStepCode(ctx context.Context, modelID, stepCode string) (Row, bool, error) {
	where := "model_id = " + r.store.ph(1) + " AND step_code = " + r.store.ph(2)
	return r.getFirst(ctx, where, []any{modelID, stepCode}, "sort")
}
