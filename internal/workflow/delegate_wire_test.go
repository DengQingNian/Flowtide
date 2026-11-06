package workflow

// 定时委托接线与触发扫描测试：
// 1. 待办创建链接线 —— MessageExtendService.CreateForTask 新建待办时同步触发
//    DelegateRuleService.ApplyOnTodoCreate（对位 Java saveMessageExtendInfo 自动委托段）；
// 2. 失败可观测但不阻断待办创建；钩子内重建待办不再二次触发委托；
// 3. TriggerDelegateScan 运维触发 —— 扫描未委托活跃待办，三级命中后按
//    AdjustTaskOwner 改派（对位 Java FlowableScheduleTask.scheduledDelegatedTask）。

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	clockpkg "github.com/DengQingNian/Flowtide/internal/clock"
	enginepkg "github.com/DengQingNian/Flowtide/internal/engine"
)

// wireBPMN 是单用户任务的最小流程模型（apply 节点处理人 11）。
const wireBPMN = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn"><process id="wire" isExecutable="true"><startEvent id="start"/><sequenceFlow id="one" sourceRef="start" targetRef="apply"/><userTask id="apply" flowable:assignee="11"/><sequenceFlow id="two" sourceRef="apply" targetRef="end"/><endEvent id="end"/></process></definitions>`

