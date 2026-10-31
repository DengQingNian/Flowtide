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

// ─── 推演测试语料（全部为本文件专用，前缀 sim 避免与他人语料冲突）───

// simLinearProcess 是线性语料：apply(${initiator}) → handle(${initiator}) → end。
const simLinearProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="simLinear" name="线性推演流程" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="apply"/>
    <userTask id="apply" name="申请" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f1" sourceRef="apply" targetRef="handle"/>
    <userTask id="handle" name="办理" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f2" sourceRef="handle" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// simGatewayProcess 是排他网关语料：gate 命中 ${pass == true} 走 approve，否则默认流走 review。
const simGatewayProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="simGateway" name="网关推演流程" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="apply"/>
    <userTask id="apply" name="申请" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f1" sourceRef="apply" targetRef="gate"/>
    <exclusiveGateway id="gate" default="f3"/>
    <sequenceFlow id="f2" sourceRef="gate" targetRef="approve">
      <conditionExpression>${pass == true}</conditionExpression>
    </sequenceFlow>
    <sequenceFlow id="f3" sourceRef="gate" targetRef="review"/>
    <userTask id="approve" name="批准" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f4" sourceRef="approve" targetRef="end"/>
    <userTask id="review" name="复核" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f5" sourceRef="review" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// simParallelProcess 是并行网关语料：fork 分出 ta/tb 两分支，join 汇聚后办结。
const simParallelProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="simParallel" name="并行推演流程" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="apply"/>
    <userTask id="apply" name="申请" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f1" sourceRef="apply" targetRef="fork"/>
    <parallelGateway id="fork"/>
    <sequenceFlow id="f2" sourceRef="fork" targetRef="ta"/>
    <sequenceFlow id="f3" sourceRef="fork" targetRef="tb"/>
    <userTask id="ta" name="分支A" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f4" sourceRef="ta" targetRef="join"/>
    <userTask id="tb" name="分支B" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f5" sourceRef="tb" targetRef="join"/>
    <parallelGateway id="join"/>
    <sequenceFlow id="f6" sourceRef="join" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// simSubProcessProcess 是子流程语料：apply → 子流程( inner(${initiator}) ) → after(${initiator}) → end。
const simSubProcessProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="simSub" name="子流程推演流程" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="apply"/>
    <userTask id="apply" name="申请" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f1" sourceRef="apply" targetRef="sub"/>
    <subProcess id="sub" name="子流程">
      <startEvent id="sStart"/>
      <sequenceFlow id="sf0" sourceRef="sStart" targetRef="inner"/>
      <userTask id="inner" name="内部任务" flowable:assignee="${initiator}"/>
      <sequenceFlow id="sf1" sourceRef="inner" targetRef="sEnd"/>
      <endEvent id="sEnd"/>
    </subProcess>
    <sequenceFlow id="f2" sourceRef="sub" targetRef="after"/>
    <userTask id="after" name="善后" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f3" sourceRef="after" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// simExecutorProcess 是执行人形态语料：literal（字面 assignee）与 var（${owner} 表单取值）两支。
const simExecutorProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="simExec" name="执行人推演流程" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="literal"/>
    <userTask id="literal" name="字面节点" flowable:assignee="bob"/>
    <sequenceFlow id="f1" sourceRef="literal" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// simVarExecutorProcess 是 ${owner} / ${entity.owner} 表单取值语料。
const simVarExecutorProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="simVarExec" name="变量执行人流程" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="varNode"/>
    <userTask id="varNode" name="变量节点" flowable:assignee="${entity.owner}"/>
    <sequenceFlow id="f1" sourceRef="varNode" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// simGroupExecutorProcess 是候选组语料：grpNode 走 grpA（有成员 alice/bob），dynNode 走 dynRole（无成员→动态角色）。
const simGroupExecutorProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="simGroup" name="候选组流程" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="grpNode"/>
    <userTask id="grpNode" name="组节点" flowable:candidateGroups="grpA"/>
    <sequenceFlow id="f1" sourceRef="grpNode" targetRef="dynNode"/>
    <userTask id="dynNode" name="动态角色节点" flowable:candidateGroups="dynRole"/>
    <sequenceFlow id="f2" sourceRef="dynNode" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// simExecutorListProcess 是 ${executor} 语料：pNode 人员直用 / oNode 组织展开 / rNode 角色展开 / dNode 动态角色。
const simExecutorListProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="simExecutorList" name="执行者配置流程" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="pNode"/>
    <userTask id="pNode" name="人员节点" flowable:assignee="${executor}"
              executorType="person" executorList='[{"identityId":"carl"},{"identityId":"dave"}]'/>
    <sequenceFlow id="f1" sourceRef="pNode" targetRef="oNode"/>
    <userTask id="oNode" name="组织节点" flowable:assignee="${executor}"
              executorType="org" executorList='[{"orgId":"grpA"}]'/>
    <sequenceFlow id="f2" sourceRef="oNode" targetRef="rNode"/>
    <userTask id="rNode" name="角色节点" flowable:assignee="${executor}"
              executorType="role" executorList='[{"roleId":"grpB"}]'/>
    <sequenceFlow id="f3" sourceRef="rNode" targetRef="dNode"/>
    <userTask id="dNode" name="动态角色配置节点" flowable:assignee="${executor}"
              executorType="role" executorList='[{"roleId":"dynRole"}]'/>
    <sequenceFlow id="f4" sourceRef="dNode" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// simCountersignProcess 是会签推演语料：mtask（多实例 ${executor} 人员配置）→ end。
const simCountersignProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="simCounter" name="会签推演流程" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="mtask"/>
    <userTask id="mtask" name="会签任务" flowable:assignee="${executor}"
              executorType="person" executorList='[{"identityId":"alice"},{"identityId":"bob"}]'>
      <multiInstanceLoopCharacteristics isSequential="false"/>
    </userTask>
    <sequenceFlow id="f1" sourceRef="mtask" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// simCallProcess 是调用活动边界语料：apply → call（边界不深入）→ end。
const simCallProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="simCall" name="调用活动流程" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="apply"/>
    <userTask id="apply" name="申请" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f1" sourceRef="apply" targetRef="call"/>
    <callActivity id="call" name="调用活动" calledElement="simLinear"/>
    <sequenceFlow id="f2" sourceRef="call" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// simUsers 是推演测试用户（Id → 姓名）。
var simUsers = []api.User{
	{ID: "alice", DisplayName: "张三"},
	{ID: "bob", DisplayName: "李四"},
	{ID: "carl", DisplayName: "王五"},
	{ID: "dave", DisplayName: "赵六"},
}

// simFixture 是推演测试环境（内存引擎 + 步骤规则桩 + 推演服务）。
type simFixture struct {
	t            *testing.T
	engine       api.Engine            // 内存引擎
	stores       *MemStores            // 域内存存储
	steps        *MemStepStore         // his_step 步骤规则桩
	fake         *clock.Fake           // 假时钟
	meta         *ModelMetaService     // 模型元数据
	next         *NextActivityService  // 下一步预生成
	messages     *MessageExtendService // 待办扩展
	histories    *HistoryExtendService // 已办扩展
	sim          *SimulateService      // 推演服务
	simNoSteps   *SimulateService      // 未注入步骤规则的推演服务（跳过退回模式分支）
	deploymentID string                // 语料部署 Id
}

// newSimFixture 构造推演测试环境（部署全部语料、建用户与组、装配服务）。
// @param t 测试上下文
// @return 测试环境
func newSimFixture(t *testing.T) *simFixture {
	t.Helper()
	fake := clock.NewFake(time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC))
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
	deploy, err := eng.RepositoryService().Deploy(context.Background(), api.DeploymentRequest{
		Name:     "simulate-test",
		TenantID: "dev",
		Resources: map[string][]byte{
			"linear.bpmn20.xml":  []byte(simLinearProcess),
			"gateway.bpmn20.xml": []byte(simGatewayProcess),
			"parallel.bpmn20.xml": []byte(simParallelProcess),
			"sub.bpmn20.xml":      []byte(simSubProcessProcess),
			"exec.bpmn20.xml":     []byte(simExecutorProcess),
			"varexec.bpmn20.xml":  []byte(simVarExecutorProcess),
			"group.bpmn20.xml":    []byte(simGroupExecutorProcess),
			"executorlist.bpmn20.xml": []byte(simExecutorListProcess),
			"counter.bpmn20.xml":      []byte(simCountersignProcess),
			"call.bpmn20.xml":         []byte(simCallProcess),
		},
	})
	if err != nil {
		t.Fatalf("部署失败: %v", err)
	}
	// 创建用户与组（grpA/grpB 有成员，dynRole 故意无成员→动态角色）
	ctx := context.Background()
	for _, user := range simUsers {
		if _, err := eng.IdentityService().CreateUser(ctx, user); err != nil {
			t.Fatalf("创建用户 %s 失败: %v", user.ID, err)
		}
	}
	for _, group := range []api.Group{{ID: "grpA", Name: "组A"}, {ID: "grpB", Name: "组B"}, {ID: "dynRole", Name: "动态角色"}} {
		if _, err := eng.IdentityService().CreateGroup(ctx, group); err != nil {
			t.Fatalf("创建组 %s 失败: %v", group.ID, err)
		}
	}
	for _, pair := range [][2]string{{"alice", "grpA"}, {"bob", "grpA"}, {"carl", "grpB"}} {
		if err := eng.IdentityService().AddMembership(ctx, pair[0], pair[1]); err != nil {
			t.Fatalf("添加成员 %v 失败: %v", pair, err)
		}
	}
	fx := &simFixture{t: t, engine: eng, fake: fake, deploymentID: deploy.ID}
	fx.stores = NewMemStores(fake)
	fx.steps = NewMemStepStore()
	fx.meta = NewModelMetaService(eng)
	fx.next = NewNextActivityService(fx.stores.Next, fake)
	fx.messages = NewMessageExtendService(fx.stores.Message, fake)
	fx.histories = NewHistoryExtendService(fx.stores.History, fake)
	fx.sim = NewSimulateService(eng, fx.meta, fx.next, fx.histories, fx.messages, eng.IdentityService(), fake, WithSimulateSteps(fx.steps))
	// 未注入步骤规则的对照实例（跳过退回模式分支）
	fx.simNoSteps = NewSimulateService(eng, fx.meta, fx.next, fx.histories, fx.messages, eng.IdentityService(), fake)
	return fx
}

