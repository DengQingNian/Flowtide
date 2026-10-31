// reset_test.go 流程重置门面服务测试（P2 波2-B，内存引擎 + MemStores）。
// 全链路：发起 → 办结A/B → 重置 → 回首任务且执行人重建（used=1 预生成）→
// 已办清空重建 → 待办清理重建 → LCCZ 记录 → document 刷新 → 重走全流程可达终点；
// 负向：入参缺失、实例不存在、已结束（无任务）、挂起、首步未设置执行者、组无成员。
package workflow

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
	"github.com/DengQingNian/Flowtide/internal/engine"
	"github.com/DengQingNian/Flowtide/internal/idgen"
)

// rstFixtureTime 是重置域测试的固定时钟基准。
var rstFixtureTime = time.Date(2026, 10, 31, 11, 0, 0, 0, time.UTC)

// rstFlowProcess 是重置主语料：A → B → C 线性三节点（办理人均发起人）。
const rstFlowProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="rstFlow" name="重置测试流程" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="A"/>
    <userTask id="A" name="节点甲" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f1" sourceRef="A" targetRef="B"/>
    <userTask id="B" name="节点乙" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f2" sourceRef="B" targetRef="C"/>
    <userTask id="C" name="节点丙" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f3" sourceRef="C" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// rstNoExecutorProcess 是首步未设置执行者语料：NE（无任何执行者配置）→ B → end。
const rstNoExecutorProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="rstNoExecutor" name="重置无执行者流程" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="NE"/>
    <userTask id="NE" name="无执行者节点"/>
    <sequenceFlow id="f1" sourceRef="NE" targetRef="B"/>
    <userTask id="B" name="节点乙" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f2" sourceRef="B" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// rstGroupEmptyProcess 是候选组无成员语料：GE（candidateGroups=grpEmpty）→ B → end。
const rstGroupEmptyProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="rstGroupEmpty" name="重置空组流程" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="GE"/>
    <userTask id="GE" name="空组节点" flowable:candidateGroups="grpEmpty"/>
    <sequenceFlow id="f1" sourceRef="GE" targetRef="B"/>
    <userTask id="B" name="节点乙" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f2" sourceRef="B" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// rstFixture 是重置域测试环境（内存引擎 + MemStores + 重置域服务全套依赖）。
type rstFixture struct {
	engine     api.Engine                  // 内存引擎
	stores     *MemStores                  // 五大域内存存储
	completeTo *MemCompleteToActivityStore // 直送记录内存存储
	meta       *ModelMetaService           // 模型元数据
	next       *NextActivityService        // 下一步预生成
	comments   *CommentService             // 意见
	operations *OperationRecordService     // 操作记录
	messages   *MessageExtendService       // 待办扩展
	histories  *HistoryExtendService       // 已办扩展
	documents  *DocumentService            // 表单状态机
	reset      *ResetService               // 重置域服务
	fakeClock  *clock.Fake                 // 假时钟
}

