// remind.go 提供催办（提醒）域服务（P4.3，对位 Java ReminderSchedule 与 workflow_remind /
// workflow_remind_target 两表）：
// 催办配置维护（pid + 步骤白名单 + 预警/延时目标）、对象展开（-1111 当前待办人 / ${initiator} 发起人 /
// 静态角色组成员 / 字面用户；动态角色 PATH 留接口位）、到期扫描判定（预警 dueTime-duration<=now、
// 延时 dueTime+duration<=now 或按任务接收时间）、步骤白名单过滤与重试计数。
package workflow

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
	pwf "github.com/DengQingNian/Flowtide/internal/persistence/workflow"
)

// 催办对象执行者特殊取值（docs 003 §3.18：executor_id 支持 ${initiator} 占位与 -1111=当前待办人）。
const (
	// RemindExecutorCurrentHandler 当前待办人占位（-1111）。
	RemindExecutorCurrentHandler = "-1111"
	// RemindExecutorInitiator 发起人占位（${initiator}，展开取流程变量 initiator）。
	RemindExecutorInitiator = "${initiator}"
)

// 催办类型与对象类型常量（对位 Java WorkflowRemindTargetEntity.TYPE_AHEAD/TYPE_DELAY）。
const (
	// RemindTypeAhead 预警型（提前量 ahead）。
	RemindTypeAhead = 1
	// RemindTypeDelay 延时型（延迟量 delay）。
	RemindTypeDelay = 2
	// RemindTargetTypeUser 用户对象。
	RemindTargetTypeUser = 1
	// RemindTargetTypeRole 角色对象（静态展开组成员）。
	RemindTargetTypeRole = 2
)

// 催办命中类别文案（对位 Java ReminderSchedule 的 messageType 参数）。
const (
	// RemindKindAhead 预警提醒。
	RemindKindAhead = "预警提醒"
	// RemindKindDelay 延迟提醒。
	RemindKindDelay = "延迟提醒"
	// RemindKindManual 手动催办。
	RemindKindManual = "手动催办"
	// RemindKindTimeout 任务超时提醒（调度器超时提醒共用文案）。
	RemindKindTimeout = "任务超时提醒"
)

// ─── 实体 ─────────────────────────────────────────────

// Remind 是 workflow_remind（催办配置主表）的领域实体
// （不继承 BaseEntity，自带 creator/create_time/deleted，docs 003 §3.18）。
type Remind struct {
	ID         string          // 主键
	Creator    string          // 创建人
	CreateTime time.Time       // 创建时间
	Updater    string          // 更新人
	UpdateTime time.Time       // 更新时间
	TenantID   string          // 租户
	PID        string          // 模型 Id（→ workflow_model.id）
	StepIDs    string          // 步骤 Id 逗号串（白名单；空表示无可催步骤，Java 同款跳过）
	EnablePre  int             // 启用预警：0/1
	EnablePost int             // 启用延时：0/1
	Sent       int             // 已发送：0 未发送、1 已发送
	Deleted    int             // 逻辑删除：0 正常、1 已删除
	Targets    []*RemindTarget // 级联对象（Get 时填充）
}

// RemindTarget 是 workflow_remind_target（催办对象）的领域实体。
type RemindTarget struct {
	ID                 string    // 主键
	Creator            string    // 创建人
	CreateTime         time.Time // 创建时间
	TenantID           string    // 租户
	PID                string    // 催办配置 Id（→ workflow_remind.id）
	ExecutorID         string    // 执行者（用户 Id / 角色 Id / -1111 / ${initiator}）
	Enable             int       // 启用：0/1
	Name               string    // 对象名称
	TargetType         int       // 对象类型：1 用户、2 角色
	Type               int       // 催办类型：1 预警、2 延时
	Retry              int       // 是否重试：0/1
	RetryDuration      float64   // 重试间隔
	RetryTimes         int       // 重试次数上限
	RetryCount         int       // 已重试次数
	RetryTimeUnit      string    // 重试间隔单位
	Ahead              float64   // 提前量（预警型）
	Delay              float64   // 延迟量（延时型）
	TimeUnit           string    // 时间单位（天/小时/分钟）
	ByPlanCompleteTime int       // 按计划完成时间判定：0/1
	Removable          int       // 可移除标记：0/1
	Sent               int       // 已发送：0/1
	Deleted            int       // 逻辑删除：0/1
}

// ─── 存储端口 ─────────────────────────────────────────────

// RemindStore 是催办配置主表的存储端口。
type RemindStore interface {
	// Save 插入或更新催办配置。
	// @param ctx 操作上下文
	// @param remind 催办配置（必填 PID）
	// @return 错误
	Save(ctx context.Context, remind *Remind) error
	// Get 按主键查询催办配置（级联对象）。
	// @param ctx 操作上下文
	// @param id 催办配置 Id
	// @return 催办配置
	// @return 是否存在
	// @return 错误
	Get(ctx context.Context, id string) (*Remind, bool, error)
	// ListByModel 查询模型下的催办配置（deleted=0）。
	// @param ctx 操作上下文
	// @param modelID 模型 Id
	// @return 配置列表
	// @return 错误
	ListByModel(ctx context.Context, modelID string) ([]*Remind, error)
	// ListActive 查询全部未删除配置（手动催办数据源）。
	// @param ctx 操作上下文
	// @return 配置列表
	// @return 错误
	ListActive(ctx context.Context) ([]*Remind, error)
	// ListNotSent 查询未发送且未删除的配置（扫描数据源，Java getWorkflowRemindNotSent）。
	// @param ctx 操作上下文
	// @return 配置列表
	// @return 错误
	ListNotSent(ctx context.Context) ([]*Remind, error)
	// MarkSent 标记配置已发送。
	// @param ctx 操作上下文
	// @param id 催办配置 Id
	// @return 错误
	MarkSent(ctx context.Context, id string) error
	// MarkDeleted 按主键逻辑删除。
	// @param ctx 操作上下文
	// @param id 催办配置 Id
	// @return 错误
	MarkDeleted(ctx context.Context, id string) error
	// MarkDeletedByIDs 批量逻辑删除。
	// @param ctx 操作上下文
	// @param ids 催办配置 Id 列表
	// @return 错误
	MarkDeletedByIDs(ctx context.Context, ids []string) error
}

// RemindTargetStore 是催办对象表的存储端口。
type RemindTargetStore interface {
	// Save 插入催办对象。
	// @param ctx 操作上下文
	// @param target 催办对象（必填 PID）
	// @return 错误
	Save(ctx context.Context, target *RemindTarget) error
	// ListByRemind 查询配置的启用对象列表（enable=1 & deleted=0）。
	// @param ctx 操作上下文
	// @param remindID 催办配置 Id
	// @return 对象列表
	// @return 错误
	ListByRemind(ctx context.Context, remindID string) ([]*RemindTarget, error)
	// DeleteByRemind 删除配置的全部对象（配置重建时先删后增）。
	// @param ctx 操作上下文
	// @param remindID 催办配置 Id
	// @return 错误
	DeleteByRemind(ctx context.Context, remindID string) error
	// MarkSent 标记对象已发送。
	// @param ctx 操作上下文
	// @param id 对象 Id
	// @return 错误
	MarkSent(ctx context.Context, id string) error
	// UpdateRetryCount 回写重试计数。
	// @param ctx 操作上下文
	// @param id 对象 Id
	// @param count 新计数
	// @return 错误
	UpdateRetryCount(ctx context.Context, id string, count int) error
}

