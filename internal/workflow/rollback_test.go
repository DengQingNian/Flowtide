// rollback_test.go 撤回门面服务测试（P2 波2-B，内存引擎 + MemStores）。
// 全链路：发起 → 办结任务A → 撤回A → 任务A重新生成且执行人=撤回人（注册
// ROLLBACK_* 消费钩子——Java RollbackEventListener 的测试等价物——验证回填）→
// CH 记录断言、预存清理断言、待办遗留清理断言、重走全流程可达终点；
// 负向：未办结拒绝、任务不存在、实例归属不符、实例已结束、实例挂起。
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

// rbtFlowProcess 是撤回主语料：A → B → C 线性三节点（办理人均发起人）。
const rbtFlowProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="rbtFlow" name="撤回测试流程" isExecutable="true">
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

// rbtFixtureTime 是撤回测试的固定时钟基准（自持常量，避免跨文件耦合）。
var rbtFixtureTime = time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)

// rbtFixture 是撤回测试环境（内存引擎 + MemStores + 撤回域服务全套依赖）。
type rbtFixture struct {
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
	rollback   *RollbackService            // 撤回域服务
	fakeClock  *clock.Fake                 // 假时钟
}

// newRollbackFixture 构造撤回测试环境（部署语料、创建用户、注册 ROLLBACK_* 消费钩子
// 与执行人重写钩子）。
// @param t 测试上下文
// @return 测试环境
func newRollbackFixture(t *testing.T) *rbtFixture {
	t.Helper()
	fake := clock.NewFake(rbtFixtureTime)
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
		Name:     "rollback-test",
		TenantID: "dev",
		Resources: map[string][]byte{
			"rbt.bpmn20.xml": []byte(rbtFlowProcess),
		},
	}); err != nil {
		t.Fatalf("部署失败: %v", err)
	}
	for _, user := range []api.User{
		{ID: "alice", DisplayName: "张三"},
		{ID: "bob", DisplayName: "李四"},
		{ID: "carol", DisplayName: "王五"},
	} {
		if _, err := eng.IdentityService().CreateUser(context.Background(), user); err != nil {
			t.Fatalf("创建用户 %s 失败: %v", user.ID, err)
		}
	}
	// ROLLBACK_* 消费钩子：任务创建时按 ROLLBACK_ASSIGNEE_PREFIX_ 变量回填办理人
	// （Java RollbackEventListener 的测试等价物，验证引擎撤回变量 → 任务执行人回填链路）
	eng.AddTaskCreatedHook(func(ctx context.Context, task *api.Task, ctl api.HookControl) error {
		key := VarRollbackAssigneePrefix + task.ProcessInstanceID + task.TaskDefinitionKey
		value, ok := task.Variables[key]
		if !ok {
			return nil
		}
		assignee, _ := value.(string)
		return ctl.SetAssignee(task.ID, assignee)
	})
	// 执行人重写钩子（门面同款注册形态）：注册在服务装配之后（依赖预生成域服务），
	// 撤回命令未带门控变量时应静默跳过不干扰
	fx := &rbtFixture{engine: eng, fakeClock: fake}
	fx.stores = NewMemStores(fake)
	fx.completeTo = NewMemCompleteToActivityStore(fake)
	fx.meta = NewModelMetaService(eng)
	fx.next = NewNextActivityService(fx.stores.Next, fake)
	fx.comments = NewCommentService(eng.TaskService(), eng.HistoryService(), eng.IdentityService())
	fx.operations = NewOperationRecordService(fx.stores.Operation, eng.HistoryService(), engineInstanceQuery{runtime: eng.RuntimeService(), history: eng.HistoryService()}, eng.RepositoryService(), eng.IdentityService(), fake)
	fx.messages = NewMessageExtendService(fx.stores.Message, fake)
	fx.histories = NewHistoryExtendService(fx.stores.History, fake)
	fx.documents = NewDocumentService(fx.stores.Document, fx.stores.Message, fx.stores.Operation, engineInstanceQuery{runtime: eng.RuntimeService(), history: eng.HistoryService()}, eng.HistoryService(), fake)
	fx.rollback = NewRollbackService(eng, fx.next, fx.comments, fx.operations, fx.messages, fx.histories, fx.documents, fake)
	NewExecutorRewriteHook(fx.meta, fx.next).Register(eng)
	return fx
}

