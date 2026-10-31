// terminate_test.go 终止/删除/挂起激活/重激活门面服务测试（P2 波2-B，内存引擎 + MemStores）。
// 软终止：end 历史存在 / LCZZ 意见与操作记录 / next_activity 双表清空 / 待办已办清空 /
// document 已终止；删除：运行与已结束两路 + 扩展数据全清断言；业务键批量删除；
// 挂起激活：LCZT/LCJH 记录 + next_activity 不动 + 挂起后办结拒绝；
// 重激活：历史变量重建 / BZJH 记录 / document 重建 / 新待办生成。
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

// trmFixtureTime 是终止域测试的固定时钟基准。
var trmFixtureTime = time.Date(2026, 10, 31, 10, 0, 0, 0, time.UTC)

// trmFlowProcess 是终止域主语料：A → B 两节点（办理人均发起人）。
const trmFlowProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="trmFlow" name="终止测试流程" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="A"/>
    <userTask id="A" name="节点甲" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f1" sourceRef="A" targetRef="B"/>
    <userTask id="B" name="节点乙" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f2" sourceRef="B" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// trmFixture 是终止域测试环境（内存引擎 + MemStores + 终止域服务全套依赖）。
type trmFixture struct {
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
	terminate  *TerminateService           // 终止/删除/挂起激活/重激活域服务
	fakeClock  *clock.Fake                 // 假时钟
}

// newTerminateFixture 构造终止域测试环境。
// @param t 测试上下文
// @return 测试环境
func newTerminateFixture(t *testing.T) *trmFixture {
	t.Helper()
	fake := clock.NewFake(trmFixtureTime)
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
		Name:     "terminate-test",
		TenantID: "dev",
		Resources: map[string][]byte{
			"trm.bpmn20.xml": []byte(trmFlowProcess),
		},
	}); err != nil {
		t.Fatalf("部署失败: %v", err)
	}
	if _, err := eng.IdentityService().CreateUser(context.Background(), api.User{ID: "carl", DisplayName: "王五"}); err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}
	fx := &trmFixture{engine: eng, fakeClock: fake}
	fx.stores = NewMemStores(fake)
	fx.completeTo = NewMemCompleteToActivityStore(fake)
	fx.meta = NewModelMetaService(eng)
	fx.next = NewNextActivityService(fx.stores.Next, fake)
	fx.comments = NewCommentService(eng.TaskService(), eng.HistoryService(), eng.IdentityService())
	fx.operations = NewOperationRecordService(fx.stores.Operation, eng.HistoryService(), engineInstanceQuery{runtime: eng.RuntimeService(), history: eng.HistoryService()}, eng.RepositoryService(), eng.IdentityService(), fake)
	fx.messages = NewMessageExtendService(fx.stores.Message, fake)
	fx.histories = NewHistoryExtendService(fx.stores.History, fake)
	fx.documents = NewDocumentService(fx.stores.Document, fx.stores.Message, fx.stores.Operation, engineInstanceQuery{runtime: eng.RuntimeService(), history: eng.HistoryService()}, eng.HistoryService(), fake)
	fx.terminate = NewTerminateService(eng, fx.next, fx.comments, fx.operations, fx.messages, fx.histories, fx.documents, fake, WithTerminateDocumentStore(fx.stores.Document))
	return fx
}

// trmStart 发起流程实例并返回实例与首个任务。
// @param t 测试上下文
// @param f 测试环境
// @param businessKey 业务单据 Id
// @return 流程实例
// @return 首个任务
func trmStart(t *testing.T, f *trmFixture, businessKey string) (api.ProcessInstance, api.Task) {
	t.Helper()
	instance, err := f.engine.RuntimeService().StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "trmFlow",
		BusinessKey:          businessKey,
		Name:                 "标题-" + businessKey,
		TenantID:             "dev",
		UserID:               "alice",
		Variables:            []api.Variable{{Name: VarInitiator, Value: "alice"}},
	})
	if err != nil {
		t.Fatalf("发起失败: %v", err)
	}
	return instance, trmFirstTask(t, f, instance.ID)
}

