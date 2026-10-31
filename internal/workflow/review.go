package workflow

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
)

// 审阅域错误码（30xx 段为本文件专用，避免与既有 04xx-29xx 段冲突）。
const (
	errCodeReviewParam   = "WORKFLOW-3001" // 审阅同步/办结参数缺失或非法
	errCodeReviewMissing = "WORKFLOW-3002" // 审阅影子任务不存在
	errCodeReviewDef     = "WORKFLOW-3003" // 流程定义不存在
	errCodeReviewInst    = "WORKFLOW-3004" // 流程实例不存在（运行态与历史双查失败）
)

// 审阅人绑定的 owner_type 常量（003 文档 §workflow_act_extend：person/org/role）。
const (
	// ActOwnerTypePerson 人员绑定：owner_value 即审阅人身份 Id，直接使用。
	ActOwnerTypePerson = "person"
	// ActOwnerTypeOrg 组织绑定：owner_value 为组织/组 Id，经身份服务展开成员。
	ActOwnerTypeOrg = "org"
	// ActOwnerTypeRole 角色绑定：owner_value 为角色 Id，经身份服务展开成员。
	ActOwnerTypeRole = "role"
)

// 审阅影子任务状态常量（status 列；0/1 对齐 Java 线上语义，2 为 Go 侧扩展）。
const (
	// ReviewStatusPending 待审阅：影子任务已生成，审阅人尚未办结。
	ReviewStatusPending = 0
	// ReviewStatusDone 已办结：审阅人已完成审阅并回写意见。
	ReviewStatusDone = 1
	// ReviewStatusUnresolved 待解析：org/role 展开失败，reviewer 保留原组/角色值，需业务系统二次解析。
	ReviewStatusUnresolved = 2
)

// ActExtend 是 workflow_act_extend（活动扩展，审阅人绑定）的领域实体：
// 按（流程定义 Key + BPMN 节点 ID）配置审阅人，运行期任务到达节点时据此生成审阅影子任务。
type ActExtend struct {
	BaseColumns                 // 公共审计列
	ActivityID           string // BPMN 节点 ID（userTask id）
	ActivityName         string // 节点名称（设计器展示名）
	ProcessDefinitionKey string // 流程定义 Key
	OwnerType            string // 审阅人类型：person/org/role（ActOwnerType* 常量）
	OwnerValue           string // 审阅人值：身份 Id / 组织 Id / 角色 Id
	StepID               string // 关联设计步骤 Id（→workflow_step.id，可空）
}

// ReviewTask 是 workflow_review_task（审阅/知会影子任务）的领域实体。
// 影子任务与引擎任务 1:N：同一任务节点按审阅人绑定展开出 N 条审阅记录，
// 幂等键为（task_id + reviewer）。
type ReviewTask struct {
	BaseColumns                      // 公共审计列
	StartTime             time.Time  // 影子任务生成时间（start_time）
	TaskID                string     // 关联引擎任务 Id（ACT_RU_TASK.ID_）
	TaskName              string     // 任务名称
	ProcessInstanceID     string     // 流程实例 Id
	ProcessDefinitionID   string     // 流程定义 Id
	TaskKey               string     // 任务节点 ID（BPMN userTask id）
	BusinessKey           string     // 业务键（=实例 businessKey）
	ProcessName           string     // 实例名称（列 process_name 与 process_definition_name 区分）
	ProcessDefinitionKey  string     // 流程定义 Key
	ProcessDefinitionName string     // 流程定义名称
	SenderID              string     // 发送人身份 Id（sender 列，对齐 creator/creator_name 的"值/名"约定）
	SenderName            string     // 发送人姓名（sender_name 列）
	Reviewer              string     // 审阅人身份 Id（org/role 未解析时保留原组/角色值）
	ReviewTime            *time.Time // 审阅时间（办结回写）
	Remark                string     // 备注
	Opinion               string     // 审阅意见（办结回写）
	Status                int        // 状态：0 待审阅、1 已办结、2 待解析（ReviewStatus* 常量）
}

