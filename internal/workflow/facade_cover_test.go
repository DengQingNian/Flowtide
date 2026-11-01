package workflow

// facade_cover_test.go 覆盖门面层薄封装目前未被覆盖的方法与分支：
// 引擎适配器（变量读取/调度退回）、退回与多实例成员、委托/催办/异常、审阅影子任务、
// 设计器 XML 存取、模型/步骤/模板管理，以及服务装配（SQL 与内存）与历史查询入口。
// 全部用例使用固定假时钟与显式时间常量，不依赖真实时间、睡眠或随机数。

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
	pwf "github.com/DengQingNian/Flowtide/internal/persistence/workflow"
	"github.com/DengQingNian/Flowtide/internal/tenant"
	_ "github.com/go-sql-driver/mysql"
)

// facadeCoverTime 是门面覆盖测试的固定时钟基准（与 opsFixture 的假时钟一致）。
var facadeCoverTime = time.Date(2026, 10, 31, 8, 0, 0, 0, time.UTC)

// facadeCoverErrCode 断言错误为指定错误码的 api.Error。
// @param t 测试上下文
// @param err 实际错误
// @param code 期望错误码
func facadeCoverErrCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("期望错误码 %s，实际无错误", code)
	}
	var apiErr *api.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("期望 api.Error，实际 %T: %v", err, err)
	}
	if apiErr.Code != code {
		t.Fatalf("错误码不符：期望 %s，实际 %s（%s）", code, apiErr.Code, apiErr.Message)
	}
}

// facadeCoverCurrentTask 查询实例当前第一个运行态任务。
// @param t 测试上下文
// @param engine 引擎门面
// @param instanceID 流程实例 Id
// @return 当前任务
func facadeCoverCurrentTask(t *testing.T, engine api.Engine, instanceID string) api.Task {
	t.Helper()
	page, err := engine.TaskService().ListTasks(tenant.With(context.Background(), "bizA"), api.TaskFilter{
		ProcessInstanceID: instanceID, Size: 10,
	})
	if err != nil {
		t.Fatalf("查询实例任务失败: %v", err)
	}
	if len(page.Data) == 0 {
		t.Fatalf("实例 %s 无运行态任务", instanceID)
	}
	return page.Data[0]
}

// facadeCoverExecutorList 读取实例的会签执行者集合变量。
// @param t 测试上下文
// @param engine 引擎门面
// @param instanceID 流程实例 Id
// @return 成员列表
func facadeCoverExecutorList(t *testing.T, engine api.Engine, instanceID string) []string {
	t.Helper()
	raw, err := engine.RuntimeService().GetVariable(context.Background(), instanceID, VarExecutorList)
	if err != nil {
		t.Fatalf("读取 executorList 失败: %v", err)
	}
	list, ok := raw.([]string)
	if !ok {
		t.Fatalf("executorList 应为 []string，实际 %T", raw)
	}
	return list
}

// TestFacadeCoverGetVariableAndBackAdapter 覆盖变量读取适配器、调度退回适配器与任意节点退回门面。
// @param t 测试上下文
func TestFacadeCoverGetVariableAndBackAdapter(t *testing.T) {
	env := newOpsService(t)
	ctx := tenant.With(context.Background(), "bizA")
	instanceID, applyTaskID := env.start(t, "opsProc", "BIZ-COVER-BACK")

	// runtimeVariableSource 直接透传引擎读变量：存在返回值、缺失报不存在
	source := runtimeVariableSource{runtime: env.engine.RuntimeService()}
	value, err := source.GetVariable(ctx, instanceID, VarInitiator)
	if err != nil {
		t.Fatalf("读取发起人变量失败: %v", err)
	}
	if text, ok := value.(string); !ok || text != "u1" {
		t.Fatalf("发起人变量不符: %+v", value)
	}
	if _, err := source.GetVariable(ctx, instanceID, "missingVar"); err == nil {
		t.Fatalf("缺失变量应报错")
	}

	// 适配器 nil 守卫：退回服务未装配、请求为空
	facadeCoverErrCode(t, (backExecutorAdapter{}).BackToStepTask(ctx, &api.BackTaskVo{TaskID: "t"}), "WORKFLOW-0601")
	adapter := backExecutorAdapter{back: env.service.comp.back}
	facadeCoverErrCode(t, adapter.BackToStepTask(ctx, nil), "WORKFLOW-0601")

	// 推进到 handle 后可退节点应包含已完成的 apply
	if _, err := env.service.Complete(ctx, &CompleteTaskVo{BaseProcessVo: BaseProcessVo{
		TaskID: applyTaskID, UserCode: "u1", Message: "同意", ProcessInstanceID: instanceID, Type: CommentTypeSP,
	}}); err != nil {
		t.Fatalf("办结 apply 失败: %v", err)
	}
	handleTask := facadeCoverCurrentTask(t, env.engine, instanceID)
	if handleTask.TaskDefinitionKey != "handle" {
		t.Fatalf("当前任务应为 handle，实际 %s", handleTask.TaskDefinitionKey)
	}
	backR, err := env.service.GetBackNodes(ctx, handleTask.ID)
	if err != nil || !backR.Success {
		t.Fatalf("可退节点查询失败: r=%+v err=%v", backR, err)
	}
	nodes, ok := backR.Data.([]BackNodeView)
	if !ok || len(nodes) == 0 {
		t.Fatalf("可退节点不应为空: %+v", backR.Data)
	}
	found := false
	for _, node := range nodes {
		if node.ActivityID == "apply" {
			found = true
		}
	}
	if !found {
		t.Fatalf("可退节点应包含 apply: %+v", nodes)
	}

	// 适配器正常分支：调度器退回型自动办结回调退回到 apply
	if err := adapter.BackToStepTask(ctx, &api.BackTaskVo{
		TaskID: handleTask.ID, TargetActivityID: "apply", UserCode: "u1", Message: "退回修改",
	}); err != nil {
		t.Fatalf("适配器退回失败: %v", err)
	}
	if got := facadeCoverCurrentTask(t, env.engine, instanceID); got.TaskDefinitionKey != "apply" {
		t.Fatalf("适配器退回后应回到 apply，实际 %s", got.TaskDefinitionKey)
	}

	// 门面退回：目标为空时走上一节点语义
	instance2, apply2 := env.start(t, "opsProc", "BIZ-COVER-BACK-2")
	if _, err := env.service.Complete(ctx, &CompleteTaskVo{BaseProcessVo: BaseProcessVo{
		TaskID: apply2, UserCode: "u1", Message: "同意", ProcessInstanceID: instance2, Type: CommentTypeSP,
	}}); err != nil {
		t.Fatalf("第二个实例办结失败: %v", err)
	}
	handle2 := facadeCoverCurrentTask(t, env.engine, instance2)
	previousR, err := env.service.BackToStepTask(ctx, &BackTaskVo{
		TaskID: handle2.ID, UserCode: "u1", Message: "退回上一节点",
	})
	if err != nil || !previousR.Success {
		t.Fatalf("空目标退回失败: r=%+v err=%v", previousR, err)
	}
	if got := facadeCoverCurrentTask(t, env.engine, instance2); got.TaskDefinitionKey != "apply" {
		t.Fatalf("空目标退回后应回到 apply，实际 %s", got.TaskDefinitionKey)
	}

	// 门面退回：显式目标节点
	instance3, apply3 := env.start(t, "opsProc", "BIZ-COVER-BACK-3")
	if _, err := env.service.Complete(ctx, &CompleteTaskVo{BaseProcessVo: BaseProcessVo{
		TaskID: apply3, UserCode: "u1", Message: "同意", ProcessInstanceID: instance3, Type: CommentTypeSP,
	}}); err != nil {
		t.Fatalf("第三个实例办结失败: %v", err)
	}
	handle3 := facadeCoverCurrentTask(t, env.engine, instance3)
	targetR, err := env.service.BackToStepTask(ctx, &BackTaskVo{
		TaskID: handle3.ID, TargetActivityID: "apply", UserCode: "u1", Message: "退回指定节点",
	})
	if err != nil || !targetR.Success {
		t.Fatalf("指定目标退回失败: r=%+v err=%v", targetR, err)
	}
	if got := facadeCoverCurrentTask(t, env.engine, instance3); got.TaskDefinitionKey != "apply" {
		t.Fatalf("指定目标退回后应回到 apply，实际 %s", got.TaskDefinitionKey)
	}

	// 空请求拒绝
	facadeCoverErrCode(t, func() error {
		_, err := env.service.BackToStepTask(ctx, nil)
		return err
	}(), "WORKFLOW-0400")
}

