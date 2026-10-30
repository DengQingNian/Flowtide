package workflow

import (
	"context"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/bpmn/model"
)

// TestBackNodesFaultStepCodesLate 验证 BackNodes 中 BackStepCodes 的延迟失败分支
// （AllowReturn 首读成功、白名单次读失败）。
// @param t 测试上下文
func TestBackNodesFaultStepCodesLate(t *testing.T) {
	fx := newBackFixture(t)
	ctx := context.Background()
	instance, task := startBack(t, fx, "backFlow3", "alice")
	completeBack(t, fx, task.ID, "alice")
	s2 := fx.firstBackTask(t, instance.ID)
	baseStores := NewMemStores(fx.fakeClock)
	svc := newFaultBackService(fx, &faultEngine{Engine: fx.engine},
		&faultStepStore{StepStore: fx.steps, failGetAfter: 1}, baseStores.Next,
		baseStores.Message, baseStores.History, baseStores.Operation, fx.models)
	if _, err := svc.BackNodes(ctx, s2.ID); err == nil {
		t.Fatalf("白名单次读失败应上抛")
	}
}

// TestParentTaskIDsCycleAndOrder 验证父链收集的防环终止与 returnSort 逆序。
// @param t 测试上下文
func TestParentTaskIDsCycleAndOrder(t *testing.T) {
	fx := newBackFixture(t)
	ctx := context.Background()
	baseStores := NewMemStores(fx.fakeClock)
	svc := NewBackService(fx.engine, NewModelMetaService(fx.engine), NewNextActivityService(baseStores.Next, fx.fakeClock),
		fxComments(fx), fxOperations(fx, baseStores.Operation), NewMessageExtendService(baseStores.Message, fx.fakeClock),
		NewHistoryExtendService(baseStores.History, fx.fakeClock), fxDocuments(fx), fx.stepSvc,
		engineInstanceQuery{runtime: fx.engine.RuntimeService(), history: fx.engine.HistoryService()}, fx.fakeClock)
	// 播种两级链：t3 消耗记录（父 t2）、t2 消耗记录（父 t1）
	seed := func(taskID, preTaskID string) {
		if err := NewNextActivityService(baseStores.Next, fx.fakeClock).InsertDual(ctx, &NextActivity{
			BaseColumns:       BaseColumns{TenantID: "dev"},
			NextActivityID:    "n",
			IdentityID:        "alice",
			ProcessInstanceID: "pi",
			Used:              1,
			PreTaskID:         preTaskID,
			TaskID:            taskID,
			Version:           1,
		}, "alice"); err != nil {
			t.Fatal(err)
		}
	}
	seed("t3", "t2")
	seed("t2", "t1")
	// 升序链：最近父级在前
	chain, err := svc.parentTaskIDs(ctx, "t3", false)
	if err != nil || len(chain) != 2 || chain[0] != "t2" || chain[1] != "t1" {
		t.Fatalf("升序父链错误: %v %v", chain, err)
	}
	// 逆序链（returnSort=true）：最远祖先在前
	chain, err = svc.parentTaskIDs(ctx, "t3", true)
	if err != nil || len(chain) != 2 || chain[0] != "t1" || chain[1] != "t2" {
		t.Fatalf("逆序父链错误: %v %v", chain, err)
	}
	// 环路：t1 ↔ t2 成环，防环终止
	seed("t1", "t2")
	chain, err = svc.parentTaskIDs(ctx, "t3", false)
	if err != nil || len(chain) != 2 {
		t.Fatalf("环路应终止收集: %v %v", chain, err)
	}
}