// TodoSource 是催办/调度扫描的待办数据源端口（SQL 模式由主控接裸 SQL 查询；
// 内存模式经 MemTodoSource 适配 MemMessageStore）。
type TodoSource interface {
	// ListAll 返回全部待办扩展。
	// @param ctx 操作上下文
	// @return 待办列表
	// @return 错误
	ListAll(ctx context.Context) ([]*MessageExtend, error)
	// GetByTaskID 按任务查询待办扩展。
	// @param ctx 操作上下文
	// @param taskID 任务 Id
	// @return 待办扩展
	// @return 是否存在
	// @return 错误
	GetByTaskID(ctx context.Context, taskID string) (*MessageExtend, bool, error)
}

// AutoCompleteRule 是自动办结步骤规则（his_step 规则字典投影）。
type AutoCompleteRule struct {
	AutoComplete int      // 自动办结类型：0 无、1 同意、2 退回
	TimeLimit    *float64 // 时限数值（nil 未配置）
	TimeUnit     string   // 时限单位（天/小时/分钟）
	AllowReturn  int      // 允许退回：0/1（autoComplete=2 时需为 1）
	BackStepCode string   // 退回目标步骤代号（back_step_codes 首个，退回型自动办结用）
}

// ScheduleRuleProvider 是步骤规则提供方端口（his_step 规则字典；P2-B steprules 完成后由主控接线，
// 默认空实现返回空规则，即自动办结/催办时限判定全部跳过）。
type ScheduleRuleProvider interface {
	// RemindLimitOf 返回步骤的催办时限与单位（对位 Java VO 的 remindTimeLimit/remindTimeLimitUnit）。
	// @param ctx 操作上下文
	// @param stepCode 步骤代号
	// @return 时限数值，未配置返回 nil
	// @return 单位
	// @return 错误
	RemindLimitOf(ctx context.Context, stepCode string) (*float64, string, error)
	// AutoCompleteRuleOf 返回步骤的自动办结规则。
	// @param ctx 操作上下文
	// @param stepCode 步骤代号
	// @return 自动办结规则
	// @return 错误
	AutoCompleteRuleOf(ctx context.Context, stepCode string) (AutoCompleteRule, error)
}

// NopScheduleRuleProvider 是步骤规则提供方的空实现（默认装配；规则数据接通前全部判空跳过）。
type NopScheduleRuleProvider struct{}

// RemindLimitOf 空实现：无时限配置。
// @param ctx 操作上下文
// @param stepCode 步骤代号
// @return 恒为 nil
// @return 空单位
// @return 恒为 nil
func (NopScheduleRuleProvider) RemindLimitOf(ctx context.Context, stepCode string) (*float64, string, error) {
	return nil, "", nil
}

// AutoCompleteRuleOf 空实现：无自动办结规则。
// @param ctx 操作上下文
// @param stepCode 步骤代号
// @return 零值规则
// @return 恒为 nil
func (NopScheduleRuleProvider) AutoCompleteRuleOf(ctx context.Context, stepCode string) (AutoCompleteRule, error) {
	return AutoCompleteRule{}, nil
}

// StepResolver 是步骤代号→设计时步骤 Id 的解析端口（催办白名单匹配用；
// P2-B steprules 完成后由主控接线，nil 时按代号直匹配白名单）。
type StepResolver interface {
	// ResolveStepID 按步骤代号解析设计时步骤 Id。
	// @param ctx 操作上下文
	// @param stepCode 步骤代号
	// @return 步骤 Id，未解析到返回空串
	// @return 错误
	ResolveStepID(ctx context.Context, stepCode string) (string, error)
}

// ChannelProvider 是催办渠道来源端口（P2-B steprules 的 his_step 渠道完成后由主控接线；
// 默认空实现返回空渠道）。
type ChannelProvider interface {
	// ChannelsOf 返回待办步骤的通知渠道列表。
	// @param ctx 操作上下文
	// @param todo 待办扩展
	// @return 渠道列表
	// @return 错误
	ChannelsOf(ctx context.Context, todo *MessageExtend) ([]string, error)
}

// NopChannelProvider 是催办渠道来源的空实现（默认装配）。
type NopChannelProvider struct{}

// ChannelsOf 空实现：无渠道。
// @param ctx 操作上下文
// @param todo 待办扩展
// @return 空渠道列表
// @return 恒为 nil
func (NopChannelProvider) ChannelsOf(ctx context.Context, todo *MessageExtend) ([]string, error) {
	return nil, nil
}

// RoleMemberLookup 是角色成员查询端口（静态角色展开，api.IdentityService 子集；
// 动态角色 PATH 取值本阶段留接口位：由主控在 P6 前接入取值范围 SQL 执行器）。
type RoleMemberLookup interface {
	// ListMembers 返回角色（组）成员用户列表。
	// @param ctx 操作上下文
	// @param groupID 角色/组 Id
	// @return 用户列表
	// @return 错误
	ListMembers(ctx context.Context, groupID string) ([]api.User, error)
}

// InstanceVariableSource 是流程变量读取端口（${initiator} 展开用，api.RuntimeService 子集）。
type InstanceVariableSource interface {
	// GetVariable 读取流程实例变量。
	// @param ctx 操作上下文
	// @param processInstanceID 实例 Id
	// @param name 变量名
	// @return 变量值
	// @return 查询错误
	GetVariable(ctx context.Context, processInstanceID, name string) (any, error)
}

// RemindNotification 是一次催办送达的报文（Notifier 消费）。
type RemindNotification struct {
	ReceiverID   string   // 接收人 Id
	ReceiverName string   // 接收人姓名
	Subject      string   // 主题（=待办标题）
	Content      string   // 内容（Java sendToUser 同款格式）
	Channels     []string // 渠道列表（渠道提供方注入）
	Kind         string   // 类别（预警提醒/延迟提醒/任务超时提醒/手动催办）
}

// Notifier 是催办送达器端口（默认日志实现；P6 接真实消息渠道）。
type Notifier interface {
	// Notify 送达一条催办通知。
	// @param ctx 操作上下文
	// @param notification 通知报文
	// @return 错误
	Notify(ctx context.Context, notification *RemindNotification) error
}

// LogNotifier 是送达器的日志实现（默认装配；仅打印不外发）。
type LogNotifier struct {
	Logger *log.Logger // 日志输出（nil 时输出 stderr）
}

// Notify 打印催办通知。
// @param ctx 操作上下文
// @param notification 通知报文
// @return 恒为 nil
func (n LogNotifier) Notify(ctx context.Context, notification *RemindNotification) error {
	logger := n.Logger
	// 未注入日志器时回退 stderr
	if logger == nil {
		logger = log.New(os.Stderr, "workflow-remind: ", log.LstdFlags)
	}
	if notification != nil {
		logger.Printf("[%s] -> %s: %s", notification.Kind, notification.ReceiverID, notification.Content)
	}
	return nil
}

// ─── 内存实现 ─────────────────────────────────────────────

// MemRemindStore 是催办配置端口的内存实现（线程安全 map + 行克隆存取）。
type MemRemindStore struct {
	mu    sync.RWMutex       // 读写锁
	rows  map[string]*Remind // id → 配置
	clock clock.Clock        // 时间源
	newID func() string      // ID 生成函数
}

// NewMemRemindStore 创建内存催办配置存储。
// @param c 时间源，nil 时回退真实时钟
// @return 内存存储
func NewMemRemindStore(c clock.Clock) *MemRemindStore {
	if c == nil {
		c = clock.Real{}
	}
	return &MemRemindStore{rows: make(map[string]*Remind), clock: c, newID: DefaultIDGen()}
}