// newResetFixture 构造重置域测试环境。
// @param t 测试上下文
// @return 测试环境
func newResetFixture(t *testing.T) *rstFixture {
	t.Helper()
	fake := clock.NewFake(rstFixtureTime)
	eng, err := engine.New(engine.Config{
		HistoryLevel: engine.HistoryAudit,
		Clock:        fake,
		IDs:          idgen.NewUUIDv7(),
		TenantID:     "dev",
	})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close(context.Background()) })
	if _, err := eng.RepositoryService().Deploy(context.Background(), api.DeploymentRequest{
		Name:     "reset-test",
		TenantID: "dev",
		Resources: map[string][]byte{
			"rst.bpmn20.xml":       []byte(rstFlowProcess),
			"rstnoexec.bpmn20.xml": []byte(rstNoExecutorProcess),
			"rstgroup.bpmn20.xml":  []byte(rstGroupEmptyProcess),
		},
	}); err != nil {
		t.Fatalf("部署失败: %v", err)
	}
	if _, err := eng.IdentityService().CreateUser(context.Background(), api.User{ID: "carl", DisplayName: "王五"}); err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}
	fx := &rstFixture{engine: eng, fakeClock: fake}
	fx.stores = NewMemStores(fake)
	fx.completeTo = NewMemCompleteToActivityStore(fake)
	fx.meta = NewModelMetaService(eng)
	fx.next = NewNextActivityService(fx.stores.Next, fake)
	fx.comments = NewCommentService(eng.TaskService(), eng.HistoryService(), eng.IdentityService())
	fx.operations = NewOperationRecordService(fx.stores.Operation, eng.HistoryService(), engineInstanceQuery{runtime: eng.RuntimeService(), history: eng.HistoryService()}, eng.RepositoryService(), eng.IdentityService(), fake)
	fx.messages = NewMessageExtendService(fx.stores.Message, fake)
	fx.histories = NewHistoryExtendService(fx.stores.History, fake)
	fx.documents = NewDocumentService(fx.stores.Document, fx.stores.Message, fx.stores.Operation, engineInstanceQuery{runtime: eng.RuntimeService(), history: eng.HistoryService()}, eng.HistoryService(), fake)
	fx.reset = NewResetService(eng, fx.meta, fx.next, fx.comments, fx.operations, fx.messages, fx.histories, fx.documents, fake)
	return fx
}

// rstStart 发起流程实例并返回实例与首个任务。
// @param t 测试上下文
// @param f 测试环境
// @param definitionKey 流程定义 Key
// @param businessKey 业务单据 Id
// @return 流程实例
// @return 首个任务
func rstStart(t *testing.T, f *rstFixture, definitionKey, businessKey string) (api.ProcessInstance, api.Task) {
	t.Helper()
	instance, err := f.engine.RuntimeService().StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: definitionKey,
		BusinessKey:          businessKey,
		Name:                 "标题-" + businessKey,
		TenantID:             "dev",
		UserID:               "alice",
		Variables:            []api.Variable{{Name: VarInitiator, Value: "alice"}},
	})
	if err != nil {
		t.Fatalf("发起失败: %v", err)
	}
	return instance, rstFirstTask(t, f, instance.ID)
}

// rstFirstTask 返回实例当前首个任务。
// @param t 测试上下文
// @param f 测试环境
// @param instanceID 实例 Id
// @return 任务
func rstFirstTask(t *testing.T, f *rstFixture, instanceID string) api.Task {
	t.Helper()
	page, err := f.engine.TaskService().ListTasks(context.Background(), api.TaskFilter{ProcessInstanceID: instanceID, Size: 10})
	if err != nil {
		t.Fatalf("查询任务失败: %v", err)
	}
	if len(page.Data) == 0 {
		t.Fatalf("实例 %s 无任务", instanceID)
	}
	return page.Data[0]
}

// rstComplete 推进时钟后完成任务（引擎直驱）。
// @param t 测试上下文
// @param f 测试环境
// @param taskID 任务 Id
// @param userID 完成人
func rstComplete(t *testing.T, f *rstFixture, taskID, userID string) {
	t.Helper()
	f.fakeClock.Advance(time.Minute)
	if err := f.engine.TaskService().Complete(context.Background(), taskID, api.CompleteTaskRequest{UserID: userID}); err != nil {
		t.Fatalf("完成任务失败: %v", err)
	}
}

