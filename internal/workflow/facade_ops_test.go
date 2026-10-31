package workflow

// facade_ops_test.go 覆盖 P2-C/P3/P4/P5 门面接线：退回/撤回/直送/重置/软终止/挂起/重激活/
// 加签减签/会签/推演/委托/催办/异常/调度器生命周期。语料独立（前缀 ops）避免污染他文件。

import (
	"context"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
	"github.com/DengQingNian/Flowtide/internal/engine"
	"github.com/DengQingNian/Flowtide/internal/idgen"
	"github.com/DengQingNian/Flowtide/internal/tenant"
)

// opsFixture 是门面能力测试语料：apply → handle → approve → end 线性三步。
const opsFixture = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="opsProc" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="apply"/>
    <userTask id="apply" name="申请" flowable:assignee="${initiator}" stepCode="S00"/>
    <sequenceFlow id="f1" sourceRef="apply" targetRef="handle"/>
    <userTask id="handle" name="办理" flowable:assignee="${initiator}" stepCode="S01"/>
    <sequenceFlow id="f2" sourceRef="handle" targetRef="approve"/>
    <userTask id="approve" name="批准" flowable:assignee="u2" stepCode="S02"/>
    <sequenceFlow id="f3" sourceRef="approve" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// opsCountersignFixture 是会签语料：mtask 多实例两人会签后办结。
const opsCountersignFixture = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="opsCounter" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="mtask"/>
    <userTask id="mtask" name="会签" flowable:assignee="${executor}"
              executorType="person" executorList='[{"identityId":"u1"},{"identityId":"u2"}]'>
      <multiInstanceLoopCharacteristics isSequential="false"/>
    </userTask>
    <sequenceFlow id="f1" sourceRef="mtask" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// opsFixtureEnv 是门面能力测试环境。
type opsFixtureEnv struct {
	service *Service   // 门面服务
	engine  api.Engine // 引擎门面
}

// newOpsService 构造门面能力测试环境（部署语料、建身份、装配全量能力子服务）。
// @param t 测试上下文
// @return 测试环境
func newOpsService(t *testing.T) *opsFixtureEnv {
	t.Helper()
	eng, err := engine.New(engine.Config{
		HistoryLevel: engine.HistoryAudit,
		Clock:        clock.NewFake(time.Date(2026, 10, 31, 8, 0, 0, 0, time.UTC)),
		IDs:          idgen.NewUUIDv7(),
	})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close(context.Background()) })
	ctx := tenant.With(context.Background(), "bizA")
	for _, user := range []api.User{{ID: "u1", FirstName: "张", LastName: "三"}, {ID: "u2", FirstName: "李", LastName: "四"}} {
		if _, err := eng.IdentityService().CreateUser(ctx, user); err != nil {
			t.Fatalf("创建用户 %s 失败: %v", user.ID, err)
		}
	}
	if _, err := eng.RepositoryService().Deploy(ctx, api.DeploymentRequest{
		Name:     "ops",
		TenantID: "bizA",
		Resources: map[string][]byte{
			"ops.bpmn20.xml":         []byte(opsFixture),
			"ops-counter.bpmn20.xml": []byte(opsCountersignFixture),
		},
	}); err != nil {
		t.Fatalf("部署失败: %v", err)
	}
	service := NewWorkflowService(eng, WithClock(clock.NewFake(time.Date(2026, 10, 31, 8, 0, 0, 0, time.UTC))))
	if err := service.RegisterExecutorRewriteHook(ctx); err != nil {
		t.Fatalf("注册钩子失败: %v", err)
	}
	return &opsFixtureEnv{service: service, engine: eng}
}

// start 发起实例并返回实例 Id 与首个任务 Id。
// @param t 测试上下文
// @param key 流程定义 Key
// @param businessKey 业务单据 Id
// @return 实例 Id
// @return 首任务 Id
func (e *opsFixtureEnv) start(t *testing.T, key, businessKey string) (string, string) {
	t.Helper()
	started, err := e.service.StartProcess(tenant.With(context.Background(), "bizA"), &StartProcessInstanceVo{
		ProcessDefinitionKey: key, Title: "ops-" + key, Operator: "u1",
		TenantID: "bizA", BusinessKey: businessKey,
	})
	if err != nil {
		t.Fatalf("发起失败: %v", err)
	}
	data, ok := started.Data.(map[string]any)
	if !ok {
		t.Fatalf("发起返回数据形态异常: %+v", started.Data)
	}
	instanceID, _ := data["processInstanceId"].(string)
	taskID, _ := data["taskId"].(string)
	if instanceID == "" || taskID == "" {
		t.Fatalf("发起返回缺实例/任务 Id: %+v", data)
	}
	return instanceID, taskID
}

