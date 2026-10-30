package workflow

import (
	"context"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/internal/clock"
)

// newNextFixture 构造下一步域服务测试夹具（内存存储 + 固定时钟）。
// @param t 测试上下文
// @return 域服务
// @return 内存存储聚合
// @return 固定时钟
func newNextFixture(t *testing.T) (*NextActivityService, *MemStores, *clock.Fake) {
	t.Helper()
	fake := clock.NewFake(time.Date(2026, 1, 1, 8, 0, 0, 0, time.Local))
	stores := NewMemStores(fake)
	return NewNextActivityService(stores.Next, fake), stores, fake
}

// TestNextActivityInsertDual 验证双写落库与版本自增。
// @param t 测试上下文
func TestNextActivityInsertDual(t *testing.T) {
	service, stores, _ := newNextFixture(t)
	ctx := context.Background()
	act := &NextActivity{ProcessInstanceID: "pi-1", NextActivityID: "act-b", IdentityID: "u1", TenantID: "sys"}
	if err := service.InsertDual(ctx, act, "op-1"); err != nil {
		t.Fatalf("双写失败: %v", err)
	}
	// 主键由服务生成
	if act.ID == "" {
		t.Fatal("InsertDual 应生成主键")
	}
	// 版本自增为 1（无已消耗记录）
	if act.Version != 1 {
		t.Fatalf("首条版本应为 1，实际 %d", act.Version)
	}
	// 双写：运行态与历史各一条
	if rows, _ := stores.Next.ListUnusedByInstance(ctx, "pi-1"); len(rows) != 1 {
		t.Fatalf("运行态应有 1 条记录，实际 %d", len(rows))
	}
	if rows, _ := stores.Next.ListHisByInstance(ctx, "pi-1"); len(rows) != 1 {
		t.Fatalf("历史表应有 1 条记录，实际 %d", len(rows))
	}
	// 审计列填充
	if act.Creator != "op-1" || act.Updater != "op-1" || act.CreateTime.IsZero() {
		t.Fatalf("审计列未填充: %+v", act.BaseColumns)
	}
	// 缺少执行人报错
	if err := service.InsertDual(ctx, &NextActivity{ProcessInstanceID: "pi-1", NextActivityID: "act-b"}, "op-1"); err == nil {
		t.Fatal("缺少执行人应报错")
	}
}

// TestNextActivitySaveFirstTaskUser 验证首步预生成 used=1 + taskId 语义。
// @param t 测试上下文
func TestNextActivitySaveFirstTaskUser(t *testing.T) {
	service, stores, _ := newNextFixture(t)
	ctx := context.Background()
	err := service.SaveFirstTaskUser(ctx, FirstStepRequest{
		ProcessInstanceID: "pi-1",
		ActivityID:        "act-first",
		IdentityIDs:       []string{"u1", "", "null", "u2"},
		GroupIDs:          []string{"g1", "g2"},
		SendIdentityID:    "sender",
		SendUserName:      "发送人",
		TaskID:            "task-1",
		TenantID:          "sys",
		Creator:           "op-1",
	})
	if err != nil {
		t.Fatalf("首步预生成失败: %v", err)
	}
	rows, err := stores.Next.ListUsedByInstance(ctx, "pi-1")
	if err != nil || len(rows) != 2 {
		t.Fatalf("应落 2 条记录（清洗空白与 null），实际 %d %v", len(rows), err)
	}
	for _, row := range rows {
		// Java 首步语义：used=1、taskId=首任务、preTaskId 不设置
		if row.Used != 1 {
			t.Fatalf("首步记录应为已消耗 used=1，实际 %d", row.Used)
		}
		if row.TaskID != "task-1" {
			t.Fatalf("首步记录应回写任务 ID，实际 %q", row.TaskID)
		}
		if row.PreTaskID != "" {
			t.Fatalf("首步记录不应有来源任务，实际 %q", row.PreTaskID)
		}
		if row.SendIdentityID != "sender" || row.SendUserName != "发送人" {
			t.Fatalf("发送人信息错误: %+v", row)
		}
	}
	// 组按下标配对
	byUser := map[string]string{}
	for _, row := range rows {
		byUser[row.IdentityID] = row.GroupID
	}
	if byUser["u1"] != "g1" || byUser["u2"] != "g2" {
		t.Fatalf("组应按下标配对: %v", byUser)
	}
	// 单组共享
	if err := service.SaveFirstTaskUser(ctx, FirstStepRequest{ProcessInstanceID: "pi-2", ActivityID: "a", IdentityIDs: []string{"u1", "u2"}, GroupIDs: []string{"g9"}, TaskID: "t2", TenantID: "sys"}); err != nil {
		t.Fatal(err)
	}
	rows2, _ := stores.Next.ListUnusedByInstance(ctx, "pi-2")
	for _, row := range rows2 {
		if row.GroupID != "g9" {
			t.Fatalf("单组应共享: %+v", row)
		}
	}
	// 缺执行人报错
	if err := service.SaveFirstTaskUser(ctx, FirstStepRequest{ProcessInstanceID: "pi-3", ActivityID: "a", IdentityIDs: []string{" "}, TenantID: "sys"}); err == nil {
		t.Fatal("清洗后执行人为空应报错")
	}
}

