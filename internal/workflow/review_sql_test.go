package workflow

import (
	"context"
	"database/sql/driver"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	pwf "github.com/DengQingNian/Flowtide/internal/persistence/workflow"
)

// ─── 审阅域 SQL 适配器守卫测试（模式对齐 store_sql_test.go，不依赖真实数据库）───

// rvReviewColumns 是 workflow_review_task 的列名全集（与 pwf.tableReviewTasks 顺序一致，
// 供 sqlmock 结果集构造；表定义变更时需同步维护）。
var rvReviewColumns = []string{
	"id", "start_time", "task_id", "task_name", "process_instance_id", "process_definition_id",
	"task_key", "business_key", "process_name", "process_definition_key", "process_definition_name",
	"sender", "sender_name", "reviewer", "review_time", "remark", "opinion", "status",
	"creator", "creator_name", "create_time", "updater", "update_time", "tenant_id",
}

// rvActExtendColumns 是 workflow_act_extend 的列名全集（与 pwf.tableActExtends 顺序一致）。
var rvActExtendColumns = []string{
	"id", "activity_id", "activity_name", "process_definition_key", "owner_type", "owner_value", "step_id",
	"creator", "creator_name", "create_time", "updater", "update_time", "tenant_id",
}

// rvNewSQLFixture 基于 sqlmock 构造审阅域 SQL 适配器（真实 pwf 仓储 + 模拟连接池）。
// @param t 测试上下文
// @return 审阅域适配器聚合
// @return 数据库模拟
func rvNewSQLFixture(t *testing.T) (*SQLReviewStores, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("创建 sqlmock 失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store, err := pwf.NewStore(db, pwf.DialectMySQL)
	if err != nil {
		t.Fatalf("创建仓储入口失败: %v", err)
	}
	return NewSQLReviewStoresWithDB(store, db), mock
}

// TestSQLReviewAdapterSkeleton 验证审阅域适配器骨架：聚合完整、裸 SQL 缺连接池报错、编译期断言成立。
// @param t 测试上下文
func TestReviewSQLAdapterSkeleton(t *testing.T) {
	// 无 DB 构造（编译期装配路径）
	set := NewSQLReviewStores(nil)
	if set.Tasks == nil || set.Extends == nil {
		t.Fatal("审阅域适配器聚合应完整")
	}
	// 编译期断言（var _ ReviewTaskStore = ...）
	var taskStore ReviewTaskStore = set.Tasks
	var extendStore ActExtendStore = set.Extends
	if taskStore == nil || extendStore == nil {
		t.Fatal("适配器应满足存储端口")
	}
	// 裸 SQL 依赖连接池：ListByReviewer / CountUnread / ListByDefinitionKey 缺池报错
	if _, _, err := set.Tasks.ListByReviewer(context.Background(), "u1", "sys", PageQuery{}); err == nil {
		t.Fatal("缺连接池 ListByReviewer 应报错")
	}
	if _, err := set.Tasks.CountUnread(context.Background(), "u1", "sys"); err == nil {
		t.Fatal("缺连接池 CountUnread 应报错")
	}
	if _, err := set.Extends.ListByDefinitionKey(context.Background(), "rvProc"); err == nil {
		t.Fatal("缺连接池 ListByDefinitionKey 应报错")
	}
	// WithDB 构造路径（nil 连接池等价于 NewSQLReviewStores）
	setWithDB := NewSQLReviewStoresWithDB(nil, nil)
	if setWithDB.Tasks == nil || setWithDB.Extends == nil {
		t.Fatal("WithDB 构造应完整")
	}
}

// TestSQLReviewRowRoundtrip 验证审阅影子任务实体的行往返一致性（含可空时间列与审计列）。
// @param t 测试上下文
func TestReviewSQLRowRoundtrip(t *testing.T) {
	stamped := time.Date(2026, 10, 31, 10, 0, 0, 0, time.Local)
	review := &ReviewTask{
		BaseColumns:           BaseColumns{ID: "rt-1", Creator: "op", CreatorName: "操作人", CreateTime: stamped, Updater: "op", UpdateTime: stamped, TenantID: "sys"},
		StartTime:             stamped,
		TaskID:                "task-1",
		TaskName:              "申请",
		ProcessInstanceID:     "pi-1",
		ProcessDefinitionID:   "def-1",
		TaskKey:               "apply",
		BusinessKey:           "BIZ-1",
		ProcessName:           "审阅实例",
		ProcessDefinitionKey:  "rvProc",
		ProcessDefinitionName: "审阅测试流程",
		SenderID:              "alice",
		SenderName:            "张三",
		Reviewer:              "carl",
		ReviewTime:            &stamped,
		Remark:                "备注",
		Opinion:               "同意",
		Status:                ReviewStatusDone,
	}
	got := rvReviewFromRow(rvReviewToRow(review))
	if got.TaskID != "task-1" || got.TaskName != "申请" || got.TaskKey != "apply" {
		t.Fatalf("任务字段往返错误: %+v", got)
	}
	if got.ProcessInstanceID != "pi-1" || got.BusinessKey != "BIZ-1" || got.ProcessName != "审阅实例" {
		t.Fatalf("实例字段往返错误: %+v", got)
	}
	if got.ProcessDefinitionKey != "rvProc" || got.ProcessDefinitionName != "审阅测试流程" || got.ProcessDefinitionID != "def-1" {
		t.Fatalf("定义字段往返错误: %+v", got)
	}
	// sender/sender_name 列映射（sender 存 Id）
	if got.SenderID != "alice" || got.SenderName != "张三" {
		t.Fatalf("发送人往返错误: %+v", got)
	}
	if got.Reviewer != "carl" || got.Status != ReviewStatusDone || got.Opinion != "同意" || got.Remark != "备注" {
		t.Fatalf("审阅字段往返错误: %+v", got)
	}
	if got.ReviewTime == nil || !got.ReviewTime.Equal(stamped) || !got.StartTime.Equal(stamped) {
		t.Fatalf("时间列往返错误: %+v", got)
	}
	if got.ID != "rt-1" || got.TenantID != "sys" || got.CreatorName != "操作人" {
		t.Fatalf("审计列往返错误: %+v", got)
	}
	// 可空列为 nil 时保持 NULL 语义
	bare := &ReviewTask{BaseColumns: BaseColumns{ID: "rt-2", TenantID: "sys"}, TaskID: "task-2", Status: ReviewStatusPending}
	bareRow := rvReviewToRow(bare)
	if _, ok := bareRow["review_time"]; ok {
		t.Fatal("nil review_time 应省略")
	}
	if _, ok := bareRow["start_time"]; ok {
		t.Fatal("零值 start_time 应省略")
	}
	bareBack := rvReviewFromRow(bareRow)
	if bareBack.ReviewTime != nil || !bareBack.StartTime.IsZero() {
		t.Fatalf("NULL 语义回读错误: %+v", bareBack)
	}
	// 批量转换
	if rows := rvReviewsFromRows([]pwf.Row{rvReviewToRow(review), bareRow}); len(rows) != 2 {
		t.Fatal("rvReviewsFromRows 错误")
	}
}

// TestSQLActExtendRowRoundtrip 验证审阅人绑定实体的行往返一致性。
// @param t 测试上下文
func TestReviewSQLActExtendRowRoundtrip(t *testing.T) {
	stamped := time.Date(2026, 10, 31, 10, 0, 0, 0, time.Local)
	ext := &ActExtend{
		BaseColumns:          BaseColumns{ID: "ext-1", Creator: "op", CreateTime: stamped, UpdateTime: stamped, TenantID: "sys"},
		ActivityID:           "apply",
		ActivityName:         "申请",
		ProcessDefinitionKey: "rvProc",
		OwnerType:            ActOwnerTypeOrg,
		OwnerValue:           "grpA",
		StepID:               "step-1",
	}
	got := rvActExtendFromRow(rvActExtendToRow(ext))
	if got.ActivityID != "apply" || got.ActivityName != "申请" || got.OwnerType != ActOwnerTypeOrg || got.OwnerValue != "grpA" || got.StepID != "step-1" {
		t.Fatalf("绑定往返错误: %+v", got)
	}
	if got.ID != "ext-1" || got.TenantID != "sys" {
		t.Fatalf("绑定审计列往返错误: %+v", got)
	}
	// 空可空列省略
	bare := &ActExtend{BaseColumns: BaseColumns{ID: "ext-2", TenantID: "sys"}, ActivityID: "handle", ProcessDefinitionKey: "rvProc"}
	bareRow := rvActExtendToRow(bare)
	if _, ok := bareRow["step_id"]; ok {
		t.Fatal("空 step_id 应省略")
	}
	if rows := rvActExtendsFromRows([]pwf.Row{rvActExtendToRow(ext), bareRow}); len(rows) != 2 {
		t.Fatal("rvActExtendsFromRows 错误")
	}
}

// ─── SQL CRUD（sqlmock 覆盖仓储调用路径，双库集成另有环境变量门控测试）───

// rvReviewRowValues 构造一行审阅影子任务结果值（与 rvReviewColumns 顺序对应）。
// @param stamped 时间值
// @return 行值
func rvReviewRowValues(stamped time.Time) []driver.Value {
	return []driver.Value{
		"rt-1", stamped, "task-1", "申请", "pi-1", "def-1",
		"apply", "BIZ-1", "审阅实例", "rvProc", "审阅测试流程",
		"alice", "张三", "carl", stamped, "备注", "同意", 1,
		"op", "操作人", stamped, "op", stamped, "sys",
	}
}

// TestReviewSQLTaskCRUD 审阅影子任务适配器的插入/查询/回写/实例列表与删除/分页/计数。
// @param t 测试上下文
func TestReviewSQLTaskCRUD(t *testing.T) {
	set, mock := rvNewSQLFixture(t)
	stamped := time.Date(2026, 10, 31, 10, 0, 0, 0, time.UTC)
	ctx := context.Background()
	rt := &ReviewTask{
		BaseColumns:           BaseColumns{ID: "rt-1", Creator: "op", CreateTime: stamped, UpdateTime: stamped, TenantID: "sys"},
		StartTime:             stamped,
		TaskID:                "task-1",
		TaskName:              "申请",
		ProcessInstanceID:     "pi-1",
		ProcessDefinitionID:   "def-1",
		TaskKey:               "apply",
		BusinessKey:           "BIZ-1",
		ProcessName:           "审阅实例",
		ProcessDefinitionKey:  "rvProc",
		ProcessDefinitionName: "审阅测试流程",
		SenderID:              "alice",
		SenderName:            "张三",
		Reviewer:              "carl",
		ReviewTime:            &stamped,
		Remark:                "备注",
		Opinion:               "同意",
		Status:                ReviewStatusDone,
	}
	// 插入
	mock.ExpectExec("INSERT INTO workflow_review_task").WillReturnResult(sqlmock.NewResult(1, 1))
	if err := set.Tasks.Insert(ctx, rt); err != nil {
		t.Fatalf("插入失败: %v", err)
	}
	// 插入失败传播
	mock.ExpectExec("INSERT INTO workflow_review_task").WillReturnError(errors.New("数据库故障"))
	if err := set.Tasks.Insert(ctx, rt); err == nil {
		t.Fatal("插入失败应传播错误")
	}
	// 按主键查询
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, start_time, task_id, task_name, process_instance_id, process_definition_id, task_key, business_key, process_name, process_definition_key, process_definition_name, sender, sender_name, reviewer, review_time, remark, opinion, status, creator, creator_name, create_time, updater, update_time, tenant_id FROM workflow_review_task WHERE id = ?")).
		WithArgs("rt-1").
		WillReturnRows(sqlmock.NewRows(rvReviewColumns).AddRow(rvReviewRowValues(stamped)...))
	got, found, err := set.Tasks.Get(ctx, "rt-1")
	if err != nil || !found {
		t.Fatalf("主键查询失败: found=%v err=%v", found, err)
	}
	if got.TaskID != "task-1" || got.Reviewer != "carl" || got.Status != ReviewStatusDone || got.ReviewTime == nil {
		t.Fatalf("主键查询映射错误: %+v", got)
	}
	// 主键不存在
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, start_time, task_id, task_name, process_instance_id, process_definition_id, task_key, business_key, process_name, process_definition_key, process_definition_name, sender, sender_name, reviewer, review_time, remark, opinion, status, creator, creator_name, create_time, updater, update_time, tenant_id FROM workflow_review_task WHERE id = ?")).
		WithArgs("missing").
		WillReturnRows(sqlmock.NewRows(rvReviewColumns))
	if _, found, _ := set.Tasks.Get(ctx, "missing"); found {
		t.Fatal("空结果集应返回不存在")
	}
	// 办结回写（SET 列序按表定义顺序：review_time/remark/opinion/status + 尾部 update_time）
	mock.ExpectExec(regexp.QuoteMeta("UPDATE workflow_review_task SET review_time = ?, remark = ?, opinion = ?, status = ?, update_time = ? WHERE id = ?")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := set.Tasks.Update(ctx, rt); err != nil {
		t.Fatalf("办结回写失败: %v", err)
	}
	// 实例列表
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, start_time, task_id, task_name, process_instance_id, process_definition_id, task_key, business_key, process_name, process_definition_key, process_definition_name, sender, sender_name, reviewer, review_time, remark, opinion, status, creator, creator_name, create_time, updater, update_time, tenant_id FROM workflow_review_task WHERE process_instance_id = ? ORDER BY create_time")).
		WithArgs("pi-1").
		WillReturnRows(sqlmock.NewRows(rvReviewColumns).AddRow(rvReviewRowValues(stamped)...))
	rows, err := set.Tasks.ListByInstance(ctx, "pi-1")
	if err != nil || len(rows) != 1 || rows[0].TaskID != "task-1" {
		t.Fatalf("实例列表失败: n=%d err=%v", len(rows), err)
	}
	// 按实例删除：先列表后逐条删除
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, start_time, task_id, task_name, process_instance_id, process_definition_id, task_key, business_key, process_name, process_definition_key, process_definition_name, sender, sender_name, reviewer, review_time, remark, opinion, status, creator, creator_name, create_time, updater, update_time, tenant_id FROM workflow_review_task WHERE process_instance_id = ? ORDER BY create_time")).
		WithArgs("pi-1").
		WillReturnRows(sqlmock.NewRows(rvReviewColumns).AddRow(rvReviewRowValues(stamped)...))
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM workflow_review_task WHERE id = ?")).WithArgs("rt-1").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := set.Tasks.DeleteByInstance(ctx, "pi-1"); err != nil {
		t.Fatalf("按实例删除失败: %v", err)
	}
	// 按审阅人分页（带租户）
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM workflow_review_task WHERE reviewer = ? AND tenant_id = ? ORDER BY start_time")).
		WithArgs("carl", "sys").
		WillReturnRows(sqlmock.NewRows(rvReviewColumns).AddRow(rvReviewRowValues(stamped)...).AddRow(rvReviewRowValues(stamped)...).AddRow(rvReviewRowValues(stamped)...))
	page, total, err := set.Tasks.ListByReviewer(ctx, "carl", "sys", PageQuery{Start: 0, Size: 2})
	if err != nil || total != 3 || len(page) != 2 {
		t.Fatalf("分页查询失败: total=%d n=%d err=%v", total, len(page), err)
	}
	// 按审阅人分页（空租户不过滤）
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM workflow_review_task WHERE reviewer = ? ORDER BY start_time")).
		WithArgs("carl").
		WillReturnRows(sqlmock.NewRows(rvReviewColumns))
	if _, total, _ := set.Tasks.ListByReviewer(ctx, "carl", "", PageQuery{}); total != 0 {
		t.Fatalf("空租户计数错误: %d", total)
	}
	// 未读计数
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) AS cnt FROM workflow_review_task WHERE reviewer = ? AND status = 0 AND tenant_id = ?")).
		WithArgs("carl", "sys").
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(int64(7)))
	count, err := set.Tasks.CountUnread(ctx, "carl", "sys")
	if err != nil || count != 7 {
		t.Fatalf("未读计数失败: %d err=%v", count, err)
	}
	// 回写含 updater 与 nil review_time（显式置 NULL）
	reopen := &ReviewTask{
		BaseColumns: BaseColumns{ID: "rt-1", Updater: "carl", UpdateTime: stamped},
		Status:      ReviewStatusPending,
		Opinion:     "",
		Remark:      "",
	}
	mock.ExpectExec(regexp.QuoteMeta("UPDATE workflow_review_task SET review_time = ?, remark = ?, opinion = ?, status = ?, updater = ?, update_time = ? WHERE id = ?")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := set.Tasks.Update(ctx, reopen); err != nil {
		t.Fatalf("回写（含 updater/NULL review_time）失败: %v", err)
	}
	// 回写失败传播
	mock.ExpectExec(regexp.QuoteMeta("UPDATE workflow_review_task SET review_time = ?, remark = ?, opinion = ?, status = ?, updater = ?, update_time = ? WHERE id = ?")).
		WillReturnError(errors.New("数据库故障"))
	if err := set.Tasks.Update(ctx, reopen); err == nil {
		t.Fatal("回写失败应传播错误")
	}
	// 按实例删除失败传播
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, start_time, task_id, task_name, process_instance_id, process_definition_id, task_key, business_key, process_name, process_definition_key, process_definition_name, sender, sender_name, reviewer, review_time, remark, opinion, status, creator, creator_name, create_time, updater, update_time, tenant_id FROM workflow_review_task WHERE process_instance_id = ? ORDER BY create_time")).
		WithArgs("pi-1").
		WillReturnRows(sqlmock.NewRows(rvReviewColumns).AddRow(rvReviewRowValues(stamped)...))
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM workflow_review_task WHERE id = ?")).WithArgs("rt-1").WillReturnError(errors.New("数据库故障"))
	if err := set.Tasks.DeleteByInstance(ctx, "pi-1"); err == nil {
		t.Fatal("按实例删除失败应传播错误")
	}
	// 按实例删除的列表查询失败传播
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, start_time, task_id, task_name, process_instance_id, process_definition_id, task_key, business_key, process_name, process_definition_key, process_definition_name, sender, sender_name, reviewer, review_time, remark, opinion, status, creator, creator_name, create_time, updater, update_time, tenant_id FROM workflow_review_task WHERE process_instance_id = ? ORDER BY create_time")).
		WithArgs("pi-1").
		WillReturnError(errors.New("数据库故障"))
	if err := set.Tasks.DeleteByInstance(ctx, "pi-1"); err == nil {
		t.Fatal("删除列表查询失败应传播错误")
	}
	// 全部期望已消费
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL 期望未全部消费: %v", err)
	}
}