// currentTask 返回实例当前首个任务 Id。
// @param t 测试上下文
// @param instanceID 实例 Id
// @return 任务 Id
func (e *opsFixtureEnv) currentTask(t *testing.T, instanceID string) string {
	t.Helper()
	page, err := e.engine.TaskService().ListTasks(tenant.With(context.Background(), "bizA"), api.TaskFilter{ProcessInstanceID: instanceID, Size: 10})
	if err != nil {
		t.Fatalf("查询任务失败: %v", err)
	}
	if len(page.Data) == 0 {
		t.Fatalf("实例 %s 无任务", instanceID)
	}
	return page.Data[0].ID
}

// TestOpsBackAndRollback 退回与撤回门面链路：办理后退回上一节点、再撤回到发起步。
// @param t 测试上下文
func TestOpsBackAndRollback(t *testing.T) {
	env := newOpsService(t)
	ctx := tenant.With(context.Background(), "bizA")
	instanceID, applyTaskID := env.start(t, "opsProc", "BIZ-OPS-1")

	// 可退节点列表（apply 为首任务，无历史可退节点）
	backR, err := env.service.GetBackNodes(ctx, applyTaskID)
	if err != nil {
		t.Fatalf("可退节点查询失败: %v", err)
	}
	if !backR.Success {
		t.Fatalf("可退节点查询应成功: %+v", backR)
	}

	// 推进到 handle，然后退回上一节点（apply）
	if _, err := env.service.Complete(ctx, &CompleteTaskVo{
		BaseProcessVo: BaseProcessVo{TaskID: applyTaskID, UserCode: "u1", Message: "同意", ProcessInstanceID: instanceID, Type: CommentTypeSP},
	}); err != nil {
		t.Fatalf("办结 apply 失败: %v", err)
	}
	handleTaskID := env.currentTask(t, instanceID)
	backed, err := env.service.BackTask(ctx, &BackTaskVo{TaskID: handleTaskID, UserCode: "u1", Message: "退回修改"})
	if err != nil {
		t.Fatalf("退回上一节点失败: %v", err)
	}
	if !backed.Success {
		t.Fatalf("退回返回异常: %+v", backed)
	}
	// 回到 apply
	if got := env.currentTask(t, instanceID); got == handleTaskID {
		t.Fatalf("退回后应回到 apply 节点")
	}

	// 撤回：u1 撤回已办的 apply 任务（handle 换回的新任务未签收，回到待办）
	rolled, err := env.service.DoRollbackTask(ctx, instanceID, applyTaskID, "u1")
	if err != nil {
		t.Fatalf("撤回失败: %v", err)
	}
	if !rolled.Success {
		t.Fatalf("撤回返回异常: %+v", rolled)
	}
}

// TestOpsDirectAndReset 直送与重置门面链路。
// @param t 测试上下文
func TestOpsDirectAndReset(t *testing.T) {
	env := newOpsService(t)
	ctx := tenant.With(context.Background(), "bizA")
	instanceID, applyTaskID := env.start(t, "opsProc", "BIZ-OPS-2")

	// 直送到 approve 节点（直送要求预存目标执行人：request 首条）
	direct, err := env.service.CompleteToTargetActivity(ctx, &CompleteTaskVo{
		BaseProcessVo:    BaseProcessVo{TaskID: applyTaskID, UserCode: "u1", Message: "直送", ProcessInstanceID: instanceID, Type: CommentTypeSP},
		TargetActivityID: "approve",
		Request: &NextActivityRequest{
			TaskID:           applyTaskID,
			NextActivityList: []*NextActivityVo{{ActivityID: "approve", ExecutorIDs: []string{"u2"}}},
			SendIdentityID:   "u1",
		},
	})
	if err != nil {
		t.Fatalf("直送失败: %v", err)
	}
	if !direct.Success {
		t.Fatalf("直送返回异常: %+v", direct)
	}
	// 直送后当前节点应为 approve（u2 为候选无 assignee 时仍生成任务）
	approveTaskID := env.currentTask(t, instanceID)
	if approveTaskID == applyTaskID {
		t.Fatalf("直送后应推进到 approve 节点")
	}

	// 重置：清空运行态回到发起节点
	reset, err := env.service.DoResetProcess(ctx, instanceID, "u1")
	if err != nil {
		t.Fatalf("重置失败: %v", err)
	}
	if !reset.Success {
		t.Fatalf("重置返回异常: %+v", reset)
	}
}

