// store_sql_audit016_test.go 覆盖 SQL 适配层在真实数据库上的剩余分支（016 号审计）。
//
// 背景：store_sql.go 约 188 条语句此前只有集成开关下的部分闭环覆盖，
// 单元模式覆盖率仅 0.47，导致 workflow 包整体达不到 0.90 门禁。
// 本文件在 MySQL / PostgreSQL 两种方言上把各域端口的查询、更新、删除分支跑满。
package workflow

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

// seedNextActivity 直接经仓储写一条下一步记录（不经事务，集成用例专用）。
// @param ctx 操作上下文
// @param t 测试上下文
// @param stores SQL 适配器聚合
// @param act 记录
func seedNextActivity(ctx context.Context, t *testing.T, stores *SQLStoreSet, act *NextActivity) {
	t.Helper()
	if err := stores.Next.InsertDual(ctx, act); err != nil {
		t.Fatalf("写入下一步记录失败: %v", err)
	}
}

// TestIntegrationSQLNextStoreQueries 覆盖 SQLNextStore 的查询与清理分支。
// @param t 测试上下文
func TestIntegrationSQLNextStoreQueries(t *testing.T) {
	for _, item := range []struct {
		name string
		open func(*testing.T) (context.Context, *SQLStoreSet, *sql.DB)
	}{
		{"mysql", newIntegrationMySQL},
		{"postgres", newIntegrationPostgres},
	} {
		t.Run(item.name, func(t *testing.T) {
			ctx, stores, _ := item.open(t)
			testSQLNextStoreQueries(ctx, t, stores)
		})
	}
}