// newWireService 构造内存门面服务并部署单用户任务模型（真实装配链：钩子在 assembleCapabilities 注入），
// 同时播种委托链涉及的身份 11/22/33（改派链校验「任务处理人不存在」需要账号存在）。
// @param t 测试上下文
// @return 门面服务
// @return 租户请求上下文
func newWireService(t *testing.T) (*Service, context.Context) {
	t.Helper()
	ctx := context.Background()
	eng, err := enginepkg.New(enginepkg.Config{TenantID: "flow"})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"11", "22", "33"} {
		if _, err := eng.IdentityService().CreateUser(ctx, api.User{ID: id, DisplayName: "用户" + id, TenantID: "flow"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := eng.RepositoryService().Deploy(ctx, api.DeploymentRequest{Resources: map[string][]byte{"wire.bpmn20.xml": []byte(wireBPMN)}}); err != nil {
		t.Fatal(err)
	}
	return NewWorkflowService(eng), ctx
}

// startWireInstance 发起实例并返回首任务 Id（owner=办理人 11）。
// @param t 测试上下文
// @param svc 门面服务
// @param ctx 请求上下文
// @return 任务 ID
// @return 实例 ID
func startWireInstance(t *testing.T, svc *Service, ctx context.Context) (string, string) {
	t.Helper()
	result, err := svc.StartProcess(ctx, &api.StartProcessInstanceVo{
		ProcessDefinitionKey: "wire", Title: "定时委托接线", Operator: "99",
		TenantID: "flow", BusinessKey: "BIZ-WIRE-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	data, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatalf("发起结果结构错误：%#v", result.Data)
	}
	instanceID, _ := data["processInstanceId"].(string)
	taskID, _ := data["taskId"].(string)
	if instanceID == "" || taskID == "" {
		t.Fatalf("发起结果缺少实例/任务 Id：%#v", data)
	}
	return taskID, instanceID
}

// addAllRule 新增一条「全部委托」规则（窗口覆盖当前时刻前后各一天）。
// @param t 测试上下文
// @param svc 门面服务
// @param ctx 请求上下文
// @param ownerID 委托人
// @param consignerID 被委托人
func addAllRule(t *testing.T, svc *Service, ctx context.Context, ownerID, consignerID string) {
	t.Helper()
	now := time.Now()
	if _, err := svc.AddDelegateTask(ctx, &DelegateRule{
		ConsignType: ConsignTypeAll, OwnerID: ownerID, SourceCode: "flow",
		VaildFrom: now.Add(-24 * time.Hour), VaildTo: now.Add(24 * time.Hour),
		Consigners: []DelegateConsigner{{ConsignerID: consignerID}},
	}); err != nil {
		t.Fatal(err)
	}
}

// TestTodoCreateAppliesDelegateRule 验证待办创建链已接线委托钩子。
// 场景：先建规则（11→22 全部委托），再发起实例（首任务办理人 11）；
// 预期：待办在创建时同步改派 —— owner=22、consigner=22（ApplyOnTodoCreate 语义），委托镜像落一行。
// @param t 测试上下文
func TestTodoCreateAppliesDelegateRule(t *testing.T) {
	svc, ctx := newWireService(t)
	addAllRule(t, svc, ctx, "11", "22")
	taskID, _ := startWireInstance(t, svc, ctx)
	msg, found, err := svc.comp.messages.GetByTaskID(ctx, taskID)
	if err != nil || !found {
		t.Fatalf("待办应已生成：found=%v err=%v", found, err)
	}
	if msg.OwnerID != "22" {
		t.Errorf("待办创建时应同步改派 owner=22，实际 %q", msg.OwnerID)
	}
	if msg.ConsignerID != "22" {
		t.Errorf("改派后 consigner 应记录受托人 22，实际 %q", msg.ConsignerID)
	}
	mirrors, err := svc.comp.delegate.mirrors.ListByTaskID(ctx, taskID)
	if err != nil || len(mirrors) != 1 {
		t.Fatalf("委托镜像应落一行：n=%d err=%v", len(mirrors), err)
	}
	// 引擎任务 assignee 同步改写（ApplyOnTodoCreate 重建待办不改引擎任务，镜像 owner 为准）
	if mirrors[0].EntrustID != "22" {
		t.Errorf("镜像被委托人应为 22，实际 %q", mirrors[0].EntrustID)
	}
}

// TestTodoCreateHookSkipsAlreadyDelegated 验证委托钩子不再改派已委托待办（Java：
// 「不支持第三人再将工作委托」，同时阻断钩子重建待办造成的递归触发）。
// 场景：直调待办服务创建一条已带 consigner 的待办（owner=22 已是受托人），且 22 存在生效规则（22→33）；
// 预期：钩子跳过，owner 保持 22、不被改派为 33。
// @param t 测试上下文
func TestTodoCreateHookSkipsAlreadyDelegated(t *testing.T) {
	svc, ctx := newWireService(t)
	addAllRule(t, svc, ctx, "22", "33")
	taskID, _ := startWireInstance(t, svc, ctx)
	// 场景一：发起链新建的待办（owner=11，无规则）不应被 22/33 规则误改派
	msg, found, err := svc.comp.messages.GetByTaskID(ctx, taskID)
	if err != nil || !found {
		t.Fatalf("待办应已生成：found=%v err=%v", found, err)
	}
	if msg.OwnerID != "11" {
		t.Errorf("无规则处理人的待办应保持原 owner，实际 %q", msg.OwnerID)
	}
	// 场景二：已委托待办（consigner 非空）重建时不再二次委托
	rebuilt := &MessageExtend{
		BaseColumns: BaseColumns{TenantID: "flow"}, TaskID: "T-DELEGATED",
		ProcessInstanceID: "PI-1", OwnerID: "22", ConsignerID: "11", FlowCategoryCode: "wire",
	}
	if _, err := svc.comp.messages.CreateForTask(ctx, rebuilt); err != nil {
		t.Fatal(err)
	}
	final, found, err := svc.comp.messages.GetByTaskID(ctx, "T-DELEGATED")
	if err != nil || !found {
		t.Fatalf("已委托待办应存在：found=%v err=%v", found, err)
	}
	if final.OwnerID != "22" {
		t.Errorf("已委托待办不得再改派（应保持 22），实际 %q", final.OwnerID)
	}
}

// TestTodoCreateHookFailureNonBlocking 验证钩子失败不阻断待办创建，失败时记录告警并保留原处理人。
// 场景：注入恒失败的钩子后新建待办；
// 预期：CreateForTask 无错误返回，待办行存在且 owner 保持原值；幂等重建（同任务二次创建）不再触发钩子。
// @param t 测试上下文
func TestTodoCreateHookFailureNonBlocking(t *testing.T) {
	store := NewMemMessageStore(clockpkg.Real{})
	svc := NewMessageExtendService(store, clockpkg.Real{})
	calls := 0
	svc.InstallDelegateHook(func(ctx context.Context, msg *MessageExtend) (*MessageExtend, error) {
		calls++
		return nil, errors.New("钩子故障")
	})
	ctx := context.Background()
	req := &MessageExtend{BaseColumns: BaseColumns{TenantID: "flow"}, TaskID: "T-1",
		ProcessInstanceID: "PI-1", OwnerID: "11", FlowCategoryCode: "wire"}
	if _, err := svc.CreateForTask(ctx, req); err != nil {
		t.Fatalf("钩子失败不得阻断待办创建：%v", err)
	}
	if calls != 1 {
		t.Fatalf("新建待办应恰好触发一次钩子，实际 %d", calls)
	}
	if _, found, err := store.GetByTaskID(ctx, "T-1"); err != nil || !found {
		t.Fatalf("待办行应已落库：found=%v err=%v", found, err)
	}
	// 幂等路径（同任务已有待办）不再触发钩子
	if _, err := svc.CreateForTask(ctx, req); err != nil {
		t.Fatalf("幂等创建不应报错：%v", err)
	}
	if calls != 1 {
		t.Errorf("幂等重建不应再次触发钩子，实际 %d 次", calls)
	}
}

// TestTriggerDelegateScanAdjustsTodo 验证运维触发扫描按规则改派未委托待办。
// 场景：先发起（无规则，待办 owner=11）→ 再建规则（11→22）→ 触发扫描；
// 预期：扫描结果 Adjusted=1；待办按 AdjustTaskOwner 重建（owner=22、consigner 记原处理人 11）；
// 重复扫描幂等（已委托待办跳过，第二次 Adjusted=0）。
// @param t 测试上下文
func TestTriggerDelegateScanAdjustsTodo(t *testing.T) {
	svc, ctx := newWireService(t)
	taskID, _ := startWireInstance(t, svc, ctx)
	addAllRule(t, svc, ctx, "11", "22")
	result, err := svc.TriggerDelegateScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	summary, ok := result.Data.(*DelegateScanResult)
	if !ok {
		t.Fatalf("扫描结果类型错误：%T", result.Data)
	}
	if summary.Adjusted != 1 || summary.Failed != 0 {
		t.Fatalf("首次扫描应改派 1 条且无失败：%+v", summary)
	}
	if summary.Scanned < 1 {
		t.Fatalf("扫描应覆盖未委托待办：%+v", summary)
	}
	msg, found, err := svc.comp.messages.GetByTaskID(ctx, taskID)
	if err != nil || !found {
		t.Fatalf("待办应存在：found=%v err=%v", found, err)
	}
	if msg.OwnerID != "22" {
		t.Errorf("扫描改派后 owner 应为 22，实际 %q", msg.OwnerID)
	}
	if msg.ConsignerID != "11" {
		t.Errorf("扫描改派后 consigner 应记录原处理人 11，实际 %q", msg.ConsignerID)
	}
	// 幂等：第二次扫描不再改派（consigner 非空跳过，对位 Java getDelegatedMessageList consigner_id IS NULL）
	again, err := svc.TriggerDelegateScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if second, ok := again.Data.(*DelegateScanResult); !ok || second.Adjusted != 0 {
		t.Fatalf("重复扫描应零改派：%+v", again.Data)
	}
}

// TestTriggerDelegateScanNoHitAndNoSource 验证扫描的未命中与数据源缺失分支。
// 场景一：规则与待办处理人无关，扫描零改派不报错；
// 场景二：扫描数据源未接入（todos 为 nil）时返回可观测错误。
// @param t 测试上下文
func TestTriggerDelegateScanNoHitAndNoSource(t *testing.T) {
	svc, ctx := newWireService(t)
	startWireInstance(t, svc, ctx)
	// 规则与处理人无关（owner=88）
	addAllRule(t, svc, ctx, "88", "99")
	result, err := svc.TriggerDelegateScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if summary, ok := result.Data.(*DelegateScanResult); !ok || summary.Adjusted != 0 || summary.Failed != 0 {
		t.Fatalf("无命中扫描应零改派零失败：%+v", result.Data)
	}
	// 数据源缺失：直接构造无 todos 的域服务（对位 SQL 模式裸 SQL 数据源未接入口径）
	bare := NewDelegateRuleService(NewMemTaskDelegateStore(clockpkg.Real{}), nil, nil, nil, nil, nil, nil, nil, nil, nil)
	if _, err := bare.TriggerScan(ctx); err == nil || !strings.Contains(err.Error(), "数据源") {
		t.Fatalf("数据源缺失应返回可观测错误，实际 %v", err)
	}
}