// ─── 存储端口 ─────────────────────────────────────────────
// 端口定义在本包（消费者侧）；内存实现见本文件 MemReviewTaskStore / MemActExtendStore，
// SQL 适配器见 review_sql.go（组合 internal/persistence/workflow 仓储）。

// ReviewTaskStore 是审阅影子任务的存储端口。
type ReviewTaskStore interface {
	// Insert 插入一条审阅影子任务（id 由调用方生成）。
	Insert(ctx context.Context, rt *ReviewTask) error
	// Get 按主键查询审阅影子任务。
	Get(ctx context.Context, id string) (*ReviewTask, bool, error)
	// Update 办结回写：按主键回写可变列（status/opinion/review_time/remark/updater/update_time）。
	Update(ctx context.Context, rt *ReviewTask) error
	// ListByReviewer 按审阅人分页查询（租户过滤，空租户不过滤），按 start_time 升序，返回当前页与总数。
	ListByReviewer(ctx context.Context, reviewer, tenantID string, page PageQuery) ([]*ReviewTask, int64, error)
	// ListByInstance 查询实例下全部审阅影子任务（按创建时间升序）。
	ListByInstance(ctx context.Context, processInstanceID string) ([]*ReviewTask, error)
	// DeleteByInstance 删除实例下全部审阅影子任务（实例级联清理）。
	DeleteByInstance(ctx context.Context, processInstanceID string) error
	// CountUnread 统计审阅人的待审阅数（status=待审阅，租户过滤）。
	CountUnread(ctx context.Context, reviewer, tenantID string) (int64, error)
}

// ActExtendStore 是审阅人绑定（活动扩展）的存储端口。
type ActExtendStore interface {
	// GetByDefinitionKeyAndActivity 按流程定义 Key 与 BPMN 节点 ID 查询绑定（运行期审阅人读取入口）。
	GetByDefinitionKeyAndActivity(ctx context.Context, definitionKey, activityID string) ([]*ActExtend, error)
	// ListByDefinitionKey 查询流程定义 Key 下全部绑定（设计器配置读取入口）。
	ListByDefinitionKey(ctx context.Context, definitionKey string) ([]*ActExtend, error)
	// Save 保存一条绑定（id 由调用方生成）。
	Save(ctx context.Context, ext *ActExtend) error
	// Delete 按主键删除绑定。
	Delete(ctx context.Context, id string) error
}

// ─── MemActExtendStore：审阅人绑定内存实现 ─────────────────────────────────────────────

// MemActExtendStore 是 ActExtendStore 的内存实现（线程安全 map + 时钟注入 + 克隆存取）。
type MemActExtendStore struct {
	mu  sync.RWMutex          // 保护 ext map
	ext map[string]*ActExtend // 绑定（id → 行）
}

// NewMemActExtendStore 创建审阅人绑定内存存储。
// @return 内存存储
func NewMemActExtendStore() *MemActExtendStore {
	return &MemActExtendStore{ext: make(map[string]*ActExtend)}
}

// 编译期断言。
var _ ActExtendStore = (*MemActExtendStore)(nil)

// Save 保存一条审阅人绑定。
// @param ctx 操作上下文
// @param ext 绑定（必填 id）
// @return 错误
func (m *MemActExtendStore) Save(ctx context.Context, ext *ActExtend) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ext[ext.ID] = rvCloneActExtend(ext)
	return nil
}

// GetByDefinitionKeyAndActivity 按流程定义 Key 与 BPMN 节点 ID 查询绑定（按创建时间升序）。
// @param ctx 操作上下文
// @param definitionKey 流程定义 Key
// @param activityID BPMN 节点 ID
// @return 绑定列表
// @return 错误
func (m *MemActExtendStore) GetByDefinitionKeyAndActivity(ctx context.Context, definitionKey, activityID string) ([]*ActExtend, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rows := make([]*ActExtend, 0, 2)
	for _, ext := range m.ext {
		if ext.ProcessDefinitionKey == definitionKey && ext.ActivityID == activityID {
			rows = append(rows, rvCloneActExtend(ext))
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].CreateTime.Before(rows[j].CreateTime)
	})
	return rows, nil
}

