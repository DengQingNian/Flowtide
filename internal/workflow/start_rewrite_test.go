// start_rewrite_test.go 发起 + 执行人重写全链路集成测试（P1-D，内存引擎 + MemStores）。
// 语料：发起 → 首任务 assignee=${initiator} 生效（TJ 意见/操作记录/待办/已办镜像/表单落位）→
// 预存下一步（模拟 SaveAgreeNext）→ 办结携带门控变量 → 新任务执行人由预生成记录自动回填；
// 负向：门控缺失保留模型默认；会签：并行 2 实例按序消费 2 条记录且办理人各归位。
package workflow

import (
	"context"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/tenant"
)

// rewriteIntegrationCountersign 是会签集成语料：apply → 组内并行会签 cs → 结束。
const rewriteIntegrationCountersign = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="csFlowProc" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="apply"/>
    <userTask id="apply" name="申请" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f1" sourceRef="apply" targetRef="cs"/>
    <userTask id="cs" name="会签" flowable:assignee="${signer}" executorType="person">
      <multiInstanceLoopCharacteristics isSequential="false"
          flowable:collection="${executorList}" flowable:elementVariable="signer"/>
    </userTask>
    <sequenceFlow id="f2" sourceRef="cs" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// TestIntegrationStartAndRewriteFullChain 验证发起全链路落位 + 第二步执行人自动回填。
// @param t 测试上下文
func TestIntegrationStartAndRewriteFullChain(t *testing.T) {
	eng, stores, start, hook := newStartEnv(t)
	seedStartIdentities(t, eng)
	deployStartResource(t, eng, hook, "oa", "base.bpmn20.xml", startCorpusBase)
	ctx := tenant.With(context.Background(), "oa")
	// ── 发起 ──
	result, err := start.Start(ctx, startVO("startProc", "u1"))
	if err != nil {
		t.Fatalf("发起失败: %v", err)
	}
	// 首任务 assignee=${initiator} 生效
	first, err := eng.TaskService().GetTask(ctx, result.TaskID)
	if err != nil || first.Assignee != "u1" {
		t.Fatalf("首任务办理人应为发起人 u1: %v %+v", err, first)
	}
	// TJ 意见落位
	comments, err := eng.TaskService().ListComments(ctx, result.TaskID)
	if err != nil || len(comments) != 1 || comments[0].Message != "请审批" {
		t.Fatalf("TJ 意见落位不符: %v %+v", err, comments)
	}
	// 操作记录落位（TJ）
	records, _ := stores.Operation.ListByInstance(ctx, result.ProcessInstanceID)
	if len(records) != 1 || records[0].Type != CommentTypeTJ || records[0].OwnerID != "u1" {
		t.Fatalf("操作记录落位不符: %+v", records)
	}
	// 待办落位（待办中心唯一数据源）
	message, found, _ := stores.Message.GetByTaskID(ctx, result.TaskID)
	if !found || message.OwnerID != "u1" || message.Title != "测试标题" || message.BusinessID != "biz-1" {
		t.Fatalf("待办落位不符: %v %+v", found, message)
	}
	// 已办镜像落位
	if _, found, _ := stores.History.GetByTaskID(ctx, result.TaskID); !found {
		t.Fatalf("已办镜像应落位")
	}
	// 表单状态落位
	document, found, _ := stores.Document.GetByBusinessID(ctx, "biz-1", "oa")
	if !found || document.FlowStatus != FlowStatusNotSubmitted || document.FlowInstanceID != result.ProcessInstanceID {
		t.Fatalf("表单状态落位不符: %v %+v", found, document)
	}
	// ── 预存下一步（模拟门面 SaveAgreeNextActivity）──
	if err := storesNextSaveAgreeNext(t, start, result.ProcessInstanceID, result.TaskID, "approve", "u2", "oa", "u1"); err != nil {
		t.Fatalf("预存下一步失败: %v", err)
	}
	// ── 办结首任务（携带门控变量）→ 新任务执行人由预生成记录自动回填 ──
	if err := eng.TaskService().Complete(ctx, result.TaskID, api.CompleteTaskRequest{
		UserID:    "u1",
		Variables: []api.Variable{{Name: VarOperationType, Value: OperationTypeAgreeComplete}},
	}); err != nil {
		t.Fatalf("办结首任务失败: %v", err)
	}
	page, err := eng.TaskService().ListTasks(ctx, api.TaskFilter{ProcessInstanceID: result.ProcessInstanceID})
	if err != nil || page.Total != 1 {
		t.Fatalf("办结后应仅剩审批任务: %v %+v", err, page)
	}
	second := page.Data[0]
	// 新任务执行人 = 预生成记录回填（u2），模型默认 u9 被覆盖
	if second.TaskDefinitionKey != "approve" || second.Assignee != "u2" {
		t.Fatalf("新任务执行人应回填为 u2: %+v", second)
	}
	// 预生成记录消耗：used=1 且 taskId 回写为新任务
	unused, _ := stores.Next.ListUnusedByInstanceAndActivity(ctx, result.ProcessInstanceID, "approve")
	if len(unused) != 0 {
		t.Fatalf("预生成记录应被消耗: %+v", unused)
	}
	used, _ := stores.Next.ListUsedByInstance(ctx, result.ProcessInstanceID)
	consumed := false
	for _, record := range used {
		if record.NextActivityID == "approve" && record.TaskID == second.ID && record.Used == 1 {
			consumed = true
		}
	}
	if !consumed {
		t.Fatalf("消耗记录应回写新任务 ID: %+v", used)
	}
}

