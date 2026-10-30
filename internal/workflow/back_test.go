package workflow

import (
	"context"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/bpmn/model"
	"github.com/DengQingNian/Flowtide/internal/clock"
	"github.com/DengQingNian/Flowtide/internal/engine"
	"github.com/DengQingNian/Flowtide/internal/idgen"
)

// backFlow3Process 是退回主语料：s1 → s2 → s3 线性三节点（办理人均发起人）。
const backFlow3Process = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="backFlow3" name="退回三节点流程" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="s1"/>
    <userTask id="s1" name="节点一" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f1" sourceRef="s1" targetRef="s2"/>
    <userTask id="s2" name="节点二" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f2" sourceRef="s2" targetRef="s3"/>
    <userTask id="s3" name="节点三" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f3" sourceRef="s3" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// backLoopProcess 是退回循环语料：a → b → c → gate →(again=true) b / (默认) fend → end，
// 用于验证同节点多轮历史聚合（b 出现两轮）。
const backLoopProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="backLoop" name="退回循环流程" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="a"/>
    <userTask id="a" name="甲" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f1" sourceRef="a" targetRef="b"/>
    <userTask id="b" name="乙" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f2" sourceRef="b" targetRef="c"/>
    <userTask id="c" name="丙" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f3" sourceRef="c" targetRef="gate"/>
    <exclusiveGateway id="gate" default="f_end"/>
    <sequenceFlow id="f_loop" sourceRef="gate" targetRef="b">
      <conditionExpression>${again == true}</conditionExpression>
    </sequenceFlow>
    <sequenceFlow id="f_end" sourceRef="gate" targetRef="fend"/>
    <endEvent id="fend"/>
  </process>
</definitions>`

// backBranchEndProcess 是可达性过滤语料：fork 并行分叉后 y1→endY、x1→endX 各自终结，
// y1 与 x1 互不可达（y1 的历史候选应被可达性过滤剔除）。
const backBranchEndProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="backBranchEnd" name="并行互斥退回流程" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="fork"/>
    <parallelGateway id="fork"/>
    <sequenceFlow id="f1" sourceRef="fork" targetRef="y1"/>
    <sequenceFlow id="f2" sourceRef="fork" targetRef="x1"/>
    <userTask id="y1" name="乙支" flowable:assignee="bob"/>
    <sequenceFlow id="f3" sourceRef="y1" targetRef="endY"/>
    <endEvent id="endY"/>
    <userTask id="x1" name="甲支" flowable:assignee="carl"/>
    <sequenceFlow id="f4" sourceRef="x1" targetRef="endX"/>
    <endEvent id="endX"/>
  </process>
</definitions>`

// backConvergeProcess 是多入线语料：a1 与 b1 汇聚到 join，join 入线不唯一，
// 用于验证退回上一节点的多入线防护。
const backConvergeProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="backConverge" name="汇聚退回流程" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="a1"/>
    <userTask id="a1" name="支线一" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f1" sourceRef="a1" targetRef="join"/>
    <userTask id="b1" name="支线二" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f2" sourceRef="b1" targetRef="join"/>
    <userTask id="join" name="汇聚" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f3" sourceRef="join" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// backSubProcess 是子流程图遍历语料：主流程 start → sub → after → end；
// sub 内 subStart → u1（无名称）→ subEnd；事件子流程 esub 内 esStart → esTask → esEnd。
// 用于可达性/后继枚举纯函数的子流程分支测试。
const backSubProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="backSub" name="子流程遍历流程" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="sub"/>
    <subProcess id="sub" name="嵌入式子流程">
      <startEvent id="subStart"/>
      <sequenceFlow id="sf0" sourceRef="subStart" targetRef="u1"/>
      <userTask id="u1" flowable:assignee="bob"/>
      <sequenceFlow id="sf1" sourceRef="u1" targetRef="inner"/>
      <subProcess id="inner" name="嵌套子流程">
        <startEvent id="innerStart"/>
        <sequenceFlow id="nf0" sourceRef="innerStart" targetRef="innerTask"/>
        <userTask id="innerTask" name="内层任务" flowable:assignee="bob"/>
        <sequenceFlow id="nf1" sourceRef="innerTask" targetRef="innerEnd"/>
        <endEvent id="innerEnd"/>
      </subProcess>
      <sequenceFlow id="sf2" sourceRef="inner" targetRef="subEnd"/>
      <endEvent id="subEnd"/>
    </subProcess>
    <sequenceFlow id="f1" sourceRef="sub" targetRef="after"/>
    <userTask id="after" name="子流程后" flowable:assignee="carl"/>
    <sequenceFlow id="f2" sourceRef="after" targetRef="end"/>
    <endEvent id="end"/>
    <subProcess id="esub" name="事件子流程" triggeredByEvent="true">
      <startEvent id="esStart"/>
      <sequenceFlow id="ef0" sourceRef="esStart" targetRef="esTask"/>
      <userTask id="esTask" name="事件任务" flowable:assignee="bob"/>
      <sequenceFlow id="ef1" sourceRef="esTask" targetRef="esEnd"/>
      <endEvent id="esEnd"/>
    </subProcess>
  </process>