// Save 插入或更新催办配置（ID 缺失自动生成）。
// @param ctx 操作上下文
// @param remind 催办配置
// @return 错误
func (m *MemRemindStore) Save(ctx context.Context, remind *Remind) error {
	// 模型归属是催办配置的最小定位信息
	if remind == nil || remind.PID == "" {
		return apiError("WORKFLOW-1801", "催办配置缺少模型 Id")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if remind.ID == "" {
		remind.ID = m.newID()
	}
	if remind.CreateTime.IsZero() {
		remind.CreateTime = m.clock.Now()
	}
	remind.UpdateTime = m.clock.Now()
	m.rows[remind.ID] = cloneRemind(remind)
	return nil
}

// Get 按主键查询催办配置。
// @param ctx 操作上下文
// @param id 催办配置 Id
// @return 催办配置
// @return 是否存在
// @return 错误
func (m *MemRemindStore) Get(ctx context.Context, id string) (*Remind, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	row, ok := m.rows[id]
	if !ok {
		return nil, false, nil
	}
	return cloneRemind(row), true, nil
}

// listLocked 按谓词过滤并按创建时间升序返回（调用方须持锁）。
// @param match 过滤谓词
// @return 配置列表
func (m *MemRemindStore) listLocked(match func(*Remind) bool) []*Remind {
	out := make([]*Remind, 0, 4)
	for _, row := range m.rows {
		if match(row) {
			out = append(out, cloneRemind(row))
		}
	}
	sortReminds(out)
	return out
}

// ListByModel 查询模型下的催办配置（deleted=0）。
// @param ctx 操作上下文
// @param modelID 模型 Id
// @return 配置列表
// @return 错误
func (m *MemRemindStore) ListByModel(ctx context.Context, modelID string) ([]*Remind, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.listLocked(func(r *Remind) bool { return r.PID == modelID && r.Deleted == 0 }), nil
}

// ListActive 查询全部未删除配置。
// @param ctx 操作上下文
// @return 配置列表
// @return 错误
func (m *MemRemindStore) ListActive(ctx context.Context) ([]*Remind, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.listLocked(func(r *Remind) bool { return r.Deleted == 0 }), nil
}

// ListNotSent 查询未发送且未删除的配置。
// @param ctx 操作上下文
// @return 配置列表
// @return 错误
func (m *MemRemindStore) ListNotSent(ctx context.Context) ([]*Remind, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.listLocked(func(r *Remind) bool { return r.Deleted == 0 && r.Sent == 0 }), nil
}

// MarkSent 标记配置已发送。
// @param ctx 操作上下文
// @param id 催办配置 Id
// @return 错误
func (m *MemRemindStore) MarkSent(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if row, ok := m.rows[id]; ok {
		row.Sent = 1
		row.UpdateTime = m.clock.Now()
	}
	return nil
}

// MarkDeleted 按主键逻辑删除。
// @param ctx 操作上下文
// @param id 催办配置 Id
// @return 错误
func (m *MemRemindStore) MarkDeleted(ctx context.Context, id string) error {
	return m.MarkDeletedByIDs(ctx, []string{id})
}

// MarkDeletedByIDs 批量逻辑删除。
// @param ctx 操作上下文
// @param ids 催办配置 Id 列表
// @return 错误
func (m *MemRemindStore) MarkDeletedByIDs(ctx context.Context, ids []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range ids {
		if row, ok := m.rows[id]; ok {
			row.Deleted = 1
			row.UpdateTime = m.clock.Now()
		}
	}
	return nil
}

// 编译期断言：内存实现满足端口。
var _ RemindStore = (*MemRemindStore)(nil)

// cloneRemind 复制催办配置（含级联对象）。
// @param r 原配置
// @return 副本
func cloneRemind(r *Remind) *Remind {
	if r == nil {
		return nil
	}
	copied := *r
	copied.Targets = make([]*RemindTarget, len(r.Targets))
	for i, target := range r.Targets {
		targetCopy := *target
		copied.Targets[i] = &targetCopy
	}
	return &copied
}

// sortReminds 按创建时间升序排序配置。
// @param rows 配置列表
func sortReminds(rows []*Remind) {
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && rows[j].CreateTime.Before(rows[j-1].CreateTime); j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
}

// MemRemindTargetStore 是催办对象端口的内存实现。
type MemRemindTargetStore struct {
	mu    sync.RWMutex             // 读写锁
	rows  map[string]*RemindTarget // id → 对象
	clock clock.Clock              // 时间源
	newID func() string            // ID 生成函数
}

// NewMemRemindTargetStore 创建内存催办对象存储。
// @param c 时间源，nil 时回退真实时钟
// @return 内存存储
func NewMemRemindTargetStore(c clock.Clock) *MemRemindTargetStore {
	if c == nil {
		c = clock.Real{}
	}
	return &MemRemindTargetStore{rows: make(map[string]*RemindTarget), clock: c, newID: DefaultIDGen()}
}

// Save 插入催办对象（ID 缺失自动生成）。
// @param ctx 操作上下文
// @param target 催办对象
// @return 错误
func (m *MemRemindTargetStore) Save(ctx context.Context, target *RemindTarget) error {
	// 配置归属是对象行的最小定位信息
	if target == nil || target.PID == "" {
		return apiError("WORKFLOW-1802", "催办对象缺少配置 Id")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if target.ID == "" {
		target.ID = m.newID()
	}
	if target.CreateTime.IsZero() {
		target.CreateTime = m.clock.Now()
	}
	m.rows[target.ID] = cloneRemindTarget(target)
	return nil
}

// ListByRemind 查询配置的启用对象列表（enable=1 & deleted=0，按创建时间升序）。
// @param ctx 操作上下文
// @param remindID 催办配置 Id
// @return 对象列表
// @return 错误
func (m *MemRemindTargetStore) ListByRemind(ctx context.Context, remindID string) ([]*RemindTarget, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*RemindTarget, 0, 4)
	for _, row := range m.rows {
		// 与 Java 查询一致：pid + enable=true + deleted=false
		if row.PID == remindID && row.Enable == 1 && row.Deleted == 0 {
			out = append(out, cloneRemindTarget(row))
		}
	}
	sortRemindTargets(out)
	return out, nil
}

// DeleteByRemind 删除配置的全部对象。
// @param ctx 操作上下文
// @param remindID 催办配置 Id
// @return 错误
func (m *MemRemindTargetStore) DeleteByRemind(ctx context.Context, remindID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, row := range m.rows {
		if row.PID == remindID {
			delete(m.rows, id)
		}
	}
	return nil
}

// MarkSent 标记对象已发送。
// @param ctx 操作上下文
// @param id 对象 Id
// @return 错误
func (m *MemRemindTargetStore) MarkSent(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if row, ok := m.rows[id]; ok {
		row.Sent = 1
	}
	return nil
}

// UpdateRetryCount 回写重试计数。
// @param ctx 操作上下文
// @param id 对象 Id
// @param count 新计数
// @return 错误
func (m *MemRemindTargetStore) UpdateRetryCount(ctx context.Context, id string, count int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if row, ok := m.rows[id]; ok {
		row.RetryCount = count
	}
	return nil
}

// 编译期断言：内存实现满足端口。
var _ RemindTargetStore = (*MemRemindTargetStore)(nil)

// cloneRemindTarget 复制催办对象。
// @param t 原对象
// @return 副本
func cloneRemindTarget(t *RemindTarget) *RemindTarget {
	if t == nil {
		return nil
	}
	copied := *t
	return &copied
}

// sortRemindTargets 按创建时间升序排序对象。
// @param rows 对象列表
func sortRemindTargets(rows []*RemindTarget) {
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && rows[j].CreateTime.Before(rows[j-1].CreateTime); j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
}

// MemTodoSource 是待办数据源端口的内存适配（包装 MemMessageStore，同包读取内部行）。
type MemTodoSource struct {
	store *MemMessageStore // 待办内存存储
}

// NewMemTodoSource 创建内存待办数据源。
// @param store 待办内存存储
// @return 待办数据源
func NewMemTodoSource(store *MemMessageStore) *MemTodoSource {
	return &MemTodoSource{store: store}
}

// ListAll 返回全部待办扩展（按创建时间升序）。
// @param ctx 操作上下文
// @return 待办列表
// @return 错误
func (s *MemTodoSource) ListAll(ctx context.Context) ([]*MessageExtend, error) {
	s.store.mu.RLock()
	defer s.store.mu.RUnlock()
	out := make([]*MessageExtend, 0, len(s.store.msg))
	for _, row := range s.store.msg {
		out = append(out, cloneMessage(row))
	}
	// 按创建时间升序，保证扫描顺序确定
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].CreateTime.Before(out[j-1].CreateTime); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, nil
}