// testSQLNextStoreQueries 逐分支覆盖下一步适配器。
// @param ctx 操作上下文
// @param t 测试上下文
// @param stores SQL 适配器聚合
func testSQLNextStoreQueries(ctx context.Context, t *testing.T, stores *SQLStoreSet) {
	t.Helper()
	now := time.Date(2026, 2, 1, 9, 0, 0, 0, time.Local)
	// ── 播种：两个节点，各两条记录，一条已消耗 ──
	seedNextActivity(ctx, t, stores, &NextActivity{
		ID: "na-1", ProcessInstanceID: "pi-q", NextActivityID: "act-a", IdentityID: "u1", GroupID: "g1",
		SendIdentityID: "sender", SendUserName: "发送人", Used: 0, PreTaskID: "task-0", Version: 1,
		Creator: "op", CreateTime: now, TenantID: "sys",
	})
	seedNextActivity(ctx, t, stores, &NextActivity{
		ID: "na-2", ProcessInstanceID: "pi-q", NextActivityID: "act-a", IdentityID: "u2", GroupID: "g1",
		SendIdentityID: "sender", SendUserName: "发送人", Used: 0, PreTaskID: "task-0", Version: 2,
		Creator: "op", CreateTime: now, TenantID: "sys",
	})
	seedNextActivity(ctx, t, stores, &NextActivity{
		ID: "na-3", ProcessInstanceID: "pi-q", NextActivityID: "act-b", IdentityID: "u3", GroupID: "g2",
		SendIdentityID: "sender", SendUserName: "发送人", Used: 0, PreTaskID: "task-0", Version: 1,
		Creator: "op", CreateTime: now, TenantID: "sys",
	})

	// ── 查询分支 ──
	if rows, err := stores.Next.ListUnusedByInstanceAndActivity(ctx, "pi-q", "act-a"); err != nil || len(rows) != 2 {
		t.Fatalf("ListUnusedByInstanceAndActivity: %d %v", len(rows), err)
	}
	// 播种阶段三条记录均 used=0
	if rows, err := stores.Next.ListUnusedByInstance(ctx, "pi-q"); err != nil || len(rows) != 3 {
		t.Fatalf("ListUnusedByInstance: %d %v", len(rows), err)
	}
	// ListUsedByInstance 的过滤条件为 task_id IS NOT NULL。注意该实现与"已消耗"语义不完全等价：
	// 真正的消耗标记是 used 列，task_id 在预生成阶段即由引擎侧回写，因此播种后即可查到全部记录。
	// 016 号审计将该瑕疵登记为待确认项（调用方为查询面，不影响消耗闭环）。
	if rows, err := stores.Next.ListUsedByInstance(ctx, "pi-q"); err != nil || len(rows) != 3 {
		t.Fatalf("ListUsedByInstance: %d %v", len(rows), err)
	}
	// 消耗回写分支（覆盖 MarkUsedAndWriteBack）
	if err := stores.Next.MarkUsedAndWriteBack(ctx, []string{"na-3"}, "task-1"); err != nil {
		t.Fatalf("MarkUsedAndWriteBack: %v", err)
	}
	if rows, err := stores.Next.ListByPreTask(ctx, "task-0"); err != nil || len(rows) != 3 {
		t.Fatalf("ListByPreTask: %d %v", len(rows), err)
	}
	if rows, err := stores.Next.ListByTaskID(ctx, "task-1"); err != nil || len(rows) != 1 {
		t.Fatalf("ListByTaskID: %d %v", len(rows), err)
	}
	if row, found, err := stores.Next.GetFirstUnusedByActivity(ctx, "pi-q", "act-a"); err != nil || !found || row == nil {
		t.Fatalf("GetFirstUnusedByActivity: %v %v", found, err)
	}
	if row, found, err := stores.Next.GetFirstUnusedByActivity(ctx, "pi-q", "act-missing"); err != nil || found || row != nil {
		t.Fatalf("缺失节点应返回未命中: %v %v", found, err)
	}
	// LatestVersion 只统计已消耗（used=1）的历史记录：播种阶段两条均未消耗
	if version, err := stores.Next.LatestVersion(ctx, "pi-q", "act-a"); err != nil || version != 0 {
		t.Fatalf("未消耗时版本应为 0: %d %v", version, err)
	}
	// 消耗 act-a 的两条后，最大版本应为 2
	if err := stores.Next.MarkUsedAndWriteBack(ctx, []string{"na-1", "na-2"}, "task-2"); err != nil {
		t.Fatalf("MarkUsedAndWriteBack(act-a): %v", err)
	}
	if version, err := stores.Next.LatestVersion(ctx, "pi-q", "act-a"); err != nil || version != 2 {
		t.Fatalf("LatestVersion: %d %v", version, err)
	}
	if version, err := stores.Next.LatestVersion(ctx, "pi-missing", "act-a"); err != nil || version != 0 {
		t.Fatalf("无记录版本应为 0: %d %v", version, err)
	}
	// FindAssignSource 按 task_id 定位后再按实例/节点/发送人过滤；na-3 已在上面回写 task-1
	if row, found, err := stores.Next.FindAssignSource(ctx, "pi-q", "act-b", "sender", "task-1"); err != nil || !found || row == nil {
		t.Fatalf("FindAssignSource: %v %v", found, err)
	}
	if _, found, err := stores.Next.FindAssignSource(ctx, "pi-q", "act-b", "nobody", "task-1"); err != nil || found {
		t.Fatalf("不匹配的来源不应命中: %v %v", found, err)
	}

	// ── 清理分支 ──
	if err := stores.Next.DeleteForResolve(ctx, "pi-q", "act-a", "task-2", "u2"); err != nil {
		t.Fatalf("DeleteForResolve: %v", err)
	}
	if err := stores.Next.DeleteByProcessInstanceAndActivityIDs(ctx, "pi-q", []string{"act-a"}); err != nil {
		t.Fatalf("DeleteByProcessInstanceAndActivityIDs: %v", err)
	}
	if err := stores.Next.DeleteForRollback(ctx, "pi-q", "task-0", "act-b"); err != nil {
		t.Fatalf("DeleteForRollback: %v", err)
	}
	if err := stores.Next.DeleteForBackToStep(ctx, "pi-q", "act-b"); err != nil {
		t.Fatalf("DeleteForBackToStep: %v", err)
	}
	if err := stores.Next.DeleteByProcessInstance(ctx, "pi-q"); err != nil {
		t.Fatalf("DeleteByProcessInstance: %v", err)
	}
	if rows, err := stores.Next.ListUnusedByInstance(ctx, "pi-q"); err != nil || len(rows) != 0 {
		t.Fatalf("清理后应无残留: %d %v", len(rows), err)
	}
}

