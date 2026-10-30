package workflow

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/DengQingNian/Flowtide/internal/clock"
)

// MemStores 是五大域存储端口的内存实现聚合（线程安全 map + 时钟注入），
// 供单元测试与嵌入式无 DB 模式使用；行数据按值克隆存取，避免调用方持有内部引用。
// 五个端口存在同名方法（Save/GetByTaskID 等），故拆分为独立结构体分别实现。
type MemStores struct {
	Next      *MemNextStore      // 下一步预生成-消耗
	Message   *MemMessageStore   // 待办扩展
	History   *MemHistoryStore   // 已办扩展
	Operation *MemOperationStore // 操作记录
	Document  *MemDocumentStore  // 业务表单状态
}

// NewMemStores 创建内存存储聚合。
// @param c 时间源，nil 时回退真实时钟
// @return 内存存储聚合
func NewMemStores(c clock.Clock) *MemStores {
	return &MemStores{
		Next:      NewMemNextStore(c),
		Message:   NewMemMessageStore(c),
		History:   NewMemHistoryStore(c),
		Operation: NewMemOperationStore(c),
		Document:  NewMemDocumentStore(c),
	}
}

// ─── 克隆工具 ─────────────────────────────────────────────

// cloneNext 复制下一步记录（含指针字段深拷贝）。
// @param a 原记录
// @return 副本
func cloneNext(a *NextActivity) *NextActivity {
	if a == nil {
		return nil
	}
	copied := *a
	copied.AgreeWeight = cloneFloat(a.AgreeWeight)
	copied.RejectWeight = cloneFloat(a.RejectWeight)
	return &copied
}

// cloneFloat 复制浮点指针。
// @param v 原指针
// @return 副本指针，nil 透传
func cloneFloat(v *float64) *float64 {
	if v == nil {
		return nil
	}
	copied := *v
	return &copied
}

// cloneTime 复制时间指针。
// @param t 原指针
// @return 副本指针，nil 透传
func cloneTime(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	copied := *t
	return &copied
}

// cloneI64 复制整型指针。
// @param v 原指针
// @return 副本指针，nil 透传
func cloneI64(v *int64) *int64 {
	if v == nil {
		return nil
	}
	copied := *v
	return &copied
}

// cloneInt 复制 int 指针。
// @param v 原指针
// @return 副本指针，nil 透传
func cloneInt(v *int) *int {
	if v == nil {
		return nil
	}
	copied := *v
	return &copied
}

// cloneExt 复制扩展列。
// @param e 原扩展列
// @return 副本
func cloneExt(e ExtendFields) ExtendFields {
	copied := e
	copied.ExtendI1, copied.ExtendI2, copied.ExtendI3 = cloneI64(e.ExtendI1), cloneI64(e.ExtendI2), cloneI64(e.ExtendI3)
	copied.ExtendF1, copied.ExtendF2, copied.ExtendF3 = cloneFloat(e.ExtendF1), cloneFloat(e.ExtendF2), cloneFloat(e.ExtendF3)
	copied.ExtendD1, copied.ExtendD2, copied.ExtendD3 = cloneTime(e.ExtendD1), cloneTime(e.ExtendD2), cloneTime(e.ExtendD3)
	return copied
}

// cloneMessage 复制待办扩展记录。
// @param msg 原记录
// @return 副本
func cloneMessage(msg *MessageExtend) *MessageExtend {
	if msg == nil {
		return nil
	}
	copied := *msg
	copied.ExtendFields = cloneExt(msg.ExtendFields)
	copied.ReadTime = cloneTime(msg.ReadTime)
	copied.PlanFinishTime = cloneTime(msg.PlanFinishTime)
	copied.TaskReceiptTime = cloneTime(msg.TaskReceiptTime)
	copied.TaskStartTime = cloneTime(msg.TaskStartTime)
	copied.DelayTime = cloneTime(msg.DelayTime)
	return &copied
}

// cloneHistory 复制已办扩展记录。
// @param h 原记录
// @return 副本
func cloneHistory(h *HistoryExtend) *HistoryExtend {
	if h == nil {
		return nil
	}
	copied := *h
	copied.MessageExtend = *cloneMessage(&h.MessageExtend)
	copied.TaskFinishTime = cloneTime(h.TaskFinishTime)
	return &copied
}

// cloneOperation 复制操作记录。
// @param r 原记录
// @return 副本
func cloneOperation(r *OperationRecord) *OperationRecord {
	if r == nil {
		return nil
	}
	copied := *r
	copied.IsAgree = cloneInt(r.IsAgree)
	copied.DealTime = cloneTime(r.DealTime)
	copied.TaskReceiptTime = cloneTime(r.TaskReceiptTime)
	copied.TaskStartTime = cloneTime(r.TaskStartTime)
	copied.TaskFinishTime = cloneTime(r.TaskFinishTime)
	return &copied
}

// cloneDocument 复制业务表单状态。
// @param d 原记录
// @return 副本
func cloneDocument(d *Document) *Document {
	if d == nil {
		return nil
	}
	copied := *d
	copied.ExtendFields = cloneExt(d.ExtendFields)
	copied.FlowStartTime = cloneTime(d.FlowStartTime)
	copied.FlowFinishTime = cloneTime(d.FlowFinishTime)
	copied.FlowTerminationTime = cloneTime(d.FlowTerminationTime)
	return &copied
}