// GetByTaskID 按任务查询待办扩展。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @return 待办扩展
// @return 是否存在
// @return 错误
func (s *MemTodoSource) GetByTaskID(ctx context.Context, taskID string) (*MessageExtend, bool, error) {
	return s.store.GetByTaskID(ctx, taskID)
}

// 编译期断言：内存适配满足端口。
var _ TodoSource = (*MemTodoSource)(nil)

// ─── SQL 适配 ─────────────────────────────────────────────

// SQLRemindStore 是催办配置端口的 SQL 适配器（组合 pwf 仓储）。
type SQLRemindStore struct {
	repos *pwf.Repositories // workflow 行仓储聚合
	clock clock.Clock       // 时间源
}

// NewSQLRemindStore 创建催办配置 SQL 适配器。
// @param store workflow 仓储入口
// @param c 时间源（审计列填充），nil 回退真实时钟
// @return SQL 适配器
func NewSQLRemindStore(store *pwf.Store, c clock.Clock) *SQLRemindStore {
	if c == nil {
		c = clock.Real{}
	}
	return &SQLRemindStore{repos: pwf.NewRepositories(store), clock: c}
}

// Save 插入催办配置（存在则更新主键行）。
// @param ctx 操作上下文
// @param remind 催办配置
// @return 错误
func (s *SQLRemindStore) Save(ctx context.Context, remind *Remind) error {
	if remind == nil || remind.PID == "" {
		return apiError("WORKFLOW-1801", "催办配置缺少模型 Id")
	}
	row := remindToRow(remind)
	affected, err := s.repos.Reminds.UpdateByPK(ctx, remind.ID, row)
	// 更新命中即完成（0 行视为新增场景）
	if err == nil && affected > 0 {
		return nil
	}
	return s.repos.Reminds.Insert(ctx, row)
}

// Get 按主键查询催办配置（级联对象）。
// @param ctx 操作上下文
// @param id 催办配置 Id
// @return 催办配置
// @return 是否存在
// @return 错误
func (s *SQLRemindStore) Get(ctx context.Context, id string) (*Remind, bool, error) {
	row, found, err := s.repos.Reminds.GetByPK(ctx, id)
	if err != nil || !found {
		return nil, false, err
	}
	remind := remindFromRow(row)
	// 级联装载对象（deleted=0 全量，Get 面向管理台展示）
	targetRows, err := s.repos.RemindTargets.ListByRemind(ctx, id)
	if err != nil {
		return nil, false, err
	}
	for _, targetRow := range targetRows {
		remind.Targets = append(remind.Targets, remindTargetFromRow(targetRow))
	}
	return remind, true, nil
}

// ListByModel 查询模型下的催办配置。
// @param ctx 操作上下文
// @param modelID 模型 Id
// @return 配置列表
// @return 错误
func (s *SQLRemindStore) ListByModel(ctx context.Context, modelID string) ([]*Remind, error) {
	rows, err := s.repos.Reminds.ListByModel(ctx, modelID)
	return remindsFromRows(rows), err
}

// ListActive 查询全部未删除配置（仓储无全量查询，退化为按模型逐个扫描不可行——
// 这里经裸 SQL 缺口由主控接线，SQL 适配返回按租户过滤结果为空时与内存语义一致）。
// @param ctx 操作上下文
// @return 配置列表
// @return 错误
func (s *SQLRemindStore) ListActive(ctx context.Context) ([]*Remind, error) {
	// 行仓储按模型维度查询；全量扫描交由主控按租户的模型列表展开（P6 接线），
	// 当前实现返回空集保证接口可用
	return []*Remind{}, nil
}

// ListNotSent 查询未发送且未删除的配置。
// @param ctx 操作上下文
// @return 配置列表
// @return 错误
func (s *SQLRemindStore) ListNotSent(ctx context.Context) ([]*Remind, error) {
	// 同 ListActive：全量扫描由主控接线；行仓储的 deleted=0 过滤在 ListByModel 内完成
	return []*Remind{}, nil
}

// MarkSent 标记配置已发送。
// @param ctx 操作上下文
// @param id 催办配置 Id
// @return 错误
func (s *SQLRemindStore) MarkSent(ctx context.Context, id string) error {
	_, err := s.repos.Reminds.MarkSent(ctx, id)
	return err
}

// MarkDeleted 按主键逻辑删除。
// @param ctx 操作上下文
// @param id 催办配置 Id
// @return 错误
func (s *SQLRemindStore) MarkDeleted(ctx context.Context, id string) error {
	return s.MarkDeletedByIDs(ctx, []string{id})
}

// MarkDeletedByIDs 批量逻辑删除。
// @param ctx 操作上下文
// @param ids 催办配置 Id 列表
// @return 错误
func (s *SQLRemindStore) MarkDeletedByIDs(ctx context.Context, ids []string) error {
	for _, id := range ids {
		if _, err := s.repos.Reminds.UpdateByPK(ctx, id, pwf.Row{"deleted": 1, "update_time": s.clock.Now()}); err != nil {
			return err
		}
	}
	return nil
}

// 编译期断言：SQL 适配满足端口。
var _ RemindStore = (*SQLRemindStore)(nil)

// remindToRow 实体转通用行。
// @param remind 催办配置
// @return 通用行
func remindToRow(remind *Remind) pwf.Row {
	row := pwf.Row{
		"id":          remind.ID,
		"pid":         remind.PID,
		"step_ids":    remind.StepIDs,
		"enable_pre":  remind.EnablePre,
		"enable_post": remind.EnablePost,
		"sent":        remind.Sent,
		"deleted":     remind.Deleted,
		"creator":     remind.Creator,
		"updater":     remind.Updater,
		"tenant_id":   remind.TenantID,
	}
	if !remind.CreateTime.IsZero() {
		row["create_time"] = remind.CreateTime
	}
	if !remind.UpdateTime.IsZero() {
		row["update_time"] = remind.UpdateTime
	}
	return row
}

// remindFromRow 通用行转实体。
// @param row 通用行
// @return 催办配置
func remindFromRow(row pwf.Row) *Remind {
	return &Remind{
		ID:         rowString(row, "id"),
		Creator:    rowString(row, "creator"),
		CreateTime: timeOrZero(row, "create_time"),
		Updater:    rowString(row, "updater"),
		UpdateTime: timeOrZero(row, "update_time"),
		TenantID:   rowString(row, "tenant_id"),
		PID:        rowString(row, "pid"),
		StepIDs:    rowString(row, "step_ids"),
		EnablePre:  rowInt(row, "enable_pre"),
		EnablePost: rowInt(row, "enable_post"),
		Sent:       rowInt(row, "sent"),
		Deleted:    rowInt(row, "deleted"),
	}
}

// remindsFromRows 行列表转实体列表。
// @param rows 通用行列表
// @return 配置列表
func remindsFromRows(rows []pwf.Row) []*Remind {
	out := make([]*Remind, 0, len(rows))
	for _, row := range rows {
		out = append(out, remindFromRow(row))
	}
	return out
}

// SQLRemindTargetStore 是催办对象端口的 SQL 适配器。
type SQLRemindTargetStore struct {
	repos *pwf.Repositories // workflow 行仓储聚合
}

