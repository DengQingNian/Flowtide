package workflow

import (
	"context"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/internal/clock"
)

// newMessageFixture 构造待办扩展域服务测试夹具。
// @param t 测试上下文
// @return 域服务
// @return 内存存储聚合
// @return 固定时钟
func newMessageFixture(t *testing.T) (*MessageExtendService, *MemStores, *clock.Fake) {
	t.Helper()
	fake := clock.NewFake(time.Date(2026, 1, 1, 10, 0, 0, 0, time.Local))
	stores := NewMemStores(fake)
	return NewMessageExtendService(stores.Message, fake), stores, fake
}

// sampleMessage 构造样例待办扩展。
// @param taskID 任务 Id
// @param ownerID 处理人 Id
// @return 待办扩展
func sampleMessage(taskID, ownerID string) *MessageCreateRequest {
	return &MessageCreateRequest{
		BaseColumns:       BaseColumns{TenantID: "sys"},
		TaskID:            taskID,
		ProcessInstanceID: "pi-1",
		Title:             "请假申请",
		BusinessID:        "biz-1",
		TaskStepCode:      "step-approve",
		TaskStepName:      "审批",
		FlowCategoryCode:  "leave",
		FlowCategoryName:  "请假流程",
		OwnerID:           ownerID,
		OwnerName:         "张三",
		ExtendFields:      ExtendFields{ExtendS1: "1001", ExtendI1: int64Ptr(7)},
	}
}

// intPtr 构造整型指针。
// @param v 值
// @return 指针
func intPtr(v int) *int { return &v }

// int64Ptr 构造 int64 指针。
// @param v 值
// @return 指针
func int64Ptr(v int64) *int64 { return &v }

