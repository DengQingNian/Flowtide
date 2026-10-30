// delegate.go 提供规则型委托（代理人）域服务（P4.1-P4.2，对位 Java WorkflowTaskDelegateServiceImpl
// 与 ProcessesUtil#doTaskOwnerAdjustment:5856）：
// 三表模型（主表 workflow_task_delegate + 被委托人 workflow_task_consigners + 委托对象 workflow_task_consign_obj）、
// 时间重叠三段式校验、禁止委托传递、命中优先级（单项目 > 单流程分类 > 全部）、
// 待办生成改派钩子与待办调整（多 owner 复制 next_activity + 委托镜像 + WP 操作记录）。
// 注意：按任务委派/转办/归还（delegateTask/transferTask/resolveTask）已在 complete.go 实现，本文件不重复。
package workflow

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
	pwf "github.com/DengQingNian/Flowtide/internal/persistence/workflow"
	"github.com/DengQingNian/Flowtide/internal/tenant"
)

// 委托范围类型常量（对位 Java WorkflowTaskDelegateEntity.CONSIGNTYPE_*）。
const (
	// ConsignTypeAll 全部委托（不限定项目/流程分类）。
	ConsignTypeAll = "All"
	// ConsignTypeSingleProject 单项目委托。
	ConsignTypeSingleProject = "SingleProject"
	// ConsignTypeMultiProject 多项目委托。
	ConsignTypeMultiProject = "MultiProject"
	// ConsignTypeAllProject 全部项目委托（协议保留值，命中不参与）。
	ConsignTypeAllProject = "AllProject"
	// ConsignTypeSingleFlowCategory 单流程分类委托。
	ConsignTypeSingleFlowCategory = "SingleFlowCategory"
	// ConsignTypeMultiFlowCategory 多流程分类委托。
	ConsignTypeMultiFlowCategory = "MultiFlowCategory"
	// ConsignTypeAllFlowCategory 全部流程分类委托（协议保留值，命中不参与）。
	ConsignTypeAllFlowCategory = "AllFlowCategory"
)

// 委托条件串前缀（对位 Java CONDITION_PROJECT_PREFIX / CONDITION_FLOWCATEGORY_PREFIX）。
const (
	// ConditionProjectPrefix 项目条件前缀（condition 列取值如 "@ProjId='p1,p2'"）。
	ConditionProjectPrefix = "@ProjId"
	// ConditionFlowCategoryPrefix 流程分类条件前缀（condition 列取值如 "@FlowCategory='m1'"）。
	ConditionFlowCategoryPrefix = "@FlowCategory"
)

// 委托规则启用/退休标记取值。
const (
	// DelegateEnabled 启用。
	DelegateEnabled = 1
	// DelegateDisabled 停用。
	DelegateDisabled = 0
	// DelegateRetired 退休（ retirement=1 时同时停用，Java Retire 语义）。
	DelegateRetired = 1
)

// DelegateConsigner 是 workflow_task_consigners（被委托人）的领域实体。
type DelegateConsigner struct {
	ID            string // 行主键（空时由存储生成）
	DelegateID    string // 委托规则 Id（主表外键）
	ConsignerID   string // 被委托人身份 Id
	ConsignerName string // 被委托人姓名
}

// DelegateConsignObj 是 workflow_task_consign_obj（委托对象）的领域实体。
type DelegateConsignObj struct {
	ID        string // 行主键（空时由存储生成）
	DelegateID string // 委托规则 Id（主表外键）
	OwnerType string // 对象类型（=主表委托范围：SingleProject/SingleFlowCategory…）
	OwnerID   string // 对象 Id（项目 Id 或 workflow_model.id）
	OwnerName string // 对象名称
}

// DelegateRule 是 workflow_task_delegate（委托规则主表）的领域实体，
// 级联携带被委托人与委托对象（三表模型）。
type DelegateRule struct {
	BaseColumns          // 公共审计列
	ConsignType string   // 委托范围（ConsignType* 常量）
	OwnerID     string   // 委托人身份 Id
	OwnerName   string   // 委托人姓名
	Source      string   // 来源系统 Id
	SourceCode  string   // 来源系统代号（= 租户，必填）
	SourceName  string   // 来源系统名称
	Type        string   // 类型（协议保留列）
	Condition   string   // 条件串（保留字列，"@ProjId='...'" / "@FlowCategory='...'"）
	VaildFrom   time.Time // 生效时间（保留 Java 拼写）
	VaildTo     time.Time // 失效时间（保留 Java 拼写）
	Enable      int      // 启用：0 停用、1 启用
	Retire      int      // 退休：0 正常、1 退休
	Remark      string   // 备注
	Consigners  []DelegateConsigner  // 被委托人（级联）
	ConsignObjs []DelegateConsignObj // 委托对象（级联）
}

// DelegateTaskMirror 是 workflow_delegate_task（委托镜像待办）的领域实体：
// 待办整行复制 + 被委托人信息（docs 003 §3.8）。
type DelegateTaskMirror struct {
	MessageExtend           // 与待办扩展同构（整行复制）
	EntrustID    string     // 被委托人 Id（多值逗号串）
	EntrustName  string     // 被委托人姓名（多值逗号串）
}

// ─── 存储端口 ─────────────────────────────────────────────

// TaskDelegateStore 是委托规则三表的存储端口（主表 + consigners + consignObj 级联）。
type TaskDelegateStore interface {
	// Save 级联保存委托规则：主表 upsert，被委托人/委托对象先删后增（Java updateEntity 同款）。
	// @param ctx 操作上下文
	// @param rule 委托规则（必填 SourceCode/OwnerID）
	// @return 错误
	Save(ctx context.Context, rule *DelegateRule) error
	// Get 按主键级联读取委托规则。
	// @param ctx 操作上下文
	// @param id 委托规则 Id
	// @return 委托规则
	// @return 是否存在
	// @return 错误
	Get(ctx context.Context, id string) (*DelegateRule, bool, error)
	// ListByOwner 查询委托人在指定租户下的全部规则（含停用，供时间重叠校验）。
	// @param ctx 操作上下文
	// @param ownerID 委托人 Id
	// @param tenantID 租户（=source_code）
	// @return 规则列表
	// @return 错误
	ListByOwner(ctx context.Context, ownerID, tenantID string) ([]*DelegateRule, error)
	// ListByConsigner 查询指定身份作为被委托人的启用规则（禁止传递校验的跨 owner 反查）。
	// @param ctx 操作上下文
	// @param consignerID 被委托人 Id
	// @param tenantID 租户（=source_code）
	// @return 启用中的规则列表
	// @return 错误
	ListByConsigner(ctx context.Context, consignerID, tenantID string) ([]*DelegateRule, error)
	// UpdateState 更新规则启用/退休标记（Enable/Disable/Retire 共用）。
	// @param ctx 操作上下文
	// @param id 委托规则 Id
	// @param enable 启用标记
	// @param retire 退休标记
	// @return 错误
	UpdateState(ctx context.Context, id string, enable, retire int) error
	// Delete 按主键级联删除委托规则（含被委托人与委托对象，Java deleteBatch 语义）。
	// @param ctx 操作上下文
	// @param id 委托规则 Id
	// @return 错误
	Delete(ctx context.Context, id string) error
}

// DelegateTaskMirrorStore 是委托镜像待办的存储端口（workflow_delegate_task）。
type DelegateTaskMirrorStore interface {
	// Save 插入一条委托镜像待办。
	// @param ctx 操作上下文
	// @param mirror 镜像待办（必填 TaskID/EntrustID）
	// @return 错误
	Save(ctx context.Context, mirror *DelegateTaskMirror) error
	// ListByTaskID 查询任务的镜像待办列表。
	// @param ctx 操作上下文
	// @param taskID 任务 Id
	// @return 镜像列表
	// @return 错误
	ListByTaskID(ctx context.Context, taskID string) ([]*DelegateTaskMirror, error)
	// ListByEntrust 查询被委托人的镜像待办列表。
	// @param ctx 操作上下文
	// @param entrustID 被委托人 Id
	// @return 镜像列表
	// @return 错误
	ListByEntrust(ctx context.Context, entrustID string) ([]*DelegateTaskMirror, error)
	// DeleteByProcessInstance 删除实例下全部镜像待办（实例级联）。
	// @param ctx 操作上下文
	// @param processInstanceID 流程实例 Id
	// @return 错误
	DeleteByProcessInstance(ctx context.Context, processInstanceID string) error
}