// NewSQLRemindTargetStore 创建催办对象 SQL 适配器。
// @param store workflow 仓储入口
// @return SQL 适配器
func NewSQLRemindTargetStore(store *pwf.Store) *SQLRemindTargetStore {
	return &SQLRemindTargetStore{repos: pwf.NewRepositories(store)}
}

// Save 插入催办对象（存在则更新主键行）。
// @param ctx 操作上下文
// @param target 催办对象
// @return 错误
func (s *SQLRemindTargetStore) Save(ctx context.Context, target *RemindTarget) error {
	if target == nil || target.PID == "" {
		return apiError("WORKFLOW-1802", "催办对象缺少配置 Id")
	}
	row := remindTargetToRow(target)
	affected, err := s.repos.RemindTargets.UpdateByPK(ctx, target.ID, row)
	// 更新命中即完成（0 行视为新增场景）
	if err == nil && affected > 0 {
		return nil
	}
	return s.repos.RemindTargets.Insert(ctx, row)
}

// ListByRemind 查询配置的启用对象列表。
// @param ctx 操作上下文
// @param remindID 催办配置 Id
// @return 对象列表
// @return 错误
func (s *SQLRemindTargetStore) ListByRemind(ctx context.Context, remindID string) ([]*RemindTarget, error) {
	rows, err := s.repos.RemindTargets.ListByRemind(ctx, remindID)
	if err != nil {
		return nil, err
	}
	out := make([]*RemindTarget, 0, len(rows))
	for _, row := range rows {
		// 行仓储仅过滤 deleted，enable 条件在内存补齐（与 Java 查询一致）
		if rowInt(row, "enable") == 1 {
			out = append(out, remindTargetFromRow(row))
		}
	}
	return out, nil
}

// DeleteByRemind 删除配置的全部对象。
// @param ctx 操作上下文
// @param remindID 催办配置 Id
// @return 错误
func (s *SQLRemindTargetStore) DeleteByRemind(ctx context.Context, remindID string) error {
	_, err := s.repos.RemindTargets.DeleteByRemind(ctx, remindID)
	return err
}

// MarkSent 标记对象已发送。
// @param ctx 操作上下文
// @param id 对象 Id
// @return 错误
func (s *SQLRemindTargetStore) MarkSent(ctx context.Context, id string) error {
	_, err := s.repos.RemindTargets.MarkSent(ctx, id)
	return err
}

// UpdateRetryCount 回写重试计数。
// @param ctx 操作上下文
// @param id 对象 Id
// @param count 新计数
// @return 错误
func (s *SQLRemindTargetStore) UpdateRetryCount(ctx context.Context, id string, count int) error {
	_, err := s.repos.RemindTargets.UpdateByPK(ctx, id, pwf.Row{"retry_count": count})
	return err
}

// 编译期断言：SQL 适配满足端口。
var _ RemindTargetStore = (*SQLRemindTargetStore)(nil)

// remindTargetToRow 实体转通用行。
// @param target 催办对象
// @return 通用行
func remindTargetToRow(target *RemindTarget) pwf.Row {
	row := pwf.Row{
		"id":                    target.ID,
		"pid":                   target.PID,
		"executor_id":           target.ExecutorID,
		"enable":                target.Enable,
		"name":                  target.Name,
		"target_type":           target.TargetType,
		"type":                  target.Type,
		"retry":                 target.Retry,
		"retry_duration":        target.RetryDuration,
		"retry_times":           target.RetryTimes,
		"retry_count":           target.RetryCount,
		"retry_time_unit":       target.RetryTimeUnit,
		"ahead":                 target.Ahead,
		"delay":                 target.Delay,
		"by_plan_complete_time": target.ByPlanCompleteTime,
		"removable":             target.Removable,
		"sent":                  target.Sent,
		"deleted":               target.Deleted,
		"creator":               target.Creator,
		"tenant_id":             target.TenantID,
	}
	if !target.CreateTime.IsZero() {
		row["create_time"] = target.CreateTime
	}
	return row
}

// remindTargetFromRow 通用行转实体。
// @param row 通用行
// @return 催办对象
func remindTargetFromRow(row pwf.Row) *RemindTarget {
	return &RemindTarget{
		ID:                 rowString(row, "id"),
		Creator:            rowString(row, "creator"),
		CreateTime:         timeOrZero(row, "create_time"),
		TenantID:           rowString(row, "tenant_id"),
		PID:                rowString(row, "pid"),
		ExecutorID:         rowString(row, "executor_id"),
		Enable:             rowInt(row, "enable"),
		Name:               rowString(row, "name"),
		TargetType:         rowInt(row, "target_type"),
		Type:               rowInt(row, "type"),
		Retry:              rowInt(row, "retry"),
		RetryDuration:      rowFloat(row, "retry_duration"),
		RetryTimes:         rowInt(row, "retry_times"),
		RetryCount:         rowInt(row, "retry_count"),
		RetryTimeUnit:      rowString(row, "retry_time_unit"),
		Ahead:              rowFloat(row, "ahead"),
		Delay:              rowFloat(row, "delay"),
		TimeUnit:           rowString(row, "time_unit"),
		ByPlanCompleteTime: rowInt(row, "by_plan_complete_time"),
		Removable:          rowInt(row, "removable"),
		Sent:               rowInt(row, "sent"),
		Deleted:            rowInt(row, "deleted"),
	}
}

// rowFloat 读取行为浮点值（nil → 0，兼容 DECIMAL 字符串）。
// @param row 通用行
// @param key 列名
// @return 浮点值
func rowFloat(row pwf.Row, key string) float64 {
	if value := rowFloatPtr(row, key); value != nil {
		return *value
	}
	return 0
}

// ─── 域服务 ─────────────────────────────────────────────

// RemindHit 是一次催办命中（判定结果，供调度器送达与标记）。
type RemindHit struct {
	Remind   *Remind        // 催办配置
	Target   *RemindTarget  // 命中的催办对象
	Todo     *MessageExtend // 命中的待办
	Channels []string       // 渠道（渠道提供方注入）
	Kind     string         // 类别（预警提醒/延迟提醒/手动催办）
	Content  string         // 建议内容（Java sendToUser 同款格式）
}

// RemindInstanceContext 是催办对象展开的实例上下文。
type RemindInstanceContext struct {
	Todo      *MessageExtend // 当前待办（-1111 展开为当前待办人）
	Initiator string         // 发起人（${initiator} 展开）
}

// RemindService 是催办域服务：配置维护、对象展开、到期扫描与手动催办。
type RemindService struct {
	reminds  RemindStore            // 配置主表
	targets  RemindTargetStore      // 对象表
	todos    TodoSource             // 待办数据源
	rules    ScheduleRuleProvider   // 步骤规则（催办时限）
	channels ChannelProvider        // 渠道来源
	resolver StepResolver           // 步骤代号解析（白名单匹配）
	members  RoleMemberLookup       // 角色成员（静态角色展开）
	vars     InstanceVariableSource // 流程变量（${initiator} 展开）
	clock    clock.Clock            // 时间源
	newID    func() string          // ID 生成函数
}

// NewRemindService 创建催办域服务。
// @param reminds 催办配置存储
// @param targets 催办对象存储
// @param todos 待办数据源
// @param rules 步骤规则提供方，nil 时使用空实现
// @param channels 渠道提供方，nil 时使用空实现
// @param resolver 步骤代号解析（可 nil，按代号直匹配）
// @param members 角色成员查询（可 nil，角色对象展开为空）
// @param vars 流程变量读取（可 nil，${initiator} 展开为空）
// @param c 时间源，nil 时回退真实时钟
// @return 域服务
func NewRemindService(reminds RemindStore, targets RemindTargetStore, todos TodoSource, rules ScheduleRuleProvider, channels ChannelProvider, resolver StepResolver, members RoleMemberLookup, vars InstanceVariableSource, c clock.Clock) *RemindService {
	if c == nil {
		c = clock.Real{}
	}
	// 规则/渠道缺省空实现，保证判空分支不空指针
	if rules == nil {
		rules = NopScheduleRuleProvider{}
	}
	if channels == nil {
		channels = NopChannelProvider{}
	}
	return &RemindService{
		reminds: reminds, targets: targets, todos: todos, rules: rules, channels: channels,
		resolver: resolver, members: members, vars: vars, clock: c, newID: DefaultIDGen(),
	}
}

