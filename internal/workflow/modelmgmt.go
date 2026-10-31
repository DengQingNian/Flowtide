package workflow

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
)

// 模型管理域错误码（WORKFLOW-30xx 段：模型/分类、设计时步骤、设计器模板）。
const (
	errCodeMMParamsBlank  = "WORKFLOW-3001" // 管理请求参数缺失（主键/代号/租户）
	errCodeMMModelMissing = "WORKFLOW-3002" // 模型/分类不存在
	errCodeMMModelDup     = "WORKFLOW-3003" // 模型代号在租户内重复
	errCodeMMStepBlank    = "WORKFLOW-3004" // 步骤请求缺少模型/代号/租户
	errCodeMMStepDup      = "WORKFLOW-3005" // 模型下步骤代号重复
	errCodeMMStepMissing  = "WORKFLOW-3006" // 步骤不存在
	errCodeMMTplBlank     = "WORKFLOW-3007" // 模板请求缺少名称/租户
	errCodeMMTplMissing   = "WORKFLOW-3008" // 模板不存在
)

// ─── 实体 ─────────────────────────────────────────────
// 字段清单对位 docs/006-workflow二次封装/003-表结构.md 与 DDL
// internal/persistence/schema/sql/*/002-workflow-1.0.sql。
// "0"/"1" 布尔与比例类规则列沿用 StepRule 的约定：原样保留数据库形态字符串，
// 由服务层按 Java 语义解释。

// ModelCategory 是 workflow_model 行实体（一表两用：流程分类与流程模型）。
// 分类与模型共用同一套列：分类节点的 model_code 为分类代号，模型节点的
// model_code = 流程定义 key（租户内唯一，(model_code, tenant_id) 唯一索引）。
type ModelCategory struct {
	BaseColumns
	ResourceID           string     // resource_id 关联资源 ID
	ModelName            string     // model_name 名称（分类页为分类名称）
	ModelCode            string     // model_code 代号（模型=流程定义 key；分类=分类代号）
	Tips                 string     // tips 提示文案
	Target               string     // target 打开目标（如 _blank）
	Roles                string     // roles 角色配置（JSON 原文）
	MenuIcon             string     // menu_icon 菜单图标
	Remark               string     // remark 备注
	Sort                 int        // sort 排序号
	ParentID             string     // parent_id 父分类 ID（根节点为空串）
	Enable               int        // enable 启用（1 启用/0 停用）
	ReturnSort           int        // return_sort 退回步骤排序开关
	IsLeaf               int        // is_leaf 叶子标记
	ProcessDefinitionKey string     // process_definition_key 绑定流程定义 key
	Visible              int        // visible 可见
	EngineModelID        string     // model_id 引擎侧模型 ID（列名 model_id，避免与主键混淆改名）
	LastDeployTime       *time.Time // last_deploy_time 最近部署时间（nil 未部署）
}

// StepDesign 是 workflow_step 行实体（设计时步骤配置）。
// allow_return/countersign_percent_return/return_percent 等规则列原样保留
// 数据库形态，部署时经 DeploySnapshotService 转为运行期 StepRule。
type StepDesign struct {
	BaseColumns
	Name                     string     // name 步骤名称
	StepCode                 string     // step_code 步骤代号（=BPMN 节点 ID）
	Description              string     // description 描述
	Value                    string     // value 扩展值
	AccountName              string     // account_name 账套名
	Source                   int        // source 来源
	ModelID                  string     // model_id 所属模型 ID
	ResourceID               string     // resource_id 关联资源 ID
	Sort                     float64    // sort 排序号（DDL double）
	Agree                    string     // agree 同意配置
	Reject                   string     // reject 拒绝配置
	Remark                   string     // remark 备注
	Fix                      string     // fix 固定配置（text）
	URL                      string     // url 步骤处理地址
	Visible                  int        // visible 可见
	TaskType                 string     // task_type 任务类型
	EditInfo                 string     // edit_info 编辑信息（text）
	OperateAuth              string     // operate_auth 操作权限（text）
	AllowReturn              string     // allow_return 允许退回原始值（"0" 禁止/"1" 允许）
	BackStepCodes            string     // back_step_codes 可退回步骤代号白名单（逗号分隔）
	BackStepNames            string     // back_step_names 可退回步骤名称（逗号分隔）
	AutoComplete             int        // auto_complete 自动办结开关
	AutoCompleteStartTime    *time.Time // auto_complete_start_time 自动办结起始
	AutoCompleteEndTime      *time.Time // auto_complete_end_time 自动办结截止
	TimeLimit                float64    // time_limit 办结时限
	TimeLimitUnit            string     // time_limit_unit 办结时限单位
	RemindTimeLimit          float64    // remind_time_limit 提醒时限
	RemindTimeLimitUnit      string     // remind_time_limit_unit 提醒时限单位
	ChannelTypeName          string     // channel_type_name 渠道类型
	ChannelTypeNameCN        string     // channel_type_name_cn 渠道类型中文名
	ReturnApprovalStep       string     // return_approval_step 退回审批步骤原始值
	CountersignPercentReturn string     // countersign_percent_return 会签比例退回原始值（"1" 启用）
	ReturnPercent            string     // return_percent 退回比例原始值（百分数）
}

// TemplateDef 是 workflow_template 行实体（设计器模板）。
type TemplateDef struct {
	BaseColumns
	Name       string // name 模板名称
	Layout     string // layout 布局配置（JSON 原文）
	FlowXML    string // flow_xml 流程 XML
	IsVisible  int    // is_visible 可见
	IsDefault  int    // is_default 默认模板
	Source     int    // source 来源
	CategoryID string // category_id 所属分类
}

