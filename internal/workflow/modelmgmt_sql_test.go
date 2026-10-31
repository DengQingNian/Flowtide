package workflow

import (
	"context"
	"database/sql/driver"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	pwf "github.com/DengQingNian/Flowtide/internal/persistence/workflow"
)

// ─── 测试基建 ─────────────────────────────────────────────
// 与 store_sql_integration_test.go 不同，本文件用 go-sqlmock 在无真实库环境
// 验证 SQL 适配器的语句拼装、行映射与错误透传；方言固定 MySQL（"\\?" 占位）。

// mmModelCols workflow_model 全列（与 pwf tableModels / DDL 列序一致，首列恒为 id）。
var mmModelCols = []string{"id", "resource_id", "model_name", "model_code", "tips", "target", "roles", "menu_icon", "remark", "sort", "parent_id", "enable", "return_sort", "is_leaf", "process_definition_key", "visible", "model_id", "last_deploy_time", "creator", "creator_name", "create_time", "updater", "update_time", "tenant_id"}

// mmStepCols workflow_step 全列（与 pwf tableSteps / DDL 列序一致）。
var mmStepCols = []string{"id", "name", "step_code", "description", "value", "account_name", "source", "model_id", "resource_id", "sort", "agree", "reject", "remark", "fix", "url", "visible", "task_type", "edit_info", "operate_auth", "allow_return", "back_step_codes", "back_step_names", "auto_complete", "auto_complete_start_time", "auto_complete_end_time", "time_limit", "time_limit_unit", "remind_time_limit", "remind_time_limit_unit", "channel_type_name", "channel_type_name_cn", "return_approval_step", "countersign_percent_return", "return_percent", "creator", "creator_name", "create_time", "updater", "update_time", "tenant_id"}

// mmTemplateCols workflow_template 全列（与 pwf tableTemplates / DDL 列序一致）。
var mmTemplateCols = []string{"id", "name", "layout", "flow_xml", "is_visible", "is_default", "source", "category_id", "creator", "creator_name", "create_time", "updater", "update_time", "tenant_id"}

// mmHisStepCols workflow_his_step 全列（与 pwf tableHisSteps / DDL 列序一致）。
var mmHisStepCols = []string{"id", "step_id", "step_name", "step_code", "account_name", "model_id", "sort", "agree", "reject", "remark", "url", "visible", "task_type", "status", "step_creator", "step_create_time", "step_updater", "step_update_time", "deployment_id", "edit_info", "operate_auth", "process_instance_id", "description", "value", "fix", "resource_id", "source", "allow_return", "back_step_codes", "back_step_names", "auto_complete", "auto_complete_start_time", "auto_complete_end_time", "time_limit", "time_limit_unit", "remind_time_limit", "remind_time_limit_unit", "channel_type_name", "channel_type_name_cn", "return_approval_step", "countersign_percent_return", "return_percent", "creator", "creator_name", "create_time", "updater", "update_time", "tenant_id"}

// mmNewSQLModelMgmt 创建 sqlmock 连接与模型管理域适配器聚合（固定假时钟）。
// @param t 测试上下文
// @return 操作上下文
// @return sqlmock 连接
// @return sqlmock 控制器
// @return 适配器聚合
func mmNewSQLModelMgmt(t *testing.T) (context.Context, sqlmock.Sqlmock, *SQLModelMgmtStoreSet) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("创建 sqlmock 失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	// 假时钟固定 create_time/update_time，使参数断言确定
	store, err := pwf.NewStore(db, pwf.DialectMySQL, pwf.WithClock(pwf.ClockFunc(func() time.Time { return mmTestStamp })))
	if err != nil {
		t.Fatalf("创建 workflow 仓储入口失败: %v", err)
	}
	return context.Background(), mock, NewSQLModelMgmtStoresWithDB(store, db)
}

// mmRowsOf 按列序构造结果集（缺省列填 NULL）。
// @param cols 列序
// @param rows 行值表（列名→值）
// @return sqlmock 结果集
func mmRowsOf(cols []string, rows ...map[string]any) *sqlmock.Rows {
	result := sqlmock.NewRows(cols)
	for _, vals := range rows {
		args := make([]driver.Value, 0, len(cols))
		// 按列序取值，缺失列落 NULL，保证与 SELECT 列序一致
		for _, column := range cols {
			if value, ok := vals[column]; ok {
				args = append(args, value)
			} else {
				args = append(args, nil)
			}
		}
		result.AddRow(args...)
	}
	return result
}

// mmPlaceholders 生成 n 个 "\\?" 占位符的 VALUES 段。
// @param n 占位符数量
// @return "\\?, \\?, ..." 文本
func mmPlaceholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("\\?, ", n), ", ")
}