// RemindSaveRequest 是催办配置保存请求（Add/Update 共用）。
type RemindSaveRequest struct {
	Remind         *Remind         // 主表数据（必填 PID）
	Targets        []*RemindTarget // 对象列表；Add 时为空则生成默认"当前处理人"（-1111）预警行
	ReplaceStepIDs bool            // Update 时白名单允许清空（true 表示用 Remind.StepIDs 覆盖，含清空）
}

// Add 新增催办配置（pid + 步骤白名单 + 对象列表；对象缺省生成 -1111 当前处理人预警行）。
// @param ctx 操作上下文
// @param req 保存请求
// @return 落库后的催办配置
// @return 错误
func (s *RemindService) Add(ctx context.Context, req RemindSaveRequest) (*Remind, error) {
	remind := req.Remind
	// 模型归属必填（pid → workflow_model.id）
	if remind == nil || remind.PID == "" {
		return nil, apiError("WORKFLOW-1801", "催办配置缺少模型 Id")
	}
	remind.Deleted = 0
	fillRemindAudit(remind, s.clock.Now, firstNonEmpty(remind.Creator, remind.TenantID))
	targets := req.Targets
	// 对象缺省：生成"当前处理人"（-1111）预警行（docs 003：默认当前处理人 executorId=-1111）
	if len(targets) == 0 {
		targets = []*RemindTarget{{
			ExecutorID: RemindExecutorCurrentHandler,
			Name:       "当前处理人",
			TargetType: RemindTargetTypeUser,
			Type:       RemindTypeAhead,
			Enable:     1,
		}}
	}
	if err := s.reminds.Save(ctx, remind); err != nil {
		return nil, err
	}
	if err := s.saveTargets(ctx, remind, targets); err != nil {
		return nil, err
	}
	remind.Targets = targets
	return remind, nil
}

// Update 更新催办配置（主表更新 + 对象先删后增）。
// @param ctx 操作上下文
// @param req 保存请求（Remind.ID 必填）
// @return 更新后的催办配置
// @return 错误
func (s *RemindService) Update(ctx context.Context, req RemindSaveRequest) (*Remind, error) {
	remind := req.Remind
	if remind == nil || remind.ID == "" {
		return nil, apiError("WORKFLOW-1801", "催办配置或配置 Id 为空")
	}
	existing, found, err := s.reminds.Get(ctx, remind.ID)
	if err != nil {
		return nil, err
	}
	// 配置已删除/不存在时拒绝更新
	if !found {
		return nil, apiError("WORKFLOW-1803", "催办配置不存在")
	}
	// 非空字段覆盖（与委托 Update 同款 ignoreNull 语义）
	if remind.PID != "" {
		existing.PID = remind.PID
	}
	if remind.StepIDs != "" || req.ReplaceStepIDs {
		existing.StepIDs = remind.StepIDs
	}
	existing.EnablePre = remind.EnablePre
	existing.EnablePost = remind.EnablePost
	existing.Updater = firstNonEmpty(remind.Updater, existing.Updater)
	existing.UpdateTime = s.clock.Now()
	if err := s.reminds.Save(ctx, existing); err != nil {
		return nil, err
	}
	// 对象列表非空时重建（先删后增）
	if len(req.Targets) > 0 {
		if err := s.targets.DeleteByRemind(ctx, existing.ID); err != nil {
			return nil, err
		}
		if err := s.saveTargets(ctx, existing, req.Targets); err != nil {
			return nil, err
		}
		existing.Targets = req.Targets
	}
	return existing, nil
}

// Get 查询催办配置（主表 + 启用对象级联，服务层聚合两表）。
// @param ctx 操作上下文
// @param id 配置 Id
// @return 催办配置
// @return 是否存在
// @return 错误
func (s *RemindService) Get(ctx context.Context, id string) (*Remind, bool, error) {
	remind, found, err := s.reminds.Get(ctx, id)
	if err != nil || !found {
		return remind, found, err
	}
	// 对象列表按启用态装载（与扫描口径一致）
	targets, err := s.targets.ListByRemind(ctx, id)
	if err != nil {
		return remind, found, err
	}
	remind.Targets = targets
	return remind, found, nil
}

// List 按模型分页查询催办配置（deleted=0）。
// @param ctx 操作上下文
// @param modelID 模型 Id
// @param page 分页参数
// @return 分页结果
// @return 错误
func (s *RemindService) List(ctx context.Context, modelID string, page PageQuery) (PageResult[*Remind], error) {
	rows, err := s.reminds.ListByModel(ctx, modelID)
	if err != nil {
		return PageResult[*Remind]{}, err
	}
	// 内存分页（ListByModel 单模型配置量为个位数，无需下推 SQL）
	start, size := page.Start, page.Size
	if start < 0 {
		start = 0
	}
	if size <= 0 || start >= len(rows) {
		return PageResult[*Remind]{Data: []*Remind{}, Total: int64(len(rows)), Start: start, Size: size}, nil
	}
	end := start + size
	// 尾页截断
	if end > len(rows) {
		end = len(rows)
	}
	return PageResult[*Remind]{Data: rows[start:end], Total: int64(len(rows)), Start: start, Size: size}, nil
}

// Remove 按主键逻辑删除催办配置。
// @param ctx 操作上下文
// @param id 配置 Id
// @return 错误
func (s *RemindService) Remove(ctx context.Context, id string) error {
	return s.reminds.MarkDeleted(ctx, id)
}

// RemoveBatch 批量逻辑删除催办配置。
// @param ctx 操作上下文
// @param ids 配置 Id 列表
// @return 错误
func (s *RemindService) RemoveBatch(ctx context.Context, ids []string) error {
	return s.reminds.MarkDeletedByIDs(ctx, ids)
}

// saveTargets 保存对象列表（补 PID/租户/审计列与启用态缺省）。
// @param ctx 操作上下文
// @param remind 催办配置
// @param targets 对象列表
// @return 错误
func (s *RemindService) saveTargets(ctx context.Context, remind *Remind, targets []*RemindTarget) error {
	for _, target := range targets {
		target.PID = remind.ID
		// 对象租户/创建信息继承主配置
		if target.TenantID == "" {
			target.TenantID = remind.TenantID
		}
		if target.Creator == "" {
			target.Creator = remind.Creator
		}
		if target.CreateTime.IsZero() {
			target.CreateTime = s.clock.Now()
		}
		// 启用态缺省启用（Java 查询固定 enable=true）
		if target.Enable == 0 {
			target.Enable = 1
		}
		if err := s.targets.Save(ctx, target); err != nil {
			return err
		}
	}
	return nil
}

// fillRemindAudit 填充催办配置审计列（ID/创建时间缺省填充）。
// @param remind 催办配置
// @param now 当前时间函数
// @param creator 创建人
func fillRemindAudit(remind *Remind, now func() time.Time, creator string) {
	if remind.ID == "" {
		remind.ID = DefaultIDGen()()
	}
	if remind.CreateTime.IsZero() {
		remind.CreateTime = now()
	}
	if remind.UpdateTime.IsZero() {
		remind.UpdateTime = now()
	}
	if creator != "" && remind.Creator == "" {
		remind.Creator = creator
	}
}