// TestFacadeCoverMultiInstanceMembers 覆盖多实例会签成员动态增删门面的正常与校验分支。
// @param t 测试上下文
func TestFacadeCoverMultiInstanceMembers(t *testing.T) {
	// 复用会签集合语料（executorList + 集合配置监听器），门面只做引擎变量维护
	cs := newCountersignFixture(t)
	service := NewWorkflowService(cs.engine, WithClock(clock.NewFake(facadeCoverTime)))
	ctx := tenant.With(context.Background(), "dev")
	instance, _ := cs.startCountersign(t, "csCollection", "alice", nil)

	// 初始成员来自模型 executorList（alice、bob；重复 alice 已去重）
	list := facadeCoverExecutorList(t, cs.engine, instance.ID)
	if len(list) != 2 {
		t.Fatalf("初始会签成员应为 2 人: %v", list)
	}
	// 动态加签
	if _, err := service.AddMultiInstanceMember(ctx, instance.ID, "csTask", "carol"); err != nil {
		t.Fatalf("动态加签失败: %v", err)
	}
	list = facadeCoverExecutorList(t, cs.engine, instance.ID)
	if len(list) != 3 || list[2] != "carol" {
		t.Fatalf("动态加签后成员不符: %v", list)
	}
	// 重复加签幂等（不重复追加）
	if _, err := service.AddMultiInstanceMember(ctx, instance.ID, "csTask", "carol"); err != nil {
		t.Fatalf("重复加签失败: %v", err)
	}
	if got := facadeCoverExecutorList(t, cs.engine, instance.ID); len(got) != 3 {
		t.Fatalf("重复加签应幂等: %v", got)
	}
	// 动态减签
	if _, err := service.RemoveMultiInstanceMember(ctx, instance.ID, "csTask", "carol", "", false); err != nil {
		t.Fatalf("动态减签失败: %v", err)
	}
	if got := facadeCoverExecutorList(t, cs.engine, instance.ID); len(got) != 2 {
		t.Fatalf("动态减签后成员不符: %v", got)
	}
	// 参数校验
	_, err := service.AddMultiInstanceMember(ctx, instance.ID, "csTask", " ")
	facadeCoverErrCode(t, err, "WORKFLOW-2220")
	_, err = service.RemoveMultiInstanceMember(ctx, "", "csTask", "carol", "", false)
	facadeCoverErrCode(t, err, "WORKFLOW-2220")
}

// TestFacadeCoverDelegateAndRemind 覆盖委托规则编辑/启用/注销与催办配置更新门面。
// @param t 测试上下文
func TestFacadeCoverDelegateAndRemind(t *testing.T) {
	env := newOpsService(t)
	ctx := tenant.With(context.Background(), "bizA")

	// 委托规则：新增 → 编辑 → 启用 → 注销
	rule := &DelegateRule{
		OwnerID: "u1", SourceCode: "bizA",
		VaildFrom: facadeCoverTime, VaildTo: facadeCoverTime.Add(24 * time.Hour),
		ConsignType: ConsignTypeAll, Consigners: []DelegateConsigner{{ConsignerID: "u2"}},
	}
	if _, err := env.service.AddDelegateTask(ctx, rule); err != nil {
		t.Fatalf("新增委托失败: %v", err)
	}
	if rule.ID == "" {
		t.Fatalf("新增委托未回填规则 Id")
	}
	editedR, err := env.service.EditDelegateTask(ctx, &DelegateRule{ID: rule.ID, Remark: "更新备注"})
	if err != nil || !editedR.Success {
		t.Fatalf("编辑委托失败: r=%+v err=%v", editedR, err)
	}
	listedR, err := env.service.ListDelegateTask(ctx, "u1", "bizA")
	if err != nil {
		t.Fatalf("查询委托失败: %v", err)
	}
	rules := listedR.Data.([]*DelegateRule)
	if len(rules) != 1 || rules[0].Remark != "更新备注" {
		t.Fatalf("编辑未生效: %+v", rules)
	}
	if _, err := env.service.EnableDelegateTask(ctx, rule.ID); err != nil {
		t.Fatalf("启用委托失败: %v", err)
	}
	if _, err := env.service.RetireDelegateTask(ctx, rule.ID); err != nil {
		t.Fatalf("注销委托失败: %v", err)
	}
	listedR, err = env.service.ListDelegateTask(ctx, "u1", "bizA")
	if err != nil {
		t.Fatalf("注销后查询委托失败: %v", err)
	}
	rules = listedR.Data.([]*DelegateRule)
	if len(rules) != 1 || rules[0].Enable != DelegateDisabled || rules[0].Retire != DelegateRetired {
		t.Fatalf("注销后规则状态不符: %+v", rules)
	}
	// 更新不存在的规则
	_, err = env.service.EditDelegateTask(ctx, &DelegateRule{ID: "ghost-rule"})
	facadeCoverErrCode(t, err, "WORKFLOW-1704")

	// 催办配置：新增 → 更新（步骤白名单 + 对象列表）→ 回查
	savedR, err := env.service.AddRemind(ctx, RemindSaveRequest{Remind: &Remind{TenantID: "bizA", PID: "P-COVER"}})
	if err != nil {
		t.Fatalf("新增催办失败: %v", err)
	}
	saved := savedR.Data.(*Remind)
	updatedR, err := env.service.EditRemind(ctx, RemindSaveRequest{
		Remind: &Remind{ID: saved.ID, StepIDs: "step-a,step-b", EnablePre: 1, EnablePost: 1},
		Targets: []*RemindTarget{{
			ExecutorID: "u1", Name: "张三", TargetType: RemindTargetTypeUser, Type: RemindTypeAhead, Enable: 1,
		}},
		ReplaceStepIDs: true,
	})
	if err != nil || !updatedR.Success {
		t.Fatalf("更新催办失败: r=%+v err=%v", updatedR, err)
	}
	updated := updatedR.Data.(*Remind)
	if updated.StepIDs != "step-a,step-b" || len(updated.Targets) != 1 {
		t.Fatalf("催办更新结果不符: %+v", updated)
	}
	gotR, err := env.service.GetRemind(ctx, saved.ID)
	if err != nil {
		t.Fatalf("回查催办失败: %v", err)
	}
	got := gotR.Data.(*Remind)
	if got.StepIDs != "step-a,step-b" || len(got.Targets) != 1 {
		t.Fatalf("催办回查结果不符: %+v", got)
	}
	// 更新不存在与空请求
	_, err = env.service.EditRemind(ctx, RemindSaveRequest{Remind: &Remind{ID: "ghost-remind"}})
	facadeCoverErrCode(t, err, "WORKFLOW-1803")
	_, err = env.service.EditRemind(ctx, RemindSaveRequest{})
	facadeCoverErrCode(t, err, "WORKFLOW-1801")
}