// ModelTreeNode 是模型/分类树节点（Tree 输出）。
type ModelTreeNode struct {
	Model    *ModelCategory   // 节点实体
	Children []*ModelTreeNode // 子节点（按 sort 升序，同号按代号）
}

// ─── 存储端口 ─────────────────────────────────────────────

// ModelMgmtStore 是 workflow_model（模型/分类主数据）的存储端口。
type ModelMgmtStore interface {
	// Insert 插入一条模型/分类（ID/审计列由调用方生成填充）。
	// @param ctx 操作上下文
	// @param m 实体（必填 id/tenant_id）
	// @return 错误
	Insert(ctx context.Context, m *ModelCategory) error
	// Update 按主键更新整行业务列（主键与创建审计不参与更新）。
	// @param ctx 操作上下文
	// @param m 实体（必填 id）
	// @return 错误（不存在时报未找到）
	Update(ctx context.Context, m *ModelCategory) error
	// GetByPK 按主键查询。
	// @param ctx 操作上下文
	// @param id 主键
	// @return 实体
	// @return 是否存在
	// @return 查询错误
	GetByPK(ctx context.Context, id string) (*ModelCategory, bool, error)
	// GetByCode 按租户内模型代号（=流程定义 key 或分类代号）查询。
	// @param ctx 操作上下文
	// @param tenantID 租户
	// @param modelCode 模型代号
	// @return 实体
	// @return 是否存在
	// @return 查询错误
	GetByCode(ctx context.Context, tenantID, modelCode string) (*ModelCategory, bool, error)
	// ListByTenant 查询租户下全部模型与分类（按 sort 升序）。
	// @param ctx 操作上下文
	// @param tenantID 租户
	// @return 实体列表（无记录返回空列表）
	// @return 查询错误
	ListByTenant(ctx context.Context, tenantID string) ([]*ModelCategory, error)
	// UpdateLastDeployTime 回写模型最近部署时间（部署快照后调用）。
	// @param ctx 操作上下文
	// @param id 模型主键
	// @param deployTime 部署时间
	// @return 错误（不存在时报未找到）
	UpdateLastDeployTime(ctx context.Context, id string, deployTime time.Time) error
	// DeleteByIDs 按主键批量物理删除（缺失主键忽略）。
	// @param ctx 操作上下文
	// @param ids 主键集合
	// @return 错误
	DeleteByIDs(ctx context.Context, ids []string) error
}

// StepDesignStore 是 workflow_step（设计时步骤配置）的存储端口。
type StepDesignStore interface {
	// Insert 插入一条设计时步骤（ID/审计列由调用方生成填充）。
	// @param ctx 操作上下文
	// @param s 实体（必填 id/model_id/step_code/tenant_id）
	// @return 错误
	Insert(ctx context.Context, s *StepDesign) error
	// Update 按主键更新整行业务列（主键与创建审计不参与更新）。
	// @param ctx 操作上下文
	// @param s 实体（必填 id）
	// @return 错误（不存在时报未找到）
	Update(ctx context.Context, s *StepDesign) error
	// GetByPK 按主键查询。
	// @param ctx 操作上下文
	// @param id 主键
	// @return 实体
	// @return 是否存在
	// @return 查询错误
	GetByPK(ctx context.Context, id string) (*StepDesign, bool, error)
	// GetByModelAndCode 按模型与步骤代号查询单条配置。
	// @param ctx 操作上下文
	// @param modelID 模型 ID
	// @param stepCode 步骤代号
	// @return 实体
	// @return 是否存在
	// @return 查询错误
	GetByModelAndCode(ctx context.Context, modelID, stepCode string) (*StepDesign, bool, error)
	// ListByModel 查询模型下全部步骤配置（按 sort 升序）。
	// @param ctx 操作上下文
	// @param modelID 模型 ID
	// @return 实体列表（无记录返回空列表）
	// @return 查询错误
	ListByModel(ctx context.Context, modelID string) ([]*StepDesign, error)
	// DeleteByIDs 按主键批量物理删除（缺失主键忽略）。
	// @param ctx 操作上下文
	// @param ids 主键集合
	// @return 错误
	DeleteByIDs(ctx context.Context, ids []string) error
}

// TemplateStore 是 workflow_template（设计器模板）的存储端口。
type TemplateStore interface {
	// Insert 插入一条模板（ID/审计列由调用方生成填充）。
	// @param ctx 操作上下文
	// @param t 实体（必填 id/tenant_id）
	// @return 错误
	Insert(ctx context.Context, t *TemplateDef) error
	// Update 按主键更新整行业务列（主键与创建审计不参与更新）。
	// @param ctx 操作上下文
	// @param t 实体（必填 id）
	// @return 错误（不存在时报未找到）
	Update(ctx context.Context, t *TemplateDef) error
	// GetByPK 按主键查询。
	// @param ctx 操作上下文
	// @param id 主键
	// @return 实体
	// @return 是否存在
	// @return 查询错误
	GetByPK(ctx context.Context, id string) (*TemplateDef, bool, error)
	// ListByTenant 查询租户下全部模板（按名称升序）。
	// @param ctx 操作上下文
	// @param tenantID 租户
	// @return 实体列表（无记录返回空列表）
	// @return 查询错误
	ListByTenant(ctx context.Context, tenantID string) ([]*TemplateDef, error)
	// ListByCategory 查询分类下全部模板（按名称升序，跨租户以分类归属隔离）。
	// @param ctx 操作上下文
	// @param categoryID 分类 ID
	// @return 实体列表（无记录返回空列表）
	// @return 查询错误
	ListByCategory(ctx context.Context, categoryID string) ([]*TemplateDef, error)
	// DeleteByIDs 按主键批量物理删除（缺失主键忽略）。
	// @param ctx 操作上下文
	// @param ids 主键集合
	// @return 错误
	DeleteByIDs(ctx context.Context, ids []string) error
}

