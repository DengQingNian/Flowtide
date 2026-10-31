package workflow

import (
	"context"
	"database/sql"

	pwf "github.com/DengQingNian/Flowtide/internal/persistence/workflow"
)

// SQLReviewStores 是审阅域两个存储端口 SQL 适配器的聚合
// （审阅影子任务 + 审阅人绑定），复用 store_sql.go 的 sqlCore 共享内核。
type SQLReviewStores struct {
	Tasks   *SQLReviewTaskStore // 审阅影子任务
	Extends *SQLActExtendStore  // 审阅人绑定（活动扩展）
}

// NewSQLReviewStores 创建审阅域 SQL 适配器（不带裸 SQL 补齐通道）。
// @param store workflow 仓储入口
// @return 适配器聚合
func NewSQLReviewStores(store *pwf.Store) *SQLReviewStores {
	return NewSQLReviewStoresWithDB(store, nil)
}

// NewSQLReviewStoresWithDB 创建审阅域 SQL 适配器并附带裸 SQL 只读补齐通道。
// ListByReviewer / CountUnread / ListByDefinitionKey 在 pwf 仓储暂无对应方法，
// 经裸 SQL 下推补齐（模式与 store_sql.go 的仓储缺口补齐一致）。
// @param store workflow 仓储入口
// @param db 连接池（与 store 共用同一连接池）
// @return 适配器聚合
func NewSQLReviewStoresWithDB(store *pwf.Store, db *sql.DB) *SQLReviewStores {
	core := &sqlCore{store: store, repos: pwf.NewRepositories(store), db: db}
	return &SQLReviewStores{
		Tasks:   &SQLReviewTaskStore{core: core},
		Extends: &SQLActExtendStore{core: core},
	}
}

// 编译期断言：SQL 适配器满足审阅域两个存储端口。
var (
	_ ReviewTaskStore = (*SQLReviewTaskStore)(nil)
	_ ActExtendStore  = (*SQLActExtendStore)(nil)
)

// ─── ReviewTask 行转换 ─────────────────────────────────────────────

// rvReviewToRow 将审阅影子任务实体转为通用行。
// @param rt 审阅影子任务实体
// @return 通用行
func rvReviewToRow(rt *ReviewTask) pwf.Row {
	row := pwf.Row{
		"task_id":                 rt.TaskID,
		"task_name":               rt.TaskName,
		"process_instance_id":     rt.ProcessInstanceID,
		"process_definition_id":   rt.ProcessDefinitionID,
		"task_key":                rt.TaskKey,
		"business_key":            rt.BusinessKey,
		"process_name":            rt.ProcessName,
		"process_definition_key":  rt.ProcessDefinitionKey,
		"process_definition_name": rt.ProcessDefinitionName,
		"sender":                  rt.SenderID,
		"sender_name":             rt.SenderName,
		"reviewer":                rt.Reviewer,
		"remark":                  rt.Remark,
		"opinion":                 rt.Opinion,
		"status":                  rt.Status,
	}
	// 可空时间列仅在有值时落库，保持 NULL 语义
	if !rt.StartTime.IsZero() {
		row["start_time"] = rt.StartTime
	}
	if rt.ReviewTime != nil {
		row["review_time"] = *rt.ReviewTime
	}
	appendBaseRow(row, rt.BaseColumns)
	return row
}

// rvReviewFromRow 将通用行转为审阅影子任务实体。
// @param row 通用行
// @return 审阅影子任务实体
func rvReviewFromRow(row pwf.Row) *ReviewTask {
	return &ReviewTask{
		BaseColumns:           baseFromRow(row),
		StartTime:             timeOrZero(row, "start_time"),
		TaskID:                rowString(row, "task_id"),
		TaskName:              rowString(row, "task_name"),
		ProcessInstanceID:     rowString(row, "process_instance_id"),
		ProcessDefinitionID:   rowString(row, "process_definition_id"),
		TaskKey:               rowString(row, "task_key"),
		BusinessKey:           rowString(row, "business_key"),
		ProcessName:           rowString(row, "process_name"),
		ProcessDefinitionKey:  rowString(row, "process_definition_key"),
		ProcessDefinitionName: rowString(row, "process_definition_name"),
		SenderID:              rowString(row, "sender"),
		SenderName:            rowString(row, "sender_name"),
		Reviewer:              rowString(row, "reviewer"),
		ReviewTime:            rowTimePtr(row, "review_time"),
		Remark:                rowString(row, "remark"),
		Opinion:               rowString(row, "opinion"),
		Status:                rowInt(row, "status"),
	}
}

