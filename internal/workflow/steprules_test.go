package workflow

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/DengQingNian/Flowtide/api"
	pwf "github.com/DengQingNian/Flowtide/internal/persistence/workflow"
)

// newStepFixture 构造步骤规则测试环境（复用办结语料：apply → handle → end 线性流程）。
// @param t 测试上下文
// @return 办结测试环境（含引擎与全套域服务）
func newStepFixture(t *testing.T) *completeFixture {
	return newCompleteFixture(t)
}

// TestMemStepStore 验证内存步骤存储的写读与部署过滤。
// @param t 测试上下文
func TestMemStepStore(t *testing.T) {
	store := NewMemStepStore()
	ctx := context.Background()
	// 空存储查询：不存在且不报错
	if _, found, err := store.GetByDeploymentAndActivity(ctx, "dep-1", "apply"); err != nil || found {
		t.Fatalf("空存储应查无记录: %v %v", found, err)
	}
	if rows, err := store.ListByDeployment(ctx, "dep-1"); err != nil || len(rows) != 0 {
		t.Fatalf("空存储列表应为空: %v %v", rows, err)
	}
	// 播种两个部署的规则
	store.Put(&StepRule{DeploymentID: "dep-1", StepCode: "apply", AllowReturn: "0", Sort: 1, StepName: "申请"})
	store.Put(&StepRule{DeploymentID: "dep-1", StepCode: "handle", AllowReturn: "1", BackStepCodes: "apply", Sort: 2})
	store.Put(&StepRule{DeploymentID: "dep-2", StepCode: "apply", AllowReturn: "1", Sort: 1})
	// 按部署+节点定位
	rule, found, err := store.GetByDeploymentAndActivity(ctx, "dep-1", "apply")
	if err != nil || !found || rule.AllowReturn != "0" || rule.StepName != "申请" {
		t.Fatalf("定位失败: %v %v %+v", found, err, rule)
	}
	// 部署隔离：dep-2 查不到 handle
	if _, found, _ := store.GetByDeploymentAndActivity(ctx, "dep-2", "handle"); found {
		t.Fatalf("部署隔离失败")
	}
	// 列表按 sort 升序
	rows, err := store.ListByDeployment(ctx, "dep-1")
	if err != nil || len(rows) != 2 || rows[0].StepCode != "apply" || rows[1].StepCode != "handle" {
		t.Fatalf("列表过滤/排序失败: %v %v", rows, err)
	}
}

// TestStepRuleServiceRules 验证步骤规则服务的各规则读取语义（"0" 显式禁止、白名单拆分、排序号）。
// @param t 测试上下文
func TestStepRuleServiceRules(t *testing.T) {
	store := NewMemStepStore()
	store.Put(&StepRule{DeploymentID: "dep-1", StepCode: "apply", AllowReturn: "0", Sort: 3, StepName: "申请", URL: "http://a"})
	store.Put(&StepRule{DeploymentID: "dep-1", StepCode: "handle", AllowReturn: "1", BackStepCodes: " apply ,sign, ", CountersignPercentReturn: "1", ReturnPercent: "50", Sort: 2})
	store.Put(&StepRule{DeploymentID: "dep-1", StepCode: "end", AllowReturn: "", CountersignPercentReturn: "0", ReturnPercent: "abc"})
	svc := NewStepRuleService(store, nil)
	ctx := context.Background()
	// allowReturn："0" 禁止；"1"/缺失允许
	if allowed, err := svc.AllowReturn(ctx, "dep-1", "apply"); err != nil || allowed {
		t.Fatalf("allowReturn=0 应禁止: %v %v", allowed, err)
	}
	if allowed, _ := svc.AllowReturn(ctx, "dep-1", "handle"); !allowed {
		t.Fatalf("allowReturn=1 应允许")
	}
	// 规则行缺失：按允许处理
	if allowed, err := svc.AllowReturn(ctx, "dep-1", "missing"); err != nil || !allowed {
		t.Fatalf("规则缺失应允许: %v %v", allowed, err)
	}
	// 白名单拆分：去空白、剔空段
	codes, err := svc.BackStepCodes(ctx, "dep-1", "handle")
	if err != nil || len(codes) != 2 || codes[0] != "apply" || codes[1] != "sign" {
		t.Fatalf("白名单拆分失败: %v %v", codes, err)
	}
	// 未配置白名单 → 空列表
	if codes, _ := svc.BackStepCodes(ctx, "dep-1", "apply"); len(codes) != 0 {
		t.Fatalf("未配置白名单应为空: %v", codes)
	}
	// 排序号与步骤名
	if sortNo, err := svc.SortOf(ctx, "dep-1", "apply"); err != nil || sortNo != 3 {
		t.Fatalf("排序号读取失败: %d %v", sortNo, err)
	}
	if name, err := svc.StepNameOf(ctx, "dep-1", "apply"); err != nil || name != "申请" {
		t.Fatalf("步骤名读取失败: %s %v", name, err)
	}
	if _, err := svc.SortOf(ctx, "dep-1", "missing"); err != nil {
		t.Fatalf("缺失规则排序号应给零值: %v", err)
	}
	// 比例退回：启用+合法比例 / 未启用 / 非法比例 / 缺失
	percent, enabled, err := svc.ReturnPercentOf(ctx, "dep-1", "handle")
	if err != nil || !enabled || percent == nil || *percent != 50 {
		t.Fatalf("比例退回读取失败: %v %v %v", percent, enabled, err)
	}
	if _, enabled, _ := svc.ReturnPercentOf(ctx, "dep-1", "end"); enabled {
		t.Fatalf("countersign_percent_return=0 应未启用")
	}
	if _, enabled, _ := svc.ReturnPercentOf(ctx, "dep-1", "apply"); enabled {
		t.Fatalf("未配置比例退回应未启用")
	}
	// 缺失规则：不启用
	if _, enabled, err := svc.ReturnPercentOf(ctx, "dep-1", "missing"); err != nil || enabled {
		t.Fatalf("缺失规则应未启用: %v %v", enabled, err)
	}
}