// CategoryLookup 是流程分类代号解析端口（单分类委托命中时 model_code → workflow_model.id；
// nil 时按对象 Id 与代号直接相等匹配，适配嵌入模式 id==code 的部署）。
type CategoryLookup interface {
	// ResolveModelID 解析流程分类代号对应的模型 Id。
	// @param ctx 操作上下文
	// @param tenantID 租户
	// @param flowCategoryCode 分类代号（=定义 Key）
	// @return 模型 Id，未解析到返回空串
	// @return 错误
	ResolveModelID(ctx context.Context, tenantID, flowCategoryCode string) (string, error)
}

// ─── 内存实现 ─────────────────────────────────────────────

// MemTaskDelegateStore 是委托规则端口的内存实现（线程安全 map + 行克隆存取）。
type MemTaskDelegateStore struct {
	mu    sync.RWMutex                // 读写锁
	rules map[string]*DelegateRule    // id → 规则
	clock clock.Clock                 // 时间源（审计列填充）
	newID func() string               // ID 生成函数
}

// NewMemTaskDelegateStore 创建内存委托规则存储。
// @param c 时间源，nil 时回退真实时钟
// @return 内存存储
func NewMemTaskDelegateStore(c clock.Clock) *MemTaskDelegateStore {
	if c == nil {
		c = clock.Real{}
	}
	return &MemTaskDelegateStore{rules: make(map[string]*DelegateRule), clock: c, newID: DefaultIDGen()}
}

// Save 级联保存委托规则（主表 upsert，子表先删后增，ID 缺失自动生成）。
// @param ctx 操作上下文
// @param rule 委托规则
// @return 错误
func (m *MemTaskDelegateStore) Save(ctx context.Context, rule *DelegateRule) error {
	// 主表定位键缺失即拒绝（委托人 + 来源代号是校验与命中的最小键）
	if rule == nil || rule.OwnerID == "" || rule.SourceCode == "" {
		return apiError("WORKFLOW-1701", "委托规则缺少委托人/来源系统代号")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	fillBase(&rule.BaseColumns, m.newID, m.clock.Now, "", rule.TenantID)
	// 子表 Id 缺失自动生成并回填外键
	for i := range rule.Consigners {
		if rule.Consigners[i].ID == "" {
			rule.Consigners[i].ID = m.newID()
		}
		rule.Consigners[i].DelegateID = rule.ID
	}
	for i := range rule.ConsignObjs {
		if rule.ConsignObjs[i].ID == "" {
			rule.ConsignObjs[i].ID = m.newID()
		}
		rule.ConsignObjs[i].DelegateID = rule.ID
	}
	m.rules[rule.ID] = cloneDelegateRule(rule)
	return nil
}

// Get 按主键级联读取委托规则。
// @param ctx 操作上下文
// @param id 委托规则 Id
// @return 委托规则
// @return 是否存在
// @return 错误
func (m *MemTaskDelegateStore) Get(ctx context.Context, id string) (*DelegateRule, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rule, ok := m.rules[id]
	if !ok {
		return nil, false, nil
	}
	return cloneDelegateRule(rule), true, nil
}

// ListByOwner 查询委托人在指定租户下的全部规则（按创建时间升序）。
// @param ctx 操作上下文
// @param ownerID 委托人 Id
// @param tenantID 租户（=source_code）
// @return 规则列表
// @return 错误
func (m *MemTaskDelegateStore) ListByOwner(ctx context.Context, ownerID, tenantID string) ([]*DelegateRule, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*DelegateRule, 0, 4)
	for _, rule := range m.rules {
		// 与 Java selectByOwnerIdAndTime 一致：按 owner_id + source_code 过滤（不过滤启用态）
		if rule.OwnerID == ownerID && (tenantID == "" || rule.SourceCode == tenantID) {
			out = append(out, cloneDelegateRule(rule))
		}
	}
	sortDelegateRules(out)
	return out, nil
}

// ListByConsigner 查询指定身份作为被委托人的启用规则（按创建时间升序）。
// @param ctx 操作上下文
// @param consignerID 被委托人 Id
// @param tenantID 租户（=source_code）
// @return 启用中的规则列表
// @return 错误
func (m *MemTaskDelegateStore) ListByConsigner(ctx context.Context, consignerID, tenantID string) ([]*DelegateRule, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*DelegateRule, 0, 4)
	for _, rule := range m.rules {
		// 传递反查仅认启用中的规则
		if rule.Enable != DelegateEnabled {
			continue
		}
		if tenantID != "" && rule.SourceCode != tenantID {
			continue
		}
		// 被委托人列表包含目标身份即命中
		for _, consigner := range rule.Consigners {
			if consigner.ConsignerID == consignerID {
				out = append(out, cloneDelegateRule(rule))
				break
			}
		}
	}
	sortDelegateRules(out)
	return out, nil
}

// UpdateState 更新规则启用/退休标记。
// @param ctx 操作上下文
// @param id 委托规则 Id
// @param enable 启用标记
// @param retire 退休标记
// @return 错误
func (m *MemTaskDelegateStore) UpdateState(ctx context.Context, id string, enable, retire int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	rule, ok := m.rules[id]
	// 与 Java updateById 一致：规则不存在时静默无效果（影响行数 0）
	if !ok {
		return nil
	}
	rule.Enable = enable
	rule.Retire = retire
	rule.UpdateTime = m.clock.Now()
	return nil
}

// Delete 按主键级联删除委托规则。
// @param ctx 操作上下文
// @param id 委托规则 Id
// @return 错误
func (m *MemTaskDelegateStore) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.rules, id)
	return nil
}

// 编译期断言：内存实现满足端口。
var _ TaskDelegateStore = (*MemTaskDelegateStore)(nil)

// cloneDelegateRule 复制委托规则（含级联子表深拷贝）。
// @param r 原规则
// @return 副本
func cloneDelegateRule(r *DelegateRule) *DelegateRule {
	if r == nil {
		return nil
	}
	copied := *r
	copied.Consigners = make([]DelegateConsigner, len(r.Consigners))
	copy(copied.Consigners, r.Consigners)
	copied.ConsignObjs = make([]DelegateConsignObj, len(r.ConsignObjs))
	copy(copied.ConsignObjs, r.ConsignObjs)
	return &copied
}

// sortDelegateRules 按创建时间升序排序（同刻按 Id 保证确定性）。
// @param rules 规则列表
func sortDelegateRules(rules []*DelegateRule) {
	for i := 1; i < len(rules); i++ {
		for j := i; j > 0 && rules[j].CreateTime.Before(rules[j-1].CreateTime); j-- {
			rules[j], rules[j-1] = rules[j-1], rules[j]
		}
	}
}

// MemDelegateTaskMirrorStore 是委托镜像待办端口的内存实现。
type MemDelegateTaskMirrorStore struct {
	mu     sync.RWMutex                 // 读写锁
	mirror map[string]*DelegateTaskMirror // id → 镜像行
	clock  clock.Clock                  // 时间源
	newID  func() string                // ID 生成函数
}

// NewMemDelegateTaskMirrorStore 创建内存委托镜像存储。
// @param c 时间源，nil 时回退真实时钟
// @return 内存存储
func NewMemDelegateTaskMirrorStore(c clock.Clock) *MemDelegateTaskMirrorStore {
	if c == nil {
		c = clock.Real{}
	}
	return &MemDelegateTaskMirrorStore{mirror: make(map[string]*DelegateTaskMirror), clock: c, newID: DefaultIDGen()}
}

// Save 插入一条委托镜像待办（ID 缺失自动生成，审计列按当前时间填充）。
// @param ctx 操作上下文
// @param mirror 镜像待办
// @return 错误
func (m *MemDelegateTaskMirrorStore) Save(ctx context.Context, mirror *DelegateTaskMirror) error {
	// 任务与被委托人是镜像行的最小定位信息
	if mirror == nil || mirror.TaskID == "" || mirror.EntrustID == "" {
		return apiError("WORKFLOW-1702", "委托镜像缺少任务/被委托人信息")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	fillBase(&mirror.BaseColumns, m.newID, m.clock.Now, "", mirror.TenantID)
	m.mirror[mirror.ID] = cloneDelegateMirror(mirror)
	return nil
}

// ListByTaskID 查询任务的镜像待办列表（按创建时间升序）。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @return 镜像列表
// @return 错误
func (m *MemDelegateTaskMirrorStore) ListByTaskID(ctx context.Context, taskID string) ([]*DelegateTaskMirror, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*DelegateTaskMirror, 0, 4)
	for _, row := range m.mirror {
		if row.TaskID == taskID {
			out = append(out, cloneDelegateMirror(row))
		}
	}
	sortDelegateMirrors(out)
	return out, nil
}