// start 发起流程实例并返回实例与首个任务。
// @param definitionKey 流程定义 Key
// @param initiator 发起人
// @param vars 启动变量
// @return 流程实例
// @return 首个任务
func (f *simFixture) start(definitionKey, initiator string, vars map[string]any) (api.ProcessInstance, api.Task) {
	f.t.Helper()
	variables := make([]api.Variable, 0, len(vars)+1)
	variables = append(variables, api.Variable{Name: VarInitiator, Value: initiator})
	for name, value := range vars {
		variables = append(variables, api.Variable{Name: name, Value: value})
	}
	instance, err := f.engine.RuntimeService().StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: definitionKey,
		BusinessKey:          "BIZ-" + definitionKey,
		Name:                 "推演-" + definitionKey,
		TenantID:             "dev",
		UserID:               initiator,
		Variables:            variables,
	})
	if err != nil {
		f.t.Fatalf("发起失败: %v", err)
	}
	return instance, f.firstTask(instance.ID)
}

// firstTask 返回实例当前首个任务。
// @param instanceID 实例 Id
// @return 首个任务
func (f *simFixture) firstTask(instanceID string) api.Task {
	f.t.Helper()
	page, err := f.engine.TaskService().ListTasks(context.Background(), api.TaskFilter{ProcessInstanceID: instanceID, Size: 10})
	if err != nil {
		f.t.Fatalf("查询任务失败: %v", err)
	}
	if len(page.Data) == 0 {
		f.t.Fatalf("实例 %s 无任务", instanceID)
	}
	return page.Data[0]
}

// allTasks 返回实例当前全部任务 Key（一致性断言用）。
// @param instanceID 实例 Id
// @return 任务 Key 列表
func (f *simFixture) allTaskKeys(instanceID string) []string {
	f.t.Helper()
	page, err := f.engine.TaskService().ListTasks(context.Background(), api.TaskFilter{ProcessInstanceID: instanceID, Size: 20})
	if err != nil {
		f.t.Fatalf("查询任务失败: %v", err)
	}
	keys := make([]string, 0, len(page.Data))
	for _, task := range page.Data {
		keys = append(keys, task.TaskDefinitionKey)
	}
	return keys
}

// complete 以指定变量完成任务。
// @param taskID 任务 Id
// @param operator 操作人
// @param vars 流程变量
func (f *simFixture) complete(taskID, operator string, vars map[string]any) {
	f.t.Helper()
	req := api.CompleteTaskRequest{UserID: operator}
	for name, value := range vars {
		req.Variables = append(req.Variables, api.Variable{Name: name, Value: value})
	}
	if err := f.engine.TaskService().Complete(context.Background(), taskID, req); err != nil {
		f.t.Fatalf("办结任务失败: %v", err)
	}
}

// viewIDs 提取视图列表的节点 Id。
// @param views 推演视图
// @return 节点 Id 列表
func viewIDs(views []NextUserTaskView) []string {
	ids := make([]string, 0, len(views))
	for _, view := range views {
		ids = append(ids, view.ActivityID)
	}
	return ids
}

// assertSameStrings 断言两个字符串集合逐项相等（保序）。
// @param t 测试上下文
// @param got 实际值
// @param want 期望值
func assertSameStrings(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("集合不一致: got=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("集合不一致: got=%v want=%v", got, want)
		}
	}
}