// TestFacadeCoverTaskExceptions 覆盖任务异常列表与批量处理门面的正常与空集合分支。
// @param t 测试上下文
func TestFacadeCoverTaskExceptions(t *testing.T) {
	env := newOpsService(t)
	ctx := tenant.With(context.Background(), "bizA")
	instanceID, taskID := env.start(t, "opsProc", "BIZ-COVER-EX")

	// 播种两条未处理异常
	ids := make([]string, 0, 2)
	for i := 0; i < 2; i++ {
		record, err := env.service.comp.exception.Record(ctx, &ExceptionRequest{
			TaskID: taskID, ProcessInstanceID: instanceID, Message: "回调失败",
			ExceptionType: ExceptionTypeRuntime, TenantID: "bizA",
		})
		if err != nil {
			t.Fatalf("记录异常失败: %v", err)
		}
		ids = append(ids, record.ID)
	}
	listedR, err := env.service.ListTaskExceptions(ctx, instanceID)
	if err != nil {
		t.Fatalf("查询异常失败: %v", err)
	}
	if got := len(listedR.Data.([]*TaskException)); got != 2 {
		t.Fatalf("异常数量不符: %d", got)
	}
	// 批量置为已处理
	if _, err := env.service.HandleTaskExceptions(ctx, ids, true); err != nil {
		t.Fatalf("批量处理异常失败: %v", err)
	}
	listedR, err = env.service.ListTaskExceptions(ctx, instanceID)
	if err != nil {
		t.Fatalf("处理后再查异常失败: %v", err)
	}
	for _, item := range listedR.Data.([]*TaskException) {
		if item.HandleStatus != ExceptionHandled {
			t.Fatalf("异常未置为已处理: %+v", item)
		}
	}
	// 空集合为无操作
	if _, err := env.service.HandleTaskExceptions(ctx, nil, true); err != nil {
		t.Fatalf("空集合处理应无操作: %v", err)
	}
	// 无异常实例返回空列表
	emptyR, err := env.service.ListTaskExceptions(ctx, "ghost-instance")
	if err != nil || len(emptyR.Data.([]*TaskException)) != 0 {
		t.Fatalf("无异常实例应为空列表: %+v err=%v", emptyR.Data, err)
	}
}

// TestFacadeCoverExtraGuards 覆盖收尾能力门面的参数校验、引擎失败、分页边界、身份树与挂起按钮。
// @param t 测试上下文
func TestFacadeCoverExtraGuards(t *testing.T) {
	env := newOpsService(t)
	ctx := tenant.With(context.Background(), "bizA")
	instanceID, applyTaskID := env.start(t, "opsProc", "BIZ-COVER-EXTRA")

	// 审阅绑定参数校验
	_, err := env.service.SaveActExtend(ctx, nil)
	facadeCoverErrCode(t, err, "WORKFLOW-0400")
	_, err = env.service.ListActExtends(ctx, "", "")
	facadeCoverErrCode(t, err, "WORKFLOW-0400")

	// 会签权重参数校验与定义缺失
	_, err = env.service.SaveCountersignWeights(ctx, "", "apply", "bizA", nil)
	facadeCoverErrCode(t, err, "WORKFLOW-0400")
	_, err = env.service.SaveCountersignWeights(ctx, "opsProc", "", "bizA", nil)
	facadeCoverErrCode(t, err, "WORKFLOW-0400")
	_, err = env.service.ListCountersignWeights(ctx, "opsProc", "", "bizA")
	facadeCoverErrCode(t, err, "WORKFLOW-0400")
	if _, err := env.service.ListCountersignWeights(ctx, "ghostProc", "apply", "bizA"); err == nil {
		t.Fatalf("未部署定义的权重查询应报错")
	}

	// 签收/处理委派：参数校验与引擎失败
	_, err = env.service.ClaimTask(ctx, "", "u1")
	facadeCoverErrCode(t, err, "WORKFLOW-0400")
	if _, err := env.service.ClaimTask(ctx, "ghost-task", "u1"); err == nil {
		t.Fatalf("不存在任务的签收应报错")
	}
	_, err = env.service.ResolveTask(ctx, "", "u1")
	facadeCoverErrCode(t, err, "WORKFLOW-0400")
	if _, err := env.service.ResolveTask(ctx, applyTaskID, "u1"); err == nil {
		t.Fatalf("未处于委派状态的任务处理应报错")
	}

	// 批量挂起/激活：空 Id 跳过、不存在实例报错
	skippedR, err := env.service.SuspendProcessInstances(ctx, []string{""}, "u1")
	if err != nil || skippedR.Data.(map[string]any)["suspended"] != 0 {
		t.Fatalf("空 Id 批量挂起计数不符: %+v err=%v", skippedR.Data, err)
	}
	skippedR, err = env.service.ActivateProcessInstances(ctx, []string{""}, "u1")
	if err != nil || skippedR.Data.(map[string]any)["activated"] != 0 {
		t.Fatalf("空 Id 批量激活计数不符: %+v err=%v", skippedR.Data, err)
	}
	if _, err := env.service.SuspendProcessInstances(ctx, []string{"ghost-instance"}, "u1"); err == nil {
		t.Fatalf("不存在实例的挂起应报错")
	}
	if _, err := env.service.ActivateProcessInstances(ctx, []string{"ghost-instance"}, "u1"); err == nil {
		t.Fatalf("不存在实例的激活应报错")
	}

	// 实例分页边界：size<=0 取全量、start 越界夹到末尾
	pageR, err := env.service.GetProcessInstancePageList(ctx, PageQuery{Start: 0, Size: 0})
	if err != nil {
		t.Fatalf("实例分页查询失败: %v", err)
	}
	page := pageR.Data.(PageResult[api.ProcessInstance])
	if page.Total != 1 || len(page.Data) != 1 || page.Size != 1 {
		t.Fatalf("size<=0 分页不符: %+v", page)
	}
	pageR, err = env.service.GetProcessInstancePageList(ctx, PageQuery{Start: -5, Size: 1})
	if err != nil {
		t.Fatalf("负偏移分页查询失败: %v", err)
	}
	page = pageR.Data.(PageResult[api.ProcessInstance])
	if page.Start != 1 || len(page.Data) != 0 {
		t.Fatalf("负偏移分页不符: %+v", page)
	}
	pageR, err = env.service.GetProcessInstancePageList(ctx, PageQuery{Start: 99, Size: 1})
	if err != nil {
		t.Fatalf("越界偏移分页查询失败: %v", err)
	}
	page = pageR.Data.(PageResult[api.ProcessInstance])
	if page.Start != 1 || len(page.Data) != 0 {
		t.Fatalf("越界偏移分页不符: %+v", page)
	}

	// 身份树：组节点下挂成员，未分组用户挂根
	if _, err := env.engine.IdentityService().CreateGroup(ctx, api.Group{ID: "grp-cover", Name: "覆盖组"}); err != nil {
		t.Fatalf("创建组失败: %v", err)
	}
	if err := env.engine.IdentityService().AddMembership(ctx, "u1", "grp-cover"); err != nil {
		t.Fatalf("添加组成员失败: %v", err)
	}
	treeR, err := env.service.GetIdentityTree(ctx)
	if err != nil {
		t.Fatalf("身份树查询失败: %v", err)
	}
	nodes := treeR.Data.([]map[string]any)
	// 组节点 1 个 + 未分组节点 1 个（u1 已入组，只剩 u2）
	if len(nodes) != 2 {
		t.Fatalf("身份树节点数不符: %+v", nodes)
	}
	groupChildren := 0
	ungroupedChildren := 0
	for _, node := range nodes {
		children, _ := node["children"].([]map[string]any)
		switch node["id"] {
		case "grp-cover":
			groupChildren = len(children)
			if groupChildren != 1 || children[0]["id"] != "u1" {
				t.Fatalf("组成员不符: %+v", children)
			}
		case "ungrouped":
			ungroupedChildren = len(children)
			if ungroupedChildren != 1 || children[0]["id"] != "u2" {
				t.Fatalf("未分组用户不符: %+v", children)
			}
		}
	}
	if groupChildren != 1 || ungroupedChildren != 1 {
		t.Fatalf("身份树分组投影不符: %+v", nodes)
	}

	// 挂起态操作按钮应给出激活入口
	if _, err := env.service.DoSuspendProcessInstance(ctx, instanceID, "u1"); err != nil {
		t.Fatalf("挂起实例失败: %v", err)
	}
	buttonsR, err := env.service.GetOperationButtons(ctx, instanceID)
	if err != nil {
		t.Fatalf("挂起态按钮查询失败: %v", err)
	}
	buttons := buttonsR.Data.([]string)
	if len(buttons) == 0 || buttons[0] != "activate" {
		t.Fatalf("挂起态按钮不符: %v", buttons)
	}
	if _, err := env.service.DoActivateProcessInstance(ctx, instanceID, "u1"); err != nil {
		t.Fatalf("激活实例失败: %v", err)
	}
	// 运行态查不到实例按已结束处理（给出重激活入口）
	endedR, err := env.service.GetOperationButtons(ctx, "ghost-instance")
	if err != nil || endedR.Data.([]string)[0] != "reactivate" {
		t.Fatalf("已结束实例按钮不符: %+v err=%v", endedR.Data, err)
	}
}

