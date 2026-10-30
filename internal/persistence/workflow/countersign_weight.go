package workflow

import "context"

// CountersignWeightRepository 是 workflow_countersign_weight（会签权重配置）的仓储。
type CountersignWeightRepository struct {
	baseRepo
}

// NewCountersignWeightRepository 创建 workflow_countersign_weight 仓储。
// @param store 仓储入口
// @return 仓储
func NewCountersignWeightRepository(store *Store) CountersignWeightRepository {
	return CountersignWeightRepository{baseRepo{store: store, def: tableCountersignWeights}}
}

// ListByDefinitionAndActivity 按流程定义 key 与节点查询权重配置（节点×身份）。
// @param ctx 操作上下文
// @param definitionKey 流程定义 key
// @param activityID BPMN 节点 ID
// @return 行列表
// @return 查询错误
func (r CountersignWeightRepository) ListByDefinitionAndActivity(ctx context.Context, definitionKey, activityID string) ([]Row, error) {
	where := "process_definition_key = " + r.store.ph(1) + " AND activity_id = " + r.store.ph(2)
	return r.list(ctx, where, []any{definitionKey, activityID}, "identity_id")
}

// ListByIdentity 查询身份参与的权重配置。
// @param ctx 操作上下文
// @param definitionKey 流程定义 key
// @param identityID 身份 ID
// @return 行列表
// @return 查询错误
func (r CountersignWeightRepository) ListByIdentity(ctx context.Context, definitionKey, identityID string) ([]Row, error) {
	where := "process_definition_key = " + r.store.ph(1) + " AND identity_id = " + r.store.ph(2)
	return r.list(ctx, where, []any{definitionKey, identityID}, "")
}
