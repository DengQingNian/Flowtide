package workflow

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	pwf "github.com/DengQingNian/Flowtide/internal/persistence/workflow"
)

// ─── SQL 适配器骨架 ─────────────────────────────────────────────
// 五大存储端口存在同名方法（Save/GetByTaskID 等），与内存实现同理拆分为
// 五个适配器结构体，共享 sqlCore（仓储入口 + 聚合仓储 + 裸 SQL 只读补齐通道）。
// 事务经 pwf.Store.WithTx 透传：ctx 携带 *sql.Tx 时全部写入走同一工作单元。

// SQLStoreSet 是五大域存储端口 SQL 适配器的聚合。
type SQLStoreSet struct {
	Next      *SQLNextStore      // 下一步预生成-消耗
	Message   *SQLMessageStore   // 待办扩展
	History   *SQLHistoryStore   // 已办扩展
	Operation *SQLOperationStore // 操作记录
	Document  *SQLDocumentStore  // 业务表单状态
}

// sqlCore 是 SQL 适配器共享内核。
type sqlCore struct {
	store *pwf.Store        // workflow 仓储入口（方言/时钟/事务）
	repos *pwf.Repositories // 行仓储聚合
	db    *sql.DB           // 连接池（可选，仅裸 SQL 只读补齐查询使用；nil 时缺口查询报错）
}

// NewSQLStores 创建 SQL 适配器聚合（不带裸 SQL 补齐通道）。
// @param store workflow 仓储入口
// @return 适配器聚合
func NewSQLStores(store *pwf.Store) *SQLStoreSet {
	core := &sqlCore{store: store, repos: pwf.NewRepositories(store)}
	return &SQLStoreSet{
		Next:      &SQLNextStore{core: core},
		Message:   &SQLMessageStore{core: core},
		History:   &SQLHistoryStore{core: core},
		Operation: &SQLOperationStore{core: core},
		Document:  &SQLDocumentStore{core: core},
	}
}

// NewSQLStoresWithDB 创建 SQL 适配器聚合并附带裸 SQL 只读补齐通道。
// @param store workflow 仓储入口
// @param db 连接池（与 store 共用同一连接池）
// @return 适配器聚合
func NewSQLStoresWithDB(store *pwf.Store, db *sql.DB) *SQLStoreSet {
	set := NewSQLStores(store)
	set.Next.core.db = db
	return set
}

