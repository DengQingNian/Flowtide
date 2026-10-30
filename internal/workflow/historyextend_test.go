package workflow

import (
	"context"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/internal/clock"
)

// newHistoryFixture 构造已办扩展域服务测试夹具。
// @param t 测试上下文
// @return 域服务
// @return 内存存储聚合
// @return 固定时钟
func newHistoryFixture(t *testing.T) (*HistoryExtendService, *MemStores, *clock.Fake) {
	t.Helper()
	fake := clock.NewFake(time.Date(2026, 1, 1, 11, 0, 0, 0, time.Local))
	stores := NewMemStores(fake)
	return NewHistoryExtendService(stores.History, fake), stores, fake
}

// sampleHistory 构造样例已办扩展。
// @param taskID 任务 Id
// @param ownerID 处理人 Id
// @return 已办扩展
func sampleHistory(taskID, ownerID string) *HistoryExtend {
	return &HistoryExtend{
		MessageExtend: MessageExtend{
			BaseColumns:       BaseColumns{TenantID: "sys"},
			TaskID:            taskID,
			ProcessInstanceID: "pi-1",
			BusinessID:        "biz-1",
			TaskStepCode:      "step-approve",
			TaskStepName:      "审批",
			OwnerID:           ownerID,
			OwnerName:         "张三",
		},
	}
}

// TestHistoryCreateForTask 验证已办创建与必填校验。
// @param t 测试上下文
func TestHistoryCreateForTask(t *testing.T) {
	service, stores, fake := newHistoryFixture(t)
	ctx := context.Background()
	created, err := service.CreateForTask(ctx, sampleHistory("task-1", "u1"))
	if err != nil {
		t.Fatalf("已办创建失败: %v", err)
	}
	if created.ID == "" || created.Creator != "u1" {
		t.Fatalf("审计列未填充: %+v", created.BaseColumns)
	}
	// 办结时间初始为空
	if created.TaskFinishTime != nil {
		t.Fatalf("办结时间应初始为空: %v", created.TaskFinishTime)
	}
	// 签收时间缺省为时钟时间
	if created.TaskReceiptTime == nil || !created.TaskReceiptTime.Equal(fake.Now()) {
		t.Fatalf("签收时间缺省错误: %v", created.TaskReceiptTime)
	}
	// 缺参校验
	if _, err := service.CreateForTask(ctx, &HistoryExtend{}); err == nil {
		t.Fatal("空请求应报错")
	}
	if _, err := service.CreateForTask(ctx, &HistoryExtend{MessageExtend: MessageExtend{TaskID: "task-1"}}); err == nil {
		t.Fatal("缺处理人/租户应报错")
	}
	// 存入后可按任务查询
	if _, found, err := service.GetByTaskID(ctx, "task-1"); err != nil || !found {
		t.Fatalf("按任务查询失败: %v %v", found, err)
	}
	if rows, _ := stores.History.ListByInstance(ctx, "pi-1"); len(rows) != 1 {
		t.Fatalf("按实例查询失败: %d", len(rows))
	}
}

// TestHistoryUpdateFinishTime 验证办结回写：命中处理人回写、其他处理人删除、缺参校验。
// @param t 测试上下文
func TestHistoryUpdateFinishTime(t *testing.T) {
	service, _, fake := newHistoryFixture(t)
	ctx := context.Background()
	// 同任务两个处理人的已办记录（多人任务办结场景）
	if _, err := service.CreateForTask(ctx, sampleHistory("task-1", "u1")); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateForTask(ctx, sampleHistory("task-1", "u2")); err != nil {
		t.Fatal(err)
	}
	// u1 办结：回写 u1 办结时间并删除 u2 记录
	finishAt := time.Date(2026, 1, 2, 10, 0, 0, 0, time.Local)
	if err := service.UpdateFinishTime(ctx, "task-1", "u1", finishAt); err != nil {
		t.Fatal(err)
	}
	row, found, err := service.GetByTaskID(ctx, "task-1")
	if err != nil || !found {
		t.Fatalf("办结后查询失败: %v %v", found, err)
	}
	if row.OwnerID != "u1" {
		t.Fatalf("其他处理人记录应被删除，实际剩余 %s", row.OwnerID)
	}
	if row.TaskFinishTime == nil || !row.TaskFinishTime.Equal(finishAt) {
		t.Fatalf("办结时间回写错误: %v", row.TaskFinishTime)
	}
	// 零值时间取当前时间
	if _, err := service.CreateForTask(ctx, sampleHistory("task-2", "u1")); err != nil {
		t.Fatal(err)
	}
	if err := service.UpdateFinishTime(ctx, "task-2", "u1", time.Time{}); err != nil {
		t.Fatal(err)
	}
	row, _, _ = service.GetByTaskID(ctx, "task-2")
	if row.TaskFinishTime == nil || !row.TaskFinishTime.Equal(fake.Now()) {
		t.Fatalf("零值时间应取时钟时间: %v", row.TaskFinishTime)
	}
	// 缺参校验
	if err := service.UpdateFinishTime(ctx, "", "u1", time.Time{}); err == nil {
		t.Fatal("缺任务应报错")
	}
	if err := service.UpdateFinishTime(ctx, "task-1", "", time.Time{}); err == nil {
		t.Fatal("缺处理人应报错")
	}
}