// seedTaskMessage 为任务播种待办扩展（退回步骤 sender 信息）。
// @param instanceID 实例 Id
// @param task 任务
// @param ownerID 处理人身份 Id
// @param senderStepCode 发送步骤代号
// @param senderStepName 发送步骤名称
func (f *simFixture) seedTaskMessage(instanceID string, task api.Task, ownerID, senderStepCode, senderStepName string) {
	f.t.Helper()
	if _, err := f.messages.CreateForTask(context.Background(), &MessageExtend{
		BaseColumns:       BaseColumns{TenantID: "dev"},
		TaskID:            task.ID,
		ProcessInstanceID: instanceID,
		OwnerID:           ownerID,
		SenderStepCode:    senderStepCode,
		SenderStepName:    senderStepName,
	}); err != nil {
		f.t.Fatalf("播种待办失败: %v", err)
	}
}

// seedNext 为实例节点播种历史执行人（his_next_activity，used=1 双写）。
// @param instanceID 实例 Id
// @param activityID 节点 Id
// @param identityID 执行人 Id
// @param taskID 关联任务 Id
// @param preTaskID 来源任务 Id
func (f *simFixture) seedNext(instanceID, activityID, identityID, taskID, preTaskID string) {
	f.t.Helper()
	if err := f.next.InsertDual(context.Background(), &NextActivity{
		BaseColumns:       BaseColumns{TenantID: "dev"},
		NextActivityID:    activityID,
		IdentityID:        identityID,
		ProcessInstanceID: instanceID,
		Used:              1,
		TaskID:            taskID,
		PreTaskID:         preTaskID,
		Version:           1,
	}, "tester"); err != nil {
		f.t.Fatalf("播种下一步记录失败: %v", err)
	}
}

// ─── 一致性回归：推演的下一节点/人员 = 实际流转生成的任务（常驻★）───

// TestSimulateConsistencyLinear 线性语料一致性：推演 handle → 实际流转生成 handle 任务。
func TestSimulateConsistencyLinear(t *testing.T) {
	fx := newSimFixture(t)
	instance, task := fx.start("simLinear", "alice", nil)
	// 推演：apply 的下一步 = handle（发起人 alice）
	views, err := fx.sim.SimulateByTask(context.Background(), task.ID, nil)
	if err != nil {
		t.Fatalf("推演失败: %v", err)
	}
	assertSameStrings(t, viewIDs(views), []string{"handle"})
	if views[0].ActivityName != "办理" || len(views[0].ExecutorIDs) != 1 || views[0].ExecutorIDs[0] != "alice" {
		t.Fatalf("推演执行人不符: %+v", views[0])
	}
	if views[0].ExecutorNames[0] != "张三" {
		t.Fatalf("执行人姓名补全不符: %+v", views[0])
	}
	// 实际流转
	fx.complete(task.ID, "alice", nil)
	assertSameStrings(t, fx.allTaskKeys(instance.ID), []string{"handle"})
	// handle 的推演 = 办结伪节点
	handleTask := fx.firstTask(instance.ID)
	views, err = fx.sim.SimulateByTask(context.Background(), handleTask.ID, nil)
	if err != nil {
		t.Fatalf("推演失败: %v", err)
	}
	assertSameStrings(t, viewIDs(views), []string{"end"})
	if !strings.Contains(views[0].ActivityName, "办结") {
		t.Fatalf("办结节点名不符: %+v", views[0])
	}
	// 实际办结后无任务（与 end 伪节点一致）
	fx.complete(handleTask.ID, "alice", nil)
	if keys := fx.allTaskKeys(instance.ID); len(keys) != 0 {
		t.Fatalf("实例应已办结: %v", keys)
	}
}

// TestSimulateConsistencyGateway 排他网关两条条件走向一致性。
func TestSimulateConsistencyGateway(t *testing.T) {
	fx := newSimFixture(t)
	// 走向一：pass=true → approve
	instance, task := fx.start("simGateway", "alice", nil)
	views, err := fx.sim.SimulateByTask(context.Background(), task.ID, map[string]any{"pass": true})
	if err != nil {
		t.Fatalf("推演失败: %v", err)
	}
	assertSameStrings(t, viewIDs(views), []string{"approve"})
	fx.complete(task.ID, "alice", map[string]any{"pass": true})
	assertSameStrings(t, fx.allTaskKeys(instance.ID), []string{"approve"})
	// 走向二：pass=false → 默认流 review
	instance2, task2 := fx.start("simGateway", "alice", nil)
	views, err = fx.sim.SimulateByTask(context.Background(), task2.ID, map[string]any{"pass": false})
	if err != nil {
		t.Fatalf("推演失败: %v", err)
	}
	assertSameStrings(t, viewIDs(views), []string{"review"})
	fx.complete(task2.ID, "alice", map[string]any{"pass": false})
	assertSameStrings(t, fx.allTaskKeys(instance2.ID), []string{"review"})
}