</definitions>`

// backParallelStepProcess 是并行块退回语料：s → pb_begin → (p1 | p2) → pb_end → s3 → end，
// 用于验证"目标在并行块内、当前在块外"的并行退回拒绝透传。
const backParallelStepProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="backParallelStep" name="并行退回流程" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="pb_begin"/>
    <parallelGateway id="pb_begin"/>
    <sequenceFlow id="f1" sourceRef="pb_begin" targetRef="p1"/>
    <sequenceFlow id="f2" sourceRef="pb_begin" targetRef="p2"/>
    <userTask id="p1" name="并行一" flowable:assignee="bob"/>
    <userTask id="p2" name="并行二" flowable:assignee="carl"/>
    <sequenceFlow id="f3" sourceRef="p1" targetRef="pb_end"/>
    <sequenceFlow id="f4" sourceRef="p2" targetRef="pb_end"/>
    <parallelGateway id="pb_end"/>
    <sequenceFlow id="f5" sourceRef="pb_end" targetRef="s3"/>
    <userTask id="s3" name="汇聚后" flowable:assignee="alice"/>
    <sequenceFlow id="f6" sourceRef="s3" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// backFixture 是退回/直送测试环境（内存引擎 + MemStores + 退回/直送域服务）。
type backFixture struct {
	engine     api.Engine                  // 内存引擎
	stores     *MemStores                  // 五大域内存存储
	steps      *MemStepStore               // 步骤规则内存存储
	models     *MemModelStore              // 模型主数据内存存储
	completeTo *MemCompleteToActivityStore // 直送记录内存存储
	stepSvc    *StepRuleService            // 步骤规则服务
	next       *NextActivityService        // 下一步预生成
	comments   *CommentService             // 意见
	operations *OperationRecordService     // 操作记录
	messages   *MessageExtendService       // 待办扩展
	histories  *HistoryExtendService       // 已办扩展
	documents  *DocumentService            // 表单状态机
	back       *BackService                // 退回服务
	direct     *DirectService              // 直送服务
	fakeClock  *clock.Fake                 // 假时钟
}

// newBackFixture 构造退回/直送测试环境（部署全部语料、创建测试用户）。
// @param t 测试上下文
// @return 测试环境
func newBackFixture(t *testing.T) *backFixture {
	t.Helper()
	fake := clock.NewFake(completeFixtureTime)
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
	// 部署全部退回语料（含办结测试复用的 flowComplete）
	if _, err := eng.RepositoryService().Deploy(context.Background(), api.DeploymentRequest{
		Name:     "back-test",
		TenantID: "dev",
		Resources: map[string][]byte{
			"back3.bpmn20.xml":        []byte(backFlow3Process),
			"backloop.bpmn20.xml":     []byte(backLoopProcess),
			"backbranch.bpmn20.xml":   []byte(backBranchEndProcess),
			"backconverge.bpmn20.xml": []byte(backConvergeProcess),
			"backparallel.bpmn20.xml": []byte(backParallelStepProcess),
			"backsub.bpmn20.xml":      []byte(backSubProcess),
			"backgroup.bpmn20.xml":    []byte(flowGroupProcess),
			"complete.bpmn20.xml":     []byte(flowCompleteProcess),
		},
	}); err != nil {
		t.Fatalf("部署失败: %v", err)
	}
	// 创建测试用户
	for _, user := range []api.User{
		{ID: "alice", DisplayName: "张三"},
		{ID: "bob", DisplayName: "李四"},
		{ID: "carl", DisplayName: "王五"},
	} {
		if _, err := eng.IdentityService().CreateUser(context.Background(), user); err != nil {
			t.Fatalf("创建用户 %s 失败: %v", user.ID, err)
		}
	}
	fx := &backFixture{engine: eng, fakeClock: fake}
	fx.stores = NewMemStores(fake)
	fx.steps = NewMemStepStore()
	fx.models = NewMemModelStore()
	fx.completeTo = NewMemCompleteToActivityStore(fake)
	fx.stepSvc = NewStepRuleService(fx.steps, eng)
	fx.next = NewNextActivityService(fx.stores.Next, fake)
	fx.comments = NewCommentService(eng.TaskService(), eng.HistoryService(), eng.IdentityService())
	fx.operations = NewOperationRecordService(fx.stores.Operation, eng.HistoryService(), engineInstanceQuery{runtime: eng.RuntimeService(), history: eng.HistoryService()}, eng.RepositoryService(), eng.IdentityService(), fake)
	fx.messages = NewMessageExtendService(fx.stores.Message, fake)
	fx.histories = NewHistoryExtendService(fx.stores.History, fake)
	fx.documents = NewDocumentService(fx.stores.Document, fx.stores.Message, fx.stores.Operation, engineInstanceQuery{runtime: eng.RuntimeService(), history: eng.HistoryService()}, eng.HistoryService(), fake)
	fx.back = NewBackService(eng, NewModelMetaService(eng), fx.next, fx.comments, fx.operations, fx.messages, fx.histories, fx.documents, fx.stepSvc, engineInstanceQuery{runtime: eng.RuntimeService(), history: eng.HistoryService()}, fake, WithBackModels(fx.models), WithBackUsers(eng.IdentityService()))
	fx.direct = NewDirectService(eng, NewModelMetaService(eng), fx.next, fx.comments, fx.operations, fx.messages, fx.histories, fx.documents, fx.completeTo, fake, WithDirectUsers(eng.IdentityService()))
	return fx
}

// deploymentOf 按流程 Key 取定义的部署 ID。
// @param t 测试上下文
// @param f 测试环境
// @param key 流程定义 Key
// @return 部署 ID
func deploymentOf(t *testing.T, f *backFixture, key string) string {
	t.Helper()
	definition, err := f.engine.RepositoryService().GetProcessDefinitionByKey(context.Background(), key)
	if err != nil {
		t.Fatalf("查询定义 %s 失败: %v", key, err)
	}
	return definition.DeploymentID
}

// startBack 发起流程实例并返回实例与首个任务。
// @param t 测试上下文
// @param f 测试环境
// @param definitionKey 流程定义 Key
// @param initiator 发起人
// @return 流程实例
// @return 首个任务
func startBack(t *testing.T, f *backFixture, definitionKey, initiator string) (api.ProcessInstance, api.Task) {
	t.Helper()
	instance, err := f.engine.RuntimeService().StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: definitionKey,
		BusinessKey:          "biz-" + definitionKey,
		Name:                 "标题-" + definitionKey,
		TenantID:             "dev",
		UserID:               initiator,
		Variables:            []api.Variable{{Name: VarInitiator, Value: initiator}},
	})
	if err != nil {
		t.Fatalf("发起失败: %v", err)
	}
	task := f.firstBackTask(t, instance.ID)
	return instance, task
}

// firstBackTask 返回实例当前首个任务。
// @param t 测试上下文
// @param f 测试环境
// @param instanceID 实例 Id
// @return 任务
func (f *backFixture) firstBackTask(t *testing.T, instanceID string) api.Task {
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

// completeBack 推进时钟后完成任务。
// @param t 测试上下文
// @param f 测试环境
// @param taskID 任务 Id
// @param userID 完成人
func completeBack(t *testing.T, f *backFixture, taskID, userID string) {
	t.Helper()
	f.fakeClock.Advance(time.Minute)
	if err := f.engine.TaskService().Complete(context.Background(), taskID, api.CompleteTaskRequest{UserID: userID}); err != nil {
		t.Fatalf("完成任务失败: %v", err)
	}
}

// historicActivityRows 统计实例某活动的历史活动行数（退回历史清理断言用）。
// @param t 测试上下文
// @param f 测试环境
// @param instanceID 实例 Id
// @param activityID 活动 Id
// @return 行数
func historicActivityRows(t *testing.T, f *backFixture, instanceID, activityID string) int {
	t.Helper()
	page, err := f.engine.HistoryService().QueryHistoricActivityInstances(context.Background(), api.HistoricActivityInstanceFilter{
		ProcessInstanceID: instanceID,
		ActivityID:        activityID,
		Size:              100,
	})
	if err != nil {
		t.Fatalf("查询历史活动失败: %v", err)
	}
	return int(page.Total)
}

// TestBackNodesBasics 验证可退节点计算的基础链路：空入参、任务不存在、线性候选与办理人姓名。
// @param t 测试上下文
func TestBackNodesBasics(t *testing.T) {
	fx := newBackFixture(t)
	ctx := context.Background()
	// 空任务 ID → 空列表
	if views, err := fx.back.BackNodes(ctx, " "); err != nil || len(views) != 0 {
		t.Fatalf("空任务 ID 应返回空列表: %v %v", views, err)
	}
	// 任务不存在 → 空列表
	if views, err := fx.back.BackNodes(ctx, "no-such-task"); err != nil || len(views) != 0 {
		t.Fatalf("任务不存在应返回空列表: %v %v", views, err)
	}
	// 正常链路：s2 任务上可退回 s1
	instance, task := startBack(t, fx, "backFlow3", "alice")
	_ = instance
	completeBack(t, fx, task.ID, "alice")
	s2 := fx.firstBackTask(t, instance.ID)
	views, err := fx.back.BackNodes(ctx, s2.ID)
	if err != nil {
		t.Fatalf("可退节点计算失败: %v", err)
	}
	if len(views) != 1 || views[0].ActivityID != "s1" {
		t.Fatalf("s2 应仅可退回 s1: %+v", views)
	}
	if views[0].ActivityName != "节点一" {
		t.Fatalf("节点名称应为模型名: %+v", views[0])
	}
	if len(views[0].TaskIDs) != 1 || views[0].TaskIDs[0] != task.ID {
		t.Fatalf("候选任务 ID 应为首轮 s1 任务: %+v", views[0].TaskIDs)
	}
	// 办理人姓名经身份库解析（alice → 张三）
	if len(views[0].AssigneeNames) != 1 || views[0].AssigneeNames[0] != "张三" {
		t.Fatalf("办理人姓名应为张三: %+v", views[0].AssigneeNames)
	}
}

// TestBackNodesAllowReturnGate 验证过滤链①：当前节点 allowReturn=="0" 时返回空列表。
// @param t 测试上下文
func TestBackNodesAllowReturnGate(t *testing.T) {
	fx := newBackFixture(t)
	deploymentID := deploymentOf(t, fx, "backFlow3")
	// s2 禁止退回
	fx.steps.Put(&StepRule{DeploymentID: deploymentID, StepCode: "s2", AllowReturn: "0", Sort: 2})
	instance, task := startBack(t, fx, "backFlow3", "alice")
	completeBack(t, fx, task.ID, "alice")
	s2 := fx.firstBackTask(t, instance.ID)
	views, err := fx.back.BackNodes(context.Background(), s2.ID)
	if err != nil || len(views) != 0 {
		t.Fatalf("allowReturn=0 应返回空列表: %v %v", views, err)
	}
}

// TestBackNodesWhitelist 验证过滤链⑤：backStepCodes 白名单非空时仅白名单内节点可退。
// @param t 测试上下文
func TestBackNodesWhitelist(t *testing.T) {
	fx := newBackFixture(t)
	deploymentID := deploymentOf(t, fx, "backFlow3")
	instance, task := startBack(t, fx, "backFlow3", "alice")
	completeBack(t, fx, task.ID, "alice")
	s2 := fx.firstBackTask(t, instance.ID)
	// 白名单命中：保留 s1
	fx.steps.Put(&StepRule{DeploymentID: deploymentID, StepCode: "s2", AllowReturn: "1", BackStepCodes: "s1", Sort: 2})
	views, err := fx.back.BackNodes(context.Background(), s2.ID)
	if err != nil || len(views) != 1 || views[0].ActivityID != "s1" {
		t.Fatalf("白名单命中应保留 s1: %v %v", views, err)
	}
	// 白名单未命中：结果为空
	fx.steps.Put(&StepRule{DeploymentID: deploymentID, StepCode: "s2", AllowReturn: "1", BackStepCodes: "other", Sort: 2})
	views, err = fx.back.BackNodes(context.Background(), s2.ID)
	if err != nil || len(views) != 0 {
		t.Fatalf("白名单未命中应为空: %v %v", views, err)
	}
}

// TestBackNodesAncestorChain 验证过滤链④：祖先链（ModelStore 注入 + preTaskId 链）。
// 仅保留父链命中的活动：未注入 ModelStore 时 s1/s2 均候选；注入后父链仅含 s2 任务 → 只保留 s2。
// @param t 测试上下文
func TestBackNodesAncestorChain(t *testing.T) {
	fx := newBackFixture(t)
	ctx := context.Background()
	// 未注入 ModelStore 的服务实例：跳过祖先链过滤
	plainBack := NewBackService(fx.engine, NewModelMetaService(fx.engine), fx.next, fx.comments, fx.operations, fx.messages, fx.histories, fx.documents, fx.stepSvc, engineInstanceQuery{runtime: fx.engine.RuntimeService(), history: fx.engine.HistoryService()}, fx.fakeClock)
	instance, task := startBack(t, fx, "backFlow3", "alice")
	completeBack(t, fx, task.ID, "alice")
	s2 := fx.firstBackTask(t, instance.ID)
	completeBack(t, fx, s2.ID, "alice")
	s3 := fx.firstBackTask(t, instance.ID)
	// 播种父链：仅 s3 消耗记录（pre_task_id=s2 任务），s2 无消耗记录 → 链仅含 s2 任务
	if err := fx.next.InsertDual(ctx, &NextActivity{
		BaseColumns:       BaseColumns{TenantID: "dev"},
		NextActivityID:    "s3",
		IdentityID:        "alice",
		ProcessInstanceID: instance.ID,
		Used:              1,
		PreTaskID:         s2.ID,
		TaskID:            s3.ID,
		Version:           1,
	}, "alice"); err != nil {
		t.Fatalf("播种父链记录失败: %v", err)
	}
	// 未注入 ModelStore：s1/s2 均候选
	plainViews, err := plainBack.BackNodes(ctx, s3.ID)
	if err != nil || len(plainViews) != 2 {
		t.Fatalf("无祖先链应保留 s1/s2: %v %v", plainViews, err)
	}
	// 注入 ModelStore（returnSort=false 升序链）：父链 {s2 任务} → 仅保留 s2
	fx.models.PutReturnSort("backFlow3", false)
	views, err := fx.back.BackNodes(ctx, s3.ID)
	if err != nil || len(views) != 1 || views[0].ActivityID != "s2" {
		t.Fatalf("祖先链过滤应仅保留 s2: %v %v", views, err)
	}
	// returnSort=true 逆序链：父链集合不变，结果一致
	fx.models.PutReturnSort("backFlow3", true)
	views, err = fx.back.BackNodes(ctx, s3.ID)
	if err != nil || len(views) != 1 || views[0].ActivityID != "s2" {
		t.Fatalf("逆序祖先链应同样仅保留 s2: %v %v", views, err)
	}
}

// TestBackNodesReachability 验证过滤链⑥：与当前节点互不可达的历史候选被剔除。
// @param t 测试上下文
func TestBackNodesReachability(t *testing.T) {
	fx := newBackFixture(t)
	instance, _ := startBack(t, fx, "backBranchEnd", "alice")
	// 并行分支：完成 y1（其分支终结），x1 仍在办
	var y1Task, x1Task api.Task
	for _, task := range func() []api.Task {
		page, err := fx.engine.TaskService().ListTasks(context.Background(), api.TaskFilter{ProcessInstanceID: instance.ID, Size: 10})
		if err != nil {
			t.Fatalf("查询任务失败: %v", err)
		}
		return page.Data
	}() {
		if task.TaskDefinitionKey == "y1" {
			y1Task = task
		}
		if task.TaskDefinitionKey == "x1" {
			x1Task = task
		}
	}
	completeBack(t, fx, y1Task.ID, "bob")
	// x1 上的可退节点：y1 与 x1 互不可达 → 空列表
	views, err := fx.back.BackNodes(context.Background(), x1Task.ID)
	if err != nil || len(views) != 0 {
		t.Fatalf("互不可达候选应被过滤: %v %v", views, err)
	}
}

// TestBackNodesAggregatesReworkTasks 验证同节点多轮历史的任务 ID 聚合（循环重走场景）。
// @param t 测试上下文
func TestBackNodesAggregatesReworkTasks(t *testing.T) {
	fx := newBackFixture(t)
	ctx := context.Background()
	instance, task := startBack(t, fx, "backLoop", "alice")
	completeBack(t, fx, task.ID, "alice")  // a 完成
	b1 := fx.firstBackTask(t, instance.ID) // b 第一轮
	completeBack(t, fx, b1.ID, "alice")    // b 完成
	c1 := fx.firstBackTask(t, instance.ID) // c 第一轮
	// 条件循环：again=true 回到 b
	if err := fx.engine.RuntimeService().SetVariable(ctx, instance.ID, "again", true); err != nil {
		t.Fatal(err)
	}
	completeBack(t, fx, c1.ID, "alice")
	b2 := fx.firstBackTask(t, instance.ID)
	completeBack(t, fx, b2.ID, "alice")    // b 第二轮完成
	c2 := fx.firstBackTask(t, instance.ID) // c 第二轮
	// 播种差异排序号（a=2/b=1）：覆盖结果排序的比较分支
	deploymentID := deploymentOf(t, fx, "backLoop")
	fx.steps.Put(&StepRule{DeploymentID: deploymentID, StepCode: "a", Sort: 2})
	fx.steps.Put(&StepRule{DeploymentID: deploymentID, StepCode: "b", Sort: 1})
	// c2 的可退节点：b 聚合两轮任务 ID（b1/b2），c 第一轮被"非当前节点"过滤排除
	views, err := fx.back.BackNodes(ctx, c2.ID)
	if err != nil {
		t.Fatalf("可退节点计算失败: %v", err)
	}
	viewsByID := map[string]BackNodeView{}
	for _, view := range views {
		viewsByID[view.ActivityID] = view
	}
	bView, ok := viewsByID["b"]
	if !ok || len(bView.TaskIDs) != 2 {
		t.Fatalf("b 应聚合两轮任务 ID: %+v", views)
	}
	// c 第一轮历史与当前同节点，被过滤排除
	if _, exists := viewsByID["c"]; exists {
		t.Fatalf("当前节点 c 不应出现在候选中: %+v", views)
	}
	// 排序：b 排序号 1 在前、a 排序号 2 在后
	if len(views) != 2 || views[0].ActivityID != "b" || views[1].ActivityID != "a" {
		t.Fatalf("候选应按排序号排列 b 在前: %+v", views)
	}
}

// TestBackToStepFullChain 验证任意退回全链路（退回 → 账本收尾 → 再办结 → 完成）：
// 预存目标执行人、TH 意见、操作记录(isAgree=0)、历史活动行清理、待办清理与重生成、
// 已办完成时间回写、退回后重走全流程正常结束。
// @param t 测试上下文
func TestBackToStepFullChain(t *testing.T) {
	fx := newBackFixture(t)
	ctx := context.Background()
	deploymentID := deploymentOf(t, fx, "flowComplete")
	// 步骤规则：apply/handle 均允许退回
	fx.steps.Put(&StepRule{DeploymentID: deploymentID, StepCode: "apply", AllowReturn: "1", Sort: 1})
	fx.steps.Put(&StepRule{DeploymentID: deploymentID, StepCode: "handle", AllowReturn: "1", Sort: 2})
	instance, apply := startBack(t, fx, "flowComplete", "alice")
	// 播种 apply 首任务待办 + 已办镜像（供清理断言）
	message := &MessageExtend{
		BaseColumns:       BaseColumns{TenantID: "dev"},
		TaskID:            apply.ID,
		ProcessInstanceID: instance.ID,
		TaskStepCode:      apply.TaskDefinitionKey,
		OwnerID:           "alice",
	}
	if _, err := fx.messages.CreateForTask(ctx, message); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.histories.CreateForTask(ctx, &HistoryExtend{MessageExtend: *message}); err != nil {
		t.Fatal(err)
	}
	completeBack(t, fx, apply.ID, "alice")
	handle := fx.firstBackTask(t, instance.ID)
	// 播种 handle 待办与已办镜像（退回后待办应被清理、已办完成时间应回写）
	if _, err := fx.messages.CreateForTask(ctx, &MessageExtend{
		BaseColumns:       BaseColumns{TenantID: "dev"},
		TaskID:            handle.ID,
		ProcessInstanceID: instance.ID,
		TaskStepCode:      "handle",
		OwnerID:           "alice",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.histories.CreateForTask(ctx, &HistoryExtend{MessageExtend: MessageExtend{
		BaseColumns:       BaseColumns{TenantID: "dev"},
		TaskID:            handle.ID,
		ProcessInstanceID: instance.ID,
		TaskStepCode:      "handle",
		OwnerID:           "alice",
	}}); err != nil {
		t.Fatal(err)
	}
	// 退回 handle → apply
	if err := fx.back.BackToStep(ctx, BackTaskVo{TaskID: handle.ID, UserCode: "alice", Message: "退回重办"}, "apply"); err != nil {
		t.Fatalf("退回失败: %v", err)
	}
	// 引擎落位：apply 新任务在办，handle 任务消失
	newApply := fx.firstBackTask(t, instance.ID)
	if newApply.TaskDefinitionKey != "apply" || newApply.ID == apply.ID {
		t.Fatalf("退回后应落位 apply 新任务: %+v", newApply)
	}
	// 历史活动行清理：handle 0 行（目标后继），apply 恰 1 行（新落位）
	if rows := historicActivityRows(t, fx, instance.ID, "handle"); rows != 0 {
		t.Fatalf("退回后 handle 历史活动行应被清理，实际 %d", rows)
	}
	if rows := historicActivityRows(t, fx, instance.ID, "apply"); rows != 1 {
		t.Fatalf("退回后 apply 应恰有 1 行历史活动，实际 %d", rows)
	}
	// 目标执行人预存：apply 节点存在 used=0 记录（执行人=历史办理人 alice）
	records, err := fx.next.ListUnusedByInstanceAndActivity(ctx, instance.ID, "apply")
	if err != nil || len(records) != 1 || records[0].IdentityID != "alice" || records[0].PreTaskID != handle.ID {
		t.Fatalf("目标执行人预存失败: %v %+v", err, records)
	}
	// TH 意见 + 操作记录（isAgree=0）
	comments, err := fx.comments.ListByProcessInstance(ctx, instance.ID)
	if err != nil || len(comments) == 0 {
		t.Fatalf("退回意见缺失: %v %v", comments, err)
	}
	records0, err := fx.operations.ListByInstance(ctx, instance.ID)
	if err != nil || len(records0) == 0 {
		t.Fatalf("操作记录缺失: %v %v", records0, err)
	}
	last := records0[len(records0)-1]
	if last.IsAgree == nil || *last.IsAgree != 0 || last.Type != CommentTypeTH {
		t.Fatalf("操作记录应为 isAgree=0/TH: %+v", last)
	}
	// 待办清理与重生成：旧 apply/handle 待办删除，新 apply 任务待办生成
	if _, found, _ := fx.messages.GetByTaskID(ctx, apply.ID); found {
		t.Fatalf("旧 apply 待办应被清理")
	}
	if _, found, _ := fx.messages.GetByTaskID(ctx, handle.ID); found {
		t.Fatalf("handle 待办应被清理")
	}
	newTodo, found, err := fx.messages.GetByTaskID(ctx, newApply.ID)
	if err != nil || !found {
		t.Fatalf("新 apply 任务待办应生成: %v %v", found, err)
	}
	if newTodo.OwnerID != "alice" {
		t.Fatalf("新待办处理人应为 alice: %+v", newTodo)
	}
	// 已办完成时间回写（被退回的 handle 任务已办镜像）
	his, found, err := fx.histories.GetByTaskID(ctx, handle.ID)
	if err != nil || !found || his.TaskFinishTime == nil {
		t.Fatalf("已办完成时间应回写: %v %v %+v", found, err, his)
	}
	// 退回后重走全流程：apply → handle → end
	completeBack(t, fx, newApply.ID, "alice")
	handle2 := fx.firstBackTask(t, instance.ID)
	if handle2.TaskDefinitionKey != "handle" {
		t.Fatalf("重走应推进到 handle: %+v", handle2)
	}
	completeBack(t, fx, handle2.ID, "alice")
	got, err := fx.engine.RuntimeService().GetProcessInstance(ctx, instance.ID)
	if err != nil || !got.Ended {
		t.Fatalf("退回后重走应正常结束: %v %+v", err, got)
	}
}

// TestBackToStepGuards 验证任意退回的校验链：入参缺失、步骤禁止退回、实例挂起。
// @param t 测试上下文
func TestBackToStepGuards(t *testing.T) {
	fx := newBackFixture(t)
	ctx := context.Background()
	deploymentID := deploymentOf(t, fx, "flowComplete")
	fx.steps.Put(&StepRule{DeploymentID: deploymentID, StepCode: "handle", AllowReturn: "0", Sort: 2})
	instance, apply := startBack(t, fx, "flowComplete", "alice")
	completeBack(t, fx, apply.ID, "alice")
	handle := fx.firstBackTask(t, instance.ID)
	// 入参校验
	if err := fx.back.BackToStep(ctx, BackTaskVo{UserCode: "alice"}, "apply"); err == nil || !containsText(err, "taskId 为空") {
		t.Fatalf("taskId 缺失应报错: %v", err)
	}
	if err := fx.back.BackToStep(ctx, BackTaskVo{TaskID: handle.ID}, "apply"); err == nil || !containsText(err, "userCode 为空") {
		t.Fatalf("userCode 缺失应报错: %v", err)
	}
	if err := fx.back.BackToStep(ctx, BackTaskVo{TaskID: handle.ID, UserCode: "alice"}, " "); err == nil || !containsText(err, "targetActivityId 为空") {
		t.Fatalf("目标缺失应报错: %v", err)
	}
	// 任务不存在
	if err := fx.back.BackToStep(ctx, BackTaskVo{TaskID: "no-such", UserCode: "alice"}, "apply"); err == nil || !containsText(err, "任务不存在") {
		t.Fatalf("任务不存在应报错: %v", err)
	}
	// 步骤禁止退回（handle allowReturn=0）
	if err := fx.back.BackToStep(ctx, BackTaskVo{TaskID: handle.ID, UserCode: "alice", Message: "退回"}, "apply"); err == nil || !containsText(err, "当前步骤不允许退回") {
		t.Fatalf("禁止退回应报错: %v", err)
	}
	// 实例挂起
	if err := fx.engine.RuntimeService().SuspendProcessInstance(ctx, instance.ID); err != nil {
		t.Fatal(err)
	}
	fx.steps.Put(&StepRule{DeploymentID: deploymentID, StepCode: "handle", AllowReturn: "1", Sort: 2})
	if err := fx.back.BackToStep(ctx, BackTaskVo{TaskID: handle.ID, UserCode: "alice", Message: "退回"}, "apply"); err == nil || !containsText(err, "流程已暂停") {
		t.Fatalf("挂起实例应拒绝退回: %v", err)
	}
}

// TestBackToStepParallelReject 验证并行退回报错透传与目标校验透传：
// 目标在并行块内、当前在块外时引擎错误（并行流转已结束）原样上抛；目标不存在/不可达同样透传。
// @param t 测试上下文
func TestBackToStepParallelReject(t *testing.T) {
	fx := newBackFixture(t)
	ctx := context.Background()
	// 并行块语料：完成 p1/p2 汇聚后从 s3 退回块内 p1 → 并行流转已结束
	instance, _ := startBack(t, fx, "backParallelStep", "alice")
	// 并行块内两个任务同时在办，逐个完成
	page, err := fx.engine.TaskService().ListTasks(ctx, api.TaskFilter{ProcessInstanceID: instance.ID, Size: 10})
	if err != nil || len(page.Data) != 2 {
		t.Fatalf("并行块内应有 2 个任务: %v %v", page.Data, err)
	}
	for _, task := range page.Data {
		completeBack(t, fx, task.ID, task.Assignee)
	}
	s3 := fx.firstBackTask(t, instance.ID)
	if s3.TaskDefinitionKey != "s3" {
		t.Fatalf("应推进到汇聚后节点 s3: %+v", s3)
	}
	// 并行拒绝透传（分支3：目标在块内、当前在块外）
	if err := fx.back.BackToStep(ctx, BackTaskVo{TaskID: s3.ID, UserCode: "alice", Message: "退回"}, "p1"); err == nil || !containsText(err, "并行流转已结束") {
		t.Fatalf("并行退回应透传引擎拒绝: %v", err)
	}
	// 目标不存在透传
	if err := fx.back.BackToStep(ctx, BackTaskVo{TaskID: s3.ID, UserCode: "alice", Message: "退回"}, "missing-node"); err == nil {
		t.Fatalf("目标不存在应报错")
	}
}

// TestBackToPrevious 验证退回上一节点：串行单入线直迁 + 多入线报错。
// @param t 测试上下文
func TestBackToPrevious(t *testing.T) {
	fx := newBackFixture(t)
	ctx := context.Background()
	// 串行：s2 任务退回上一节点 → s1
	instance, task := startBack(t, fx, "backFlow3", "alice")
	completeBack(t, fx, task.ID, "alice")
	s2 := fx.firstBackTask(t, instance.ID)
	// 入参校验
	if err := fx.back.BackToPrevious(ctx, BackTaskVo{UserCode: "alice"}); err == nil || !containsText(err, "taskId 为空") {
		t.Fatalf("taskId 缺失应报错: %v", err)
	}
	if err := fx.back.BackToPrevious(ctx, BackTaskVo{TaskID: s2.ID}); err == nil || !containsText(err, "userCode 为空") {
		t.Fatalf("userCode 缺失应报错: %v", err)
	}
	// 正常退回上一节点
	if err := fx.back.BackToPrevious(ctx, BackTaskVo{TaskID: s2.ID, UserCode: "alice", Message: "退回上一节点"}); err != nil {
		t.Fatalf("退回上一节点失败: %v", err)
	}
	prev := fx.firstBackTask(t, instance.ID)
	if prev.TaskDefinitionKey != "s1" {
		t.Fatalf("退回上一节点应落位 s1: %+v", prev)
	}
	// 多入线防护：汇聚节点 join 入线不唯一
	instance2, a1 := startBack(t, fx, "backConverge", "alice")
	completeBack(t, fx, a1.ID, "alice")
	join := fx.firstBackTask(t, instance2.ID)
	if err := fx.back.BackToPrevious(ctx, BackTaskVo{TaskID: join.ID, UserCode: "alice"}); err == nil || !containsText(err, "上一节点不唯一") {
		t.Fatalf("多入线应报错: %v", err)
	}
}

// TestBackToPreviousLedger 验证退回上一节点的账本闭合：当前任务待办清理、
// 已办完成时间回写、新落位任务待办生成。
// @param t 测试上下文
func TestBackToPreviousLedger(t *testing.T) {
	fx := newBackFixture(t)
	ctx := context.Background()
	instance, apply := startBack(t, fx, "flowComplete", "alice")
	completeBack(t, fx, apply.ID, "alice")
	handle := fx.firstBackTask(t, instance.ID)
	// 播种 handle 待办与已办镜像
	if _, err := fx.messages.CreateForTask(ctx, &MessageExtend{
		BaseColumns:       BaseColumns{TenantID: "dev"},
		TaskID:            handle.ID,
		ProcessInstanceID: instance.ID,
		TaskStepCode:      "handle",
		OwnerID:           "alice",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.histories.CreateForTask(ctx, &HistoryExtend{MessageExtend: MessageExtend{
		BaseColumns:       BaseColumns{TenantID: "dev"},
		TaskID:            handle.ID,
		ProcessInstanceID: instance.ID,
		TaskStepCode:      "handle",
		OwnerID:           "alice",
	}}); err != nil {
		t.Fatal(err)
	}
	// 空意见：跳过意见写入，不阻断退回
	if err := fx.back.BackToPrevious(ctx, BackTaskVo{TaskID: handle.ID, UserCode: "alice"}); err != nil {
		t.Fatalf("退回上一节点失败: %v", err)
	}
	// 落位 apply 新任务并生成待办
	newApply := fx.firstBackTask(t, instance.ID)
	if newApply.TaskDefinitionKey != "apply" {
		t.Fatalf("退回上一节点应落位 apply: %+v", newApply)
	}
	if _, found, _ := fx.messages.GetByTaskID(ctx, handle.ID); found {
		t.Fatalf("handle 待办应被清理")
	}
	if _, found, err := fx.messages.GetByTaskID(ctx, newApply.ID); err != nil || !found {
		t.Fatalf("新 apply 待办应生成: %v %v", found, err)
	}
	his, found, err := fx.histories.GetByTaskID(ctx, handle.ID)
	if err != nil || !found || his.TaskFinishTime == nil {
		t.Fatalf("已办完成时间应回写: %v %v %+v", found, err, his)
	}
}

// TestGenerateTodosAfterBackBranches 验证按实例重生成待办的幂等与无主任务分支：
// 已有待办的任务跳过（返回其 ID），无处理人任务跳过（组任务待执行人重写补齐）。
// @param t 测试上下文
func TestGenerateTodosAfterBackBranches(t *testing.T) {
	fx := newBackFixture(t)
	ctx := context.Background()
	instance, gtask := startBack(t, fx, "flowGroup", "alice")
	completeBack(t, fx, gtask.ID, "alice")
	gnext := fx.firstBackTask(t, instance.ID)
	// gnext 仅候选组无处理人：跳过生成
	ids, err := fx.back.generateTodosAfterBack(ctx, instance.ID)
	if err != nil || len(ids) != 0 {
		t.Fatalf("无处理人任务应跳过生成: %v %v", ids, err)
	}
	// 已有待办的任务：幂等返回其 ID
	if _, err := fx.messages.CreateForTask(ctx, &MessageExtend{
		BaseColumns:       BaseColumns{TenantID: "dev"},
		TaskID:            gnext.ID,
		ProcessInstanceID: instance.ID,
		OwnerID:           "alice",
	}); err != nil {
		t.Fatal(err)
	}
	ids, err = fx.back.generateTodosAfterBack(ctx, instance.ID)
	if err != nil || len(ids) != 1 || ids[0] != gnext.ID {
		t.Fatalf("已有待办任务应幂等返回: %v %v", ids, err)
	}
}

// TestBackNodeReachableGraph 验证可达性/后继枚举纯函数的子流程分支：
// 子流程内终点提升、容器包含判定、事件子流程开始事件排除、不可达判定与后继展开。
// @param t 测试上下文
func TestBackNodeReachableGraph(t *testing.T) {
	fx := newBackFixture(t)
	ctx := context.Background()
	bpmn, err := fx.engine.RepositoryService().GetBpmnModel(ctx, deploymentOfDefID(t, fx, "backSub"))
	if err != nil {
		t.Fatal(err)
	}
	process := bpmn.Processes[0]
	subEl, _ := findFlowElement(process, "sub")
	sub := subEl.(*model.SubProcess)
	u1El, _ := findFlowElement(process, "u1")
	u1 := u1El.(model.FlowNode)
	afterEl, _ := findFlowElement(process, "after")
	after := afterEl.(model.FlowNode)
	esStartEl, _ := findFlowElement(process, "esStart")
	esStart := esStartEl.(model.FlowNode)
	// 子流程内任务经终点提升可达子流程后节点
	if !isNodeReachable(process, u1, after) {
		t.Fatalf("u1 应可达 after")
	}
	// 容器节点可达其内部元素
	if !isNodeReachable(process, sub, u1) {
		t.Fatalf("sub 应可达其内部 u1")
	}
	// 自环可达
	if !isNodeReachable(process, u1, u1) {
		t.Fatalf("节点自身应可达")
	}
	// 子流程后节点不可达子流程内部（外部无入线路径）
	if isNodeReachable(process, after, u1) {
		t.Fatalf("after 不应可达子流程内部 u1")
	}
	// 事件子流程开始事件不构成路径起点
	if isNodeReachable(process, esStart, after) {
		t.Fatalf("事件子流程开始事件不应构成路径起点")
	}
	// 后继枚举：sub 的后继含内部元素与子流程后节点、结束事件
	successors := nextActivityIDsAfter(process, "sub")
	set := map[string]bool{}
	for _, id := range successors {
		set[id] = true
	}
	for _, expected := range []string{"subStart", "u1", "subEnd", "after", "end"} {
		if !set[expected] {
			t.Fatalf("sub 后继应包含 %s: %v", expected, successors)
		}
	}
	if set["sub"] {
		t.Fatalf("sub 后继不应包含自身: %v", successors)
	}
	// 无名称节点展示名回退节点 ID
	if name := nodeDisplayName(u1); name != "u1" {
		t.Fatalf("无名称节点应回退 ID: %s", name)
	}
}

// deploymentOfDefID 按流程 Key 取定义 ID。
// @param t 测试上下文
// @param f 测试环境
// @param key 流程定义 Key
// @return 定义 ID
func deploymentOfDefID(t *testing.T, f *backFixture, key string) string {
	t.Helper()
	definition, err := f.engine.RepositoryService().GetProcessDefinitionByKey(context.Background(), key)
	if err != nil {
		t.Fatalf("查询定义 %s 失败: %v", key, err)
	}
	return definition.ID
}

// TestBackServiceHelpers 验证退回服务辅助分支：无身份查询时办理人保留 Id、
// 历史缺失任务跳过、空办理人跳过、nil 步骤存储排序索引、nil 时钟构造。
// @param t 测试上下文
func TestBackServiceHelpers(t *testing.T) {
	fx := newBackFixture(t)
	ctx := context.Background()
	// nil 时钟构造（回退真实时钟）与 nil 步骤排序索引守卫
	plain := NewBackService(fx.engine, NewModelMetaService(fx.engine), fx.next, fx.comments, fx.operations, fx.messages, fx.histories, fx.documents, nil, engineInstanceQuery{runtime: fx.engine.RuntimeService(), history: fx.engine.HistoryService()}, nil)
	if index, err := plain.stepSortIndex(ctx, "any"); err != nil || len(index) != 0 {
		t.Fatalf("nil 步骤服务排序索引应为空: %v %v", index, err)
	}
	// 无身份查询服务：办理人保留身份 Id
	noUsers := NewBackService(fx.engine, NewModelMetaService(fx.engine), fx.next, fx.comments, fx.operations, fx.messages, fx.histories, fx.documents, fx.stepSvc, engineInstanceQuery{runtime: fx.engine.RuntimeService(), history: fx.engine.HistoryService()}, fx.fakeClock)
	if name := noUsers.displayName(ctx, "alice"); name != "alice" {
		t.Fatalf("无身份查询应保留身份 Id: %s", name)
	}
	// 历史缺失任务：跳过（运行任务回退也失败）
	if names := noUsers.assigneeNamesOf(ctx, []string{"no-such-task"}); len(names) != 0 {
		t.Fatalf("缺失任务应跳过: %v", names)
	}
	// 运行中任务回退：申请任务仍在办时按运行任务取办理人
	instance, apply := startBack(t, fx, "flowComplete", "alice")
	if names := noUsers.assigneeNamesOf(ctx, []string{apply.ID}); len(names) != 1 || names[0] != "alice" {
		t.Fatalf("运行任务应回退取办理人: %v", names)
	}
	_ = instance
}

// TestBackNodesDisplayNamesWithoutUsers 验证可退节点视图在无身份查询时保留身份 Id。
// @param t 测试上下文
func TestBackNodesDisplayNamesWithoutUsers(t *testing.T) {
	fx := newBackFixture(t)
	ctx := context.Background()
	noUsers := NewBackService(fx.engine, NewModelMetaService(fx.engine), fx.next, fx.comments, fx.operations, fx.messages, fx.histories, fx.documents, fx.stepSvc, engineInstanceQuery{runtime: fx.engine.RuntimeService(), history: fx.engine.HistoryService()}, fx.fakeClock)
	instance, task := startBack(t, fx, "backFlow3", "alice")
	completeBack(t, fx, task.ID, "alice")
	s2 := fx.firstBackTask(t, instance.ID)
	views, err := noUsers.BackNodes(ctx, s2.ID)
	if err != nil || len(views) != 1 {
		t.Fatalf("可退节点计算失败: %v %v", views, err)
	}
	if len(views[0].AssigneeNames) != 1 || views[0].AssigneeNames[0] != "alice" {
		t.Fatalf("无身份查询应保留身份 Id: %+v", views[0].AssigneeNames)
	}
}
