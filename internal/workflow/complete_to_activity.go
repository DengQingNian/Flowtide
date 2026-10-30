package workflow

import (
	"context"
	"sync"

	"github.com/DengQingNian/Flowtide/internal/clock"
	pwf "github.com/DengQingNian/Flowtide/internal/persistence/workflow"
)

// CompleteToActivity 是 workflow_complete_to_activity（直送记录）的领域实体。
// 对位 Java WorkflowCompleteToActivityEntity：自由选路兜底迁移时落一条
// "从当前节点强制提交到目标节点"的轨迹（flow_id 对应 Java 随机连线 ID，Go 迁移语义不产生新连线，保留空值）。
type CompleteToActivity struct {
	BaseColumns              // 公共审计列
	FlowID            string // 连线 Id（Java 内存改线时生成的随机连线 ID；Go 引擎级迁移不产生新连线，为空）
	SourceActivityID  string // 来源节点 ID（办结任务的 taskDefinitionKey）
	TargetActivityID  string // 目标节点 ID（自由选路指定的直送目标）
	TaskID            string // 办结任务 Id
	ProcessInstanceID string // 流程实例 Id
}

// CompleteToActivityStore 是直送记录的存储端口（P1-E 新增，对位 Java workflowCompleteToActivityService）。
type CompleteToActivityStore interface {
	// Save 插入一条直送记录（id 由调用方生成）。
	// @param ctx 操作上下文
	// @param record 直送记录（必填 target/task/process_instance_id）
	// @return 错误
	Save(ctx context.Context, record *CompleteToActivity) error
	// ListByProcessInstance 查询实例的直送记录（按创建时间升序）。
	// @param ctx 操作上下文
	// @param processInstanceID 流程实例 Id
	// @return 记录列表
	// @return 错误
	ListByProcessInstance(ctx context.Context, processInstanceID string) ([]*CompleteToActivity, error)
	// ListByTask 查询任务的直送记录。
	// @param ctx 操作上下文
	// @param taskID 任务 Id
	// @return 记录列表
	// @return 错误
	ListByTask(ctx context.Context, taskID string) ([]*CompleteToActivity, error)
}

// ─── 内存实现 ─────────────────────────────────────────────

// MemCompleteToActivityStore 是直送记录端口的内存实现（线程安全 map + 行克隆存取），
// 供单元测试与嵌入式无 DB 模式使用。
type MemCompleteToActivityStore struct {
	mu    sync.RWMutex                   // 读写锁
	rows  map[string]*CompleteToActivity // id → 记录
	clock clock.Clock                    // 时间源（审计列填充）
	newID func() string                  // ID 生成函数
}

// NewMemCompleteToActivityStore 创建内存直送记录存储。
// @param c 时间源，nil 时回退真实时钟
// @return 内存存储
func NewMemCompleteToActivityStore(c clock.Clock) *MemCompleteToActivityStore {
	if c == nil {
		c = clock.Real{}
	}
	return &MemCompleteToActivityStore{rows: make(map[string]*CompleteToActivity), clock: c, newID: DefaultIDGen()}
}