// rvReviewsFromRows 批量转换审阅影子任务行。
// @param rows 通用行列表
// @return 实体列表
func rvReviewsFromRows(rows []pwf.Row) []*ReviewTask {
	out := make([]*ReviewTask, 0, len(rows))
	for _, row := range rows {
		out = append(out, rvReviewFromRow(row))
	}
	return out
}

// ─── ActExtend 行转换 ─────────────────────────────────────────────

// rvActExtendToRow 将审阅人绑定实体转为通用行。
// @param ext 审阅人绑定实体
// @return 通用行
func rvActExtendToRow(ext *ActExtend) pwf.Row {
	row := pwf.Row{
		"activity_id":            ext.ActivityID,
		"activity_name":          ext.ActivityName,
		"process_definition_key": ext.ProcessDefinitionKey,
		"owner_type":             ext.OwnerType,
		"owner_value":            ext.OwnerValue,
	}
	// 可空列仅在有值时落库，保持 NULL 语义
	if ext.StepID != "" {
		row["step_id"] = ext.StepID
	}
	appendBaseRow(row, ext.BaseColumns)
	return row
}

// rvActExtendFromRow 将通用行转为审阅人绑定实体。
// @param row 通用行
// @return 审阅人绑定实体
func rvActExtendFromRow(row pwf.Row) *ActExtend {
	return &ActExtend{
		BaseColumns:          baseFromRow(row),
		ActivityID:           rowString(row, "activity_id"),
		ActivityName:         rowString(row, "activity_name"),
		ProcessDefinitionKey: rowString(row, "process_definition_key"),
		OwnerType:            rowString(row, "owner_type"),
		OwnerValue:           rowString(row, "owner_value"),
		StepID:               rowString(row, "step_id"),
	}
}

// rvActExtendsFromRows 批量转换审阅人绑定行。
// @param rows 通用行列表
// @return 实体列表
func rvActExtendsFromRows(rows []pwf.Row) []*ActExtend {
	out := make([]*ActExtend, 0, len(rows))
	for _, row := range rows {
		out = append(out, rvActExtendFromRow(row))
	}
	return out
}

// ─── SQLReviewTaskStore：审阅影子任务 ─────────────────────────────────────────────

// SQLReviewTaskStore 是 ReviewTaskStore 的 SQL 适配器。
type SQLReviewTaskStore struct {
	core *sqlCore // 共享内核
}

// Insert 插入一条审阅影子任务。
// @param ctx 操作上下文
// @param rt 审阅影子任务
// @return 错误
func (s *SQLReviewTaskStore) Insert(ctx context.Context, rt *ReviewTask) error {
	return s.core.repos.ReviewTasks.Insert(ctx, rvReviewToRow(rt))
}

// Get 按主键查询审阅影子任务。
// @param ctx 操作上下文
// @param id 影子任务 Id
// @return 审阅影子任务
// @return 是否存在
// @return 错误
func (s *SQLReviewTaskStore) Get(ctx context.Context, id string) (*ReviewTask, bool, error) {
	row, found, err := s.core.repos.ReviewTasks.GetByPK(ctx, id)
	if err != nil || !found {
		return nil, false, err
	}
	return rvReviewFromRow(row), true, nil
}

// Update 办结回写：按主键回写可变列（status/opinion/review_time/remark/updater/update_time）。
// @param ctx 操作上下文
// @param rt 审阅影子任务（必填 id）
// @return 错误
func (s *SQLReviewTaskStore) Update(ctx context.Context, rt *ReviewTask) error {
	changes := pwf.Row{
		"status":  rt.Status,
		"opinion": rt.Opinion,
		"remark":  rt.Remark,
	}
	// review_time 为 nil 时显式置 NULL（键存在值为 nil），非 nil 回写时间
	if rt.ReviewTime == nil {
		changes["review_time"] = nil
	} else {
		changes["review_time"] = *rt.ReviewTime
	}
	if rt.Updater != "" {
		changes["updater"] = rt.Updater
	}
	// 零值 update_time 交由仓储统一填充
	if !rt.UpdateTime.IsZero() {
		changes["update_time"] = rt.UpdateTime
	}
	_, err := s.core.repos.ReviewTasks.UpdateByPK(ctx, rt.ID, changes)
	return err
}