// TestBackNodeReachableGraphEdges 验证可达性与容器遍历的边界分支：
// 普通子流程开始事件、主流程终点、幽灵元素、嵌套子流程容器定位。
// @param t 测试上下文
func TestBackNodeReachableGraphEdges(t *testing.T) {
	fx := newBackFixture(t)
	ctx := context.Background()
	bpmn, err := fx.engine.RepositoryService().GetBpmnModel(ctx, deploymentOfDefID(t, fx, "backSub"))
	if err != nil {
		t.Fatal(err)
	}
	process := bpmn.Processes[0]
	subStartEl, _ := findFlowElement(process, "subStart")
	subStart := subStartEl.(model.FlowNode)
	u1El, _ := findFlowElement(process, "u1")
	u1 := u1El.(model.FlowNode)
	afterEl, _ := findFlowElement(process, "after")
	after := afterEl.(model.FlowNode)
	endEl, _ := findFlowElement(process, "end")
	mainEnd := endEl.(model.FlowNode)
	innerTaskEl, _ := findFlowElement(process, "innerTask")
	innerTask := innerTaskEl.(model.FlowNode)
	// 普通子流程开始事件可沿路径继续（事件子流程排除仅针对 triggeredByEvent 容器）
	if !isNodeReachable(process, subStart, u1) {
		t.Fatalf("普通子流程开始事件应可达内部任务")
	}
	// 主流程终点无出线且无容器：不可达任何节点
	if isNodeReachable(process, mainEnd, after) {
		t.Fatalf("主流程终点不应可达其他节点")
	}
	// 嵌套子流程内部元素可达子流程后节点（终点逐层提升）
	if !isNodeReachable(process, innerTask, after) {
		t.Fatalf("嵌套子流程内任务应可达 after")
	}
	// 幽灵元素无容器
	if containerOfFlowElement(process, "ghost") != nil {
		t.Fatalf("幽灵元素应无容器")
	}
	// 嵌套子流程内部元素的直接容器为内层子流程
	if got := containerOfFlowElement(process, "innerTask"); got == nil || got.ID != "inner" {
		t.Fatalf("innerTask 的直接容器应为 inner: %+v", got)
	}
	// 幽灵目标的后继枚举为空
	if ids := nextActivityIDsAfter(process, "ghost"); len(ids) != 0 {
		t.Fatalf("幽灵目标后继应为空: %v", ids)
	}
	_ = u1
}

// TestBackToStepFaultsStage2 验证任意退回前段依赖故障：定义查询、步骤规则、预存版本推算。
// @param t 测试上下文
func TestBackToStepFaultsStage2(t *testing.T) {
	fx := newBackFixture(t)
	ctx := context.Background()
	deploymentID := deploymentOf(t, fx, "flowComplete")
	fx.steps.Put(&StepRule{DeploymentID: deploymentID, StepCode: "handle", AllowReturn: "1", Sort: 2})
	baseStores := NewMemStores(fx.fakeClock)
	cases := []struct {
		name  string
		eng   api.Engine
		steps StepStore
		next  NextActivityStore
	}{
		{"定义查询故障", &faultEngine{Engine: fx.engine, failRepo: true}, fx.steps, baseStores.Next},
		{"步骤规则故障", &faultEngine{Engine: fx.engine}, &faultStepStore{StepStore: fx.steps, failGet: true}, baseStores.Next},
		{"预存版本故障", &faultEngine{Engine: fx.engine}, fx.steps, &faultNextStore{NextActivityStore: baseStores.Next, failLatest: true}},
		{"历史任务查询故障", &faultEngine{Engine: fx.engine, failHisTaskQry: true}, fx.steps, baseStores.Next},
	}
	for _, c := range cases {
		instance, apply := startBack(t, fx, "flowComplete", "alice")
		completeBack(t, fx, apply.ID, "alice")
		handle := fx.firstBackTask(t, instance.ID)
		svc := newFaultBackService(fx, c.eng, c.steps, c.next, baseStores.Message, baseStores.History, baseStores.Operation, fx.models)
		if err := svc.BackToStep(ctx, BackTaskVo{TaskID: handle.ID, UserCode: "alice", Message: "退回"}, "apply"); err == nil {
			t.Fatalf("%s 应上抛错误", c.name)
		}
	}
}

// TestBackToStepFaultDedupExecutors 验证目标执行人预存的多轮去重分支
// （循环语料中 b 节点两轮历史同办理人，第二轮触发 seen 去重跳过）。
// @param t 测试上下文
func TestBackToStepFaultDedupExecutors(t *testing.T) {
	fx := newBackFixture(t)
	ctx := context.Background()
	instance, task := startBack(t, fx, "backLoop", "alice")
	completeBack(t, fx, task.ID, "alice") // a
	b1 := fx.firstBackTask(t, instance.ID)
	completeBack(t, fx, b1.ID, "alice") // b 第一轮
	c1 := fx.firstBackTask(t, instance.ID)
	if err := fx.engine.RuntimeService().SetVariable(ctx, instance.ID, "again", true); err != nil {
		t.Fatal(err)
	}
	completeBack(t, fx, c1.ID, "alice") // 循环回 b
	b2 := fx.firstBackTask(t, instance.ID)
	completeBack(t, fx, b2.ID, "alice") // b 第二轮
	c2 := fx.firstBackTask(t, instance.ID)
	// 从 c2 退回 b：目标执行人为两轮去重后的 alice（单条预存记录）
	if err := fx.back.BackToStep(ctx, BackTaskVo{TaskID: c2.ID, UserCode: "alice", Message: "退回"}, "b"); err != nil {
		t.Fatalf("退回失败: %v", err)
	}
	records, err := fx.next.ListUnusedByInstanceAndActivity(ctx, instance.ID, "b")
	if err != nil || len(records) != 1 || records[0].IdentityID != "alice" {
		t.Fatalf("目标执行人应去重为单条: %v %+v", err, records)
	}
}