// TestNextActivitySaveAgreeAndBackToStep 验证同意/退回预存 used=0 与版本推进。
// @param t 测试上下文
func TestNextActivitySaveAgreeAndBackToStep(t *testing.T) {
	service, stores, _ := newNextFixture(t)
	ctx := context.Background()
	req := NextActivitySaveRequest{
		ProcessInstanceID: "pi-1",
		ActivityID:        "act-b",
		IdentityIDs:       []string{"u1", "u2"},
		PreTaskID:         "task-cur",
		SendIdentityID:    "sender",
		SendUserName:      "发送人",
		TenantID:          "sys",
		Creator:           "op-1",
	}
	if err := service.SaveAgreeNext(ctx, req); err != nil {
		t.Fatalf("同意预存失败: %v", err)
	}
	rows, _ := stores.Next.ListUnusedByInstanceAndActivity(ctx, "pi-1", "act-b")
	if len(rows) != 2 {
		t.Fatalf("应落 2 条记录，实际 %d", len(rows))
	}
	for _, row := range rows {
		// Java saveAgreeNextActivity 语义：used=0、taskId 空、preTaskId=当前任务
		if row.Used != 0 || row.TaskID != "" || row.PreTaskID != "task-cur" {
			t.Fatalf("同意预存语义错误: %+v", row)
		}
		if row.Version != 1 {
			t.Fatalf("首版应为 1，实际 %d", row.Version)
		}
	}
	// 退回预存与同意同构
	req2 := req
	req2.ActivityID = "act-back"
	if err := service.SaveBackToStepNext(ctx, req2); err != nil {
		t.Fatalf("退回预存失败: %v", err)
	}
	backRows, _ := stores.Next.ListUnusedByInstanceAndActivity(ctx, "pi-1", "act-back")
	if len(backRows) != 2 || backRows[0].Used != 0 {
		t.Fatalf("退回预存语义错误: %+v", backRows)
	}
	// 消耗后同节点再预存：版本推进为 2
	if err := service.MarkUsedAndWriteBack(ctx, []string{rows[0].ID, rows[1].ID}, "task-next"); err != nil {
		t.Fatal(err)
	}
	if err := service.SaveAgreeNext(ctx, req); err != nil {
		t.Fatal(err)
	}
	rows, _ = stores.Next.ListUnusedByInstanceAndActivity(ctx, "pi-1", "act-b")
	if len(rows) != 2 || rows[0].Version != 2 {
		t.Fatalf("消耗后再预存版本应为 2: %+v", rows)
	}
	// 缺执行人报错
	if err := service.SaveAgreeNext(ctx, NextActivitySaveRequest{ProcessInstanceID: "pi-1", ActivityID: "a", IdentityIDs: nil, TenantID: "sys"}); err == nil {
		t.Fatal("执行人为空应报错")
	}
}