// TestSimulateConsistencyParallel 并行网关一致性：推演双分支 = 实际生成双任务。
func TestSimulateConsistencyParallel(t *testing.T) {
	fx := newSimFixture(t)
	instance, task := fx.start("simParallel", "alice", nil)
	views, err := fx.sim.SimulateByTask(context.Background(), task.ID, nil)
	if err != nil {
		t.Fatalf("推演失败: %v", err)
	}
	// 双分支按节点 Id 排序：ta、tb
	assertSameStrings(t, viewIDs(views), []string{"ta", "tb"})
	fx.complete(task.ID, "alice", nil)
	keys := fx.allTaskKeys(instance.ID)
	if len(keys) != 2 {
		t.Fatalf("应生成两个并行任务: %v", keys)
	}
	assertSameStrings(t, keys, []string{"ta", "tb"})
}

// TestSimulateConsistencySubProcess 子流程一致性：进入子流程首节点、穿出后到 after。
func TestSimulateConsistencySubProcess(t *testing.T) {
	fx := newSimFixture(t)
	instance, task := fx.start("simSub", "alice", nil)
	// apply 推演 → 进入子流程首个节点 inner
	views, err := fx.sim.SimulateByTask(context.Background(), task.ID, nil)
	if err != nil {
		t.Fatalf("推演失败: %v", err)
	}
	assertSameStrings(t, viewIDs(views), []string{"inner"})
	fx.complete(task.ID, "alice", nil)
	assertSameStrings(t, fx.allTaskKeys(instance.ID), []string{"inner"})
	// inner 推演 → 子流程结束上溯到主流程 after
	innerTask := fx.firstTask(instance.ID)
	views, err = fx.sim.SimulateByTask(context.Background(), innerTask.ID, nil)
	if err != nil {
		t.Fatalf("推演失败: %v", err)
	}
	assertSameStrings(t, viewIDs(views), []string{"after"})
	fx.complete(innerTask.ID, "alice", nil)
	assertSameStrings(t, fx.allTaskKeys(instance.ID), []string{"after"})
}

// TestSimulateConsistencyWithEntity 推演含 entity 取值表达式与实际流转的一致性。
func TestSimulateConsistencyWithEntity(t *testing.T) {
	fx := newSimFixture(t)
	instance, task := fx.start("simVarExec", "alice", map[string]any{
		"entity": map[string]any{"owner": "bob"},
	})
	// ${entity.owner} → 实际首任务执行人 = bob（与推演解析同源）
	if task.Assignee != "bob" {
		t.Fatalf("entity 取值执行人不符: %+v", task)
	}
	// varNode 推演 → 办结伪节点（结束无执行人）
	views, err := fx.sim.SimulateByTask(context.Background(), task.ID, nil)
	if err != nil {
		t.Fatalf("推演失败: %v", err)
	}
	assertSameStrings(t, viewIDs(views), []string{SimEndActivityID})
	// 实际办结后无任务（与 end 伪节点一致）
	fx.complete(task.ID, "bob", nil)
	if keys := fx.allTaskKeys(instance.ID); len(keys) != 0 {
		t.Fatalf("实例应已办结: %v", keys)
	}
}

// ─── SimulateByKey：未发起实例按定义推演 ───

// TestSimulateByKeyLinear 按 Key 推演线性语料：从开始事件得首任务 apply（含 initiator 注入）。
func TestSimulateByKeyLinear(t *testing.T) {
	fx := newSimFixture(t)
	views, err := fx.sim.SimulateByKey(context.Background(), "dev", "simLinear", "BIZ-1", map[string]any{VarInitiator: "alice"})
	if err != nil {
		t.Fatalf("按 Key 推演失败: %v", err)
	}
	assertSameStrings(t, viewIDs(views), []string{"apply"})
	if len(views[0].ExecutorIDs) != 1 || views[0].ExecutorIDs[0] != "alice" {
		t.Fatalf("initiator 执行人不符: %+v", views[0])
	}
}