// TestBackToPreviousFaultsStage2 验证退回上一节点的任务缺失、定义查询与收尾故障分支。
// @param t 测试上下文
func TestBackToPreviousFaultsStage2(t *testing.T) {
	fx := newBackFixture(t)
	ctx := context.Background()
	baseStores := NewMemStores(fx.fakeClock)
	// 任务缺失（NotFound → 任务不存在错误）
	svc := newFaultBackService(fx, &faultEngine{Engine: fx.engine, taskMissing: true}, fx.steps, baseStores.Next, baseStores.Message, baseStores.History, baseStores.Operation, fx.models)
	if err := svc.BackToPrevious(ctx, BackTaskVo{TaskID: "ghost", UserCode: "alice"}); err == nil || !containsText(err, "任务不存在") {
		t.Fatalf("任务缺失应报错: %v", err)
	}
	// 定义查询故障
	svc = newFaultBackService(fx, &faultEngine{Engine: fx.engine, failRepo: true}, fx.steps, baseStores.Next, baseStores.Message, baseStores.History, baseStores.Operation, fx.models)
	if err := svc.BackToPrevious(ctx, BackTaskVo{TaskID: "ghost", UserCode: "alice"}); err == nil {
		t.Fatalf("定义查询故障应上抛")
	}
	// 收尾链故障：待办删除 / 已办回写 / 待办重建
	tails := []struct {
		name string
		msgs MessageExtendStore
		hist HistoryExtendStore
		ops  OperationRecordStore
	}{
		{"当前待办删除故障", &faultMessageStore{MessageExtendStore: baseStores.Message, failDeleteTask: true}, baseStores.History, baseStores.Operation},
		{"已办回写故障", baseStores.Message, &faultHistoryStore{HistoryExtendStore: baseStores.History, failFinish: true}, baseStores.Operation},
		{"待办重建故障", &faultMessageStore{MessageExtendStore: baseStores.Message, failCreate: true}, baseStores.History, baseStores.Operation},
		{"待办查询故障", &faultMessageStore{MessageExtendStore: baseStores.Message, failGet: true}, baseStores.History, baseStores.Operation},
		{"任务列表故障", baseStores.Message, baseStores.History, baseStores.Operation},
	}
	for _, tail := range tails {
		instance, apply := startBack(t, fx, "backFlow3", "alice")
		completeBack(t, fx, apply.ID, "alice")
		s2 := fx.firstBackTask(t, instance.ID)
		var eng api.Engine = fx.engine
		if tail.name == "任务列表故障" {
			eng = &faultEngine{Engine: fx.engine, failTaskList: true}
		}
		svc = newFaultBackService(fx, eng, fx.steps, baseStores.Next, tail.msgs, tail.hist, tail.ops, fx.models)
		if err := svc.BackToPrevious(ctx, BackTaskVo{TaskID: s2.ID, UserCode: "alice", Message: "退回"}); err == nil {
			t.Fatalf("%s 应上抛错误", tail.name)
		}
	}
}

// TestBackServiceEnsureInstanceActiveEdges 验证挂起校验的空实例服务与查询失败分支。
// @param t 测试上下文
func TestBackServiceEnsureInstanceActiveEdges(t *testing.T) {
	fx := newBackFixture(t)
	ctx := context.Background()
	// instances 未注入：直接放行
	bare := &BackService{}
	if err := bare.ensureInstanceActive(ctx, "any"); err != nil {
		t.Fatalf("空实例服务应放行: %v", err)
	}
	// 实例不存在（运行表查不到）：放行交由引擎兜底
	if err := fx.back.ensureInstanceActive(ctx, "ghost-instance"); err != nil {
		t.Fatalf("不存在的实例应放行: %v", err)
	}
}