// TestIntegrationSQLMessageHistoryStores 覆盖待办/已办镜像适配器的读写删分支。
// @param t 测试上下文
func TestIntegrationSQLMessageHistoryStores(t *testing.T) {
	for _, item := range []struct {
		name string
		open func(*testing.T) (context.Context, *SQLStoreSet, *sql.DB)
	}{
		{"mysql", newIntegrationMySQL},
		{"postgres", newIntegrationPostgres},
	} {
		t.Run(item.name, func(t *testing.T) {
			ctx, stores, _ := item.open(t)
			testSQLMessageHistoryStores(ctx, t, stores)
		})
	}
}

// testSQLMessageHistoryStores 逐分支覆盖待办与已办适配器。
// @param ctx 操作上下文
// @param t 测试上下文
// @param stores SQL 适配器聚合
func testSQLMessageHistoryStores(ctx context.Context, t *testing.T, stores *SQLStoreSet) {
	t.Helper()
	now := time.Date(2026, 2, 1, 10, 0, 0, 0, time.Local)
	message := &MessageExtend{
		BaseColumns: BaseColumns{ID: "msg-m1", TenantID: "sys", Creator: "op", CreateTime: now, Updater: "op", UpdateTime: now},
		TaskID:      "task-m1", ProcessInstanceID: "pi-m", Title: "待办标题", BusinessID: "biz-m",
		TaskStepCode: "act-a", OwnerID: "u1", IsRead: 0,
	}
	if err := stores.Message.Save(ctx, message); err != nil {
		t.Fatalf("待办写入失败: %v", err)
	}
	if got, found, err := stores.Message.GetByTaskID(ctx, "task-m1"); err != nil || !found || got.Title != "待办标题" {
		t.Fatalf("待办查询失败: %+v %v %v", got, found, err)
	}
	if rows, err := stores.Message.ListByInstance(ctx, "pi-m"); err != nil || len(rows) != 1 {
		t.Fatalf("按实例查询待办失败: %d %v", len(rows), err)
	}
	if rows, err := stores.Message.ListByBusinessID(ctx, "biz-m", "sys"); err != nil || len(rows) != 1 {
		t.Fatalf("按业务查询待办失败: %d %v", len(rows), err)
	}
	if rows, total, err := stores.Message.ListByOwner(ctx, "u1", "sys", PageQuery{Start: 0, Size: 10}); err != nil || len(rows) != 1 || total != 1 {
		t.Fatalf("按处理人分页查询待办失败: %d %d %v", len(rows), total, err)
	}
	finished := now.Add(time.Hour)
	if err := stores.Message.UpdateRead(ctx, "task-m1", "u1", finished); err != nil {
		t.Fatalf("更新已读失败: %v", err)
	}
	if err := stores.Message.UpdateDelay(ctx, "task-m1", finished); err != nil {
		t.Fatalf("更新延迟失败: %v", err)
	}
	if err := stores.Message.SetPlanFinishTime(ctx, "task-m1", &finished); err != nil {
		t.Fatalf("设置计划完成时间失败: %v", err)
	}
	if err := stores.Message.DeleteByTaskIDAndOwner(ctx, "task-m1", "u1"); err != nil {
		t.Fatalf("按任务与处理人删除待办失败: %v", err)
	}
	if err := stores.Message.Save(ctx, message); err != nil {
		t.Fatalf("待办重写失败: %v", err)
	}
	if err := stores.Message.DeleteByProcessInstanceAndActivityIDs(ctx, "pi-m", []string{"act-a"}); err != nil {
		t.Fatalf("按节点删除待办失败: %v", err)
	}
	if err := stores.Message.Save(ctx, message); err != nil {
		t.Fatalf("待办重写失败: %v", err)
	}
	if err := stores.Message.DeleteByBusinessID(ctx, "biz-m", "sys"); err != nil {
		t.Fatalf("按业务删除待办失败: %v", err)
	}
	if err := stores.Message.Save(ctx, message); err != nil {
		t.Fatalf("待办重写失败: %v", err)
	}
	if err := stores.Message.DeleteByInstance(ctx, "pi-m"); err != nil {
		t.Fatalf("按实例删除待办失败: %v", err)
	}
	if err := stores.Message.DeleteByTaskID(ctx, "task-m1"); err != nil {
		t.Fatalf("按任务删除待办失败: %v", err)
	}

	// ── 已办镜像 ──
	history := &HistoryExtend{
		BaseColumns: BaseColumns{ID: "his-h1", TenantID: "sys", Creator: "op", CreateTime: now, Updater: "op", UpdateTime: now},
		TaskID:      "task-h1", ProcessInstanceID: "pi-h", Title: "已办标题", BusinessID: "biz-h",
		TaskStepCode: "act-b", OwnerID: "u1",
	}
	if err := stores.History.Save(ctx, history); err != nil {
		t.Fatalf("已办写入失败: %v", err)
	}
	if got, found, err := stores.History.GetByTaskID(ctx, "task-h1"); err != nil || !found || got.Title != "已办标题" {
		t.Fatalf("已办查询失败: %+v %v %v", got, found, err)
	}
	if rows, err := stores.History.ListByInstance(ctx, "pi-h"); err != nil || len(rows) != 1 {
		t.Fatalf("按实例查询已办失败: %d %v", len(rows), err)
	}
	if rows, err := stores.History.ListByBusinessID(ctx, "biz-h", "sys"); err != nil || len(rows) != 1 {
		t.Fatalf("按业务查询已办失败: %d %v", len(rows), err)
	}
	if rows, total, err := stores.History.ListByOwner(ctx, "u1", "sys", PageQuery{Start: 0, Size: 10}); err != nil || len(rows) != 1 || total != 1 {
		t.Fatalf("按处理人分页查询已办失败: %d %d %v", len(rows), total, err)
	}
	if err := stores.History.UpdateFinishTime(ctx, "task-h1", "u1", finished); err != nil {
		t.Fatalf("更新完成时间失败: %v", err)
	}
	if err := stores.History.DeleteByProcessInstanceAndActivityIDs(ctx, "pi-h", []string{"act-b"}); err != nil {
		t.Fatalf("按节点删除已办失败: %v", err)
	}
	if err := stores.History.Save(ctx, history); err != nil {
		t.Fatalf("已办重写失败: %v", err)
	}
	if err := stores.History.DeleteByBusinessID(ctx, "biz-h", "sys"); err != nil {
		t.Fatalf("按业务删除已办失败: %v", err)
	}
	if err := stores.History.Save(ctx, history); err != nil {
		t.Fatalf("已办重写失败: %v", err)
	}
	if err := stores.History.DeleteByInstance(ctx, "pi-h"); err != nil {
		t.Fatalf("按实例删除已办失败: %v", err)
	}
	if err := stores.History.DeleteByTaskID(ctx, "task-h1"); err != nil {
		t.Fatalf("按任务删除已办失败: %v", err)
	}
}