// ListByEntrust 查询被委托人的镜像待办列表（按创建时间升序）。
// @param ctx 操作上下文
// @param entrustID 被委托人 Id
// @return 镜像列表
// @return 错误
func (m *MemDelegateTaskMirrorStore) ListByEntrust(ctx context.Context, entrustID string) ([]*DelegateTaskMirror, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*DelegateTaskMirror, 0, 4)
	for _, row := range m.mirror {
		// entrust_id 为逗号串（Java String.join 语义），包含即命中
		if mirrorContainsID(row.EntrustID, entrustID) {
			out = append(out, cloneDelegateMirror(row))
		}
	}
	sortDelegateMirrors(out)
	return out, nil
}

// DeleteByProcessInstance 删除实例下全部镜像待办。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 错误
func (m *MemDelegateTaskMirrorStore) DeleteByProcessInstance(ctx context.Context, processInstanceID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, row := range m.mirror {
		if row.ProcessInstanceID == processInstanceID {
			delete(m.mirror, id)
		}
	}
	return nil
}

// 编译期断言：内存实现满足端口。
var _ DelegateTaskMirrorStore = (*MemDelegateTaskMirrorStore)(nil)

// cloneDelegateMirror 复制镜像待办（含指针字段深拷贝）。
// @param r 原镜像
// @return 副本
func cloneDelegateMirror(r *DelegateTaskMirror) *DelegateTaskMirror {
	if r == nil {
		return nil
	}
	copied := *r
	copied.ExtendFields = cloneExt(r.ExtendFields)
	copied.ReadTime = cloneTime(r.ReadTime)
	copied.PlanFinishTime = cloneTime(r.PlanFinishTime)
	copied.TaskReceiptTime = cloneTime(r.TaskReceiptTime)
	copied.TaskStartTime = cloneTime(r.TaskStartTime)
	copied.DelayTime = cloneTime(r.DelayTime)
	return &copied
}

// sortDelegateMirrors 按创建时间升序排序镜像行。
// @param rows 镜像列表
func sortDelegateMirrors(rows []*DelegateTaskMirror) {
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && rows[j].CreateTime.Before(rows[j-1].CreateTime); j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
}

// mirrorContainsID 判断逗号串中是否包含指定 Id（去空白精确匹配）。
// @param joined 逗号串
// @param id 目标 Id
// @return 是否包含
func mirrorContainsID(joined, id string) bool {
	if id == "" {
		return false
	}
	for _, part := range strings.Split(joined, ",") {
		if strings.TrimSpace(part) == id {
			return true
		}
	}
	return false
}

// ─── SQL 适配 ─────────────────────────────────────────────

// SQLTaskDelegateStore 是委托规则端口的 SQL 适配器（组合 pwf 三表仓储）。
type SQLTaskDelegateStore struct {
	repos *pwf.Repositories // workflow 行仓储聚合
}

// NewSQLTaskDelegateStore 创建委托规则 SQL 适配器。
// @param store workflow 仓储入口（事务经 ctx 透传）
// @return SQL 适配器
func NewSQLTaskDelegateStore(store *pwf.Store) *SQLTaskDelegateStore {
	return &SQLTaskDelegateStore{repos: pwf.NewRepositories(store)}
}

// Save 级联保存委托规则（主表 upsert：更新命中行则完成；0 行或失败回退插入；子表先删后增）。
// @param ctx 操作上下文
// @param rule 委托规则
// @return 错误
func (s *SQLTaskDelegateStore) Save(ctx context.Context, rule *DelegateRule) error {
	if rule == nil || rule.OwnerID == "" || rule.SourceCode == "" {
		return apiError("WORKFLOW-1701", "委托规则缺少委托人/来源系统代号")
	}
	affected, err := s.repos.TaskDelegates.UpdateByPK(ctx, rule.ID, delegateRuleToRow(rule))
	// 更新命中：仅重建子表
	if err == nil && affected > 0 {
		return s.saveChildren(ctx, rule)
	}
	// 新增/更新未命中：插入主表后重建子表
	if insertErr := s.repos.TaskDelegates.Insert(ctx, delegateRuleToRow(rule)); insertErr != nil {
		return insertErr
	}
	return s.saveChildren(ctx, rule)
}

// saveChildren 重建子表关联（被委托人/委托对象先删后增，Java updateEntity 同款）。
// @param ctx 操作上下文
// @param rule 委托规则
// @return 错误
func (s *SQLTaskDelegateStore) saveChildren(ctx context.Context, rule *DelegateRule) error {
	if _, err := s.repos.TaskConsigners.DeleteByDelegate(ctx, rule.ID); err != nil {
		return err
	}
	for _, consigner := range rule.Consigners {
		row := pwf.Row{
			"id":            consigner.ID,
			"delegate_id":   rule.ID,
			"consigner_id":  consigner.ConsignerID,
			"consigner_name": consigner.ConsignerName,
		}
		if err := s.repos.TaskConsigners.Insert(ctx, row); err != nil {
			return err
		}
	}
	if _, err := s.repos.TaskConsignObjs.DeleteByDelegate(ctx, rule.ID); err != nil {
		return err
	}
	for _, obj := range rule.ConsignObjs {
		row := pwf.Row{
			"id":          obj.ID,
			"delegate_id": rule.ID,
			"owner_type":  obj.OwnerType,
			"owner_id":    obj.OwnerID,
			"owner_name":  obj.OwnerName,
		}
		if err := s.repos.TaskConsignObjs.Insert(ctx, row); err != nil {
			return err
		}
	}
	return nil
}

// Get 按主键级联读取委托规则（主表 + 子表聚合）。
// @param ctx 操作上下文
// @param id 委托规则 Id
// @return 委托规则
// @return 是否存在
// @return 错误
func (s *SQLTaskDelegateStore) Get(ctx context.Context, id string) (*DelegateRule, bool, error) {
	row, found, err := s.repos.TaskDelegates.GetByPK(ctx, id)
	if err != nil || !found {
		return nil, false, err
	}
	rule := delegateRuleFromRow(row)
	if err := s.loadChildren(ctx, rule); err != nil {
		return nil, false, err
	}
	return rule, true, nil
}

// loadChildren 装载规则的子表数据。
// @param ctx 操作上下文
// @param rule 委托规则
// @return 错误
func (s *SQLTaskDelegateStore) loadChildren(ctx context.Context, rule *DelegateRule) error {
	consignerRows, err := s.repos.TaskConsigners.ListByDelegate(ctx, rule.ID)
	if err != nil {
		return err
	}
	for _, row := range consignerRows {
		rule.Consigners = append(rule.Consigners, DelegateConsigner{
			ID:            rowString(row, "id"),
			DelegateID:    rowString(row, "delegate_id"),
			ConsignerID:   rowString(row, "consigner_id"),
			ConsignerName: rowString(row, "consigner_name"),
		})
	}
	objRows, err := s.repos.TaskConsignObjs.ListByDelegate(ctx, rule.ID)
	if err != nil {
		return err
	}
	for _, row := range objRows {
		rule.ConsignObjs = append(rule.ConsignObjs, DelegateConsignObj{
			ID:        rowString(row, "id"),
			DelegateID: rowString(row, "delegate_id"),
			OwnerType: rowString(row, "owner_type"),
			OwnerID:   rowString(row, "owner_id"),
			OwnerName: rowString(row, "owner_name"),
		})
	}
	return nil
}