// TestGenerateTodosAfterBackFaults 验证待办重生成的故障分支与镜像构建失败分支。
// @param t 测试上下文
func TestGenerateTodosAfterBackFaults(t *testing.T) {
	fx := newBackFixture(t)
	ctx := context.Background()
	baseStores := NewMemStores(fx.fakeClock)
	// 任务列表故障
	svc := newFaultBackService(fx, &faultEngine{Engine: fx.engine, failTaskList: true}, fx.steps, baseStores.Next, baseStores.Message, baseStores.History, baseStores.Operation, fx.models)
	if _, err := svc.generateTodosAfterBack(ctx, "any"); err == nil {
		t.Fatalf("任务列表故障应上抛")
	}
	// 待办查询故障（幂等检查）
	svc = newFaultBackService(fx, fx.engine, fx.steps, baseStores.Next, &faultMessageStore{MessageExtendStore: baseStores.Message, failGet: true}, baseStores.History, baseStores.Operation, fx.models)
	instance, apply := startBack(t, fx, "backFlow3", "alice")
	_ = apply
	if _, err := svc.generateTodosAfterBack(ctx, instance.ID); err == nil {
		t.Fatalf("待办查询故障应上抛")
	}
	// 镜像构建失败：实例不存在（GetProcessInstance 失败）
	svc = newFaultBackService(fx, fx.engine, fx.steps, baseStores.Next, baseStores.Message, baseStores.History, baseStores.Operation, fx.models)
	if _, err := svc.buildBackMessageExtend(ctx, "ghost-instance", api.Task{ProcessDefinitionID: "any"}); err == nil {
		t.Fatalf("实例缺失应上抛")
	}
	// 镜像构建失败：定义查询故障（幽灵定义）
	if _, err := svc.buildBackMessageExtend(ctx, instance.ID, api.Task{ProcessDefinitionID: "ghost-def"}); err == nil {
		t.Fatalf("定义缺失应上抛")
	}
}

// TestDirectServiceFaultsStage2 验证直送的办理人更新、直送记录落盘与重生成故障分支。
// @param t 测试上下文
func TestDirectServiceFaultsStage2(t *testing.T) {
	fx := newBackFixture(t)
	ctx := context.Background()
	baseStores := NewMemStores(fx.fakeClock)
	build := func(eng api.Engine, next, msgs, hist, ops, completeTo any) *DirectService {
		nextStore, _ := next.(NextActivityStore)
		messageStore, _ := msgs.(MessageExtendStore)
		historyStore, _ := hist.(HistoryExtendStore)
		operationStore, _ := ops.(OperationRecordStore)
		completeToStore, _ := completeTo.(CompleteToActivityStore)
		return NewDirectService(eng, NewModelMetaService(eng), NewNextActivityService(nextStore, fx.fakeClock),
			fxComments(fx), fxOperations(fx, operationStore), NewMessageExtendService(messageStore, fx.fakeClock),
			NewHistoryExtendService(historyStore, fx.fakeClock), fxDocuments(fx), completeToStore, fx.fakeClock, WithDirectUsers(errUsers{}))
	}
	cases := []struct {
		name       string
		eng        api.Engine
		msgs       MessageExtendStore
		completeTo any
	}{
		{"办理人更新故障", &faultEngine{Engine: fx.engine, failTaskUpd: true}, baseStores.Message, fx.completeTo},
		{"直送记录故障", &faultEngine{Engine: fx.engine}, baseStores.Message, &faultCompleteToStore{CompleteToActivityStore: fx.completeTo, failSave: true}},
	}
	for _, c := range cases {
		instance, apply := startBack(t, fx, "flowComplete", "alice")
		completeBack(t, fx, apply.ID, "alice")
		handle := fx.firstBackTask(t, instance.ID)
		svc := build(c.eng, baseStores.Next, c.msgs, baseStores.History, baseStores.Operation, c.completeTo)
		err := svc.CompleteToTargetActivity(ctx, &api.CompleteTaskVo{
			BaseProcessVo:    api.BaseProcessVo{TaskID: handle.ID, UserCode: "alice", Message: "直送"},
			TargetActivityID: "apply",
			Request: &api.NextActivityRequest{
				TaskID:           handle.ID,
				NextActivityList: []*api.NextActivityVo{{ActivityID: "apply", ExecutorIDs: []string{"bob"}}},
			},
		})
		if err == nil {
			t.Fatalf("%s 应上抛错误", c.name)
		}
	}
}