// TestSimulateByKeyVarExecutor 按 Key 推演 ${entity.owner} 形态（表单 Map 与平铺变量两形态）。
func TestSimulateByKeyVarExecutor(t *testing.T) {
	fx := newSimFixture(t)
	// entity 表单 Map 形态
	views, err := fx.sim.SimulateByKey(context.Background(), "dev", "simVarExec", "BIZ-1", map[string]any{
		"entity": map[string]any{"owner": "carl"},
	})
	if err != nil {
		t.Fatalf("按 Key 推演失败: %v", err)
	}
	if len(views) != 1 || len(views[0].ExecutorIDs) != 1 || views[0].ExecutorIDs[0] != "carl" {
		t.Fatalf("entity 表单取值不符: %+v", views)
	}
	// 平铺变量形态（entity. 前缀剥除后取顶层键）
	views, err = fx.sim.SimulateByKey(context.Background(), "dev", "simVarExec", "BIZ-1", map[string]any{"owner": "dave"})
	if err != nil {
		t.Fatalf("按 Key 推演失败: %v", err)
	}
	if len(views) != 1 || len(views[0].ExecutorIDs) != 1 || views[0].ExecutorIDs[0] != "dave" {
		t.Fatalf("平铺变量取值不符: %+v", views)
	}
}

// TestSimulateByKeyNoEnd 按 Key 推演不包含办结伪节点（直达结束的语料应返回空）。
func TestSimulateByKeyNoEnd(t *testing.T) {
	fx := newSimFixture(t)
	views, err := fx.sim.SimulateByKey(context.Background(), "dev", "simLinear", "BIZ-1", nil)
	if err != nil {
		t.Fatalf("按 Key 推演失败: %v", err)
	}
	// 从开始事件推演只会给出 apply，不会有 end 伪节点
	for _, view := range views {
		if view.ActivityID == SimEndActivityID {
			t.Fatalf("按 Key 推演不应包含 end 伪节点: %+v", views)
		}
	}
}

// TestSimulateByKeyErrors 按 Key 推演的参数与定义校验。
func TestSimulateByKeyErrors(t *testing.T) {
	fx := newSimFixture(t)
	// 空 Key
	if _, err := fx.sim.SimulateByKey(context.Background(), "dev", "", "BIZ-1", nil); err == nil {
		t.Fatalf("空 Key 应报错")
	}
	// 不存在的 Key
	if _, err := fx.sim.SimulateByKey(context.Background(), "dev", "no-such-def", "BIZ-1", nil); err == nil {
		t.Fatalf("未知定义应报错")
	}
}

// ─── 执行人解析链 ───

// TestSimulateExecutorShapes 全执行人形态：字面/变量/候选用户/候选组/动态角色/executorList 各型。
func TestSimulateExecutorShapes(t *testing.T) {
	fx := newSimFixture(t)
	ctx := context.Background()
	// 字面 assignee=bob
	views, err := fx.sim.SimulateByKey(ctx, "dev", "simExec", "BIZ-1", nil)
	if err != nil {
		t.Fatalf("推演失败: %v", err)
	}
	if len(views) != 1 || len(views[0].ExecutorIDs) != 1 || views[0].ExecutorIDs[0] != "bob" || views[0].ExecutorNames[0] != "李四" {
		t.Fatalf("字面 assignee 不符: %+v", views)
	}
	// 候选组 grpA → alice/bob；动态角色 dynRole → DynamicRoleIDs
	views, err = fx.sim.SimulateByKey(ctx, "dev", "simGroup", "BIZ-1", nil)
	if err != nil {
		t.Fatalf("推演失败: %v", err)
	}
	if len(views) != 1 || len(views[0].ExecutorIDs) != 2 {
		t.Fatalf("候选组展开不符: %+v", views)
	}
	if views[0].ExecutorIDs[0] != "alice" || views[0].ExecutorIDs[1] != "bob" {
		t.Fatalf("候选组成员不符: %+v", views[0])
	}
	// executorList 链：按 Key 推演首节点 pNode（person 配置直用身份 Id）
	views, err = fx.sim.SimulateByKey(ctx, "dev", "simExecutorList", "BIZ-1", nil)
	if err != nil {
		t.Fatalf("executorList 按 Key 推演失败: %v", err)
	}
	if len(views) != 1 || views[0].ActivityID != "pNode" {
		t.Fatalf("executorList 首节点不符: %+v", views)
	}
	assertSameStrings(t, views[0].ExecutorIDs, []string{"carl", "dave"})
	// 真实实例逐步推进：pNode 办结后推演 oNode（org 配置 grpA 展开成员）
	instance, task := fx.start("simExecutorList", "alice", map[string]any{"executor": "carl"})
	views, err = fx.sim.SimulateByTask(ctx, task.ID, nil)
	if err != nil {
		t.Fatalf("pNode 推演失败: %v", err)
	}
	assertSameStrings(t, viewIDs(views), []string{"oNode"})
	assertSameStrings(t, views[0].ExecutorIDs, []string{"alice", "bob"})
	fx.complete(task.ID, "carl", nil)
	oTask := fx.firstTask(instance.ID)
	views, err = fx.sim.SimulateByTask(ctx, oTask.ID, nil)
	if err != nil {
		t.Fatalf("oNode 推演失败: %v", err)
	}
	assertSameStrings(t, viewIDs(views), []string{"rNode"})
	// rNode：role 配置 grpB 展开成员 carl
	assertSameStrings(t, views[0].ExecutorIDs, []string{"carl"})
	fx.complete(oTask.ID, "carl", nil)
	rTask := fx.firstTask(instance.ID)
	views, err = fx.sim.SimulateByTask(ctx, rTask.ID, nil)
	if err != nil {
		t.Fatalf("rNode 推演失败: %v", err)
	}
	assertSameStrings(t, viewIDs(views), []string{"dNode"})
	// dNode：role 配置 dynRole 无成员 → 动态角色标记（不产出执行人）
	if len(views[0].ExecutorIDs) != 0 {
		t.Fatalf("动态角色不应解析出执行人: %+v", views[0])
	}
	assertSameStrings(t, views[0].DynamicRoleIDs, []string{"dynRole"})
	// 会签节点：多实例 ${executor} 人员配置按 executorList 展开
	views, err = fx.sim.SimulateByKey(ctx, "dev", "simCounter", "BIZ-1", nil)
	if err != nil {
		t.Fatalf("会签按 Key 推演失败: %v", err)
	}
	if len(views) != 1 || views[0].ActivityID != "mtask" {
		t.Fatalf("会签节点不符: %+v", views)
	}
	assertSameStrings(t, views[0].ExecutorIDs, []string{"alice", "bob"})
	// 调用活动：边界节点不深入子定义，推演视图停在 call 本身
	instance, task = fx.start("simCall", "alice", nil)
	views, err = fx.sim.SimulateByTask(ctx, task.ID, nil)
	if err != nil {
		t.Fatalf("调用活动推演失败: %v", err)
	}
	assertSameStrings(t, viewIDs(views), []string{"call"})
}