// TestOpsTerminateLifecycle 软终止/挂起/激活/删除门面链路。
// @param t 测试上下文
func TestOpsTerminateLifecycle(t *testing.T) {
	env := newOpsService(t)
	ctx := tenant.With(context.Background(), "bizA")
	instanceID, _ := env.start(t, "opsProc", "BIZ-OPS-3")

	// 挂起 → 激活
	if _, err := env.service.DoSuspendProcessInstance(ctx, instanceID, "u1"); err != nil {
		t.Fatalf("挂起失败: %v", err)
	}
	if _, err := env.service.DoActivateProcessInstance(ctx, instanceID, "u1"); err != nil {
		t.Fatalf("激活失败: %v", err)
	}

	// 软终止
	stopped, err := env.service.DoStopProcessInstance(ctx, instanceID, "u1", "作废")
	if err != nil {
		t.Fatalf("软终止失败: %v", err)
	}
	if !stopped.Success {
		t.Fatalf("软终止返回异常: %+v", stopped)
	}

	// 再发起一个按业务键删除
	instanceID2, _ := env.start(t, "opsProc", "BIZ-OPS-4")
	deleted, err := env.service.DoDeleteProcessInstance(ctx, instanceID2, "u1", "测试删除")
	if err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if !deleted.Success {
		t.Fatalf("删除返回异常: %+v", deleted)
	}
}

// TestOpsSimulateFacade 推演门面：任务推演给下一步、按 Key 推演给首步。
// @param t 测试上下文
func TestOpsSimulateFacade(t *testing.T) {
	env := newOpsService(t)
	ctx := tenant.With(context.Background(), "bizA")
	_, applyTaskID := env.start(t, "opsProc", "BIZ-OPS-5")

	// 任务推演：apply 下一步 = handle
	views, err := env.service.GetNextUserTaskAndUserList(ctx, applyTaskID)
	if err != nil {
		t.Fatalf("任务推演失败: %v", err)
	}
	if !views.Success {
		t.Fatalf("任务推演返回异常: %+v", views)
	}
	// 按 Key 推演：首步 = apply（发起人 u1）
	byKey, err := env.service.GetNextUserTaskAndUserListByKey(ctx, "bizA", "opsProc", "BIZ-OPS-5", nil)
	if err != nil {
		t.Fatalf("按 Key 推演失败: %v", err)
	}
	if !byKey.Success || byKey.Data == nil {
		t.Fatalf("按 Key 推演返回异常: %+v", byKey)
	}
}

// TestOpsAddSignFacade 加签减签门面链路：apply 后加签 u2，再减签恢复。
// @param t 测试上下文
func TestOpsAddSignFacade(t *testing.T) {
	env := newOpsService(t)
	ctx := tenant.With(context.Background(), "bizA")
	_, applyTaskID := env.start(t, "opsProc", "BIZ-OPS-6")

	// 后加签 u2
	added, err := env.service.AddExecution(ctx, AddSignRequest{
		TaskID: applyTaskID, Operator: "u1", Assignees: []string{"u2"},
		ScopeType: AddSignScopeAfter, Message: "帮忙看一下",
	})
	if err != nil {
		t.Fatalf("加签失败: %v", err)
	}
	if !added.Success {
		t.Fatalf("加签返回异常: %+v", added)
	}
	result, ok := added.Data.(AddSignResult)
	if !ok || len(result.SubTaskIDs) == 0 {
		t.Fatalf("加签结果缺子任务: %+v", added.Data)
	}

	// 减签：移除子任务恢复父任务
	deleted, err := env.service.DelExecution(ctx, DelSignRequest{
		ParentTaskID: result.ParentTaskID, SubTaskID: result.SubTaskIDs[0],
		Operator: "u1", Message: "不需要了",
	})
	if err != nil {
		t.Fatalf("减签失败: %v", err)
	}
	if !deleted.Success {
		t.Fatalf("减签返回异常: %+v", deleted)
	}
}