// ─── MemNextStore：下一步预生成-消耗 ─────────────────────────────────────────────

// MemNextStore 是 NextActivityStore 的内存实现。
type MemNextStore struct {
	mu    sync.RWMutex             // 保护 next 与 his 两个 map
	clock clock.Clock              // 时间源（更新时间填充）
	next  map[string]*NextActivity // 运行态记录（id → 行）
	his   map[string]*NextActivity // 历史记录（id → 行）
}

// NewMemNextStore 创建下一步内存存储。
// @param c 时间源，nil 时回退真实时钟
// @return 内存存储
func NewMemNextStore(c clock.Clock) *MemNextStore {
	if c == nil {
		c = clock.Real{}
	}
	return &MemNextStore{clock: c, next: make(map[string]*NextActivity), his: make(map[string]*NextActivity)}
}

// 编译期断言。
var _ NextActivityStore = (*MemNextStore)(nil)

// Now 返回下一步存储时间源当前时间。
// @return 当前时间
func (m *MemNextStore) Now() time.Time { return m.clock.Now() }

// InsertDual 双写运行态与历史表（同 ID 同内容）。
// @param ctx 操作上下文
// @param act 下一步记录（必填 id/process_instance_id/next_activity_id）
// @return 双写错误
func (m *MemNextStore) InsertDual(ctx context.Context, act *NextActivity) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := cloneNext(act)
	m.next[copied.ID] = copied
	m.his[copied.ID] = cloneNext(copied)
	return nil
}

// LatestVersion 返回实例下指定节点已消耗（used=1）记录的最大版本号，无记录返回 0。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param nextActivityID 节点 ID
// @return 最大版本号
// @return 错误
func (m *MemNextStore) LatestVersion(ctx context.Context, processInstanceID, nextActivityID string) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	latest := 0
	for _, a := range m.his {
		// 仅统计已消耗记录（Java getNextActivityLatestVersionByProcInstAndTaskKey 语义）
		if a.ProcessInstanceID == processInstanceID && a.NextActivityID == nextActivityID && a.Used == 1 && a.Version > latest {
			latest = a.Version
		}
	}
	return latest, nil
}

// ListUnusedByInstanceAndActivity 查询实例下指定节点未消耗记录（按版本升序）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param nextActivityID 节点 ID
// @return 记录列表
// @return 错误
func (m *MemNextStore) ListUnusedByInstanceAndActivity(ctx context.Context, processInstanceID, nextActivityID string) ([]*NextActivity, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.listNext(false, func(a *NextActivity) bool {
		return a.ProcessInstanceID == processInstanceID && a.NextActivityID == nextActivityID && a.Used == 0
	}, "version"), nil
}

// ListUnusedByInstance 查询实例下全部未消耗记录（按版本升序）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 记录列表
// @return 错误
func (m *MemNextStore) ListUnusedByInstance(ctx context.Context, processInstanceID string) ([]*NextActivity, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.listNext(false, func(a *NextActivity) bool {
		return a.ProcessInstanceID == processInstanceID && a.Used == 0
	}, "version"), nil
}

// GetFirstUnusedByActivity 按创建时间升序返回实例下指定节点的第一条未消耗记录。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param nextActivityID 节点 ID
// @return 记录
// @return 是否存在
// @return 错误
func (m *MemNextStore) GetFirstUnusedByActivity(ctx context.Context, processInstanceID, nextActivityID string) (*NextActivity, bool, error) {
	rows, err := m.ListUnusedByInstanceAndActivity(ctx, processInstanceID, nextActivityID)
	if err != nil || len(rows) == 0 {
		return nil, false, err
	}
	// 再按创建时间稳定取最早一条（Java getNextActivityByProcInstAndTaskKey：orderByAsc create_time）
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].CreateTime.Before(rows[j].CreateTime)
	})
	return rows[0], true, nil
}

// ListByPreTask 查询以上一任务为来源的运行态记录（按版本升序）。
// @param ctx 操作上下文
// @param preTaskID 上一任务 Id
// @return 记录列表
// @return 错误
func (m *MemNextStore) ListByPreTask(ctx context.Context, preTaskID string) ([]*NextActivity, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.listNext(false, func(a *NextActivity) bool {
		return a.PreTaskID == preTaskID
	}, "version"), nil
}

// ListByTaskID 查询已挂到任务上的运行态记录（按创建时间升序）。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @return 记录列表
// @return 错误
func (m *MemNextStore) ListByTaskID(ctx context.Context, taskID string) ([]*NextActivity, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.listNext(false, func(a *NextActivity) bool {
		return a.TaskID == taskID
	}, "create_time"), nil
}

// ListUsedByInstance 查询实例下已回写任务 ID 的记录。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 记录列表
// @return 错误
func (m *MemNextStore) ListUsedByInstance(ctx context.Context, processInstanceID string) ([]*NextActivity, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.listNext(false, func(a *NextActivity) bool {
		// Java getNextActivityListByProcessInstanceId：task_id is not null
		return a.ProcessInstanceID == processInstanceID && a.TaskID != ""
	}, "create_time"), nil
}