// ─── 退回模式（his_step 退回审批步骤 + 待办扩展 sender_step）───

// seedReturnScenario 构造"批准人退回后办理人再提交"场景：
// 语料 simLinear（apply → handle），handle 任务挂待办扩展（sender=approve，排序 3 > 2 触发退回位），
// approve 节点播种历史执行人 bob（his_next_activity）。
// @param returnMode 退回审批模式原始值（"" / "1" / "3" / "4" / "5"）
// @return 流程实例
// @return 当前 handle 任务
func (f *simFixture) seedReturnScenario(returnMode string) (api.ProcessInstance, api.Task) {
	f.t.Helper()
	// 步骤规则：apply(1) → handle(2, 退回模式) → approve(3)
	for _, rule := range []*StepRule{
		{DeploymentID: f.deploymentID, StepCode: "apply", StepName: "申请", Sort: 1},
		{DeploymentID: f.deploymentID, StepCode: "handle", StepName: "办理", AllowReturn: "1", ReturnApprovalStep: returnMode, Sort: 2},
		{DeploymentID: f.deploymentID, StepCode: "approve", StepName: "批准", Sort: 3},
	} {
		f.steps.Put(rule)
	}
	instance, applyTask := f.start("simLinear", "alice", nil)
	// apply 正常办结，推进到 handle 任务（退回场景的持有者）
	f.complete(applyTask.ID, "alice", nil)
	task := f.firstTask(instance.ID)
	f.seedTaskMessage(instance.ID, task, "alice", "approve", "批准")
	f.seedNext(instance.ID, "approve", "bob", task.ID, "")
	return instance, task
}

// TestSimulateReturnMode1OnlyBack 模式 1：只给退回步骤，人员取历史设置。
func TestSimulateReturnMode1OnlyBack(t *testing.T) {
	fx := newSimFixture(t)
	_, task := fx.seedReturnScenario("1")
	views, err := fx.sim.SimulateByTask(context.Background(), task.ID, nil)
	if err != nil {
		t.Fatalf("推演失败: %v", err)
	}
	assertSameStrings(t, viewIDs(views), []string{"approve"})
	if !views[0].ReturnStep {
		t.Fatalf("退回步骤应带 ReturnStep 标记: %+v", views[0])
	}
	assertSameStrings(t, views[0].ExecutorIDs, []string{"bob"})
	assertSameStrings(t, views[0].ExecutorNames, []string{"李四"})
}