// ListByOwner 查询委托人在指定租户下的全部规则（逐条级联）。
// @param ctx 操作上下文
// @param ownerID 委托人 Id
// @param tenantID 租户（=source_code）
// @return 规则列表
// @return 错误
func (s *SQLTaskDelegateStore) ListByOwner(ctx context.Context, ownerID, tenantID string) ([]*DelegateRule, error) {
	// 行仓储仅有按 owner 的启用查询，这里取 owner 全量后按租户内存过滤
	rows, err := s.repos.TaskDelegates.ListByOwner(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	out := make([]*DelegateRule, 0, len(rows))
	for _, row := range rows {
		if tenantID != "" && rowString(row, "source_code") != tenantID {
			continue
		}
		rule := delegateRuleFromRow(row)
		// 级联装载失败即整体失败，避免校验/命中读到残缺规则
		if err := s.loadChildren(ctx, rule); err != nil {
			return nil, err
		}
		out = append(out, rule)
	}
	return out, nil
}

// ListByConsigner 查询指定身份作为被委托人的启用规则
// （行仓储按来源代号查询启用规则后内存过滤被委托人；启用过滤与仓储 SQL 一致）。
// @param ctx 操作上下文
// @param consignerID 被委托人 Id
// @param tenantID 租户（=source_code）
// @return 启用中的规则列表
// @return 错误
func (s *SQLTaskDelegateStore) ListByConsigner(ctx context.Context, consignerID, tenantID string) ([]*DelegateRule, error) {
	// 租户缺失时无法下推来源过滤，返回空集（传递反查的最小键）
	if tenantID == "" {
		return []*DelegateRule{}, nil
	}
	rows, err := s.repos.TaskDelegates.ListBySourceCode(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]*DelegateRule, 0, 4)
	for _, row := range rows {
		rule := delegateRuleFromRow(row)
		// 子表未随行返回，按被委托人表逐规则核验（enable 过滤已由仓储 SQL 完成）
		consignerRows, err := s.repos.TaskConsigners.ListByDelegate(ctx, rule.ID)
		if err != nil {
			return nil, err
		}
		for _, consignerRow := range consignerRows {
			if rowString(consignerRow, "consigner_id") == consignerID {
				out = append(out, rule)
				break
			}
		}
	}
	return out, nil
}

// UpdateState 更新规则启用/退休标记。
// @param ctx 操作上下文
// @param id 委托规则 Id
// @param enable 启用标记
// @param retire 退休标记
// @return 错误
func (s *SQLTaskDelegateStore) UpdateState(ctx context.Context, id string, enable, retire int) error {
	_, err := s.repos.TaskDelegates.UpdateByPK(ctx, id, pwf.Row{"enable": enable, "retire": retire})
	return err
}

// Delete 按主键级联删除委托规则（主表 + 两张子表）。
// @param ctx 操作上下文
// @param id 委托规则 Id
// @return 错误
func (s *SQLTaskDelegateStore) Delete(ctx context.Context, id string) error {
	if _, err := s.repos.TaskConsigners.DeleteByDelegate(ctx, id); err != nil {
		return err
	}
	if _, err := s.repos.TaskConsignObjs.DeleteByDelegate(ctx, id); err != nil {
		return err
	}
	_, err := s.repos.TaskDelegates.DeleteByPK(ctx, id)
	return err
}

// 编译期断言：SQL 适配满足端口。
var _ TaskDelegateStore = (*SQLTaskDelegateStore)(nil)

// delegateRuleToRow 实体转通用行。
// @param rule 委托规则
// @return 通用行
func delegateRuleToRow(rule *DelegateRule) pwf.Row {
	row := pwf.Row{
		"consign_type": rule.ConsignType,
		"owner_id":     rule.OwnerID,
		"owner_name":   rule.OwnerName,
		"source":       rule.Source,
		"source_code":  rule.SourceCode,
		"source_name":  rule.SourceName,
		"type":         rule.Type,
		"condition":    rule.Condition,
		"vaild_from":   rule.VaildFrom,
		"vaild_to":     rule.VaildTo,
		"enable":       rule.Enable,
		"retire":       rule.Retire,
		"remark":       rule.Remark,
	}
	// 零值时间不落库（交由仓储语义或保持 NULL）
	if !rule.VaildFrom.IsZero() {
		row["vaild_from"] = rule.VaildFrom
	}
	if !rule.VaildTo.IsZero() {
		row["vaild_to"] = rule.VaildTo
	}
	appendBaseRow(row, rule.BaseColumns)
	return row
}

// delegateRuleFromRow 通用行转实体。
// @param row 通用行
// @return 委托规则
func delegateRuleFromRow(row pwf.Row) *DelegateRule {
	return &DelegateRule{
		BaseColumns: baseFromRow(row),
		ConsignType: rowString(row, "consign_type"),
		OwnerID:     rowString(row, "owner_id"),
		OwnerName:   rowString(row, "owner_name"),
		Source:      rowString(row, "source"),
		SourceCode:  rowString(row, "source_code"),
		SourceName:  rowString(row, "source_name"),
		Type:        rowString(row, "type"),
		Condition:   rowString(row, "condition"),
		VaildFrom:   timeOrZero(row, "vaild_from"),
		VaildTo:     timeOrZero(row, "vaild_to"),
		Enable:      rowInt(row, "enable"),
		Retire:      rowInt(row, "retire"),
		Remark:      rowString(row, "remark"),
	}
}

// SQLDelegateTaskMirrorStore 是委托镜像待办端口的 SQL 适配器。
type SQLDelegateTaskMirrorStore struct {
	repos *pwf.Repositories // workflow 行仓储聚合
}

// NewSQLDelegateTaskMirrorStore 创建委托镜像 SQL 适配器。
// @param store workflow 仓储入口
// @return SQL 适配器
func NewSQLDelegateTaskMirrorStore(store *pwf.Store) *SQLDelegateTaskMirrorStore {
	return &SQLDelegateTaskMirrorStore{repos: pwf.NewRepositories(store)}
}

// Save 插入一条委托镜像待办。
// @param ctx 操作上下文
// @param mirror 镜像待办
// @return 错误
func (s *SQLDelegateTaskMirrorStore) Save(ctx context.Context, mirror *DelegateTaskMirror) error {
	if mirror == nil || mirror.TaskID == "" || mirror.EntrustID == "" {
		return apiError("WORKFLOW-1702", "委托镜像缺少任务/被委托人信息")
	}
	// 镜像行 = 待办整行复制 + entrust 两列（messageToRow 已含全部 message_extend 列）
	row := messageToRow(&mirror.MessageExtend)
	row["entrust_id"] = mirror.EntrustID
	row["entrust_name"] = mirror.EntrustName
	return s.repos.DelegateTasks.Insert(ctx, row)
}

// ListByTaskID 查询任务的镜像待办列表。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @return 镜像列表
// @return 错误
func (s *SQLDelegateTaskMirrorStore) ListByTaskID(ctx context.Context, taskID string) ([]*DelegateTaskMirror, error) {
	row, found, err := s.repos.DelegateTasks.GetByTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	out := make([]*DelegateTaskMirror, 0, 1)
	// 行仓储 GetByTask 为单行查询，镜像行按任务至多一条与 DDL 语义一致
	if found {
		out = append(out, delegateMirrorFromRow(row))
	}
	return out, nil
}

// ListByEntrust 查询被委托人的镜像待办列表。
// @param ctx 操作上下文
// @param entrustID 被委托人 Id
// @return 镜像列表
// @return 错误
func (s *SQLDelegateTaskMirrorStore) ListByEntrust(ctx context.Context, entrustID string) ([]*DelegateTaskMirror, error) {
	rows, err := s.repos.DelegateTasks.ListByEntrust(ctx, entrustID)
	if err != nil {
		return nil, err
	}
	out := make([]*DelegateTaskMirror, 0, len(rows))
	for _, row := range rows {
		out = append(out, delegateMirrorFromRow(row))
	}
	return out, nil
}

// DeleteByProcessInstance 删除实例下全部镜像待办。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 错误
func (s *SQLDelegateTaskMirrorStore) DeleteByProcessInstance(ctx context.Context, processInstanceID string) error {
	_, err := s.repos.DelegateTasks.DeleteByProcessInstance(ctx, processInstanceID)
	return err
}

// 编译期断言：SQL 适配满足端口。
var _ DelegateTaskMirrorStore = (*SQLDelegateTaskMirrorStore)(nil)

// delegateMirrorFromRow 通用行转镜像实体。
// @param row 通用行
// @return 镜像待办
func delegateMirrorFromRow(row pwf.Row) *DelegateTaskMirror {
	return &DelegateTaskMirror{
		MessageExtend: *messageFromRow(row),
		EntrustID:     rowString(row, "entrust_id"),
		EntrustName:   rowString(row, "entrust_name"),
	}
}

// ─── 域服务 ─────────────────────────────────────────────

