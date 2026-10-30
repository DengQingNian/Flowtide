package workflow

import (
	"context"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
)

// newOperationFixture 构造操作记录域服务测试夹具（含完整引擎桩）。
// @param t 测试上下文
// @return 域服务
// @return 内存存储聚合
// @return 固定时钟
func newOperationFixture(t *testing.T) (*OperationRecordService, *MemStores, *clock.Fake) {
	t.Helper()
	fake := clock.NewFake(time.Date(2026, 1, 1, 9, 0, 0, 0, time.Local))
	stores := NewMemStores(fake)
	history := &fakeTaskHistory{tasks: map[string]api.HistoricTaskInstance{
		"task-1": {ID: "task-1", Name: "审批", ProcessInstanceID: "pi-1", TaskDefinitionKey: "step-approve", StartTime: fake.Now()},
	}}
	instances := &fakeInstances{
		running: map[string]api.ProcessInstance{
			"pi-1": {ID: "pi-1", BusinessKey: "biz-1", TenantID: "sys", ProcessDefinitionID: "def-1"},
		},
		history: map[string]api.HistoricProcessInstance{
			"pi-2": {ID: "pi-2", BusinessKey: "biz-2", TenantID: "sys", ProcessDefinitionID: "def-1"},
		},
	}
	definitions := &fakeDefinitions{definitions: map[string]api.ProcessDefinition{
		"def-1": {ID: "def-1", Key: "leave", Name: "请假流程"},
	}}
	users := &fakeUsers{users: map[string]api.User{
		"u1": {ID: "u1", DisplayName: "张三"},
	}, groups: map[string][]api.Group{
		"u1": {{ID: "g1", Name: "研发部"}},
	}}
	service := NewOperationRecordService(stores.Operation, history, instances, definitions, users, fake)
	return service, stores, fake
}

// TestOperationRecordSave 验证操作记录落库的引擎补全语义。
// @param t 测试上下文
func TestOperationRecordSave(t *testing.T) {
	service, _, fake := newOperationFixture(t)
	ctx := context.Background()
	agree := 1
	record, err := service.Save(ctx, OperationRequest{TaskID: "task-1", ProcessInstanceID: "pi-1", OwnerID: "u1", IsAgree: &agree, Message: "同意", Type: CommentTypeSP})
	if err != nil {
		t.Fatalf("操作记录落库失败: %v", err)
	}
	// 任务补全：步骤代号/名称/签收时间/办结时间
	if record.TaskStepCode != "step-approve" || record.TaskStepName != "审批" {
		t.Fatalf("任务步骤补全错误: %+v", record)
	}
	if record.TaskReceiptTime == nil || record.TaskFinishTime == nil {
		t.Fatalf("签收/办结时间未补全: %+v", record)
	}
	// 实例补全：业务键/租户/流程分类
	if record.BusinessID != "biz-1" || record.TenantID != "sys" {
		t.Fatalf("业务键/租户补全错误: %+v", record)
	}
	if record.FlowCategoryCode != "leave" || record.FlowCategoryName != "请假流程" {
		t.Fatalf("流程分类补全错误: %+v", record)
	}
	// 处理人补全：姓名/组
	if record.OwnerName != "张三" || record.OwnerGroup != "研发部" || record.OwnerGroupID != "g1" {
		t.Fatalf("处理人补全错误: %+v", record)
	}
	// 审计与时间
	if record.ID == "" || record.Creator != "u1" || record.DealTime == nil {
		t.Fatalf("审计列未填充: %+v", record.BaseColumns)
	}
	if !record.DealTime.Equal(fake.Now()) {
		t.Fatalf("处理时间应为时钟时间: %v", record.DealTime)
	}
	// 已结束实例回退历史实例
	fallback, err := service.Save(ctx, OperationRequest{ProcessInstanceID: "pi-2", OwnerID: "u1", Type: CommentTypeTJ})
	if err != nil {
		t.Fatalf("历史实例回退失败: %v", err)
	}
	if fallback.BusinessID != "biz-2" {
		t.Fatalf("历史实例业务键错误: %+v", fallback)
	}
	// 未知实例：字段留空但不报错
	unknown, err := service.Save(ctx, OperationRequest{ProcessInstanceID: "pi-x", OwnerID: "u1", Type: CommentTypeTJ})
	if err != nil {
		t.Fatalf("未知实例不应报错: %v", err)
	}
	if unknown.BusinessID != "" || unknown.TenantID != "" {
		t.Fatalf("未知实例字段应为空: %+v", unknown)
	}
	// 未知任务/用户容错
	if _, err := service.Save(ctx, OperationRequest{TaskID: "task-x", ProcessInstanceID: "pi-1", OwnerID: "u-ghost", Type: CommentTypeTJ}); err != nil {
		t.Fatalf("未知任务/用户不应报错: %v", err)
	}
	// 缺参校验
	if _, err := service.Save(ctx, OperationRequest{ProcessInstanceID: "pi-1", Type: CommentTypeTJ}); err == nil {
		t.Fatal("缺处理人应报错")
	}
	if _, err := service.Save(ctx, OperationRequest{ProcessInstanceID: "pi-1", OwnerID: "u1"}); err == nil {
		t.Fatal("缺类型应报错")
	}
}