// queryRaw 执行只读裸 SQL 补齐查询（仓储缺口专用；复用同一连接池，不走 workflow 事务）。
// @param ctx 操作上下文
// @param query 含 {pN} 占位符的查询模板
// @param args 参数列表
// @return 通用行列表
// @return 错误
func (c *sqlCore) queryRaw(ctx context.Context, query string, args []any) ([]pwf.Row, error) {
	// 裸 SQL 依赖外部传入的连接池，未提供时明确报错而非静默空结果
	if c.db == nil {
		return nil, api.NewError("SQL-ADAPTER-001", "裸 SQL 补齐查询需要通过 NewSQLStoresWithDB 注入连接池", nil)
	}
	rendered := query
	// {pN} 按方言渲染：MySQL "?"，PostgreSQL "$N"
	for i := range args {
		rendered = strings.ReplaceAll(rendered, "{p"+strconv.Itoa(i+1)+"}", c.ph(i+1))
	}
	rows, err := c.db.QueryContext(ctx, rendered, args...)
	if err != nil {
		return nil, api.NewError("SQL-ADAPTER-002", "裸 SQL 查询失败", err)
	}
	defer func() { _ = rows.Close() }()
	columns, err := rows.Columns()
	if err != nil {
		return nil, api.NewError("SQL-ADAPTER-003", "读取结果集列失败", err)
	}
	result := make([]pwf.Row, 0, 8)
	for rows.Next() {
		values := make([]any, len(columns))
		targets := make([]any, len(columns))
		for i := range values {
			targets[i] = &values[i]
		}
		if err := rows.Scan(targets...); err != nil {
			return nil, api.NewError("SQL-ADAPTER-004", "扫描结果行失败", err)
		}
		row := make(pwf.Row, len(columns))
		for i, column := range columns {
			// 与仓储 normalizeValue 一致：[]byte 统一转字符串
			if bytes, ok := values[i].([]byte); ok {
				row[column] = string(bytes)
			} else {
				row[column] = values[i]
			}
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// ph 按方言返回第 n 个占位符。
// @param n 序号（从 1 开始）
// @return 占位符文本
func (c *sqlCore) ph(n int) string {
	return dialectPlaceholder(c.store.Dialect(), n)
}

// dialectPlaceholder 按方言渲染占位符（MySQL "?"，PostgreSQL "$n"）。
// @param dialect 方言名
// @param n 序号（从 1 开始）
// @return 占位符文本
func dialectPlaceholder(dialect string, n int) string {
	// 与 pwf.Store.ph 一致
	if dialect == pwf.DialectMySQL {
		return "?"
	}
	return "$" + strconv.Itoa(n)
}

// 编译期断言：SQL 适配器满足全部五个存储端口。
var (
	_ NextActivityStore    = (*SQLNextStore)(nil)
	_ MessageExtendStore   = (*SQLMessageStore)(nil)
	_ HistoryExtendStore   = (*SQLHistoryStore)(nil)
	_ OperationRecordStore = (*SQLOperationStore)(nil)
	_ DocumentStore        = (*SQLDocumentStore)(nil)
)

// ─── 值转换工具 ─────────────────────────────────────────────

// rowString 读取行为字符串（nil → 空串，[]byte → string）。
// @param row 通用行
// @param key 列名
// @return 字符串值
func rowString(row pwf.Row, key string) string {
	value, ok := row[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return fmt.Sprintf("%v", typed)
	}
}

// rowInt 读取行为整型（兼容 int64/int/float64/字符串）。
// @param row 通用行
// @param key 列名
// @return 整型值，无法解析返回 0
func rowInt(row pwf.Row, key string) int {
	value, ok := row[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case int64:
		return int(typed)
	case int:
		return typed
	case float64:
		return int(typed)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0
		}
		return parsed
	case []byte:
		parsed, err := strconv.Atoi(strings.TrimSpace(string(typed)))
		if err != nil {
			return 0
		}
		return parsed
	default:
		return 0
	}
}

// rowIntPtr 读取行为可空整型指针（NULL → nil）。
// @param row 通用行
// @param key 列名
// @return 整型指针
func rowIntPtr(row pwf.Row, key string) *int {
	if _, ok := row[key]; !ok || row[key] == nil {
		return nil
	}
	value := rowInt(row, key)
	return &value
}

// rowI64Ptr 读取行为可空 int64 指针（NULL → nil）。
// @param row 通用行
// @param key 列名
// @return int64 指针
func rowI64Ptr(row pwf.Row, key string) *int64 {
	if _, ok := row[key]; !ok || row[key] == nil {
		return nil
	}
	value := int64(rowInt(row, key))
	return &value
}

// rowFloatPtr 读取行为可空浮点指针（NULL → nil，兼容 DECIMAL 的字符串/字节表示）。
// @param row 通用行
// @param key 列名
// @return 浮点指针
func rowFloatPtr(row pwf.Row, key string) *float64 {
	value, ok := row[key]
	if !ok || value == nil {
		return nil
	}
	var parsed float64
	switch typed := value.(type) {
	case float64:
		parsed = typed
	case float32:
		parsed = float64(typed)
	case int64:
		parsed = float64(typed)
	case int:
		parsed = float64(typed)
	case string:
		v, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err != nil {
			return nil
		}
		parsed = v
	case []byte:
		v, err := strconv.ParseFloat(strings.TrimSpace(string(typed)), 64)
		if err != nil {
			return nil
		}
		parsed = v
	default:
		return nil
	}
	return &parsed
}

// rowTimePtr 读取行为可空时间指针（NULL → nil）。
// @param row 通用行
// @param key 列名
// @return 时间指针
func rowTimePtr(row pwf.Row, key string) *time.Time {
	value, ok := row[key]
	if !ok || value == nil {
		return nil
	}
	switch typed := value.(type) {
	case time.Time:
		return &typed
	case string:
		// 双库 datetime/timestamp 文本回退解析
		for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05"} {
			if parsed, err := time.Parse(layout, typed); err == nil {
				return &parsed
			}
		}
		return nil
	default:
		return nil
	}
}

// timeOrZero 读取行为时间（NULL → 零值）。
// @param row 通用行
// @param key 列名
// @return 时间值
func timeOrZero(row pwf.Row, key string) time.Time {
	if t := rowTimePtr(row, key); t != nil {
		return *t
	}
	return time.Time{}
}

// baseFromRow 从通用行读取公共审计列。
// @param row 通用行
// @return 公共审计列
func baseFromRow(row pwf.Row) BaseColumns {
	return BaseColumns{
		ID:          rowString(row, "id"),
		Creator:     rowString(row, "creator"),
		CreatorName: rowString(row, "creator_name"),
		CreateTime:  timeOrZero(row, "create_time"),
		Updater:     rowString(row, "updater"),
		UpdateTime:  timeOrZero(row, "update_time"),
		TenantID:    rowString(row, "tenant_id"),
	}
}

// appendBaseRow 将公共审计列写入通用行（零值时间列省略，由仓储填充）。
// @param row 通用行
// @param base 公共审计列
func appendBaseRow(row pwf.Row, base BaseColumns) {
	row["id"] = base.ID
	row["creator"] = base.Creator
	row["creator_name"] = base.CreatorName
	row["updater"] = base.Updater
	row["tenant_id"] = base.TenantID
	// 零值 create_time/update_time 省略，交由仓储统一填充
	if !base.CreateTime.IsZero() {
		row["create_time"] = base.CreateTime
	}
	if !base.UpdateTime.IsZero() {
		row["update_time"] = base.UpdateTime
	}
}

// appendExtendRow 将扩展列写入通用行（nil 值省略保持 NULL 语义）。
// @param row 通用行
// @param ext 扩展列
func appendExtendRow(row pwf.Row, ext ExtendFields) {
	// s 串型 8 列：空串省略（落 NULL）
	for i, value := range []string{ext.ExtendS1, ext.ExtendS2, ext.ExtendS3, ext.ExtendS4, ext.ExtendS5, ext.ExtendS6, ext.ExtendS7, ext.ExtendS8} {
		if value != "" {
			row[fmt.Sprintf("extend_s%d", i+1)] = value
		}
	}
	// i 整型 3 列
	for i, value := range []*int64{ext.ExtendI1, ext.ExtendI2, ext.ExtendI3} {
		if value != nil {
			row[fmt.Sprintf("extend_i%d", i+1)] = *value
		}
	}
	// f 浮点 3 列
	for i, value := range []*float64{ext.ExtendF1, ext.ExtendF2, ext.ExtendF3} {
		if value != nil {
			row[fmt.Sprintf("extend_f%d", i+1)] = *value
		}
	}
	// d 时间 3 列
	for i, value := range []*time.Time{ext.ExtendD1, ext.ExtendD2, ext.ExtendD3} {
		if value != nil {
			row[fmt.Sprintf("extend_d%d", i+1)] = *value
		}
	}
}

// extendFromRow 从通用行读取扩展列。
// @param row 通用行
// @return 扩展列
func extendFromRow(row pwf.Row) ExtendFields {
	var ext ExtendFields
	// s 串型 8 列
	sFields := []*string{&ext.ExtendS1, &ext.ExtendS2, &ext.ExtendS3, &ext.ExtendS4, &ext.ExtendS5, &ext.ExtendS6, &ext.ExtendS7, &ext.ExtendS8}
	for i, field := range sFields {
		*field = rowString(row, fmt.Sprintf("extend_s%d", i+1))
	}
	// i 整型 3 列
	ext.ExtendI1 = rowI64Ptr(row, "extend_i1")
	ext.ExtendI2 = rowI64Ptr(row, "extend_i2")
	ext.ExtendI3 = rowI64Ptr(row, "extend_i3")
	// f 浮点 3 列
	ext.ExtendF1 = rowFloatPtr(row, "extend_f1")
	ext.ExtendF2 = rowFloatPtr(row, "extend_f2")
	ext.ExtendF3 = rowFloatPtr(row, "extend_f3")
	// d 时间 3 列
	ext.ExtendD1 = rowTimePtr(row, "extend_d1")
	ext.ExtendD2 = rowTimePtr(row, "extend_d2")
	ext.ExtendD3 = rowTimePtr(row, "extend_d3")
	return ext
}

// sortNextRowsByCreateTime 按创建时间升序原地排序（短列表插入排序）。
// @param rows 实体列表
func sortNextRowsByCreateTime(rows []*NextActivity) {
	for i := 1; i < len(rows); i++ {
		// 插入排序：列表通常为同节点执行人数量级
		for j := i; j > 0 && rows[j].CreateTime.Before(rows[j-1].CreateTime); j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
}

// ─── NextActivity 行转换 ─────────────────────────────────────────────

// nextToRow 将下一步实体转为通用行。
// @param a 下一步实体
// @return 通用行
func nextToRow(a *NextActivity) pwf.Row {
	row := pwf.Row{
		"next_activity_id":    a.NextActivityID,
		"group_id":            a.GroupID,
		"identity_id":         a.IdentityID,
		"process_instance_id": a.ProcessInstanceID,
		"used":                a.Used,
		"pre_task_id":         a.PreTaskID,
		"task_id":             a.TaskID,
		"version":             a.Version,
	}
	// 可空列仅在有值时落库，保持 NULL 语义
	if a.SendIdentityID != "" {
		row["send_identity_id"] = a.SendIdentityID
	}
	if a.SendUserName != "" {
		row["send_user_name"] = a.SendUserName
	}
	if a.AgreeWeight != nil {
		row["agree_weight"] = *a.AgreeWeight
	}
	if a.RejectWeight != nil {
		row["reject_weight"] = *a.RejectWeight
	}
	appendBaseRow(row, a.BaseColumns)
	return row
}

// nextFromRow 将通用行转为下一步实体。
// @param row 通用行
// @return 下一步实体
func nextFromRow(row pwf.Row) *NextActivity {
	return &NextActivity{
		BaseColumns:       baseFromRow(row),
		NextActivityID:    rowString(row, "next_activity_id"),
		GroupID:           rowString(row, "group_id"),
		IdentityID:        rowString(row, "identity_id"),
		SendIdentityID:    rowString(row, "send_identity_id"),
		SendUserName:      rowString(row, "send_user_name"),
		ProcessInstanceID: rowString(row, "process_instance_id"),
		AgreeWeight:       rowFloatPtr(row, "agree_weight"),
		RejectWeight:      rowFloatPtr(row, "reject_weight"),
		Used:              rowInt(row, "used"),
		PreTaskID:         rowString(row, "pre_task_id"),
		TaskID:            rowString(row, "task_id"),
		Version:           rowInt(row, "version"),
	}
}

// nextsFromRows 批量转换下一步行。
// @param rows 通用行列表
// @return 实体列表
func nextsFromRows(rows []pwf.Row) []*NextActivity {
	out := make([]*NextActivity, 0, len(rows))
	for _, row := range rows {
		out = append(out, nextFromRow(row))
	}
	return out
}

// ─── SQLNextStore：下一步预生成-消耗 ─────────────────────────────────────────────

// SQLNextStore 是 NextActivityStore 的 SQL 适配器。
type SQLNextStore struct {
	core *sqlCore // 共享内核
}

// InsertDual 双写运行态与历史表（同 ID 同内容），依赖 ctx 事务保证原子性。
// @param ctx 操作上下文
// @param act 下一步记录
// @return 错误
func (s *SQLNextStore) InsertDual(ctx context.Context, act *NextActivity) error {
	return s.core.repos.NextActivities.InsertDual(ctx, nextToRow(act))
}

// LatestVersion 返回实例下指定节点已消耗（used=1）记录的最大版本号，无记录返回 0。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param nextActivityID 节点 ID
// @return 最大版本号
// @return 错误
func (s *SQLNextStore) LatestVersion(ctx context.Context, processInstanceID, nextActivityID string) (int, error) {
	rows, err := s.core.repos.HisNextActivities.ListByActivityByVersionDesc(ctx, processInstanceID, nextActivityID)
	if err != nil {
		return 0, err
	}
	// 版本降序取第一条已消耗记录（Java getNextActivityLatestVersionByProcInstAndTaskKey 语义）
	for _, row := range rows {
		if rowInt(row, "used") == 1 {
			return rowInt(row, "version"), nil
		}
	}
	return 0, nil
}

// ListUnusedByInstanceAndActivity 查询实例下指定节点未消耗记录。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param nextActivityID 节点 ID
// @return 记录列表
// @return 错误
func (s *SQLNextStore) ListUnusedByInstanceAndActivity(ctx context.Context, processInstanceID, nextActivityID string) ([]*NextActivity, error) {
	rows, err := s.core.repos.NextActivities.ListUnusedByActivity(ctx, processInstanceID, nextActivityID)
	return nextsFromRows(rows), err
}

// ListUnusedByInstance 查询实例下全部未消耗记录。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 记录列表
// @return 错误
func (s *SQLNextStore) ListUnusedByInstance(ctx context.Context, processInstanceID string) ([]*NextActivity, error) {
	rows, err := s.core.repos.NextActivities.ListUnusedByProcessInstance(ctx, processInstanceID)
	return nextsFromRows(rows), err
}

// GetFirstUnusedByActivity 按创建时间升序返回实例下指定节点的第一条未消耗记录。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param nextActivityID 节点 ID
// @return 记录
// @return 是否存在
// @return 错误
func (s *SQLNextStore) GetFirstUnusedByActivity(ctx context.Context, processInstanceID, nextActivityID string) (*NextActivity, bool, error) {
	rows, err := s.core.repos.NextActivities.ListUnusedByActivity(ctx, processInstanceID, nextActivityID)
	if err != nil || len(rows) == 0 {
		return nil, false, err
	}
	acts := nextsFromRows(rows)
	// 仓储按 version 排序，这里再按创建时间取最早一条（Java orderByAsc create_time）
	sortNextRowsByCreateTime(acts)
	return acts[0], true, nil
}

// ListByPreTask 查询以上一任务为来源的运行态记录。
// @param ctx 操作上下文
// @param preTaskID 上一任务 Id
// @return 记录列表
// @return 错误
func (s *SQLNextStore) ListByPreTask(ctx context.Context, preTaskID string) ([]*NextActivity, error) {
	rows, err := s.core.repos.NextActivities.ListByPreTask(ctx, preTaskID)
	return nextsFromRows(rows), err
}

// ListByTaskID 查询已挂到任务上的运行态记录。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @return 记录列表
// @return 错误
func (s *SQLNextStore) ListByTaskID(ctx context.Context, taskID string) ([]*NextActivity, error) {
	rows, err := s.core.repos.NextActivities.ListByTask(ctx, taskID)
	return nextsFromRows(rows), err
}

// ListUsedByInstance 查询实例下已回写任务 ID 的记录（仓储缺口，裸 SQL 只读补齐）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 记录列表
// @return 错误
func (s *SQLNextStore) ListUsedByInstance(ctx context.Context, processInstanceID string) ([]*NextActivity, error) {
	rows, err := s.core.queryRaw(ctx, "SELECT * FROM workflow_next_activity WHERE process_instance_id = {p1} AND task_id IS NOT NULL ORDER BY create_time", []any{processInstanceID})
	return nextsFromRows(rows), err
}

// MarkUsedAndWriteBack 置 used=1 并回写任务 ID（运行态与历史表同步，仅未消耗记录生效）。
// @param ctx 操作上下文
// @param ids 记录 Id 集合
// @param taskID 任务 Id
// @return 错误
func (s *SQLNextStore) MarkUsedAndWriteBack(ctx context.Context, ids []string, taskID string) error {
	// 逐条更新（记录数为同节点执行人数量级），依赖 ctx 事务保证批次原子性
	for _, id := range ids {
		if _, err := s.core.repos.NextActivities.MarkUsed(ctx, id, taskID); err != nil {
			return err
		}
		if _, err := s.core.repos.HisNextActivities.MarkUsed(ctx, id, taskID); err != nil {
			return err
		}
	}
	return nil
}

// ListHisByInstance 查询实例的全部历史记录（按版本升序）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 记录列表
// @return 错误
func (s *SQLNextStore) ListHisByInstance(ctx context.Context, processInstanceID string) ([]*NextActivity, error) {
	rows, err := s.core.repos.HisNextActivities.ListByProcessInstance(ctx, processInstanceID)
	return nextsFromRows(rows), err
}

// ListHisByTaskID 查询历史表中任务相关且非自环（pre_task_id != taskID）的记录（仓储缺口，裸 SQL 只读补齐）。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @return 记录列表
// @return 错误
func (s *SQLNextStore) ListHisByTaskID(ctx context.Context, taskID string) ([]*NextActivity, error) {
	// Java WorkflowHisNextActivityServiceImpl#findByTaskId：task_id = ? AND pre_task_id != ?（NULL 不排除）
	rows, err := s.core.queryRaw(ctx, "SELECT * FROM workflow_his_next_activity WHERE task_id = {p1} AND (pre_task_id IS NULL OR pre_task_id != {p2}) ORDER BY create_time", []any{taskID, taskID})
	return nextsFromRows(rows), err
}

// DeleteByProcessInstance 删除实例的全部运行态记录。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 错误
func (s *SQLNextStore) DeleteByProcessInstance(ctx context.Context, processInstanceID string) error {
	_, err := s.core.repos.NextActivities.DeleteByProcessInstance(ctx, processInstanceID)
	return err
}

// DeleteByProcessInstanceAndActivityIDs 删除实例下指定节点集合的运行态与历史记录。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param nextActivityIDs 节点 ID 集合
// @return 错误
func (s *SQLNextStore) DeleteByProcessInstanceAndActivityIDs(ctx context.Context, processInstanceID string, nextActivityIDs []string) error {
	// 运行态与历史表同步清理（Java 两侧 mapper 同构删除）
	if _, err := s.core.repos.NextActivities.DeleteByProcessInstanceAndActivities(ctx, processInstanceID, nextActivityIDs); err != nil {
		return err
	}
	_, err := s.core.repos.HisNextActivities.DeleteByProcessInstanceAndActivities(ctx, processInstanceID, nextActivityIDs)
	return err
}

// DeleteForBackToStep 退回清理：删除实例下目标节点（含已消耗）的全部运行态记录。
// 说明：Java 原实现附带"顺带删除更早创建记录"的 create_time 全表比较，语义模糊且副作用大，
// Go 侧收敛为删除目标节点全部记录。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param nextActivityID 目标节点 ID
// @return 错误
func (s *SQLNextStore) DeleteForBackToStep(ctx context.Context, processInstanceID, nextActivityID string) error {
	_, err := s.core.repos.NextActivities.DeleteByProcessInstanceAndActivities(ctx, processInstanceID, []string{nextActivityID})
	return err
}

// DeleteForRollback 撤回清理：删除实例下以 preTaskID 为来源、目标节点之外的记录（仅运行态，Java 语义）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param preTaskID 来源任务 Id
// @param keepActivityID 保留的目标节点 ID
// @return 错误
func (s *SQLNextStore) DeleteForRollback(ctx context.Context, processInstanceID, preTaskID, keepActivityID string) error {
	rows, err := s.core.repos.NextActivities.ListByPreTask(ctx, preTaskID)
	if err != nil {
		return err
	}
	// 逐条按 ID 删除，保留 keepActivityID（Java deleteRollbackWorkflowNextActivity）
	for _, row := range rows {
		if rowString(row, "process_instance_id") == processInstanceID && rowString(row, "next_activity_id") != keepActivityID {
			if _, err := s.core.repos.NextActivities.DeleteByPK(ctx, rowString(row, "id")); err != nil {
				return err
			}
		}
	}
	return nil
}

// DeleteForResolve 处理委派清理：删除实例下指定节点+任务+执行人的记录。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param nextActivityID 节点 ID
// @param taskID 任务 Id
// @param identityID 执行人 Id
// @return 错误
func (s *SQLNextStore) DeleteForResolve(ctx context.Context, processInstanceID, nextActivityID, taskID, identityID string) error {
	rows, err := s.core.repos.NextActivities.ListByTask(ctx, taskID)
	if err != nil {
		return err
	}
	// Java deleteResolveWorkflowNextActivity 四条件过滤
	for _, row := range rows {
		if rowString(row, "process_instance_id") == processInstanceID &&
			rowString(row, "next_activity_id") == nextActivityID &&
			rowString(row, "identity_id") == identityID {
			if _, err := s.core.repos.NextActivities.DeleteByPK(ctx, rowString(row, "id")); err != nil {
				return err
			}
		}
	}
	return nil
}

// FindAssignSource 按（实例/节点/发送人/任务）定位既有记录（转办/委派/还原继承组用）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param nextActivityID 节点 ID
// @param sendIdentityID 发送人 Id
// @param taskID 任务 Id
// @return 记录
// @return 是否存在
// @return 错误
func (s *SQLNextStore) FindAssignSource(ctx context.Context, processInstanceID, nextActivityID, sendIdentityID, taskID string) (*NextActivity, bool, error) {
	rows, err := s.core.repos.NextActivities.ListByTask(ctx, taskID)
	if err != nil {
		return nil, false, err
	}
	// Java findNextActivityByParams 四条件取首条
	for _, row := range rows {
		if rowString(row, "process_instance_id") == processInstanceID &&
			rowString(row, "next_activity_id") == nextActivityID &&
			rowString(row, "send_identity_id") == sendIdentityID {
			return nextFromRow(row), true, nil
		}
	}
	return nil, false, nil
}

// ─── MessageExtend 行转换 ─────────────────────────────────────────────

// messageToRow 将待办扩展实体转为通用行。
// @param msg 待办扩展实体
// @return 通用行
func messageToRow(msg *MessageExtend) pwf.Row {
	row := pwf.Row{
		"task_id":                 msg.TaskID,
		"process_instance_id":     msg.ProcessInstanceID,
		"title":                   msg.Title,
		"message":                 msg.Message,
		"business_id":             msg.BusinessID,
		"target_url":              msg.TargetURL,
		"task_step_name":          msg.TaskStepName,
		"task_step_code":          msg.TaskStepCode,
		"flow_category_code":      msg.FlowCategoryCode,
		"flow_category_name":      msg.FlowCategoryName,
		"owner_id":                msg.OwnerID,
		"owner_name":              msg.OwnerName,
		"owner_group":             msg.OwnerGroup,
		"owner_group_id":          msg.OwnerGroupID,
		"owner_group_paths":       msg.OwnerGroupPaths,
		"owner_group_path_ids":    msg.OwnerGroupPathIDs,
		"document_owner_id":       msg.DocumentOwnerID,
		"document_owner_name":     msg.DocumentOwnerName,
		"document_group_id":       msg.DocumentGroupID,
		"document_group":          msg.DocumentGroup,
		"document_group_paths":    msg.DocumentGroupPaths,
		"document_group_path_ids": msg.DocumentGroupPathIDs,
		"sender_id":               msg.SenderID,
		"sender":                  msg.Sender,
		"sender_step_name":        msg.SenderStepName,
		"sender_step_code":        msg.SenderStepCode,
		"consigner_id":            msg.ConsignerID,
		"consigner":               msg.Consigner,
		"is_read":                 msg.IsRead,
		"is_delay":                msg.IsDelay,
		"message_group_id":        msg.MessageGroupID,
	}
	// 可空时间列仅在有值时落库
	if msg.ReadTime != nil {
		row["read_time"] = *msg.ReadTime
	}
	if msg.PlanFinishTime != nil {
		row["plan_finish_time"] = *msg.PlanFinishTime
	}
	if msg.TaskReceiptTime != nil {
		row["task_receipt_time"] = *msg.TaskReceiptTime
	}
	if msg.TaskStartTime != nil {
		row["task_start_time"] = *msg.TaskStartTime
	}
	if msg.DelayTime != nil {
		row["delay_time"] = *msg.DelayTime
	}
	appendExtendRow(row, msg.ExtendFields)
	appendBaseRow(row, msg.BaseColumns)
	return row
}

// messageFromRow 将通用行转为待办扩展实体。
// @param row 通用行
// @return 待办扩展实体
func messageFromRow(row pwf.Row) *MessageExtend {
	return &MessageExtend{
		BaseColumns:          baseFromRow(row),
		TaskID:               rowString(row, "task_id"),
		ProcessInstanceID:    rowString(row, "process_instance_id"),
		Title:                rowString(row, "title"),
		Message:              rowString(row, "message"),
		BusinessID:           rowString(row, "business_id"),
		TargetURL:            rowString(row, "target_url"),
		TaskStepName:         rowString(row, "task_step_name"),
		TaskStepCode:         rowString(row, "task_step_code"),
		FlowCategoryCode:     rowString(row, "flow_category_code"),
		FlowCategoryName:     rowString(row, "flow_category_name"),
		OwnerID:              rowString(row, "owner_id"),
		OwnerName:            rowString(row, "owner_name"),
		OwnerGroup:           rowString(row, "owner_group"),
		OwnerGroupID:         rowString(row, "owner_group_id"),
		OwnerGroupPaths:      rowString(row, "owner_group_paths"),
		OwnerGroupPathIDs:    rowString(row, "owner_group_path_ids"),
		DocumentOwnerID:      rowString(row, "document_owner_id"),
		DocumentOwnerName:    rowString(row, "document_owner_name"),
		DocumentGroupID:      rowString(row, "document_group_id"),
		DocumentGroup:        rowString(row, "document_group"),
		DocumentGroupPaths:   rowString(row, "document_group_paths"),
		DocumentGroupPathIDs: rowString(row, "document_group_path_ids"),
		SenderID:             rowString(row, "sender_id"),
		Sender:               rowString(row, "sender"),
		SenderStepName:       rowString(row, "sender_step_name"),
		SenderStepCode:       rowString(row, "sender_step_code"),
		ConsignerID:          rowString(row, "consigner_id"),
		Consigner:            rowString(row, "consigner"),
		IsRead:               rowInt(row, "is_read"),
		ReadTime:             rowTimePtr(row, "read_time"),
		PlanFinishTime:       rowTimePtr(row, "plan_finish_time"),
		TaskReceiptTime:      rowTimePtr(row, "task_receipt_time"),
		TaskStartTime:        rowTimePtr(row, "task_start_time"),
		IsDelay:              rowInt(row, "is_delay"),
		DelayTime:            rowTimePtr(row, "delay_time"),
		MessageGroupID:       rowString(row, "message_group_id"),
		ExtendFields:         extendFromRow(row),
	}
}

// messagesFromRows 批量转换待办扩展行。
// @param rows 通用行列表
// @return 实体列表
func messagesFromRows(rows []pwf.Row) []*MessageExtend {
	out := make([]*MessageExtend, 0, len(rows))
	for _, row := range rows {
		out = append(out, messageFromRow(row))
	}
	return out
}

// ─── SQLMessageStore：待办扩展 ─────────────────────────────────────────────

// SQLMessageStore 是 MessageExtendStore 的 SQL 适配器。
type SQLMessageStore struct {
	core *sqlCore // 共享内核
}

// Save 插入一条待办扩展。
// @param ctx 操作上下文
// @param msg 待办扩展
// @return 错误
func (s *SQLMessageStore) Save(ctx context.Context, msg *MessageExtend) error {
	return s.core.repos.MessageExtends.Insert(ctx, messageToRow(msg))
}

// GetByTaskID 按任务 ID 查询待办扩展。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @return 待办扩展
// @return 是否存在
// @return 错误
func (s *SQLMessageStore) GetByTaskID(ctx context.Context, taskID string) (*MessageExtend, bool, error) {
	row, found, err := s.core.repos.MessageExtends.GetByTask(ctx, taskID)
	if err != nil || !found {
		return nil, false, err
	}
	return messageFromRow(row), true, nil
}

// ListByInstance 查询实例下全部待办扩展。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 列表
// @return 错误
func (s *SQLMessageStore) ListByInstance(ctx context.Context, processInstanceID string) ([]*MessageExtend, error) {
	rows, err := s.core.repos.MessageExtends.ListByProcessInstance(ctx, processInstanceID)
	return messagesFromRows(rows), err
}

// ListByBusinessID 按业务单据与租户查询待办扩展。
// @param ctx 操作上下文
// @param businessID 业务单据 Id
// @param tenantID 租户
// @return 列表
// @return 错误
func (s *SQLMessageStore) ListByBusinessID(ctx context.Context, businessID, tenantID string) ([]*MessageExtend, error) {
	rows, err := s.core.repos.MessageExtends.ListByBusiness(ctx, businessID, tenantID)
	return messagesFromRows(rows), err
}

// ListByOwner 按处理人分页查询待办扩展（实现 MessageExtendStore.ListByOwner；
// 仓储暂无租户+分页下推，先全量取回后内存过滤分页，P1.2 接入待办列表接口时再补下推）。
// @param ctx 操作上下文
// @param ownerID 处理人 Id
// @param tenantID 租户
// @param page 分页参数
// @return 当前页列表
// @return 总数
// @return 错误
func (s *SQLMessageStore) ListByOwner(ctx context.Context, ownerID, tenantID string, page PageQuery) ([]*MessageExtend, int64, error) {
	rows, err := s.core.repos.MessageExtends.ListByOwner(ctx, ownerID)
	if err != nil {
		return nil, 0, err
	}
	// 内存租户过滤（叠加在仓储 owner 查询之上）
	filtered := make([]*MessageExtend, 0, len(rows))
	for _, row := range messagesFromRows(rows) {
		if tenantID == "" || row.TenantID == tenantID {
			filtered = append(filtered, row)
		}
	}
	data, total := paginate(filtered, page.Start, page.Size)
	return data, total, nil
}

// DeleteByTaskID 按任务删除待办扩展。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @return 错误
func (s *SQLMessageStore) DeleteByTaskID(ctx context.Context, taskID string) error {
	_, err := s.core.repos.MessageExtends.DeleteByTask(ctx, taskID)
	return err
}

// DeleteByTaskIDAndOwner 按任务与处理人删除待办扩展（经 GetByTask 定位后按主键删除）。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @param ownerID 处理人 Id
// @return 错误
func (s *SQLMessageStore) DeleteByTaskIDAndOwner(ctx context.Context, taskID, ownerID string) error {
	row, found, err := s.core.repos.MessageExtends.GetByTask(ctx, taskID)
	if err != nil || !found {
		return err
	}
	// 处理人不匹配时不删除（与 Java 按（任务+处理人）删除语义一致）
	if rowString(row, "owner_id") != ownerID {
		return nil
	}
	_, err = s.core.repos.MessageExtends.DeleteByPK(ctx, rowString(row, "id"))
	return err
}

// DeleteByInstance 按实例删除全部待办扩展（仓储无按实例删除方法，经列表逐条按主键删除补齐）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 错误
func (s *SQLMessageStore) DeleteByInstance(ctx context.Context, processInstanceID string) error {
	rows, err := s.core.repos.MessageExtends.ListByProcessInstance(ctx, processInstanceID)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := s.core.repos.MessageExtends.DeleteByPK(ctx, rowString(row, "id")); err != nil {
			return err
		}
	}
	return nil
}

// DeleteByBusinessID 按业务单据与租户删除待办扩展。
// @param ctx 操作上下文
// @param businessID 业务单据 Id
// @param tenantID 租户
// @return 错误
func (s *SQLMessageStore) DeleteByBusinessID(ctx context.Context, businessID, tenantID string) error {
	rows, err := s.core.repos.MessageExtends.ListByBusiness(ctx, businessID, tenantID)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := s.core.repos.MessageExtends.DeleteByPK(ctx, rowString(row, "id")); err != nil {
			return err
		}
	}
	return nil
}