// DelegateRuleService 是规则型委托域服务：委托规则维护、命中判定与待办调整。
type DelegateRuleService struct {
	store      TaskDelegateStore       // 三表存储
	mirrors    DelegateTaskMirrorStore // 委托镜像待办
	messages   *MessageExtendService   // 待办扩展服务
	next       *NextActivityService    // 下一步预生成服务
	operations *OperationRecordService // 操作记录服务（WP 留痕）
	engine     api.Engine              // 引擎门面（待办调整时改写任务 assignee）
	users      UserQuery               // 身份查询（补姓名展示）
	categories CategoryLookup          // 分类代号解析（可 nil）
	clock      clock.Clock             // 时间源
	newID      func() string           // ID 生成函数
}

// NewDelegateRuleService 创建规则型委托域服务。
// @param store 委托规则三表存储
// @param mirrors 委托镜像待办存储（可 nil，镜像功能降级跳过）
// @param messages 待办扩展服务（待办改派用，可 nil）
// @param next 下一步预生成服务（待办调整用，可 nil）
// @param operations 操作记录服务（WP 留痕用，可 nil）
// @param engine 引擎门面（任务 assignee 改写用，可 nil）
// @param users 身份查询（可 nil，姓名留空）
// @param categories 分类代号解析（可 nil，按 Id 直接匹配）
// @param c 时间源，nil 时回退真实时钟
// @return 域服务
func NewDelegateRuleService(store TaskDelegateStore, mirrors DelegateTaskMirrorStore, messages *MessageExtendService, next *NextActivityService, operations *OperationRecordService, engine api.Engine, users UserQuery, categories CategoryLookup, c clock.Clock) *DelegateRuleService {
	if c == nil {
		c = clock.Real{}
	}
	return &DelegateRuleService{
		store: store, mirrors: mirrors, messages: messages, next: next, operations: operations,
		engine: engine, users: users, categories: categories, clock: c, newID: DefaultIDGen(),
	}
}

// Add 新增委托规则（对位 Java WorkflowTaskDelegateServiceImpl#add:58）：
// 校验链（系统代号可选存在 / 委托人与时间必填 / 被委托人非空且非本人 / 时间重叠 / 禁止传递）
// → 生成 condition 串 → 级联保存（enable=1）。
// @param ctx 操作上下文
// @param rule 委托规则
// @return 错误
func (s *DelegateRuleService) Add(ctx context.Context, rule *DelegateRule) error {
	if rule == nil {
		return apiError("WORKFLOW-1703", "委托规则为空")
	}
	if err := s.validateRule(ctx, rule, ""); err != nil {
		return err
	}
	s.buildCondition(rule)
	rule.Enable = DelegateEnabled
	fillBase(&rule.BaseColumns, s.newID, s.clock.Now, firstNonEmpty(rule.Creator, rule.OwnerID), rule.SourceCode)
	return s.store.Save(ctx, rule)
}

// Update 更新委托规则（对位 Java updateEntity:199）：存在性校验 + 复制可变字段 + 与 Add 同款校验链
// → 重建 condition → 子表先删后增（存储 Save 内完成）。
// @param ctx 操作上下文
// @param rule 委托规则（必填 ID）
// @return 错误
func (s *DelegateRuleService) Update(ctx context.Context, rule *DelegateRule) error {
	if rule == nil || rule.ID == "" {
		return apiError("WORKFLOW-1703", "委托规则或规则 ID 为空")
	}
	existing, found, err := s.store.Get(ctx, rule.ID)
	if err != nil {
		return err
	}
	// Java："该工作流委托已经不存在，请联系管理员"
	if !found {
		return apiError("WORKFLOW-1704", "该工作流委托已经不存在，请联系管理员")
	}
	// 非空字段覆盖既有实体（SpringBeanUtil.copyPropertiesIgnoreNull 等价）
	if rule.ConsignType != "" {
		existing.ConsignType = rule.ConsignType
	}
	if rule.OwnerID != "" {
		existing.OwnerID = rule.OwnerID
	}
	if rule.OwnerName != "" {
		existing.OwnerName = rule.OwnerName
	}
	if rule.SourceCode != "" {
		existing.SourceCode = rule.SourceCode
	}
	if rule.Type != "" {
		existing.Type = rule.Type
	}
	if rule.Remark != "" {
		existing.Remark = rule.Remark
	}
	if !rule.VaildFrom.IsZero() {
		existing.VaildFrom = rule.VaildFrom
	}
	if !rule.VaildTo.IsZero() {
		existing.VaildTo = rule.VaildTo
	}
	// 子表列表非空才覆盖（与 copyPropertiesIgnoreNull 语义一致）
	if len(rule.Consigners) > 0 {
		existing.Consigners = rule.Consigners
	}
	if len(rule.ConsignObjs) > 0 {
		existing.ConsignObjs = rule.ConsignObjs
	}
	if err := s.validateRule(ctx, existing, rule.ID); err != nil {
		return err
	}
	s.buildCondition(existing)
	existing.Updater = firstNonEmpty(existing.Updater, existing.OwnerID)
	existing.UpdateTime = s.clock.Now()
	return s.store.Save(ctx, existing)
}

// Enable 启用委托规则。
// @param ctx 操作上下文
// @param id 委托规则 Id
// @return 错误
func (s *DelegateRuleService) Enable(ctx context.Context, id string) error {
	return s.store.UpdateState(ctx, id, DelegateEnabled, DelegateDisabled)
}

// Disable 停用委托规则。
// @param ctx 操作上下文
// @param id 委托规则 Id
// @return 错误
func (s *DelegateRuleService) Disable(ctx context.Context, id string) error {
	return s.store.UpdateState(ctx, id, DelegateDisabled, DelegateDisabled)
}

// Retire 退休委托规则（retire=1 且停用，Java Retire 语义）。
// @param ctx 操作上下文
// @param id 委托规则 Id
// @return 错误
func (s *DelegateRuleService) Retire(ctx context.Context, id string) error {
	return s.store.UpdateState(ctx, id, DelegateDisabled, DelegateRetired)
}

// List 查询委托人在指定租户下的全部规则（含停用，管理台列表用）。
// @param ctx 操作上下文
// @param ownerID 委托人 Id
// @param tenantID 租户
// @return 规则列表
// @return 错误
func (s *DelegateRuleService) List(ctx context.Context, ownerID, tenantID string) ([]*DelegateRule, error) {
	return s.store.ListByOwner(ctx, ownerID, tenantID)
}

// Remove 删除委托规则（级联子表）。
// @param ctx 操作上下文
// @param id 委托规则 Id
// @return 错误
func (s *DelegateRuleService) Remove(ctx context.Context, id string) error {
	return s.store.Delete(ctx, id)
}

// validateRule 新增/更新共用的校验链（对位 Java add:58 / updateEntity:199）。
// @param ctx 操作上下文
// @param rule 待校验规则
// @param withoutID 更新场景排除自身 Id，新增传空串
// @return 错误
func (s *DelegateRuleService) validateRule(ctx context.Context, rule *DelegateRule, withoutID string) error {
	// 委托人必填（Java "委托人不能为空！"）
	if rule.OwnerID == "" {
		return apiError("WORKFLOW-1705", "委托人不能为空！")
	}
	// 生效/失效时间必填
	if rule.VaildFrom.IsZero() {
		return apiError("WORKFLOW-1705", "生效时间不能为空！")
	}
	if rule.VaildTo.IsZero() {
		return apiError("WORKFLOW-1705", "失效时间不能为空！")
	}
	// 失效时间早于生效时间为非法区间
	if rule.VaildTo.Before(rule.VaildFrom) {
		return apiError("WORKFLOW-1705", "失效时间不能早于生效时间！")
	}
	// 被委托人必填且不能包含委托人本人
	if len(rule.Consigners) == 0 {
		return apiError("WORKFLOW-1705", "被委托人不能为空！")
	}
	for _, consigner := range rule.Consigners {
		if consigner.ConsignerID == "" {
			return apiError("WORKFLOW-1705", "被委托人ID不能为空！")
		}
		if consigner.ConsignerID == rule.OwnerID {
			return apiError("WORKFLOW-1705", "被委托人中不能包含委托人！")
		}
	}
	// 全部委托无需委托对象；范围委托必须携带对象
	if rule.ConsignType == "" {
		rule.ConsignType = ConsignTypeAll
	}
	if rule.ConsignType != ConsignTypeAll && len(rule.ConsignObjs) == 0 {
		// Java："项目名称不能为空！" / "流程名称不能为空！"
		if isProjectConsignType(rule.ConsignType) {
			return apiError("WORKFLOW-1705", "项目名称不能为空！")
		}
		return apiError("WORKFLOW-1705", "流程名称不能为空！")
	}
	// 时间重叠三段式校验（按任务规格仅校验启用中的既有规则）
	overlap, err := s.hasOverlappingRule(ctx, rule, withoutID)
	if err != nil {
		return err
	}
	if overlap {
		return apiError("WORKFLOW-1706", "当前委托时间与已有委托时间重叠！不能保存当前委托！")
	}
	// 禁止委托传递：委托人本身已在生效委托的被委托人列表
	passed, err := s.isConsignerChain(ctx, rule, withoutID)
	if err != nil {
		return err
	}
	if passed {
		return apiError("WORKFLOW-1706", "委托人在当前委托时间已被委托！委托人不能传递委托")
	}
	return nil
}