// ─── 内存实现 ─────────────────────────────────────────────
// 参照 MemStepStore 风格：RWMutex + map、clock.Clock 填充审计列、
// 存取双向克隆防止内部状态外泄。

// mmBlankID 判断主键是否缺失（空白视为缺失）。
// @param id 主键
// @return 是否缺失
func mmBlankID(id string) bool {
	return strings.TrimSpace(id) == ""
}

// mmCloneModel 深拷贝模型实体（指针列独立复制）。
// @param m 原实体
// @return 副本
func mmCloneModel(m *ModelCategory) *ModelCategory {
	if m == nil {
		return nil
	}
	copied := *m
	copied.LastDeployTime = cloneTime(m.LastDeployTime)
	return &copied
}

// mmCloneStepDesign 深拷贝设计时步骤实体（指针列独立复制）。
// @param s 原实体
// @return 副本
func mmCloneStepDesign(s *StepDesign) *StepDesign {
	if s == nil {
		return nil
	}
	copied := *s
	copied.AutoCompleteStartTime = cloneTime(s.AutoCompleteStartTime)
	copied.AutoCompleteEndTime = cloneTime(s.AutoCompleteEndTime)
	return &copied
}

// mmCloneTemplateDef 深拷贝模板实体（全值列，值拷贝即可）。
// @param t 原实体
// @return 副本
func mmCloneTemplateDef(t *TemplateDef) *TemplateDef {
	if t == nil {
		return nil
	}
	copied := *t
	return &copied
}

// MemModelMgmtStore 是 ModelMgmtStore 的内存实现。
type MemModelMgmtStore struct {
	mu    sync.RWMutex              // 读写锁
	clock clock.Clock               // 时间源（审计列填充）
	rows  map[string]*ModelCategory // id → 实体
}

// NewMemModelMgmtStore 创建模型内存存储。
// @param c 时间源，nil 时回退真实时钟
// @return 内存存储
func NewMemModelMgmtStore(c clock.Clock) *MemModelMgmtStore {
	if c == nil {
		c = clock.Real{}
	}
	return &MemModelMgmtStore{clock: c, rows: make(map[string]*ModelCategory)}
}

// 编译期断言：内存实现满足端口。
var _ ModelMgmtStore = (*MemModelMgmtStore)(nil)