// TestFacadeCoverDesignerBranches 覆盖设计器域名门的参数校验、定位失败与复制/回填错误分支。
// @param t 测试上下文
func TestFacadeCoverDesignerBranches(t *testing.T) {
	env := newOpsService(t)
	ctx := tenant.With(context.Background(), "bizA")

	// 用户任务节点元数据：opsProc 三个用户任务
	definition, err := env.engine.RepositoryService().GetProcessDefinitionByKey(ctx, "opsProc")
	if err != nil {
		t.Fatalf("查询流程定义失败: %v", err)
	}
	metasR, err := env.service.GetUserTaskMetas(ctx, definition.ID)
	if err != nil {
		t.Fatalf("查询节点元数据失败: %v", err)
	}
	metas := metasR.Data.([]*UserTaskMeta)
	if len(metas) != 3 {
		t.Fatalf("节点元数据数量不符: %+v", metas)
	}
	seen := make(map[string]bool, len(metas))
	for _, meta := range metas {
		seen[meta.ActivityID] = true
	}
	for _, activityID := range []string{"apply", "handle", "approve"} {
		if !seen[activityID] {
			t.Fatalf("节点元数据缺少 %s: %+v", activityID, metas)
		}
	}
	_, err = env.service.GetUserTaskMetas(ctx, " ")
	facadeCoverErrCode(t, err, "WORKFLOW-0400")
	if _, err := env.service.GetUserTaskMetas(ctx, "ghost-definition"); err == nil {
		t.Fatalf("不存在定义的节点元数据应报错")
	}

	// 建模并读回编辑源
	createdR, err := env.service.CreateProcessModel(ctx, "bizA", "覆盖模型", "coverProc", opsFixture)
	if err != nil {
		t.Fatalf("建模失败: %v", err)
	}
	created := createdR.Data.(CreatedProcessModel)
	xmlR, err := env.service.GetModelXML(ctx, created.BusinessID)
	if err != nil {
		t.Fatalf("读取模型源失败: %v", err)
	}
	if got := xmlR.Data.(map[string]any)["xml"].(string); got != opsFixture {
		t.Fatalf("模型源内容不符")
	}

	// 未绑定引擎模型的业务模型不能读源
	unboundR, err := env.service.AddModelCategory(ctx, &ModelCategory{ModelName: "未绑定", ModelCode: "unboundProc", TenantID: "bizA"})
	if err != nil {
		t.Fatalf("新增未绑定模型失败: %v", err)
	}
	unbound := unboundR.Data.(*ModelCategory)
	_, err = env.service.GetModelXML(ctx, unbound.ID)
	facadeCoverErrCode(t, err, "WORKFLOW-0400")

	// 模型定位与保存校验
	_, err = env.service.GetModelXML(ctx, "")
	facadeCoverErrCode(t, err, "WORKFLOW-0400")
	_, err = env.service.GetModelXML(ctx, "ghost-model")
	facadeCoverErrCode(t, err, "WORKFLOW-1404")
	_, err = env.service.SaveModelXML(ctx, created.BusinessID, "  ")
	facadeCoverErrCode(t, err, "WORKFLOW-0400")
	_, err = env.service.SaveModelXML(ctx, "ghost-model", "<definitions/>")
	facadeCoverErrCode(t, err, "WORKFLOW-1404")

	// 历史定义 XML 与回填
	_, err = env.service.GetDefinitionXML(ctx, "")
	facadeCoverErrCode(t, err, "WORKFLOW-0400")
	if _, err := env.service.GetDefinitionXML(ctx, "ghost-definition"); err == nil {
		t.Fatalf("不存在定义的 XML 应报错")
	}
	_, err = env.service.UpdateModelFromDefinition(ctx, "bizA", definition.ID)
	facadeCoverErrCode(t, err, "WORKFLOW-1404")
	_, err = env.service.UpdateModelFromDefinition(ctx, "bizA", "")
	facadeCoverErrCode(t, err, "WORKFLOW-0400")

	// 复制流程：源与目标缺失
	_, err = env.service.CopyProcess(ctx, "ghost-model", []string{created.BusinessID})
	facadeCoverErrCode(t, err, "WORKFLOW-1404")
	_, err = env.service.CopyProcess(ctx, created.BusinessID, []string{"ghost-target"})
	facadeCoverErrCode(t, err, "WORKFLOW-1404")

	// 模型步骤联查租户为空
	if _, err := env.service.GetModelAndStepPage(ctx, ""); err == nil {
		t.Fatalf("空租户联查应报错")
	}

	// 步骤详情：空主键拒绝、未命中返回空数据
	_, err = env.service.GetStepDesign(ctx, "")
	facadeCoverErrCode(t, err, "WORKFLOW-0400")
	stepR, err := env.service.GetStepDesign(ctx, "ghost-step")
	if err != nil || stepR.Data != nil {
		t.Fatalf("未命中步骤应返回空数据: %+v err=%v", stepR.Data, err)
	}

	// 引擎委托：参数校验与任务不存在
	_, err = env.service.DelegateTask(ctx, "", "u2")
	facadeCoverErrCode(t, err, "WORKFLOW-0400")
	if _, err := env.service.DelegateTask(ctx, "ghost-task", "u2"); err == nil {
		t.Fatalf("不存在任务的委派应报错")
	}

	// 权重删除参数校验
	_, err = env.service.DeleteCountersignWeights(ctx, "", "apply", "bizA")
	facadeCoverErrCode(t, err, "WORKFLOW-0400")
	_, err = env.service.DeleteCountersignWeights(ctx, "opsProc", "", "bizA")
	facadeCoverErrCode(t, err, "WORKFLOW-0400")

	// 建模参数校验
	_, err = env.service.CreateProcessModel(ctx, "", "", "", "")
	facadeCoverErrCode(t, err, "WORKFLOW-0400")
}