// storesNextSaveAgreeNext 经域服务预存下一步记录（used=0，任务创建时由钩子消耗）。
// @param t 测试上下文
// @param start 发起域服务（借其 next 域服务）
// @param processInstanceID 实例 Id
// @param preTaskID 来源任务 Id
// @param activityID 目标节点 Id
// @param identityID 执行人 Id
// @param tenantID 租户
// @param senderID 发送人 Id
// @return 错误
func storesNextSaveAgreeNext(t *testing.T, start *StartService, processInstanceID, preTaskID, activityID, identityID, tenantID, senderID string) error {
	t.Helper()
	return start.next.SaveAgreeNext(context.Background(), NextActivitySaveRequest{
		ProcessInstanceID: processInstanceID,
		ActivityID:        activityID,
		IdentityIDs:       []string{identityID},
		PreTaskID:         preTaskID,
		SendIdentityID:    senderID,
		TenantID:          tenantID,
		Creator:           senderID,
	})
}

// TestIntegrationStartGateMissingKeepsModelDefault 验证门控变量缺失时新任务保留模型默认执行人。
// @param t 测试上下文
func TestIntegrationStartGateMissingKeepsModelDefault(t *testing.T) {
	eng, stores, start, hook := newStartEnv(t)
	seedStartIdentities(t, eng)
	deployStartResource(t, eng, hook, "oa", "base.bpmn20.xml", startCorpusBase)
	ctx := tenant.With(context.Background(), "oa")
	result, err := start.Start(ctx, startVO("startProc", "u1"))
	if err != nil {
		t.Fatalf("发起失败: %v", err)
	}
	// 预存下一步
	if err := storesNextSaveAgreeNext(t, start, result.ProcessInstanceID, result.TaskID, "approve", "u2", "oa", "u1"); err != nil {
		t.Fatalf("预存下一步失败: %v", err)
	}
	// 办结不携带门控变量：钩子不触发
	if err := eng.TaskService().Complete(ctx, result.TaskID, api.CompleteTaskRequest{UserID: "u1"}); err != nil {
		t.Fatalf("办结首任务失败: %v", err)
	}
	page, _ := eng.TaskService().ListTasks(ctx, api.TaskFilter{ProcessInstanceID: result.ProcessInstanceID})
	if page.Total != 1 {
		t.Fatalf("办结后任务数不符: %+v", page)
	}
	second := page.Data[0]
	// 模型默认执行人 u9 保留，预生成记录未消耗
	if second.TaskDefinitionKey != "approve" || second.Assignee != "u9" {
		t.Fatalf("门控缺失应保留模型默认执行人 u9: %+v", second)
	}
	unused, _ := stores.Next.ListUnusedByInstanceAndActivity(ctx, result.ProcessInstanceID, "approve")
	if len(unused) != 1 || unused[0].Used != 0 {
		t.Fatalf("门控缺失时预生成记录应保持未消耗: %+v", unused)
	}
}

