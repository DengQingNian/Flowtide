package workflow

import (
	"context"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
)

// directNoExecutorProcess 是直送校验语料：d1（有办理人）→ d2（无任何执行者配置）→ end，
// 用于验证目标节点未设置执行者的拒绝分支。
const directNoExecutorProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="directNoExecutor" name="直送校验流程" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="d1"/>
    <userTask id="d1" name="直送源" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f1" sourceRef="d1" targetRef="d2"/>
    <userTask id="d2" name="无执行者节点"/>
    <sequenceFlow id="f2" sourceRef="d2" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// newDirectFixture 构造直送测试环境（复用退回环境并追加直送校验语料）。
// @param t 测试上下文
// @return 退回测试环境
func newDirectFixture(t *testing.T) *backFixture {
	t.Helper()
	fx := newBackFixture(t)
	// 追加部署直送校验语料
	if _, err := fx.engine.RepositoryService().Deploy(context.Background(), api.DeploymentRequest{
		Name:     "direct-test",
		TenantID: "dev",
		Resources: map[string][]byte{
			"directnoexec.bpmn20.xml": []byte(directNoExecutorProcess),
		},
	}); err != nil {
		t.Fatalf("部署直送语料失败: %v", err)
	}
	return fx
}

// registerRewriteHook 注册执行人重写钩子并预热元数据缓存（直送门控回填验证用）。
// @param t 测试上下文
// @param f 测试环境
// @return 执行人重写钩子
func registerRewriteHook(t *testing.T, f *backFixture) *ExecutorRewriteHook {
	t.Helper()
	hook := NewExecutorRewriteHook(NewModelMetaService(f.engine), f.next)
	hook.Register(f.engine)
	// 预热全部已部署定义（锁外，接线约定）
	if err := hook.WarmupAll(context.Background()); err != nil {
		t.Fatalf("预热失败: %v", err)
	}
	return hook
}