// ListByDefinitionKey 查询流程定义 Key 下全部绑定（按创建时间升序）。
// @param ctx 操作上下文
// @param definitionKey 流程定义 Key
// @return 绑定列表
// @return 错误
func (m *MemActExtendStore) ListByDefinitionKey(ctx context.Context, definitionKey string) ([]*ActExtend, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rows := make([]*ActExtend, 0, 4)
	for _, ext := range m.ext {
		if ext.ProcessDefinitionKey == definitionKey {
			rows = append(rows, rvCloneActExtend(ext))
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].CreateTime.Before(rows[j].CreateTime)
	})
	return rows, nil
}

// Delete 按主键删除绑定。
// @param ctx 操作上下文
// @param id 绑定 Id
// @return 错误
func (m *MemActExtendStore) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.ext, id)
	return nil
}

// ─── MemReviewTaskStore：审阅影子任务内存实现 ─────────────────────────────────────────────

// MemReviewTaskStore 是 ReviewTaskStore 的内存实现（线程安全 map + 时钟注入 + 租户隔离 + 克隆存取）。
type MemReviewTaskStore struct {
	mu    sync.RWMutex           // 保护 task map
	clock clock.Clock            // 时间源（预留排序/回写场景）
	task  map[string]*ReviewTask // 影子任务（id → 行）
}

// NewMemReviewTaskStore 创建审阅影子任务内存存储。
// @param c 时间源，nil 时回退真实时钟
// @return 内存存储
func NewMemReviewTaskStore(c clock.Clock) *MemReviewTaskStore {
	if c == nil {
		c = clock.Real{}
	}
	return &MemReviewTaskStore{clock: c, task: make(map[string]*ReviewTask)}
}

// 编译期断言。
var _ ReviewTaskStore = (*MemReviewTaskStore)(nil)

// Now 返回审阅影子任务存储时间源当前时间。
// @return 当前时间
func (m *MemReviewTaskStore) Now() time.Time { return m.clock.Now() }

// Insert 插入一条审阅影子任务。
// @param ctx 操作上下文
// @param rt 影子任务（必填 id）
// @return 错误
func (m *MemReviewTaskStore) Insert(ctx context.Context, rt *ReviewTask) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.task[rt.ID] = rvCloneReview(rt)
	return nil
}

// Get 按主键查询审阅影子任务。
// @param ctx 操作上下文
// @param id 影子任务 Id
// @return 影子任务
// @return 是否存在
// @return 错误
func (m *MemReviewTaskStore) Get(ctx context.Context, id string) (*ReviewTask, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rt, ok := m.task[id]
	if !ok {
		return nil, false, nil
	}
	return rvCloneReview(rt), true, nil
}

// Update 办结回写：按主键回写可变列（status/opinion/review_time/remark/updater/update_time）。
// 主键不存在时静默忽略（与 SQL UpdateByPK 影响 0 行语义一致）。
// @param ctx 操作上下文
// @param rt 影子任务（必填 id，可变列取实体现值）
// @return 错误
func (m *MemReviewTaskStore) Update(ctx context.Context, rt *ReviewTask) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.task[rt.ID]
	if !ok {
		return nil
	}
	existing.Status = rt.Status
	existing.Opinion = rt.Opinion
	existing.Remark = rt.Remark
	existing.ReviewTime = cloneTime(rt.ReviewTime)
	if rt.Updater != "" {
		existing.Updater = rt.Updater
	}
	// 更新时间为零值时取存储时钟，保证回写留痕
	if rt.UpdateTime.IsZero() {
		existing.UpdateTime = m.clock.Now()
	} else {
		existing.UpdateTime = rt.UpdateTime
	}
	return nil
}