// ExpandTargets 展开催办对象为具体用户 Id 列表（对位 Java updateRoleRemind:268 的执行者解析段）：
// -1111 → 当前待办人；${initiator} → 流程发起人；角色对象 → 静态组成员；其余按字面用户。
// 动态角色 PATH 取值本阶段留接口位（RoleMemberLookup 注释与报告说明，主控后续接线）。
// @param ctx 操作上下文
// @param target 催办对象
// @param instance 实例上下文（当前待办 + 发起人）
// @return 具体用户 Id 列表（保持展开顺序，去重）
func (s *RemindService) ExpandTargets(ctx context.Context, target *RemindTarget, instance *RemindInstanceContext) []string {
	// 对象缺失或禁用不展开
	if target == nil || target.Enable != 1 {
		return nil
	}
	switch {
	case target.ExecutorID == RemindExecutorCurrentHandler:
		// 当前待办人（-1111）
		if instance != nil && instance.Todo != nil && instance.Todo.OwnerID != "" {
			return []string{instance.Todo.OwnerID}
		}
		return nil
	case target.ExecutorID == RemindExecutorInitiator:
		// 流程发起人（${initiator}）
		if instance != nil && instance.Initiator != "" {
			return []string{instance.Initiator}
		}
		return nil
	case target.TargetType == RemindTargetTypeRole:
		// 静态角色：展开组成员；动态角色 PATH 由主控接入取值范围执行器后在此分流
		return s.expandRoleMembers(ctx, target.ExecutorID)
	default:
		// 字面用户 Id
		return []string{target.ExecutorID}
	}
}

// expandRoleMembers 展开角色成员（api.IdentityService.ListMembers 等价端口）。
// @param ctx 操作上下文
// @param roleID 角色 Id
// @return 成员用户 Id 列表
func (s *RemindService) expandRoleMembers(ctx context.Context, roleID string) []string {
	// 角色成员查询未注入时无法展开
	if s.members == nil {
		return nil
	}
	users, err := s.members.ListMembers(ctx, roleID)
	if err != nil {
		// 成员查询失败按空展开处理，不阻断其他对象扫描
		return nil
	}
	ids := make([]string, 0, len(users))
	for _, user := range users {
		ids = append(ids, user.ID)
	}
	// 无成员返回 nil（调用方以 nil/空统一判空）
	if len(ids) == 0 {
		return nil
	}
	return ids
}

// ScanDue 扫描到期催办（对位 Java ReminderSchedule#updateWithCheckWorkflowReminds:92 判定段）：
// 预警型 dueTime-duration<=now（或计划完成时间）；延时型 dueTime+duration<=now 或按任务接收时间；
// 步骤白名单过滤（白名单为空跳过，Java 同款）；已发送对象按重试配置门槛过滤。
// @param ctx 操作上下文
// @param now 当前时间
// @return 命中列表
// @return 错误
func (s *RemindService) ScanDue(ctx context.Context, now time.Time) ([]*RemindHit, error) {
	reminds, err := s.reminds.ListNotSent(ctx)
	if err != nil {
		return nil, err
	}
	hits := make([]*RemindHit, 0, 4)
	todos, err := s.todos.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	for _, remind := range reminds {
		targets, err := s.targets.ListByRemind(ctx, remind.ID)
		if err != nil {
			return nil, err
		}
		for _, target := range targets {
			for _, todo := range todos {
				// 步骤白名单过滤（白名单为空或不在名单内均跳过，Java 同款）
				if !s.inWhitelist(ctx, remind, todo) {
					continue
				}
				// 已发送对象：未启用重试或重试次数用尽不再发
				if target.Sent == 1 && !s.retryAllowed(target) {
					continue
				}
				kind := s.judgeDue(ctx, target, todo, now)
				// 未到期跳过
				if kind == "" {
					continue
				}
				channels, err := s.channels.ChannelsOf(ctx, todo)
				if err != nil {
					return nil, err
				}
				hits = append(hits, &RemindHit{
					Remind:   remind,
					Target:   cloneRemindTarget(target),
					Todo:     todo,
					Channels: channels,
					Kind:     kind,
					Content:  remindContent(todo, kind),
				})
			}
		}
	}
	return hits, nil
}

// SendNow 手动催办（查待办 → 命中该任务步骤的催办配置 → 即时产生命中，不落发送标记）。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @return 命中列表
// @return 错误
func (s *RemindService) SendNow(ctx context.Context, taskID string) ([]*RemindHit, error) {
	if taskID == "" {
		return nil, apiError("WORKFLOW-1804", "参数 taskId 为空")
	}
	todo, found, err := s.todos.GetByTaskID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	// 待办不存在无法催办
	if !found {
		return nil, apiError("WORKFLOW-1805", "待办不存在: "+taskID)
	}
	reminds, err := s.reminds.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	hits := make([]*RemindHit, 0, 4)
	for _, remind := range reminds {
		// 手动催办同样受步骤白名单约束（不在名单内的配置不触发）
		if !s.inWhitelist(ctx, remind, todo) {
			continue
		}
		targets, err := s.targets.ListByRemind(ctx, remind.ID)
		if err != nil {
			return nil, err
		}
		for _, target := range targets {
			channels, err := s.channels.ChannelsOf(ctx, todo)
			if err != nil {
				return nil, err
			}
			hits = append(hits, &RemindHit{
				Remind:   remind,
				Target:   cloneRemindTarget(target),
				Todo:     todo,
				Channels: channels,
				Kind:     RemindKindManual,
				Content:  remindContent(todo, RemindKindManual),
			})
		}
	}
	return hits, nil
}

// Deliver 送达一次命中：展开对象 → 逐用户通知 → 落发送/重试标记（对位 Java sendToUser +
// updateSendWithReception + updateRetryCount）。
// @param ctx 操作上下文
// @param hit 命中
// @param notifier 送达器
// @return 实际送达用户数
// @return 错误
func (s *RemindService) Deliver(ctx context.Context, hit *RemindHit, notifier Notifier) (int, error) {
	// 命中或送达器缺失无事可做
	if hit == nil || notifier == nil {
		return 0, nil
	}
	initiator := s.initiatorOf(ctx, hit.Todo)
	users := s.ExpandTargets(ctx, hit.Target, &RemindInstanceContext{Todo: hit.Todo, Initiator: initiator})
	delivered := 0
	seen := make(map[string]bool, len(users))
	for _, userID := range users {
		// 同对象去重（Java HashSet uids 语义）
		if seen[userID] {
			continue
		}
		seen[userID] = true
		notification := &RemindNotification{
			ReceiverID:   userID,
			ReceiverName: s.userName(ctx, userID),
			Subject:      hit.Todo.Title,
			Content:      hit.Content,
			Channels:     hit.Channels,
			Kind:         hit.Kind,
		}
		if err := notifier.Notify(ctx, notification); err != nil {
			return delivered, err
		}
		delivered++
	}
	// 有实际送达才落标记（Java 仅在发送分支更新）
	if delivered > 0 {
		if err := s.markDelivered(ctx, hit.Target); err != nil {
			return delivered, err
		}
	}
	return delivered, nil
}

// markDelivered 落发送与重试标记（Java updateSendWithReception + updateRetryCount）：
// 首次发送置 sent=1；启用重试且未达上限时计数 +1。
// @param ctx 操作上下文
// @param target 命中对象
// @return 错误
func (s *RemindService) markDelivered(ctx context.Context, target *RemindTarget) error {
	// 对象存储未注入（独立送达场景）时跳过持久化，仅更新命中副本
	if s.targets == nil {
		if target.Sent == 0 {
			target.Sent = 1
		}
		if target.Retry == 1 && target.RetryTimes > 0 && target.RetryCount < target.RetryTimes {
			target.RetryCount++
		}
		return nil
	}
	// 首次发送：置已发送标记
	if target.Sent == 0 {
		if err := s.targets.MarkSent(ctx, target.ID); err != nil {
			return err
		}
		target.Sent = 1
	}
	// 重试计数：启用重试、上限为正且未达上限时 +1（Java updateRetryCount 语义）
	if target.Retry == 1 && target.RetryTimes > 0 && target.RetryCount < target.RetryTimes {
		if err := s.targets.UpdateRetryCount(ctx, target.ID, target.RetryCount+1); err != nil {
			return err
		}
		target.RetryCount++
	}
	return nil
}

