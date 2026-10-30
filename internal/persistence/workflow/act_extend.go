package workflow

import "context"

// ActExtendRepository 是 workflow_act_extend（活动扩展，审阅人绑定）的仓储。
type ActExtendRepository struct {
	baseRepo
}

// NewActExtendRepository 创建 workflow_act_extend 仓储。
// @param store 仓储入口
// @return 仓储
func NewActExtendRepository(store *Store) ActExtendRepository {
	return ActExtendRepository{baseRepo{store: store, def: tableActExtends}}
}

// ListByStep 查询步骤下绑定的活动扩展。
// @param ctx 操作上下文
// @param stepID 步骤 ID（→workflow_step.id）
// @return 行列表
// @return 查询错误
func (r ActExtendRepository) ListByStep(ctx context.Context, stepID string) ([]Row, error) {
	return r.list(ctx, "step_id = "+r.store.ph(1), []any{stepID}, "activity_id")
}

// ListByDefinitionAndActivity 按流程定义 key 与 BPMN 节点查询绑定（运行期审阅人读取入口）。
// @param ctx 操作上下文
// @param definitionKey 流程定义 key
// @param activityID BPMN 节点 ID
// @return 行列表
// @return 查询错误
func (r ActExtendRepository) ListByDefinitionAndActivity(ctx context.Context, definitionKey, activityID string) ([]Row, error) {
	where := "process_definition_key = " + r.store.ph(1) + " AND activity_id = " + r.store.ph(2)
	return r.list(ctx, where, []any{definitionKey, activityID}, "")
}