// TestDirectServiceGenerateFaults 验证直送待办重生成的故障与边界分支。
// @param t 测试上下文
func TestDirectServiceGenerateFaults(t *testing.T) {
	fx := newBackFixture(t)
	ctx := context.Background()
	baseStores := NewMemStores(fx.fakeClock)
	// 任务列表故障
	svc := NewDirectService(&faultEngine{Engine: fx.engine, failTaskList: true}, NewModelMetaService(fx.engine), NewNextActivityService(baseStores.Next, fx.fakeClock),
		fxComments(fx), fxOperations(fx, baseStores.Operation), NewMessageExtendService(baseStores.Message, fx.fakeClock),
		NewHistoryExtendService(baseStores.History, fx.fakeClock), fxDocuments(fx), fx.completeTo, fx.fakeClock)
	if _, err := svc.generateTodosAfterDirect(ctx, "any"); err == nil {
		t.Fatalf("任务列表故障应上抛")
	}
	// 待办查询故障（幂等检查）
	svc = NewDirectService(fx.engine, NewModelMetaService(fx.engine), NewNextActivityService(baseStores.Next, fx.fakeClock),
		fxComments(fx), fxOperations(fx, baseStores.Operation), NewMessageExtendService(&faultMessageStore{MessageExtendStore: baseStores.Message, failGet: true}, fx.fakeClock),
		NewHistoryExtendService(baseStores.History, fx.fakeClock), fxDocuments(fx), fx.completeTo, fx.fakeClock)
	instance, _ := startBack(t, fx, "backFlow3", "alice")
	if _, err := svc.generateTodosAfterDirect(ctx, instance.ID); err == nil {
		t.Fatalf("待办查询故障应上抛")
	}
	// 幂等与无主任务：gnext 组任务无处理人跳过、已有待办幂等返回（重建无故障服务）
	svc = NewDirectService(fx.engine, NewModelMetaService(fx.engine), NewNextActivityService(baseStores.Next, fx.fakeClock),
		fxComments(fx), fxOperations(fx, baseStores.Operation), NewMessageExtendService(baseStores.Message, fx.fakeClock),
		NewHistoryExtendService(baseStores.History, fx.fakeClock), fxDocuments(fx), fx.completeTo, fx.fakeClock)
	instance2, gtask := startBack(t, fx, "flowGroup", "alice")
	completeBack(t, fx, gtask.ID, "alice")
	gnext := fx.firstBackTask(t, instance2.ID)
	ids, err := svc.generateTodosAfterDirect(ctx, instance2.ID)
	if err != nil || len(ids) != 0 {
		t.Fatalf("无处理人任务应跳过: %v %v", ids, err)
	}
	if _, err := NewMessageExtendService(baseStores.Message, fx.fakeClock).CreateForTask(ctx, &MessageExtend{
		BaseColumns:       BaseColumns{TenantID: "dev"},
		TaskID:            gnext.ID,
		ProcessInstanceID: instance2.ID,
		OwnerID:           "alice",
	}); err != nil {
		t.Fatal(err)
	}
	ids, err = svc.generateTodosAfterDirect(ctx, instance2.ID)
	if err != nil || len(ids) != 1 || ids[0] != gnext.ID {
		t.Fatalf("已有待办应幂等返回: %v %v", ids, err)
	}
	// 镜像构建失败：实例缺失 / 定义缺失
	if _, err := svc.buildDirectMessageExtend(ctx, "ghost-instance", gnext); err == nil {
		t.Fatalf("实例缺失应上抛")
	}
	if _, err := svc.buildDirectMessageExtend(ctx, instance2.ID, api.Task{ProcessDefinitionID: "ghost-def"}); err == nil {
		t.Fatalf("定义缺失应上抛")
	}
}

// TestDirectEnsureInstanceActiveDeleted 验证直送挂起校验对不存在实例的放行分支。
// @param t 测试上下文
func TestDirectEnsureInstanceActiveDeleted(t *testing.T) {
	fx := newBackFixture(t)
	ctx := context.Background()
	if err := fx.direct.ensureInstanceActive(ctx, "ghost-instance"); err != nil {
		t.Fatalf("不存在的实例应放行: %v", err)
	}
}