// mmModelRowVals 构造模型全列行值（与 mmModelSample 字段对应）。
// @param stamp 时间戳
// @return 列名→值
func mmModelRowVals(stamp time.Time) map[string]any {
	return map[string]any{
		"id": "m1", "resource_id": "res-m1", "model_name": "模型-leave", "model_code": "leave",
		"tips": "提示", "target": "_blank", "roles": `["r1"]`, "menu_icon": "icon", "remark": "备注",
		"sort": int64(2), "parent_id": "", "enable": int64(1), "return_sort": int64(1), "is_leaf": int64(1),
		"process_definition_key": "leave", "visible": int64(1), "model_id": "engine-m1", "last_deploy_time": stamp,
		"creator": "op", "creator_name": "张三", "create_time": stamp, "updater": "op", "update_time": stamp, "tenant_id": "sys",
	}
}

// TestModelMgmtSQLValueHelpers 验证 SQL 适配器的原始值转换与行映射边界。
// @param t 测试上下文
func TestModelMgmtSQLValueHelpers(t *testing.T) {
	// tinyint 原始值转换：0/1/true/数字串/空/非法
	if v := mmTinyValue("0"); v != int64(0) {
		t.Fatalf("tinyvalue 0 转换错误: %v", v)
	}
	if v := mmTinyValue("1"); v != int64(1) {
		t.Fatalf("tinyvalue 1 转换错误: %v", v)
	}
	if v := mmTinyValue("true"); v != int64(1) {
		t.Fatalf("tinyvalue true 转换错误: %v", v)
	}
	if v := mmTinyValue(" 2 "); v != int64(2) {
		t.Fatalf("tinyvalue 空白数字转换错误: %v", v)
	}
	if v := mmTinyValue(""); v != nil {
		t.Fatalf("tinyvalue 空串应落 NULL: %v", v)
	}
	if v := mmTinyValue("x"); v != nil {
		t.Fatalf("tinyvalue 非法串应落 NULL: %v", v)
	}
	// int 原始值转换
	if v := mmIntValue("3"); v != 3 {
		t.Fatalf("intvalue 转换错误: %v", v)
	}
	if mmIntValue("") != nil || mmIntValue("x") != nil {
		t.Fatal("intvalue 空/非法应落 NULL")
	}
	// decimal 原始值转换
	if v := mmDecValue("50.5"); v != 50.5 {
		t.Fatalf("decvalue 转换错误: %v", v)
	}
	if mmDecValue("") != nil || mmDecValue("x") != nil {
		t.Fatal("decvalue 空/非法应落 NULL")
	}
	// 行浮点读取：NULL → 0
	row := pwf.Row{"sort": 2.5, "nil": nil}
	if mmFloatOf(row, "sort") != 2.5 || mmFloatOf(row, "nil") != 0 || mmFloatOf(row, "missing") != 0 {
		t.Fatal("mmFloatOf 语义错误")
	}

	// 模型行往返：nil 部署时间
	stamp := mmTestStamp
	model := mmModelSample("m1", "leave", "sys")
	model.LastDeployTime = nil
	fromRow := mmModelFromRow(mmModelToRow(model))
	if fromRow.LastDeployTime != nil || fromRow.ModelCode != "leave" || fromRow.Sort != 2 || fromRow.EngineModelID != "engine-m1" {
		t.Fatalf("模型行往返错误: %+v", fromRow)
	}
	model.LastDeployTime = &stamp
	fromRow = mmModelFromRow(mmModelToRow(model))
	if fromRow.LastDeployTime == nil || !fromRow.LastDeployTime.Equal(stamp) {
		t.Fatalf("模型部署时间往返错误: %v", fromRow.LastDeployTime)
	}

	// 步骤行往返：nil 时间与空规则列
	step := &StepDesign{BaseColumns: BaseColumns{ID: "d1", TenantID: "sys"}, Name: "部门审批", StepCode: "deptAudit", ModelID: "m1", Sort: 2.5, AllowReturn: "1", ReturnPercent: "50", ReturnApprovalStep: "3", CountersignPercentReturn: "1"}
	fromStep := mmStepFromRow(mmStepToRow(step))
	// 注意：空规则列写入省略 → 读回空串；"1"/"3"/"50" 以原始串往返
	if fromStep.AllowReturn != "1" || fromStep.ReturnPercent != "50" || fromStep.ReturnApprovalStep != "3" || fromStep.CountersignPercentReturn != "1" {
		t.Fatalf("步骤规则列往返错误: %+v", fromStep)
	}
	if fromStep.AutoCompleteStartTime != nil || fromStep.AutoCompleteEndTime != nil || fromStep.BackStepCodes != "" {
		t.Fatalf("步骤可空列往返错误: %+v", fromStep)
	}
	if fromStep.Sort != 2.5 || fromStep.TimeLimit != 0 || fromStep.RemindTimeLimit != 0 {
		t.Fatalf("步骤浮点列往返错误: %+v", fromStep)
	}

	// 模板行往返
	tpl := &TemplateDef{BaseColumns: BaseColumns{ID: "t1", TenantID: "sys"}, Name: "经典", Layout: "{}", FlowXML: "<x/>", IsVisible: 1, IsDefault: 1, Source: 2, CategoryID: "cat"}
	fromTpl := mmTemplateFromRow(mmTemplateToRow(tpl))
	if fromTpl.Name != "经典" || fromTpl.IsDefault != 1 || fromTpl.Source != 2 || fromTpl.CategoryID != "cat" {
		t.Fatalf("模板行往返错误: %+v", fromTpl)
	}
}