// DeleteByProcessInstanceAndActivityIDs 按实例与步骤代号集合删除待办扩展（退回/撤回清理）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param stepCodes 步骤代号集合
// @return 错误
func (s *SQLMessageStore) DeleteByProcessInstanceAndActivityIDs(ctx context.Context, processInstanceID string, stepCodes []string) error {
	_, err := s.core.repos.MessageExtends.DeleteByProcessInstanceAndSteps(ctx, processInstanceID, stepCodes)
	return err
}

// UpdateRead 标记待办已读并回写阅读时间（仅（任务+处理人）命中时更新）。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @param ownerID 处理人 Id
// @param readTime 阅读时间
// @return 错误
func (s *SQLMessageStore) UpdateRead(ctx context.Context, taskID, ownerID string, readTime time.Time) error {
	row, found, err := s.core.repos.MessageExtends.GetByTask(ctx, taskID)
	if err != nil || !found {
		return err
	}
	// 仅（任务+处理人）命中时更新，避免误标记他人待办
	if rowString(row, "owner_id") != ownerID {
		return nil
	}
	_, err = s.core.repos.MessageExtends.MarkRead(ctx, taskID, readTime)
	return err
}

// UpdateDelay 标记待办已延时并回写延时时间点（经 GetByTask 定位主键后按主键更新）。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @param delayTime 延时时间点
// @return 错误
func (s *SQLMessageStore) UpdateDelay(ctx context.Context, taskID string, delayTime time.Time) error {
	row, found, err := s.core.repos.MessageExtends.GetByTask(ctx, taskID)
	if err != nil || !found {
		return err
	}
	_, err = s.core.repos.MessageExtends.UpdateByPK(ctx, rowString(row, "id"), pwf.Row{"is_delay": 1, "delay_time": delayTime})
	return err
}