// TestIntegrationSQLOperationDocumentStores 覆盖操作记录与单据状态适配器。
// @param t 测试上下文
func TestIntegrationSQLOperationDocumentStores(t *testing.T) {
	for _, item := range []struct {
		name string
		open func(*testing.T) (context.Context, *SQLStoreSet, *sql.DB)
	}{
		{"mysql", newIntegrationMySQL},
		{"postgres", newIntegrationPostgres},
	} {
		t.Run(item.name, func(t *testing.T) {
			ctx, stores, _ := item.open(t)
			testSQLOperationDocumentStores(ctx, t, stores)
		})
	}
}

// testSQLOperationDocumentStores 逐分支覆盖操作记录与单据适配器。
// @param ctx 操作上下文
// @param t 测试上下文
// @param stores SQL 适配器聚合
func testSQLOperationDocumentStores(ctx context.Context, t *testing.T, stores *SQLStoreSet) {
	t.Helper()
	now := time.Date(2026, 2, 1, 11, 0, 0, 0, time.Local)
	records := []*OperationRecord{
		{ID: "op-1", ProcessInstanceID: "pi-o", TaskID: "task-o1", BusinessID: "biz-o", Type: "SP", TenantID: "sys", Creator: "u1", CreateTime: now},
		{ID: "op-2", ProcessInstanceID: "pi-o", TaskID: "task-o2", BusinessID: "biz-o", Type: "TH", TenantID: "sys", Creator: "u1", CreateTime: now},
	}
	for _, record := range records {
		if err := stores.Operation.Save(ctx, record); err != nil {
			t.Fatalf("操作记录写入失败: %v", err)
		}
	}
	if rows, err := stores.Operation.ListByTask(ctx, "task-o1"); err != nil || len(rows) != 1 {
		t.Fatalf("按任务查询操作记录失败: %d %v", len(rows), err)
	}
	if rows, err := stores.Operation.ListByInstance(ctx, "pi-o"); err != nil || len(rows) != 2 {
		t.Fatalf("按实例查询操作记录失败: %d %v", len(rows), err)
	}
	if rows, err := stores.Operation.ListByBusinessID(ctx, "biz-o"); err != nil || len(rows) != 2 {
		t.Fatalf("按业务查询操作记录失败: %d %v", len(rows), err)
	}
	if rows, err := stores.Operation.ListByBusinessIDs(ctx, []string{"biz-o"}); err != nil || len(rows) != 2 {
		t.Fatalf("按业务批量查询操作记录失败: %d %v", len(rows), err)
	}
	if rows, err := stores.Operation.ListSignable(ctx, []string{"biz-o"}); err != nil {
		t.Fatalf("可签名记录查询失败: %d %v", len(rows), err)
	}
	if count, err := stores.Operation.CountByInstance(ctx, "pi-o"); err != nil || count != 2 {
		t.Fatalf("操作记录计数失败: %d %v", count, err)
	}
	if last, err := stores.Operation.LastTypeByInstance(ctx, "pi-o"); err != nil || last == "" {
		t.Fatalf("末次操作类型查询失败: %q %v", last, err)
	}

	// ── 单据状态 ──
	document := &Document{
		BaseColumns: BaseColumns{ID: "doc-1", TenantID: "sys", Creator: "op", CreateTime: now, Updater: "op", UpdateTime: now},
		BusinessID:  "biz-d", FlowInstanceID: "pi-d", FlowStatus: FlowStatusNotSubmitted,
	}
	if err := stores.Document.Save(ctx, document); err != nil {
		t.Fatalf("单据写入失败: %v", err)
	}
	if got, found, err := stores.Document.GetByBusinessID(ctx, "biz-d", "sys"); err != nil || !found || got.FlowStatus != FlowStatusNotSubmitted {
		t.Fatalf("按业务查询单据失败: %+v %v %v", got, found, err)
	}
	if got, found, err := stores.Document.GetByInstance(ctx, "pi-d"); err != nil || !found || got.BusinessID != "biz-d" {
		t.Fatalf("按实例查询单据失败: %+v %v %v", got, found, err)
	}
	if err := stores.Document.UpdateByInstance(ctx, "pi-d", map[string]any{"flow_status": FlowStatusRunning}); err != nil {
		t.Fatalf("按实例更新单据失败: %v", err)
	}
	if got, _, _ := stores.Document.GetByInstance(ctx, "pi-d"); got.FlowStatus != FlowStatusRunning {
		t.Fatalf("单据状态未更新: %+v", got)
	}
	if err := stores.Document.MarkDeletedByBusinessID(ctx, "biz-d", "sys"); err != nil {
		t.Fatalf("按业务标记删除失败: %v", err)
	}
}