// TestStepRuleServiceRawHelpers 验证布尔/比例原始值的解释边界。
// @param t 测试上下文
func TestStepRuleServiceRawHelpers(t *testing.T) {
	// rawEnabled："1"/"true" 启用，其余（含 " 1 " 空白、"0"、空串）按字面判断
	if !rawEnabled("1") || !rawEnabled("true") || !rawEnabled("TRUE") {
		t.Fatalf("rawEnabled 应识别 1/true")
	}
	if rawEnabled("0") || rawEnabled("") || rawEnabled("yes") {
		t.Fatalf("rawEnabled 应拒绝非 1/true")
	}
	// parsePercent：合法数值/空白/非法/空串
	if v := parsePercent(" 12.5 "); v == nil || *v != 12.5 {
		t.Fatalf("parsePercent 合法值解析失败: %v", v)
	}
	if parsePercent("abc") != nil || parsePercent("") != nil {
		t.Fatalf("parsePercent 非法/空值应返回 nil")
	}
	// 空存储服务：全部规则读取按缺省形态返回（不报错）
	empty := NewStepRuleService(NewMemStepStore(), nil)
	if allowed, _ := empty.AllowReturn(context.Background(), "d", "a"); !allowed {
		t.Fatalf("空存储应允许退回")
	}
	if codes, _ := empty.BackStepCodes(context.Background(), "d", "a"); len(codes) != 0 {
		t.Fatalf("空存储白名单应为空")
	}
	if name, _ := empty.StepNameOf(context.Background(), "d", "a"); name != "" {
		t.Fatalf("空存储步骤名应为空")
	}
}