// SetPlanFinishTime 设置计划办结时间（经 GetByTask 定位主键后按主键更新，nil 表示清空）。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @param planFinishTime 计划办结时间
// @return 错误
func (s *SQLMessageStore) SetPlanFinishTime(ctx context.Context, taskID string, planFinishTime *time.Time) error {
	row, found, err := s.core.repos.MessageExtends.GetByTask(ctx, taskID)
	if err != nil || !found {
		return err
	}
	changes := pwf.Row{}
	// nil 显式写入 NULL（键存在值为 nil），非 nil 写入时间
	if planFinishTime == nil {
		changes["plan_finish_time"] = nil
	} else {
		changes["plan_finish_time"] = *planFinishTime
	}
	_, err = s.core.repos.MessageExtends.UpdateByPK(ctx, rowString(row, "id"), changes)
	return err
}

// ─── HistoryExtend 行转换 ─────────────────────────────────────────────

// historyToRow 将已办扩展实体转为通用行。
// @param h 已办扩展实体
// @return 通用行
func historyToRow(h *HistoryExtend) pwf.Row {
	row := messageToRow(&h.MessageExtend)
	// 可空办结时间仅在有值时落库
	if h.TaskFinishTime != nil {
		row["task_finish_time"] = *h.TaskFinishTime
	}
	return row
}