// TestSimulateReturnMode3BackFirst 模式 3 退回优先：退回步骤在前，常规下一步在后。
func TestSimulateReturnMode3BackFirst(t *testing.T) {
	fx := newSimFixture(t)
	_, task := fx.seedReturnScenario("3")
	views, err := fx.sim.SimulateByTask(context.Background(), task.ID, nil)
	if err != nil {
		t.Fatalf("推演失败: %v", err)
	}
	// handle 的常规下一步是办结伪节点；退回步骤 approve 排在最前
	assertSameStrings(t, viewIDs(views), []string{"approve", SimEndActivityID})
	if !views[0].ReturnStep || views[1].ReturnStep {
		t.Fatalf("ReturnStep 标记不符: %+v", views)
	}
}

// TestSimulateReturnMode4BackLast 模式 4 下一步优先：常规下一步在前，退回步骤在后。
func TestSimulateReturnMode4BackLast(t *testing.T) {
	fx := newSimFixture(t)
	_, task := fx.seedReturnScenario("4")
	views, err := fx.sim.SimulateByTask(context.Background(), task.ID, nil)
	if err != nil {
		t.Fatalf("推演失败: %v", err)
	}
	assertSameStrings(t, viewIDs(views), []string{SimEndActivityID, "approve"})
	if views[0].ReturnStep || !views[1].ReturnStep {
		t.Fatalf("ReturnStep 标记不符: %+v", views)
	}
}

// TestSimulateReturnDefaultIgnoresBack 未配置退回模式：即使存在退回位也只给常规下一步。
func TestSimulateReturnDefaultIgnoresBack(t *testing.T) {
	fx := newSimFixture(t)
	_, task := fx.seedReturnScenario("")
	views, err := fx.sim.SimulateByTask(context.Background(), task.ID, nil)
	if err != nil {
		t.Fatalf("推演失败: %v", err)
	}
	assertSameStrings(t, viewIDs(views), []string{SimEndActivityID})
}

// TestSimulateReturnMode5HistoryChain 模式 5：沿 preTaskId 链上溯历史步骤（排除当前步骤）。
func TestSimulateReturnMode5HistoryChain(t *testing.T) {
	fx := newSimFixture(t)
	// 步骤规则：handle 配置模式 5，approve 为发送步骤（排序 3 > 2 触发退回位）
	fx.steps.Put(&StepRule{DeploymentID: fx.deploymentID, StepCode: "apply", StepName: "申请", Sort: 1})
	fx.steps.Put(&StepRule{DeploymentID: fx.deploymentID, StepCode: "handle", StepName: "办理", ReturnApprovalStep: "5", Sort: 2})
	fx.steps.Put(&StepRule{DeploymentID: fx.deploymentID, StepCode: "approve", StepName: "批准", Sort: 3})
	instance, applyTask := fx.start("simLinear", "alice", nil)
	fx.complete(applyTask.ID, "alice", nil)
	handleTask := fx.firstTask(instance.ID)
	fx.seedTaskMessage(instance.ID, handleTask, "alice", "approve", "批准")
	// preTaskId 链：handle 任务记录指回 apply 任务
	fx.seedNext(instance.ID, "handle", "alice", handleTask.ID, applyTask.ID)
	views, err := fx.sim.SimulateByTask(context.Background(), handleTask.ID, nil)
	if err != nil {
		t.Fatalf("推演失败: %v", err)
	}
	// 链上溯只到 apply（流转过程中出现过），人员取其历史办理人
	assertSameStrings(t, viewIDs(views), []string{"apply"})
	if !views[0].ReturnStep {
		t.Fatalf("历史步骤应带 ReturnStep 标记: %+v", views[0])
	}
	assertSameStrings(t, views[0].ExecutorIDs, []string{"alice"})
}

// TestSimulateWithoutStepsSkipsReturnBranch 未注入步骤规则：跳过退回分支，返回常规下一步。
func TestSimulateWithoutStepsSkipsReturnBranch(t *testing.T) {
	fx := newSimFixture(t)
	_, task := fx.seedReturnScenario("1")
	views, err := fx.simNoSteps.SimulateByTask(context.Background(), task.ID, nil)
	if err != nil {
		t.Fatalf("推演失败: %v", err)
	}
	assertSameStrings(t, viewIDs(views), []string{SimEndActivityID})
}

// TestSimulateUnknownTaskReturnsEmpty 不存在任务按 Java 语义返回空列表而非错误。
func TestSimulateUnknownTaskReturnsEmpty(t *testing.T) {
	fx := newSimFixture(t)
	views, err := fx.sim.SimulateByTask(context.Background(), "no-such-task", nil)
	if err != nil {
		t.Fatalf("未知任务不应报错: %v", err)
	}
	if len(views) != 0 {
		t.Fatalf("未知任务应返回空列表: %+v", views)
	}
}