// rstSeedInstanceData 播种实例扩展数据：首任务待办/已办镜像 + 首步已消耗预存 + TJ 操作记录 + 表单状态。
// @param t 测试上下文
// @param f 测试环境
// @param instance 流程实例
// @param task 首个任务
func rstSeedInstanceData(t *testing.T, f *rstFixture, instance api.ProcessInstance, task api.Task) {
	t.Helper()
	ctx := context.Background()
	message := &MessageExtend{
		BaseColumns:       BaseColumns{TenantID: instance.TenantID},
		TaskID:            task.ID,
		ProcessInstanceID: instance.ID,
		TaskStepCode:      task.TaskDefinitionKey,
		OwnerID:           task.Assignee,
		BusinessID:        instance.BusinessKey,
	}
	if _, err := f.messages.CreateForTask(ctx, message); err != nil {
		t.Fatalf("播种待办失败: %v", err)
	}
	if _, err := f.histories.CreateForTask(ctx, &HistoryExtend{MessageExtend: *message}); err != nil {
		t.Fatalf("播种已办镜像失败: %v", err)
	}
	if err := f.next.InsertDual(ctx, &NextActivity{
		BaseColumns:       BaseColumns{TenantID: instance.TenantID},
		NextActivityID:    task.TaskDefinitionKey,
		IdentityID:        task.Assignee,
		ProcessInstanceID: instance.ID,
		Used:              1,
		TaskID:            task.ID,
		Version:           1,
	}, "alice"); err != nil {
		t.Fatalf("播种首步预存失败: %v", err)
	}
	if _, err := f.operations.Save(ctx, OperationRequest{
		TaskID:            task.ID,
		ProcessInstanceID: instance.ID,
		OwnerID:           instance.StartUserID,
		Message:           "发起申请",
		Type:              CommentTypeTJ,
	}); err != nil {
		t.Fatalf("播种发起操作记录失败: %v", err)
	}
	if _, err := f.documents.Save(ctx, DocumentSaveRequest{Document: &Document{
		BaseColumns:    BaseColumns{TenantID: instance.TenantID},
		BusinessID:     instance.BusinessKey,
		FlowInstanceID: instance.ID,
		FlowTitle:      instance.Name,
	}}); err != nil {
		t.Fatalf("播种表单状态失败: %v", err)
	}
}