// TestFacadeCoverMgmtBranches 覆盖审阅影子任务、流程图高亮、步骤/模板/模型管理与部署错误分支。
// @param t 测试上下文
func TestFacadeCoverMgmtBranches(t *testing.T) {
	env := newOpsService(t)
	ctx := tenant.With(context.Background(), "bizA")
	_, applyTaskID := env.start(t, "opsProc", "BIZ-COVER-MGMT")

	// 审阅绑定 → 同步影子任务 → 单条查询
	if _, err := env.service.SaveActExtend(ctx, &ActExtend{
		BaseColumns: BaseColumns{TenantID: "bizA"}, ActivityID: "apply", ActivityName: "申请",
		ProcessDefinitionKey: "opsProc", OwnerType: ActOwnerTypePerson, OwnerValue: "u2",
	}); err != nil {
		t.Fatalf("保存审阅绑定失败: %v", err)
	}
	task, err := env.engine.TaskService().GetTask(ctx, applyTaskID)
	if err != nil {
		t.Fatalf("查询任务失败: %v", err)
	}
	syncedR, err := env.service.SyncReviewForTask(ctx, &task, "u1", "张三")
	if err != nil {
		t.Fatalf("同步审阅影子任务失败: %v", err)
	}
	created := syncedR.Data.([]*ReviewTask)
	if len(created) != 1 {
		t.Fatalf("影子任务数量不符: %+v", created)
	}
	reviewR, err := env.service.GetReviewTask(ctx, created[0].ID)
	if err != nil {
		t.Fatalf("查询影子任务失败: %v", err)
	}
	review, ok := reviewR.Data.(*ReviewTask)
	if !ok || review.Reviewer != "u2" || review.Status != ReviewStatusPending {
		t.Fatalf("影子任务内容不符: %+v", reviewR.Data)
	}
	// 未命中返回空数据而非错误
	missingR, err := env.service.GetReviewTask(ctx, "ghost-review")
	if err != nil || missingR.Data != nil {
		t.Fatalf("未命中影子任务应返回空数据: %+v err=%v", missingR.Data, err)
	}
	// 空任务同步拒绝
	if _, err := env.service.SyncReviewForTask(ctx, nil, "u1", "张三"); err == nil {
		t.Fatalf("空任务同步应报错")
	}
	// 无命中审阅人/实例返回空集合
	emptyReviewerR, err := env.service.ListReviewTasksByReviewer(ctx, "nobody", "bizA", PageQuery{})
	if err != nil || emptyReviewerR.Data.(PageResult[*ReviewTask]).Total != 0 {
		t.Fatalf("无命中审阅人列表不符: %+v err=%v", emptyReviewerR.Data, err)
	}
	emptyInstR, err := env.service.ListReviewTasksByInstance(ctx, "ghost-instance")
	if err != nil || len(emptyInstR.Data.([]*ReviewTask)) != 0 {
		t.Fatalf("无命中实例列表不符: %+v err=%v", emptyInstR.Data, err)
	}
	// 未读计数：待审阅 1 → 办结后 0
	unreadR, err := env.service.CountUnreadReviews(ctx, "u2", "bizA")
	if err != nil || unreadR.Data.(int64) != 1 {
		t.Fatalf("未读计数不符: %+v err=%v", unreadR.Data, err)
	}
	doneR, err := env.service.CompleteReviewTask(ctx, created[0].ID, "已阅", "u2")
	if err != nil || doneR.Data.(*ReviewTask).Status != ReviewStatusDone {
		t.Fatalf("办结影子任务失败: %+v err=%v", doneR.Data, err)
	}
	afterR, err := env.service.CountUnreadReviews(ctx, "u2", "bizA")
	if err != nil || afterR.Data.(int64) != 0 {
		t.Fatalf("办结后未读计数不符: %+v err=%v", afterR.Data, err)
	}
	if _, err := env.service.CompleteReviewTask(ctx, "ghost-review", "已阅", "u2"); err == nil {
		t.Fatalf("不存在的影子任务办结应报错")
	}

	// 流程图高亮：空实例/空任务/不存在实例均报错
	if _, err := env.service.GetFlowHighlight(ctx, ""); err == nil {
		t.Fatalf("空实例高亮应报错")
	}
	if _, err := env.service.GetFlowHighlight(ctx, "ghost-instance"); err == nil {
		t.Fatalf("不存在实例高亮应报错")
	}
	if _, err := env.service.GetFlowHighlightByTask(ctx, ""); err == nil {
		t.Fatalf("空任务高亮应报错")
	}
	if _, err := env.service.GetFlowHighlightByTask(ctx, "ghost-task"); err == nil {
		t.Fatalf("不存在任务高亮应报错")
	}

	// 设计时步骤：空请求/缺模型/主键不存在/空模型列表
	if _, err := env.service.AddStepDesign(ctx, nil); err == nil {
		t.Fatalf("空步骤新增应报错")
	}
	if _, err := env.service.AddStepDesign(ctx, &StepDesign{
		BaseColumns: BaseColumns{TenantID: "bizA"}, Name: "缺代号",
	}); err == nil {
		t.Fatalf("缺模型代号的步骤新增应报错")
	}
	if _, err := env.service.EditStepDesign(ctx, nil); err == nil {
		t.Fatalf("空步骤编辑应报错")
	}
	if _, err := env.service.EditStepDesign(ctx, &StepDesign{
		BaseColumns: BaseColumns{ID: "ghost-step", TenantID: "bizA"}, StepCode: "apply", ModelID: "M-1",
	}); err == nil {
		t.Fatalf("不存在步骤的编辑应报错")
	}
	if _, err := env.service.ListStepDesignByModel(ctx, ""); err == nil {
		t.Fatalf("空模型步骤列表应报错")
	}
	if _, err := env.service.DeleteStepDesign(ctx, nil); err == nil {
		t.Fatalf("空集合步骤删除应报错")
	}

	// 设计器模板：空请求/缺名称/主键不存在/空租户列表
	if _, err := env.service.AddTemplateDef(ctx, nil); err == nil {
		t.Fatalf("空模板新增应报错")
	}
	if _, err := env.service.AddTemplateDef(ctx, &TemplateDef{
		BaseColumns: BaseColumns{TenantID: "bizA"},
	}); err == nil {
		t.Fatalf("缺名称模板新增应报错")
	}
	if _, err := env.service.EditTemplateDef(ctx, nil); err == nil {
		t.Fatalf("空模板编辑应报错")
	}
	if _, err := env.service.EditTemplateDef(ctx, &TemplateDef{
		BaseColumns: BaseColumns{ID: "ghost-template", TenantID: "bizA"}, Name: "模板",
	}); err == nil {
		t.Fatalf("不存在模板的编辑应报错")
	}
	if _, err := env.service.ListTemplateDefs(ctx, ""); err == nil {
		t.Fatalf("空租户模板列表应报错")
	}
	if _, err := env.service.DeleteTemplateDef(ctx, nil); err == nil {
		t.Fatalf("空集合模板删除应报错")
	}

	// 模型/分类：空请求/缺代号/不存在/空租户
	if _, err := env.service.AddModelCategory(ctx, nil); err == nil {
		t.Fatalf("空模型新增应报错")
	}
	if _, err := env.service.AddModelCategory(ctx, &ModelCategory{ModelName: "缺代号", TenantID: "bizA"}); err == nil {
		t.Fatalf("缺代号模型新增应报错")
	}
	if _, err := env.service.EditModelCategory(ctx, nil); err == nil {
		t.Fatalf("空模型编辑应报错")
	}
	if _, err := env.service.EditModelCategory(ctx, &ModelCategory{
		BaseColumns: BaseColumns{ID: "ghost-model", TenantID: "bizA"}, ModelCode: "ghost",
	}); err == nil {
		t.Fatalf("不存在模型的编辑应报错")
	}
	if _, err := env.service.DeleteModelCategory(ctx, nil); err == nil {
		t.Fatalf("空集合模型删除应报错")
	}
	if _, err := env.service.ListModelCategory(ctx, ""); err == nil {
		t.Fatalf("空租户模型列表应报错")
	}
	if _, err := env.service.GetModelCategoryTree(ctx, ""); err == nil {
		t.Fatalf("空租户模型树应报错")
	}
	byIDR, err := env.service.GetModelCategoryByID(ctx, "ghost-model")
	if err != nil || byIDR.Data != nil {
		t.Fatalf("未命中模型应返回空数据: %+v err=%v", byIDR.Data, err)
	}
	if _, err := env.service.GetModelCategoryByCode(ctx, "", "ghost"); err == nil {
		t.Fatalf("空租户按代号查询应报错")
	}
	byCodeR, err := env.service.GetModelCategoryByCode(ctx, "bizA", "ghost-code")
	if err != nil || byCodeR.Data != nil {
		t.Fatalf("未命中代号应返回空数据: %+v err=%v", byCodeR.Data, err)
	}

	// 部署：模型不存在/未绑定引擎模型/非法 XML 源
	_, err = env.service.DeployModel(ctx, "bizA", "ghost-model")
	facadeCoverErrCode(t, err, "WORKFLOW-1404")
	unboundR, err := env.service.AddModelCategory(ctx, &ModelCategory{
		ModelName: "未绑定部署", ModelCode: "unboundDeploy", TenantID: "bizA",
	})
	if err != nil {
		t.Fatalf("新增未绑定模型失败: %v", err)
	}
	unbound := unboundR.Data.(*ModelCategory)
	_, err = env.service.DeployModel(ctx, "bizA", unbound.ID)
	facadeCoverErrCode(t, err, "WORKFLOW-0400")
	badR, err := env.service.CreateProcessModel(ctx, "bizA", "坏源模型", "badProc", "not-a-bpmn")
	if err != nil {
		t.Fatalf("建模坏源失败: %v", err)
	}
	bad := badR.Data.(CreatedProcessModel)
	if _, err := env.service.DeployModel(ctx, "bizA", bad.BusinessID); err == nil {
		t.Fatalf("非法 XML 源部署应报错")
	}
	// 快照查询无部署记录返回空列表
	hisR, err := env.service.ListHisSteps(ctx, "ghost-deployment")
	if err != nil || len(hisR.Data.([]*StepRule)) != 0 {
		t.Fatalf("无部署快照应为空列表: %+v err=%v", hisR.Data, err)
	}
}