// Save 插入一条直送记录（ID 缺失时自动生成，审计列缺失时按当前时间填充）。
// @param ctx 操作上下文
// @param record 直送记录
// @return 错误
func (m *MemCompleteToActivityStore) Save(ctx context.Context, record *CompleteToActivity) error {
	// 目标/任务/实例是直送轨迹的最小定位信息，缺失即拒绝
	if record == nil || record.TargetActivityID == "" || record.TaskID == "" || record.ProcessInstanceID == "" {
		return apiError("WORKFLOW-1601", "直送记录缺少目标/任务/实例信息")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	fillBase(&record.BaseColumns, m.newID, m.clock.Now, "", record.TenantID)
	m.rows[record.ID] = cloneCompleteToActivity(record)
	return nil
}

// ListByProcessInstance 查询实例的直送记录（按创建时间升序）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 记录列表
// @return 错误
func (m *MemCompleteToActivityStore) ListByProcessInstance(ctx context.Context, processInstanceID string) ([]*CompleteToActivity, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.listLocked(func(r *CompleteToActivity) bool { return r.ProcessInstanceID == processInstanceID }), nil
}

// ListByTask 查询任务的直送记录（按创建时间升序）。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @return 记录列表
// @return 错误
func (m *MemCompleteToActivityStore) ListByTask(ctx context.Context, taskID string) ([]*CompleteToActivity, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.listLocked(func(r *CompleteToActivity) bool { return r.TaskID == taskID }), nil
}

// listLocked 按谓词过滤并按创建时间升序返回（调用方须持锁）。
// @param match 过滤谓词
// @return 记录列表
func (m *MemCompleteToActivityStore) listLocked(match func(*CompleteToActivity) bool) []*CompleteToActivity {
	rows := make([]*CompleteToActivity, 0, 4)
	for _, row := range m.rows {
		if match(row) {
			rows = append(rows, cloneCompleteToActivity(row))
		}
	}
	// 按创建时间升序稳定排序，保证断言顺序确定
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && rows[j].CreateTime.Before(rows[j-1].CreateTime); j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
	return rows
}

// cloneCompleteToActivity 复制直送记录（行克隆存取）。
// @param r 原记录
// @return 副本
func cloneCompleteToActivity(r *CompleteToActivity) *CompleteToActivity {
	if r == nil {
		return nil
	}
	copied := *r
	return &copied
}

// 编译期断言：内存实现满足端口。
var _ CompleteToActivityStore = (*MemCompleteToActivityStore)(nil)

// ─── SQL 适配 ─────────────────────────────────────────────

// SQLCompleteToActivityStore 是直送记录端口的 SQL 适配器（组合 pwf 仓储，独立构造，
// 便于在不改动 store_sql.go 的前提下随 SQLStoreSet 并行装配）。
type SQLCompleteToActivityStore struct {
	repos *pwf.Repositories // workflow 行仓储聚合
}

// NewSQLCompleteToActivityStore 创建直送记录 SQL 适配器。
// @param store workflow 仓储入口（与 SQLStoreSet 共用同一 pwf.Store，事务经 ctx 透传）
// @return SQL 适配器
func NewSQLCompleteToActivityStore(store *pwf.Store) *SQLCompleteToActivityStore {
	return &SQLCompleteToActivityStore{repos: pwf.NewRepositories(store)}
}

// Save 插入一条直送记录。
// @param ctx 操作上下文
// @param record 直送记录
// @return 错误
func (s *SQLCompleteToActivityStore) Save(ctx context.Context, record *CompleteToActivity) error {
	return s.repos.CompleteToActivities.Insert(ctx, completeToActivityToRow(record))
}

// ListByProcessInstance 查询实例的直送记录（按创建时间升序）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 记录列表
// @return 错误
func (s *SQLCompleteToActivityStore) ListByProcessInstance(ctx context.Context, processInstanceID string) ([]*CompleteToActivity, error) {
	rows, err := s.repos.CompleteToActivities.ListByProcessInstance(ctx, processInstanceID)
	return completeToActivitiesFromRows(rows), err
}

// ListByTask 查询任务的直送记录。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @return 记录列表
// @return 错误
func (s *SQLCompleteToActivityStore) ListByTask(ctx context.Context, taskID string) ([]*CompleteToActivity, error) {
	rows, err := s.repos.CompleteToActivities.ListByTask(ctx, taskID)
	return completeToActivitiesFromRows(rows), err
}

// completeToActivityToRow 实体转行。
// @param record 直送记录
// @return 通用行
func completeToActivityToRow(record *CompleteToActivity) pwf.Row {
	return pwf.Row{
		"id":                     record.ID,
		"flow_id":                record.FlowID,
		"source_flow_element_id": record.SourceActivityID,
		"target_flow_element_id": record.TargetActivityID,
		"task_id":                record.TaskID,
		"process_instance_id":    record.ProcessInstanceID,
		"creator":                record.Creator,
		"creator_name":           record.CreatorName,
		"create_time":            record.CreateTime,
		"updater":                record.Updater,
		"update_time":            record.UpdateTime,
		"tenant_id":              record.TenantID,
	}
}

// completeToActivitiesFromRows 行列表转实体列表。
// @param rows 通用行列表
// @return 实体列表
func completeToActivitiesFromRows(rows []pwf.Row) []*CompleteToActivity {
	out := make([]*CompleteToActivity, 0, len(rows))
	for _, row := range rows {
		out = append(out, &CompleteToActivity{
			BaseColumns:       baseFromRow(row),
			FlowID:            rowString(row, "flow_id"),
			SourceActivityID:  rowString(row, "source_flow_element_id"),
			TargetActivityID:  rowString(row, "target_flow_element_id"),
			TaskID:            rowString(row, "task_id"),
			ProcessInstanceID: rowString(row, "process_instance_id"),
		})
	}
	return out
}

// 编译期断言：SQL 适配满足端口。
var _ CompleteToActivityStore = (*SQLCompleteToActivityStore)(nil)