// ListByReviewer 按审阅人分页查询（租户过滤，空租户不过滤；仓储缺口，裸 SQL 只读补齐，
// 内存分页与 ListByOwner 既有模式一致）。
// @param ctx 操作上下文
// @param reviewer 审阅人 Id
// @param tenantID 租户
// @param page 分页参数
// @return 当前页列表
// @return 总数
// @return 错误
func (s *SQLReviewTaskStore) ListByReviewer(ctx context.Context, reviewer, tenantID string, page PageQuery) ([]*ReviewTask, int64, error) {
	query := "SELECT * FROM workflow_review_task WHERE reviewer = {p1}"
	args := []any{reviewer}
	// 空租户不过滤（与内存实现语义一致）
	if tenantID != "" {
		query += " AND tenant_id = {p2}"
		args = append(args, tenantID)
	}
	query += " ORDER BY start_time"
	rows, err := s.core.queryRaw(ctx, query, args)
	if err != nil {
		return nil, 0, err
	}
	data, total := paginate(rvReviewsFromRows(rows), page.Start, page.Size)
	return data, total, nil
}

// ListByInstance 查询实例下全部审阅影子任务（按创建时间升序）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 列表
// @return 错误
func (s *SQLReviewTaskStore) ListByInstance(ctx context.Context, processInstanceID string) ([]*ReviewTask, error) {
	rows, err := s.core.repos.ReviewTasks.ListByProcessInstance(ctx, processInstanceID)
	return rvReviewsFromRows(rows), err
}

// DeleteByInstance 删除实例下全部审阅影子任务（仓储无按实例删除方法，经列表逐条按主键删除补齐）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 错误
func (s *SQLReviewTaskStore) DeleteByInstance(ctx context.Context, processInstanceID string) error {
	rows, err := s.core.repos.ReviewTasks.ListByProcessInstance(ctx, processInstanceID)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := s.core.repos.ReviewTasks.DeleteByPK(ctx, rowString(row, "id")); err != nil {
			return err
		}
	}
	return nil
}

// CountUnread 统计审阅人的待审阅数（status=0，租户过滤；仓储缺口，裸 SQL 只读补齐）。
// @param ctx 操作上下文
// @param reviewer 审阅人 Id
// @param tenantID 租户
// @return 待审阅数
// @return 错误
func (s *SQLReviewTaskStore) CountUnread(ctx context.Context, reviewer, tenantID string) (int64, error) {
	query := "SELECT COUNT(*) AS cnt FROM workflow_review_task WHERE reviewer = {p1} AND status = 0"
	args := []any{reviewer}
	// 空租户不过滤（与内存实现语义一致）
	if tenantID != "" {
		query += " AND tenant_id = {p2}"
		args = append(args, tenantID)
	}
	rows, err := s.core.queryRaw(ctx, query, args)
	if err != nil || len(rows) == 0 {
		return 0, err
	}
	return int64(rowInt(rows[0], "cnt")), nil
}

// ─── SQLActExtendStore：审阅人绑定 ─────────────────────────────────────────────

// SQLActExtendStore 是 ActExtendStore 的 SQL 适配器。
type SQLActExtendStore struct {
	core *sqlCore // 共享内核
}

// GetByDefinitionKeyAndActivity 按流程定义 Key 与 BPMN 节点 ID 查询绑定。
// @param ctx 操作上下文
// @param definitionKey 流程定义 Key
// @param activityID BPMN 节点 ID
// @return 绑定列表
// @return 错误
func (s *SQLActExtendStore) GetByDefinitionKeyAndActivity(ctx context.Context, definitionKey, activityID string) ([]*ActExtend, error) {
	rows, err := s.core.repos.ActExtends.ListByDefinitionAndActivity(ctx, definitionKey, activityID)
	return rvActExtendsFromRows(rows), err
}

// ListByDefinitionKey 查询流程定义 Key 下全部绑定（仓储缺口，裸 SQL 只读补齐）。
// @param ctx 操作上下文
// @param definitionKey 流程定义 Key
// @return 绑定列表
// @return 错误
func (s *SQLActExtendStore) ListByDefinitionKey(ctx context.Context, definitionKey string) ([]*ActExtend, error) {
	rows, err := s.core.queryRaw(ctx, "SELECT * FROM workflow_act_extend WHERE process_definition_key = {p1} ORDER BY id", []any{definitionKey})
	return rvActExtendsFromRows(rows), err
}

// Save 保存一条审阅人绑定。
// @param ctx 操作上下文
// @param ext 审阅人绑定
// @return 错误
func (s *SQLActExtendStore) Save(ctx context.Context, ext *ActExtend) error {
	return s.core.repos.ActExtends.Insert(ctx, rvActExtendToRow(ext))
}

// Delete 按主键删除审阅人绑定。
// @param ctx 操作上下文
// @param id 绑定 Id
// @return 错误
func (s *SQLActExtendStore) Delete(ctx context.Context, id string) error {
	_, err := s.core.repos.ActExtends.DeleteByPK(ctx, id)
	return err
}