// TestNextActivitySaveResolveNext 验证还原预存：组继承、used=1、无来源跳过。
// @param t 测试上下文
func TestNextActivitySaveResolveNext(t *testing.T) {
	service, stores, _ := newNextFixture(t)
	ctx := context.Background()
	// 无既有记录：跳过插入（Java 语义）
	err := service.SaveResolveNext(ctx, ResolveNextSaveRequest{
		ProcessInstanceID: "pi-1", ActivityID: "act-b", IdentityIDs: []string{"u1"},
		TaskID: "task-1", SendIdentityID: "delegator", TenantID: "sys", Creator: "op-1",
	})
	if err != nil {
		t.Fatalf("还原预存失败: %v", err)
	}
	if rows, _ := stores.Next.ListUnusedByInstance(ctx, "pi-1"); len(rows) != 0 {
		t.Fatalf("无来源记录时不应落库，实际 %d", len(rows))
	}
	// 造一条来源记录（转办写入 used=1 + taskId，sendIdentityId 为受托人）
	source := &NextActivity{
		ProcessInstanceID: "pi-1", NextActivityID: "act-b", IdentityID: "delegator",
		SendIdentityID: "delegator", TaskID: "task-1", Used: 1, GroupID: "g-inherit", Version: 1, TenantID: "sys",
	}
	if err := stores.Next.InsertDual(ctx, source); err != nil {
		t.Fatal(err)
	}
	err = service.SaveResolveNext(ctx, ResolveNextSaveRequest{
		ProcessInstanceID: "pi-1", ActivityID: "act-b", IdentityIDs: []string{"u1", "u2"},
		TaskID: "task-1", SendIdentityID: "delegator", SendUserName: "受托人", TenantID: "sys", Creator: "op-1",
	})
	if err != nil {
		t.Fatalf("还原预存失败: %v", err)
	}
	rows, _ := stores.Next.ListByTaskID(ctx, "task-1")
	resolveRows := make([]*NextActivity, 0)
	for _, row := range rows {
		// 新增的两条还原记录 IdentityID 为 u1/u2
		if row.IdentityID == "u1" || row.IdentityID == "u2" {
			resolveRows = append(resolveRows, row)
		}
	}
	if len(resolveRows) != 2 {
		t.Fatalf("应落 2 条还原记录，实际 %d", len(resolveRows))
	}
	for _, row := range resolveRows {
		// Java saveResolveNextActivity 语义：used=1、taskId=当前任务、groupId 继承
		if row.Used != 1 || row.TaskID != "task-1" || row.GroupID != "g-inherit" {
			t.Fatalf("还原语义错误: %+v", row)
		}
		if row.PreTaskID != "task-1" {
			t.Fatalf("还原记录来源任务应为当前任务: %+v", row)
		}
	}
	// 缺任务 ID 报错
	if err := service.SaveResolveNext(ctx, ResolveNextSaveRequest{ProcessInstanceID: "pi-1", ActivityID: "a", IdentityIDs: []string{"u1"}, TenantID: "sys"}); err == nil {
		t.Fatal("缺任务 ID 应报错")
	}
}

// TestNextActivityMarkUsedAndWriteBack 验证消耗与任务回写（含 used=0 门控）。
// @param t 测试上下文
func TestNextActivityMarkUsedAndWriteBack(t *testing.T) {
	service, stores, _ := newNextFixture(t)
	ctx := context.Background()
	req := NextActivitySaveRequest{ProcessInstanceID: "pi-1", ActivityID: "act-b", IdentityIDs: []string{"u1", "u2"}, PreTaskID: "t0", TenantID: "sys"}
	if err := service.SaveAgreeNext(ctx, req); err != nil {
		t.Fatal(err)
	}
	rows, _ := stores.Next.ListUnusedByInstanceAndActivity(ctx, "pi-1", "act-b")
	if err := service.MarkUsedAndWriteBack(ctx, []string{rows[0].ID, rows[1].ID}, "task-1"); err != nil {
		t.Fatal(err)
	}
	// 未消耗清零
	if again, _ := stores.Next.ListUnusedByInstanceAndActivity(ctx, "pi-1", "act-b"); len(again) != 0 {
		t.Fatalf("消耗后仍有未消耗记录: %d", len(again))
	}
	his, _ := stores.Next.ListHisByInstance(ctx, "pi-1")
	for _, row := range his {
		if row.Used != 1 || row.TaskID != "task-1" {
			t.Fatalf("历史表未同步回写: %+v", row)
		}
	}
	// 已消耗记录不可重复消耗（used=0 门控）
	if err := service.MarkUsedAndWriteBack(ctx, []string{rows[0].ID}, "task-2"); err != nil {
		t.Fatal(err)
	}
	his, _ = stores.Next.ListHisByInstance(ctx, "pi-1")
	for _, row := range his {
		if row.TaskID != "task-1" {
			t.Fatalf("已消耗记录被重复回写: %+v", row)
		}
	}
	// 空集合/空任务报错
	if err := service.MarkUsedAndWriteBack(ctx, nil, "task-1"); err == nil {
		t.Fatal("空集合应报错")
	}
	if err := service.MarkUsedAndWriteBack(ctx, []string{"x"}, ""); err == nil {
		t.Fatal("空任务 ID 应报错")
	}
}