// TestOperationRecordQueries 验证查询族与可签名过滤。
// @param t 测试上下文
func TestOperationRecordQueries(t *testing.T) {
	service, stores, fake := newOperationFixture(t)
	ctx := context.Background()
	agree := 1
	// 写入：TJ（提交）、SP（审批）、TH（退回）、TZ（可签名），逐条推进时钟保证处理时间严格递增
	for _, req := range []OperationRequest{
		{TaskID: "task-1", ProcessInstanceID: "pi-1", OwnerID: "u1", Type: CommentTypeTJ},
		{TaskID: "task-1", ProcessInstanceID: "pi-1", OwnerID: "u1", IsAgree: &agree, Type: CommentTypeSP},
		{TaskID: "task-1", ProcessInstanceID: "pi-1", OwnerID: "u1", Type: CommentTypeTH},
		{TaskID: "task-1", ProcessInstanceID: "pi-1", OwnerID: "u1", Type: CommentTypeTZ},
	} {
		if _, err := service.Save(ctx, req); err != nil {
			t.Fatalf("写入操作记录失败: %v", err)
		}
		fake.Advance(time.Second)
	}
	// 按实例查询（按处理时间升序）
	rows, err := service.ListByInstance(ctx, "pi-1")
	if err != nil || len(rows) != 4 {
		t.Fatalf("按实例查询失败: %d %v", len(rows), err)
	}
	if rows[0].Type != CommentTypeTJ {
		t.Fatalf("应按处理时间升序: %+v", rows[0])
	}
	// 按任务查询
	if rows, err = service.store.ListByTask(ctx, "task-1"); err != nil || len(rows) != 4 {
		t.Fatalf("按任务查询失败: %d %v", len(rows), err)
	}
	// 按业务单据查询
	if rows, err = service.store.ListByBusinessID(ctx, "biz-1"); err != nil || len(rows) != 4 {
		t.Fatalf("按业务单据查询失败: %d %v", len(rows), err)
	}
	// 按业务单据集合查询
	if rows, err = service.ListByBusinessIDs(ctx, []string{"biz-1", "biz-2"}); err != nil || len(rows) != 4 {
		t.Fatalf("按业务单据集合查询失败: %d %v", len(rows), err)
	}
	// 可签名：只取 TJ/SP/TZ 且步骤代号非空（TH 不在三类集合）
	signable, err := service.ListSignable(ctx, []string{"biz-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(signable) != 3 {
		t.Fatalf("可签名记录应 3 条（TJ/SP/TZ，排除 TH），实际 %d", len(signable))
	}
	// 追加一条无步骤代号的 TZ（任务查不到 → 步骤代号空 → 不可签名）
	if _, err := service.Save(ctx, OperationRequest{TaskID: "task-x", ProcessInstanceID: "pi-1", OwnerID: "u1", Type: CommentTypeTZ}); err != nil {
		t.Fatal(err)
	}
	signable, err = service.ListSignable(ctx, []string{"biz-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(signable) != 3 {
		t.Fatalf("无步骤代号记录应被排除，实际 %d", len(signable))
	}
	for _, row := range signable {
		if row.TaskStepCode == "" {
			t.Fatalf("可签名记录步骤代号不应为空: %+v", row)
		}
	}
	// 状态判据：计数与最后类型
	if count, err := stores.Operation.CountByInstance(ctx, "pi-1"); err != nil || count != 5 {
		t.Fatalf("计数失败: %d %v", count, err)
	}
	if lastType, err := stores.Operation.LastTypeByInstance(ctx, "pi-1"); err != nil || lastType != CommentTypeTZ {
		t.Fatalf("最后类型失败: %s %v", lastType, err)
	}
	// 空集合查询
	if rows, err = service.ListByBusinessIDs(ctx, nil); err != nil || len(rows) != 0 {
		t.Fatalf("空集合查询失败: %d %v", len(rows), err)
	}
	// 空实例最后类型给空
	if lastType, err := stores.Operation.LastTypeByInstance(ctx, "pi-none"); err != nil || lastType != "" {
		t.Fatalf("空实例最后类型应为空: %s %v", lastType, err)
	}
}