// hasOverlappingRule 时间重叠三段式校验（Java selectByOwnerIdAndTime:101 → selectByConsignerIdAndTime:547
// 的 or 包裹逻辑）：同 owner+tenant 的既有启用规则中，任一条与新区间满足三段式交集即拒。
// 范围委托按对象匹配（共享对象或对方为全部委托）；全部委托仅与全部委托冲突。
// @param ctx 操作上下文
// @param rule 待校验规则
// @param withoutID 排除的规则 Id（更新自身）
// @return 是否存在重叠
// @return 错误
func (s *DelegateRuleService) hasOverlappingRule(ctx context.Context, rule *DelegateRule, withoutID string) (bool, error) {
	existing, err := s.store.ListByOwner(ctx, rule.OwnerID, rule.SourceCode)
	if err != nil {
		return false, err
	}
	for _, other := range existing {
		// 更新场景排除自身
		if withoutID != "" && other.ID == withoutID {
			continue
		}
		// 仅启用中的规则参与冲突判定（任务规格：与既有启用委托交集即拒）
		if other.Enable != DelegateEnabled {
			continue
		}
		// 全部委托仅与全部委托比较（Java consign_type 过滤语义）
		if rule.ConsignType == ConsignTypeAll || other.ConsignType == ConsignTypeAll {
			// 一方为全部委托时：Java 语义为 All 只对比 All，此处取对称收紧——双方均为 All 才冲突
			if rule.ConsignType == ConsignTypeAll && other.ConsignType == ConsignTypeAll && timeOverlaps(other.VaildFrom, other.VaildTo, rule.VaildFrom, rule.VaildTo) {
				return true, nil
			}
			continue
		}
		// 范围委托需共享至少一个委托对象
		if !sharesConsignObj(rule, other) {
			continue
		}
		if timeOverlaps(other.VaildFrom, other.VaildTo, rule.VaildFrom, rule.VaildTo) {
			return true, nil
		}
	}
	return false, nil
}

// isConsignerChain 禁止委托传递（Java selectByConsignerIdAndTime 传递校验段）：
// 存在启用中的既有规则其被委托人列表包含当前规则的委托人（时间区间交集）即拒。
// 反查经 ListByConsigner 跨 owner 定位"委托人已是他人被委托人"的规则。
// @param ctx 操作上下文
// @param rule 待校验规则
// @param withoutID 排除的规则 Id（更新自身）
// @return 是否形成传递链
// @return 错误
func (s *DelegateRuleService) isConsignerChain(ctx context.Context, rule *DelegateRule, withoutID string) (bool, error) {
	// 委托人不能出现在自身被委托人列表（防御）
	for _, consigner := range rule.Consigners {
		if consigner.ConsignerID == rule.OwnerID {
			return true, nil
		}
	}
	existing, err := s.store.ListByConsigner(ctx, rule.OwnerID, rule.SourceCode)
	if err != nil {
		return false, err
	}
	for _, other := range existing {
		if withoutID != "" && other.ID == withoutID {
			continue
		}
		// 时间区间无交集不构成当前时间段的传递链
		if !timeOverlaps(other.VaildFrom, other.VaildTo, rule.VaildFrom, rule.VaildTo) {
			continue
		}
		// 委托人已是他人（含自身名下其他规则）的被委托人 → 不能再委托出去
		return true, nil
	}
	return false, nil
}

// buildCondition 生成委托条件串（对位 Java add:156-169；修正 Java :161 流程分类误用项目前缀的笔误）。
// 全部委托不生成；其余类型按范围取 @ProjId/@FlowCategory 前缀 + 对象 Id 逗号串。
// @param rule 委托规则（原位写入 Condition）
func (s *DelegateRuleService) buildCondition(rule *DelegateRule) {
	// 全部委托无条件串（Java 同款）
	if rule.ConsignType == ConsignTypeAll {
		rule.Condition = ""
		return
	}
	prefix := ConditionProjectPrefix
	// 流程分类范围使用分类前缀（修正 Java :161 对 SingleFlowCategory 误用项目前缀的笔误）
	if isCategoryConsignType(rule.ConsignType) {
		prefix = ConditionFlowCategoryPrefix
	}
	// Java :163 判空对象为 consigners、取值对象为 consignObjs（笔误风险）；
	// Go 修正为按委托对象拼接
	ids := make([]string, 0, len(rule.ConsignObjs))
	for _, obj := range rule.ConsignObjs {
		ids = append(ids, obj.OwnerID)
	}
	rule.Condition = prefix + "='" + strings.Join(ids, ",") + "'"
}

// HitConsigner 命中委托（对位 Java getTaskConsignerId:627）：
// 优先级 单项目委托 > 单流程分类委托 > 全部委托，按当前时间过滤启用中的委托。
// @param ctx 操作上下文
// @param ownerID 待办处理人身份 Id（委托人）
// @param tenantID 租户（=source_code）
// @param projectID 项目 Id（可空）
// @param flowCategory 流程分类代号（可空）
// @return 命中的被委托人 Id，无命中返回空串
// @return 错误
func (s *DelegateRuleService) HitConsigner(ctx context.Context, ownerID, tenantID, projectID, flowCategory string) (string, error) {
	// 与 Java 一致：委托人或租户缺失直接返回未命中
	if ownerID == "" || tenantID == "" {
		return "", nil
	}
	rules, err := s.store.ListByOwner(ctx, ownerID, tenantID)
	if err != nil {
		return "", err
	}
	now := s.clock.Now()
	// 1. 单项目委托（含多项目命中该项目）
	if projectID != "" {
		if consigner := hitConsignerOf(rules, now, projectID, isProjectConsignType); consigner != "" {
			return consigner, nil
		}
	}
	// 2. 单流程分类委托（对象 Id 为模型 Id，经分类解析端口对齐；未注入时按 Id==代号匹配）
	if flowCategory != "" {
		modelID, err := s.resolveCategoryModelID(ctx, tenantID, flowCategory)
		if err != nil {
			return "", err
		}
		if consigner := hitConsignerOf(rules, now, modelID, isCategoryConsignType); consigner != "" {
			return consigner, nil
		}
	}
	// 3. 全部委托
	if consigner := hitConsignerOf(rules, now, "", func(consignType string) bool { return consignType == ConsignTypeAll }); consigner != "" {
		return consigner, nil
	}
	return "", nil
}

// hitConsignerOf 在规则列表中按范围谓词与时间窗口命中第一条被委托人。
// @param rules 委托规则列表
// @param now 当前时间
// @param objectID 委托对象 Id（空表示不匹配对象，用于全部委托）
// @param typeMatch 委托范围谓词
// @return 被委托人 Id，无命中返回空串
func hitConsignerOf(rules []*DelegateRule, now time.Time, objectID string, typeMatch func(string) bool) string {
	for _, rule := range rules {
		// 启用与退休过滤（命中仅认启用中的规则）
		if rule.Enable != DelegateEnabled || rule.Retire == DelegateRetired {
			continue
		}
		if !typeMatch(rule.ConsignType) {
			continue
		}
		// 对象匹配（全部委托跳过）
		if objectID != "" && !ruleHasConsignObj(rule, objectID) {
			continue
		}
		// 时间窗口：Java 按天宽化（vaild_from 00:00:00 ~ vaild_to 23:59:59）
		if !withinValidWindow(rule.VaildFrom, rule.VaildTo, now) {
			continue
		}
		// 取第一个被委托人（Java SQL 无排序取首条）
		if len(rule.Consigners) > 0 {
			return rule.Consigners[0].ConsignerID
		}
	}
	return ""
}