// TestNextActivityDeleteRules 验证退回/撤回/还原/按节点删除规则。
// @param t 测试上下文
func TestNextActivityDeleteRules(t *testing.T) {
	service, stores, _ := newNextFixture(t)
	ctx := context.Background()
	// 造数据：task-1 → act-b(u1/u2 used=0)、act-c(u3)、act-keep(u4)
	for _, actID := range []string{"act-b", "act-c", "act-keep"} {
		if err := service.SaveAgreeNext(ctx, NextActivitySaveRequest{ProcessInstanceID: "pi-1", ActivityID: actID, IdentityIDs: []string{"u-" + actID}, PreTaskID: "task-1", TenantID: "sys"}); err != nil {
			t.Fatal(err)
		}
	}
	// DeleteForBackToStep：删目标节点全部记录
	if err := service.DeleteForBackToStep(ctx, "pi-1", "act-b"); err != nil {
		t.Fatal(err)
	}
	if rows, _ := stores.Next.ListUnusedByInstanceAndActivity(ctx, "pi-1", "act-b"); len(rows) != 0 {
		t.Fatalf("退回清理后仍有目标节点记录: %d", len(rows))
	}
	// DeleteForRollback：删以 task-1 为来源、act-keep 之外的记录
	if err := service.DeleteForRollback(ctx, "pi-1", "task-1", "act-keep"); err != nil {
		t.Fatal(err)
	}
	if rows, _ := stores.Next.ListUnusedByInstanceAndActivity(ctx, "pi-1", "act-c"); len(rows) != 0 {
		t.Fatalf("撤回清理后 act-c 仍有记录: %d", len(rows))
	}
	if rows, _ := stores.Next.ListUnusedByInstanceAndActivity(ctx, "pi-1", "act-keep"); len(rows) != 1 {
		t.Fatalf("保留节点被误删: %d", len(rows))
	}
	// DeleteByProcessInstanceAndActivityIDs：两套表同步删
	if err := service.SaveAgreeNext(ctx, NextActivitySaveRequest{ProcessInstanceID: "pi-2", ActivityID: "act-x", IdentityIDs: []string{"u1"}, TenantID: "sys"}); err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteByProcessInstanceAndActivityIDs(ctx, "pi-2", []string{"act-x"}); err != nil {
		t.Fatal(err)
	}
	if rows, _ := stores.Next.ListUnusedByInstance(ctx, "pi-2"); len(rows) != 0 {
		t.Fatalf("按节点删除后运行态仍有记录: %d", len(rows))
	}
	if rows, _ := stores.Next.ListHisByInstance(ctx, "pi-2"); len(rows) != 0 {
		t.Fatalf("按节点删除后历史表仍有记录: %d", len(rows))
	}
	// DeleteForResolve：按（实例/节点/任务/执行人）删
	if err := service.SaveAgreeNext(ctx, NextActivitySaveRequest{ProcessInstanceID: "pi-3", ActivityID: "act-r", IdentityIDs: []string{"u1", "u2"}, TenantID: "sys"}); err != nil {
		t.Fatal(err)
	}
	rows, _ := stores.Next.ListUnusedByInstanceAndActivity(ctx, "pi-3", "act-r")
	for _, row := range rows {
		// 先消耗挂任务，模拟转办记录
		if err := service.MarkUsedAndWriteBack(ctx, []string{row.ID}, "task-9"); err != nil {
			t.Fatal(err)
		}
	}
	if err := service.DeleteForResolve(ctx, "pi-3", "act-r", "task-9", "u1"); err != nil {
		t.Fatal(err)
	}
	left, _ := stores.Next.ListByTaskID(ctx, "task-9")
	if len(left) != 1 || left[0].IdentityID != "u2" {
		t.Fatalf("还原清理应只删 u1 记录: %+v", left)
	}
	// DeleteByProcessInstance：清空运行态
	if err := service.DeleteByProcessInstance(ctx, "pi-3"); err != nil {
		t.Fatal(err)
	}
	if rows, _ := stores.Next.ListByTaskID(ctx, "task-9"); len(rows) != 0 {
		t.Fatalf("按实例删除后仍有记录: %d", len(rows))
	}
}