// Insert 插入一条模型/分类（缺主键/空实体报错；缺审计时间按时钟填充）。
// @param ctx 操作上下文
// @param m 实体
// @return 错误
func (m *MemModelMgmtStore) Insert(_ context.Context, row *ModelCategory) error {
	// 空实体/缺主键与 pwf 仓储 WF-0006/WF-0008 契约一致
	if row == nil {
		return apiError(errCodeMMParamsBlank, "模型插入实体为空")
	}
	if mmBlankID(row.ID) {
		return apiError(errCodeMMParamsBlank, "模型插入必须携带非空主键")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := mmCloneModel(row)
	// 审计时间缺省由存储时钟填充
	if copied.CreateTime.IsZero() {
		copied.CreateTime = m.clock.Now()
	}
	if copied.UpdateTime.IsZero() {
		copied.UpdateTime = m.clock.Now()
	}
	m.rows[copied.ID] = copied
	return nil
}

// Update 按主键更新整行业务列（不存在时报未找到；更新时间刷新为当前时钟）。
// @param ctx 操作上下文
// @param row 实体
// @return 错误
func (m *MemModelMgmtStore) Update(_ context.Context, row *ModelCategory) error {
	// 空实体/缺主键防御
	if row == nil || mmBlankID(row.ID) {
		return apiError(errCodeMMParamsBlank, "模型更新必须携带非空主键")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.rows[row.ID]; !ok {
		return api.NewError(errCodeMMModelMissing, "模型/分类不存在: "+row.ID, api.ErrNotFound)
	}
	copied := mmCloneModel(row)
	copied.UpdateTime = m.clock.Now()
	m.rows[copied.ID] = copied
	return nil
}

// GetByPK 按主键查询（返回副本）。
// @param ctx 操作上下文
// @param id 主键
// @return 实体副本
// @return 是否存在
// @return 查询错误
func (m *MemModelMgmtStore) GetByPK(_ context.Context, id string) (*ModelCategory, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	row, ok := m.rows[id]
	if !ok {
		return nil, false, nil
	}
	return mmCloneModel(row), true, nil
}

// GetByCode 按租户与模型代号查询（返回副本）。
// @param ctx 操作上下文
// @param tenantID 租户
// @param modelCode 模型代号
// @return 实体副本
// @return 是否存在
// @return 查询错误
func (m *MemModelMgmtStore) GetByCode(_ context.Context, tenantID, modelCode string) (*ModelCategory, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, row := range m.rows {
		// 租户与代号双条件命中（唯一索引 (model_code, tenant_id) 语义）
		if row.TenantID == tenantID && row.ModelCode == modelCode {
			return mmCloneModel(row), true, nil
		}
	}
	return nil, false, nil
}

// ListByTenant 查询租户下全部模型与分类（按 sort 升序，同号按代号）。
// @param ctx 操作上下文
// @param tenantID 租户
// @return 实体副本列表
// @return 查询错误
func (m *MemModelMgmtStore) ListByTenant(_ context.Context, tenantID string) ([]*ModelCategory, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*ModelCategory, 0, 8)
	for _, row := range m.rows {
		if row.TenantID == tenantID {
			out = append(out, mmCloneModel(row))
		}
	}
	mmSortModels(out)
	return out, nil
}

// UpdateLastDeployTime 回写模型最近部署时间（不存在时报未找到）。
// @param ctx 操作上下文
// @param id 模型主键
// @param deployTime 部署时间
// @return 错误
func (m *MemModelMgmtStore) UpdateLastDeployTime(_ context.Context, id string, deployTime time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.rows[id]
	// 缺失行报未找到（与 pwf UpdateByPK 影响行数 0 的可观察语义对齐并强化）
	if !ok {
		return api.NewError(errCodeMMModelMissing, "模型/分类不存在: "+id, api.ErrNotFound)
	}
	stamped := deployTime
	row.LastDeployTime = &stamped
	row.UpdateTime = m.clock.Now()
	return nil
}

// DeleteByIDs 按主键批量物理删除（缺失主键忽略，与 pwf 删除零影响行数语义一致）。
// @param ctx 操作上下文
// @param ids 主键集合
// @return 错误
func (m *MemModelMgmtStore) DeleteByIDs(_ context.Context, ids []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range ids {
		delete(m.rows, id)
	}
	return nil
}

// mmSortModels 按 sort 升序、同号按代号原地排序（短列表插入排序，与 MemStepStore 风格一致）。
// @param rows 实体列表
func mmSortModels(rows []*ModelCategory) {
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && mmModelLess(rows[j], rows[j-1]); j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
}

// mmModelLess 比较两个模型的排序键（sort 升序，同号按代号字典序）。
// @param a 左操作数
// @param b 右操作数
// @return a 是否排在 b 前
func mmModelLess(a, b *ModelCategory) bool {
	// 同排序号回退代号比较，保证输出顺序确定
	if a.Sort != b.Sort {
		return a.Sort < b.Sort
	}
	return a.ModelCode < b.ModelCode
}

// MemStepDesignStore 是 StepDesignStore 的内存实现。
type MemStepDesignStore struct {
	mu    sync.RWMutex           // 读写锁
	clock clock.Clock            // 时间源（审计列填充）
	rows  map[string]*StepDesign // id → 实体
}

// NewMemStepDesignStore 创建设计时步骤内存存储。
// @param c 时间源，nil 时回退真实时钟
// @return 内存存储
func NewMemStepDesignStore(c clock.Clock) *MemStepDesignStore {
	if c == nil {
		c = clock.Real{}
	}
	return &MemStepDesignStore{clock: c, rows: make(map[string]*StepDesign)}
}

// 编译期断言：内存实现满足端口。
var _ StepDesignStore = (*MemStepDesignStore)(nil)

// Insert 插入一条设计时步骤（缺主键/空实体报错；缺审计时间按时钟填充）。
// @param ctx 操作上下文
// @param s 实体
// @return 错误
func (m *MemStepDesignStore) Insert(_ context.Context, s *StepDesign) error {
	// 空实体/缺主键与 pwf 仓储契约一致
	if s == nil {
		return apiError(errCodeMMStepBlank, "步骤插入实体为空")
	}
	if mmBlankID(s.ID) {
		return apiError(errCodeMMStepBlank, "步骤插入必须携带非空主键")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := mmCloneStepDesign(s)
	// 审计时间缺省由存储时钟填充
	if copied.CreateTime.IsZero() {
		copied.CreateTime = m.clock.Now()
	}
	if copied.UpdateTime.IsZero() {
		copied.UpdateTime = m.clock.Now()
	}
	m.rows[copied.ID] = copied
	return nil
}

// Update 按主键更新整行业务列（不存在时报未找到；更新时间刷新为当前时钟）。
// @param ctx 操作上下文
// @param s 实体
// @return 错误
func (m *MemStepDesignStore) Update(_ context.Context, s *StepDesign) error {
	// 空实体/缺主键防御
	if s == nil || mmBlankID(s.ID) {
		return apiError(errCodeMMStepBlank, "步骤更新必须携带非空主键")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.rows[s.ID]; !ok {
		return api.NewError(errCodeMMStepMissing, "步骤不存在: "+s.ID, api.ErrNotFound)
	}
	copied := mmCloneStepDesign(s)
	copied.UpdateTime = m.clock.Now()
	m.rows[copied.ID] = copied
	return nil
}

// GetByPK 按主键查询（返回副本）。
// @param ctx 操作上下文
// @param id 主键
// @return 实体副本
// @return 是否存在
// @return 查询错误
func (m *MemStepDesignStore) GetByPK(_ context.Context, id string) (*StepDesign, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	row, ok := m.rows[id]
	if !ok {
		return nil, false, nil
	}
	return mmCloneStepDesign(row), true, nil
}

// GetByModelAndCode 按模型与步骤代号查询（返回副本）。
// @param ctx 操作上下文
// @param modelID 模型 ID
// @param stepCode 步骤代号
// @return 实体副本
// @return 是否存在
// @return 查询错误
func (m *MemStepDesignStore) GetByModelAndCode(_ context.Context, modelID, stepCode string) (*StepDesign, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, row := range m.rows {
		// 模型与代号双条件命中（模型 ID 全局唯一，天然含租户隔离）
		if row.ModelID == modelID && row.StepCode == stepCode {
			return mmCloneStepDesign(row), true, nil
		}
	}
	return nil, false, nil
}

// ListByModel 查询模型下全部步骤配置（按 sort 升序，同号按代号）。
// @param ctx 操作上下文
// @param modelID 模型 ID
// @return 实体副本列表
// @return 查询错误
func (m *MemStepDesignStore) ListByModel(_ context.Context, modelID string) ([]*StepDesign, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*StepDesign, 0, 8)
	for _, row := range m.rows {
		if row.ModelID == modelID {
			out = append(out, mmCloneStepDesign(row))
		}
	}
	// 插入排序：sort 升序，同号按步骤代号
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && mmStepLess(out[j], out[j-1]); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, nil
}

// DeleteByIDs 按主键批量物理删除（缺失主键忽略）。
// @param ctx 操作上下文
// @param ids 主键集合
// @return 错误
func (m *MemStepDesignStore) DeleteByIDs(_ context.Context, ids []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range ids {
		delete(m.rows, id)
	}
	return nil
}

// mmStepLess 比较两个步骤的排序键（sort 升序，同号按代号字典序）。
// @param a 左操作数
// @param b 右操作数
// @return a 是否排在 b 前
func mmStepLess(a, b *StepDesign) bool {
	// 同排序号回退代号比较，保证输出顺序确定
	if a.Sort != b.Sort {
		return a.Sort < b.Sort
	}
	return a.StepCode < b.StepCode
}

// MemTemplateStore 是 TemplateStore 的内存实现。
type MemTemplateStore struct {
	mu    sync.RWMutex            // 读写锁
	clock clock.Clock             // 时间源（审计列填充）
	rows  map[string]*TemplateDef // id → 实体
}

// NewMemTemplateStore 创建模板内存存储。
// @param c 时间源，nil 时回退真实时钟
// @return 内存存储
func NewMemTemplateStore(c clock.Clock) *MemTemplateStore {
	if c == nil {
		c = clock.Real{}
	}
	return &MemTemplateStore{clock: c, rows: make(map[string]*TemplateDef)}
}

// 编译期断言：内存实现满足端口。
var _ TemplateStore = (*MemTemplateStore)(nil)

// Insert 插入一条模板（缺主键/空实体报错；缺审计时间按时钟填充）。
// @param ctx 操作上下文
// @param t 实体
// @return 错误
func (m *MemTemplateStore) Insert(_ context.Context, t *TemplateDef) error {
	// 空实体/缺主键与 pwf 仓储契约一致
	if t == nil {
		return apiError(errCodeMMTplBlank, "模板插入实体为空")
	}
	if mmBlankID(t.ID) {
		return apiError(errCodeMMTplBlank, "模板插入必须携带非空主键")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := mmCloneTemplateDef(t)
	// 审计时间缺省由存储时钟填充
	if copied.CreateTime.IsZero() {
		copied.CreateTime = m.clock.Now()
	}
	if copied.UpdateTime.IsZero() {
		copied.UpdateTime = m.clock.Now()
	}
	m.rows[copied.ID] = copied
	return nil
}

// Update 按主键更新整行业务列（不存在时报未找到；更新时间刷新为当前时钟）。
// @param ctx 操作上下文
// @param t 实体
// @return 错误
func (m *MemTemplateStore) Update(_ context.Context, t *TemplateDef) error {
	// 空实体/缺主键防御
	if t == nil || mmBlankID(t.ID) {
		return apiError(errCodeMMTplBlank, "模板更新必须携带非空主键")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.rows[t.ID]; !ok {
		return api.NewError(errCodeMMTplMissing, "模板不存在: "+t.ID, api.ErrNotFound)
	}
	copied := mmCloneTemplateDef(t)
	copied.UpdateTime = m.clock.Now()
	m.rows[copied.ID] = copied
	return nil
}

// GetByPK 按主键查询（返回副本）。
// @param ctx 操作上下文
// @param id 主键
// @return 实体副本
// @return 是否存在
// @return 查询错误
func (m *MemTemplateStore) GetByPK(_ context.Context, id string) (*TemplateDef, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	row, ok := m.rows[id]
	if !ok {
		return nil, false, nil
	}
	return mmCloneTemplateDef(row), true, nil
}

// ListByTenant 查询租户下全部模板（按名称升序，同名列按主键）。
// @param ctx 操作上下文
// @param tenantID 租户
// @return 实体副本列表
// @return 查询错误
func (m *MemTemplateStore) ListByTenant(_ context.Context, tenantID string) ([]*TemplateDef, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*TemplateDef, 0, 8)
	for _, row := range m.rows {
		if row.TenantID == tenantID {
			out = append(out, mmCloneTemplateDef(row))
		}
	}
	// 插入排序：名称升序，同名回退主键保证顺序确定
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && mmTemplateLess(out[j], out[j-1]); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, nil
}

// ListByCategory 查询分类下全部模板（按名称升序）。
// @param ctx 操作上下文
// @param categoryID 分类 ID
// @return 实体副本列表
// @return 查询错误
func (m *MemTemplateStore) ListByCategory(_ context.Context, categoryID string) ([]*TemplateDef, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*TemplateDef, 0, 8)
	for _, row := range m.rows {
		if row.CategoryID == categoryID {
			out = append(out, mmCloneTemplateDef(row))
		}
	}
	// 插入排序：名称升序，同名回退主键保证顺序确定
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && mmTemplateLess(out[j], out[j-1]); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, nil
}

// DeleteByIDs 按主键批量物理删除（缺失主键忽略）。
// @param ctx 操作上下文
// @param ids 主键集合
// @return 错误
func (m *MemTemplateStore) DeleteByIDs(_ context.Context, ids []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range ids {
		delete(m.rows, id)
	}
	return nil
}

// mmTemplateLess 比较两个模板的排序键（名称升序，同名按主键）。
// @param a 左操作数
// @param b 右操作数
// @return a 是否排在 b 前
func mmTemplateLess(a, b *TemplateDef) bool {
	// 同名回退主键比较，保证输出顺序确定
	if a.Name != b.Name {
		return a.Name < b.Name
	}
	return a.ID < b.ID
}

// ─── 领域服务 ─────────────────────────────────────────────
// 删除语义取舍（对位 003 文档 §2）：workflow_model/workflow_step/workflow_template
// 均无 deleted 列（deleted 仅存在于 document/remind/remind_target 且需手工过滤）；
// enable/visible 为管理台"启用/可见"开关的业务标记，若复用承担软删将无法区分
// "已删除"与"已停用"，故三个管理服务统一采用物理删除，启停经 Edit 调整。

// ModelService 是模型/分类管理域服务。
type ModelService struct {
	store ModelMgmtStore // 模型存储端口
	clock clock.Clock    // 时间源（审计列填充）
	newID func() string  // ID 生成函数
}

// NewModelService 构造模型/分类管理服务。
// @param store 模型存储端口
// @param c 时间源，nil 时回退真实时钟
// @return 模型服务
func NewModelService(store ModelMgmtStore, c clock.Clock) *ModelService {
	if c == nil {
		c = clock.Real{}
	}
	return &ModelService{store: store, clock: c, newID: DefaultIDGen()}
}

// Add 新增模型/分类（校验代号与租户、租户内代号唯一，补齐主键与审计列）。
// @param ctx 操作上下文
// @param m 模型数据（必填 model_code/tenant_id，主键留空自动生成）
// @return 落库后的模型实体
// @return 错误
func (s *ModelService) Add(ctx context.Context, m *ModelCategory) (*ModelCategory, error) {
	// 空请求/缺代号/缺租户统一参数错误
	if m == nil || strings.TrimSpace(m.ModelCode) == "" || strings.TrimSpace(m.TenantID) == "" {
		return nil, apiError(errCodeMMParamsBlank, "模型新增缺少代号或租户")
	}
	entity := *m
	// 租户内代号唯一（(model_code, tenant_id) 唯一索引语义）
	if _, found, err := s.store.GetByCode(ctx, entity.TenantID, entity.ModelCode); err != nil {
		return nil, err
	} else if found {
		return nil, apiError(errCodeMMModelDup, "模型代号已存在: "+entity.ModelCode)
	}
	fillBase(&entity.BaseColumns, s.newID, s.clock.Now, "", entity.TenantID)
	if err := s.store.Insert(ctx, &entity); err != nil {
		return nil, err
	}
	return &entity, nil
}

// Edit 编辑模型/分类（保留创建审计与归属租户，刷新更新时间；代号变更需通过租户内查重）。
// @param ctx 操作上下文
// @param m 模型数据（必填主键）
// @return 编辑后的模型实体
// @return 错误
func (s *ModelService) Edit(ctx context.Context, m *ModelCategory) (*ModelCategory, error) {
	// 空请求/缺主键统一参数错误
	if m == nil || mmBlankID(m.ID) {
		return nil, apiError(errCodeMMParamsBlank, "模型编辑缺少主键")
	}
	existing, found, err := s.store.GetByPK(ctx, m.ID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, api.NewError(errCodeMMModelMissing, "模型/分类不存在: "+m.ID, api.ErrNotFound)
	}
	if strings.TrimSpace(m.ModelCode) == "" {
		return nil, apiError(errCodeMMParamsBlank, "模型编辑缺少代号")
	}
	// 代号查重以既有行租户为准（编辑不改变归属租户，避免树/列表跨租户漂移）
	if other, found, err := s.store.GetByCode(ctx, existing.TenantID, m.ModelCode); err != nil {
		return nil, err
	} else if found && other.ID != m.ID {
		return nil, apiError(errCodeMMModelDup, "模型代号已存在: "+m.ModelCode)
	}
	merged := *m
	// 创建审计保持既有值
	merged.Creator = existing.Creator
	merged.CreatorName = existing.CreatorName
	merged.CreateTime = existing.CreateTime
	merged.TenantID = existing.TenantID
	merged.Updater = firstNonEmpty(m.Updater, existing.Updater)
	merged.UpdateTime = s.clock.Now()
	if err := s.store.Update(ctx, &merged); err != nil {
		return nil, err
	}
	return &merged, nil
}

// Delete 按主键批量删除（物理删除，语义取舍见本文件域服务段说明）。
// @param ctx 操作上下文
// @param ids 主键集合
// @return 错误
func (s *ModelService) Delete(ctx context.Context, ids ...string) error {
	// 空集合明确拒绝，避免无调用参
	if len(ids) == 0 {
		return apiError(errCodeMMParamsBlank, "模型删除缺少主键集合")
	}
	return s.store.DeleteByIDs(ctx, ids)
}

// List 查询租户下全部模型与分类（按 sort 升序平铺）。
// @param ctx 操作上下文
// @param tenantID 租户
// @return 模型列表
// @return 错误
func (s *ModelService) List(ctx context.Context, tenantID string) ([]*ModelCategory, error) {
	// 租户必填（存储层按租户隔离）
	if strings.TrimSpace(tenantID) == "" {
		return nil, apiError(errCodeMMParamsBlank, "模型列表缺少租户")
	}
	return s.store.ListByTenant(ctx, tenantID)
}

// Tree 输出租户下模型/分类树（根=父为空或父缺失；孤儿节点提升为根，成环节点不输出）。
// @param ctx 操作上下文
// @param tenantID 租户
// @return 树根列表
// @return 错误
func (s *ModelService) Tree(ctx context.Context, tenantID string) ([]*ModelTreeNode, error) {
	rows, err := s.List(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	// 建节点索引
	nodes := make(map[string]*ModelTreeNode, len(rows))
	for _, row := range rows {
		nodes[row.ID] = &ModelTreeNode{Model: row}
	}
	roots := make([]*ModelTreeNode, 0, 4)
	// 父子挂接：父缺失的孤儿提升为根（成环节点既无根也无父，不输出）
	for _, row := range rows {
		node := nodes[row.ID]
		parent, ok := nodes[row.ParentID]
		if row.ParentID == "" || !ok {
			roots = append(roots, node)
			continue
		}
		parent.Children = append(parent.Children, node)
	}
	// 全树排序（sort 升序，同号按代号）
	mmSortModelNodes(roots)
	return roots, nil
}

// GetByCode 按租户与代号取模型/分类。
// @param ctx 操作上下文
// @param tenantID 租户
// @param modelCode 模型代号
// @return 模型实体
// @return 是否存在
// @return 错误
func (s *ModelService) GetByCode(ctx context.Context, tenantID, modelCode string) (*ModelCategory, bool, error) {
	// 租户与代号必填
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(modelCode) == "" {
		return nil, false, apiError(errCodeMMParamsBlank, "按代号取模型缺少租户或代号")
	}
	return s.store.GetByCode(ctx, tenantID, modelCode)
}

// GetByID 按主键查询模型/分类（部署链路按模型 Id 定位用）。
// @param ctx 操作上下文
// @param id 模型主键
// @return 实体
// @return 是否存在
// @return 查询错误
func (s *ModelService) GetByID(ctx context.Context, id string) (*ModelCategory, bool, error) {
	if strings.TrimSpace(id) == "" {
		return nil, false, apiError(errCodeMMParamsBlank, "按 Id 取模型缺少主键")
	}
	return s.store.GetByPK(ctx, id)
}

// mmSortModelNodes 递归排序树节点（sort 升序，同号按代号）。
// @param nodes 节点列表
func mmSortModelNodes(nodes []*ModelTreeNode) {
	// 同层插入排序：节点数为租户分类量级
	for i := 1; i < len(nodes); i++ {
		for j := i; j > 0 && mmNodeLess(nodes[j], nodes[j-1]); j-- {
			nodes[j], nodes[j-1] = nodes[j-1], nodes[j]
		}
	}
	// 递归排序子层
	for _, node := range nodes {
		if len(node.Children) > 0 {
			mmSortModelNodes(node.Children)
		}
	}
}

// mmNodeLess 比较两个树节点的排序键。
// @param a 左操作数
// @param b 右操作数
// @return a 是否排在 b 前
func mmNodeLess(a, b *ModelTreeNode) bool {
	return mmModelLess(a.Model, b.Model)
}

// StepDesignService 是设计时步骤管理域服务。
type StepDesignService struct {
	store StepDesignStore // 步骤存储端口
	clock clock.Clock     // 时间源（审计列填充）
	newID func() string   // ID 生成函数
}

// NewStepDesignService 构造设计时步骤管理服务。
// @param store 步骤存储端口
// @param c 时间源，nil 时回退真实时钟
// @return 步骤服务
func NewStepDesignService(store StepDesignStore, c clock.Clock) *StepDesignService {
	if c == nil {
		c = clock.Real{}
	}
	return &StepDesignService{store: store, clock: c, newID: DefaultIDGen()}
}

// Add 新增设计时步骤（校验模型/代号/租户，模型内代号唯一，补齐主键与审计列）。
// @param ctx 操作上下文
// @param s 步骤数据（必填 model_id/step_code/tenant_id）
// @return 落库后的步骤实体
// @return 错误
func (s *StepDesignService) Add(ctx context.Context, d *StepDesign) (*StepDesign, error) {
	// 空请求/缺模型/缺代号/缺租户统一参数错误
	if d == nil || strings.TrimSpace(d.ModelID) == "" || strings.TrimSpace(d.StepCode) == "" || strings.TrimSpace(d.TenantID) == "" {
		return nil, apiError(errCodeMMStepBlank, "步骤新增缺少模型、代号或租户")
	}
	entity := *d
	// 模型内代号唯一（同代号不同模型放行）
	if _, found, err := s.store.GetByModelAndCode(ctx, entity.ModelID, entity.StepCode); err != nil {
		return nil, err
	} else if found {
		return nil, apiError(errCodeMMStepDup, "模型下步骤代号已存在: "+entity.StepCode)
	}
	fillBase(&entity.BaseColumns, s.newID, s.clock.Now, "", entity.TenantID)
	if err := s.store.Insert(ctx, &entity); err != nil {
		return nil, err
	}
	return &entity, nil
}

// Edit 编辑设计时步骤（保留创建审计，刷新更新时间；代号变更需通过模型内查重）。
// @param ctx 操作上下文
// @param s 步骤数据（必填主键）
// @return 编辑后的步骤实体
// @return 错误
func (s *StepDesignService) Edit(ctx context.Context, d *StepDesign) (*StepDesign, error) {
	// 空请求/缺主键统一参数错误
	if d == nil || mmBlankID(d.ID) {
		return nil, apiError(errCodeMMStepBlank, "步骤编辑缺少主键")
	}
	existing, found, err := s.store.GetByPK(ctx, d.ID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, api.NewError(errCodeMMStepMissing, "步骤不存在: "+d.ID, api.ErrNotFound)
	}
	if strings.TrimSpace(d.StepCode) == "" || strings.TrimSpace(d.ModelID) == "" {
		return nil, apiError(errCodeMMStepBlank, "步骤编辑缺少模型或代号")
	}
	// 模型内代号查重（排除自身）
	if other, found, err := s.store.GetByModelAndCode(ctx, d.ModelID, d.StepCode); err != nil {
		return nil, err
	} else if found && other.ID != d.ID {
		return nil, apiError(errCodeMMStepDup, "模型下步骤代号已存在: "+d.StepCode)
	}
	merged := *d
	// 创建审计保持既有值
	merged.Creator = existing.Creator
	merged.CreatorName = existing.CreatorName
	merged.CreateTime = existing.CreateTime
	merged.Updater = firstNonEmpty(d.Updater, existing.Updater)
	merged.UpdateTime = s.clock.Now()
	if err := s.store.Update(ctx, &merged); err != nil {
		return nil, err
	}
	return &merged, nil
}

// Delete 按主键批量删除步骤（物理删除，语义取舍见本文件域服务段说明）。
// @param ctx 操作上下文
// @param ids 主键集合
// @return 错误
func (s *StepDesignService) Delete(ctx context.Context, ids ...string) error {
	// 空集合明确拒绝
	if len(ids) == 0 {
		return apiError(errCodeMMStepBlank, "步骤删除缺少主键集合")
	}
	return s.store.DeleteByIDs(ctx, ids)
}

// ListByModel 查询模型下全部设计时步骤（按 sort 升序）。
// @param ctx 操作上下文
// @param modelID 模型 ID
// @return 步骤列表
// @return 错误
func (s *StepDesignService) ListByModel(ctx context.Context, modelID string) ([]*StepDesign, error) {
	// 模型 ID 必填
	if strings.TrimSpace(modelID) == "" {
		return nil, apiError(errCodeMMStepBlank, "步骤列表缺少模型 ID")
	}
	return s.store.ListByModel(ctx, modelID)
}

// TemplateService 是设计器模板管理域服务。
type TemplateService struct {
	store TemplateStore // 模板存储端口
	clock clock.Clock   // 时间源（审计列填充）
	newID func() string // ID 生成函数
}

// NewTemplateService 构造设计器模板管理服务。
// @param store 模板存储端口
// @param c 时间源，nil 时回退真实时钟
// @return 模板服务
func NewTemplateService(store TemplateStore, c clock.Clock) *TemplateService {
	if c == nil {
		c = clock.Real{}
	}
	return &TemplateService{store: store, clock: c, newID: DefaultIDGen()}
}

// Add 新增设计器模板（校验名称与租户，补齐主键与审计列；模板无唯一约束不查重）。
// @param ctx 操作上下文
// @param t 模板数据（必填 name/tenant_id）
// @return 落库后的模板实体
// @return 错误
func (s *TemplateService) Add(ctx context.Context, t *TemplateDef) (*TemplateDef, error) {
	// 空请求/缺名称/缺租户统一参数错误
	if t == nil || strings.TrimSpace(t.Name) == "" || strings.TrimSpace(t.TenantID) == "" {
		return nil, apiError(errCodeMMTplBlank, "模板新增缺少名称或租户")
	}
	entity := *t
	fillBase(&entity.BaseColumns, s.newID, s.clock.Now, "", entity.TenantID)
	if err := s.store.Insert(ctx, &entity); err != nil {
		return nil, err
	}
	return &entity, nil
}

// Edit 编辑设计器模板（保留创建审计，刷新更新时间）。
// @param ctx 操作上下文
// @param t 模板数据（必填主键）
// @return 编辑后的模板实体
// @return 错误
func (s *TemplateService) Edit(ctx context.Context, t *TemplateDef) (*TemplateDef, error) {
	// 空请求/缺主键统一参数错误
	if t == nil || mmBlankID(t.ID) {
		return nil, apiError(errCodeMMTplBlank, "模板编辑缺少主键")
	}
	existing, found, err := s.store.GetByPK(ctx, t.ID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, api.NewError(errCodeMMTplMissing, "模板不存在: "+t.ID, api.ErrNotFound)
	}
	merged := *t
	// 创建审计保持既有值
	merged.Creator = existing.Creator
	merged.CreatorName = existing.CreatorName
	merged.CreateTime = existing.CreateTime
	merged.Updater = firstNonEmpty(t.Updater, existing.Updater)
	merged.UpdateTime = s.clock.Now()
	if err := s.store.Update(ctx, &merged); err != nil {
		return nil, err
	}
	return &merged, nil
}

// Delete 按主键批量删除模板（物理删除，语义取舍见本文件域服务段说明）。
// @param ctx 操作上下文
// @param ids 主键集合
// @return 错误
func (s *TemplateService) Delete(ctx context.Context, ids ...string) error {
	// 空集合明确拒绝
	if len(ids) == 0 {
		return apiError(errCodeMMTplBlank, "模板删除缺少主键集合")
	}
	return s.store.DeleteByIDs(ctx, ids)
}

// List 查询租户下全部设计器模板（按名称升序）。
// @param ctx 操作上下文
// @param tenantID 租户
// @return 模板列表
// @return 错误
func (s *TemplateService) List(ctx context.Context, tenantID string) ([]*TemplateDef, error) {
	// 租户必填（存储层按租户隔离）
	if strings.TrimSpace(tenantID) == "" {
		return nil, apiError(errCodeMMTplBlank, "模板列表缺少租户")
	}
	return s.store.ListByTenant(ctx, tenantID)
}

// ListByCategory 查询分类下全部设计器模板（按名称升序）。
// @param ctx 操作上下文
// @param categoryID 分类 ID
// @return 模板列表
// @return 错误
func (s *TemplateService) ListByCategory(ctx context.Context, categoryID string) ([]*TemplateDef, error) {
	// 分类 ID 必填
	if strings.TrimSpace(categoryID) == "" {
		return nil, apiError(errCodeMMTplBlank, "模板列表缺少分类 ID")
	}
	return s.store.ListByCategory(ctx, categoryID)
}