// MarkUsedAndWriteBack 置 used=1 并回写任务 ID（运行态与历史表同步，仅未消耗记录生效）。
// @param ctx 操作上下文
// @param ids 记录 Id 集合
// @param taskID 任务 Id
// @return 错误
func (m *MemNextStore) MarkUsedAndWriteBack(ctx context.Context, ids []string, taskID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range ids {
		// 与 SQL 版 MarkUsed 一致：WHERE used = 0，防并发重复消耗
		if a, ok := m.next[id]; ok && a.Used == 0 {
			a.Used = 1
			a.TaskID = taskID
		}
		if a, ok := m.his[id]; ok && a.Used == 0 {
			a.Used = 1
			a.TaskID = taskID
		}
	}
	return nil
}

// ListHisByInstance 查询实例的全部历史记录（按版本升序）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 记录列表
// @return 错误
func (m *MemNextStore) ListHisByInstance(ctx context.Context, processInstanceID string) ([]*NextActivity, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.listNext(true, func(a *NextActivity) bool {
		return a.ProcessInstanceID == processInstanceID
	}, "version"), nil
}

// ListHisByTaskID 查询历史表中任务相关且非自环（pre_task_id != taskID）的记录（按创建时间升序）。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @return 记录列表
// @return 错误
func (m *MemNextStore) ListHisByTaskID(ctx context.Context, taskID string) ([]*NextActivity, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.listNext(true, func(a *NextActivity) bool {
		// Java WorkflowHisNextActivityServiceImpl#findByTaskId：排除自环（转办场景 pre_task_id = task_id）
		return a.TaskID == taskID && a.PreTaskID != taskID
	}, "create_time"), nil
}

// DeleteByProcessInstance 删除实例的全部运行态记录。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 错误
func (m *MemNextStore) DeleteByProcessInstance(ctx context.Context, processInstanceID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, a := range m.next {
		if a.ProcessInstanceID == processInstanceID {
			delete(m.next, id)
		}
	}
	return nil
}

// DeleteByProcessInstanceAndActivityIDs 删除实例下指定节点集合的运行态与历史记录。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param nextActivityIDs 节点 ID 集合
// @return 错误
func (m *MemNextStore) DeleteByProcessInstanceAndActivityIDs(ctx context.Context, processInstanceID string, nextActivityIDs []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	targets := make(map[string]bool, len(nextActivityIDs))
	for _, id := range nextActivityIDs {
		targets[id] = true
	}
	for id, a := range m.next {
		if a.ProcessInstanceID == processInstanceID && targets[a.NextActivityID] {
			delete(m.next, id)
		}
	}
	for id, a := range m.his {
		if a.ProcessInstanceID == processInstanceID && targets[a.NextActivityID] {
			delete(m.his, id)
		}
	}
	return nil
}

// DeleteForBackToStep 退回清理：删除实例下目标节点（含已消耗）的全部运行态记录。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param nextActivityID 目标节点 ID
// @return 错误
func (m *MemNextStore) DeleteForBackToStep(ctx context.Context, processInstanceID, nextActivityID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, a := range m.next {
		if a.ProcessInstanceID == processInstanceID && a.NextActivityID == nextActivityID {
			delete(m.next, id)
		}
	}
	return nil
}

