package workflow

import "context"

// HisStepRepository 是 workflow_his_step（步骤部署快照，运行期规则字典）的仓储。
type HisStepRepository struct {
	baseRepo
}

// NewHisStepRepository 创建 workflow_his_step 仓储。
// @param store 仓储入口
// @return 仓储
func NewHisStepRepository(store *Store) HisStepRepository {
	return HisStepRepository{baseRepo{store: store, def: tableHisSteps}}
}

// ListByDeployment 查询某次部署的全部步骤快照（部署后按 deploymentId 读取规则）。
// @param ctx 操作上下文
// @param deploymentID 部署 ID（→ACT_RE_PROCDEF.DEPLOYMENT_ID_）
// @return 行列表
// @return 查询错误
func (r HisStepRepository) ListByDeployment(ctx context.Context, deploymentID string) ([]Row, error) {
	return r.list(ctx, "deployment_id = "+r.store.ph(1), []any{deploymentID}, "sort")
}

// ListByModel 查询模型的历史步骤快照。
// @param ctx 操作上下文
// @param modelID 模型 ID
// @return 行列表
// @return 查询错误
func (r HisStepRepository) ListByModel(ctx context.Context, modelID string) ([]Row, error) {
	return r.list(ctx, "model_id = "+r.store.ph(1), []any{modelID}, "sort")
}

// GetByDeploymentAndStepCode 按部署与步骤代号定位快照行。
// @param ctx 操作上下文
// @param deploymentID 部署 ID
// @param stepCode 步骤代号
// @return 行数据
// @return 是否存在
// @return 查询错误
func (r HisStepRepository) GetByDeploymentAndStepCode(ctx context.Context, deploymentID, stepCode string) (Row, bool, error) {
	where := "deployment_id = " + r.store.ph(1) + " AND step_code = " + r.store.ph(2)
	return r.getFirst(ctx, where, []any{deploymentID, stepCode}, "")
}