// rbtStart 发起流程实例并返回实例与首个任务。
// @param t 测试上下文
// @param f 测试环境
// @param businessKey 业务单据 Id
// @param initiator 发起人
// @return 流程实例
// @return 首个任务
func rbtStart(t *testing.T, f *rbtFixture, businessKey, initiator string) (api.ProcessInstance, api.Task) {
	t.Helper()
	instance, err := f.engine.RuntimeService().StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "rbtFlow",
		BusinessKey:          businessKey,
		Name:                 "标题-" + businessKey,
		TenantID:             "dev",
		UserID:               initiator,
		Variables:            []api.Variable{{Name: VarInitiator, Value: initiator}},
	})
	if err != nil {
		t.Fatalf("发起失败: %v", err)
	}
	task := rbtFirstTask(t, f, instance.ID)
	return instance, task
}

// rbtFirstTask 返回实例当前首个任务。
// @param t 测试上下文
// @param f 测试环境
// @param instanceID 实例 Id
// @return 任务
func rbtFirstTask(t *testing.T, f *rbtFixture, instanceID string) api.Task {
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

// rbtComplete 推进时钟后完成任务（引擎直驱，不注入门控变量：
// 避免残留 operationType 触发执行人重写钩子消耗撤回保留的自环预存记录）。
// @param t 测试上下文
// @param f 测试环境
// @param taskID 任务 Id
// @param userID 完成人
func rbtComplete(t *testing.T, f *rbtFixture, taskID, userID string) {
	t.Helper()
	f.fakeClock.Advance(time.Minute)
	if err := f.engine.TaskService().Complete(context.Background(), taskID, api.CompleteTaskRequest{UserID: userID}); err != nil {
		t.Fatalf("完成任务失败: %v", err)
	}
}

// rbtSeedTaskData 播种任务待办 + 已办镜像（供清理断言）。
// @param t 测试上下文
// @param f 测试环境
// @param instance 流程实例
// @param task 任务
func rbtSeedTaskData(t *testing.T, f *rbtFixture, instance api.ProcessInstance, task api.Task) {
	t.Helper()
	message := &MessageExtend{
		BaseColumns:       BaseColumns{TenantID: instance.TenantID},
		TaskID:            task.ID,
		ProcessInstanceID: instance.ID,
		TaskStepCode:      task.TaskDefinitionKey,
		OwnerID:           task.Assignee,
		BusinessID:        instance.BusinessKey,
	}
	if _, err := f.messages.CreateForTask(context.Background(), message); err != nil {
		t.Fatalf("播种待办失败: %v", err)
	}
	if _, err := f.histories.CreateForTask(context.Background(), &HistoryExtend{MessageExtend: *message}); err != nil {
		t.Fatalf("播种已办镜像失败: %v", err)
	}
}

// TestRollbackFullChain 验证撤回全链路：发起 → 办结A → 撤回A → 任务A重新生成且
// 执行人=撤回人 → CH 意见/操作记录 → 预存清理 → 待办遗留清理 → 表单刷新 → 重走可达终点。
// @param t 测试上下文
func TestRollbackFullChain(t *testing.T) {
	fx := newRollbackFixture(t)
	ctx := context.Background()
	instance, taskA := rbtStart(t, fx, "biz-rollback", "alice")
	rbtSeedTaskData(t, fx, instance, taskA)
	// 播种初始表单状态与发起 TJ 操作记录（表单刷新断言数据源，模拟发起链落位）
	if _, err := fx.documents.Save(ctx, DocumentSaveRequest{Document: &Document{
		BaseColumns:    BaseColumns{TenantID: instance.TenantID},
		BusinessID:     instance.BusinessKey,
		FlowInstanceID: instance.ID,
		FlowTitle:      instance.Name,
	}}); err != nil {
		t.Fatalf("播种表单状态失败: %v", err)
	}
	if _, err := fx.operations.Save(ctx, OperationRequest{
		TaskID: taskA.ID, ProcessInstanceID: instance.ID, OwnerID: instance.StartUserID, Message: "发起申请", Type: CommentTypeTJ,
	}); err != nil {
		t.Fatalf("播种发起操作记录失败: %v", err)
	}
	// 办结 A（引擎直驱，执行人重写钩子无预存记录时保留模型默认）
	rbtComplete(t, fx, taskA.ID, "alice")
	taskB := rbtFirstTask(t, fx, instance.ID)
	if taskB.TaskDefinitionKey != "B" {
		t.Fatalf("办结 A 后应落位 B: %+v", taskB)
	}
	rbtSeedTaskData(t, fx, instance, taskB)
	// 播种 A 节点的两条预存记录：C 目标（应被 DeleteForRollback 清理）与 A 自环（应保留）
	if err := fx.next.SaveAgreeNext(ctx, NextActivitySaveRequest{
		ProcessInstanceID: instance.ID,
		ActivityID:        "C",
		IdentityIDs:       []string{"bob"},
		PreTaskID:         taskA.ID,
		TenantID:          "dev",
		Creator:           "alice",
	}); err != nil {
		t.Fatalf("播种 C 预存失败: %v", err)
	}
	if err := fx.next.SaveAgreeNext(ctx, NextActivitySaveRequest{
		ProcessInstanceID: instance.ID,
		ActivityID:        "A",
		IdentityIDs:       []string{"alice"},
		PreTaskID:         taskA.ID,
		TenantID:          "dev",
		Creator:           "alice",
	}); err != nil {
		t.Fatalf("播种 A 自环预存失败: %v", err)
	}
	// ── 撤回 A（操作人 carol）──
	if err := fx.rollback.Rollback(ctx, instance.ID, taskA.ID, "carol"); err != nil {
		t.Fatalf("撤回失败: %v", err)
	}
	// 任务 A 重新生成且执行人=撤回人（ROLLBACK_* 变量 → 钩子回填）
	newA := rbtFirstTask(t, fx, instance.ID)
	if newA.TaskDefinitionKey != "A" || newA.ID == taskA.ID {
		t.Fatalf("撤回后应落位 A 新任务: %+v", newA)
	}
	if newA.Assignee != "carol" {
		t.Fatalf("撤回任务执行人应为撤回人 carol: %+v", newA)
	}
	// 引擎 ROLLBACK_* 变量落位（新执行局部变量经任务快照可读）
	if got, ok := newA.Variables[VarRollbackAssigneePrefix+instance.ID+"A"]; !ok || got != "carol" {
		t.Fatalf("ROLLBACK_ASSIGNEE 变量未落位: %v", newA.Variables)
	}
	if got, ok := newA.Variables[VarRollbackTaskTypePrefix+instance.ID+"A"]; !ok || got != "NextDefaultUserTaskRollbackOperateStrategy" {
		t.Fatalf("ROLLBACK_TASK_TYPE 变量未落位: %v", newA.Variables)
	}
	// 旧 A/B 任务历史被引擎删除
	if _, err := fx.engine.HistoryService().GetHistoricTaskInstance(ctx, taskA.ID); err == nil {
		t.Fatalf("旧 A 历史任务应删除")
	}
	if _, err := fx.engine.HistoryService().GetHistoricTaskInstance(ctx, taskB.ID); err == nil {
		t.Fatalf("后继 B 历史任务应删除")
	}
	// CH 意见（流程级）与操作记录断言
	comments, err := fx.comments.ListByProcessInstance(ctx, instance.ID)
	if err != nil || len(comments) == 0 {
		t.Fatalf("撤回意见缺失: %v %v", comments, err)
	}
	ops, err := fx.operations.ListByInstance(ctx, instance.ID)
	if err != nil || len(ops) == 0 {
		t.Fatalf("操作记录缺失: %v %v", ops, err)
	}
	last := ops[len(ops)-1]
	if last.Type != CommentTypeCH || last.OwnerID != "carol" || last.TaskID != taskA.ID {
		t.Fatalf("撤回操作记录应为 CH/carol/旧任务: %+v", last)
	}
	if last.Message != "任务撤回" {
		t.Fatalf("撤回操作记录文案不符: %+v", last)
	}
	// 预存清理：C 目标记录删除（目标节点之外的记录），A 自环保留（Java 语义）
	cRecords, _ := fx.next.ListUnusedByInstanceAndActivity(ctx, instance.ID, "C")
	if len(cRecords) != 0 {
		t.Fatalf("C 目标预存记录应被清理: %+v", cRecords)
	}
	aRecords, _ := fx.next.ListUnusedByInstanceAndActivity(ctx, instance.ID, "A")
	if len(aRecords) != 1 {
		t.Fatalf("A 自环预存记录应保留 1 条: %+v", aRecords)
	}
	// 待办遗留清理：旧 A/B 待办删除
	if _, found, _ := fx.messages.GetByTaskID(ctx, taskA.ID); found {
		t.Fatalf("旧 A 待办应被清理")
	}
	if _, found, _ := fx.messages.GetByTaskID(ctx, taskB.ID); found {
		t.Fatalf("B 待办应被清理")
	}
	// 表单状态刷新：TJ + CH 两条记录 → 流转中
	document, found, _ := fx.documents.GetByInstance(ctx, instance.ID)
	if !found || document.FlowStatus != FlowStatusRunning {
		t.Fatalf("表单状态应刷新为流转中: %v %+v", found, document)
	}
	// ── 重走全流程可达终点（重做任务后继按模型默认执行人 ${initiator}=alice 办结）──
	rbtComplete(t, fx, newA.ID, "carol")
	taskB2 := rbtFirstTask(t, fx, instance.ID)
	if taskB2.TaskDefinitionKey != "B" {
		t.Fatalf("重走应推进到 B: %+v", taskB2)
	}
	rbtComplete(t, fx, taskB2.ID, taskB2.Assignee)
	taskC := rbtFirstTask(t, fx, instance.ID)
	if taskC.TaskDefinitionKey != "C" {
		t.Fatalf("重走应推进到 C: %+v", taskC)
	}
	rbtComplete(t, fx, taskC.ID, taskC.Assignee)
	got, err := fx.engine.RuntimeService().GetProcessInstance(ctx, instance.ID)
	if err != nil || !got.Ended {
		t.Fatalf("撤回后重走应正常结束: %v %+v", err, got)
	}
}

// TestRollbackGuards 验证撤回校验链：入参缺失、任务不存在、未办结拒绝、
// 实例归属不符、实例已结束、实例挂起。
// @param t 测试上下文
func TestRollbackGuards(t *testing.T) {
	fx := newRollbackFixture(t)
	ctx := context.Background()
	instance, taskA := rbtStart(t, fx, "biz-guard", "alice")
	// 入参缺失
	if err := fx.rollback.Rollback(ctx, instance.ID, " ", "carol"); err == nil || !strings.Contains(err.Error(), "taskId") {
		t.Fatalf("空 taskId 应拒绝: %v", err)
	}
	if err := fx.rollback.Rollback(ctx, instance.ID, taskA.ID, " "); err == nil || !strings.Contains(err.Error(), "userCode") {
		t.Fatalf("空 userCode 应拒绝: %v", err)
	}
	// 任务不存在
	if err := fx.rollback.Rollback(ctx, instance.ID, "missing", "carol"); err == nil || !strings.Contains(err.Error(), "撤回任务不存在") {
		t.Fatalf("任务不存在应拒绝: %v", err)
	}
	// 未办结拒绝（正在执行的任务）
	if err := fx.rollback.Rollback(ctx, instance.ID, taskA.ID, "carol"); err == nil || !strings.Contains(err.Error(), "正在执行") {
		t.Fatalf("未办结任务应拒绝: %v", err)
	}
	// 实例归属不符
	if err := fx.rollback.Rollback(ctx, "pi-other", taskA.ID, "carol"); err == nil || !strings.Contains(err.Error(), "不属于") {
		t.Fatalf("实例归属不符应拒绝: %v", err)
	}
	// 实例挂起拒绝
	if err := fx.engine.RuntimeService().SuspendProcessInstance(ctx, instance.ID); err != nil {
		t.Fatalf("挂起失败: %v", err)
	}
	if err := fx.rollback.Rollback(ctx, instance.ID, taskA.ID, "carol"); err == nil || !strings.Contains(err.Error(), "已暂停") {
		t.Fatalf("挂起实例应拒绝: %v", err)
	}
	if err := fx.engine.RuntimeService().ActivateProcessInstance(ctx, instance.ID); err != nil {
		t.Fatalf("激活失败: %v", err)
	}
	// 实例已结束拒绝
	rbtComplete(t, fx, taskA.ID, "alice")
	b := rbtFirstTask(t, fx, instance.ID)
	rbtComplete(t, fx, b.ID, "alice")
	c := rbtFirstTask(t, fx, instance.ID)
	rbtComplete(t, fx, c.ID, "alice")
	if err := fx.rollback.Rollback(ctx, instance.ID, b.ID, "carol"); err == nil || !strings.Contains(err.Error(), "已完成") {
		t.Fatalf("已结束实例应拒绝: %v", err)
	}
}