// TestMessageCreateForTask 验证待办创建：幂等、必填校验、签收时间缺省、extend 透传。
// @param t 测试上下文
func TestMessageCreateForTask(t *testing.T) {
	service, stores, fake := newMessageFixture(t)
	ctx := context.Background()
	created, err := service.CreateForTask(ctx, sampleMessage("task-1", "u1"))
	if err != nil {
		t.Fatalf("待办创建失败: %v", err)
	}
	if created.ID == "" || created.Creator != "u1" {
		t.Fatalf("审计列未填充: %+v", created.BaseColumns)
	}
	// 签收时间缺省为时钟时间
	if created.TaskReceiptTime == nil || !created.TaskReceiptTime.Equal(fake.Now()) {
		t.Fatalf("签收时间缺省错误: %v", created.TaskReceiptTime)
	}
	// extend 透传
	if created.ExtendS1 != "1001" || created.ExtendI1 == nil || *created.ExtendI1 != 7 {
		t.Fatalf("扩展列透传错误: %+v", created.ExtendFields)
	}
	// 幂等：同任务再次创建返回既有记录
	existing, err := service.CreateForTask(ctx, sampleMessage("task-1", "u1"))
	if err != nil {
		t.Fatal(err)
	}
	if existing.ID != created.ID {
		t.Fatalf("同任务应返回既有记录: %s vs %s", existing.ID, created.ID)
	}
	if rows, _ := stores.Message.ListByInstance(ctx, "pi-1"); len(rows) != 1 {
		t.Fatalf("幂等后应只有 1 条记录，实际 %d", len(rows))
	}
	// 显式签收时间尊重原值
	stamped := time.Date(2025, 12, 31, 8, 0, 0, 0, time.Local)
	req := sampleMessage("task-2", "u1")
	req.TaskReceiptTime = &stamped
	stampedEntity, err := service.CreateForTask(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if !stampedEntity.TaskReceiptTime.Equal(stamped) {
		t.Fatalf("显式签收时间被覆盖: %v", stampedEntity.TaskReceiptTime)
	}
	// 缺参校验
	if _, err := service.CreateForTask(ctx, &MessageCreateRequest{TaskID: "task-3", OwnerID: "u1"}); err == nil {
		t.Fatal("缺实例/租户应报错")
	}
	if _, err := service.CreateForTask(ctx, nil); err == nil {
		t.Fatal("空请求应报错")
	}
}

// TestMessageUpdateReadDelayPlan 验证已读/延时/计划办结时间更新。
// @param t 测试上下文
func TestMessageUpdateReadDelayPlan(t *testing.T) {
	service, _, fake := newMessageFixture(t)
	ctx := context.Background()
	if _, err := service.CreateForTask(ctx, sampleMessage("task-1", "u1")); err != nil {
		t.Fatal(err)
	}
	// 已读回写
	readAt := time.Date(2026, 1, 2, 9, 0, 0, 0, time.Local)
	if err := service.UpdateRead(ctx, "task-1", "u1", readAt); err != nil {
		t.Fatal(err)
	}
	msg, found, err := service.GetByTaskID(ctx, "task-1")
	if err != nil || !found {
		t.Fatalf("待办查询失败: %v %v", found, err)
	}
	if msg.IsRead != 1 || msg.ReadTime == nil || !msg.ReadTime.Equal(readAt) {
		t.Fatalf("已读回写错误: %+v", msg)
	}
	// 他人待办不可标记
	if err := service.UpdateRead(ctx, "task-1", "u2", readAt); err != nil {
		t.Fatalf("他人待办标记应静默: %v", err)
	}
	msg, _, _ = service.GetByTaskID(ctx, "task-1")
	if msg.IsRead != 1 {
		t.Fatal("不应受他人标记影响")
	}
	// 延时回写（零值取当前时间）
	if err := service.UpdateDelay(ctx, "task-1", time.Time{}); err != nil {
		t.Fatal(err)
	}
	msg, _, _ = service.GetByTaskID(ctx, "task-1")
	if msg.IsDelay != 1 || msg.DelayTime == nil || !msg.DelayTime.Equal(fake.Now()) {
		t.Fatalf("延时回写错误: %+v", msg)
	}
	// 计划办结时间
	plan := time.Date(2026, 1, 5, 18, 0, 0, 0, time.Local)
	if err := service.SetPlanFinishTime(ctx, "task-1", &plan); err != nil {
		t.Fatal(err)
	}
	msg, _, _ = service.GetByTaskID(ctx, "task-1")
	if msg.PlanFinishTime == nil || !msg.PlanFinishTime.Equal(plan) {
		t.Fatalf("计划办结时间错误: %v", msg.PlanFinishTime)
	}
	// 清空计划办结时间
	if err := service.SetPlanFinishTime(ctx, "task-1", nil); err != nil {
		t.Fatal(err)
	}
	msg, _, _ = service.GetByTaskID(ctx, "task-1")
	if msg.PlanFinishTime != nil {
		t.Fatalf("计划办结时间未清空: %v", msg.PlanFinishTime)
	}
}

// TestMessageListAndDelete 验证列表分页与各类删除。
// @param t 测试上下文
func TestMessageListAndDelete(t *testing.T) {
	service, stores, _ := newMessageFixture(t)
	ctx := context.Background()
	// 三个人的待办
	for _, owner := range []string{"u1", "u2", "u3"} {
		if _, err := service.CreateForTask(ctx, sampleMessage("task-"+owner, owner)); err != nil {
			t.Fatal(err)
		}
	}
	// 分页：size=2 取第一页
	page, err := service.ListByOwner(ctx, "u1", "sys", PageQuery{Start: 0, Size: 2})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Data) != 1 {
		t.Fatalf("u1 分页错误: total=%d len=%d", page.Total, len(page.Data))
	}
	// 租户不匹配过滤为空
	if page, err = service.ListByOwner(ctx, "u1", "other", PageQuery{}); err != nil || page.Total != 0 {
		t.Fatalf("租户过滤失败: %d %v", page.Total, err)
	}
	// 按业务单据查询
	if rows, err := service.ListByBusinessID(ctx, "biz-1", "sys"); err != nil || len(rows) != 3 {
		t.Fatalf("按业务单据查询失败: %d %v", len(rows), err)
	}
	// 按步骤代号集合删除
	if err := service.DeleteByProcessInstanceAndActivityIDs(ctx, "pi-1", []string{"step-approve", "no-step"}); err != nil {
		t.Fatal(err)
	}
	if rows, _ := stores.Message.ListByInstance(ctx, "pi-1"); len(rows) != 0 {
		t.Fatalf("按步骤删除失败: %d", len(rows))
	}
	// 重建数据后逐类删除
	if _, err := service.CreateForTask(ctx, sampleMessage("task-a", "u1")); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateForTask(ctx, sampleMessage("task-b", "u2")); err != nil {
		t.Fatal(err)
	}
	// 按任务+处理人删除
	if err := service.DeleteByTaskIDAndOwner(ctx, "task-a", "u1"); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := service.GetByTaskID(ctx, "task-a"); found {
		t.Fatal("按任务+处理人删除失败")
	}
	// 按任务删除
	if err := service.DeleteByTaskID(ctx, "task-b"); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := service.GetByTaskID(ctx, "task-b"); found {
		t.Fatal("按任务删除失败")
	}
	// 按业务单据删除
	if _, err := service.CreateForTask(ctx, sampleMessage("task-c", "u3")); err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteByBusinessID(ctx, "biz-1", "sys"); err != nil {
		t.Fatal(err)
	}
	if rows, _ := stores.Message.ListByInstance(ctx, "pi-1"); len(rows) != 0 {
		t.Fatalf("按业务单据删除失败: %d", len(rows))
	}
	// 按实例删除
	if _, err := service.CreateForTask(ctx, sampleMessage("task-d", "u1")); err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteByInstance(ctx, "pi-1"); err != nil {
		t.Fatal(err)
	}
	if rows, _ := stores.Message.ListByInstance(ctx, "pi-1"); len(rows) != 0 {
		t.Fatalf("按实例删除失败: %d", len(rows))
	}
}