// TestResetFullChain 验证重置全链路：回首任务、执行人重建、已办清空重建、
// 待办清理重建、LCCZ 记录、document 刷新、重走全流程可达终点。
// @param t 测试上下文
func TestResetFullChain(t *testing.T) {
	fx := newResetFixture(t)
	ctx := context.Background()
	instance, taskA := rstStart(t, fx, "rstFlow", "biz-reset")
	rstSeedInstanceData(t, fx, instance, taskA)
	// 推进到 C（办结 A、B）
	rstComplete(t, fx, taskA.ID, "alice")
	taskB := rstFirstTask(t, fx, instance.ID)
	rstComplete(t, fx, taskB.ID, "alice")
	taskC := rstFirstTask(t, fx, instance.ID)
	if taskC.TaskDefinitionKey != "C" {
		t.Fatalf("应推进到 C: %+v", taskC)
	}
	// 播种 C 任务扩展数据与 B 来源预存记录（清理断言数据源）
	message := &MessageExtend{
		BaseColumns:       BaseColumns{TenantID: instance.TenantID},
		TaskID:            taskC.ID,
		ProcessInstanceID: instance.ID,
		TaskStepCode:      "C",
		OwnerID:           "alice",
		BusinessID:        instance.BusinessKey,
	}
	if _, err := fx.messages.CreateForTask(ctx, message); err != nil {
		t.Fatalf("播种 C 待办失败: %v", err)
	}
	if _, err := fx.histories.CreateForTask(ctx, &HistoryExtend{MessageExtend: *message}); err != nil {
		t.Fatalf("播种 C 已办失败: %v", err)
	}
	if err := fx.next.SaveAgreeNext(ctx, NextActivitySaveRequest{
		ProcessInstanceID: instance.ID,
		ActivityID:        "end2",
		IdentityIDs:       []string{"bob"},
		PreTaskID:         taskC.ID,
		TenantID:          instance.TenantID,
		Creator:           "alice",
	}); err != nil {
		t.Fatalf("播种 C 预存失败: %v", err)
	}
	// ── 重置（操作人 carl）──
	if err := fx.reset.Reset(ctx, instance.ID, "carl"); err != nil {
		t.Fatalf("重置失败: %v", err)
	}
	// 回首任务：A 新任务落位，执行人=模型默认（${initiator}=alice）
	newA := rstFirstTask(t, fx, instance.ID)
	if newA.TaskDefinitionKey != "A" || newA.ID == taskA.ID {
		t.Fatalf("重置后应回首 A 新任务: %+v", newA)
	}
	if newA.Assignee != "alice" {
		t.Fatalf("回首任务执行人应为 alice: %+v", newA)
	}
	// 首步执行人重建：A 节点 used=1 预存记录且 taskId=新首任务
	used, _ := fx.stores.Next.ListUsedByInstance(ctx, instance.ID)
	rebuilt := false
	for _, record := range used {
		if record.NextActivityID == "A" && record.TaskID == newA.ID && record.Used == 1 {
			rebuilt = true
		}
	}
	if !rebuilt {
		t.Fatalf("首步执行人预存应重建: %+v", used)
	}
	unusedA, _ := fx.next.ListUnusedByInstanceAndActivity(ctx, instance.ID, "A")
	if len(unusedA) != 0 {
		t.Fatalf("A 节点不应有未消耗记录: %+v", unusedA)
	}
	// 旧预存清理：C 任务来源的 end2 记录与全部历史记录清空
	unusedEnd2, _ := fx.next.ListUnusedByInstanceAndActivity(ctx, instance.ID, "end2")
	if len(unusedEnd2) != 0 {
		t.Fatalf("旧预存记录应清空: %+v", unusedEnd2)
	}
	hisRecords, _ := fx.next.ListHisByInstance(ctx, instance.ID)
	// 历史预存清空后仅剩 rebuildFirstStep 重建写入的首步记录（SaveFirstTaskUser 双写）
	if len(hisRecords) != 1 || hisRecords[0].NextActivityID != "A" || hisRecords[0].Used != 1 {
		t.Fatalf("历史预存应清空并仅剩重建首步记录: %+v", hisRecords)
	}
	// 已办清空重建：仅剩新首任务镜像
	histories, _ := fx.histories.ListByInstance(ctx, instance.ID)
	if len(histories) != 1 || histories[0].TaskID != newA.ID {
		t.Fatalf("已办应清空并重建为仅新首任务: %+v", histories)
	}
	// 待办清理重建：旧 C 待办删除，新 A 待办生成
	if _, found, _ := fx.messages.GetByTaskID(ctx, taskC.ID); found {
		t.Fatalf("旧 C 待办应被清理")
	}
	todo, found, _ := fx.messages.GetByTaskID(ctx, newA.ID)
	if !found || todo.OwnerID != "alice" {
		t.Fatalf("新首任务待办应生成: %v %+v", found, todo)
	}
	// LCCZ 意见与操作记录（isAgree=0，任务列=重置前首任务）
	last, err := fx.operations.ListByInstance(ctx, instance.ID)
	if err != nil || len(last) == 0 {
		t.Fatalf("操作记录缺失: %v %v", last, err)
	}
	tail := last[len(last)-1]
	if tail.Type != CommentTypeLCCZ || tail.OwnerID != "carl" || tail.TaskID != taskC.ID {
		t.Fatalf("重置操作记录应为 LCCZ/carl/C 任务: %+v", tail)
	}
	if tail.IsAgree == nil || *tail.IsAgree != 0 {
		t.Fatalf("重置操作记录应为不同意方向: %+v", tail)
	}
	comments, err := fx.comments.ListByProcessInstance(ctx, instance.ID)
	if err != nil || len(comments) == 0 || comments[len(comments)-1].Message != "流程重置" {
		t.Fatalf("重置意见缺失或文案不符: %v %+v", err, comments)
	}
	// document 刷新：TJ + LCCZ 两条记录 → 流转中
	document, found, _ := fx.documents.GetByInstance(ctx, instance.ID)
	if !found || document.FlowStatus != FlowStatusRunning {
		t.Fatalf("表单状态应刷新为流转中: %v %+v", found, document)
	}
	// ── 重走全流程可达终点 ──
	rstComplete(t, fx, newA.ID, "alice")
	b2 := rstFirstTask(t, fx, instance.ID)
	if b2.TaskDefinitionKey != "B" {
		t.Fatalf("重走应推进到 B: %+v", b2)
	}
	rstComplete(t, fx, b2.ID, "alice")
	c2 := rstFirstTask(t, fx, instance.ID)
	if c2.TaskDefinitionKey != "C" {
		t.Fatalf("重走应推进到 C: %+v", c2)
	}
	rstComplete(t, fx, c2.ID, "alice")
	got, err := fx.engine.RuntimeService().GetProcessInstance(ctx, instance.ID)
	if err != nil || !got.Ended {
		t.Fatalf("重置后重走应正常结束: %v %+v", err, got)
	}
}