// TestFacadeCoverAssemblyAndServiceExtras 覆盖服务装配（SQL/内存）、未装配入口、桩函数与历史查询。
// @param t 测试上下文
func TestFacadeCoverAssemblyAndServiceExtras(t *testing.T) {
	env := newOpsService(t)
	ctx := tenant.With(context.Background(), "bizA")

	// 未装配组件的协议兼容场景：注册/预热入口为空操作、桩函数返回未实现
	nilService := NewWorkflowService(nil)
	if err := nilService.RegisterCountersignRuntime(ctx); err != nil {
		t.Fatalf("无引擎会签注册应为空操作: %v", err)
	}
	if err := nilService.RegisterBusinessCallbackHook(ctx); err != nil {
		t.Fatalf("无引擎回调注册应为空操作: %v", err)
	}
	if err := nilService.RewarmCountersignDefinition(ctx, ""); err != nil {
		t.Fatalf("空定义会签预热应为空操作: %v", err)
	}
	if err := nilService.RewarmExecutorRewriteDefinition(ctx, ""); err != nil {
		t.Fatalf("空定义执行人预热应为空操作: %v", err)
	}
	if err := nilService.RewarmBusinessCallbackDefinition(ctx, ""); err != nil {
		t.Fatalf("空定义回调预热应为空操作: %v", err)
	}
	nilService.logWarn("门面覆盖测试告警", "case", "nil-engine")
	stubR, err := stub()
	if err == nil || stubR.Success {
		t.Fatalf("桩函数应返回未实现失败: r=%+v err=%v", stubR, err)
	}
	facadeCoverErrCode(t, err, "WORKFLOW-0501")
	_, err = nilService.StartProcessAndFirstStep(ctx, nil)
	facadeCoverErrCode(t, err, "WORKFLOW-0400")

	// SQL 组件装配：mysql 驱动懒连接，构造期不访问网络
	db, err := sql.Open("mysql", "cover:cover@tcp(127.0.0.1:1)/cover")
	if err != nil {
		t.Fatalf("打开 SQL 连接句柄失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store, err := pwf.NewStore(db, "mysql")
	if err != nil {
		t.Fatalf("创建 workflow 仓储入口失败: %v", err)
	}
	var sqlOpts Options
	WithSQLStores(store, db)(&sqlOpts)
	if sqlOpts.sqlStore != store || sqlOpts.sqlDB != db {
		t.Fatalf("WithSQLStores 未写入仓储入口")
	}
	var nilOpts Options
	WithSQLStores(nil, db)(&nilOpts)
	if nilOpts.sqlStore != nil || nilOpts.sqlDB != nil {
		t.Fatalf("空仓储入口不应覆盖配置")
	}

	// 完整 SQL 装配：能力子服务全部来自 SQL 适配器族
	checked, err := NewWorkflowServiceChecked(env.engine, WithSQLStores(store, db), WithClock(clock.NewFake(facadeCoverTime)))
	if err != nil {
		t.Fatalf("SQL 装配失败: %v", err)
	}
	if checked.comp == nil || checked.comp.meta == nil || checked.comp.modelMgmt == nil ||
		checked.comp.snapshot == nil || checked.comp.businesses == nil || checked.comp.bizCallback == nil {
		t.Fatalf("SQL 装配缺组件: %+v", checked.comp)
	}
	if _, ok := checked.comp.next.(*SQLNextStore); !ok {
		t.Fatalf("SQL 装配的下一步存储类型不符: %T", checked.comp.next)
	}
	if _, ok := checked.comp.completeTo.(*SQLCompleteToActivityStore); !ok {
		t.Fatalf("SQL 装配的直送存储类型不符: %T", checked.comp.completeTo)
	}
	if checked.Options().sqlStore != store {
		t.Fatalf("SQL 装配未保留仓储入口配置")
	}

	// 未带独立连接池：业务系统注册存储跳过（回调能力停用）
	noDBComp, err := assembleSQLComponents(env.engine, store, nil, Options{}, clock.NewFake(facadeCoverTime))
	if err != nil {
		t.Fatalf("无连接池 SQL 装配失败: %v", err)
	}
	if noDBComp.businesses != nil {
		t.Fatalf("无连接池时业务系统存储应为空")
	}
	if _, ok := noDBComp.next.(*SQLNextStore); !ok {
		t.Fatalf("无连接池装配的下一步存储类型不符: %T", noDBComp.next)
	}

	// 仓储入口为空：装配明确失败且不返回组件
	brokenComp, err := assembleSQLComponents(env.engine, nil, nil, Options{}, clock.NewFake(facadeCoverTime))
	if brokenComp != nil {
		t.Fatalf("空仓储入口不应返回组件")
	}
	facadeCoverErrCode(t, err, "WORKFLOW-0600")

	// 未检查构造函数走 SQL 分支而非内存回退
	sqlService := NewWorkflowService(env.engine, WithSQLStores(store, db))
	if _, ok := sqlService.comp.next.(*SQLNextStore); !ok {
		t.Fatalf("NewWorkflowService SQL 分支存储类型不符: %T", sqlService.comp.next)
	}

	// 未选择 SQL 时走内存装配
	memService, err := NewWorkflowServiceChecked(env.engine, WithClock(clock.NewFake(facadeCoverTime)))
	if err != nil || memService.comp == nil {
		t.Fatalf("内存装配失败: err=%v", err)
	}
	if _, ok := memService.comp.next.(*MemNextStore); !ok {
		t.Fatalf("内存装配的下一步存储类型不符: %T", memService.comp.next)
	}

	// 发起入口别名与历史查询：发起即生成首步已办镜像
	facade, _ := newFacadeService(t)
	started, err := facade.StartProcessAndFirstStep(ctx, &StartProcessInstanceVo{
		ProcessDefinitionKey: "facadeProc", Title: "首步发起覆盖", Operator: "u1",
		TenantID: "bizA", BusinessKey: "BIZ-COVER-HIS",
	})
	if err != nil {
		t.Fatalf("首步发起失败: %v", err)
	}
	data := started.Data.(map[string]any)
	facadeInstanceID := data["processInstanceId"].(string)
	firstTaskID := data["taskId"].(string)
	historyR, err := facade.GetTaskHistoryListByProcessInstanceID(ctx, facadeInstanceID)
	if err != nil {
		t.Fatalf("按实例查询历史失败: %v", err)
	}
	if got := len(historyR.Data.([]*HistoryExtend)); got != 1 {
		t.Fatalf("发起后历史数量不符: %d", got)
	}
	recordR, err := facade.GetTaskHistoryListByTaskID(ctx, firstTaskID)
	if err != nil || recordR.Data == nil {
		t.Fatalf("按任务查询历史失败: %+v err=%v", recordR.Data, err)
	}
	// 不存在任务返回空数据而非错误
	missingHistoryR, err := facade.GetTaskHistoryListByTaskID(ctx, "ghost-task")
	missingHistory, _ := missingHistoryR.Data.(*HistoryExtend)
	if err != nil || missingHistory != nil {
		t.Fatalf("未命中任务历史应为空: %+v err=%v", missingHistoryR.Data, err)
	}
	// 无历史实例返回空列表
	emptyHistoryR, err := facade.GetTaskHistoryListByProcessInstanceID(ctx, "ghost-instance")
	if err != nil || len(emptyHistoryR.Data.([]*HistoryExtend)) != 0 {
		t.Fatalf("无历史实例应为空列表: %+v err=%v", emptyHistoryR.Data, err)
	}

	// 办结首任务生成下一待办后历史镜像追加（发起镜像 + 新待办镜像）
	completed, err := facade.Complete(ctx, &CompleteTaskVo{
		BaseProcessVo: BaseProcessVo{
			TaskID: firstTaskID, UserCode: "u1", Message: "同意", ProcessInstanceID: facadeInstanceID, Type: CommentTypeSP,
		},
		Variables: map[string]any{"operationType": "agreeComplete"},
		Request: &NextActivityRequest{
			TaskID: firstTaskID,
			NextActivityList: []*NextActivityVo{
				{ActivityID: "approve", ExecutorIDs: []string{"u2"}},
			},
			SendIdentityID: "u1", SendUserName: "张三",
		},
	})
	if err != nil {
		t.Fatalf("办结首任务失败: %v", err)
	}
	nextIDs := completed.Data.(map[string]any)["nextTaskIds"].([]string)
	if len(nextIDs) != 1 {
		t.Fatalf("办结后下一任务数量不符: %v", nextIDs)
	}
	historyR, err = facade.GetTaskHistoryListByProcessInstanceID(ctx, facadeInstanceID)
	if err != nil {
		t.Fatalf("办结后按实例查询历史失败: %v", err)
	}
	if got := len(historyR.Data.([]*HistoryExtend)); got != 2 {
		t.Fatalf("办结后历史数量不符: %d", got)
	}
	nextRecordR, err := facade.GetTaskHistoryListByTaskID(ctx, nextIDs[0])
	if err != nil || nextRecordR.Data == nil {
		t.Fatalf("下一任务历史镜像缺失: %+v err=%v", nextRecordR.Data, err)
	}
}

// TestFacadeCoverUnassembledGuards 覆盖无引擎（协议兼容）场景下门面能力方法的统一装配错误。
// @param t 测试上下文
func TestFacadeCoverUnassembledGuards(t *testing.T) {
	nilService := NewWorkflowService(nil)
	ctx := tenant.With(context.Background(), "bizA")

	cases := []struct {
		name string
		call func() (R, error)
	}{
		{"GetBackNodes", func() (R, error) { return nilService.GetBackNodes(ctx, "task") }},
		{"BackToStepTask", func() (R, error) { return nilService.BackToStepTask(ctx, &BackTaskVo{TaskID: "task"}) }},
		{"AddMultiInstanceMember", func() (R, error) {
			return nilService.AddMultiInstanceMember(ctx, "pi", "act", "u1")
		}},
		{"RemoveMultiInstanceMember", func() (R, error) {
			return nilService.RemoveMultiInstanceMember(ctx, "pi", "act", "u1", "", false)
		}},
		{"CompleteSign", func() (R, error) { return nilService.CompleteSign(ctx, "task", 1) }},
		{"DoTaskOwnerAdjustment", func() (R, error) {
			return nilService.DoTaskOwnerAdjustment(ctx, "pi", "key", "u1", "u2", "op")
		}},
		{"DoTaskOwnerAdjustmentByID", func() (R, error) {
			return nilService.DoTaskOwnerAdjustmentByID(ctx, "task", "u2", "op")
		}},
		{"EditDelegateTask", func() (R, error) { return nilService.EditDelegateTask(ctx, &DelegateRule{ID: "r"}) }},
		{"EnableDelegateTask", func() (R, error) { return nilService.EnableDelegateTask(ctx, "r") }},
		{"RetireDelegateTask", func() (R, error) { return nilService.RetireDelegateTask(ctx, "r") }},
		{"EditRemind", func() (R, error) { return nilService.EditRemind(ctx, RemindSaveRequest{}) }},
		{"HandleTaskExceptions", func() (R, error) { return nilService.HandleTaskExceptions(ctx, []string{"e"}, true) }},
		{"ListTaskExceptions", func() (R, error) { return nilService.ListTaskExceptions(ctx, "pi") }},
		{"SaveActExtend", func() (R, error) { return nilService.SaveActExtend(ctx, &ActExtend{ActivityID: "a"}) }},
		{"ListActExtends", func() (R, error) { return nilService.ListActExtends(ctx, "key", "") }},
		{"SaveCountersignWeights", func() (R, error) {
			return nilService.SaveCountersignWeights(ctx, "key", "act", "", nil)
		}},
		{"ListCountersignWeights", func() (R, error) {
			return nilService.ListCountersignWeights(ctx, "key", "act", "")
		}},
		{"ClaimTask", func() (R, error) { return nilService.ClaimTask(ctx, "task", "u1") }},
		{"ResolveTask", func() (R, error) { return nilService.ResolveTask(ctx, "task", "u1") }},
		{"SuspendProcessInstances", func() (R, error) { return nilService.SuspendProcessInstances(ctx, []string{"pi"}, "u1") }},
		{"ActivateProcessInstances", func() (R, error) { return nilService.ActivateProcessInstances(ctx, []string{"pi"}, "u1") }},
		{"GetProcessInstancePageList", func() (R, error) { return nilService.GetProcessInstancePageList(ctx, PageQuery{}) }},
		{"GetIdentityTree", func() (R, error) { return nilService.GetIdentityTree(ctx) }},
		{"GetOperationButtons", func() (R, error) { return nilService.GetOperationButtons(ctx, "pi") }},
		{"GetUserTaskMetas", func() (R, error) { return nilService.GetUserTaskMetas(ctx, "def") }},
		{"GetModelXML", func() (R, error) { return nilService.GetModelXML(ctx, "model") }},
		{"SaveModelXML", func() (R, error) { return nilService.SaveModelXML(ctx, "model", "<definitions/>") }},
		{"GetDefinitionXML", func() (R, error) { return nilService.GetDefinitionXML(ctx, "def") }},
		{"UpdateModelFromDefinition", func() (R, error) {
			return nilService.UpdateModelFromDefinition(ctx, "bizA", "def")
		}},
		{"CopyProcess", func() (R, error) { return nilService.CopyProcess(ctx, "source", []string{"target"}) }},
		{"GetModelAndStepPage", func() (R, error) { return nilService.GetModelAndStepPage(ctx, "bizA") }},
		{"GetStepDesign", func() (R, error) { return nilService.GetStepDesign(ctx, "step") }},
		{"DelegateTask", func() (R, error) { return nilService.DelegateTask(ctx, "task", "u2") }},
		{"DeleteCountersignWeights", func() (R, error) {
			return nilService.DeleteCountersignWeights(ctx, "key", "act", "")
		}},
		{"GetReviewTask", func() (R, error) { return nilService.GetReviewTask(ctx, "review") }},
		{"ListReviewTasksByReviewer", func() (R, error) {
			return nilService.ListReviewTasksByReviewer(ctx, "u1", "bizA", PageQuery{})
		}},
		{"ListReviewTasksByInstance", func() (R, error) {
			return nilService.ListReviewTasksByInstance(ctx, "pi")
		}},
		{"CountUnreadReviews", func() (R, error) { return nilService.CountUnreadReviews(ctx, "u1", "bizA") }},
		{"GetFlowHighlight", func() (R, error) { return nilService.GetFlowHighlight(ctx, "pi") }},
		{"GetFlowHighlightByTask", func() (R, error) { return nilService.GetFlowHighlightByTask(ctx, "task") }},
		{"AddStepDesign", func() (R, error) { return nilService.AddStepDesign(ctx, &StepDesign{}) }},
		{"EditStepDesign", func() (R, error) { return nilService.EditStepDesign(ctx, &StepDesign{}) }},
		{"ListStepDesignByModel", func() (R, error) { return nilService.ListStepDesignByModel(ctx, "model") }},
		{"AddTemplateDef", func() (R, error) { return nilService.AddTemplateDef(ctx, &TemplateDef{}) }},
		{"EditTemplateDef", func() (R, error) { return nilService.EditTemplateDef(ctx, &TemplateDef{}) }},
		{"ListTemplateDefs", func() (R, error) { return nilService.ListTemplateDefs(ctx, "bizA") }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, err := c.call()
			if r.Success {
				t.Fatalf("无引擎时 %s 不应返回成功", c.name)
			}
			facadeCoverErrCode(t, err, "WORKFLOW-0601")
		})
	}
}
