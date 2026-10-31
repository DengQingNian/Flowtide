package workflow

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"time"

	pwf "github.com/DengQingNian/Flowtide/internal/persistence/workflow"
)

// ─── 模型管理域 SQL 适配器 ─────────────────────────────────────────────
// 适配 internal/workflow 的模型管理端口（ModelMgmtStore/StepDesignStore/
// TemplateStore/HisStepStore）到 pwf 行仓储（workflow_model/workflow_step/
// workflow_template/workflow_his_step）。模式与 store_sql.go 一致：
// 共享 sqlCore、Row 字段映射、方言占位符、ctx 事务透传。
// 唯一的裸 SQL 补齐查询是模板按租户列表（TemplateRepository 缺 ListByTenant，
// 需要 NewSQLModelMgmtStoresWithDB 注入连接池），其余全部走仓储方法。

// SQLModelMgmtStoreSet 是模型管理域四个 SQL 适配器的聚合。
type SQLModelMgmtStoreSet struct {
	Model    *SQLModelMgmtStore  // 模型/分类主数据
	Step     *SQLStepDesignStore // 设计时步骤配置
	Template *SQLTemplateStore   // 设计器模板
	HisStep  *SQLHisStepStore    // 步骤部署快照
}

// NewSQLModelMgmtStores 创建模型管理域 SQL 适配器聚合（不带裸 SQL 补齐通道，
// Template.ListByTenant 将报 SQL-ADAPTER-001，其余能力完整）。
// @param store workflow 仓储入口（internal/persistence/workflow.New）
// @return 适配器聚合
func NewSQLModelMgmtStores(store *pwf.Store) *SQLModelMgmtStoreSet {
	core := &sqlCore{store: store, repos: pwf.NewRepositories(store)}
	return &SQLModelMgmtStoreSet{
		Model:    &SQLModelMgmtStore{core: core},
		Step:     &SQLStepDesignStore{core: core},
		Template: &SQLTemplateStore{core: core},
		HisStep:  &SQLHisStepStore{core: core, newID: DefaultIDGen()},
	}
}

// NewSQLModelMgmtStoresWithDB 创建模型管理域 SQL 适配器聚合并附带裸 SQL 补齐通道。
// @param store workflow 仓储入口
// @param db 连接池（与 store 共用同一连接池）
// @return 适配器聚合
func NewSQLModelMgmtStoresWithDB(store *pwf.Store, db *sql.DB) *SQLModelMgmtStoreSet {
	set := NewSQLModelMgmtStores(store)
	set.Template.core.db = db
	return set
}

// 编译期断言：SQL 适配满足模型管理域端口（HisStepStore 亦满足 StepStore 语义）。
var (
	_ ModelMgmtStore  = (*SQLModelMgmtStore)(nil)
	_ StepDesignStore = (*SQLStepDesignStore)(nil)
	_ TemplateStore   = (*SQLTemplateStore)(nil)
	_ HisStepStore    = (*SQLHisStepStore)(nil)
	_ StepStore       = (*SQLHisStepStore)(nil)
)

// ─── 值转换工具 ─────────────────────────────────────────────
// workflow_step/workflow_his_step 的规则列在 DDL 中为 tinyint/int/double/decimal，
// 而域实体沿用 StepRule 约定保留原始字符串形态；写入时做安全转换，
// 空串落 NULL（NULL 回读为空串，与"缺失即允许"的 Java 语义一致）。

// mmTinyValue 原始布尔/整型串转 tinyint 列值。
// @param raw 原始串（"0"/"1"/"true"/数字串）
// @return int64 值；空串或不可解析返回 nil（落 NULL）
func mmTinyValue(raw string) any {
	trimmed := strings.TrimSpace(raw)
	// 空串落 NULL：MySQL/PG 均安全，回读为空串保持"缺失"语义
	if trimmed == "" {
		return nil
	}
	if strings.EqualFold(trimmed, "true") {
		return int64(1)
	}
	parsed, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return nil
	}
	return parsed
}

// mmIntValue 原始整型串转 int 列值。
// @param raw 原始串
// @return int 值；空串或不可解析返回 nil（落 NULL）
func mmIntValue(raw string) any {
	trimmed := strings.TrimSpace(raw)
	// 空串落 NULL
	if trimmed == "" {
		return nil
	}
	parsed, err := strconv.Atoi(trimmed)
	if err != nil {
		return nil
	}
	return parsed
}