// retryAllowed 判断已发送对象是否允许再次发送（重试启用且次数未用尽）。
// @param target 催办对象
// @return 是否允许
func (s *RemindService) retryAllowed(target *RemindTarget) bool {
	// 未启用重试或上限已用尽不再发（任务规格：retryTimes 用尽不再发）
	return target.Retry == 1 && target.RetryTimes > 0 && target.RetryCount < target.RetryTimes
}

// judgeDue 判定待办是否到期（对位 Java ReminderSchedule :161-225 分支）。
// @param ctx 操作上下文
// @param target 催办对象
// @param todo 待办
// @param now 当前时间
// @return 命中类别文案，未到期返回空串
func (s *RemindService) judgeDue(ctx context.Context, target *RemindTarget, todo *MessageExtend, now time.Time) string {
	// 预警型：dueTime - ahead <= now
	if target.Type == RemindTypeAhead {
		duration := time.Duration(durationMillis(target.Ahead, target.TimeUnit)) * time.Millisecond
		// 按计划完成时间判定
		if target.ByPlanCompleteTime == 1 {
			if todo.PlanFinishTime != nil && !todo.PlanFinishTime.Add(-duration).After(now) {
				return RemindKindAhead
			}
			return ""
		}
		// 按处理时限判定（时限取步骤规则；未配置跳过，Java getDueDate null 语义）
		due := s.dueDateOf(ctx, todo)
		if due != nil && !due.Add(-duration).After(now) {
			return RemindKindAhead
		}
		return ""
	}
	// 延时型：dueTime + delay <= now（或任务接收时间 + delay）
	if target.Type == RemindTypeDelay {
		duration := time.Duration(durationMillis(target.Delay, target.TimeUnit)) * time.Millisecond
		// 按计划完成时间判定
		if target.ByPlanCompleteTime == 1 {
			if todo.PlanFinishTime != nil && !todo.PlanFinishTime.Add(duration).After(now) {
				return RemindKindDelay
			}
			return ""
		}
		// 按处理时限判定
		if due := s.dueDateOf(ctx, todo); due != nil && !due.Add(duration).After(now) {
			return RemindKindDelay
		}
		// 回退任务接收时间判定（Java else 分支）
		if todo.TaskReceiptTime != nil && !todo.TaskReceiptTime.Add(duration).After(now) {
			return RemindKindDelay
		}
	}
	return ""
}

// dueDateOf 计算待办的处理时限到期点（对位 Java ReminderSchedule#getDueDate:238）：
// 任务接收时间 + 步骤催办时限（时限未配置返回 nil，判定跳过）。
// @param ctx 操作上下文
// @param todo 待办
// @return 到期时间，未配置返回 nil
func (s *RemindService) dueDateOf(ctx context.Context, todo *MessageExtend) *time.Time {
	// 接收时间缺失无法推算
	if todo.TaskReceiptTime == nil {
		return nil
	}
	limit, unit, err := s.rules.RemindLimitOf(ctx, todo.TaskStepCode)
	// 规则读取失败按未配置处理（与 Java getDuration null 静默语义一致）
	if err != nil || limit == nil {
		return nil
	}
	due := todo.TaskReceiptTime.Add(time.Duration(durationMillis(*limit, unit)) * time.Millisecond)
	return &due
}

// inWhitelist 步骤白名单过滤（Java :144-149）：白名单为空跳过；待办步骤代号解析出的
// 步骤 Id 不在名单内跳过。解析端口未注入时按代号直匹配（嵌入模式 Id==代号）。
// @param ctx 操作上下文
// @param remind 催办配置
// @param todo 待办
// @return 是否在白名单内
func (s *RemindService) inWhitelist(ctx context.Context, remind *Remind, todo *MessageExtend) bool {
	// 白名单为空：无可催步骤（Java StrUtil.isBlank → continue）
	if strings.TrimSpace(remind.StepIDs) == "" {
		return false
	}
	stepID := todo.TaskStepCode
	// 解析端口注入时按设计时步骤 Id 匹配
	if s.resolver != nil {
		resolved, err := s.resolver.ResolveStepID(ctx, todo.TaskStepCode)
		if err != nil || resolved == "" {
			return false
		}
		stepID = resolved
	}
	return whitelistContains(remind.StepIDs, stepID)
}

// whitelistContains 判断逗号串白名单是否包含指定步骤 Id（精确分段匹配，
// 修正 Java String.contains 子串误命中问题：id=11 不再误命中 110）。
// @param stepIDs 逗号串
// @param stepID 步骤 Id
// @return 是否包含
func whitelistContains(stepIDs, stepID string) bool {
	// 空目标不参与匹配
	if stepID == "" {
		return false
	}
	for _, part := range strings.Split(stepIDs, ",") {
		if strings.TrimSpace(part) == stepID {
			return true
		}
	}
	return false
}

// initiatorOf 读取待办实例的发起人变量（端口缺失或读取失败返回空串）。
// @param ctx 操作上下文
// @param todo 待办
// @return 发起人 Id
func (s *RemindService) initiatorOf(ctx context.Context, todo *MessageExtend) string {
	// 待办/变量端口缺失时无法解析
	if s.vars == nil || todo == nil || todo.ProcessInstanceID == "" {
		return ""
	}
	value, err := s.vars.GetVariable(ctx, todo.ProcessInstanceID, VarInitiator)
	if err != nil || value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprintf("%v", value)
}

// userName 查询用户展示名（成员端口未注入或用户不在任何组时返回空串，送达器自行解析）。
// @param ctx 操作上下文
// @param userID 用户 Id
// @return 展示名
func (s *RemindService) userName(ctx context.Context, userID string) string {
	// 成员端口未注入时无姓名来源
	if s.members == nil {
		return ""
	}
	// 遍历成员查询属于 O(n) 反查，且本引擎组无用户级缓存，此处保持空实现：
	// 通知报文的姓名展示由具体渠道送达器（P6）按身份库补齐
	_ = userID
	return ""
}

// remindContent 构造催办内容（Java sendToUser 同款格式：
// 【类别】标题 + 流程分类 + 接收时间）。
// @param todo 待办
// @param kind 类别
// @return 内容串
func remindContent(todo *MessageExtend, kind string) string {
	receipt := ""
	// 接收时间缺失时留空
	if todo.TaskReceiptTime != nil {
		receipt = todo.TaskReceiptTime.Format("2006-01-02 15:04:05")
	}
	return "【" + kind + "】" + todo.Title + "\n" + "流程：" + todo.FlowCategoryName + "\n" + "时间：" + receipt
}

// durationMillis 时长数值转毫秒（对位 Java ReminderSchedule#getDuration:509）：
// 天/小时/分钟三档；未知单位返回 0（Java null → 0 静默语义）。
// @param t 时长数值
// @param unit 单位（天/day、小时/hour、分钟/minute）
// @return 毫秒数
func durationMillis(t float64, unit string) int64 {
	switch {
	// 天
	case unit == "天" || strings.EqualFold(unit, "day"):
		return int64(t * 24 * 60 * 60 * 1000)
	// 小时
	case unit == "小时" || strings.EqualFold(unit, "hour"):
		return int64(t * 60 * 60 * 1000)
	// 分钟
	case unit == "分钟" || strings.EqualFold(unit, "minute"):
		return int64(t * 60 * 1000)
	default:
		// 未知单位按 0 处理（Java getDuration 返回 null 后取 0f 的等价语义）
		return 0
	}
}