// ListByReviewer 按审阅人分页查询（租户过滤，空租户不过滤；按 start_time 升序）。
// @param ctx 操作上下文
// @param reviewer 审阅人 Id
// @param tenantID 租户
// @param page 分页参数
// @return 当前页列表
// @return 总数
// @return 错误
func (m *MemReviewTaskStore) ListByReviewer(ctx context.Context, reviewer, tenantID string, page PageQuery) ([]*ReviewTask, int64, error) {
	m.mu.RLock()
	rows := m.listLocked(func(rt *ReviewTask) bool {
		return rt.Reviewer == reviewer && (tenantID == "" || rt.TenantID == tenantID)
	})
	m.mu.RUnlock()
	data, total := paginate(rows, page.Start, page.Size)
	return data, total, nil
}

// ListByInstance 查询实例下全部审阅影子任务（按创建时间升序）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 列表
// @return 错误
func (m *MemReviewTaskStore) ListByInstance(ctx context.Context, processInstanceID string) ([]*ReviewTask, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.listLocked(func(rt *ReviewTask) bool {
		return rt.ProcessInstanceID == processInstanceID
	}), nil
}

// DeleteByInstance 删除实例下全部审阅影子任务。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 错误
func (m *MemReviewTaskStore) DeleteByInstance(ctx context.Context, processInstanceID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, rt := range m.task {
		if rt.ProcessInstanceID == processInstanceID {
			delete(m.task, id)
		}
	}
	return nil
}

// CountUnread 统计审阅人的待审阅数（status=待审阅，租户过滤，空租户不过滤）。
// @param ctx 操作上下文
// @param reviewer 审阅人 Id
// @param tenantID 租户
// @return 待审阅数
// @return 错误
func (m *MemReviewTaskStore) CountUnread(ctx context.Context, reviewer, tenantID string) (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	count := int64(0)
	for _, rt := range m.task {
		// 未读 = 待审阅状态；待解析（status=2）不算未读，由业务系统二次解析后重新同步
		if rt.Reviewer == reviewer && rt.Status == ReviewStatusPending && (tenantID == "" || rt.TenantID == tenantID) {
			count++
		}
	}
	return count, nil
}

// listLocked 按谓词过滤影子任务并按 start_time/创建时间升序（调用方须持锁）。
// @param match 谓词
// @return 列表
func (m *MemReviewTaskStore) listLocked(match func(*ReviewTask) bool) []*ReviewTask {
	rows := make([]*ReviewTask, 0, 4)
	for _, rt := range m.task {
		if match(rt) {
			rows = append(rows, rvCloneReview(rt))
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		// 与 SQL 适配器 ORDER BY start_time 对齐；同 start_time 再按创建时间稳定排序
		if !rows[i].StartTime.Equal(rows[j].StartTime) {
			return rows[i].StartTime.Before(rows[j].StartTime)
		}
		return rows[i].CreateTime.Before(rows[j].CreateTime)
	})
	return rows
}

// ─── 克隆工具（rv 前缀，避免与并发开发文件撞名）─────────────────────────────────────────────

// rvCloneReview 复制审阅影子任务（含指针字段深拷贝）。
// @param rt 原记录
// @return 副本
func rvCloneReview(rt *ReviewTask) *ReviewTask {
	if rt == nil {
		return nil
	}
	copied := *rt
	copied.ReviewTime = cloneTime(rt.ReviewTime)
	return &copied
}

// rvCloneActExtend 复制审阅人绑定。
// @param ext 原绑定
// @return 副本
func rvCloneActExtend(ext *ActExtend) *ActExtend {
	if ext == nil {
		return nil
	}
	copied := *ext
	return &copied
}

// ─── ReviewTaskService：审阅影子任务域服务 ─────────────────────────────────────────────