// mmDecValue 原始比例串转 decimal 列值。
// @param raw 原始串（百分数文本）
// @return float64 值；空串或不可解析返回 nil（落 NULL）
func mmDecValue(raw string) any {
	trimmed := strings.TrimSpace(raw)
	// 空串落 NULL
	if trimmed == "" {
		return nil
	}
	parsed, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return nil
	}
	return parsed
}

// mmFloatOf 读取行为浮点值（NULL → 0，兼容 double/decimal 多类型返回）。
// @param row 通用行
// @param key 列名
// @return 浮点值
func mmFloatOf(row pwf.Row, key string) float64 {
	// 复用 rowFloatPtr 的多类型解析，仅取值（指针为 nil 时给零值）
	if ptr := rowFloatPtr(row, key); ptr != nil {
		return *ptr
	}
	return 0
}

// ─── ModelCategory 行转换 ─────────────────────────────────────────────

// mmModelToRow 将模型实体转为通用行（可空时间列仅在有值时落库）。
// @param m 模型实体
// @return 通用行
func mmModelToRow(m *ModelCategory) pwf.Row {
	row := pwf.Row{
		"id":                     m.ID,
		"resource_id":            m.ResourceID,
		"model_name":             m.ModelName,
		"model_code":             m.ModelCode,
		"tips":                   m.Tips,
		"target":                 m.Target,
		"roles":                  m.Roles,
		"menu_icon":              m.MenuIcon,
		"remark":                 m.Remark,
		"sort":                   m.Sort,
		"parent_id":              m.ParentID,
		"enable":                 m.Enable,
		"return_sort":            m.ReturnSort,
		"is_leaf":                m.IsLeaf,
		"process_definition_key": m.ProcessDefinitionKey,
		"visible":                m.Visible,
		"model_id":               m.EngineModelID,
	}
	// 可空时间列仅在有值时落库，保持 NULL 语义
	if m.LastDeployTime != nil {
		row["last_deploy_time"] = *m.LastDeployTime
	}
	appendBaseRow(row, m.BaseColumns)
	return row
}

// mmModelFromRow 将通用行转为模型实体（缺失列给零值）。
// @param row 通用行
// @return 模型实体
func mmModelFromRow(row pwf.Row) *ModelCategory {
	return &ModelCategory{
		BaseColumns:          baseFromRow(row),
		ResourceID:           rowString(row, "resource_id"),
		ModelName:            rowString(row, "model_name"),
		ModelCode:            rowString(row, "model_code"),
		Tips:                 rowString(row, "tips"),
		Target:               rowString(row, "target"),
		Roles:                rowString(row, "roles"),
		MenuIcon:             rowString(row, "menu_icon"),
		Remark:               rowString(row, "remark"),
		Sort:                 rowInt(row, "sort"),
		ParentID:             rowString(row, "parent_id"),
		Enable:               rowInt(row, "enable"),
		ReturnSort:           rowInt(row, "return_sort"),
		IsLeaf:               rowInt(row, "is_leaf"),
		ProcessDefinitionKey: rowString(row, "process_definition_key"),
		Visible:              rowInt(row, "visible"),
		EngineModelID:        rowString(row, "model_id"),
		LastDeployTime:       rowTimePtr(row, "last_deploy_time"),
	}
}

// mmModelsFromRows 批量转换模型行。
// @param rows 通用行列表
// @return 实体列表
func mmModelsFromRows(rows []pwf.Row) []*ModelCategory {
	out := make([]*ModelCategory, 0, len(rows))
	for _, row := range rows {
		out = append(out, mmModelFromRow(row))
	}
	return out
}

// ─── SQLModelMgmtStore：模型/分类主数据 ─────────────────────────────────────────────

// SQLModelMgmtStore 是 ModelMgmtStore 的 SQL 适配器（workflow_model 行仓储）。
type SQLModelMgmtStore struct {
	core *sqlCore // 共享内核
}

// Insert 插入一条模型/分类。
// @param ctx 操作上下文（携带事务时透传同一工作单元）
// @param m 模型实体（必填 id/tenant_id）
// @return 错误
func (s *SQLModelMgmtStore) Insert(ctx context.Context, m *ModelCategory) error {
	return s.core.repos.Models.Insert(ctx, mmModelToRow(m))
}