// TestStepRuleServicePercentReturnRule 验证 StepRuleProvider（按任务定位比例退回规则）：
// 规则启用、未启用、任务/定义缺失与 nil 引擎分支。
// @param t 测试上下文
func TestStepRuleServicePercentReturnRule(t *testing.T) {
	fx := newStepFixture(t)
	ctx := context.Background()
	// 播种规则：apply 节点启用比例退回（比例 60），handle 未启用
	definitions, err := fx.engine.RepositoryService().ListProcessDefinitions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	deploymentID := ""
	for _, definition := range definitions {
		if definition.Key == "flowComplete" {
			deploymentID = definition.DeploymentID
		}
	}
	if deploymentID == "" {
		t.Fatalf("未找到 flowComplete 定义")
	}
	store := NewMemStepStore()
	store.Put(&StepRule{DeploymentID: deploymentID, StepCode: "apply", CountersignPercentReturn: "1", ReturnPercent: "60"})
	store.Put(&StepRule{DeploymentID: deploymentID, StepCode: "handle", CountersignPercentReturn: "0"})
	svc := NewStepRuleService(store, fx.engine)
	// 发起流程取首任务（apply 节点）
	instance, task := fx.start(t, "flowComplete", "biz-step-rule", "alice")
	_ = instance
	// 启用：返回规则与比例
	rule, enabled, err := svc.PercentReturnRule(ctx, task.ID)
	if err != nil || !enabled || rule == nil || rule.ReturnPercent == nil || *rule.ReturnPercent != 60 {
		t.Fatalf("比例退回规则读取失败: %v %v %+v", enabled, err, rule)
	}
	if !rule.CountersignPercentReturn {
		t.Fatalf("规则启用标记应为真")
	}
	// 未启用节点：handle 任务（先办结首任务）
	if err := fx.engine.TaskService().Complete(ctx, task.ID, api.CompleteTaskRequest{UserID: "alice"}); err != nil {
		t.Fatal(err)
	}
	handleTask := fx.firstTaskOf(t, instance.ID)
	if _, enabled, err := svc.PercentReturnRule(ctx, handleTask.ID); err != nil || enabled {
		t.Fatalf("未启用节点应返回 false: %v %v", enabled, err)
	}
	// 任务不存在：报任务缺失错误
	if _, _, err := svc.PercentReturnRule(ctx, "no-such-task"); err == nil || !containsText(err, "任务不存在") {
		t.Fatalf("任务缺失应报错: %v", err)
	}
	// nil 引擎/nil 存储：按未启用处理
	if _, enabled, err := NewStepRuleService(store, nil).PercentReturnRule(ctx, task.ID); err != nil || enabled {
		t.Fatalf("nil 引擎应按未启用处理: %v %v", enabled, err)
	}
	if _, enabled, err := NewStepRuleService(nil, fx.engine).PercentReturnRule(ctx, task.ID); err != nil || enabled {
		t.Fatalf("nil 存储应按未启用处理: %v %v", enabled, err)
	}
}

// newStepSQLAdapter 创建步骤规则 SQL 适配器与其 sqlmock。
// @param t 测试上下文
// @return 适配器
// @return 数据库模拟
func newStepSQLAdapter(t *testing.T) (*SQLStepStore, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("创建 sqlmock 失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store, err := pwf.NewStore(db, pwf.DialectMySQL)
	if err != nil {
		t.Fatalf("创建 workflow 仓储入口失败: %v", err)
	}
	return NewSQLStepStore(store), mock
}

// TestSQLStepStore 验证步骤规则 SQL 适配器的查询与行映射。
// @param t 测试上下文
func TestSQLStepStore(t *testing.T) {
	adapter, mock := newStepSQLAdapter(t)
	ctx := context.Background()
	// 单条定位（行含 NULL 与数值字符串形态）
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnRows(sqlmock.NewRows(stepSQLColumns()).
			AddRow("dep-1", "apply", "申请", "0", "", "", "1", "60", 3, "http://a"))
	rule, found, err := adapter.GetByDeploymentAndActivity(ctx, "dep-1", "apply")
	if err != nil || !found {
		t.Fatalf("单条查询失败: %v %v", found, err)
	}
	if rule.StepCode != "apply" || rule.AllowReturn != "0" || rule.Sort != 3 || rule.ReturnPercent != "60" {
		t.Fatalf("行映射错误: %+v", rule)
	}
	// 单条不存在
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnRows(sqlmock.NewRows(stepSQLColumns()))
	if _, found, err := adapter.GetByDeploymentAndActivity(ctx, "dep-1", "missing"); err != nil || found {
		t.Fatalf("缺失行应返回未找到: %v %v", found, err)
	}
	// 列表查询（含 NULL 排序号形态）
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnRows(sqlmock.NewRows(stepSQLColumns()).
			AddRow("dep-1", "apply", "申请", "1", "x", "", "0", nil, 1, nil).
			AddRow("dep-1", "handle", "办理", "1", "", "1", "2", "50", []byte("2"), nil))
	rows, err := adapter.ListByDeployment(ctx, "dep-1")
	if err != nil || len(rows) != 2 {
		t.Fatalf("列表查询失败: %v %v", rows, err)
	}
	if rows[0].ReturnPercent != "" || rows[1].Sort != 2 {
		t.Fatalf("列表行映射错误: %+v %+v", rows[0], rows[1])
	}
	// 查询错误透传
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).WillReturnError(assertError("数据库不可用"))
	if _, err := adapter.ListByDeployment(ctx, "dep-1"); err == nil {
		t.Fatalf("查询错误应上抛")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL 预期未满足: %v", err)
	}
}

// stepSQLColumns 返回 SQL 适配器行映射所需的列集（与仓储 SELECT * 的列序对齐，
// sqlmock 按列名取值，此处仅列测试涉及的列）。
// @return 列名列表
func stepSQLColumns() []string {
	return []string{"deployment_id", "step_code", "step_name", "allow_return", "back_step_codes",
		"countersign_percent_return", "return_approval_step", "return_percent", "sort", "url"}
}
