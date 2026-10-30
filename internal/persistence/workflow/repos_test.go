package workflow

import (
	"context"
	"database/sql/driver"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// expectQuery 设置一个查询期望并返回指定结果行（不校验参数）。
// @param mock 数据库模拟
// @param pattern SQL 匹配模式
// @param columns 结果列
// @param rows 结果行
func expectQuery(mock sqlmock.Sqlmock, pattern string, columns []string, rows [][]driver.Value) {
	mock.ExpectQuery(pattern).WillReturnRows(newRows(columns, rows))
}

// expectQueryArgs 设置一个带参数校验的查询期望。
// @param mock 数据库模拟
// @param pattern SQL 匹配模式
// @param args 期望参数
// @param columns 结果列
// @param rows 结果行
func expectQueryArgs(mock sqlmock.Sqlmock, pattern string, args []driver.Value, columns []string, rows [][]driver.Value) {
	mock.ExpectQuery(pattern).WithArgs(args...).WillReturnRows(newRows(columns, rows))
}

// TestNextActivityRepository 验证下一步预生成-消耗核心仓储的查询、标记与双写。
// @param t 测试上下文
func TestNextActivityRepository(t *testing.T) {
	store, mock, _ := newTestStore(t, DialectMySQL)
	repo := NewNextActivityRepository(store)
	ctx := context.Background()
	columns := []string{"id", "identity_id"}
	// 未消耗记录：按实例+活动、按实例
	expectQueryArgs(mock, "FROM workflow_next_activity WHERE process_instance_id = \\? AND next_activity_id = \\? AND used = 0 ORDER BY version",
		[]driver.Value{"pi1", "act1"}, columns, [][]driver.Value{{"n1", "u1"}})
	expectQueryArgs(mock, "FROM workflow_next_activity WHERE process_instance_id = \\? AND used = 0 ORDER BY version",
		[]driver.Value{"pi1"}, columns, nil)
	if rows, err := repo.ListUnusedByActivity(ctx, "pi1", "act1"); err != nil || len(rows) != 1 {
		t.Fatalf("按活动查询失败: %v", err)
	}
	if _, err := repo.ListUnusedByProcessInstance(ctx, "pi1"); err != nil {
		t.Fatal(err)
	}
	// 标记消耗并回写任务 ID
	mock.ExpectExec(regexp.QuoteMeta("UPDATE workflow_next_activity SET used = ?, task_id = ?, update_time = ? WHERE id = ? AND used = 0")).
		WithArgs(1, "t1", fixedTime, "n1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	if _, err := repo.MarkUsed(ctx, "n1", "t1"); err != nil {
		t.Fatal(err)
	}
	// 按上游任务与当前任务查询
	expectQueryArgs(mock, "FROM workflow_next_activity WHERE pre_task_id = \\? ORDER BY version", []driver.Value{"t0"}, columns, nil)
	expectQueryArgs(mock, "FROM workflow_next_activity WHERE task_id = \\?", []driver.Value{"t1"}, columns, nil)
	if _, err := repo.ListByPreTask(ctx, "t0"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ListByTask(ctx, "t1"); err != nil {
		t.Fatal(err)
	}
	// 清理：按实例、按实例+活动集合、空集合直通
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM workflow_next_activity WHERE process_instance_id = ?")).
		WithArgs("pi1").WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM workflow_next_activity WHERE process_instance_id = ? AND next_activity_id IN (?, ?)")).
		WithArgs("pi1", "a1", "a2").WillReturnResult(sqlmock.NewResult(0, 2))
	if _, err := repo.DeleteByProcessInstance(ctx, "pi1"); err != nil {
		t.Fatal(err)
	}
	if affected, err := repo.DeleteByProcessInstanceAndActivities(ctx, "pi1", []string{"a1", "a2"}); err != nil || affected != 2 {
		t.Fatalf("按活动删除失败: %v", err)
	}
	if affected, err := repo.DeleteByProcessInstanceAndActivities(ctx, "pi1", nil); err != nil || affected != 0 {
		t.Fatalf("空集合应直接返回: %v", err)
	}
	// 双写：同一行同时写入运行态与历史表
	dual := regexp.QuoteMeta("INSERT INTO workflow_next_activity (id, process_instance_id, create_time, update_time) VALUES (?, ?, ?, ?)")
	dualHis := regexp.QuoteMeta("INSERT INTO workflow_his_next_activity (id, process_instance_id, create_time, update_time) VALUES (?, ?, ?, ?)")
	mock.ExpectExec(dual).WithArgs("n2", "pi1", fixedTime, fixedTime).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(dualHis).WithArgs("n2", "pi1", fixedTime, fixedTime).WillReturnResult(sqlmock.NewResult(1, 1))
	if err := repo.InsertDual(ctx, Row{"id": "n2", "process_instance_id": "pi1"}); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestHisNextActivityRepository 验证下一步历史仓储。
// @param t 测试上下文
func TestHisNextActivityRepository(t *testing.T) {
	store, mock, _ := newTestStore(t, DialectMySQL)
	repo := NewHisNextActivityRepository(store)
	ctx := context.Background()
	columns := []string{"id", "version"}
	expectQueryArgs(mock, "FROM workflow_his_next_activity WHERE process_instance_id = \\? ORDER BY version",
		[]driver.Value{"pi1"}, columns, nil)
	expectQueryArgs(mock, "FROM workflow_his_next_activity WHERE process_instance_id = \\? AND next_activity_id = \\? ORDER BY version DESC",
		[]driver.Value{"pi1", "act1"}, columns, nil)
	if _, err := repo.ListByProcessInstance(ctx, "pi1"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ListByActivityByVersionDesc(ctx, "pi1", "act1"); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec(regexp.QuoteMeta("UPDATE workflow_his_next_activity SET used = ?, task_id = ?, update_time = ? WHERE id = ? AND used = 0")).
		WithArgs(1, "t1", fixedTime, "h1").WillReturnResult(sqlmock.NewResult(0, 1))
	if _, err := repo.MarkUsed(ctx, "h1", "t1"); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM workflow_his_next_activity WHERE process_instance_id = ? AND next_activity_id IN (?, ?)")).
		WithArgs("pi1", "a1", "a2").WillReturnResult(sqlmock.NewResult(0, 1))
	if _, err := repo.DeleteByProcessInstanceAndActivities(ctx, "pi1", []string{"a1", "a2"}); err != nil {
		t.Fatal(err)
	}
	if affected, err := repo.DeleteByProcessInstanceAndActivities(ctx, "pi1", nil); err != nil || affected != 0 {
		t.Fatalf("空集合应直接返回: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestMessageExtendRepository 验证待办扩展仓储的查询、删除与已读标记。
// @param t 测试上下文
func TestMessageExtendRepository(t *testing.T) {
	store, mock, _ := newTestStore(t, DialectMySQL)
	repo := NewMessageExtendRepository(store)
	ctx := context.Background()
	columns := []string{"id", "title"}
	expectQueryArgs(mock, "FROM workflow_task_message_extend WHERE task_id = \\? LIMIT 1", []driver.Value{"t1"}, columns, [][]driver.Value{{"m1", "请假"}})
	expectQueryArgs(mock, "FROM workflow_task_message_extend WHERE process_instance_id = \\? ORDER BY create_time", []driver.Value{"pi1"}, columns, nil)
	expectQueryArgs(mock, "FROM workflow_task_message_extend WHERE business_id = \\? AND tenant_id = \\? ORDER BY create_time", []driver.Value{"b1", "tenant"}, columns, nil)
	expectQueryArgs(mock, "FROM workflow_task_message_extend WHERE owner_id = \\? ORDER BY create_time", []driver.Value{"u1"}, columns, nil)
	row, found, err := repo.GetByTask(ctx, "t1")
	if err != nil || !found || row["title"] != "请假" {
		t.Fatalf("按任务查询失败: %v %v", err, found)
	}
	if _, err := repo.ListByProcessInstance(ctx, "pi1"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ListByBusiness(ctx, "b1", "tenant"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ListByOwner(ctx, "u1"); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM workflow_task_message_extend WHERE task_id = ?")).
		WithArgs("t1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM workflow_task_message_extend WHERE process_instance_id = ? AND task_step_code IN (?, ?)")).
		WithArgs("pi1", "s1", "s2").WillReturnResult(sqlmock.NewResult(0, 1))
	if _, err := repo.DeleteByTask(ctx, "t1"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.DeleteByProcessInstanceAndSteps(ctx, "pi1", []string{"s1", "s2"}); err != nil {
		t.Fatal(err)
	}
	if affected, err := repo.DeleteByProcessInstanceAndSteps(ctx, "pi1", nil); err != nil || affected != 0 {
		t.Fatalf("空集合应直接返回: %v", err)
	}
	mock.ExpectExec(regexp.QuoteMeta("UPDATE workflow_task_message_extend SET is_read = ?, read_time = ?, update_time = ? WHERE task_id = ?")).
		WithArgs(1, fixedTime, fixedTime, "t1").WillReturnResult(sqlmock.NewResult(0, 1))
	if _, err := repo.MarkRead(ctx, "t1", fixedTime); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestHistoryAndMirrorRepositories 验证已办扩展与委托镜像仓储。
// @param t 测试上下文
func TestHistoryAndMirrorRepositories(t *testing.T) {
	store, mock, _ := newTestStore(t, DialectMySQL)
	ctx := context.Background()
	columns := []string{"id"}
	history := NewHistoryExtendRepository(store)
	expectQueryArgs(mock, "FROM workflow_task_history_extend WHERE task_id = \\? LIMIT 1", []driver.Value{"t1"}, columns, nil)
	expectQueryArgs(mock, "FROM workflow_task_history_extend WHERE process_instance_id = \\? ORDER BY task_finish_time", []driver.Value{"pi1"}, columns, nil)
	expectQueryArgs(mock, "FROM workflow_task_history_extend WHERE business_id = \\? AND tenant_id = \\? ORDER BY task_finish_time", []driver.Value{"b1", "tenant"}, columns, nil)
	if _, found, err := history.GetByTask(ctx, "t1"); err != nil || found {
		t.Fatalf("已办查询失败: %v", err)
	}
	if _, err := history.ListByProcessInstance(ctx, "pi1"); err != nil {
		t.Fatal(err)
	}
	if _, err := history.ListByBusiness(ctx, "b1", "tenant"); err != nil {
		t.Fatal(err)
	}
	mirror := NewDelegateTaskRepository(store)
	expectQueryArgs(mock, "FROM workflow_delegate_task WHERE task_id = \\? LIMIT 1", []driver.Value{"t1"}, columns, nil)
	expectQueryArgs(mock, "FROM workflow_delegate_task WHERE entrust_id = \\? ORDER BY create_time", []driver.Value{"u1"}, columns, nil)
	expectQueryArgs(mock, "FROM workflow_delegate_task WHERE process_instance_id = \\? ORDER BY create_time", []driver.Value{"pi1"}, columns, nil)
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM workflow_delegate_task WHERE process_instance_id = ?")).
		WithArgs("pi1").WillReturnResult(sqlmock.NewResult(0, 1))
	if _, found, err := mirror.GetByTask(ctx, "t1"); err != nil || found {
		t.Fatalf("镜像查询失败: %v", err)
	}
	if _, err := mirror.ListByEntrust(ctx, "u1"); err != nil {
		t.Fatal(err)
	}
	if _, err := mirror.ListByProcessInstance(ctx, "pi1"); err != nil {
		t.Fatal(err)
	}
	if _, err := mirror.DeleteByProcessInstance(ctx, "pi1"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestDocumentRepository 验证单据状态仓储的 upsert 双路径与逻辑删除。
// @param t 测试上下文
func TestDocumentRepository(t *testing.T) {
	store, mock, _ := newTestStore(t, DialectMySQL)
	repo := NewDocumentRepository(store)
	ctx := context.Background()
	columns := []string{"id"}
	// upsert 插入路径：按 business_id+tenant 未命中后插入
	expectQueryArgs(mock, "FROM workflow_document WHERE business_id = \\? AND tenant_id = \\? AND deleted = 0 LIMIT 1",
		[]driver.Value{"b1", "tenant"}, columns, nil)
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO workflow_document (id, business_id, create_time, update_time, tenant_id) VALUES (?, ?, ?, ?, ?)")).
		WithArgs("doc-1", "b1", fixedTime, fixedTime, "tenant").WillReturnResult(sqlmock.NewResult(1, 1))
	if err := repo.UpsertByBusinessID(ctx, Row{"id": "doc-1", "business_id": "b1", "tenant_id": "tenant"}); err != nil {
		t.Fatal(err)
	}
	// upsert 更新路径：命中后按主键更新，排除 id 与 create_time
	expectQueryArgs(mock, "FROM workflow_document WHERE business_id = \\? AND tenant_id = \\? AND deleted = 0 LIMIT 1",
		[]driver.Value{"b1", "tenant"}, columns, [][]driver.Value{{"doc-1"}})
	mock.ExpectExec(regexp.QuoteMeta("UPDATE workflow_document SET business_id = ?, flow_status = ?, update_time = ?, tenant_id = ? WHERE id = ?")).
		WithArgs("b1", "已办结", fixedTime, "tenant", "doc-1").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := repo.UpsertByBusinessID(ctx, Row{"id": "doc-1", "business_id": "b1", "tenant_id": "tenant", "flow_status": "已办结"}); err != nil {
		t.Fatal(err)
	}
	// upsert 插入路径缺少 id 时报错
	expectQueryArgs(mock, "FROM workflow_document WHERE business_id = \\? AND tenant_id = \\? AND deleted = 0 LIMIT 1",
		[]driver.Value{"", "tenant"}, columns, nil)
	if err := repo.UpsertByBusinessID(ctx, Row{"business_id": "", "tenant_id": "tenant"}); err == nil {
		t.Fatal("upsert 缺少 id 应报错")
	}
	// 查询与逻辑删除
	expectQueryArgs(mock, "FROM workflow_document WHERE flow_instance_id = \\? AND deleted = 0 ORDER BY create_time", []driver.Value{"pi1"}, columns, nil)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE workflow_document SET deleted = ?, update_time = ? WHERE business_id = ? AND tenant_id = ? AND deleted = 0")).
		WithArgs(1, fixedTime, "b1", "tenant").WillReturnResult(sqlmock.NewResult(0, 1))
	if _, err := repo.ListByFlowInstance(ctx, "pi1"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.MarkDeletedByBusinessID(ctx, "b1", "tenant"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestTraceRepositories 验证操作记录、审阅任务、异常与直送记录仓储。
// @param t 测试上下文
func TestTraceRepositories(t *testing.T) {
	store, mock, _ := newTestStore(t, DialectMySQL)
	ctx := context.Background()
	columns := []string{"id"}
	records := NewOperationRecordRepository(store)
	expectQueryArgs(mock, "FROM workflow_operation_record WHERE task_id = \\? ORDER BY create_time", []driver.Value{"t1"}, columns, nil)
	expectQueryArgs(mock, "FROM workflow_operation_record WHERE business_id = \\? ORDER BY create_time", []driver.Value{"b1"}, columns, nil)
	expectQueryArgs(mock, "FROM workflow_operation_record WHERE process_instance_id = \\? ORDER BY create_time", []driver.Value{"pi1"}, columns, nil)
	if _, err := records.ListByTask(ctx, "t1"); err != nil {
		t.Fatal(err)
	}
	if _, err := records.ListByBusiness(ctx, "b1"); err != nil {
		t.Fatal(err)
	}
	if _, err := records.ListByProcessInstance(ctx, "pi1"); err != nil {
		t.Fatal(err)
	}
	review := NewReviewTaskRepository(store)
	expectQueryArgs(mock, "FROM workflow_review_task WHERE task_id = \\? LIMIT 1", []driver.Value{"t1"}, columns, nil)
	expectQueryArgs(mock, "FROM workflow_review_task WHERE process_instance_id = \\? ORDER BY create_time", []driver.Value{"pi1"}, columns, nil)
	expectQueryArgs(mock, "FROM workflow_review_task WHERE reviewer = \\? AND status = 0 ORDER BY start_time", []driver.Value{"u1"}, columns, nil)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE workflow_review_task SET review_time = ?, opinion = ?, status = ?, update_time = ? WHERE id = ?")).
		WithArgs(fixedTime, "同意", 1, fixedTime, "rv1").WillReturnResult(sqlmock.NewResult(0, 1))
	if _, found, err := review.GetByTask(ctx, "t1"); err != nil || found {
		t.Fatalf("审阅查询失败: %v", err)
	}
	if _, err := review.ListByProcessInstance(ctx, "pi1"); err != nil {
		t.Fatal(err)
	}
	if _, err := review.ListPendingByReviewer(ctx, "u1"); err != nil {
		t.Fatal(err)
	}
	if _, err := review.MarkReviewed(ctx, "rv1", "同意", fixedTime); err != nil {
		t.Fatal(err)
	}
	exception := NewTaskExceptionRepository(store)
	expectQuery(mock, "FROM workflow_task_exception WHERE handle_status = 0 ORDER BY create_time", columns, nil)
	expectQueryArgs(mock, "FROM workflow_task_exception WHERE process_instance_id = \\? ORDER BY create_time", []driver.Value{"pi1"}, columns, nil)
	expectQueryArgs(mock, "FROM workflow_task_exception WHERE task_id = \\? ORDER BY create_time", []driver.Value{"t1"}, columns, nil)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE workflow_task_exception SET handle_status = ?, update_time = ? WHERE id = ?")).
		WithArgs(1, fixedTime, "e1").WillReturnResult(sqlmock.NewResult(0, 1))
	if _, err := exception.ListUnhandled(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := exception.ListByProcessInstance(ctx, "pi1"); err != nil {
		t.Fatal(err)
	}
	if _, err := exception.ListByTask(ctx, "t1"); err != nil {
		t.Fatal(err)
	}
	if _, err := exception.MarkHandled(ctx, "e1"); err != nil {
		t.Fatal(err)
	}
	direct := NewCompleteToActivityRepository(store)
	expectQueryArgs(mock, "FROM workflow_complete_to_activity WHERE process_instance_id = \\? ORDER BY create_time", []driver.Value{"pi1"}, columns, nil)
	expectQueryArgs(mock, "FROM workflow_complete_to_activity WHERE task_id = \\?", []driver.Value{"t1"}, columns, nil)
	if _, err := direct.ListByProcessInstance(ctx, "pi1"); err != nil {
		t.Fatal(err)
	}
	if _, err := direct.ListByTask(ctx, "t1"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestConfigRepositories 验证配置侧（步骤快照、模板、活动扩展、会签权重）仓储。
// @param t 测试上下文
func TestConfigRepositories(t *testing.T) {
	store, mock, _ := newTestStore(t, DialectMySQL)
	ctx := context.Background()
	columns := []string{"id"}
	hisStep := NewHisStepRepository(store)
	expectQueryArgs(mock, "FROM workflow_his_step WHERE deployment_id = \\? ORDER BY sort", []driver.Value{"dep1"}, columns, nil)
	expectQueryArgs(mock, "FROM workflow_his_step WHERE model_id = \\? ORDER BY sort", []driver.Value{"m1"}, columns, nil)
	expectQueryArgs(mock, "FROM workflow_his_step WHERE deployment_id = \\? AND step_code = \\? LIMIT 1", []driver.Value{"dep1", "sp1"}, columns, nil)
	if _, err := hisStep.ListByDeployment(ctx, "dep1"); err != nil {
		t.Fatal(err)
	}
	if _, err := hisStep.ListByModel(ctx, "m1"); err != nil {
		t.Fatal(err)
	}
	if _, found, err := hisStep.GetByDeploymentAndStepCode(ctx, "dep1", "sp1"); err != nil || found {
		t.Fatalf("快照定位失败: %v", err)
	}
	model := NewModelRepository(store)
	expectQueryArgs(mock, "FROM workflow_model WHERE model_code = \\? AND tenant_id = \\? LIMIT 1", []driver.Value{"leave", "tenant"}, columns, nil)
	expectQueryArgs(mock, "FROM workflow_model WHERE parent_id = \\? ORDER BY sort", []driver.Value{"root"}, columns, nil)
	expectQueryArgs(mock, "FROM workflow_model WHERE tenant_id = \\? ORDER BY sort", []driver.Value{"tenant"}, columns, nil)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE workflow_model SET last_deploy_time = ?, update_time = ? WHERE id = ?")).
		WithArgs(fixedTime, fixedTime, "m1").WillReturnResult(sqlmock.NewResult(0, 1))
	if _, found, err := model.GetByModelCode(ctx, "leave", "tenant"); err != nil || found {
		t.Fatalf("模型查询失败: %v", err)
	}
	if _, err := model.ListByParent(ctx, "root"); err != nil {
		t.Fatal(err)
	}
	if _, err := model.ListByTenant(ctx, "tenant"); err != nil {
		t.Fatal(err)
	}
	if _, err := model.UpdateLastDeployTime(ctx, "m1", fixedTime); err != nil {
		t.Fatal(err)
	}
	step := NewStepRepository(store)
	expectQueryArgs(mock, "FROM workflow_step WHERE model_id = \\? ORDER BY sort", []driver.Value{"m1"}, columns, nil)
	expectQueryArgs(mock, "FROM workflow_step WHERE model_id = \\? AND step_code = \\? ORDER BY sort LIMIT 1", []driver.Value{"m1", "sp1"}, columns, nil)
	if _, err := step.ListByModel(ctx, "m1"); err != nil {
		t.Fatal(err)
	}
	if _, found, err := step.GetByModelAndStepCode(ctx, "m1", "sp1"); err != nil || found {
		t.Fatalf("步骤定位失败: %v", err)
	}
	template := NewTemplateRepository(store)
	expectQueryArgs(mock, "FROM workflow_template WHERE category_id = \\? ORDER BY name", []driver.Value{"c1"}, columns, nil)
	expectQuery(mock, "FROM workflow_template WHERE is_default = 1 AND is_visible = 1 ORDER BY name", columns, nil)
	if _, err := template.ListByCategory(ctx, "c1"); err != nil {
		t.Fatal(err)
	}
	if _, err := template.ListDefault(ctx); err != nil {
		t.Fatal(err)
	}
	actExtend := NewActExtendRepository(store)
	expectQueryArgs(mock, "FROM workflow_act_extend WHERE step_id = \\? ORDER BY activity_id", []driver.Value{"s1"}, columns, nil)
	expectQueryArgs(mock, "FROM workflow_act_extend WHERE process_definition_key = \\? AND activity_id = \\?", []driver.Value{"leave", "act1"}, columns, nil)
	if _, err := actExtend.ListByStep(ctx, "s1"); err != nil {
		t.Fatal(err)
	}
	if _, err := actExtend.ListByDefinitionAndActivity(ctx, "leave", "act1"); err != nil {
		t.Fatal(err)
	}
	weight := NewCountersignWeightRepository(store)
	expectQueryArgs(mock, "FROM workflow_countersign_weight WHERE process_definition_key = \\? AND activity_id = \\? ORDER BY identity_id", []driver.Value{"leave", "act1"}, columns, nil)
	expectQueryArgs(mock, "FROM workflow_countersign_weight WHERE process_definition_key = \\? AND identity_id = \\?", []driver.Value{"leave", "u1"}, columns, nil)
	if _, err := weight.ListByDefinitionAndActivity(ctx, "leave", "act1"); err != nil {
		t.Fatal(err)
	}
	if _, err := weight.ListByIdentity(ctx, "leave", "u1"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestDelegateAndRemindRepositories 验证委托三表、催办两表与业务系统仓储。
// @param t 测试上下文
func TestDelegateAndRemindRepositories(t *testing.T) {
	store, mock, _ := newTestStore(t, DialectMySQL)
	ctx := context.Background()
	columns := []string{"id"}
	delegate := NewTaskDelegateRepository(store)
	expectQueryArgs(mock, "FROM workflow_task_delegate WHERE owner_id = \\? AND enable = 1 ORDER BY create_time", []driver.Value{"u1"}, columns, nil)
	expectQueryArgs(mock, "FROM workflow_task_delegate WHERE source_code = \\? AND enable = 1 ORDER BY create_time", []driver.Value{"tenant"}, columns, nil)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE workflow_task_delegate SET enable = ?, retire = ?, update_time = ? WHERE id = ?")).
		WithArgs(0, 1, fixedTime, "d1").WillReturnResult(sqlmock.NewResult(0, 1))
	if _, err := delegate.ListByOwner(ctx, "u1"); err != nil {
		t.Fatal(err)
	}
	if _, err := delegate.ListBySourceCode(ctx, "tenant"); err != nil {
		t.Fatal(err)
	}
	if _, err := delegate.Retire(ctx, "d1"); err != nil {
		t.Fatal(err)
	}
	consigners := NewTaskConsignersRepository(store)
	expectQueryArgs(mock, "FROM workflow_task_consigners WHERE delegate_id = \\? ORDER BY create_time", []driver.Value{"d1"}, columns, nil)
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM workflow_task_consigners WHERE delegate_id = ?")).
		WithArgs("d1").WillReturnResult(sqlmock.NewResult(0, 1))
	if _, err := consigners.ListByDelegate(ctx, "d1"); err != nil {
		t.Fatal(err)
	}
	if _, err := consigners.DeleteByDelegate(ctx, "d1"); err != nil {
		t.Fatal(err)
	}
	consignObj := NewTaskConsignObjRepository(store)
	expectQueryArgs(mock, "FROM workflow_task_consign_obj WHERE delegate_id = \\? ORDER BY create_time", []driver.Value{"d1"}, columns, nil)
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM workflow_task_consign_obj WHERE delegate_id = ?")).
		WithArgs("d1").WillReturnResult(sqlmock.NewResult(0, 1))
	if _, err := consignObj.ListByDelegate(ctx, "d1"); err != nil {
		t.Fatal(err)
	}
	if _, err := consignObj.DeleteByDelegate(ctx, "d1"); err != nil {
		t.Fatal(err)
	}
	remind := NewRemindRepository(store)
	expectQueryArgs(mock, "FROM workflow_remind WHERE pid = \\? AND deleted = 0 ORDER BY create_time", []driver.Value{"m1"}, columns, nil)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE workflow_remind SET sent = ?, update_time = ? WHERE id = ?")).
		WithArgs(1, fixedTime, "r1").WillReturnResult(sqlmock.NewResult(0, 1))
	if _, err := remind.ListByModel(ctx, "m1"); err != nil {
		t.Fatal(err)
	}
	if _, err := remind.MarkSent(ctx, "r1"); err != nil {
		t.Fatal(err)
	}
	target := NewRemindTargetRepository(store)
	expectQueryArgs(mock, "FROM workflow_remind_target WHERE pid = \\? AND deleted = 0 ORDER BY create_time", []driver.Value{"r1"}, columns, nil)
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM workflow_remind_target WHERE pid = ?")).
		WithArgs("r1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE workflow_remind_target SET sent = ?, update_time = ? WHERE id = ?")).
		WithArgs(1, fixedTime, "rt1").WillReturnResult(sqlmock.NewResult(0, 1))
	if _, err := target.ListByRemind(ctx, "r1"); err != nil {
		t.Fatal(err)
	}
	if _, err := target.DeleteByRemind(ctx, "r1"); err != nil {
		t.Fatal(err)
	}
	if _, err := target.MarkSent(ctx, "rt1"); err != nil {
		t.Fatal(err)
	}
	system := NewBusinessSystemRepository(store)
	expectQueryArgs(mock, "FROM workflow_business_system WHERE system_symbol = \\? LIMIT 1", []driver.Value{"oa"}, columns, nil)
	expectQuery(mock, "FROM workflow_business_system WHERE 1 = 1 ORDER BY system_symbol", columns, nil)
	if _, found, err := system.GetBySymbol(ctx, "oa"); err != nil || found {
		t.Fatalf("系统查询失败: %v", err)
	}
	if _, err := system.ListAll(ctx); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestNewRepositories 验证聚合仓储构建完整覆盖 22 张表。
// @param t 测试上下文
func TestNewRepositories(t *testing.T) {
	store, _, _ := newTestStore(t, DialectMySQL)
	repos := NewRepositories(store)
	if repos.BusinessSystems.def.Name != "workflow_business_system" ||
		repos.Models.def.Name != "workflow_model" ||
		repos.Steps.def.Name != "workflow_step" ||
		repos.HisSteps.def.Name != "workflow_his_step" ||
		repos.Templates.def.Name != "workflow_template" ||
		repos.ActExtends.def.Name != "workflow_act_extend" ||
		repos.CountersignWeights.def.Name != "workflow_countersign_weight" ||
		repos.NextActivities.def.Name != "workflow_next_activity" ||
		repos.HisNextActivities.def.Name != "workflow_his_next_activity" ||
		repos.MessageExtends.def.Name != "workflow_task_message_extend" ||
		repos.HistoryExtends.def.Name != "workflow_task_history_extend" ||
		repos.DelegateTasks.def.Name != "workflow_delegate_task" ||
		repos.Documents.def.Name != "workflow_document" ||
		repos.OperationRecords.def.Name != "workflow_operation_record" ||
		repos.ReviewTasks.def.Name != "workflow_review_task" ||
		repos.TaskExceptions.def.Name != "workflow_task_exception" ||
		repos.CompleteToActivities.def.Name != "workflow_complete_to_activity" ||
		repos.TaskDelegates.def.Name != "workflow_task_delegate" ||
		repos.TaskConsigners.def.Name != "workflow_task_consigners" ||
		repos.TaskConsignObjs.def.Name != "workflow_task_consign_obj" ||
		repos.Reminds.def.Name != "workflow_remind" ||
		repos.RemindTargets.def.Name != "workflow_remind_target" {
		t.Fatal("聚合仓储表绑定不完整")
	}
}

// TestConditionColumnQuoting 验证委托规则表 condition 列按方言转义写入。
// @param t 测试上下文
func TestConditionColumnQuoting(t *testing.T) {
	store, mock, _ := newTestStore(t, DialectPostgres)
	repo := NewTaskDelegateRepository(store)
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE workflow_task_delegate SET "condition" = $1, update_time = $2 WHERE id = $3`)).
		WithArgs("@ProjId='1'", fixedTime, "d1").WillReturnResult(sqlmock.NewResult(0, 1))
	if _, err := repo.UpdateByPK(context.Background(), "d1", Row{"condition": "@ProjId='1'"}); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestErrorsUnwrap 验证仓储错误可解包定位原因。
// @param t 测试上下文
func TestErrorsUnwrap(t *testing.T) {
	store, mock, _ := newTestStore(t, DialectMySQL)
	repo := NewBusinessSystemRepository(store)
	mock.ExpectQuery("SELECT .* FROM workflow_business_system").WillReturnError(errors.New("数据库故障"))
	_, _, err := repo.GetBySymbol(context.Background(), "oa")
	if err == nil {
		t.Fatal("查询失败应报错")
	}
	// 错误链应能定位到根因
	if !regexp.MustCompile("数据库故障").MatchString(err.Error()) {
		t.Fatalf("错误应包含根因: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestDualWriteAndUpsertErrors 验证双写与单据 upsert 的错误传播。
// @param t 测试上下文
func TestDualWriteAndUpsertErrors(t *testing.T) {
	store, mock, _ := newTestStore(t, DialectMySQL)
	next := NewNextActivityRepository(store)
	document := NewDocumentRepository(store)
	ctx := context.Background()
	// 双写时运行态插入失败应直接返回，不再写历史表
	mock.ExpectExec("INSERT INTO workflow_next_activity").WillReturnError(errors.New("数据库故障"))
	if err := next.InsertDual(ctx, Row{"id": "n1"}); err == nil {
		t.Fatal("双写首表失败应报错")
	}
	// 双写时历史表插入失败也应报错
	mock.ExpectExec("INSERT INTO workflow_next_activity").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO workflow_his_next_activity").WillReturnError(errors.New("数据库故障"))
	if err := next.InsertDual(ctx, Row{"id": "n1"}); err == nil {
		t.Fatal("双写历史表失败应报错")
	}
	// upsert 查询失败时错误传播
	mock.ExpectQuery("FROM workflow_document").WillReturnError(errors.New("数据库故障"))
	if err := document.UpsertByBusinessID(ctx, Row{"id": "doc-1", "business_id": "b1", "tenant_id": "t1"}); err == nil {
		t.Fatal("upsert 查询失败应报错")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