// Update 按主键更新整行业务列（仓储剔除 id 列，create_time 原值回写不改变创建审计）。
// @param ctx 操作上下文
// @param m 模型实体（必填 id）
// @return 错误
func (s *SQLModelMgmtStore) Update(ctx context.Context, m *ModelCategory) error {
	_, err := s.core.repos.Models.UpdateByPK(ctx, m.ID, mmModelToRow(m))
	return err
}

// GetByPK 按主键查询。
// @param ctx 操作上下文
// @param id 主键
// @return 模型实体
// @return 是否存在
// @return 查询错误
func (s *SQLModelMgmtStore) GetByPK(ctx context.Context, id string) (*ModelCategory, bool, error) {
	row, found, err := s.core.repos.Models.GetByPK(ctx, id)
	if err != nil || !found {
		return nil, false, err
	}
	return mmModelFromRow(row), true, nil
}

// GetByCode 按租户内模型代号查询（复用仓储 (model_code, tenant_id) 查询）。
// @param ctx 操作上下文
// @param tenantID 租户
// @param modelCode 模型代号
// @return 模型实体
// @return 是否存在
// @return 查询错误
func (s *SQLModelMgmtStore) GetByCode(ctx context.Context, tenantID, modelCode string) (*ModelCategory, bool, error) {
	row, found, err := s.core.repos.Models.GetByModelCode(ctx, modelCode, tenantID)
	if err != nil || !found {
		return nil, false, err
	}
	return mmModelFromRow(row), true, nil
}

// ListByTenant 查询租户下全部模型与分类（仓储按 sort 升序返回）。
// @param ctx 操作上下文
// @param tenantID 租户
// @return 模型列表
// @return 查询错误
func (s *SQLModelMgmtStore) ListByTenant(ctx context.Context, tenantID string) ([]*ModelCategory, error) {
	rows, err := s.core.repos.Models.ListByTenant(ctx, tenantID)
	return mmModelsFromRows(rows), err
}

// UpdateLastDeployTime 回写模型最近部署时间（复用仓储专项更新，更新时间由仓储填充）。
// @param ctx 操作上下文
// @param id 模型主键
// @param deployTime 部署时间
// @return 错误
func (s *SQLModelMgmtStore) UpdateLastDeployTime(ctx context.Context, id string, deployTime time.Time) error {
	_, err := s.core.repos.Models.UpdateLastDeployTime(ctx, id, deployTime)
	return err
}

