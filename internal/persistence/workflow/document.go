package workflow

import "context"

// DocumentRepository 是 workflow_document（业务单据↔流程实例绑定与状态快照）的仓储。
// business_id 在租户内唯一（uk_wf_document_business），查询统一过滤 deleted = 0。
type DocumentRepository struct {
	baseRepo
}

// NewDocumentRepository 创建 workflow_document 仓储。
// @param store 仓储入口
// @return 仓储
func NewDocumentRepository(store *Store) DocumentRepository {
	return DocumentRepository{baseRepo{store: store, def: tableDocuments}}
}

// UpsertByBusinessID 按业务单据与租户保存在单据状态：存在则更新，不存在则插入。
// 需在 WithTx 中调用以保证查询与写入的原子性。
// @param ctx 操作上下文
// @param row 单据行（必须含 id、business_id、tenant_id）
// @return 保存错误
func (r DocumentRepository) UpsertByBusinessID(ctx context.Context, row Row) error {
	businessID, _ := row["business_id"].(string)
	tenantID, _ := row["tenant_id"].(string)
	// upsert 依赖 business_id + tenant_id 唯一键定位既有行
	existing, found, err := r.GetByBusinessID(ctx, businessID, tenantID)
	if err != nil {
		return err
	}
	if !found {
		return r.Insert(ctx, row)
	}
	changes := make(Row, len(row))
	for key, value := range row {
		// 主键与创建列不参与更新
		if key == "id" || key == "create_time" {
			continue
		}
		changes[key] = value
	}
	_, err = r.UpdateByPK(ctx, existing["id"].(string), changes)
	return err
}

// GetByBusinessID 按业务单据与租户查询未删除的单据状态。
// @param ctx 操作上下文
// @param businessID 业务单据 ID
// @param tenantID 租户
// @return 行数据
// @return 是否存在
// @return 查询错误
func (r DocumentRepository) GetByBusinessID(ctx context.Context, businessID, tenantID string) (Row, bool, error) {
	where := "business_id = " + r.store.ph(1) + " AND tenant_id = " + r.store.ph(2) + " AND deleted = 0"
	return r.getFirst(ctx, where, []any{businessID, tenantID}, "")
}

// ListByFlowInstance 按流程实例查询未删除的单据状态。
// @param ctx 操作上下文
// @param instanceID 流程实例 ID
// @return 行列表
// @return 查询错误
func (r DocumentRepository) ListByFlowInstance(ctx context.Context, instanceID string) ([]Row, error) {
	return r.list(ctx, "flow_instance_id = "+r.store.ph(1)+" AND deleted = 0", []any{instanceID}, "create_time")
}

// MarkDeletedByBusinessID 按业务单据与租户逻辑删除（deleted=1，手工过滤语义）。
// @param ctx 操作上下文
// @param businessID 业务单据 ID
// @param tenantID 租户
// @return 影响行数
// @return 更新错误
func (r DocumentRepository) MarkDeletedByBusinessID(ctx context.Context, businessID, tenantID string) (int64, error) {
	where := "business_id = " + r.store.ph(1) + " AND tenant_id = " + r.store.ph(2) + " AND deleted = 0"
	return r.updateWhere(ctx, Row{"deleted": 1}, where, []any{businessID, tenantID})
}