// TestCompleteToTargetFullChain 验证直送全链路：
// 门控变量触发执行人重写（预存记录回填目标节点执行人）、直送记录落库、TZ 意见、
// 操作记录(isAgree=1)、当前待办删除、目标节点新待办生成、表单状态机刷新。
// @param t 测试上下文
func TestCompleteToTargetFullChain(t *testing.T) {
	fx := newDirectFixture(t)
	ctx := context.Background()
	// 注册执行人重写钩子（验证目标节点执行人回填）
	registerRewriteHook(t, fx)
	instance, apply := startBack(t, fx, "flowComplete", "alice")
	completeBack(t, fx, apply.ID, "alice")
	handle := fx.firstBackTask(t, instance.ID)
	// 播种 handle 待办（直送后应被删除）
	if _, err := fx.messages.CreateForTask(ctx, &MessageExtend{
		BaseColumns:       BaseColumns{TenantID: "dev"},
		TaskID:            handle.ID,
		ProcessInstanceID: instance.ID,
		TaskStepCode:      "handle",
		OwnerID:           "alice",
	}); err != nil {
		t.Fatal(err)
	}
	// 直送：handle → apply，目标执行人预存 bob（经重写钩子回填）
	err := fx.direct.CompleteToTargetActivity(ctx, &api.CompleteTaskVo{
		BaseProcessVo:    api.BaseProcessVo{TaskID: handle.ID, UserCode: "alice", Message: "直送回申请节点"},
		TargetActivityID: "apply",
		Request: &api.NextActivityRequest{
			TaskID:         handle.ID,
			SendIdentityID: "alice",
			SendUserName:   "张三",
			NextActivityList: []*api.NextActivityVo{
				{ActivityID: "apply", ExecutorIDs: []string{"bob"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("直送失败: %v", err)
	}
	// 引擎落位：apply 新任务在办
	newApply := fx.firstBackTask(t, instance.ID)
	if newApply.TaskDefinitionKey != "apply" {
		t.Fatalf("直送后应落位 apply: %+v", newApply)
	}
	// 门控执行人重写：目标节点执行人被预存记录回填为 bob（非模型默认 alice）
	if newApply.Assignee != "bob" {
		t.Fatalf("执行人重写应回填 bob，实际 %q", newApply.Assignee)
	}
	// 预存记录被消耗（used=1 并回写任务 ID）
	remaining, err := fx.next.ListUnusedByInstanceAndActivity(ctx, instance.ID, "apply")
	if err != nil || len(remaining) != 0 {
		t.Fatalf("预存记录应被钩子消耗: %v %+v", err, remaining)
	}
	// 直送记录落库（flowId 空、source=handle、target=apply）
	records, err := fx.completeTo.ListByTask(ctx, handle.ID)
	if err != nil || len(records) != 1 {
		t.Fatalf("直送记录应落库: %v %v", records, err)
	}
	if records[0].SourceActivityID != "handle" || records[0].TargetActivityID != "apply" || records[0].FlowID != "" {
		t.Fatalf("直送记录字段不符: %+v", records[0])
	}
	// 门控变量：实例变量 operationType=completeToTarget
	operationType, err := fx.engine.RuntimeService().GetVariable(ctx, instance.ID, VarOperationType)
	if err != nil || operationType != OperationTypeCompleteToTarget {
		t.Fatalf("operationType 变量应为 completeToTarget: %v %v", operationType, err)
	}
	// 操作记录 isAgree=1 且类型 TZ
	opRecords, err := fx.operations.ListByInstance(ctx, instance.ID)
	if err != nil || len(opRecords) == 0 {
		t.Fatalf("操作记录缺失: %v %v", opRecords, err)
	}
	last := opRecords[len(opRecords)-1]
	if last.IsAgree == nil || *last.IsAgree != 1 || last.Type != CommentTypeTZ {
		t.Fatalf("操作记录应为 isAgree=1/TZ: %+v", last)
	}
	// 当前待办删除 + 目标节点新待办生成
	if _, found, _ := fx.messages.GetByTaskID(ctx, handle.ID); found {
		t.Fatalf("handle 待办应被删除")
	}
	if _, found, err := fx.messages.GetByTaskID(ctx, newApply.ID); err != nil || !found {
		t.Fatalf("apply 新任务待办应生成: %v %v", found, err)
	}
	// 直送后重走全流程（apply → handle → end）
	completeBack(t, fx, newApply.ID, "bob")
	handle2 := fx.firstBackTask(t, instance.ID)
	completeBack(t, fx, handle2.ID, "alice")
	got, err := fx.engine.RuntimeService().GetProcessInstance(ctx, instance.ID)
	if err != nil || !got.Ended {
		t.Fatalf("直送后重走应正常结束: %v %+v", err, got)
	}
}

// TestCompleteToTargetEnd 验证直送到结束节点：实例结束、直送记录落库、表单状态机置终态。
// @param t 测试上下文
func TestCompleteToTargetEnd(t *testing.T) {
	fx := newDirectFixture(t)
	ctx := context.Background()
	instance, apply := startBack(t, fx, "flowComplete", "alice")
	// 播种表单状态（未提交），直送到 end 后应置已完成
	if _, err := fx.documents.Save(ctx, DocumentSaveRequest{Document: &Document{
		BaseColumns:    BaseColumns{TenantID: "dev"},
		BusinessID:     instance.BusinessKey,
		FlowInstanceID: instance.ID,
	}}); err != nil {
		t.Fatal(err)
	}
	completeBack(t, fx, apply.ID, "alice")
	handle := fx.firstBackTask(t, instance.ID)
	// 直送到 end：request 首条为 end 保留代号 → 跳过预存
	err := fx.direct.CompleteToTargetActivity(ctx, &api.CompleteTaskVo{
		BaseProcessVo:    api.BaseProcessVo{TaskID: handle.ID, UserCode: "alice", Message: "直送结束"},
		TargetActivityID: "end",
		Request: &api.NextActivityRequest{
			TaskID:           handle.ID,
			SendIdentityID:   "alice",
			NextActivityList: []*api.NextActivityVo{{ActivityID: StepCodeEnd}},
		},
	})
	if err != nil {
		t.Fatalf("直送到 end 失败: %v", err)
	}
	// 实例结束
	got, err := fx.engine.RuntimeService().GetProcessInstance(ctx, instance.ID)
	if err != nil || !got.Ended {
		t.Fatalf("直送到 end 后实例应结束: %v %+v", err, got)
	}
	// 直送记录落库（target=end）
	records, err := fx.completeTo.ListByTask(ctx, handle.ID)
	if err != nil || len(records) != 1 || records[0].TargetActivityID != "end" {
		t.Fatalf("直送记录应落库: %v %v", records, err)
	}
	// 表单状态机置终态
	document, found, err := fx.documents.GetByBusinessID(ctx, instance.BusinessKey, "dev")
	if err != nil || !found || document.FlowStatus != FlowStatusFinished {
		t.Fatalf("表单状态应置已完成: %v %v %+v", found, err, document)
	}
}

// TestCompleteToTargetGuards 验证直送校验链：入参缺失、请求缺失、任务缺失、
// 目标未设置执行者、实例挂起。
// @param t 测试上下文
func TestCompleteToTargetGuards(t *testing.T) {
	fx := newDirectFixture(t)
	ctx := context.Background()
	instance, _ := startBack(t, fx, "directNoExecutor", "alice")
	// 入参校验
	if err := fx.direct.CompleteToTargetActivity(ctx, nil); err == nil || !containsText(err, "直送请求为空") {
		t.Fatalf("空请求应报错: %v", err)
	}
	if err := fx.direct.CompleteToTargetActivity(ctx, &api.CompleteTaskVo{}); err == nil || !containsText(err, "taskId 为空") {
		t.Fatalf("taskId 缺失应报错: %v", err)
	}
	if err := fx.direct.CompleteToTargetActivity(ctx, &api.CompleteTaskVo{BaseProcessVo: api.BaseProcessVo{TaskID: "x", UserCode: "alice"}}); err == nil || !containsText(err, "targetActivityId 为空") {
		t.Fatalf("目标缺失应报错: %v", err)
	}
	if err := fx.direct.CompleteToTargetActivity(ctx, &api.CompleteTaskVo{BaseProcessVo: api.BaseProcessVo{TaskID: "x"}, TargetActivityID: "d2"}); err == nil || !containsText(err, "userCode 为空") {
		t.Fatalf("userCode 缺失应报错: %v", err)
	}
	// 任务不存在
	if err := fx.direct.CompleteToTargetActivity(ctx, &api.CompleteTaskVo{
		BaseProcessVo:    api.BaseProcessVo{TaskID: "no-such", UserCode: "alice"},
		TargetActivityID: "d2",
	}); err == nil || !containsText(err, "任务不存在") {
		t.Fatalf("任务不存在应报错: %v", err)
	}
	// request 缺失 → "参数 nextActivityList 为空"
	d1 := fx.firstBackTask(t, instance.ID)
	if err := fx.direct.CompleteToTargetActivity(ctx, &api.CompleteTaskVo{
		BaseProcessVo:    api.BaseProcessVo{TaskID: d1.ID, UserCode: "alice"},
		TargetActivityID: "d2",
	}); err == nil || !containsText(err, "nextActivityList 为空") {
		t.Fatalf("request 缺失应报错: %v", err)
	}
	// 目标节点未设置执行者（d2 无 assignee/候选配置）
	if err := fx.direct.CompleteToTargetActivity(ctx, &api.CompleteTaskVo{
		BaseProcessVo:    api.BaseProcessVo{TaskID: d1.ID, UserCode: "alice"},
		TargetActivityID: "d2",
		Request: &api.NextActivityRequest{
			TaskID:           d1.ID,
			NextActivityList: []*api.NextActivityVo{{ActivityID: "d2", ExecutorIDs: []string{"bob"}}},
		},
	}); err == nil || !containsText(err, "步骤未设置执行者") {
		t.Fatalf("目标未设置执行者应报错: %v", err)
	}
	// 实例挂起
	if err := fx.engine.RuntimeService().SuspendProcessInstance(ctx, instance.ID); err != nil {
		t.Fatal(err)
	}
	if err := fx.direct.CompleteToTargetActivity(ctx, &api.CompleteTaskVo{
		BaseProcessVo:    api.BaseProcessVo{TaskID: d1.ID, UserCode: "alice"},
		TargetActivityID: "d1",
		Request: &api.NextActivityRequest{
			TaskID:           d1.ID,
			NextActivityList: []*api.NextActivityVo{{ActivityID: "d1", ExecutorIDs: []string{"alice"}}},
		},
	}); err == nil || !containsText(err, "流程已暂停") {
		t.Fatalf("挂起实例应拒绝直送: %v", err)
	}
}

// TestCompleteToTargetLedgerWithoutHook 验证直送账本链路的执行人重写未接线形态：
// 预存记录未被消耗（保留 used=0 供追溯）、新待办发送人信息联查预存记录、
// 当前节点预存记录按（实例+节点+任务）清理、空意见跳过意见写入。
// @param t 测试上下文
func TestCompleteToTargetLedgerWithoutHook(t *testing.T) {
	fx := newDirectFixture(t)
	ctx := context.Background()
	instance, apply := startBack(t, fx, "flowComplete", "alice")
	completeBack(t, fx, apply.ID, "alice")
	handle := fx.firstBackTask(t, instance.ID)
	// 播种当前任务的预存记录：当前节点一条（应清理）+ 其他节点一条（应保留）
	if err := fx.next.InsertDual(ctx, &NextActivity{
		BaseColumns:       BaseColumns{TenantID: "dev"},
		NextActivityID:    "handle",
		IdentityID:        "alice",
		ProcessInstanceID: instance.ID,
		Used:              1,
		TaskID:            handle.ID,
		Version:           1,
	}, "alice"); err != nil {
		t.Fatal(err)
	}
	if err := fx.next.InsertDual(ctx, &NextActivity{
		BaseColumns:       BaseColumns{TenantID: "dev"},
		NextActivityID:    "other",
		IdentityID:        "alice",
		ProcessInstanceID: instance.ID,
		Used:              1,
		TaskID:            handle.ID,
		Version:           1,
	}, "alice"); err != nil {
		t.Fatal(err)
	}
	// 空意见直送（跳过意见写入）；无重写钩子 → 目标任务保留模型默认执行人
	err := fx.direct.CompleteToTargetActivity(ctx, &api.CompleteTaskVo{
		BaseProcessVo:    api.BaseProcessVo{TaskID: handle.ID, UserCode: "alice"},
		TargetActivityID: "apply",
		Request: &api.NextActivityRequest{
			TaskID:         handle.ID,
			SendIdentityID: "alice",
			NextActivityList: []*api.NextActivityVo{
				{ActivityID: "apply", ExecutorIDs: []string{"bob"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("直送失败: %v", err)
	}
	newApply := fx.firstBackTask(t, instance.ID)
	if newApply.TaskDefinitionKey != "apply" {
		t.Fatalf("直送后应落位 apply: %+v", newApply)
	}
	// 无钩子：模型默认执行人（initiator）保留
	if newApply.Assignee != "alice" {
		t.Fatalf("无重写钩子应保留模型默认执行人 alice，实际 %q", newApply.Assignee)
	}
	// 当前节点预存记录被清理，其他节点记录保留
	records, err := fx.next.ListByTaskID(ctx, handle.ID)
	if err != nil || len(records) != 1 || records[0].NextActivityID != "other" {
		t.Fatalf("当前节点预存应清理、其他节点保留: %v %+v", err, records)
	}
	// 意见未写入（空意见跳过）
	comments, err := fx.comments.ListByProcessInstance(ctx, instance.ID)
	if err != nil || len(comments) != 0 {
		t.Fatalf("空意见不应写意见: %v %v", comments, err)
	}
	// 新待办发送人信息联查未消耗预存记录（发送人=预存 send_identity_id，发送步骤=来源任务）
	message, found, err := fx.messages.GetByTaskID(ctx, newApply.ID)
	if err != nil || !found {
		t.Fatalf("新待办应生成: %v %v", found, err)
	}
	if message.SenderID != "alice" || message.SenderStepCode != "handle" {
		t.Fatalf("待办发送人信息应联查预存记录: %+v", message)
	}
	// 预存记录未被消耗（used=0 保留）
	remaining, err := fx.next.ListUnusedByInstanceAndActivity(ctx, instance.ID, "apply")
	if err != nil || len(remaining) != 1 {
		t.Fatalf("无钩子时预存记录应保留: %v %+v", err, remaining)
	}
}

// TestDirectServiceHelpers 验证直送服务辅助分支：nil 身份查询保留 Id、身份查询失败回退、
// 实例结束判定、nil 时钟构造。
// @param t 测试上下文
func TestDirectServiceHelpers(t *testing.T) {
	fx := newDirectFixture(t)
	ctx := context.Background()
	// nil 身份查询：保留身份 Id
	noUsers := NewDirectService(fx.engine, NewModelMetaService(fx.engine), fx.next, fx.comments, fx.operations, fx.messages, fx.histories, fx.documents, fx.completeTo, nil)
	if name := noUsers.displayName(ctx, "alice"); name != "alice" {
		t.Fatalf("无身份查询应保留身份 Id: %s", name)
	}
	// 身份查询失败：回退身份 Id（查询不存在的用户）
	if name := fx.direct.displayName(ctx, "no-such-user"); name != "no-such-user" {
		t.Fatalf("身份查询失败应回退 Id: %s", name)
	}
	// 空身份 Id：直接返回空串
	if name := fx.direct.displayName(ctx, ""); name != "" {
		t.Fatalf("空身份应返回空串: %s", name)
	}
	// 实例结束判定：运行中实例返回 false，结束实例返回 true
	instance, apply := startBack(t, fx, "flowComplete", "alice")
	if fx.direct.isInstanceFinished(ctx, instance.ID) {
		t.Fatalf("运行中实例不应判定为结束")
	}
	completeBack(t, fx, apply.ID, "alice")
	handle := fx.firstBackTask(t, instance.ID)
	// 直送到 end 使实例结束
	if err := fx.direct.CompleteToTargetActivity(ctx, &api.CompleteTaskVo{
		BaseProcessVo:    api.BaseProcessVo{TaskID: handle.ID, UserCode: "alice", Message: "结束"},
		TargetActivityID: "end",
		Request: &api.NextActivityRequest{
			TaskID:           handle.ID,
			NextActivityList: []*api.NextActivityVo{{ActivityID: StepCodeEnd}},
		},
	}); err != nil {
		t.Fatalf("直送到 end 失败: %v", err)
	}
	if !fx.direct.isInstanceFinished(ctx, instance.ID) {
		t.Fatalf("已结束实例应判定为结束")
	}
	// 空实例 Id：按未结束处理
	if fx.direct.isInstanceFinished(ctx, "") {
		t.Fatalf("空实例 Id 应按未结束处理")
	}
	// 不存在的实例（查询失败）：按已结束处理
	if !fx.direct.isInstanceFinished(ctx, "ghost-instance") {
		t.Fatalf("不存在的实例应按已结束处理")
	}
}