// historyFromRow 将通用行转为已办扩展实体。
// @param row 通用行
// @return 已办扩展实体
func historyFromRow(row pwf.Row) *HistoryExtend {
	return &HistoryExtend{
		MessageExtend:  *messageFromRow(row),
		TaskFinishTime: rowTimePtr(row, "task_finish_time"),
	}
}

// historiesFromRows 批量转换已办扩展行。
// @param rows 通用行列表
// @return 实体列表
func historiesFromRows(rows []pwf.Row) []*HistoryExtend {
	out := make([]*HistoryExtend, 0, len(rows))
	for _, row := range rows {
		out = append(out, historyFromRow(row))
	}
	return out
}

// ─── SQLHistoryStore：已办扩展 ─────────────────────────────────────────────

// SQLHistoryStore 是 HistoryExtendStore 的 SQL 适配器。
type SQLHistoryStore struct {
	core *sqlCore // 共享内核
}

// Save 插入一条已办扩展。
// @param ctx 操作上下文
// @param h 已办扩展
// @return 错误
func (s *SQLHistoryStore) Save(ctx context.Context, h *HistoryExtend) error {
	return s.core.repos.HistoryExtends.Insert(ctx, historyToRow(h))
}

// GetByTaskID 按任务 ID 查询已办扩展。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @return 已办扩展
// @return 是否存在
// @return 错误
func (s *SQLHistoryStore) GetByTaskID(ctx context.Context, taskID string) (*HistoryExtend, bool, error) {
	row, found, err := s.core.repos.HistoryExtends.GetByTask(ctx, taskID)
	if err != nil || !found {
		return nil, false, err
	}
	return historyFromRow(row), true, nil
}