// resolveCategoryModelID 解析分类代号对应的模型 Id（未注入解析端口时按代号直匹配）。
// @param ctx 操作上下文
// @param tenantID 租户
// @param flowCategory 分类代号
// @return 模型 Id（解析失败返回代号本身，保持嵌入模式兼容）
// @return 错误
func (s *DelegateRuleService) resolveCategoryModelID(ctx context.Context, tenantID, flowCategory string) (string, error) {
	// 未注入解析端口：嵌入模式对象 Id 与代号同值
	if s.categories == nil {
		return flowCategory, nil
	}
	modelID, err := s.categories.ResolveModelID(ctx, tenantID, flowCategory)
	if err != nil {
		return "", err
	}
	// 未解析到模型时回退代号直匹配，避免阻断命中链
	if modelID == "" {
		return flowCategory, nil
	}
	return modelID, nil
}

// ApplyOnTodoCreate 待办生成时的规则委托改派钩子（本阶段实现方法本体，接线挂到待办创建链由主控完成）：
// 命中委托 → 待办 owner 改派为被委托人 + consigner 记录 + 委托镜像落库。
// 项目上下文取待办扩展列 extend_s1（Java 会签/委托场景约定列）。
// @param ctx 操作上下文
// @param msg 新生成的待办扩展（必填 OwnerID/FlowCategoryCode/TenantID）
// @return 改派后的待办（未命中或依赖缺失时原样返回）
// @return 错误
func (s *DelegateRuleService) ApplyOnTodoCreate(ctx context.Context, msg *MessageExtend) (*MessageExtend, error) {
	// 待办为空或缺少处理人/租户时无法判定委托，原样返回
	if msg == nil || msg.OwnerID == "" || msg.TenantID == "" {
		return msg, nil
	}
	consignerID, err := s.HitConsigner(ctx, msg.OwnerID, msg.TenantID, msg.ExtendS1, msg.FlowCategoryCode)
	if err != nil {
		return nil, err
	}
	// 未命中委托：待办保持原处理人
	if consignerID == "" {
		return msg, nil
	}
	originalOwner := msg.OwnerID
	originalOwnerName := msg.OwnerName
	// 待办改派：owner=被委托人，consigner 记录受托人（与 complete.go 委派语义一致）
	msg.OwnerID = consignerID
	msg.OwnerName = s.userDisplayName(ctx, consignerID)
	msg.ConsignerID = consignerID
	msg.Consigner = msg.OwnerName
	// 待办服务已注入时重建待办行（存储端口无列级更新，删后重建）
	if s.messages != nil {
		if _, found, getErr := s.messages.GetByTaskID(ctx, msg.TaskID); getErr == nil && found {
			if delErr := s.messages.DeleteByTaskID(ctx, msg.TaskID); delErr != nil {
				return nil, delErr
			}
			if _, createErr := s.messages.CreateForTask(ctx, msg); createErr != nil {
				return nil, createErr
			}
		}
	}
	// 委托镜像落库（镜像存储未注入时跳过，改派本身不受影响）
	if s.mirrors != nil {
		mirror := &DelegateTaskMirror{
			MessageExtend: *msg,
			EntrustID:     consignerID,
			EntrustName:   msg.OwnerName,
		}
		// 镜像行 consigner 记录委托人（原处理人），entrust 记录被委托人（docs 003 §3.8）
		mirror.ConsignerID = originalOwner
		mirror.Consigner = originalOwnerName
		if err := s.mirrors.Save(ctx, mirror); err != nil {
			return nil, err
		}
	}
	return msg, nil
}

// AdjustOwnerRequest 是待办调整请求（对位 Java ProcessesUtil#doTaskOwnerAdjustment:5856 入参）。
type AdjustOwnerRequest struct {
	TaskID          string   // 任务 Id（必填）
	CurrentOwnerID  string   // 当前处理人身份 Id（必填）
	UserCode        string   // 操作人身份 Id（必填，留痕用）
	OwnerIDs        []string // 新处理人身份 Id 列表（必填，可多人）
	KeepCurrentTask *bool    // 是否保留原处理人的预存与待办（必填，nil 视为不保留）
}

// AdjustTaskOwner 待办调整（对位 Java doTaskOwnerAdjustment:5856）：
// 校验 → 按 next_activity 的 taskId+identityId 复制新记录给多 owner（keepCurrentTask=false 删原记录）
// → 引擎任务 assignee 改写（首个新 owner，租户上下文取自任务）→ 待办重建 + 委托镜像 + WP 操作记录。
// @param ctx 操作上下文
// @param req 待办调整请求
// @return 错误
func (s *DelegateRuleService) AdjustTaskOwner(ctx context.Context, req AdjustOwnerRequest) error {
	// 入参校验链（Java 逐项 isBlank/null 校验）
	if req.TaskID == "" {
		return apiError("WORKFLOW-1707", "参数 taskId 为空")
	}
	if req.CurrentOwnerID == "" {
		return apiError("WORKFLOW-1707", "参数 currentOwnerId 为空")
	}
	if req.UserCode == "" {
		return apiError("WORKFLOW-1707", "参数 userCode 为空")
	}
	if len(req.OwnerIDs) == 0 {
		return apiError("WORKFLOW-1707", "参数 ownerIds 为空")
	}
	keepCurrentTask := false
	if req.KeepCurrentTask != nil {
		keepCurrentTask = *req.KeepCurrentTask
	}
	// Java："不能委托任务给当前办理人，请调整后重试。"
	for _, ownerID := range req.OwnerIDs {
		if ownerID == req.CurrentOwnerID {
			return apiError("WORKFLOW-1707", "不能委托任务给当前办理人，请调整后重试。")
		}
	}
	// 引擎任务存在性（Java："委托任务异常：任务不存在"）
	task, err := s.engineTask(ctx, req.TaskID)
	if err != nil {
		return err
	}
	// 预存记录复制与清理
	if s.next != nil {
		if err := s.adjustNextActivities(ctx, req, task, keepCurrentTask); err != nil {
			return err
		}
	}
	// 原处理人姓名（Java："委托任务异常：任务处理人不存在"）
	ownerName := s.userDisplayName(ctx, req.CurrentOwnerID)
	if s.users != nil && ownerName == "" {
		return apiError("WORKFLOW-1708", "委托任务异常：任务处理人不存在")
	}
	// 引擎任务 assignee 改写为首个新 owner（Java 未改写引擎任务，Go 按任务规格补齐；
	// 引擎调用需租户上下文，从任务实体取）
	if s.engine != nil && task.TenantID != "" {
		assignee := req.OwnerIDs[0]
		if _, err := s.engine.TaskService().UpdateTask(tenant.With(ctx, task.TenantID), req.TaskID, api.TaskUpdate{Assignee: &assignee, UserID: req.UserCode}); err != nil {
			return err
		}
	}
	// 待办重建：不保留原待办时改派给首个新 owner；镜像逐 owner 落库
	if err := s.adjustTodoAndMirror(ctx, req, task, ownerName, keepCurrentTask); err != nil {
		return err
	}
	// WP 操作记录（Java saveWorkflowOperationRecord）
	if s.operations != nil {
		if _, err := s.operations.Save(ctx, OperationRequest{
			TaskID:            req.TaskID,
			ProcessInstanceID: task.ProcessInstanceID,
			OwnerID:           req.UserCode,
			Message:           ownerName + "，委托任务【" + task.Name + "】给：" + strings.Join(s.ownerDisplayNames(ctx, req.OwnerIDs), ","),
			Type:              CommentTypeWP,
		}); err != nil {
			return err
		}
	}
	return nil
}

// adjustNextActivities 复制/清理下一步预存记录（Java doTaskOwnerAdjustment 1）：
// 以任务的首条预存记录为模板，为每个新 owner 复制一条；不保留原记录时按
// （任务+原 identity）删除运行态与历史记录。
// @param ctx 操作上下文
// @param req 待办调整请求
// @param task 引擎任务
// @param keepCurrentTask 是否保留原处理人记录
// @return 错误
func (s *DelegateRuleService) adjustNextActivities(ctx context.Context, req AdjustOwnerRequest, task api.Task, keepCurrentTask bool) error {
	records, err := s.next.ListByTaskID(ctx, req.TaskID)
	if err != nil {
		return err
	}
	// Java：nextActivityEntities 为空时跳过复制，不视为错误
	if len(records) == 0 {
		return nil
	}
	template := records[0]
	for _, ownerID := range req.OwnerIDs {
		copied := *template
		copied.BaseColumns = BaseColumns{TenantID: template.TenantID}
		copied.IdentityID = ownerID
		// 复制行 used=0（新记录等待执行人重写消耗，Java 新实体默认值）
		copied.Used = 0
		if err := s.next.InsertDual(ctx, &copied, req.UserCode); err != nil {
			return err
		}
	}
	// 不保留原处理人记录：删（任务+原 identity）的预存记录
	if !keepCurrentTask {
		for _, record := range records {
			// 仅清理原处理人的记录（其他 identity 的记录不属于本次调整范围）
			if record.IdentityID != req.CurrentOwnerID {
				continue
			}
			if err := s.next.DeleteForResolve(ctx, task.ProcessInstanceID, record.NextActivityID, req.TaskID, req.CurrentOwnerID); err != nil {
				return err
			}
		}
	}
	return nil
}