// ReviewTaskService 是审阅/知会影子任务域服务（P5-B，对位 Java workflow_review_task 同步与办结语义）：
// 任务到达绑定节点时按 workflow_act_extend 审阅人配置生成影子任务（幂等），
// 审阅人办结时回写意见与状态；org/role 绑定经身份服务展开成员。
type ReviewTaskService struct {
	engine  api.Engine      // 引擎门面（定义/实例/身份查询）
	tasks   ReviewTaskStore // 影子任务存储
	extends ActExtendStore  // 审阅人绑定存储
	clock   clock.Clock     // 时间源
	newID   func() string   // ID 生成函数
}

// NewReviewTaskService 创建审阅影子任务域服务。
// @param engine 引擎门面（七服务，org/role 展开与实例信息查询依赖；nil 时仅 person 绑定同步不可用）
// @param tasks 影子任务存储端口
// @param extends 审阅人绑定存储端口
// @param c 时间源，nil 时回退真实时钟
// @return 域服务
func NewReviewTaskService(engine api.Engine, tasks ReviewTaskStore, extends ActExtendStore, c clock.Clock) *ReviewTaskService {
	if c == nil {
		c = clock.Real{}
	}
	return &ReviewTaskService{engine: engine, tasks: tasks, extends: extends, clock: c, newID: DefaultIDGen()}
}

// SyncForTask 为任务节点同步生成审阅影子任务（幂等）。
// 流程：定义解析 → 读取 act_extend 审阅人绑定 → 逐绑定解析审阅人（person 直用；
// org/role 经 IdentityService.ListMembers 展开，展开失败保留原值并置待解析）→
// 按（task_id + reviewer）幂等插入。
// @param ctx 操作上下文
// @param task 引擎任务（必填 id/process_instance_id/task_definition_key）
// @param senderID 发送人身份 Id
// @param senderName 发送人姓名
// @return 本次新生成的影子任务列表（节点无绑定为空列表）
// @return 错误
func (s *ReviewTaskService) SyncForTask(ctx context.Context, task *api.Task, senderID, senderName string) ([]*ReviewTask, error) {
	if s.engine == nil || task == nil || task.ID == "" || task.ProcessInstanceID == "" || task.TaskDefinitionKey == "" {
		return nil, apiError(errCodeReviewParam, "审阅同步缺少任务/实例/节点标识或引擎未注入")
	}
	definition, err := s.engine.RepositoryService().GetProcessDefinition(ctx, task.ProcessDefinitionID)
	if err != nil {
		return nil, api.NewError(errCodeReviewDef, "流程定义不存在", err)
	}
	bindings, err := s.extends.GetByDefinitionKeyAndActivity(ctx, definition.Key, task.TaskDefinitionKey)
	if err != nil {
		return nil, err
	}
	// 节点未配置审阅人绑定：直接返回空列表（非错误）
	if len(bindings) == 0 {
		return []*ReviewTask{}, nil
	}
	instance, err := s.loadInstance(ctx, task.ProcessInstanceID)
	if err != nil {
		return nil, err
	}
	existing, err := s.tasks.ListByInstance(ctx, task.ProcessInstanceID)
	if err != nil {
		return nil, err
	}
	// 幂等键：任务 Id + 审阅人（同节点多绑定展开出的重复审阅人也一并去重）
	seen := make(map[string]bool, len(existing)+len(bindings))
	for _, row := range existing {
		seen[row.TaskID+"\x00"+row.Reviewer] = true
	}
	now := s.clock.Now()
	created := make([]*ReviewTask, 0, len(bindings))
	for _, binding := range bindings {
		reviewers, unresolved := s.resolveReviewers(ctx, binding)
		for _, reviewer := range reviewers {
			key := task.ID + "\x00" + reviewer
			// 幂等：同任务 + 审阅人已有影子任务时跳过，不重复插入
			if seen[key] {
				continue
			}
			seen[key] = true
			entity := s.newReviewTask(task, definition, instance, reviewer, senderID, senderName, unresolved, now)
			if err := s.tasks.Insert(ctx, entity); err != nil {
				return nil, err
			}
			created = append(created, entity)
		}
	}
	return created, nil
}