// ListByInstance 查询实例下全部已办扩展（按办结时间排序，聚合轨迹）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 列表
// @return 错误
func (s *SQLHistoryStore) ListByInstance(ctx context.Context, processInstanceID string) ([]*HistoryExtend, error) {
	rows, err := s.core.repos.HistoryExtends.ListByProcessInstance(ctx, processInstanceID)
	return historiesFromRows(rows), err
}

// ListByBusinessID 按业务单据与租户查询已办扩展。
// @param ctx 操作上下文
// @param businessID 业务单据 Id
// @param tenantID 租户
// @return 列表
// @return 错误
func (s *SQLHistoryStore) ListByBusinessID(ctx context.Context, businessID, tenantID string) ([]*HistoryExtend, error) {
	rows, err := s.core.repos.HistoryExtends.ListByBusiness(ctx, businessID, tenantID)
	return historiesFromRows(rows), err
}

// ListByOwner 按处理人分页查询已办扩展（仓储缺口，裸 SQL 只读补齐；P1.2 接入已办列表时再评估下推分页）。
// @param ctx 操作上下文
// @param ownerID 处理人 Id
// @param tenantID 租户
// @param page 分页参数
// @return 当前页列表
// @return 总数
// @return 错误
func (s *SQLHistoryStore) ListByOwner(ctx context.Context, ownerID, tenantID string, page PageQuery) ([]*HistoryExtend, int64, error) {
	rows, err := s.core.queryRaw(ctx, "SELECT * FROM workflow_task_history_extend WHERE owner_id = {p1} AND tenant_id = {p2} ORDER BY task_finish_time", []any{ownerID, tenantID})
	if err != nil {
		return nil, 0, err
	}
	data, total := paginate(historiesFromRows(rows), page.Start, page.Size)
	return data, total, nil
}

// UpdateFinishTime 办结回写：置（任务+处理人）记录的办结时间，并删除同任务其他处理人的记录
// （对位 Java updateByTaskIdAndOwnerId；仓储无按任务枚举方法，经实例列表过滤补齐）。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @param ownerID 处理人 Id
// @param finishTime 办结时间
// @return 错误
func (s *SQLHistoryStore) UpdateFinishTime(ctx context.Context, taskID, ownerID string, finishTime time.Time) error {
	anchor, found, err := s.core.repos.HistoryExtends.GetByTask(ctx, taskID)
	if err != nil || !found {
		return err
	}
	// 以锚点行定位实例，再枚举同任务全部记录
	rows, err := s.core.repos.HistoryExtends.ListByProcessInstance(ctx, rowString(anchor, "process_instance_id"))
	if err != nil {
		return err
	}
	for _, row := range rows {
		if rowString(row, "task_id") != taskID {
			continue
		}
		// 命中处理人回写办结时间，其余处理人删除（Java updateByTaskIdAndOwnerId）
		if rowString(row, "owner_id") == ownerID {
			if _, err := s.core.repos.HistoryExtends.UpdateByPK(ctx, rowString(row, "id"), pwf.Row{"task_finish_time": finishTime}); err != nil {
				return err
			}
		} else if _, err := s.core.repos.HistoryExtends.DeleteByPK(ctx, rowString(row, "id")); err != nil {
			return err
		}
	}
	return nil
}

// DeleteByTaskID 按任务删除已办扩展（经锚点行定位实例后逐条删除补齐）。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @return 错误
func (s *SQLHistoryStore) DeleteByTaskID(ctx context.Context, taskID string) error {
	anchor, found, err := s.core.repos.HistoryExtends.GetByTask(ctx, taskID)
	if err != nil || !found {
		return err
	}
	// 以锚点行定位实例，再枚举同任务全部记录逐条删除
	return s.deleteHistoryInInstance(ctx, rowString(anchor, "process_instance_id"), func(row pwf.Row) bool {
		return rowString(row, "task_id") == taskID
	})
}

// DeleteByInstance 按实例删除全部已办扩展。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 错误
func (s *SQLHistoryStore) DeleteByInstance(ctx context.Context, processInstanceID string) error {
	rows, err := s.core.repos.HistoryExtends.ListByProcessInstance(ctx, processInstanceID)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := s.core.repos.HistoryExtends.DeleteByPK(ctx, rowString(row, "id")); err != nil {
			return err
		}
	}
	return nil
}

// DeleteByBusinessID 按业务单据与租户删除已办扩展。
// @param ctx 操作上下文
// @param businessID 业务单据 Id
// @param tenantID 租户
// @return 错误
func (s *SQLHistoryStore) DeleteByBusinessID(ctx context.Context, businessID, tenantID string) error {
	rows, err := s.core.repos.HistoryExtends.ListByBusiness(ctx, businessID, tenantID)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := s.core.repos.HistoryExtends.DeleteByPK(ctx, rowString(row, "id")); err != nil {
			return err
		}
	}
	return nil
}