// trmFirstTask 返回实例当前首个任务。
// @param t 测试上下文
// @param f 测试环境
// @param instanceID 实例 Id
// @return 任务
func trmFirstTask(t *testing.T, f *trmFixture, instanceID string) api.Task {
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

// trmComplete 推进时钟后完成任务（引擎直驱）。
// @param t 测试上下文
// @param f 测试环境
// @param taskID 任务 Id
// @param userID 完成人
func trmComplete(t *testing.T, f *trmFixture, taskID, userID string) {
	t.Helper()
	f.fakeClock.Advance(time.Minute)
	if err := f.engine.TaskService().Complete(context.Background(), taskID, api.CompleteTaskRequest{UserID: userID}); err != nil {
		t.Fatalf("完成任务失败: %v", err)
	}
}

// trmSeedInstanceData 播种实例扩展数据：首任务待办/已办镜像 + B 节点预存记录 + 表单状态。
// @param t 测试上下文
// @param f 测试环境
// @param instance 流程实例
// @param task 首个任务
func trmSeedInstanceData(t *testing.T, f *trmFixture, instance api.ProcessInstance, task api.Task) {
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
	if err := f.next.SaveAgreeNext(ctx, NextActivitySaveRequest{
		ProcessInstanceID: instance.ID,
		ActivityID:        "B",
		IdentityIDs:       []string{"bob"},
		PreTaskID:         task.ID,
		TenantID:          instance.TenantID,
		Creator:           "alice",
	}); err != nil {
		t.Fatalf("播种预存失败: %v", err)
	}
	// 发起 TJ 操作记录（模拟发起链落位，表单状态机计数判据数据源）
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

// trmLastOperation 返回实例最后一条操作记录。
// @param t 测试上下文
// @param f 测试环境
// @param instanceID 实例 Id
// @return 操作记录
func trmLastOperation(t *testing.T, f *trmFixture, instanceID string) *OperationRecord {
	t.Helper()
	ops, err := f.operations.ListByInstance(context.Background(), instanceID)
	if err != nil || len(ops) == 0 {
		t.Fatalf("操作记录缺失: %v %v", ops, err)
	}
	return ops[len(ops)-1]
}

// TestStopByInstanceFullChain 验证软终止全链路：end 历史存在、LCZZ 意见与操作记录、
// next_activity 双表清空、待办已办清空、document 已终止。
// @param t 测试上下文
func TestStopByInstanceFullChain(t *testing.T) {
	fx := newTerminateFixture(t)
	ctx := context.Background()
	instance, taskA := trmStart(t, fx, "biz-stop")
	trmSeedInstanceData(t, fx, instance, taskA)
	// 推进时钟：LCZZ 记录 deal_time 晚于 TJ，最后一条断言排序确定
	fx.fakeClock.Advance(time.Minute)
	if err := fx.terminate.StopByInstance(ctx, instance.ID, "carl", ""); err != nil {
		t.Fatalf("软终止失败: %v", err)
	}
	// 实例已结束
	got, err := fx.engine.RuntimeService().GetProcessInstance(ctx, instance.ID)
	if err != nil || !got.Ended {
		t.Fatalf("软终止后实例应已结束: %v %+v", err, got)
	}
	// end 历史活动存在（软终止走 EndEvent，保留 end 事件历史）
	endActs, err := fx.engine.HistoryService().QueryHistoricActivityInstances(ctx, api.HistoricActivityInstanceFilter{
		ProcessInstanceID: instance.ID,
		ActivityID:        "end",
		Size:              10,
	})
	if err != nil || endActs.Total == 0 {
		t.Fatalf("end 事件历史应存在: %v %+v", err, endActs)
	}
	// LCZZ 操作记录（isAgree 不适用）
	last := trmLastOperation(t, fx, instance.ID)
	if last.Type != CommentTypeLCZZ || last.OwnerID != "carl" || last.TaskID != "" {
		t.Fatalf("终止操作记录应为 LCZZ/carl/流程级: %+v", last)
	}
	if last.Message != "流程终止" {
		t.Fatalf("终止操作记录文案不符: %+v", last)
	}
	// LCZZ 意见（流程级）
	comments, err := fx.comments.ListByProcessInstance(ctx, instance.ID)
	if err != nil || len(comments) == 0 || comments[len(comments)-1].Message != "流程终止" {
		t.Fatalf("终止意见缺失或文案不符: %v %+v", err, comments)
	}
	// next_activity 双表清空
	unused, _ := fx.next.ListUnusedByInstanceAndActivity(ctx, instance.ID, "B")
	if len(unused) != 0 {
		t.Fatalf("运行态预存应清空: %+v", unused)
	}
	hisRecords, _ := fx.next.ListHisByInstance(ctx, instance.ID)
	if len(hisRecords) != 0 {
		t.Fatalf("历史预存应清空: %+v", hisRecords)
	}
	// 待办/已办清空
	if msgs, _ := fx.messages.ListByInstance(ctx, instance.ID); len(msgs) != 0 {
		t.Fatalf("待办应清空: %+v", msgs)
	}
	if his, _ := fx.histories.ListByInstance(ctx, instance.ID); len(his) != 0 {
		t.Fatalf("已办应清空: %+v", his)
	}
	// document 已终止 + 终止时间
	document, found, _ := fx.documents.GetByInstance(ctx, instance.ID)
	if !found || document.FlowStatus != FlowStatusTerminated || document.FlowTerminationTime == nil {
		t.Fatalf("表单状态应已终止: %v %+v", found, document)
	}
}

// TestStopByTaskAndGuards 验证按任务终止与校验链：入参缺失、任务不存在、挂起拒绝、已结束拒绝。
// @param t 测试上下文
func TestStopByTaskAndGuards(t *testing.T) {
	fx := newTerminateFixture(t)
	ctx := context.Background()
	instance, taskA := trmStart(t, fx, "biz-stop-task")
	// 入参缺失
	if err := fx.terminate.StopByTask(ctx, " ", "carl"); err == nil || !strings.Contains(err.Error(), "taskId") {
		t.Fatalf("空 taskId 应拒绝: %v", err)
	}
	if err := fx.terminate.StopByTask(ctx, taskA.ID, " "); err == nil || !strings.Contains(err.Error(), "identityId") {
		t.Fatalf("空 identityId 应拒绝: %v", err)
	}
	// 任务不存在
	if err := fx.terminate.StopByTask(ctx, "missing", "carl"); err == nil || !strings.Contains(err.Error(), "任务不存在") {
		t.Fatalf("任务不存在应拒绝: %v", err)
	}
	// 挂起实例拒绝终止
	if err := fx.engine.RuntimeService().SuspendProcessInstance(ctx, instance.ID); err != nil {
		t.Fatalf("挂起失败: %v", err)
	}
	if err := fx.terminate.StopByTask(ctx, taskA.ID, "carl"); err == nil || !strings.Contains(err.Error(), "已暂停") {
		t.Fatalf("挂起实例按任务终止应拒绝: %v", err)
	}
	if err := fx.terminate.StopByInstance(ctx, instance.ID, "carl", ""); err == nil || !strings.Contains(err.Error(), "已暂停") {
		t.Fatalf("挂起实例按实例终止应拒绝: %v", err)
	}
	if err := fx.engine.RuntimeService().ActivateProcessInstance(ctx, instance.ID); err != nil {
		t.Fatalf("激活失败: %v", err)
	}
	// 按任务终止成功
	if err := fx.terminate.StopByTask(ctx, taskA.ID, "carl"); err != nil {
		t.Fatalf("按任务终止失败: %v", err)
	}
	// 已结束实例再次终止拒绝（引擎已结束校验）
	if err := fx.terminate.StopByInstance(ctx, instance.ID, "carl", ""); err == nil || !strings.Contains(err.Error(), "已结束") {
		t.Fatalf("已结束实例应拒绝终止: %v", err)
	}
	// 按实例终止入参缺失与实例不存在
	if err := fx.terminate.StopByInstance(ctx, "", "carl", ""); err == nil || !strings.Contains(err.Error(), "processInstanceId") {
		t.Fatalf("空 processInstanceId 应拒绝: %v", err)
	}
	if err := fx.terminate.StopByInstance(ctx, "pi-missing", "carl", ""); err == nil || !strings.Contains(err.Error(), "流程实例不存在") {
		t.Fatalf("实例不存在应拒绝: %v", err)
	}
}

// TestDeleteInstanceBothPaths 验证实例删除两路：运行实例级联删除、已结束实例历史删除，
// 扩展数据（next_activity 双表/待办/已办/document）全清断言。
// @param t 测试上下文
func TestDeleteInstanceBothPaths(t *testing.T) {
	fx := newTerminateFixture(t)
	ctx := context.Background()
	// ── 运行实例路径 ──
	instance, taskA := trmStart(t, fx, "biz-del-run")
	trmSeedInstanceData(t, fx, instance, taskA)
	// 推进时钟：LCSC 记录 deal_time 晚于 TJ，最后一条断言排序确定
	fx.fakeClock.Advance(time.Minute)
	if err := fx.terminate.DeleteInstance(ctx, instance.ID, "carl", ""); err != nil {
		t.Fatalf("删除运行实例失败: %v", err)
	}
	// Java cascade=true 语义：运行实例删除后历史一并清除
	if _, err := fx.engine.HistoryService().GetHistoricProcessInstance(ctx, instance.ID); err == nil {
		t.Fatalf("运行实例删除后历史应一并清除（cascade 语义）")
	}
	// LCSC 操作记录
	last := trmLastOperation(t, fx, instance.ID)
	if last.Type != CommentTypeLCSC || last.OwnerID != "carl" {
		t.Fatalf("删除操作记录应为 LCSC/carl: %+v", last)
	}
	// 扩展数据全清
	if msgs, _ := fx.messages.ListByInstance(ctx, instance.ID); len(msgs) != 0 {
		t.Fatalf("待办应清空: %+v", msgs)
	}
	if his, _ := fx.histories.ListByInstance(ctx, instance.ID); len(his) != 0 {
		t.Fatalf("已办应清空: %+v", his)
	}
	unused, _ := fx.next.ListUnusedByInstanceAndActivity(ctx, instance.ID, "B")
	if len(unused) != 0 {
		t.Fatalf("预存应清空: %+v", unused)
	}
	if _, found, _ := fx.documents.GetByBusinessID(ctx, instance.BusinessKey, instance.TenantID); found {
		t.Fatalf("document 应逻辑删除")
	}
	// ── 已结束实例路径（自然走完 → 历史删除）──
	finished, taskF := trmStart(t, fx, "biz-del-done")
	trmComplete(t, fx, taskF.ID, "alice")
	taskB := trmFirstTask(t, fx, finished.ID)
	trmComplete(t, fx, taskB.ID, "alice")
	// 推进时钟：LCSC 记录 deal_time 确定
	fx.fakeClock.Advance(time.Minute)
	if err := fx.terminate.DeleteInstance(ctx, finished.ID, "carl", "清理测试数据"); err != nil {
		t.Fatalf("删除已结束实例失败: %v", err)
	}
	// 历史实例整行删除
	if _, err := fx.engine.HistoryService().GetHistoricProcessInstance(ctx, finished.ID); err == nil {
		t.Fatalf("已结束实例删除后历史应不存在")
	}
	// 自定义删除原因落操作记录
	lastDone := trmLastOperation(t, fx, finished.ID)
	if lastDone.Type != CommentTypeLCSC || lastDone.Message != "流程删除" {
		t.Fatalf("已结束实例删除记录应为 LCSC/流程删除: %+v", lastDone)
	}
	// ── 不存在的实例 ──
	if err := fx.terminate.DeleteInstance(ctx, "pi-missing", "carl", ""); err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Fatalf("实例不存在应拒绝: %v", err)
	}
}

// TestDeleteByBusinessID 验证按业务键批量删除：运行与已结束实例逐一删除、
// 扩展数据按业务键清扫。
// @param t 测试上下文
func TestDeleteByBusinessID(t *testing.T) {
	fx := newTerminateFixture(t)
	ctx := context.Background()
	// 同业务键两实例：一个运行、一个已结束
	running, taskR := trmStart(t, fx, "biz-batch")
	trmSeedInstanceData(t, fx, running, taskR)
	finished, taskF := trmStart(t, fx, "biz-batch")
	trmComplete(t, fx, taskF.ID, "alice")
	taskFB := trmFirstTask(t, fx, finished.ID)
	trmComplete(t, fx, taskFB.ID, "alice")
	// 推进时钟：LCSC 记录 deal_time 晚于 TJ，最后一条断言排序确定
	fx.fakeClock.Advance(time.Minute)
	if err := fx.terminate.DeleteByBusinessID(ctx, "dev", "biz-batch", "carl"); err != nil {
		t.Fatalf("按业务键删除失败: %v", err)
	}
	// 两实例历史均不存在
	if _, err := fx.engine.HistoryService().GetHistoricProcessInstance(ctx, running.ID); err == nil {
		t.Fatalf("运行实例历史应删除")
	}
	if _, err := fx.engine.HistoryService().GetHistoricProcessInstance(ctx, finished.ID); err == nil {
		t.Fatalf("已结束实例历史应删除")
	}
	// 扩展数据按业务键清扫
	if msgs, _ := fx.messages.ListByBusinessID(ctx, "biz-batch", "dev"); len(msgs) != 0 {
		t.Fatalf("待办应按业务键清空: %+v", msgs)
	}
	if his, _ := fx.histories.ListByBusinessID(ctx, "biz-batch", "dev"); len(his) != 0 {
		t.Fatalf("已办应按业务键清空: %+v", his)
	}
	if _, found, _ := fx.documents.GetByBusinessID(ctx, "biz-batch", "dev"); found {
		t.Fatalf("document 应按业务键逻辑删除")
	}
	// 入参校验
	if err := fx.terminate.DeleteByBusinessID(ctx, "dev", "", "carl"); err == nil || !strings.Contains(err.Error(), "businessId") {
		t.Fatalf("空 businessId 应拒绝: %v", err)
	}
	if err := fx.terminate.DeleteByBusinessID(ctx, "dev", "biz-batch", " "); err == nil || !strings.Contains(err.Error(), "identityId") {
		t.Fatalf("空 identityId 应拒绝: %v", err)
	}
}

// TestSuspendActivateChain 验证挂起/激活链：LCZT/LCJH 意见与操作记录、
// next_activity 不动（Java 同语义）、挂起后办结拒绝、实例不存在拒绝。
// @param t 测试上下文
func TestSuspendActivateChain(t *testing.T) {
	fx := newTerminateFixture(t)
	ctx := context.Background()
	instance, taskA := trmStart(t, fx, "biz-suspend")
	trmSeedInstanceData(t, fx, instance, taskA)
	// 挂起（推进时钟：LCZT 记录 deal_time 晚于 TJ，最后一条断言排序确定）
	fx.fakeClock.Advance(time.Minute)
	if err := fx.terminate.Suspend(ctx, instance.ID, "carl"); err != nil {
		t.Fatalf("挂起失败: %v", err)
	}
	got, err := fx.engine.RuntimeService().GetProcessInstance(ctx, instance.ID)
	if err != nil || !got.Suspended {
		t.Fatalf("实例应已挂起: %v %+v", err, got)
	}
	// LCZT 操作记录
	last := trmLastOperation(t, fx, instance.ID)
	if last.Type != CommentTypeLCZT || last.OwnerID != "carl" {
		t.Fatalf("挂起操作记录应为 LCZT/carl: %+v", last)
	}
	// next_activity 不动（Java suspend 不清理预存记录）
	unused, _ := fx.next.ListUnusedByInstanceAndActivity(ctx, instance.ID, "B")
	if len(unused) != 1 {
		t.Fatalf("挂起后预存记录不应被清理: %+v", unused)
	}
	// 挂起后办结拒绝（衔接引擎 complete 校验）
	if err := fx.engine.TaskService().Complete(ctx, taskA.ID, api.CompleteTaskRequest{UserID: "alice"}); err == nil || !strings.Contains(err.Error(), "挂起") {
		t.Fatalf("挂起后办结应拒绝: %v", err)
	}
	// 激活（推进时钟，保证 LCJH 记录的 deal_time 晚于 LCZT，排序断言确定）
	fx.fakeClock.Advance(time.Minute)
	if err := fx.terminate.Activate(ctx, instance.ID, "carl"); err != nil {
		t.Fatalf("激活失败: %v", err)
	}
	got, err = fx.engine.RuntimeService().GetProcessInstance(ctx, instance.ID)
	if err != nil || got.Suspended {
		t.Fatalf("实例应已激活: %v %+v", err, got)
	}
	// LCJH 操作记录
	last = trmLastOperation(t, fx, instance.ID)
	if last.Type != CommentTypeLCJH || last.OwnerID != "carl" {
		t.Fatalf("激活操作记录应为 LCJH/carl: %+v", last)
	}
	// 激活后可正常办结
	trmComplete(t, fx, taskA.ID, "alice")
	// 入参与实例校验
	if err := fx.terminate.Suspend(ctx, "", "carl"); err == nil || !strings.Contains(err.Error(), "processInstanceId") {
		t.Fatalf("空 processInstanceId 应拒绝: %v", err)
	}
	if err := fx.terminate.Suspend(ctx, "pi-missing", "carl"); err == nil || !strings.Contains(err.Error(), "流程实例不存在") {
		t.Fatalf("实例不存在应拒绝: %v", err)
	}
	if err := fx.terminate.Activate(ctx, "pi-missing", "carl"); err == nil || !strings.Contains(err.Error(), "流程实例不存在") {
		t.Fatalf("激活实例不存在应拒绝: %v", err)
	}
}

// TestReactivateChain 验证历史实例重激活：历史变量重建、新任务落位首节点、
// BZJH 意见与操作记录、document 重建、新待办生成、运行中/不存在拒绝。
// @param t 测试上下文
func TestReactivateChain(t *testing.T) {
	fx := newTerminateFixture(t)
	ctx := context.Background()
	instance, taskA := trmStart(t, fx, "biz-reactivate")
	trmSeedInstanceData(t, fx, instance, taskA)
	// 走完全流程至结束
	trmComplete(t, fx, taskA.ID, "alice")
	taskB := trmFirstTask(t, fx, instance.ID)
	trmComplete(t, fx, taskB.ID, "alice")
	// 重激活（回首，发起人重写为操作人 carl；推进时钟保证记录排序确定）
	fx.fakeClock.Advance(time.Minute)
	if err := fx.terminate.Reactivate(ctx, instance.ID, "", "carl"); err != nil {
		t.Fatalf("重激活失败: %v", err)
	}
	// 实例复活且未结束
	got, err := fx.engine.RuntimeService().GetProcessInstance(ctx, instance.ID)
	if err != nil || got.Ended || got.Suspended {
		t.Fatalf("重激活后实例应运行中: %v %+v", err, got)
	}
	// 历史变量重建（发起人变量回到实例）
	if initiator, err := fx.engine.RuntimeService().GetVariable(ctx, instance.ID, VarInitiator); err != nil || initiator != "alice" {
		t.Fatalf("历史变量应重建: %v %v", initiator, err)
	}
	// 新任务落位首节点
	newTask := trmFirstTask(t, fx, instance.ID)
	if newTask.TaskDefinitionKey != "A" {
		t.Fatalf("重激活后应落位首节点 A: %+v", newTask)
	}
	// BZJH 操作记录与意见
	last := trmLastOperation(t, fx, instance.ID)
	if last.Type != CommentTypeBZJH || last.OwnerID != "carl" {
		t.Fatalf("重激活操作记录应为 BZJH/carl: %+v", last)
	}
	comments, err := fx.comments.ListByProcessInstance(ctx, instance.ID)
	if err != nil || len(comments) == 0 || comments[len(comments)-1].Message != "步骤激活" {
		t.Fatalf("重激活意见缺失或文案不符: %v %+v", err, comments)
	}
	// document 重建（upsert）并刷新为流转中
	document, found, _ := fx.documents.GetByBusinessID(ctx, instance.BusinessKey, instance.TenantID)
	if !found || document.FlowStatus != FlowStatusRunning || document.FlowStartTime == nil {
		t.Fatalf("document 应重建为流转中: %v %+v", found, document)
	}
	// 新待办与已办镜像生成
	todo, found, _ := fx.messages.GetByTaskID(ctx, newTask.ID)
	if !found || todo.OwnerID != "alice" {
		t.Fatalf("新任务待办应生成且处理人为 alice: %v %+v", found, todo)
	}
	if _, found, _ := fx.histories.GetByTaskID(ctx, newTask.ID); !found {
		t.Fatalf("新任务已办镜像应生成")
	}
	// 运行中实例重复复活拒绝
	if err := fx.terminate.Reactivate(ctx, instance.ID, "", "carl"); err == nil || !strings.Contains(err.Error(), "仍在运行") {
		t.Fatalf("运行中实例复活应拒绝: %v", err)
	}
	// 不存在的历史实例拒绝
	if err := fx.terminate.Reactivate(ctx, "pi-missing", "", "carl"); err == nil || !strings.Contains(err.Error(), "历史流程实例不存在") {
		t.Fatalf("历史实例不存在应拒绝: %v", err)
	}
}