// TestResetGuards 验证重置校验链：入参缺失、实例不存在、已结束（无任务）、
// 挂起、首步未设置执行者、候选组无成员。
// @param t 测试上下文
func TestResetGuards(t *testing.T) {
	fx := newResetFixture(t)
	ctx := context.Background()
	instance, taskA := rstStart(t, fx, "rstFlow", "biz-guard")
	// 入参缺失
	if err := fx.reset.Reset(ctx, " ", "carl"); err == nil || !strings.Contains(err.Error(), "processInstanceId") {
		t.Fatalf("空 processInstanceId 应拒绝: %v", err)
	}
	if err := fx.reset.Reset(ctx, instance.ID, " "); err == nil || !strings.Contains(err.Error(), "userCode") {
		t.Fatalf("空 userCode 应拒绝: %v", err)
	}
	// 实例不存在
	if err := fx.reset.Reset(ctx, "pi-missing", "carl"); err == nil || !strings.Contains(err.Error(), "流程不存在") {
		t.Fatalf("实例不存在应拒绝: %v", err)
	}
	// 挂起实例（引擎 ResetProcess 拒绝）
	if err := fx.engine.RuntimeService().SuspendProcessInstance(ctx, instance.ID); err != nil {
		t.Fatalf("挂起失败: %v", err)
	}
	if err := fx.reset.Reset(ctx, instance.ID, "carl"); err == nil || !strings.Contains(err.Error(), "挂起") {
		t.Fatalf("挂起实例应拒绝: %v", err)
	}
	if err := fx.engine.RuntimeService().ActivateProcessInstance(ctx, instance.ID); err != nil {
		t.Fatalf("激活失败: %v", err)
	}
	// 已结束实例（无活动任务 → 任务不存在）
	rstComplete(t, fx, taskA.ID, "alice")
	b := rstFirstTask(t, fx, instance.ID)
	rstComplete(t, fx, b.ID, "alice")
	c := rstFirstTask(t, fx, instance.ID)
	rstComplete(t, fx, c.ID, "alice")
	if err := fx.reset.Reset(ctx, instance.ID, "carl"); err == nil || !strings.Contains(err.Error(), "任务不存在") {
		t.Fatalf("已结束实例应拒绝: %v", err)
	}
	// 首步未设置执行者
	noExec, neTask := rstStart(t, fx, "rstNoExecutor", "biz-noexec")
	rstComplete(t, fx, neTask.ID, "alice")
	if err := fx.reset.Reset(ctx, noExec.ID, "carl"); err == nil || !strings.Contains(err.Error(), "步骤未设置执行者") {
		t.Fatalf("首步未设置执行者应拒绝: %v", err)
	}
	// 候选组无成员
	emptyGroup, geTask := rstStart(t, fx, "rstGroupEmpty", "biz-emptygroup")
	rstComplete(t, fx, geTask.ID, "alice")
	if err := fx.reset.Reset(ctx, emptyGroup.ID, "carl"); err == nil || !strings.Contains(err.Error(), "执行人身份不存在") {
		t.Fatalf("候选组无成员应拒绝: %v", err)
	}
}