// adjustTodoAndMirror 重建待办与委托镜像（Java saveTaskMessageExtend + delegateTask 镜像 + 删原待办段）：
// 不保留原待办时待办改派首个新 owner（consigner 记录原处理人）；镜像行逐 owner 落库。
// @param ctx 操作上下文
// @param req 待办调整请求
// @param task 引擎任务
// @param ownerName 原处理人姓名
// @param keepCurrentTask 是否保留原待办
// @return 错误
func (s *DelegateRuleService) adjustTodoAndMirror(ctx context.Context, req AdjustOwnerRequest, task api.Task, ownerName string, keepCurrentTask bool) error {
	if s.messages == nil {
		return nil
	}
	existing, found, err := s.messages.GetByTaskID(ctx, req.TaskID)
	if err != nil {
		return err
	}
	// 委托镜像：以待办为模板逐 owner 复制（Java workflowDelegateTaskService.save）
	if found && s.mirrors != nil {
		for _, ownerID := range req.OwnerIDs {
			mirror := &DelegateTaskMirror{
				MessageExtend: *existing,
				EntrustID:     ownerID,
				EntrustName:   s.userDisplayName(ctx, ownerID),
			}
			// 镜像行重写归属与委托字段（每行一个被委托人）
			mirror.ID = ""
			mirror.OwnerID = ownerID
			mirror.OwnerName = mirror.EntrustName
			mirror.ConsignerID = req.CurrentOwnerID
			mirror.Consigner = ownerName
			if err := s.mirrors.Save(ctx, mirror); err != nil {
				return err
			}
		}
	}
	// 不保留原待办：改派给首个新 owner（Java deleteTaskMessageExtendByTaskIdAndOwnerId + 重生成）
	if found && !keepCurrentTask && len(req.OwnerIDs) > 0 {
		updated := *existing
		updated.ID = ""
		updated.OwnerID = req.OwnerIDs[0]
		updated.OwnerName = s.userDisplayName(ctx, updated.OwnerID)
		updated.ConsignerID = req.CurrentOwnerID
		updated.Consigner = ownerName
		if err := s.messages.DeleteByTaskID(ctx, req.TaskID); err != nil {
			return err
		}
		if _, err := s.messages.CreateForTask(ctx, &updated); err != nil {
			return err
		}
	}
	return nil
}

// engineTask 查询引擎任务（引擎未注入时返回仅含 Id 的占位任务，便于无引擎装配单测）。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @return 任务
// @return 错误
func (s *DelegateRuleService) engineTask(ctx context.Context, taskID string) (api.Task, error) {
	if s.engine == nil {
		return api.Task{}, apiError("WORKFLOW-1709", "委托服务未注入引擎，无法定位任务")
	}
	task, err := s.engine.TaskService().GetTask(ctx, taskID)
	if err != nil {
		return api.Task{}, api.NewError("WORKFLOW-1708", "委托任务异常：任务不存在", err)
	}
	return task, nil
}

// ownerDisplayNames 批量解析处理人展示名（身份查询缺失时回退 Id 本身）。
// @param ctx 操作上下文
// @param ownerIDs 处理人 Id 列表
// @return 展示名列表
func (s *DelegateRuleService) ownerDisplayNames(ctx context.Context, ownerIDs []string) []string {
	names := make([]string, 0, len(ownerIDs))
	for _, ownerID := range ownerIDs {
		name := s.userDisplayName(ctx, ownerID)
		// 查不到姓名时回退 Id，保证操作记录可读
		if name == "" {
			name = ownerID
		}
		names = append(names, name)
	}
	return names
}

// userDisplayName 查询用户展示名（身份查询不可用或查不到时返回空串）。
// @param ctx 操作上下文
// @param userID 用户 Id
// @return 展示名
func (s *DelegateRuleService) userDisplayName(ctx context.Context, userID string) string {
	if s.users == nil || userID == "" {
		return ""
	}
	if user, err := s.users.GetUser(ctx, userID); err == nil {
		return userDisplayName(user)
	}
	return ""
}

// timeOverlaps 时间重叠三段式判定（Java selectByConsignerIdAndTime:579-602 的 or 包裹逻辑，
// 方向与 Java mapper 一致——既有区间为参照）：
// 1) 新生效时间落入既有区间；2) 新失效时间落入既有区间；3) 既有区间被新区间包含。
// @param existingFrom 既有生效时间
// @param existingTo 既有失效时间
// @param newFrom 新生效时间
// @param newTo 新失效时间
// @return 是否重叠
func timeOverlaps(existingFrom, existingTo, newFrom, newTo time.Time) bool {
	// 任一方区间不完整不参与判定（零值时间视为未设置）
	if existingFrom.IsZero() || existingTo.IsZero() || newFrom.IsZero() || newTo.IsZero() {
		return false
	}
	// 1. 新生效时间落入既有区间（Java wrapper.le("vaild_from", vaildFrom).ge("vaild_to", vaildFrom)）
	if !existingFrom.After(newFrom) && !existingTo.Before(newFrom) {
		return true
	}
	// 2. 新失效时间落入既有区间
	if !existingFrom.After(newTo) && !existingTo.Before(newTo) {
		return true
	}
	// 3. 既有区间被新区间包含（既有生效≥新生效 且 既有失效≤新失效）
	return !existingFrom.Before(newFrom) && !existingTo.After(newTo)
}

// withinValidWindow 判断当前时间是否落在委托生效窗口内（Java mapper 按天宽化：
// vaild_from 取当日 00:00:00、vaild_to 取当日 23:59:59）。
// @param from 生效时间
// @param to 失效时间
// @param now 当前时间
// @return 是否在窗口内
func withinValidWindow(from, to, now time.Time) bool {
	// 窗口不完整视为未生效
	if from.IsZero() || to.IsZero() {
		return false
	}
	// 按天宽化：from 截到当日零点、to 取当日末秒（与 Java date_format+concat 语义一致）
	dayStart := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, from.Location())
	dayEnd := time.Date(to.Year(), to.Month(), to.Day(), 23, 59, 59, int(time.Second-time.Nanosecond), to.Location())
	return !now.Before(dayStart) && !now.After(dayEnd)
}

// isProjectConsignType 判断委托范围是否为项目维度（单项目/多项目）。
// @param consignType 委托范围
// @return 是否项目维度
func isProjectConsignType(consignType string) bool {
	return consignType == ConsignTypeSingleProject || consignType == ConsignTypeMultiProject
}

// isCategoryConsignType 判断委托范围是否为流程分类维度（单分类/多分类）。
// @param consignType 委托范围
// @return 是否分类维度
func isCategoryConsignType(consignType string) bool {
	return consignType == ConsignTypeSingleFlowCategory || consignType == ConsignTypeMultiFlowCategory
}

// sharesConsignObj 判断两条范围委托是否共享至少一个委托对象（对象 Id 相等即视为共享）。
// @param a 规则一
// @param b 规则二
// @return 是否共享对象
func sharesConsignObj(a, b *DelegateRule) bool {
	for _, objA := range a.ConsignObjs {
		for _, objB := range b.ConsignObjs {
			// 对象 Id 相等即共享（跨维度同名对象由配置方保证唯一）
			if objA.OwnerID != "" && objA.OwnerID == objB.OwnerID {
				return true
			}
		}
	}
	return false
}

// ruleHasConsignObj 判断规则是否包含指定委托对象 Id。
// @param rule 委托规则
// @param objectID 对象 Id
// @return 是否包含
func ruleHasConsignObj(rule *DelegateRule, objectID string) bool {
	for _, obj := range rule.ConsignObjs {
		if obj.OwnerID == objectID {
			return true
		}
	}
	return false
}