// TestReviewSQLActExtendCRUD 审阅人绑定适配器的保存/按节点查询/按定义查询/删除。
// @param t 测试上下文
func TestReviewSQLActExtendCRUD(t *testing.T) {
	set, mock := rvNewSQLFixture(t)
	stamped := time.Date(2026, 10, 31, 10, 0, 0, 0, time.UTC)
	ctx := context.Background()
	ext := &ActExtend{
		BaseColumns:          BaseColumns{ID: "ext-1", Creator: "op", CreateTime: stamped, UpdateTime: stamped, TenantID: "sys"},
		ActivityID:           "apply",
		ActivityName:         "申请",
		ProcessDefinitionKey: "rvProc",
		OwnerType:            ActOwnerTypeOrg,
		OwnerValue:           "grpA",
		StepID:               "step-1",
	}
	// 保存
	mock.ExpectExec("INSERT INTO workflow_act_extend").WillReturnResult(sqlmock.NewResult(1, 1))
	if err := set.Extends.Save(ctx, ext); err != nil {
		t.Fatalf("保存绑定失败: %v", err)
	}
	// 按定义+节点查询（仓储方法列序：WHERE process_definition_key = ? AND activity_id = ?）
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, activity_id, activity_name, process_definition_key, owner_type, owner_value, step_id, creator, creator_name, create_time, updater, update_time, tenant_id FROM workflow_act_extend WHERE process_definition_key = ? AND activity_id = ?")).
		WithArgs("rvProc", "apply").
		WillReturnRows(sqlmock.NewRows(rvActExtendColumns).AddRow("ext-1", "apply", "申请", "rvProc", "org", "grpA", "step-1", "op", "操作人", stamped, "op", stamped, "sys"))
	bindings, err := set.Extends.GetByDefinitionKeyAndActivity(ctx, "rvProc", "apply")
	if err != nil || len(bindings) != 1 || bindings[0].OwnerValue != "grpA" || bindings[0].StepID != "step-1" {
		t.Fatalf("按节点查询失败: n=%d err=%v", len(bindings), err)
	}
	// 按定义查询（裸 SQL 补齐）
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM workflow_act_extend WHERE process_definition_key = ? ORDER BY id")).
		WithArgs("rvProc").
		WillReturnRows(sqlmock.NewRows(rvActExtendColumns).AddRow("ext-1", "apply", "申请", "rvProc", "org", "grpA", "step-1", "op", "操作人", stamped, "op", stamped, "sys"))
	all, err := set.Extends.ListByDefinitionKey(ctx, "rvProc")
	if err != nil || len(all) != 1 {
		t.Fatalf("按定义查询失败: n=%d err=%v", len(all), err)
	}
	// 删除
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM workflow_act_extend WHERE id = ?")).WithArgs("ext-1").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := set.Extends.Delete(ctx, "ext-1"); err != nil {
		t.Fatalf("删除绑定失败: %v", err)
	}
	// 全部期望已消费
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL 期望未全部消费: %v", err)
	}
}