// TestModelMgmtSQLModelStore 验证模型 SQL 适配器的语句、参数与行映射。
// @param t 测试上下文
func TestModelMgmtSQLModelStore(t *testing.T) {
	ctx, mock, set := mmNewSQLModelMgmt(t)
	stamp := mmTestStamp
	model := mmModelSample("m1", "leave", "sys")
	model.LastDeployTime = &stamp

	// 插入：列序与参数全量断言（映射测试核心）
	expectedArgs := []driver.Value{
		"m1", "res-m1", "模型-leave", "leave", "提示", "_blank", `["r1"]`, "icon", "备注", 2, "", 1, 1, 1, "leave", 1, "engine-m1", stamp,
		"op", "张三", stamp, "op", stamp, "sys",
	}
	mock.ExpectExec("INSERT INTO workflow_model ").WithArgs(expectedArgs...).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := set.Model.Insert(ctx, model); err != nil {
		t.Fatalf("插入失败: %v", err)
	}
	// 生成 SQL 的 VALUES 占位符数量与列数一致（防御列漂移）
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("插入参数断言失败: %v", err)
	}

	// 按主键查询：行映射
	mock.ExpectQuery("SELECT .* FROM workflow_model WHERE id = \\?").WithArgs("m1").WillReturnRows(mmRowsOf(mmModelCols, mmModelRowVals(stamp)))
	got, found, err := set.Model.GetByPK(ctx, "m1")
	if err != nil || !found {
		t.Fatalf("按主键查询失败: found=%v err=%v", found, err)
	}
	if got.ModelCode != "leave" || got.Sort != 2 || got.Enable != 1 || got.LastDeployTime == nil || !got.LastDeployTime.Equal(stamp) {
		t.Fatalf("模型行映射错误: %+v", got)
	}

	// 按主键查询：未命中与错误透传
	mock.ExpectQuery("SELECT .* FROM workflow_model WHERE id = \\?").WithArgs("nope").WillReturnRows(sqlmock.NewRows(mmModelCols))
	if _, found, err := set.Model.GetByPK(ctx, "nope"); found || err != nil {
		t.Fatalf("未命中应 (false,nil): found=%v err=%v", found, err)
	}
	mock.ExpectQuery("SELECT .* FROM workflow_model WHERE id = \\?").WithArgs("bad").WillReturnError(context.DeadlineExceeded)
	if _, _, err := set.Model.GetByPK(ctx, "bad"); err == nil {
		t.Fatal("查询错误应透传")
	}

	// 按代号与租户查询
	mock.ExpectQuery("SELECT .* FROM workflow_model WHERE model_code = \\? AND tenant_id = \\? LIMIT 1").WithArgs("leave", "sys").WillReturnRows(mmRowsOf(mmModelCols, mmModelRowVals(stamp)))
	if _, found, err := set.Model.GetByCode(ctx, "sys", "leave"); err != nil || !found {
		t.Fatalf("按代号查询失败: found=%v err=%v", found, err)
	}

	// 租户列表
	mock.ExpectQuery("SELECT .* FROM workflow_model WHERE tenant_id = \\? ORDER BY sort").WithArgs("sys").WillReturnRows(mmRowsOf(mmModelCols, mmModelRowVals(stamp), mmModelRowVals(stamp)))
	rows, err := set.Model.ListByTenant(ctx, "sys")
	if err != nil || len(rows) != 2 {
		t.Fatalf("租户列表失败: len=%d err=%v", len(rows), err)
	}

	// 整行更新（仓储剔除 id 列；update_time 缺省由仓储填充）
	mock.ExpectExec("UPDATE workflow_model SET ").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := set.Model.Update(ctx, model); err != nil {
		t.Fatalf("更新失败: %v", err)
	}

	// 回写部署时间（prepare 追加 update_time）
	mock.ExpectExec("UPDATE workflow_model SET last_deploy_time = \\?, update_time = \\? WHERE id = \\?").WithArgs(stamp.Add(time.Hour), stamp, "m1").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := set.Model.UpdateLastDeployTime(ctx, "m1", stamp.Add(time.Hour)); err != nil {
		t.Fatalf("回写部署时间失败: %v", err)
	}

	// 批量删除与错误透传
	mock.ExpectExec("DELETE FROM workflow_model WHERE id = \\?").WithArgs("a").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM workflow_model WHERE id = \\?").WithArgs("b").WillReturnError(context.DeadlineExceeded)
	if err := set.Model.DeleteByIDs(ctx, []string{"a", "b"}); err == nil {
		t.Fatal("删除错误应透传")
	}

	// 插入错误透传
	mock.ExpectExec("INSERT INTO workflow_model ").WillReturnError(context.DeadlineExceeded)
	if err := set.Model.Insert(ctx, model); err == nil {
		t.Fatal("插入错误应透传")
	}
}