// DeleteByIDs 按主键批量物理删除（仓储无批量删除，逐条删除，依赖 ctx 事务保证批次原子性）。
// @param ctx 操作上下文
// @param ids 主键集合
// @return 错误
func (s *SQLModelMgmtStore) DeleteByIDs(ctx context.Context, ids []string) error {
	// 逐条删除：管理台批量删除为主键数量级
	for _, id := range ids {
		if _, err := s.core.repos.Models.DeleteByPK(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

// ─── StepDesign 行转换 ─────────────────────────────────────────────

// mmStepToRow 将设计时步骤实体转为通用行（可空列仅在有值时落库）。
// @param d 步骤实体
// @return 通用行
func mmStepToRow(d *StepDesign) pwf.Row {
	row := pwf.Row{
		"id":                     d.ID,
		"name":                   d.Name,
		"step_code":              d.StepCode,
		"description":            d.Description,
		"value":                  d.Value,
		"account_name":           d.AccountName,
		"source":                 d.Source,
		"model_id":               d.ModelID,
		"resource_id":            d.ResourceID,
		"sort":                   d.Sort,
		"agree":                  d.Agree,
		"reject":                 d.Reject,
		"remark":                 d.Remark,
		"fix":                    d.Fix,
		"url":                    d.URL,
		"visible":                d.Visible,
		"task_type":              d.TaskType,
		"edit_info":              d.EditInfo,
		"operate_auth":           d.OperateAuth,
		"back_step_codes":        d.BackStepCodes,
		"back_step_names":        d.BackStepNames,
		"auto_complete":          d.AutoComplete,
		"time_limit":             d.TimeLimit,
		"time_limit_unit":        d.TimeLimitUnit,
		"remind_time_limit":      d.RemindTimeLimit,
		"remind_time_limit_unit": d.RemindTimeLimitUnit,
		"channel_type_name":      d.ChannelTypeName,
		"channel_type_name_cn":   d.ChannelTypeNameCN,
	}
	// 规则列：原始串安全转 tinyint/int/decimal，空串落 NULL（缺失即允许语义）
	if value := mmTinyValue(d.AllowReturn); value != nil {
		row["allow_return"] = value
	}
	if value := mmTinyValue(d.CountersignPercentReturn); value != nil {
		row["countersign_percent_return"] = value
	}
	if value := mmIntValue(d.ReturnApprovalStep); value != nil {
		row["return_approval_step"] = value
	}
	if value := mmDecValue(d.ReturnPercent); value != nil {
		row["return_percent"] = value
	}
	// 可空时间列仅在有值时落库
	if d.AutoCompleteStartTime != nil {
		row["auto_complete_start_time"] = *d.AutoCompleteStartTime
	}
	if d.AutoCompleteEndTime != nil {
		row["auto_complete_end_time"] = *d.AutoCompleteEndTime
	}
	appendBaseRow(row, d.BaseColumns)
	return row
}

// mmStepFromRow 将通用行转为设计时步骤实体（缺失列给零值）。
// @param row 通用行
// @return 步骤实体
func mmStepFromRow(row pwf.Row) *StepDesign {
	return &StepDesign{
		BaseColumns:              baseFromRow(row),
		Name:                     rowString(row, "name"),
		StepCode:                 rowString(row, "step_code"),
		Description:              rowString(row, "description"),
		Value:                    rowString(row, "value"),
		AccountName:              rowString(row, "account_name"),
		Source:                   rowInt(row, "source"),
		ModelID:                  rowString(row, "model_id"),
		ResourceID:               rowString(row, "resource_id"),
		Sort:                     mmFloatOf(row, "sort"),
		Agree:                    rowString(row, "agree"),
		Reject:                   rowString(row, "reject"),
		Remark:                   rowString(row, "remark"),
		Fix:                      rowString(row, "fix"),
		URL:                      rowString(row, "url"),
		Visible:                  rowInt(row, "visible"),
		TaskType:                 rowString(row, "task_type"),
		EditInfo:                 rowString(row, "edit_info"),
		OperateAuth:              rowString(row, "operate_auth"),
		AllowReturn:              rowString(row, "allow_return"),
		BackStepCodes:            rowString(row, "back_step_codes"),
		BackStepNames:            rowString(row, "back_step_names"),
		AutoComplete:             rowInt(row, "auto_complete"),
		AutoCompleteStartTime:    rowTimePtr(row, "auto_complete_start_time"),
		AutoCompleteEndTime:      rowTimePtr(row, "auto_complete_end_time"),
		TimeLimit:                mmFloatOf(row, "time_limit"),
		TimeLimitUnit:            rowString(row, "time_limit_unit"),
		RemindTimeLimit:          mmFloatOf(row, "remind_time_limit"),
		RemindTimeLimitUnit:      rowString(row, "remind_time_limit_unit"),
		ChannelTypeName:          rowString(row, "channel_type_name"),
		ChannelTypeNameCN:        rowString(row, "channel_type_name_cn"),
		ReturnApprovalStep:       rowString(row, "return_approval_step"),
		CountersignPercentReturn: rowString(row, "countersign_percent_return"),
		ReturnPercent:            rowString(row, "return_percent"),
	}
}

// mmStepsFromRows 批量转换设计时步骤行。
// @param rows 通用行列表
// @return 实体列表
func mmStepsFromRows(rows []pwf.Row) []*StepDesign {
	out := make([]*StepDesign, 0, len(rows))
	for _, row := range rows {
		out = append(out, mmStepFromRow(row))
	}
	return out
}

// ─── SQLStepDesignStore：设计时步骤 ─────────────────────────────────────────────

// SQLStepDesignStore 是 StepDesignStore 的 SQL 适配器（workflow_step 行仓储）。
type SQLStepDesignStore struct {
	core *sqlCore // 共享内核
}

// Insert 插入一条设计时步骤。
// @param ctx 操作上下文（携带事务时透传同一工作单元）
// @param d 步骤实体（必填 id/model_id/step_code/tenant_id）
// @return 错误
func (s *SQLStepDesignStore) Insert(ctx context.Context, d *StepDesign) error {
	return s.core.repos.Steps.Insert(ctx, mmStepToRow(d))
}

// Update 按主键更新整行业务列。
// @param ctx 操作上下文
// @param d 步骤实体（必填 id）
// @return 错误
func (s *SQLStepDesignStore) Update(ctx context.Context, d *StepDesign) error {
	_, err := s.core.repos.Steps.UpdateByPK(ctx, d.ID, mmStepToRow(d))
	return err
}

// GetByPK 按主键查询。
// @param ctx 操作上下文
// @param id 主键
// @return 步骤实体
// @return 是否存在
// @return 查询错误
func (s *SQLStepDesignStore) GetByPK(ctx context.Context, id string) (*StepDesign, bool, error) {
	row, found, err := s.core.repos.Steps.GetByPK(ctx, id)
	if err != nil || !found {
		return nil, false, err
	}
	return mmStepFromRow(row), true, nil
}

// GetByModelAndCode 按模型与步骤代号查询单条配置。
// @param ctx 操作上下文
// @param modelID 模型 ID
// @param stepCode 步骤代号
// @return 步骤实体
// @return 是否存在
// @return 查询错误
func (s *SQLStepDesignStore) GetByModelAndCode(ctx context.Context, modelID, stepCode string) (*StepDesign, bool, error) {
	row, found, err := s.core.repos.Steps.GetByModelAndStepCode(ctx, modelID, stepCode)
	if err != nil || !found {
		return nil, false, err
	}
	return mmStepFromRow(row), true, nil
}

// ListByModel 查询模型下全部步骤配置（仓储按 sort 升序返回）。
// @param ctx 操作上下文
// @param modelID 模型 ID
// @return 步骤列表
// @return 查询错误
func (s *SQLStepDesignStore) ListByModel(ctx context.Context, modelID string) ([]*StepDesign, error) {
	rows, err := s.core.repos.Steps.ListByModel(ctx, modelID)
	return mmStepsFromRows(rows), err
}

// DeleteByIDs 按主键批量物理删除（逐条删除，依赖 ctx 事务保证批次原子性）。
// @param ctx 操作上下文
// @param ids 主键集合
// @return 错误
func (s *SQLStepDesignStore) DeleteByIDs(ctx context.Context, ids []string) error {
	// 逐条删除：管理台批量删除为主键数量级
	for _, id := range ids {
		if _, err := s.core.repos.Steps.DeleteByPK(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

// ─── TemplateDef 行转换 ─────────────────────────────────────────────

// mmTemplateToRow 将模板实体转为通用行。
// @param t 模板实体
// @return 通用行
func mmTemplateToRow(t *TemplateDef) pwf.Row {
	row := pwf.Row{
		"id":          t.ID,
		"name":        t.Name,
		"layout":      t.Layout,
		"flow_xml":    t.FlowXML,
		"is_visible":  t.IsVisible,
		"is_default":  t.IsDefault,
		"source":      t.Source,
		"category_id": t.CategoryID,
	}
	appendBaseRow(row, t.BaseColumns)
	return row
}

// mmTemplateFromRow 将通用行转为模板实体（缺失列给零值）。
// @param row 通用行
// @return 模板实体
func mmTemplateFromRow(row pwf.Row) *TemplateDef {
	return &TemplateDef{
		BaseColumns: baseFromRow(row),
		Name:        rowString(row, "name"),
		Layout:      rowString(row, "layout"),
		FlowXML:     rowString(row, "flow_xml"),
		IsVisible:   rowInt(row, "is_visible"),
		IsDefault:   rowInt(row, "is_default"),
		Source:      rowInt(row, "source"),
		CategoryID:  rowString(row, "category_id"),
	}
}

// mmTemplatesFromRows 批量转换模板行。
// @param rows 通用行列表
// @return 实体列表
func mmTemplatesFromRows(rows []pwf.Row) []*TemplateDef {
	out := make([]*TemplateDef, 0, len(rows))
	for _, row := range rows {
		out = append(out, mmTemplateFromRow(row))
	}
	return out
}

// ─── SQLTemplateStore：设计器模板 ─────────────────────────────────────────────

// SQLTemplateStore 是 TemplateStore 的 SQL 适配器（workflow_template 行仓储）。
// 按租户列表为仓储缺口，经裸 SQL 只读补齐（需 WithDB 注入连接池）。
type SQLTemplateStore struct {
	core *sqlCore // 共享内核
}

// Insert 插入一条模板。
// @param ctx 操作上下文（携带事务时透传同一工作单元）
// @param t 模板实体（必填 id/tenant_id）
// @return 错误
func (s *SQLTemplateStore) Insert(ctx context.Context, t *TemplateDef) error {
	return s.core.repos.Templates.Insert(ctx, mmTemplateToRow(t))
}

// Update 按主键更新整行业务列。
// @param ctx 操作上下文
// @param t 模板实体（必填 id）
// @return 错误
func (s *SQLTemplateStore) Update(ctx context.Context, t *TemplateDef) error {
	_, err := s.core.repos.Templates.UpdateByPK(ctx, t.ID, mmTemplateToRow(t))
	return err
}

// GetByPK 按主键查询。
// @param ctx 操作上下文
// @param id 主键
// @return 模板实体
// @return 是否存在
// @return 查询错误
func (s *SQLTemplateStore) GetByPK(ctx context.Context, id string) (*TemplateDef, bool, error) {
	row, found, err := s.core.repos.Templates.GetByPK(ctx, id)
	if err != nil || !found {
		return nil, false, err
	}
	return mmTemplateFromRow(row), true, nil
}

// ListByTenant 查询租户下全部模板（仓储缺口，裸 SQL 只读补齐，按名称升序）。
// @param ctx 操作上下文
// @param tenantID 租户
// @return 模板列表
// @return 查询错误（未注入连接池时报 SQL-ADAPTER-001）
func (s *SQLTemplateStore) ListByTenant(ctx context.Context, tenantID string) ([]*TemplateDef, error) {
	rows, err := s.core.queryRaw(ctx, "SELECT * FROM workflow_template WHERE tenant_id = {p1} ORDER BY name", []any{tenantID})
	return mmTemplatesFromRows(rows), err
}

// ListByCategory 查询分类下全部模板（仓储按名称升序返回）。
// @param ctx 操作上下文
// @param categoryID 分类 ID
// @return 模板列表
// @return 查询错误
func (s *SQLTemplateStore) ListByCategory(ctx context.Context, categoryID string) ([]*TemplateDef, error) {
	rows, err := s.core.repos.Templates.ListByCategory(ctx, categoryID)
	return mmTemplatesFromRows(rows), err
}

// DeleteByIDs 按主键批量物理删除（逐条删除，依赖 ctx 事务保证批次原子性）。
// @param ctx 操作上下文
// @param ids 主键集合
// @return 错误
func (s *SQLTemplateStore) DeleteByIDs(ctx context.Context, ids []string) error {
	// 逐条删除：管理台批量删除为主键数量级
	for _, id := range ids {
		if _, err := s.core.repos.Templates.DeleteByPK(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

// ─── SQLHisStepStore：步骤部署快照 ─────────────────────────────────────────────

// SQLHisStepStore 是 HisStepStore 的 SQL 适配器（workflow_his_step 行仓储）。
// 快照行为 StepRule 规则子集，行主键由适配器生成（StepRule 无 ID 字段）；
// 批量替换经"先查后删再插"补齐（仓储无按部署删除方法），依赖 ctx 事务保证原子性。
type SQLHisStepStore struct {
	core  *sqlCore      // 共享内核
	newID func() string // 快照行主键生成
}

// NewSQLHisStepStore 创建快照 SQL 适配器（独立于聚合使用）。
// @param store workflow 仓储入口
// @return 快照适配器
func NewSQLHisStepStore(store *pwf.Store) *SQLHisStepStore {
	return &SQLHisStepStore{core: &sqlCore{store: store, repos: pwf.NewRepositories(store)}, newID: DefaultIDGen()}
}

// GetByDeploymentAndActivity 按部署与节点定位单条快照规则（复用仓储部署+代号查询）。
// @param ctx 操作上下文
// @param deploymentID 部署 ID
// @param activityID 节点 ID（=step_code）
// @return 快照规则
// @return 是否存在
// @return 查询错误
func (s *SQLHisStepStore) GetByDeploymentAndActivity(ctx context.Context, deploymentID, activityID string) (*StepRule, bool, error) {
	row, found, err := s.core.repos.HisSteps.GetByDeploymentAndStepCode(ctx, deploymentID, activityID)
	if err != nil || !found {
		return nil, false, err
	}
	return stepRuleFromRow(row), true, nil
}

// ListByDeployment 查询部署下全部快照规则（仓储按 sort 升序返回）。
// @param ctx 操作上下文
// @param deploymentID 部署 ID
// @return 快照规则列表
// @return 查询错误
func (s *SQLHisStepStore) ListByDeployment(ctx context.Context, deploymentID string) ([]*StepRule, error) {
	rows, err := s.core.repos.HisSteps.ListByDeployment(ctx, deploymentID)
	if err != nil {
		return nil, err
	}
	out := make([]*StepRule, 0, len(rows))
	for _, row := range rows {
		out = append(out, stepRuleFromRow(row))
	}
	return out, nil
}

// Put 批量替换部署的全部快照规则（先删后插实现替换语义，同部署重复快照幂等；
// 规则统一归属该部署，行主键由适配器生成）。
// @param ctx 操作上下文（建议以 pwf WithTx 包裹保证删插原子性）
// @param deploymentID 部署 ID
// @param rules 快照规则列表（空列表清空该部署快照）
// @return 错误
func (s *SQLHisStepStore) Put(ctx context.Context, deploymentID string, rules []*StepRule) error {
	// 部署 ID 是快照定位键，缺失即调用方违约
	if strings.TrimSpace(deploymentID) == "" {
		return apiError(errCodeMMSnapBlank, "快照写入缺少部署 ID")
	}
	rows, err := s.core.repos.HisSteps.ListByDeployment(ctx, deploymentID)
	if err != nil {
		return err
	}
	// 先删既有快照（逐条按主键，仓储无按部署删除方法）
	for _, row := range rows {
		if _, err := s.core.repos.HisSteps.DeleteByPK(ctx, rowString(row, "id")); err != nil {
			return err
		}
	}
	// 再插新快照（StepRule 无租户字段，快照以部署维度隔离）
	for _, rule := range rules {
		if err := s.core.repos.HisSteps.Insert(ctx, mmHisRowFromRule(rule, deploymentID, s.newID())); err != nil {
			return err
		}
	}
	return nil
}

// DeleteByDeployment 删除部署的全部快照规则（先查后逐条删除补齐仓储缺口）。
// @param ctx 操作上下文
// @param deploymentID 部署 ID
// @return 错误
func (s *SQLHisStepStore) DeleteByDeployment(ctx context.Context, deploymentID string) error {
	rows, err := s.core.repos.HisSteps.ListByDeployment(ctx, deploymentID)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := s.core.repos.HisSteps.DeleteByPK(ctx, rowString(row, "id")); err != nil {
			return err
		}
	}
	return nil
}

// mmHisRowFromRule 将快照规则转为 workflow_his_step 通用行。
// 仅落规则子集列（step_id/model_id 等设计侧关联列在 StepRule 中缺失，保持 NULL，
// 运行期规则读取只依赖 deployment_id + step_code）。
// @param rule 快照规则
// @param deploymentID 部署 ID（StepRule 归属统一由参数决定）
// @param id 快照行主键
// @return 通用行
func mmHisRowFromRule(rule *StepRule, deploymentID, id string) pwf.Row {
	row := pwf.Row{
		"id":              id,
		"deployment_id":   deploymentID,
		"step_code":       rule.StepCode,
		"step_name":       rule.StepName,
		"sort":            rule.Sort,
		"url":             rule.URL,
		"back_step_codes": rule.BackStepCodes,
	}
	// 规则列：原始串安全转列类型，空串落 NULL（NULL 回读空串=允许退回语义）
	if value := mmTinyValue(rule.AllowReturn); value != nil {
		row["allow_return"] = value
	}
	if value := mmTinyValue(rule.CountersignPercentReturn); value != nil {
		row["countersign_percent_return"] = value
	}
	if value := mmIntValue(rule.ReturnApprovalStep); value != nil {
		row["return_approval_step"] = value
	}
	if value := mmDecValue(rule.ReturnPercent); value != nil {
		row["return_percent"] = value
	}
	return row
}