// Complete 办结审阅：回写审阅时间/意见/状态（status=已办结）。
// @param ctx 操作上下文
// @param reviewTaskID 审阅影子任务 Id
// @param opinion 审阅意见
// @param operator 操作人身份 Id（回写 updater，可空）
// @return 办结后的影子任务
// @return 错误
func (s *ReviewTaskService) Complete(ctx context.Context, reviewTaskID, opinion, operator string) (*ReviewTask, error) {
	if reviewTaskID == "" {
		return nil, apiError(errCodeReviewParam, "审阅办结缺少影子任务 Id")
	}
	existing, found, err := s.tasks.Get(ctx, reviewTaskID)
	if err != nil {
		return nil, err
	}
	// 不存在时报错而非静默成功，避免调用方误以为已办结
	if !found {
		return nil, api.NewError(errCodeReviewMissing, "审阅影子任务不存在: "+reviewTaskID, nil)
	}
	now := s.clock.Now()
	existing.Status = ReviewStatusDone
	existing.Opinion = opinion
	existing.ReviewTime = &now
	if operator != "" {
		existing.Updater = operator
	}
	existing.UpdateTime = now
	if err := s.tasks.Update(ctx, existing); err != nil {
		return nil, err
	}
	return existing, nil
}

// Get 按主键查询审阅影子任务。
// @param ctx 操作上下文
// @param reviewTaskID 影子任务 Id
// @return 影子任务
// @return 是否存在
// @return 错误
func (s *ReviewTaskService) Get(ctx context.Context, reviewTaskID string) (*ReviewTask, bool, error) {
	return s.tasks.Get(ctx, reviewTaskID)
}

// ListByReviewer 按审阅人分页查询影子任务（租户过滤，审阅人待办/已办列表主查询）。
// @param ctx 操作上下文
// @param reviewer 审阅人 Id
// @param tenantID 租户
// @param page 分页参数
// @return 分页结果
// @return 错误
func (s *ReviewTaskService) ListByReviewer(ctx context.Context, reviewer, tenantID string, page PageQuery) (PageResult[*ReviewTask], error) {
	data, total, err := s.tasks.ListByReviewer(ctx, reviewer, tenantID, page)
	if err != nil {
		return PageResult[*ReviewTask]{}, err
	}
	return PageResult[*ReviewTask]{Data: data, Total: total, Start: page.Start, Size: page.Size}, nil
}

// ListByInstance 查询实例下全部审阅影子任务（按生成时间升序）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 列表
// @return 错误
func (s *ReviewTaskService) ListByInstance(ctx context.Context, processInstanceID string) ([]*ReviewTask, error) {
	return s.tasks.ListByInstance(ctx, processInstanceID)
}

// CountUnreadByReviewer 统计审阅人的待审阅数（status=待审阅）。
// @param ctx 操作上下文
// @param reviewer 审阅人 Id
// @param tenantID 租户
// @return 待审阅数
// @return 错误
func (s *ReviewTaskService) CountUnreadByReviewer(ctx context.Context, reviewer, tenantID string) (int64, error) {
	return s.tasks.CountUnread(ctx, reviewer, tenantID)
}

