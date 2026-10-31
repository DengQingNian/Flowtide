package workflow

// facade_extra_test.go 覆盖门面收尾能力：审阅绑定保存/查询、会签权重、签收/委派处理、
// 批量挂起激活、实例分页、身份树、操作按钮聚合。复用 opsFixtureEnv。

import (
	"context"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/tenant"
)

// TestOpsActExtendAndWeights 审阅绑定与权重配置的门面读写。
// @param t 测试上下文
func TestOpsActExtendAndWeights(t *testing.T) {
	env := newOpsService(t)
	ctx := tenant.With(context.Background(), "bizA")

	// 审阅绑定：保存 → 按节点查 → 按定义查
	if _, err := env.service.SaveActExtend(ctx, &ActExtend{
		BaseColumns: BaseColumns{TenantID: "bizA"}, ActivityID: "handle", ActivityName: "办理",
		ProcessDefinitionKey: "opsProc", OwnerType: ActOwnerTypePerson, OwnerValue: "u2",
	}); err != nil {
		t.Fatalf("保存审阅绑定失败: %v", err)
	}
	byNode, err := env.service.ListActExtends(ctx, "opsProc", "handle")
	if err != nil || len(byNode.Data.([]*ActExtend)) != 1 {
		t.Fatalf("按节点查询绑定不符: %+v err=%v", byNode.Data, err)
	}
	all, err := env.service.ListActExtends(ctx, "opsProc", "")
	if err != nil || len(all.Data.([]*ActExtend)) != 1 {
		t.Fatalf("按定义查询绑定不符: %+v err=%v", all.Data, err)
	}
	// 参数缺失拒绝
	if _, err := env.service.SaveActExtend(ctx, &ActExtend{ActivityID: "x"}); err == nil {
		t.Fatalf("缺定义 Key 的绑定应报错")
	}

	// 会签权重：保存 → 查询
	if _, err := env.service.SaveCountersignWeights(ctx, "opsProc", "mtask", "bizA", []CountersignWeight{
		{IdentityID: "u1", AgreeWeight: 1}, {IdentityID: "u2", AgreeWeight: 2},
	}); err != nil {
		t.Fatalf("保存权重失败: %v", err)
	}
	weights, err := env.service.ListCountersignWeights(ctx, "opsProc", "mtask", "bizA")
	if err != nil || len(weights.Data.([]CountersignWeight)) != 2 {
		t.Fatalf("权重查询不符: %+v err=%v", weights.Data, err)
	}
}

// TestOpsClaimResolveAndBatch 签收/委派处理/批量挂起激活门面链路。
// @param t 测试上下文
func TestOpsClaimResolveAndBatch(t *testing.T) {
	env := newOpsService(t)
	ctx := tenant.With(context.Background(), "bizA")
	instanceID, applyTaskID := env.start(t, "opsProc", "BIZ-OPS-CR")

	// 签收（assignee 任务签收到本人亦应成功）
	if _, err := env.service.ClaimTask(ctx, applyTaskID, "u1"); err != nil {
		t.Fatalf("签收失败: %v", err)
	}
	// 委托给 u2 后由 u2 处理恢复
	if err := env.engine.TaskService().Delegate(ctx, applyTaskID, "u2"); err != nil {
		t.Fatalf("委派失败: %v", err)
	}
	if _, err := env.service.ResolveTask(ctx, applyTaskID, "u1"); err != nil {
		t.Fatalf("委派处理失败: %v", err)
	}

	// 批量挂起/激活
	if _, err := env.service.SuspendProcessInstances(ctx, []string{instanceID, ""}, "u1"); err != nil {
		t.Fatalf("批量挂起失败: %v", err)
	}
	suspended, err := env.engine.RuntimeService().GetProcessInstance(ctx, instanceID)
	if err != nil || !suspended.Suspended {
		t.Fatalf("实例应为挂起态: %+v err=%v", suspended, err)
	}
	if _, err := env.service.ActivateProcessInstances(ctx, []string{instanceID}, "u1"); err != nil {
		t.Fatalf("批量激活失败: %v", err)
	}
	activated, err := env.engine.RuntimeService().GetProcessInstance(ctx, instanceID)
	if err != nil || activated.Suspended {
		t.Fatalf("实例应为激活态: %+v err=%v", activated, err)
	}
}

// TestOpsInstancePageAndTreeAndButtons 实例分页/身份树/操作按钮聚合。
// @param t 测试上下文
func TestOpsInstancePageAndTreeAndButtons(t *testing.T) {
	env := newOpsService(t)
	ctx := tenant.With(context.Background(), "bizA")
	instanceID, _ := env.start(t, "opsProc", "BIZ-OPS-PG")

	// 实例分页：第一页 1 条
	page, err := env.service.GetProcessInstancePageList(ctx, PageQuery{Start: 0, Size: 1})
	if err != nil {
		t.Fatalf("实例分页失败: %v", err)
	}
	result := page.Data.(PageResult[api.ProcessInstance])
	if result.Total != 1 || len(result.Data) != 1 || result.Data[0].ID != instanceID {
		t.Fatalf("实例分页不符: %+v", result)
	}

	// 身份树：组 grp（无组成员）+ 未分组 u1/u2
	tree, err := env.service.GetIdentityTree(ctx)
	if err != nil {
		t.Fatalf("身份树查询失败: %v", err)
	}
	if len(tree.Data.([]map[string]any)) == 0 {
		t.Fatalf("身份树不应为空")
	}

	// 操作按钮：运行中实例
	buttons, err := env.service.GetOperationButtons(ctx, instanceID)
	if err != nil {
		t.Fatalf("操作按钮查询失败: %v", err)
	}
	list := buttons.Data.([]string)
	if len(list) == 0 || list[0] != "stop" {
		t.Fatalf("运行中实例按钮不符: %v", list)
	}

	// 已结束实例按钮（含重激活）
	if _, err := env.service.DoStopProcessInstance(ctx, instanceID, "u1", "作废"); err != nil {
		t.Fatalf("软终止失败: %v", err)
	}
	ended, err := env.service.GetOperationButtons(ctx, instanceID)
	if err != nil || ended.Data.([]string)[0] != "reactivate" {
		t.Fatalf("已结束实例按钮不符: %+v err=%v", ended.Data, err)
	}
	_ = time.Now
}