// TestIntegrationStartCountersignTwoInstances 验证并行会签 2 实例按序消费 2 条记录且办理人各归位。
// @param t 测试上下文
func TestIntegrationStartCountersignTwoInstances(t *testing.T) {
	eng, stores, start, hook := newStartEnv(t)
	seedStartIdentities(t, eng)
	deployStartResource(t, eng, hook, "oa", "csflow.bpmn20.xml", rewriteIntegrationCountersign)
	ctx := tenant.With(context.Background(), "oa")
	result, err := start.Start(ctx, startVO("csFlowProc", "u1"))
	if err != nil {
		t.Fatalf("发起失败: %v", err)
	}
	// 预存会签两条执行人记录（u2、u3）
	if err := storesNextSaveAgreeNext(t, start, result.ProcessInstanceID, result.TaskID, "cs", "u2", "oa", "u1"); err != nil {
		t.Fatalf("预存 u2 失败: %v", err)
	}
	if err := storesNextSaveAgreeNext(t, start, result.ProcessInstanceID, result.TaskID, "cs", "u3", "oa", "u1"); err != nil {
		t.Fatalf("预存 u3 失败: %v", err)
	}
	// 办结申请任务：携带门控变量与会签集合变量（驱动 2 个并行实例创建）
	if err := eng.TaskService().Complete(ctx, result.TaskID, api.CompleteTaskRequest{
		UserID: "u1",
		Variables: []api.Variable{
			{Name: VarOperationType, Value: OperationTypeAgreeComplete},
			{Name: VarExecutorList, Value: []any{"u2", "u3"}},
		},
	}); err != nil {
		t.Fatalf("办结申请任务失败: %v", err)
	}
	// 2 个会签实例任务创建，办理人各归位（元素变量 ${signer} 驱动）
	page, err := eng.TaskService().ListTasks(ctx, api.TaskFilter{ProcessInstanceID: result.ProcessInstanceID})
	if err != nil || page.Total != 2 {
		t.Fatalf("应创建 2 个会签实例任务: %v %+v", err, page)
	}
	assignees := map[string]string{}
	for _, task := range page.Data {
		// 钩子逐人回填候选人：办理人归位 + 候选人含同身份
		assignees[task.Assignee] = task.ID
		if len(task.CandidateUsers) != 1 || task.CandidateUsers[0] != task.Assignee {
			t.Fatalf("会签实例应回填同身份候选人: %+v", task)
		}
	}
	if _, ok := assignees["u2"]; !ok {
		t.Fatalf("会签实例办理人应含 u2: %v", assignees)
	}
	if _, ok := assignees["u3"]; !ok {
		t.Fatalf("会签实例办理人应含 u3: %v", assignees)
	}
	// 2 条预生成记录被 2 个实例按序消费，taskId 各自回写
	unused, _ := stores.Next.ListUnusedByInstanceAndActivity(ctx, result.ProcessInstanceID, "cs")
	if len(unused) != 0 {
		t.Fatalf("2 实例应消费全部 2 条记录: %+v", unused)
	}
	used, _ := stores.Next.ListUsedByInstance(ctx, result.ProcessInstanceID)
	consumedCS := map[string]string{}
	for _, record := range used {
		if record.NextActivityID == "cs" {
			consumedCS[record.IdentityID] = record.TaskID
		}
	}
	if consumedCS["u2"] != assignees["u2"] || consumedCS["u3"] != assignees["u3"] {
		t.Fatalf("消耗记录应与实例任务一一对应: %v vs %v", consumedCS, assignees)
	}
}