// DeleteForRollback 撤回清理：删除实例下以 preTaskID 为来源、目标节点之外的记录（仅运行态）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param preTaskID 来源任务 Id
// @param keepActivityID 保留的目标节点 ID（撤回重做的节点）
// @return 错误
func (m *MemNextStore) DeleteForRollback(ctx context.Context, processInstanceID, preTaskID, keepActivityID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, a := range m.next {
		// Java deleteRollbackWorkflowNextActivity：删除当前节点之外的记录
		if a.ProcessInstanceID == processInstanceID && a.PreTaskID == preTaskID && a.NextActivityID != keepActivityID {
			delete(m.next, id)
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
func (m *MemNextStore) DeleteForResolve(ctx context.Context, processInstanceID, nextActivityID, taskID, identityID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, a := range m.next {
		if a.ProcessInstanceID == processInstanceID && a.NextActivityID == nextActivityID && a.TaskID == taskID && a.IdentityID == identityID {
			delete(m.next, id)
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
func (m *MemNextStore) FindAssignSource(ctx context.Context, processInstanceID, nextActivityID, sendIdentityID, taskID string) (*NextActivity, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	// Java findNextActivityByParams 取首条匹配
	for _, a := range m.next {
		if a.ProcessInstanceID == processInstanceID && a.NextActivityID == nextActivityID && a.SendIdentityID == sendIdentityID && a.TaskID == taskID {
			return cloneNext(a), true, nil
		}
	}
	return nil, false, nil
}

// listNext 按谓词过滤并排序（调用方须持锁）。
// @param his true 查历史表，false 查运行态
// @param match 谓词
// @param order 排序字段："version" 或 "create_time"
// @return 记录列表
func (m *MemNextStore) listNext(his bool, match func(*NextActivity) bool, order string) []*NextActivity {
	source := m.next
	// 按查询目标切换数据表
	if his {
		source = m.his
	}
	rows := make([]*NextActivity, 0, 4)
	for _, a := range source {
		if match(a) {
			rows = append(rows, cloneNext(a))
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if order == "version" {
			// 版本相同再按创建时间稳定排序，保证多执行人批次顺序确定
			if rows[i].Version != rows[j].Version {
				return rows[i].Version < rows[j].Version
			}
			return rows[i].CreateTime.Before(rows[j].CreateTime)
		}
		return rows[i].CreateTime.Before(rows[j].CreateTime)
	})
	return rows
}

// ─── MemMessageStore：待办扩展 ─────────────────────────────────────────────

// MemMessageStore 是 MessageExtendStore 的内存实现。
type MemMessageStore struct {
	mu    sync.RWMutex              // 保护 msg map
	clock clock.Clock               // 时间源
	msg   map[string]*MessageExtend // 待办扩展（id → 行）
}

// NewMemMessageStore 创建待办扩展内存存储。
// @param c 时间源，nil 时回退真实时钟
// @return 内存存储
func NewMemMessageStore(c clock.Clock) *MemMessageStore {
	if c == nil {
		c = clock.Real{}
	}
	return &MemMessageStore{clock: c, msg: make(map[string]*MessageExtend)}
}

// 编译期断言。
var _ MessageExtendStore = (*MemMessageStore)(nil)

// Now 返回待办扩展存储时间源当前时间。
// @return 当前时间
func (m *MemMessageStore) Now() time.Time { return m.clock.Now() }

// Save 插入一条待办扩展。
// @param ctx 操作上下文
// @param msg 待办扩展（必填 id）
// @return 错误
func (m *MemMessageStore) Save(ctx context.Context, msg *MessageExtend) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.msg[msg.ID] = cloneMessage(msg)
	return nil
}

// GetByTaskID 按任务 ID 查询待办扩展。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @return 待办扩展
// @return 是否存在
// @return 错误
func (m *MemMessageStore) GetByTaskID(ctx context.Context, taskID string) (*MessageExtend, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, msg := range m.msg {
		if msg.TaskID == taskID {
			return cloneMessage(msg), true, nil
		}
	}
	return nil, false, nil
}

// ListByInstance 查询实例下全部待办扩展（按创建时间升序）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 列表
// @return 错误
func (m *MemMessageStore) ListByInstance(ctx context.Context, processInstanceID string) ([]*MessageExtend, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.listLocked(func(msg *MessageExtend) bool {
		return msg.ProcessInstanceID == processInstanceID
	}), nil
}

// ListByBusinessID 按业务单据与租户查询待办扩展（按创建时间升序）。
// @param ctx 操作上下文
// @param businessID 业务单据 Id
// @param tenantID 租户
// @return 列表
// @return 错误
func (m *MemMessageStore) ListByBusinessID(ctx context.Context, businessID, tenantID string) ([]*MessageExtend, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.listLocked(func(msg *MessageExtend) bool {
		return msg.BusinessID == businessID && (tenantID == "" || msg.TenantID == tenantID)
	}), nil
}

// ListByOwner 按处理人分页查询待办扩展（租户过滤），返回当前页与总数。
// @param ctx 操作上下文
// @param ownerID 处理人 Id
// @param tenantID 租户
// @param page 分页参数
// @return 当前页列表
// @return 总数
// @return 错误
func (m *MemMessageStore) ListByOwner(ctx context.Context, ownerID, tenantID string, page PageQuery) ([]*MessageExtend, int64, error) {
	m.mu.RLock()
	rows := m.listLocked(func(msg *MessageExtend) bool {
		return msg.OwnerID == ownerID && (tenantID == "" || msg.TenantID == tenantID)
	})
	m.mu.RUnlock()
	data, total := paginate(rows, page.Start, page.Size)
	return data, total, nil
}

// DeleteByTaskID 按任务删除待办扩展。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @return 错误
func (m *MemMessageStore) DeleteByTaskID(ctx context.Context, taskID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, msg := range m.msg {
		if msg.TaskID == taskID {
			delete(m.msg, id)
		}
	}
	return nil
}

// DeleteByTaskIDAndOwner 按任务与处理人删除待办扩展。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @param ownerID 处理人 Id
// @return 错误
func (m *MemMessageStore) DeleteByTaskIDAndOwner(ctx context.Context, taskID, ownerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, msg := range m.msg {
		if msg.TaskID == taskID && msg.OwnerID == ownerID {
			delete(m.msg, id)
		}
	}
	return nil
}

// DeleteByInstance 按实例删除全部待办扩展。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 错误
func (m *MemMessageStore) DeleteByInstance(ctx context.Context, processInstanceID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, msg := range m.msg {
		if msg.ProcessInstanceID == processInstanceID {
			delete(m.msg, id)
		}
	}
	return nil
}

// DeleteByBusinessID 按业务单据与租户删除待办扩展。
// @param ctx 操作上下文
// @param businessID 业务单据 Id
// @param tenantID 租户
// @return 错误
func (m *MemMessageStore) DeleteByBusinessID(ctx context.Context, businessID, tenantID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, msg := range m.msg {
		if msg.BusinessID == businessID && (tenantID == "" || msg.TenantID == tenantID) {
			delete(m.msg, id)
		}
	}
	return nil
}

// DeleteByProcessInstanceAndActivityIDs 按实例与步骤代号集合删除待办扩展（退回/撤回清理）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param stepCodes 步骤代号集合
// @return 错误
func (m *MemMessageStore) DeleteByProcessInstanceAndActivityIDs(ctx context.Context, processInstanceID string, stepCodes []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	targets := make(map[string]bool, len(stepCodes))
	for _, code := range stepCodes {
		targets[code] = true
	}
	for id, msg := range m.msg {
		if msg.ProcessInstanceID == processInstanceID && targets[msg.TaskStepCode] {
			delete(m.msg, id)
		}
	}
	return nil
}

// UpdateRead 标记待办已读并回写阅读时间（按任务+处理人）。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @param ownerID 处理人 Id
// @param readTime 阅读时间
// @return 错误
func (m *MemMessageStore) UpdateRead(ctx context.Context, taskID, ownerID string, readTime time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, msg := range m.msg {
		if msg.TaskID == taskID && msg.OwnerID == ownerID {
			msg.IsRead = 1
			stamped := readTime
			msg.ReadTime = &stamped
		}
	}
	return nil
}

// UpdateDelay 标记待办已延时并回写延时时间点。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @param delayTime 延时时间点
// @return 错误
func (m *MemMessageStore) UpdateDelay(ctx context.Context, taskID string, delayTime time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, msg := range m.msg {
		if msg.TaskID == taskID {
			msg.IsDelay = 1
			stamped := delayTime
			msg.DelayTime = &stamped
		}
	}
	return nil
}

// SetPlanFinishTime 设置计划办结时间（nil 表示清空）。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @param planFinishTime 计划办结时间
// @return 错误
func (m *MemMessageStore) SetPlanFinishTime(ctx context.Context, taskID string, planFinishTime *time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, msg := range m.msg {
		if msg.TaskID == taskID {
			msg.PlanFinishTime = cloneTime(planFinishTime)
		}
	}
	return nil
}

// listLocked 按谓词过滤待办扩展并按创建时间升序（调用方须持锁）。
// @param match 谓词
// @return 列表
func (m *MemMessageStore) listLocked(match func(*MessageExtend) bool) []*MessageExtend {
	rows := make([]*MessageExtend, 0, 4)
	for _, msg := range m.msg {
		if match(msg) {
			rows = append(rows, cloneMessage(msg))
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].CreateTime.Before(rows[j].CreateTime)
	})
	return rows
}

// paginate 对已排序列表做内存分页。
// @param rows 全量列表
// @param start 偏移量
// @param size 每页数量，<=0 表示全量
// @return 当前页数据
// @return 总数
func paginate[T any](rows []T, start, size int) ([]T, int64) {
	total := int64(len(rows))
	// size<=0 视为不分页，直接返回全量
	if size <= 0 {
		if start > len(rows) || start < 0 {
			start = 0
		}
		return rows[start:], total
	}
	if start < 0 {
		start = 0
	}
	if start >= len(rows) {
		return []T{}, total
	}
	end := start + size
	if end > len(rows) {
		end = len(rows)
	}
	return rows[start:end], total
}

// ─── MemHistoryStore：已办扩展 ─────────────────────────────────────────────

// MemHistoryStore 是 HistoryExtendStore 的内存实现。
type MemHistoryStore struct {
	mu    sync.RWMutex              // 保护 hist map
	clock clock.Clock               // 时间源
	hist  map[string]*HistoryExtend // 已办扩展（id → 行）
}

// NewMemHistoryStore 创建已办扩展内存存储。
// @param c 时间源，nil 时回退真实时钟
// @return 内存存储
func NewMemHistoryStore(c clock.Clock) *MemHistoryStore {
	if c == nil {
		c = clock.Real{}
	}
	return &MemHistoryStore{clock: c, hist: make(map[string]*HistoryExtend)}
}

// 编译期断言。
var _ HistoryExtendStore = (*MemHistoryStore)(nil)

// Now 返回已办扩展存储时间源当前时间。
// @return 当前时间
func (m *MemHistoryStore) Now() time.Time { return m.clock.Now() }

// Save 插入一条已办扩展。
// @param ctx 操作上下文
// @param h 已办扩展（必填 id）
// @return 错误
func (m *MemHistoryStore) Save(ctx context.Context, h *HistoryExtend) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hist[h.ID] = cloneHistory(h)
	return nil
}

// GetByTaskID 按任务 ID 查询已办扩展。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @return 已办扩展
// @return 是否存在
// @return 错误
func (m *MemHistoryStore) GetByTaskID(ctx context.Context, taskID string) (*HistoryExtend, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, h := range m.hist {
		if h.TaskID == taskID {
			return cloneHistory(h), true, nil
		}
	}
	return nil, false, nil
}

// ListByInstance 查询实例下全部已办扩展（按办结时间/创建时间升序，聚合轨迹）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 列表
// @return 错误
func (m *MemHistoryStore) ListByInstance(ctx context.Context, processInstanceID string) ([]*HistoryExtend, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rows := make([]*HistoryExtend, 0, 4)
	for _, h := range m.hist {
		if h.ProcessInstanceID == processInstanceID {
			rows = append(rows, cloneHistory(h))
		}
	}
	sortHistoryRows(rows)
	return rows, nil
}

// ListByBusinessID 按业务单据与租户查询已办扩展。
// @param ctx 操作上下文
// @param businessID 业务单据 Id
// @param tenantID 租户
// @return 列表
// @return 错误
func (m *MemHistoryStore) ListByBusinessID(ctx context.Context, businessID, tenantID string) ([]*HistoryExtend, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rows := make([]*HistoryExtend, 0, 4)
	for _, h := range m.hist {
		if h.BusinessID == businessID && (tenantID == "" || h.TenantID == tenantID) {
			rows = append(rows, cloneHistory(h))
		}
	}
	sortHistoryRows(rows)
	return rows, nil
}

// ListByOwner 按处理人分页查询已办扩展（租户过滤），返回当前页与总数。
// @param ctx 操作上下文
// @param ownerID 处理人 Id
// @param tenantID 租户
// @param page 分页参数
// @return 当前页列表
// @return 总数
// @return 错误
func (m *MemHistoryStore) ListByOwner(ctx context.Context, ownerID, tenantID string, page PageQuery) ([]*HistoryExtend, int64, error) {
	m.mu.RLock()
	rows := make([]*HistoryExtend, 0, 4)
	for _, h := range m.hist {
		if h.OwnerID == ownerID && (tenantID == "" || h.TenantID == tenantID) {
			rows = append(rows, cloneHistory(h))
		}
	}
	m.mu.RUnlock()
	sortHistoryRows(rows)
	data, total := paginate(rows, page.Start, page.Size)
	return data, total, nil
}

// UpdateFinishTime 办结回写：置（任务+处理人）记录的办结时间，并删除同任务其他处理人的记录。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @param ownerID 处理人 Id
// @param finishTime 办结时间
// @return 错误
func (m *MemHistoryStore) UpdateFinishTime(ctx context.Context, taskID, ownerID string, finishTime time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	stamped := finishTime
	for _, h := range m.hist {
		// 命中（任务+处理人）时回写办结时间
		if h.TaskID == taskID && h.OwnerID == ownerID {
			h.TaskFinishTime = &stamped
		}
	}
	for id, h := range m.hist {
		// Java updateByTaskIdAndOwnerId：删除当前任务其他处理人的已办记录
		if h.TaskID == taskID && h.OwnerID != ownerID {
			delete(m.hist, id)
		}
	}
	return nil
}

// DeleteByTaskID 按任务删除已办扩展。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @return 错误
func (m *MemHistoryStore) DeleteByTaskID(ctx context.Context, taskID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, h := range m.hist {
		if h.TaskID == taskID {
			delete(m.hist, id)
		}
	}
	return nil
}

// DeleteByInstance 按实例删除全部已办扩展。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 错误
func (m *MemHistoryStore) DeleteByInstance(ctx context.Context, processInstanceID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, h := range m.hist {
		if h.ProcessInstanceID == processInstanceID {
			delete(m.hist, id)
		}
	}
	return nil
}

// DeleteByBusinessID 按业务单据与租户删除已办扩展。
// @param ctx 操作上下文
// @param businessID 业务单据 Id
// @param tenantID 租户
// @return 错误
func (m *MemHistoryStore) DeleteByBusinessID(ctx context.Context, businessID, tenantID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, h := range m.hist {
		if h.BusinessID == businessID && (tenantID == "" || h.TenantID == tenantID) {
			delete(m.hist, id)
		}
	}
	return nil
}

// DeleteByProcessInstanceAndActivityIDs 按实例与步骤代号集合删除已办扩展（退回/撤回清理）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param stepCodes 步骤代号集合
// @return 错误
func (m *MemHistoryStore) DeleteByProcessInstanceAndActivityIDs(ctx context.Context, processInstanceID string, stepCodes []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	targets := make(map[string]bool, len(stepCodes))
	for _, code := range stepCodes {
		targets[code] = true
	}
	for id, h := range m.hist {
		if h.ProcessInstanceID == processInstanceID && targets[h.TaskStepCode] {
			delete(m.hist, id)
		}
	}
	return nil
}

// sortHistoryRows 按办结时间/创建时间升序排序已办扩展。
// @param rows 列表
func sortHistoryRows(rows []*HistoryExtend) {
	sort.SliceStable(rows, func(i, j int) bool {
		// TODO-P5：按 his_step.sort 聚合排序（P5 接入），当前以办结时间/创建时间近似
		return historySortKey(rows[i]).Before(historySortKey(rows[j]))
	})
}

// historySortKey 返回已办扩展的排序键：优先办结时间，缺省用创建时间。
// @param h 已办扩展
// @return 排序时间
func historySortKey(h *HistoryExtend) time.Time {
	if h.TaskFinishTime != nil {
		return *h.TaskFinishTime
	}
	return h.CreateTime
}

// ─── MemOperationStore：操作记录 ─────────────────────────────────────────────

// MemOperationStore 是 OperationRecordStore 的内存实现。
type MemOperationStore struct {
	mu    sync.RWMutex                // 保护 op map
	clock clock.Clock                 // 时间源
	op    map[string]*OperationRecord // 操作记录（id → 行）
}

// NewMemOperationStore 创建操作记录内存存储。
// @param c 时间源，nil 时回退真实时钟
// @return 内存存储
func NewMemOperationStore(c clock.Clock) *MemOperationStore {
	if c == nil {
		c = clock.Real{}
	}
	return &MemOperationStore{clock: c, op: make(map[string]*OperationRecord)}
}

// 编译期断言。
var _ OperationRecordStore = (*MemOperationStore)(nil)

// Now 返回操作记录存储时间源当前时间。
// @return 当前时间
func (m *MemOperationStore) Now() time.Time { return m.clock.Now() }

// Save 插入一条操作记录。
// @param ctx 操作上下文
// @param r 操作记录（必填 id）
// @return 错误
func (m *MemOperationStore) Save(ctx context.Context, r *OperationRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.op[r.ID] = cloneOperation(r)
	return nil
}

// ListByTask 查询任务的操作记录（按处理时间升序）。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @return 列表
// @return 错误
func (m *MemOperationStore) ListByTask(ctx context.Context, taskID string) ([]*OperationRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.listLocked(func(r *OperationRecord) bool {
		return r.TaskID == taskID
	}), nil
}

// ListByInstance 查询实例的操作记录（按处理时间升序）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 列表
// @return 错误
func (m *MemOperationStore) ListByInstance(ctx context.Context, processInstanceID string) ([]*OperationRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.listLocked(func(r *OperationRecord) bool {
		return r.ProcessInstanceID == processInstanceID
	}), nil
}

// ListByBusinessID 查询业务单据的操作记录（按处理时间升序）。
// @param ctx 操作上下文
// @param businessID 业务单据 Id
// @return 列表
// @return 错误
func (m *MemOperationStore) ListByBusinessID(ctx context.Context, businessID string) ([]*OperationRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.listLocked(func(r *OperationRecord) bool {
		return r.BusinessID == businessID
	}), nil
}

// ListByBusinessIDs 查询多个业务单据的操作记录（按处理时间升序）。
// @param ctx 操作上下文
// @param businessIDs 业务单据 Id 集合
// @return 列表
// @return 错误
func (m *MemOperationStore) ListByBusinessIDs(ctx context.Context, businessIDs []string) ([]*OperationRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	targets := make(map[string]bool, len(businessIDs))
	for _, id := range businessIDs {
		targets[id] = true
	}
	return m.listLocked(func(r *OperationRecord) bool {
		return targets[r.BusinessID]
	}), nil
}

// ListSignable 查询"可签名"记录：type ∈ {TJ,SP,TZ} 且步骤代号非空（Java getSignableOperationRecordListByBusinessIds）。
// @param ctx 操作上下文
// @param businessIDs 业务单据 Id 集合
// @return 列表
// @return 错误
func (m *MemOperationStore) ListSignable(ctx context.Context, businessIDs []string) ([]*OperationRecord, error) {
	rows, err := m.ListByBusinessIDs(ctx, businessIDs)
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
func (m *MemOperationStore) CountByInstance(ctx context.Context, processInstanceID string) (int, error) {
	rows, err := m.ListByInstance(ctx, processInstanceID)
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
func (m *MemOperationStore) LastTypeByInstance(ctx context.Context, processInstanceID string) (string, error) {
	rows, err := m.ListByInstance(ctx, processInstanceID)
	if err != nil || len(rows) == 0 {
		return "", err
	}
	return rows[len(rows)-1].Type, nil
}

// listLocked 按谓词过滤操作记录并按处理时间升序（调用方须持锁）。
// @param match 谓词
// @return 列表
func (m *MemOperationStore) listLocked(match func(*OperationRecord) bool) []*OperationRecord {
	rows := make([]*OperationRecord, 0, 4)
	for _, r := range m.op {
		if match(r) {
			rows = append(rows, cloneOperation(r))
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		// 处理时间相同再按创建时间稳定排序
		ti, tj := opSortKey(rows[i]), opSortKey(rows[j])
		if ti.Equal(tj) {
			return rows[i].CreateTime.Before(rows[j].CreateTime)
		}
		return ti.Before(tj)
	})
	return rows
}

// opSortKey 返回操作记录排序键：优先处理时间，缺省用创建时间。
// @param r 操作记录
// @return 排序时间
func opSortKey(r *OperationRecord) time.Time {
	if r.DealTime != nil {
		return *r.DealTime
	}
	return r.CreateTime
}

// ─── MemDocumentStore：业务表单状态 ─────────────────────────────────────────────

// MemDocumentStore 是 DocumentStore 的内存实现。
type MemDocumentStore struct {
	mu    sync.RWMutex         // 保护 doc map
	clock clock.Clock          // 时间源
	doc   map[string]*Document // 单据状态（id → 行）
}

// NewMemDocumentStore 创建业务表单状态内存存储。
// @param c 时间源，nil 时回退真实时钟
// @return 内存存储
func NewMemDocumentStore(c clock.Clock) *MemDocumentStore {
	if c == nil {
		c = clock.Real{}
	}
	return &MemDocumentStore{clock: c, doc: make(map[string]*Document)}
}

// 编译期断言。
var _ DocumentStore = (*MemDocumentStore)(nil)

// Now 返回单据存储时间源当前时间。
// @return 当前时间
func (m *MemDocumentStore) Now() time.Time { return m.clock.Now() }

// Save 按业务单据+租户 upsert：存在则更新非主键/非创建列，不存在则插入。
// @param ctx 操作上下文
// @param d 单据状态（必填 id/business_id/tenant_id）
// @return 错误
func (m *MemDocumentStore) Save(ctx context.Context, d *Document) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	// 按（业务单据+租户）定位既有未删除行，命中则整行替换（保留原主键）
	for _, existing := range m.doc {
		if existing.BusinessID == d.BusinessID && existing.TenantID == d.TenantID && existing.Deleted == 0 {
			updated := cloneDocument(d)
			updated.ID = existing.ID
			m.doc[existing.ID] = updated
			return nil
		}
	}
	m.doc[d.ID] = cloneDocument(d)
	return nil
}

// GetByBusinessID 按业务单据与租户查询未删除单据。
// @param ctx 操作上下文
// @param businessID 业务单据 Id
// @param tenantID 租户
// @return 单据
// @return 是否存在
// @return 错误
func (m *MemDocumentStore) GetByBusinessID(ctx context.Context, businessID, tenantID string) (*Document, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, d := range m.doc {
		if d.BusinessID == businessID && (tenantID == "" || d.TenantID == tenantID) && d.Deleted == 0 {
			return cloneDocument(d), true, nil
		}
	}
	return nil, false, nil
}

// GetByInstance 按流程实例查询单据。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 单据
// @return 是否存在
// @return 错误
func (m *MemDocumentStore) GetByInstance(ctx context.Context, processInstanceID string) (*Document, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, d := range m.doc {
		if d.FlowInstanceID == processInstanceID && d.Deleted == 0 {
			return cloneDocument(d), true, nil
		}
	}
	return nil, false, nil
}

// UpdateByInstance 按实例更新指定列（changes 为列名→新值，nil 值表示置 NULL）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param changes 变更列
// @return 错误
func (m *MemDocumentStore) UpdateByInstance(ctx context.Context, processInstanceID string, changes map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, d := range m.doc {
		if d.FlowInstanceID == processInstanceID && d.Deleted == 0 {
			applyDocumentChanges(d, changes)
			d.UpdateTime = m.clock.Now()
		}
	}
	return nil
}

// MarkDeletedByBusinessID 按业务单据与租户逻辑删除。
// @param ctx 操作上下文
// @param businessID 业务单据 Id
// @param tenantID 租户
// @return 错误
func (m *MemDocumentStore) MarkDeletedByBusinessID(ctx context.Context, businessID, tenantID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, d := range m.doc {
		if d.BusinessID == businessID && (tenantID == "" || d.TenantID == tenantID) && d.Deleted == 0 {
			d.Deleted = 1
		}
	}
	return nil
}

// applyDocumentChanges 将列名→值映射应用到单据实体（内存实现的变更应用器）。
// @param d 单据实体
// @param changes 变更列（nil 值置零值/NULL，类型不匹配的列忽略）
func applyDocumentChanges(d *Document, changes map[string]any) {
	for column, value := range changes {
		// 按列名逐一映射；nil 统一处理为清空语义
		switch column {
		case "flow_status":
			applyString(&d.FlowStatus, value)
		case "document_url":
			applyString(&d.DocumentURL, value)
		case "flow_task_id":
			applyString(&d.FlowTaskID, value)
		case "flow_action_id":
			applyString(&d.FlowActionID, value)
		case "flow_action":
			applyString(&d.FlowAction, value)
		case "flow_current_step_name":
			applyString(&d.FlowCurrentStepName, value)
		case "flow_current_persons":
			applyString(&d.FlowCurrentPersons, value)
		case "flow_start_time":
			d.FlowStartTime = asTimePtr(value)
		case "flow_finish_time":
			d.FlowFinishTime = asTimePtr(value)
		case "flow_termination_time":
			d.FlowTerminationTime = asTimePtr(value)
		case "updater_name":
			applyString(&d.UpdaterName, value)
		case "flow_title":
			applyString(&d.FlowTitle, value)
		case "flow_category_id":
			applyString(&d.FlowCategoryID, value)
		case "flow_category":
			applyString(&d.FlowCategory, value)
		}
	}
}

// applyString 将变更值应用到字符串列：nil 置空、字符串生效、其他类型忽略。
// @param target 目标字段指针
// @param value 变更值
func applyString(target *string, value any) {
	// nil 显式清空（SQL NULL 语义）
	if value == nil {
		*target = ""
		return
	}
	// 类型不匹配时忽略，防止误清空
	if s, ok := value.(string); ok {
		*target = s
	}
}

// asTimePtr 将变更值转为时间指针（nil → nil，即置 NULL）。
// @param value 原始值
// @return 时间指针
func asTimePtr(value any) *time.Time {
	if value == nil {
		return nil
	}
	if t, ok := value.(time.Time); ok {
		return &t
	}
	if t, ok := value.(*time.Time); ok {
		return cloneTime(t)
	}
	return nil
}