// resolveReviewers 解析审阅人绑定对应的审阅人列表。
// person 类型直用 owner_value；org/role 经身份服务展开组成员，
// 身份服务报错或展开零成员视为展开失败，保留原值并置待解析（status=待解析）。
// @param ctx 操作上下文
// @param binding 审阅人绑定
// @return 审阅人列表（展开后逐成员，保序）
// @return 是否展开失败（true 时 reviewer 保留原值，状态置待解析）
func (s *ReviewTaskService) resolveReviewers(ctx context.Context, binding *ActExtend) ([]string, bool) {
	// 空类型按 person 兼容（历史数据未填 owner_type 时直用原值）
	if binding.OwnerType == ActOwnerTypePerson || binding.OwnerType == "" {
		return []string{binding.OwnerValue}, false
	}
	// 非 person/org/role 的未知类型按展开失败处理，保留原值
	if s.engine == nil || (binding.OwnerType != ActOwnerTypeOrg && binding.OwnerType != ActOwnerTypeRole) {
		return []string{binding.OwnerValue}, true
	}
	members, err := s.engine.IdentityService().ListMembers(ctx, binding.OwnerValue)
	// 展开失败（查询报错或空组/空角色）保留原值，待业务系统二次解析
	if err != nil || len(members) == 0 {
		return []string{binding.OwnerValue}, true
	}
	reviewers := make([]string, 0, len(members))
	for _, member := range members {
		if member.ID != "" {
			reviewers = append(reviewers, member.ID)
		}
	}
	// 成员列表全部无 Id 时同样视为展开失败
	if len(reviewers) == 0 {
		return []string{binding.OwnerValue}, true
	}
	return reviewers, false
}

// loadInstance 查询流程实例（运行态优先，实例已结束回退历史）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 流程实例
// @return 错误
func (s *ReviewTaskService) loadInstance(ctx context.Context, processInstanceID string) (*api.ProcessInstance, error) {
	instance, err := s.engine.RuntimeService().GetProcessInstance(ctx, processInstanceID)
	if err == nil {
		return &instance, nil
	}
	historic, hisErr := s.engine.HistoryService().GetHistoricProcessInstance(ctx, processInstanceID)
	if hisErr != nil {
		return nil, api.NewError(errCodeReviewInst, "流程实例不存在（运行态与历史双查失败）", err)
	}
	return &api.ProcessInstance{
		ID:                   historic.ID,
		ProcessDefinitionID:  historic.ProcessDefinitionID,
		ProcessDefinitionKey: historic.ProcessDefinitionKey,
		BusinessKey:          historic.BusinessKey,
		Name:                 historic.Name,
		TenantID:             historic.TenantID,
		StartUserID:          historic.StartUserID,
	}, nil
}

// newReviewTask 构造一条审阅影子任务实体（公共审计列由 fillBase 填充）。
// @param task 引擎任务
// @param definition 流程定义
// @param instance 流程实例
// @param reviewer 审阅人 Id（未解析时为原组/角色值）
// @param senderID 发送人身份 Id
// @param senderName 发送人姓名
// @param unresolved 是否展开失败（true 时状态置待解析）
// @param now 影子任务生成时间
// @return 影子任务实体
func (s *ReviewTaskService) newReviewTask(task *api.Task, definition api.ProcessDefinition, instance *api.ProcessInstance, reviewer, senderID, senderName string, unresolved bool, now time.Time) *ReviewTask {
	entity := &ReviewTask{
		StartTime:             now,
		TaskID:                task.ID,
		TaskName:              task.Name,
		ProcessInstanceID:     task.ProcessInstanceID,
		ProcessDefinitionID:   task.ProcessDefinitionID,
		TaskKey:               task.TaskDefinitionKey,
		BusinessKey:           instance.BusinessKey,
		ProcessName:           instance.Name,
		ProcessDefinitionKey:  definition.Key,
		ProcessDefinitionName: definition.Name,
		SenderID:              senderID,
		SenderName:            senderName,
		Reviewer:              reviewer,
		Status:                ReviewStatusPending,
	}
	// 展开失败状态：保留原组/角色值并置待解析，待业务系统解析后重新同步
	if unresolved {
		entity.Status = ReviewStatusUnresolved
	}
	fillBase(&entity.BaseColumns, s.newID, s.clock.Now, senderID, task.TenantID)
	return entity
}