// TestMessageCopyAsDelegateP4 验证委托镜像方法为 P4 预留桩。
// @param t 测试上下文
func TestMessageCopyAsDelegateP4(t *testing.T) {
	service, _, _ := newMessageFixture(t)
	if _, err := service.CopyAsDelegate(context.Background(), "task-1", "u2", "李四"); err == nil {
		t.Fatal("P4 预留方法应返回未实现错误")
	}
}

// TestMessageServicePaths 验证服务层查询透传与默认时钟回退。
// @param t 测试上下文
func TestMessageServicePaths(t *testing.T) {
	// nil 时钟回退真实时钟
	service := NewMessageExtendService(NewMemMessageStore(nil), nil)
	if service.clock == nil {
		t.Fatal("nil 时钟应回退真实时钟")
	}
	ctx := context.Background()
	if _, err := service.CreateForTask(ctx, sampleMessage("task-1", "u1")); err != nil {
		t.Fatal(err)
	}
	// 服务层 ListByInstance
	rows, err := service.ListByInstance(ctx, "pi-1")
	if err != nil || len(rows) != 1 {
		t.Fatalf("服务层按实例查询失败: %d %v", len(rows), err)
	}
	// 零值阅读时间取当前时间
	if err := service.UpdateRead(ctx, "task-1", "u1", time.Time{}); err != nil {
		t.Fatal(err)
	}
	msg, found, err := service.GetByTaskID(ctx, "task-1")
	if err != nil || !found || msg.ReadTime == nil || msg.ReadTime.IsZero() {
		t.Fatalf("零值阅读时间应取当前时间: %v %v %v", found, err, msg.ReadTime)
	}
	// 零值延时时间取当前时间
	if err := service.UpdateDelay(ctx, "task-1", time.Time{}); err != nil {
		t.Fatal(err)
	}
	msg, _, _ = service.GetByTaskID(ctx, "task-1")
	if msg.DelayTime == nil || msg.DelayTime.IsZero() {
		t.Fatalf("零值延时时间应取当前时间: %v", msg.DelayTime)
	}
	// 服务层 ListByOwner
	page, err := service.ListByOwner(ctx, "u1", "sys", PageQuery{Start: 0, Size: 10})
	if err != nil || page.Total != 1 {
		t.Fatalf("服务层按处理人查询失败: %d %v", page.Total, err)
	}
}