// TestHistoryListOrder 验证列表排序（办结时间优先、创建时间兜底）与按业务单据查询。
// @param t 测试上下文
func TestHistoryListOrder(t *testing.T) {
	service, stores, _ := newHistoryFixture(t)
	ctx := context.Background()
	// 第一条无办结时间（兜底创建时间）
	first, err := service.CreateForTask(ctx, sampleHistory("task-1", "u1"))
	if err != nil {
		t.Fatal(err)
	}
	// 第二条办结时间更晚
	second := sampleHistory("task-2", "u2")
	finish := time.Date(2026, 6, 1, 0, 0, 0, 0, time.Local)
	second.TaskFinishTime = &finish
	if _, err := service.CreateForTask(ctx, second); err != nil {
		t.Fatal(err)
	}
	rows, err := service.ListByInstance(ctx, "pi-1")
	if err != nil || len(rows) != 2 {
		t.Fatalf("按实例查询失败: %d %v", len(rows), err)
	}
	if rows[0].ID != first.ID {
		t.Fatalf("无办结时间记录应排在最前: %+v", rows[0])
	}
	// 按业务单据查询
	if rows, err = service.ListByBusinessID(ctx, "biz-1", "sys"); err != nil || len(rows) != 2 {
		t.Fatalf("按业务单据查询失败: %d %v", len(rows), err)
	}
	// 分页查询
	page, err := service.ListByOwner(ctx, "u1", "sys", PageQuery{Start: 0, Size: 1})
	if err != nil || page.Total != 1 || len(page.Data) != 1 {
		t.Fatalf("按处理人分页失败: %d %d %v", page.Total, len(page.Data), err)
	}
	// 删除族：按任务/按业务单据/按步骤代号集合/按实例
	if err := service.store.DeleteByTaskID(ctx, "task-1"); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := service.GetByTaskID(ctx, "task-1"); found {
		t.Fatal("按任务删除失败")
	}
	if err := service.DeleteByBusinessID(ctx, "biz-1", "sys"); err != nil {
		t.Fatal(err)
	}
	if rows, _ = stores.History.ListByInstance(ctx, "pi-1"); len(rows) != 0 {
		t.Fatalf("按业务单据删除失败: %d", len(rows))
	}
	// 重建数据：验证按步骤代号集合删除与按实例删除
	if _, err := service.CreateForTask(ctx, sampleHistory("task-3", "u1")); err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteByProcessInstanceAndActivityIDs(ctx, "pi-1", []string{"step-approve"}); err != nil {
		t.Fatal(err)
	}
	if rows, _ = stores.History.ListByInstance(ctx, "pi-1"); len(rows) != 0 {
		t.Fatalf("按步骤代号删除失败: %d", len(rows))
	}
	if _, err := service.CreateForTask(ctx, sampleHistory("task-4", "u1")); err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteByInstance(ctx, "pi-1"); err != nil {
		t.Fatal(err)
	}
	if rows, _ = stores.History.ListByInstance(ctx, "pi-1"); len(rows) != 0 {
		t.Fatalf("按实例删除失败: %d", len(rows))
	}
}

// TestHistoryServicePaths 验证服务层查询透传与默认时钟回退。
// @param t 测试上下文
func TestHistoryServicePaths(t *testing.T) {
	// nil 时钟回退真实时钟
	service := NewHistoryExtendService(NewMemHistoryStore(nil), nil)
	if service.clock == nil {
		t.Fatal("nil 时钟应回退真实时钟")
	}
	ctx := context.Background()
	if _, err := service.CreateForTask(ctx, sampleHistory("task-1", "u1")); err != nil {
		t.Fatal(err)
	}
	// 服务层分页查询
	page, err := service.ListByOwner(ctx, "u1", "sys", PageQuery{Start: 0, Size: 10})
	if err != nil || page.Total != 1 {
		t.Fatalf("服务层按处理人查询失败: %d %v", page.Total, err)
	}
	// 服务层按任务删除
	if err := service.DeleteByTaskID(ctx, "task-1"); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := service.GetByTaskID(ctx, "task-1"); found {
		t.Fatal("服务层按任务删除失败")
	}
	// 真实时钟下的创建
	real := NewHistoryExtendService(NewMemHistoryStore(nil), nil)
	if _, err := real.CreateForTask(ctx, sampleHistory("task-2", "u1")); err != nil {
		t.Fatalf("真实时钟创建失败: %v", err)
	}
}