// DeleteByProcessInstanceAndActivityIDs 按实例与步骤代号集合删除已办扩展（退回/撤回清理，仓储缺口经列表过滤补齐）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param stepCodes 步骤代号集合
// @return 错误
func (s *SQLHistoryStore) DeleteByProcessInstanceAndActivityIDs(ctx context.Context, processInstanceID string, stepCodes []string) error {
	targets := make(map[string]bool, len(stepCodes))
	for _, code := range stepCodes {
		targets[code] = true
	}
	rows, err := s.core.repos.HistoryExtends.ListByProcessInstance(ctx, processInstanceID)
	if err != nil {
		return err
	}
	for _, row := range rows {
		// 命中步骤代号集合时逐条删除
		if targets[rowString(row, "task_step_code")] {
			if _, err := s.core.repos.HistoryExtends.DeleteByPK(ctx, rowString(row, "id")); err != nil {
				return err
			}
		}
	}
	return nil
}

// deleteHistoryInInstance 在实例范围内按谓词删除已办记录（调用方先经锚点定位实例）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param match 谓词
// @return 错误
func (s *SQLHistoryStore) deleteHistoryInInstance(ctx context.Context, processInstanceID string, match func(row pwf.Row) bool) error {
	rows, err := s.core.repos.HistoryExtends.ListByProcessInstance(ctx, processInstanceID)
	if err != nil {
		return err
	}
	for _, row := range rows {
		// 命中谓词时逐条按主键删除
		if match(row) {
			if _, err := s.core.repos.HistoryExtends.DeleteByPK(ctx, rowString(row, "id")); err != nil {
				return err
			}
		}
	}
	return nil
}

// ─── OperationRecord 行转换 ─────────────────────────────────────────────

// operationToRow 将操作记录实体转为通用行。
// @param r 操作记录实体
// @return 通用行
func operationToRow(r *OperationRecord) pwf.Row {
	row := pwf.Row{
		"business_id":          r.BusinessID,
		"task_id":              r.TaskID,
		"process_instance_id":  r.ProcessInstanceID,
		"message":              r.Message,
		"task_step_code":       r.TaskStepCode,
		"task_step_name":       r.TaskStepName,
		"flow_category_code":   r.FlowCategoryCode,
		"flow_category_name":   r.FlowCategoryName,
		"owner_id":             r.OwnerID,
		"owner_name":           r.OwnerName,
		"owner_group":          r.OwnerGroup,
		"owner_group_id":       r.OwnerGroupID,
		"owner_group_paths":    r.OwnerGroupPaths,
		"owner_group_path_ids": r.OwnerGroupPathIDs,
		"type":                 r.Type,
	}
	// 可空列仅在有值时落库，保持 NULL 语义
	if r.IsAgree != nil {
		row["is_agree"] = *r.IsAgree
	}
	if r.DealTime != nil {
		row["deal_time"] = *r.DealTime
	}
	if r.TaskReceiptTime != nil {
		row["task_receipt_time"] = *r.TaskReceiptTime
	}
	if r.TaskStartTime != nil {
		row["task_start_time"] = *r.TaskStartTime
	}
	if r.TaskFinishTime != nil {
		row["task_finish_time"] = *r.TaskFinishTime
	}
	appendBaseRow(row, r.BaseColumns)
	return row
}

// operationFromRow 将通用行转为操作记录实体。
// @param row 通用行
// @return 操作记录实体
func operationFromRow(row pwf.Row) *OperationRecord {
	return &OperationRecord{
		BaseColumns:       baseFromRow(row),
		BusinessID:        rowString(row, "business_id"),
		TaskID:            rowString(row, "task_id"),
		ProcessInstanceID: rowString(row, "process_instance_id"),
		Message:           rowString(row, "message"),
		IsAgree:           rowIntPtr(row, "is_agree"),
		TaskStepCode:      rowString(row, "task_step_code"),
		TaskStepName:      rowString(row, "task_step_name"),
		FlowCategoryCode:  rowString(row, "flow_category_code"),
		FlowCategoryName:  rowString(row, "flow_category_name"),
		OwnerID:           rowString(row, "owner_id"),
		OwnerName:         rowString(row, "owner_name"),
		OwnerGroup:        rowString(row, "owner_group"),
		OwnerGroupID:      rowString(row, "owner_group_id"),
		OwnerGroupPaths:   rowString(row, "owner_group_paths"),
		OwnerGroupPathIDs: rowString(row, "owner_group_path_ids"),
		DealTime:          rowTimePtr(row, "deal_time"),
		TaskReceiptTime:   rowTimePtr(row, "task_receipt_time"),
		TaskStartTime:     rowTimePtr(row, "task_start_time"),
		TaskFinishTime:    rowTimePtr(row, "task_finish_time"),
		Type:              rowString(row, "type"),
	}
}

// operationsFromRows 批量转换操作记录行。
// @param rows 通用行列表
// @return 实体列表
func operationsFromRows(rows []pwf.Row) []*OperationRecord {
	out := make([]*OperationRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, operationFromRow(row))
	}
	return out
}

// ─── SQLOperationStore：操作记录 ─────────────────────────────────────────────

// SQLOperationStore 是 OperationRecordStore 的 SQL 适配器。
type SQLOperationStore struct {
	core *sqlCore // 共享内核
}

// Save 插入一条操作记录。
// @param ctx 操作上下文
// @param r 操作记录
// @return 错误
func (s *SQLOperationStore) Save(ctx context.Context, r *OperationRecord) error {
	return s.core.repos.OperationRecords.Insert(ctx, operationToRow(r))
}

// ListByTask 查询任务的操作记录（按处理时间升序）。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @return 列表
// @return 错误
func (s *SQLOperationStore) ListByTask(ctx context.Context, taskID string) ([]*OperationRecord, error) {
	rows, err := s.core.repos.OperationRecords.ListByTask(ctx, taskID)
	return operationsFromRows(rows), err
}

// ListByInstance 查询实例的操作记录（按处理时间升序）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 列表
// @return 错误
func (s *SQLOperationStore) ListByInstance(ctx context.Context, processInstanceID string) ([]*OperationRecord, error) {
	rows, err := s.core.repos.OperationRecords.ListByProcessInstance(ctx, processInstanceID)
	return operationsFromRows(rows), err
}

// ListByBusinessID 查询业务单据的操作记录（按处理时间升序）。
// @param ctx 操作上下文
// @param businessID 业务单据 Id
// @return 列表
// @return 错误
func (s *SQLOperationStore) ListByBusinessID(ctx context.Context, businessID string) ([]*OperationRecord, error) {
	rows, err := s.core.repos.OperationRecords.ListByBusiness(ctx, businessID)
	return operationsFromRows(rows), err
}

// ListByBusinessIDs 查询多个业务单据的操作记录（仓储无 IN 查询，逐单据取回后按处理时间归并）。
// @param ctx 操作上下文
// @param businessIDs 业务单据 Id 集合
// @return 列表
// @return 错误
func (s *SQLOperationStore) ListByBusinessIDs(ctx context.Context, businessIDs []string) ([]*OperationRecord, error) {
	merged := make([]*OperationRecord, 0, len(businessIDs)*2)
	// 空集合直接返回，避免无谓查询
	for _, businessID := range businessIDs {
		rows, err := s.core.repos.OperationRecords.ListByBusiness(ctx, businessID)
		if err != nil {
			return nil, err
		}
		merged = append(merged, operationsFromRows(rows)...)
	}
	sortOperationRows(merged)
	return merged, nil
}

// ListSignable 查询"可签名"记录：type ∈ {TJ,SP,TZ} 且步骤代号非空（Java getSignableOperationRecordListByBusinessIds）。
// @param ctx 操作上下文
// @param businessIDs 业务单据 Id 集合
// @return 列表
// @return 错误
func (s *SQLOperationStore) ListSignable(ctx context.Context, businessIDs []string) ([]*OperationRecord, error) {
	rows, err := s.ListByBusinessIDs(ctx, businessIDs)
	if err != nil {
		return nil, err
	}
	signable := make([]*OperationRecord, 0, len(rows))
	for _, r := range rows {
		// 与 Java 一致：三类类型 + 步骤代号非空
		if (r.Type == CommentTypeTJ || r.Type == CommentTypeSP || r.Type == CommentTypeTZ) && r.TaskStepCode != "" {
			signable = append(signable, r)
		}
	}
	return signable, nil
}