// TestNextActivityQueries 验证查询族（按来源任务/最新版本/历史追溯/首条未消耗）。
// @param t 测试上下文
func TestNextActivityQueries(t *testing.T) {
	service, stores, _ := newNextFixture(t)
	ctx := context.Background()
	// LatestVersion 空库为 0
	if version, err := service.LatestVersion(ctx, "pi-1", "act-b"); err != nil || version != 0 {
		t.Fatalf("空库最新版本应为 0: %d %v", version, err)
	}
	if err := service.SaveAgreeNext(ctx, NextActivitySaveRequest{ProcessInstanceID: "pi-1", ActivityID: "act-b", IdentityIDs: []string{"u1"}, PreTaskID: "task-1", TenantID: "sys"}); err != nil {
		t.Fatal(err)
	}
	rows, _ := stores.Next.ListUnusedByInstance(ctx, "pi-1")
	if err := service.MarkUsedAndWriteBack(ctx, []string{rows[0].ID}, "task-2"); err != nil {
		t.Fatal(err)
	}
	// 已消耗记录计入最新版本
	if version, err := service.LatestVersion(ctx, "pi-1", "act-b"); err != nil || version != 1 {
		t.Fatalf("最新版本应为 1: %d %v", version, err)
	}
	// ListByPreTask
	if list, err := service.ListByPreTask(ctx, "task-1"); err != nil || len(list) != 1 {
		t.Fatalf("按来源任务查询失败: %d %v", len(list), err)
	}
	// ListHisByInstance 追溯（含 used 状态）
	his, err := service.ListHisByInstance(ctx, "pi-1")
	if err != nil || len(his) != 1 || his[0].Used != 1 {
		t.Fatalf("历史追溯失败: %d %v", len(his), err)
	}
	// ListHisByTaskID 排除自环
	self := &NextActivity{ProcessInstanceID: "pi-1", NextActivityID: "act-self", IdentityID: "u9", TaskID: "task-2", PreTaskID: "task-2", Used: 1, Version: 1, TenantID: "sys"}
	if err := stores.Next.InsertDual(ctx, self); err != nil {
		t.Fatal(err)
	}
	if list, err := service.ListHisByTaskID(ctx, "task-2"); err != nil || len(list) != 1 {
		t.Fatalf("自环排除失败: %d %v", len(list), err)
	}
	// GetFirstUnusedByActivity 取最早一条
	if err := service.SaveAgreeNext(ctx, NextActivitySaveRequest{ProcessInstanceID: "pi-4", ActivityID: "act-f", IdentityIDs: []string{"u1", "u2"}, TenantID: "sys"}); err != nil {
		t.Fatal(err)
	}
	first, found, err := service.GetFirstUnusedByActivity(ctx, "pi-4", "act-f")
	if err != nil || !found || first == nil {
		t.Fatalf("首条未消耗查询失败: %v %v", found, err)
	}
	// ListUnusedByInstance
	if list, err := service.ListUnusedByInstance(ctx, "pi-4"); err != nil || len(list) != 2 {
		t.Fatalf("实例未消耗查询失败: %d %v", len(list), err)
	}
	// ListUsedByInstance（已回写任务 ID 的记录：消耗记录 + 自环记录）
	if list, err := stores.Next.ListUsedByInstance(ctx, "pi-1"); err != nil || len(list) != 2 {
		t.Fatalf("已消耗查询失败: %d %v", len(list), err)
	}
	// 不存在节点无未消耗记录
	if _, found, err := service.GetFirstUnusedByActivity(ctx, "pi-4", "no-act"); err != nil || found {
		t.Fatalf("不存在节点应返回未找到: %v %v", found, err)
	}
}

// TestNextActivityServiceQueriesAndDefaults 验证服务层查询透传与默认时钟回退。
// @param t 测试上下文
func TestNextActivityServiceQueriesAndDefaults(t *testing.T) {
	// nil 时钟回退真实时钟
	service := NewNextActivityService(NewMemNextStore(nil), nil)
	if service.clock == nil {
		t.Fatal("nil 时钟应回退真实时钟")
	}
	ctx := context.Background()
	if err := service.SaveAgreeNext(ctx, NextActivitySaveRequest{ProcessInstanceID: "pi-1", ActivityID: "act-b", IdentityIDs: []string{"u1"}, TenantID: "sys"}); err != nil {
		t.Fatal(err)
	}
	// 服务层 ListUnusedByInstanceAndActivity 透传
	rows, err := service.ListUnusedByInstanceAndActivity(ctx, "pi-1", "act-b")
	if err != nil || len(rows) != 1 {
		t.Fatalf("服务层未消耗查询失败: %d %v", len(rows), err)
	}
	// 服务层 ListByTaskID（消耗挂任务后查询）
	if err := service.MarkUsedAndWriteBack(ctx, []string{rows[0].ID}, "task-1"); err != nil {
		t.Fatal(err)
	}
	byTask, err := service.ListByTaskID(ctx, "task-1")
	if err != nil || len(byTask) != 1 {
		t.Fatalf("服务层按任务查询失败: %d %v", len(byTask), err)
	}
	// 真实时钟下的插入（时间非零即可）
	real := NewNextActivityService(NewMemNextStore(nil), nil)
	if err := real.InsertDual(ctx, &NextActivity{ProcessInstanceID: "pi-2", NextActivityID: "a", IdentityID: "u1", TenantID: "sys"}, "op"); err != nil {
		t.Fatalf("真实时钟插入失败: %v", err)
	}
}