// TestModelMgmtSQLStepDesignStore 验证设计时步骤 SQL 适配器的语句、参数与行映射。
// @param t 测试上下文
func TestModelMgmtSQLStepDesignStore(t *testing.T) {
	ctx, mock, set := mmNewSQLModelMgmt(t)
	stamp := mmTestStamp
	start := stamp.Add(-time.Hour)
	step := &StepDesign{
		BaseColumns: BaseColumns{ID: "d1", Creator: "op", CreatorName: "张三", CreateTime: stamp, Updater: "op", UpdateTime: stamp, TenantID: "sys"},
		Name:        "部门审批", StepCode: "deptAudit", Description: "描述", Value: "v", AccountName: "acct", Source: 1,
		ModelID: "m1", ResourceID: "res", Sort: 2.5, Agree: "a", Reject: "r", Remark: "备注", Fix: "fix", URL: "/u",
		Visible: 1, TaskType: "user", EditInfo: "{}", OperateAuth: "{}",
		AllowReturn: "1", BackStepCodes: "s1,s2", BackStepNames: "甲,乙", AutoComplete: 1,
		AutoCompleteStartTime: &start, TimeLimit: 24.5, TimeLimitUnit: "h", RemindTimeLimit: 1.5, RemindTimeLimitUnit: "h",
		ChannelTypeName: "app", ChannelTypeNameCN: "应用", ReturnApprovalStep: "3", CountersignPercentReturn: "1", ReturnPercent: "50",
	}

	// 插入：40 列全量参数断言（规则列 tinyint/int/decimal 安全转换）
	expectedArgs := []driver.Value{
		"d1", "部门审批", "deptAudit", "描述", "v", "acct", 1, "m1", "res", 2.5, "a", "r", "备注", "fix", "/u", 1, "user", "{}", "{}",
		int64(1), "s1,s2", "甲,乙", 1, start, 24.5, "h", 1.5, "h", "app", "应用", 3, int64(1), 50.0,
		"op", "张三", stamp, "op", stamp, "sys",
	}
	mock.ExpectExec("INSERT INTO workflow_step ").WithArgs(expectedArgs...).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := set.Step.Insert(ctx, step); err != nil {
		t.Fatalf("插入失败: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("插入参数断言失败: %v", err)
	}

	// 按主键读回：tinyint/decimal 列转原始串
	vals := map[string]any{
		"id": "d1", "name": "部门审批", "step_code": "deptAudit", "account_name": "acct", "source": int64(1), "model_id": "m1",
		"sort": 2.5, "visible": int64(1), "task_type": "user", "allow_return": int64(1), "back_step_codes": "s1,s2",
		"auto_complete": int64(1), "auto_complete_start_time": start, "time_limit": 24.5, "time_limit_unit": "h",
		"return_approval_step": int64(3), "countersign_percent_return": int64(1), "return_percent": "50.00",
		"create_time": stamp, "update_time": stamp, "tenant_id": "sys",
	}
	mock.ExpectQuery("SELECT .* FROM workflow_step WHERE id = \\?").WithArgs("d1").WillReturnRows(mmRowsOf(mmStepCols, vals))
	got, found, err := set.Step.GetByPK(ctx, "d1")
	if err != nil || !found {
		t.Fatalf("按主键查询失败: found=%v err=%v", found, err)
	}
	if got.AllowReturn != "1" || got.CountersignPercentReturn != "1" || got.ReturnApprovalStep != "3" || got.ReturnPercent != "50.00" {
		t.Fatalf("规则列读回错误: %+v", got)
	}
	if got.Sort != 2.5 || got.TimeLimit != 24.5 || got.AutoCompleteEndTime != nil || !got.AutoCompleteStartTime.Equal(start) {
		t.Fatalf("浮点/时间列读回错误: %+v", got)
	}

	// 按模型+代号查询与模型列表
	mock.ExpectQuery("SELECT .* FROM workflow_step WHERE model_id = \\? AND step_code = \\? ORDER BY sort LIMIT 1").WithArgs("m1", "deptAudit").WillReturnRows(mmRowsOf(mmStepCols, vals))
	if _, found, err := set.Step.GetByModelAndCode(ctx, "m1", "deptAudit"); err != nil || !found {
		t.Fatalf("按模型+代号查询失败: found=%v err=%v", found, err)
	}
	mock.ExpectQuery("SELECT .* FROM workflow_step WHERE model_id = \\? ORDER BY sort").WithArgs("m1").WillReturnRows(mmRowsOf(mmStepCols, vals, vals))
	rows, err := set.Step.ListByModel(ctx, "m1")
	if err != nil || len(rows) != 2 {
		t.Fatalf("按模型列表失败: len=%d err=%v", len(rows), err)
	}

	// 更新与批量删除
	mock.ExpectExec("UPDATE workflow_step SET ").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := set.Step.Update(ctx, step); err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	mock.ExpectExec("DELETE FROM workflow_step WHERE id = \\?").WithArgs("d1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM workflow_step WHERE id = \\?").WithArgs("d2").WillReturnResult(sqlmock.NewResult(0, 0))
	if err := set.Step.DeleteByIDs(ctx, []string{"d1", "d2"}); err != nil {
		t.Fatalf("批量删除失败: %v", err)
	}

	// 插入错误透传
	mock.ExpectExec("INSERT INTO workflow_step ").WillReturnError(context.DeadlineExceeded)
	if err := set.Step.Insert(ctx, step); err == nil {
		t.Fatal("插入错误应透传")
	}
}

// TestModelMgmtSQLTemplateStore 验证模板 SQL 适配器的语句、参数与行映射。
// @param t 测试上下文
func TestModelMgmtSQLTemplateStore(t *testing.T) {
	ctx, mock, set := mmNewSQLModelMgmt(t)
	stamp := mmTestStamp
	tpl := &TemplateDef{
		BaseColumns: BaseColumns{ID: "t1", Creator: "op", CreatorName: "张三", CreateTime: stamp, Updater: "op", UpdateTime: stamp, TenantID: "sys"},
		Name:        "经典模板", Layout: "{}", FlowXML: "<x/>", IsVisible: 1, IsDefault: 1, Source: 2, CategoryID: "cat1",
	}

	// 插入：14 列参数断言
	mock.ExpectExec("INSERT INTO workflow_template ").WithArgs(
		"t1", "经典模板", "{}", "<x/>", 1, 1, 2, "cat1", "op", "张三", stamp, "op", stamp, "sys",
	).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := set.Template.Insert(ctx, tpl); err != nil {
		t.Fatalf("插入失败: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("插入参数断言失败: %v", err)
	}

	vals := map[string]any{"id": "t1", "name": "经典模板", "layout": "{}", "flow_xml": "<x/>", "is_visible": int64(1), "is_default": int64(1), "source": int64(2), "category_id": "cat1", "create_time": stamp, "update_time": stamp, "tenant_id": "sys"}

	// 按主键查询
	mock.ExpectQuery("SELECT .* FROM workflow_template WHERE id = \\?").WithArgs("t1").WillReturnRows(mmRowsOf(mmTemplateCols, vals))
	got, found, err := set.Template.GetByPK(ctx, "t1")
	if err != nil || !found || got.Name != "经典模板" || got.IsDefault != 1 || got.Source != 2 {
		t.Fatalf("按主键查询/映射失败: found=%v err=%v got=%+v", found, err, got)
	}

	// 分类列表（仓储按名称升序）
	mock.ExpectQuery("SELECT .* FROM workflow_template WHERE category_id = \\? ORDER BY name").WithArgs("cat1").WillReturnRows(mmRowsOf(mmTemplateCols, vals, vals))
	catRows, err := set.Template.ListByCategory(ctx, "cat1")
	if err != nil || len(catRows) != 2 {
		t.Fatalf("分类列表失败: len=%d err=%v", len(catRows), err)
	}

	// 租户列表（仓储缺口，裸 SQL 补齐）
	mock.ExpectQuery("SELECT \\* FROM workflow_template WHERE tenant_id = \\? ORDER BY name").WithArgs("sys").WillReturnRows(mmRowsOf(mmTemplateCols, vals))
	tenantRows, err := set.Template.ListByTenant(ctx, "sys")
	if err != nil || len(tenantRows) != 1 {
		t.Fatalf("租户列表失败: len=%d err=%v", len(tenantRows), err)
	}

	// 更新与批量删除
	mock.ExpectExec("UPDATE workflow_template SET ").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := set.Template.Update(ctx, tpl); err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	mock.ExpectExec("DELETE FROM workflow_template WHERE id = \\?").WithArgs("t1").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := set.Template.DeleteByIDs(ctx, []string{"t1"}); err != nil {
		t.Fatalf("批量删除失败: %v", err)
	}

	// 插入错误透传
	mock.ExpectExec("INSERT INTO workflow_template ").WillReturnError(context.DeadlineExceeded)
	if err := set.Template.Insert(ctx, tpl); err == nil {
		t.Fatal("插入错误应透传")
	}
}

// TestModelMgmtSQLHisStepStore 验证快照 SQL 适配器的批量替换幂等与行映射。
// @param t 测试上下文
func TestModelMgmtSQLHisStepStore(t *testing.T) {
	ctx, mock, set := mmNewSQLModelMgmt(t)

	// 按部署+节点定位：tinyint/decimal 列转原始串
	vals := map[string]any{
		"id": "h1", "step_name": "部门审批", "step_code": "deptAudit", "sort": int64(2), "url": "/u",
		"deployment_id": "dep-1", "allow_return": int64(0), "back_step_codes": "start",
		"return_approval_step": int64(3), "countersign_percent_return": int64(1), "return_percent": "50.00",
	}
	mock.ExpectQuery("SELECT .* FROM workflow_his_step WHERE deployment_id = \\? AND step_code = \\? LIMIT 1").WithArgs("dep-1", "deptAudit").WillReturnRows(mmRowsOf(mmHisStepCols, vals))
	rule, found, err := set.HisStep.GetByDeploymentAndActivity(ctx, "dep-1", "deptAudit")
	if err != nil || !found {
		t.Fatalf("按部署+节点查询失败: found=%v err=%v", found, err)
	}
	if rule.DeploymentID != "dep-1" || rule.StepCode != "deptAudit" || rule.StepName != "部门审批" ||
		rule.AllowReturn != "0" || rule.BackStepCodes != "start" || rule.ReturnApprovalStep != "3" ||
		rule.CountersignPercentReturn != "1" || rule.ReturnPercent != "50.00" || rule.Sort != 2 || rule.URL != "/u" {
		t.Fatalf("快照行映射错误: %+v", rule)
	}

	// 按部署列表
	mock.ExpectQuery("SELECT .* FROM workflow_his_step WHERE deployment_id = \\? ORDER BY sort").WithArgs("dep-1").WillReturnRows(mmRowsOf(mmHisStepCols, vals, vals))
	rows, err := set.HisStep.ListByDeployment(ctx, "dep-1")
	if err != nil || len(rows) != 2 {
		t.Fatalf("按部署列表失败: len=%d err=%v", len(rows), err)
	}

	// 批量替换：既有 1 条 → 先删后插（幂等替换语义）
	newRule := &StepRule{StepCode: "boss", StepName: "总监审批", Sort: 3, AllowReturn: "1", BackStepCodes: "deptAudit", ReturnPercent: "80", URL: "/boss"}
	mock.ExpectQuery("SELECT .* FROM workflow_his_step WHERE deployment_id = \\? ORDER BY sort").WithArgs("dep-9").WillReturnRows(mmRowsOf(mmHisStepCols, vals))
	mock.ExpectExec("DELETE FROM workflow_his_step WHERE id = \\?").WithArgs("h1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO workflow_his_step ").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := set.HisStep.Put(ctx, "dep-9", []*StepRule{newRule}); err != nil {
		t.Fatalf("批量替换失败: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("批量替换断言失败: %v", err)
	}

	// 重复替换：既有 0 条 → 仅插入（同部署重复快照不追加）
	mock.ExpectQuery("SELECT .* FROM workflow_his_step WHERE deployment_id = \\? ORDER BY sort").WithArgs("dep-9").WillReturnRows(sqlmock.NewRows(mmHisStepCols))
	mock.ExpectExec("INSERT INTO workflow_his_step ").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := set.HisStep.Put(ctx, "dep-9", []*StepRule{newRule}); err != nil {
		t.Fatalf("重复替换失败: %v", err)
	}

	// 空批量替换清空快照
	mock.ExpectQuery("SELECT .* FROM workflow_his_step WHERE deployment_id = \\? ORDER BY sort").WithArgs("dep-9").WillReturnRows(sqlmock.NewRows(mmHisStepCols))
	if err := set.HisStep.Put(ctx, "dep-9", nil); err != nil {
		t.Fatalf("空批量替换失败: %v", err)
	}

	// 删除部署快照与空参防御
	mock.ExpectQuery("SELECT .* FROM workflow_his_step WHERE deployment_id = \\? ORDER BY sort").WithArgs("dep-1").WillReturnRows(mmRowsOf(mmHisStepCols, vals))
	mock.ExpectExec("DELETE FROM workflow_his_step WHERE id = \\?").WithArgs("h1").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := set.HisStep.DeleteByDeployment(ctx, "dep-1"); err != nil {
		t.Fatalf("删除部署快照失败: %v", err)
	}
	if err := set.HisStep.Put(ctx, "", []*StepRule{newRule}); err == nil {
		t.Fatal("空部署写入应报错")
	}

	// 删除错误透传
	mock.ExpectQuery("SELECT .* FROM workflow_his_step WHERE deployment_id = \\? ORDER BY sort").WithArgs("dep-2").WillReturnRows(mmRowsOf(mmHisStepCols, vals))
	mock.ExpectExec("DELETE FROM workflow_his_step WHERE id = \\?").WithArgs("h1").WillReturnError(context.DeadlineExceeded)
	if err := set.HisStep.DeleteByDeployment(ctx, "dep-2"); err == nil {
		t.Fatal("删除错误应透传")
	}
}

// TestModelMgmtSQLNoDBRawQuery 验证未注入连接池时模板租户列表报错（其余能力完整）。
// @param t 测试上下文
func TestModelMgmtSQLNoDBRawQuery(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("创建 sqlmock 失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store, err := pwf.NewStore(db, pwf.DialectMySQL)
	if err != nil {
		t.Fatalf("创建 workflow 仓储入口失败: %v", err)
	}
	// 不带 db 的聚合：裸 SQL 缺口查询应明确报错
	set := NewSQLModelMgmtStores(store)
	if _, err := set.Template.ListByTenant(context.Background(), "sys"); err == nil {
		t.Fatal("未注入连接池的裸 SQL 查询应报错")
	}
}

// TestModelMgmtSQLErrors 验证 SQL 适配器错误透传与行转换边界分支。
// @param t 测试上下文
func TestModelMgmtSQLErrors(t *testing.T) {
	ctx, mock, set := mmNewSQLModelMgmt(t)
	stamp := mmTestStamp

	// 模型：代号查询错误 / 部署时间回写错误 / 列表错误
	mock.ExpectQuery("SELECT .* FROM workflow_model WHERE model_code = \\? AND tenant_id = \\? LIMIT 1").WillReturnError(context.DeadlineExceeded)
	if _, _, err := set.Model.GetByCode(ctx, "sys", "leave"); err == nil {
		t.Fatal("代号查询错误应透传")
	}
	mock.ExpectExec("UPDATE workflow_model SET last_deploy_time = \\?, update_time = \\? WHERE id = \\?").WillReturnError(context.DeadlineExceeded)
	if err := set.Model.UpdateLastDeployTime(ctx, "m1", stamp); err == nil {
		t.Fatal("部署时间回写错误应透传")
	}
	mock.ExpectQuery("SELECT .* FROM workflow_model WHERE tenant_id = \\? ORDER BY sort").WillReturnError(context.DeadlineExceeded)
	if _, err := set.Model.ListByTenant(ctx, "sys"); err == nil {
		t.Fatal("租户列表错误应透传")
	}

	// 步骤：主键查询错误 / 模型+代号查询错误 / 模型列表错误 / 批量删除错误
	mock.ExpectQuery("SELECT .* FROM workflow_step WHERE id = \\?").WillReturnError(context.DeadlineExceeded)
	if _, _, err := set.Step.GetByPK(ctx, "d1"); err == nil {
		t.Fatal("步骤主键查询错误应透传")
	}
	mock.ExpectQuery("SELECT .* FROM workflow_step WHERE model_id = \\? AND step_code = \\? ORDER BY sort LIMIT 1").WillReturnError(context.DeadlineExceeded)
	if _, _, err := set.Step.GetByModelAndCode(ctx, "m1", "s"); err == nil {
		t.Fatal("步骤模型+代号查询错误应透传")
	}
	mock.ExpectQuery("SELECT .* FROM workflow_step WHERE model_id = \\? ORDER BY sort").WillReturnError(context.DeadlineExceeded)
	if _, err := set.Step.ListByModel(ctx, "m1"); err == nil {
		t.Fatal("步骤模型列表错误应透传")
	}
	mock.ExpectExec("DELETE FROM workflow_step WHERE id = \\?").WillReturnError(context.DeadlineExceeded)
	if err := set.Step.DeleteByIDs(ctx, []string{"d1"}); err == nil {
		t.Fatal("步骤批量删除错误应透传")
	}

	// 模板：主键查询错误 / 分类列表错误 / 更新错误 / 批量删除错误
	mock.ExpectQuery("SELECT .* FROM workflow_template WHERE id = \\?").WillReturnError(context.DeadlineExceeded)
	if _, _, err := set.Template.GetByPK(ctx, "t1"); err == nil {
		t.Fatal("模板主键查询错误应透传")
	}
	mock.ExpectQuery("SELECT .* FROM workflow_template WHERE category_id = \\? ORDER BY name").WillReturnError(context.DeadlineExceeded)
	if _, err := set.Template.ListByCategory(ctx, "cat1"); err == nil {
		t.Fatal("模板分类列表错误应透传")
	}
	mock.ExpectExec("UPDATE workflow_template SET ").WillReturnError(context.DeadlineExceeded)
	if err := set.Template.Update(ctx, &TemplateDef{BaseColumns: BaseColumns{ID: "t1"}}); err == nil {
		t.Fatal("模板更新错误应透传")
	}
	mock.ExpectExec("DELETE FROM workflow_template WHERE id = \\?").WillReturnError(context.DeadlineExceeded)
	if err := set.Template.DeleteByIDs(ctx, []string{"t1"}); err == nil {
		t.Fatal("模板批量删除错误应透传")
	}

	// 快照：定位错误 / 列表错误 / Put 各错误分支
	mock.ExpectQuery("SELECT .* FROM workflow_his_step WHERE deployment_id = \\? AND step_code = \\? LIMIT 1").WillReturnError(context.DeadlineExceeded)
	if _, _, err := set.HisStep.GetByDeploymentAndActivity(ctx, "dep-1", "a"); err == nil {
		t.Fatal("快照定位错误应透传")
	}
	mock.ExpectQuery("SELECT .* FROM workflow_his_step WHERE deployment_id = \\? ORDER BY sort").WillReturnError(context.DeadlineExceeded)
	if _, err := set.HisStep.ListByDeployment(ctx, "dep-1"); err == nil {
		t.Fatal("快照列表错误应透传")
	}
	mock.ExpectQuery("SELECT .* FROM workflow_his_step WHERE deployment_id = \\? ORDER BY sort").WillReturnError(context.DeadlineExceeded)
	if err := set.HisStep.Put(ctx, "dep-1", nil); err == nil {
		t.Fatal("快照替换预查询错误应透传")
	}
	mock.ExpectQuery("SELECT .* FROM workflow_his_step WHERE deployment_id = \\? ORDER BY sort").WithArgs("dep-1").WillReturnRows(mmRowsOf(mmHisStepCols, map[string]any{"id": "h1"}))
	mock.ExpectExec("DELETE FROM workflow_his_step WHERE id = \\?").WillReturnError(context.DeadlineExceeded)
	if err := set.HisStep.Put(ctx, "dep-1", nil); err == nil {
		t.Fatal("快照替换删除错误应透传")
	}
	mock.ExpectQuery("SELECT .* FROM workflow_his_step WHERE deployment_id = \\? ORDER BY sort").WithArgs("dep-1").WillReturnRows(sqlmock.NewRows(mmHisStepCols))
	mock.ExpectExec("INSERT INTO workflow_his_step ").WillReturnError(context.DeadlineExceeded)
	if err := set.HisStep.Put(ctx, "dep-1", []*StepRule{{StepCode: "a", StepName: "A"}}); err == nil {
		t.Fatal("快照替换插入错误应透传")
	}
	mock.ExpectQuery("SELECT .* FROM workflow_his_step WHERE deployment_id = \\? ORDER BY sort").WillReturnError(context.DeadlineExceeded)
	if err := set.HisStep.DeleteByDeployment(ctx, "dep-1"); err == nil {
		t.Fatal("快照删除预查询错误应透传")
	}

	// 独立构造的快照适配器（非聚合入口）
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("创建 sqlmock 失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store, err := pwf.NewStore(db, pwf.DialectMySQL)
	if err != nil {
		t.Fatalf("创建 workflow 仓储入口失败: %v", err)
	}
	if NewSQLHisStepStore(store) == nil {
		t.Fatal("独立快照适配器构造失败")
	}
}

// TestModelMgmtSQLRowEdge 验证行转换的可空分支边界。
// @param t 测试上下文
func TestModelMgmtSQLRowEdge(t *testing.T) {
	stamp := mmTestStamp
	// 快照规则空可选字段：行不携带规则列（落 NULL）
	row := mmHisRowFromRule(&StepRule{StepCode: "a", StepName: "A"}, "dep-1", "h1")
	if _, ok := row["allow_return"]; ok {
		t.Fatal("空 allow_return 应省略")
	}
	if _, ok := row["countersign_percent_return"]; ok {
		t.Fatal("空 countersign_percent_return 应省略")
	}
	if _, ok := row["return_approval_step"]; ok {
		t.Fatal("空 return_approval_step 应省略")
	}
	if _, ok := row["return_percent"]; ok {
		t.Fatal("空 return_percent 应省略")
	}
	// 快照规则全字段：规则列全落库
	full := mmHisRowFromRule(&StepRule{StepCode: "a", AllowReturn: "1", CountersignPercentReturn: "0", ReturnApprovalStep: "2", ReturnPercent: "33.3"}, "dep-1", "h1")
	if full["allow_return"] != int64(1) || full["countersign_percent_return"] != int64(0) || full["return_approval_step"] != 2 || full["return_percent"] != 33.3 {
		t.Fatalf("快照规则列转换错误: %+v", full)
	}
	// 步骤可空时间列：截止时间有值时落库
	step := &StepDesign{BaseColumns: BaseColumns{ID: "d1"}, AutoCompleteEndTime: &stamp}
	if mmStepToRow(step)["auto_complete_end_time"] != stamp {
		t.Fatal("截止时间应落库")
	}
	// 模型可空时间列：部署时间有值时落库
	model := mmModelSample("m1", "leave", "sys")
	model.LastDeployTime = &stamp
	if mmModelToRow(model)["last_deploy_time"] != stamp {
		t.Fatal("部署时间应落库")
	}
}