// CountByInstance 统计实例的操作记录数（状态机"未提交/流转中"判据）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 记录数
// @return 错误
func (s *SQLOperationStore) CountByInstance(ctx context.Context, processInstanceID string) (int, error) {
	rows, err := s.core.repos.OperationRecords.ListByProcessInstance(ctx, processInstanceID)
	if err != nil {
		return 0, err
	}
	return len(rows), nil
}

// LastTypeByInstance 返回实例最后一条（按处理时间）操作记录的类型代码，无记录返回空串。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 类型代码
// @return 错误
func (s *SQLOperationStore) LastTypeByInstance(ctx context.Context, processInstanceID string) (string, error) {
	rows, err := s.core.repos.OperationRecords.ListByProcessInstance(ctx, processInstanceID)
	if err != nil || len(rows) == 0 {
		return "", err
	}
	return rowString(rows[len(rows)-1], "type"), nil
}

// sortOperationRows 按处理时间升序排序（缺省退化创建时间）。
// @param rows 实体列表
func sortOperationRows(rows []*OperationRecord) {
	for i := 1; i < len(rows); i++ {
		// 插入排序：与仓储 SQL 排序一致（deal_time asc）
		for j := i; j > 0 && opSortKey(rows[j]).Before(opSortKey(rows[j-1])); j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
}

// ─── Document 行转换 ─────────────────────────────────────────────

// documentToRow 将单据状态实体转为通用行。
// @param d 单据状态实体
// @return 通用行
func documentToRow(d *Document) pwf.Row {
	row := pwf.Row{
		"business_id":            d.BusinessID,
		"create_group_path_ids":  d.CreateGroupPathIDs,
		"create_group_paths":     d.CreateGroupPaths,
		"create_group_id":        d.CreateGroupID,
		"create_group":           d.CreateGroup,
		"updater_name":           d.UpdaterName,
		"deleted":                d.Deleted,
		"document_url":           d.DocumentURL,
		"document_id":            d.DocumentID,
		"document_table":         d.DocumentTable,
		"flow_title":             d.FlowTitle,
		"flow_task_id":           d.FlowTaskID,
		"flow_instance_id":       d.FlowInstanceID,
		"flow_category_id":       d.FlowCategoryID,
		"flow_category":          d.FlowCategory,
		"flow_action_id":         d.FlowActionID,
		"flow_action":            d.FlowAction,
		"flow_status":            d.FlowStatus,
		"flow_current_step_name": d.FlowCurrentStepName,
		"flow_current_persons":   d.FlowCurrentPersons,
	}
	// 可空时间列仅在有值时落库
	if d.FlowStartTime != nil {
		row["flow_start_time"] = *d.FlowStartTime
	}
	if d.FlowFinishTime != nil {
		row["flow_finish_time"] = *d.FlowFinishTime
	}
	if d.FlowTerminationTime != nil {
		row["flow_termination_time"] = *d.FlowTerminationTime
	}
	appendExtendRow(row, d.ExtendFields)
	appendBaseRow(row, d.BaseColumns)
	return row
}

// documentFromRow 将通用行转为单据状态实体。
// @param row 通用行
// @return 单据状态实体
func documentFromRow(row pwf.Row) *Document {
	return &Document{
		BaseColumns:         baseFromRow(row),
		BusinessID:          rowString(row, "business_id"),
		CreateGroupPathIDs:  rowString(row, "create_group_path_ids"),
		CreateGroupPaths:    rowString(row, "create_group_paths"),
		CreateGroupID:       rowString(row, "create_group_id"),
		CreateGroup:         rowString(row, "create_group"),
		UpdaterName:         rowString(row, "updater_name"),
		Deleted:             rowInt(row, "deleted"),
		DocumentURL:         rowString(row, "document_url"),
		DocumentID:          rowString(row, "document_id"),
		DocumentTable:       rowString(row, "document_table"),
		FlowTitle:           rowString(row, "flow_title"),
		FlowTaskID:          rowString(row, "flow_task_id"),
		FlowInstanceID:      rowString(row, "flow_instance_id"),
		FlowCategoryID:      rowString(row, "flow_category_id"),
		FlowCategory:        rowString(row, "flow_category"),
		FlowActionID:        rowString(row, "flow_action_id"),
		FlowAction:          rowString(row, "flow_action"),
		FlowStatus:          rowString(row, "flow_status"),
		FlowCurrentStepName: rowString(row, "flow_current_step_name"),
		FlowCurrentPersons:  rowString(row, "flow_current_persons"),
		FlowStartTime:       rowTimePtr(row, "flow_start_time"),
		FlowFinishTime:      rowTimePtr(row, "flow_finish_time"),
		FlowTerminationTime: rowTimePtr(row, "flow_termination_time"),
		ExtendFields:        extendFromRow(row),
	}
}

// ─── SQLDocumentStore：业务表单状态 ─────────────────────────────────────────────

// SQLDocumentStore 是 DocumentStore 的 SQL 适配器。
type SQLDocumentStore struct {
	core *sqlCore // 共享内核
}

// Save 按业务单据+租户 upsert（存在则更新非主键/非创建列，不存在则插入）。
// @param ctx 操作上下文
// @param d 单据状态
// @return 错误
func (s *SQLDocumentStore) Save(ctx context.Context, d *Document) error {
	return s.core.repos.Documents.UpsertByBusinessID(ctx, documentToRow(d))
}

// GetByBusinessID 按业务单据与租户查询未删除单据。
// @param ctx 操作上下文
// @param businessID 业务单据 Id
// @param tenantID 租户
// @return 单据
// @return 是否存在
// @return 错误
func (s *SQLDocumentStore) GetByBusinessID(ctx context.Context, businessID, tenantID string) (*Document, bool, error) {
	row, found, err := s.core.repos.Documents.GetByBusinessID(ctx, businessID, tenantID)
	if err != nil || !found {
		return nil, false, err
	}
	return documentFromRow(row), true, nil
}

// GetByInstance 按流程实例查询单据（实例下多行时取首行，Java selectOne 语义）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 单据
// @return 是否存在
// @return 错误
func (s *SQLDocumentStore) GetByInstance(ctx context.Context, processInstanceID string) (*Document, bool, error) {
	rows, err := s.core.repos.Documents.ListByFlowInstance(ctx, processInstanceID)
	if err != nil || len(rows) == 0 {
		return nil, false, err
	}
	return documentFromRow(rows[0]), true, nil
}

// UpdateByInstance 按实例更新指定列（仓储无按实例更新方法，经列表逐条按主键更新补齐）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param changes 变更列（列名→新值，nil 值表示置 NULL）
// @return 错误
func (s *SQLDocumentStore) UpdateByInstance(ctx context.Context, processInstanceID string, changes map[string]any) error {
	rows, err := s.core.repos.Documents.ListByFlowInstance(ctx, processInstanceID)
	if err != nil {
		return err
	}
	rowChanges := pwf.Row(changes)
	for _, row := range rows {
		if _, err := s.core.repos.Documents.UpdateByPK(ctx, rowString(row, "id"), rowChanges); err != nil {
			return err
		}
	}
	return nil
}

// MarkDeletedByBusinessID 按业务单据与租户逻辑删除。
// @param ctx 操作上下文
// @param businessID 业务单据 Id
// @param tenantID 租户
// @return 错误
func (s *SQLDocumentStore) MarkDeletedByBusinessID(ctx context.Context, businessID, tenantID string) error {
	_, err := s.core.repos.Documents.MarkDeletedByBusinessID(ctx, businessID, tenantID)
	return err
}