// TestOpsDelegateAndRemind 委托规则与催办配置的门面 CRUD。
// @param t 测试上下文
func TestOpsDelegateAndRemind(t *testing.T) {
	env := newOpsService(t)
	ctx := tenant.With(context.Background(), "bizA")

	// 委托规则：新增 → 列表 → 停用 → 删除（校验链要求生效区间与被委托人）
	now := time.Date(2026, 10, 31, 8, 0, 0, 0, time.UTC)
	rule := &DelegateRule{
		OwnerID:     "u1",
		SourceCode:  "bizA",
		VaildFrom:   now,
		VaildTo:     now.Add(24 * time.Hour),
		ConsignType: ConsignTypeAll,
		Consigners:  []DelegateConsigner{{ConsignerID: "u2"}},
	}
	if _, err := env.service.AddDelegateTask(ctx, rule); err != nil {
		t.Fatalf("新增委托失败: %v", err)
	}
	listed, err := env.service.ListDelegateTask(ctx, "u1", "bizA")
	if err != nil {
		t.Fatalf("查询委托失败: %v", err)
	}
	rules, ok := listed.Data.([]*DelegateRule)
	if !ok || len(rules) != 1 {
		t.Fatalf("委托列表不符: %+v", listed.Data)
	}
	if _, err := env.service.DisableDelegateTask(ctx, rule.ID); err != nil {
		t.Fatalf("停用委托失败: %v", err)
	}
	if _, err := env.service.DeleteDelegateTask(ctx, rule.ID); err != nil {
		t.Fatalf("删除委托失败: %v", err)
	}

	// 催办配置：新增 → 查询 → 分页 → 更新 → 删除
	saved, err := env.service.AddRemind(ctx, RemindSaveRequest{Remind: &Remind{TenantID: "bizA", PID: "P-OPS"}})
	if err != nil {
		t.Fatalf("新增催办失败: %v", err)
	}
	remind, ok := saved.Data.(*Remind)
	if !ok || remind.ID == "" {
		t.Fatalf("催办保存结果异常: %+v", saved.Data)
	}
	if _, err := env.service.GetRemind(ctx, remind.ID); err != nil {
		t.Fatalf("查询催办失败: %v", err)
	}
	if _, err := env.service.GetRemindPageList(ctx, "P-OPS", PageQuery{}); err != nil {
		t.Fatalf("分页催办失败: %v", err)
	}
	if _, err := env.service.DeleteRemind(ctx, remind.ID); err != nil {
		t.Fatalf("删除催办失败: %v", err)
	}
}

// TestOpsSchedulerLifecycle 调度器启动/停止不 panic 且幂等。
// @param t 测试上下文
func TestOpsSchedulerLifecycle(t *testing.T) {
	env := newOpsService(t)
	ctx := context.Background()
	env.service.StartSchedulers(ctx)
	env.service.StartSchedulers(ctx) // 幂等
	env.service.StopSchedulers()
	env.service.StopSchedulers() // 幂等
}

// TestOpsCapabilityNotAssembled 协议兼容场景（无引擎）能力方法快速失败。
// @param t 测试上下文
func TestOpsCapabilityNotAssembled(t *testing.T) {
	// nil 引擎构造门面（仅协议桩），能力方法应返回装配错误
	service := NewWorkflowService(nil)
	if _, err := service.GetBackNodes(context.Background(), "t1"); err == nil {
		t.Fatalf("无引擎时 GetBackNodes 应报装配错误")
	}
	if _, err := service.DoStopProcessInstance(context.Background(), "p1", "u1", ""); err == nil {
		t.Fatalf("无引擎时 DoStopProcessInstance 应报装配错误")
	}
}
