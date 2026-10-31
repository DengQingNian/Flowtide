package workflow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/bpmn/model"
	"github.com/DengQingNian/Flowtide/internal/clock"
	"github.com/DengQingNian/Flowtide/internal/engine"
	"github.com/DengQingNian/Flowtide/internal/idgen"
)

// ─── 流程图高亮测试语料（全部为本文件专用，前缀 hl 避免与他人语料冲突）───

// hlLinearProcess 是线性语料：start → apply → handle → end。
const hlLinearProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="hlLinear" name="线性高亮流程" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="apply"/>
    <userTask id="apply" name="申请" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f1" sourceRef="apply" targetRef="handle"/>
    <userTask id="handle" name="办理" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f2" sourceRef="handle" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// hlGatewayProcess 是排他网关语料：gate 命中 ${pass == true} 走 approve，否则默认流走 review。
const hlGatewayProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="hlGateway" name="网关高亮流程" isExecutable="true">
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

// hlParallelProcess 是并行网关语料：fork 分出 ta/tb 两分支，join 汇聚后办结。
const hlParallelProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="hlParallel" name="并行高亮流程" isExecutable="true">
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

// hlSubProcessProcess 是子流程语料：apply → 子流程(sStart → inner → sEnd) → after → end。
const hlSubProcessProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="hlSub" name="子流程高亮流程" isExecutable="true">
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

// hlFixture 是流程图高亮测试环境（内存引擎 + 高亮服务）。
type hlFixture struct {
	t      *testing.T        // 测试上下文
	engine api.Engine        // 内存引擎
	svc    *HighlightService // 高亮服务
	fake   *clock.Fake       // 假时钟
}

// newHLFixture 构造高亮测试环境（部署全部语料、装配服务）。
// @param t 测试上下文
// @return 测试环境
func newHLFixture(t *testing.T) *hlFixture {
	t.Helper()
	fake := clock.NewFake(time.Date(2026, 10, 31, 11, 0, 0, 0, time.UTC))
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
		Name:     "highlight-test",
		TenantID: "dev",
		Resources: map[string][]byte{
			"linear.bpmn20.xml":   []byte(hlLinearProcess),
			"gateway.bpmn20.xml":  []byte(hlGatewayProcess),
			"parallel.bpmn20.xml": []byte(hlParallelProcess),
			"sub.bpmn20.xml":      []byte(hlSubProcessProcess),
		},
	}); err != nil {
		t.Fatalf("部署失败: %v", err)
	}
	return &hlFixture{t: t, engine: eng, svc: NewHighlightService(eng), fake: fake}
}

// hlStart 发起流程实例并返回实例与首个任务。
// @param definitionKey 流程定义 Key
// @param vars 启动变量
// @return 流程实例
// @return 首个任务
func (f *hlFixture) hlStart(definitionKey string, vars map[string]any) (api.ProcessInstance, api.Task) {
	f.t.Helper()
	variables := []api.Variable{{Name: VarInitiator, Value: "alice"}}
	for name, value := range vars {
		variables = append(variables, api.Variable{Name: name, Value: value})
	}
	instance, err := f.engine.RuntimeService().StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: definitionKey,
		BusinessKey:          "BIZ-hl",
		Name:                 "高亮-" + definitionKey,
		TenantID:             "dev",
		UserID:               "alice",
		Variables:            variables,
	})
	if err != nil {
		f.t.Fatalf("发起失败: %v", err)
	}
	return instance, f.hlTask(instance.ID)
}

// hlTask 返回实例当前首个任务。
// @param instanceID 实例 Id
// @return 首个任务
func (f *hlFixture) hlTask(instanceID string) api.Task {
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

// hlComplete 以指定变量完成任务。
// @param taskID 任务 Id
// @param operator 操作人
// @param vars 流程变量
func (f *hlFixture) hlComplete(taskID, operator string, vars map[string]any) {
	f.t.Helper()
	req := api.CompleteTaskRequest{UserID: operator}
	for name, value := range vars {
		req.Variables = append(req.Variables, api.Variable{Name: name, Value: value})
	}
	if err := f.engine.TaskService().Complete(context.Background(), taskID, req); err != nil {
		f.t.Fatalf("办结任务失败: %v", err)
	}
}

// hlAssertSet 断言高亮集合与期望逐项相等（已排序输出，保序比较）。
// @param t 测试上下文
// @param name 集合名（断言消息定位）
// @param got 实际值
// @param want 期望值
func hlAssertSet(t *testing.T, name string, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s 不符: got=%v want=%v", name, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s 不符: got=%v want=%v", name, got, want)
		}
	}
}

// ─── 线性语料：运行态三阶段 + 已办结回退全历史 ───

// TestHighlightLinearPhases 线性语料三阶段：发起、走两步、办结（回退全历史）。
func TestHighlightLinearPhases(t *testing.T) {
	fx := newHLFixture(t)
	ctx := context.Background()
	instance, task := fx.hlStart("hlLinear", nil)
	// 阶段一：apply 待办
	data, err := fx.svc.FlowHighlight(ctx, instance.ID)
	if err != nil {
		t.Fatalf("高亮查询失败: %v", err)
	}
	hlAssertSet(t, "Completed", data.CompletedActivityIDs, "start")
	hlAssertSet(t, "Active", data.ActiveActivityIDs, "apply")
	hlAssertSet(t, "Todo", data.TodoActivityIDs, "apply")
	hlAssertSet(t, "Taken", data.TakenFlowIDs, "f0")
	// 阶段二：apply 办结 → handle 待办
	fx.hlComplete(task.ID, "alice", nil)
	data, err = fx.svc.FlowHighlight(ctx, instance.ID)
	if err != nil {
		t.Fatalf("高亮查询失败: %v", err)
	}
	hlAssertSet(t, "Completed", data.CompletedActivityIDs, "apply", "start")
	hlAssertSet(t, "Active", data.ActiveActivityIDs, "handle")
	hlAssertSet(t, "Todo", data.TodoActivityIDs, "handle")
	// f0（start→apply）与 f1（apply→handle）两端均已出现于历史
	hlAssertSet(t, "Taken", data.TakenFlowIDs, "f0", "f1")
	// 按任务查询与按实例查询同源
	byTask, err := fx.svc.FlowHighlightByTask(ctx, fx.hlTask(instance.ID).ID)
	if err != nil {
		t.Fatalf("按任务高亮查询失败: %v", err)
	}
	hlAssertSet(t, "ByTask.Completed", byTask.CompletedActivityIDs, data.CompletedActivityIDs...)
	hlAssertSet(t, "ByTask.Taken", byTask.TakenFlowIDs, data.TakenFlowIDs...)
	// 阶段三：实例办结 → 回退全历史
	handleTask := fx.hlTask(instance.ID)
	fx.hlComplete(handleTask.ID, "alice", nil)
	data, err = fx.svc.FlowHighlight(ctx, instance.ID)
	if err != nil {
		t.Fatalf("结束实例高亮查询失败: %v", err)
	}
	// 回退语义：Active/Todo 为空，Completed 取全部历史节点（含 end 事件）
	if len(data.ActiveActivityIDs) != 0 || len(data.TodoActivityIDs) != 0 {
		t.Fatalf("已结束实例 Active/Todo 应为空: %+v", data)
	}
	hlAssertSet(t, "Completed", data.CompletedActivityIDs, "apply", "end", "handle", "start")
	hlAssertSet(t, "Taken", data.TakenFlowIDs, "f0", "f1", "f2")
}

// ─── 网关 / 并行 / 子流程：与实际流转一致 ───

// TestHighlightGatewayRoute 排他网关：只高亮实际命中的分支连线。
func TestHighlightGatewayRoute(t *testing.T) {
	fx := newHLFixture(t)
	instance, task := fx.hlStart("hlGateway", nil)
	fx.hlComplete(task.ID, "alice", map[string]any{"pass": true})
	data, err := fx.svc.FlowHighlight(context.Background(), instance.ID)
	if err != nil {
		t.Fatalf("高亮查询失败: %v", err)
	}
	// 网关节点已完成且入历史
	hlAssertSet(t, "Completed", data.CompletedActivityIDs, "apply", "gate", "start")
	hlAssertSet(t, "Active", data.ActiveActivityIDs, "approve")
	hlAssertSet(t, "Todo", data.TodoActivityIDs, "approve")
	// 命中分支 f2（gate→approve）已走；默认分支 f3（gate→review）未走
	hlAssertSet(t, "Taken", data.TakenFlowIDs, "f0", "f1", "f2")
}

// TestHighlightParallelBranches 并行网关：双分支任务同时活跃，未走分支连线不高亮。
func TestHighlightParallelBranches(t *testing.T) {
	fx := newHLFixture(t)
	instance, task := fx.hlStart("hlParallel", nil)
	fx.hlComplete(task.ID, "alice", nil)
	data, err := fx.svc.FlowHighlight(context.Background(), instance.ID)
	if err != nil {
		t.Fatalf("高亮查询失败: %v", err)
	}
	hlAssertSet(t, "Completed", data.CompletedActivityIDs, "apply", "fork", "start")
	// 双分支任务 + 分叉网关执行均在运行态
	hlAssertSet(t, "Active", data.ActiveActivityIDs, "fork", "ta", "tb")
	hlAssertSet(t, "Todo", data.TodoActivityIDs, "ta", "tb")
	// 分叉出线 f2/f3 已走；汇聚方向 f4/f5/f6 未走
	hlAssertSet(t, "Taken", data.TakenFlowIDs, "f0", "f1", "f2", "f3")
}

// TestHighlightSubProcessInner 子流程：内部节点与内部连线纳入高亮（拓扑递归）。
func TestHighlightSubProcessInner(t *testing.T) {
	fx := newHLFixture(t)
	instance, task := fx.hlStart("hlSub", nil)
	fx.hlComplete(task.ID, "alice", nil)
	data, err := fx.svc.FlowHighlight(context.Background(), instance.ID)
	if err != nil {
		t.Fatalf("高亮查询失败: %v", err)
	}
	// 子流程容器 sub 进行中（未完成），子流程开始事件 sStart 已完成，内部任务 inner 待办
	hlAssertSet(t, "Completed", data.CompletedActivityIDs, "apply", "sStart", "start")
	hlAssertSet(t, "Active", data.ActiveActivityIDs, "inner", "sub")
	hlAssertSet(t, "Todo", data.TodoActivityIDs, "inner")
	// f1（apply→sub）已走；子流程内部 sf0（sStart→inner）经递归纳入
	hlAssertSet(t, "Taken", data.TakenFlowIDs, "f0", "f1", "sf0")
}

// ─── 参数与边界 ───

// TestHighlightErrors 参数缺失、未知实例/任务、未注入引擎的报错路径。
func TestHighlightErrors(t *testing.T) {
	fx := newHLFixture(t)
	ctx := context.Background()
	if _, err := fx.svc.FlowHighlight(ctx, ""); err == nil {
		t.Fatal("空实例 Id 应报错")
	}
	if _, err := fx.svc.FlowHighlight(ctx, "no-such-instance"); err == nil {
		t.Fatal("未知实例应报错")
	}
	if _, err := fx.svc.FlowHighlightByTask(ctx, ""); err == nil {
		t.Fatal("空任务 Id 应报错")
	}
	if _, err := fx.svc.FlowHighlightByTask(ctx, "no-such-task"); err == nil {
		t.Fatal("未知任务应报错")
	}
	bare := NewHighlightService(nil)
	if _, err := bare.FlowHighlight(ctx, "pi-1"); err == nil {
		t.Fatal("未注入引擎应报错")
	}
	if _, err := bare.FlowHighlightByTask(ctx, "task-1"); err == nil {
		t.Fatal("未注入引擎按任务查询应报错")
	}
}

// TestHighlightEndedInstanceFallback 已结束实例即使运行态仍可查（内存引擎 Ended 标记），
// 也按回退语义输出：Active/Todo 为空。
func TestHighlightEndedInstanceFallback(t *testing.T) {
	fx := newHLFixture(t)
	instance, task := fx.hlStart("hlLinear", nil)
	fx.hlComplete(task.ID, "alice", nil)
	fx.hlComplete(fx.hlTask(instance.ID).ID, "alice", nil)
	data, err := fx.svc.FlowHighlight(context.Background(), instance.ID)
	if err != nil {
		t.Fatalf("结束实例高亮查询失败: %v", err)
	}
	if len(data.ActiveActivityIDs) != 0 || len(data.TodoActivityIDs) != 0 {
		t.Fatalf("已结束实例 Active/Todo 应为空: %+v", data)
	}
	// 已办结实例的全部节点与连线都进入高亮
	hlAssertSet(t, "Completed", data.CompletedActivityIDs, "apply", "end", "handle", "start")
	hlAssertSet(t, "Taken", data.TakenFlowIDs, "f0", "f1", "f2")
}

// TestHighlightStableOrdering 同一实例多次查询输出顺序稳定（排序幂等）。
func TestHighlightStableOrdering(t *testing.T) {
	fx := newHLFixture(t)
	instance, task := fx.hlStart("hlParallel", nil)
	fx.hlComplete(task.ID, "alice", nil)
	first, err := fx.svc.FlowHighlight(context.Background(), instance.ID)
	if err != nil {
		t.Fatalf("高亮查询失败: %v", err)
	}
	second, err := fx.svc.FlowHighlight(context.Background(), instance.ID)
	if err != nil {
		t.Fatalf("高亮查询失败: %v", err)
	}
	hlAssertSet(t, "Active", second.ActiveActivityIDs, first.ActiveActivityIDs...)
	hlAssertSet(t, "Taken", second.TakenFlowIDs, first.TakenFlowIDs...)
}

// ─── 引擎错误注入桩（覆盖高亮服务的错误传播与历史回退分支）───

// hlFailHistory 是历史服务错误注入桩（仅高亮相关方法失效，其余透传）。
type hlFailHistory struct {
	api.HistoryService      // 内嵌历史服务
	failActs           bool // ListHistoricActivityInstances 注入错误
	failInstance       bool // GetHistoricProcessInstance 注入错误
}

// ListHistoricActivityInstances 按开关返回错误。
// @param ctx 上下文
// @param processInstanceID 实例 ID
// @return 历史活动列表
// @return 错误
func (h hlFailHistory) ListHistoricActivityInstances(ctx context.Context, processInstanceID string) ([]api.HistoricActivityInstance, error) {
	// 错误注入点
	if h.failActs {
		return nil, errors.New("注入错误")
	}
	return h.HistoryService.ListHistoricActivityInstances(ctx, processInstanceID)
}

// GetHistoricProcessInstance 按开关返回错误。
// @param ctx 上下文
// @param processInstanceID 实例 ID
// @return 历史实例
// @return 错误
func (h hlFailHistory) GetHistoricProcessInstance(ctx context.Context, processInstanceID string) (api.HistoricProcessInstance, error) {
	// 错误注入点
	if h.failInstance {
		return api.HistoricProcessInstance{}, errors.New("注入错误")
	}
	return h.HistoryService.GetHistoricProcessInstance(ctx, processInstanceID)
}

// hlFailRepo 是仓库服务错误注入桩（仅 GetBpmnModel 失效或替换返回值）。
type hlFailRepo struct {
	api.RepositoryService      // 内嵌仓库服务
	failModel             bool // GetBpmnModel 注入错误
	emptyModel            bool // GetBpmnModel 返回空模型
}

// GetBpmnModel 按开关返回错误或空模型。
// @param ctx 上下文
// @param definitionID 流程定义 ID
// @return BPMN 模型
// @return 错误
func (h hlFailRepo) GetBpmnModel(ctx context.Context, definitionID string) (*model.BpmnModel, error) {
	// 注入错误
	if h.failModel {
		return nil, errors.New("注入错误")
	}
	// 注入空模型（无流程定义的异常形态）
	if h.emptyModel {
		return &model.BpmnModel{}, nil
	}
	return h.RepositoryService.GetBpmnModel(ctx, definitionID)
}

// hlFailRuntime 是运行态服务错误注入桩。
type hlFailRuntime struct {
	api.RuntimeService      // 内嵌运行态服务
	failInstance       bool // GetProcessInstance 注入错误（模拟已结束实例运行态 404）
	failActs           bool // ListActivityInstances 注入错误
}

// GetProcessInstance 按开关返回错误。
// @param ctx 上下文
// @param id 实例 ID
// @return 实例
// @return 错误
func (h hlFailRuntime) GetProcessInstance(ctx context.Context, id string) (api.ProcessInstance, error) {
	// 错误注入点
	if h.failInstance {
		return api.ProcessInstance{}, errors.New("注入错误")
	}
	return h.RuntimeService.GetProcessInstance(ctx, id)
}

// ListActivityInstances 按开关返回错误。
// @param ctx 上下文
// @param processInstanceID 实例 ID
// @return 活动实例列表
// @return 错误
func (h hlFailRuntime) ListActivityInstances(ctx context.Context, processInstanceID string) ([]api.ActivityInstance, error) {
	// 错误注入点
	if h.failActs {
		return nil, errors.New("注入错误")
	}
	return h.RuntimeService.ListActivityInstances(ctx, processInstanceID)
}

// hlFailTasks 是任务服务错误注入桩。
type hlFailTasks struct {
	api.TaskService // 内嵌任务服务
}

// ListTasks 恒返回错误。
// @param ctx 上下文
// @param filter 查询条件
// @return 分页结果
// @return 错误
func (hlFailTasks) ListTasks(ctx context.Context, filter api.TaskFilter) (api.Page[api.Task], error) {
	return api.Page[api.Task]{}, errors.New("注入错误")
}

// hlFailEngine 是高亮服务的错误注入引擎包装，按开关失效对应服务。
type hlFailEngine struct {
	api.Engine           // 内嵌引擎门面
	failHistActs    bool // ListHistoricActivityInstances 注入错误
	failHisInstance bool // GetHistoricProcessInstance 注入错误
	failModel       bool // GetBpmnModel 注入错误
	emptyModel      bool // GetBpmnModel 返回空模型
	failInstance    bool // GetProcessInstance 注入错误
	failRunActs     bool // ListActivityInstances 注入错误
	failTasks       bool // ListTasks 注入错误
}

// HistoryService 按开关返回错误注入的历史服务。
// @return 历史服务
func (e hlFailEngine) HistoryService() api.HistoryService {
	return hlFailHistory{HistoryService: e.Engine.HistoryService(), failActs: e.failHistActs, failInstance: e.failHisInstance}
}

// RepositoryService 按开关返回错误注入的仓库服务。
// @return 仓库服务
func (e hlFailEngine) RepositoryService() api.RepositoryService {
	return hlFailRepo{RepositoryService: e.Engine.RepositoryService(), failModel: e.failModel, emptyModel: e.emptyModel}
}

// RuntimeService 按开关返回错误注入的运行态服务。
// @return 运行态服务
func (e hlFailEngine) RuntimeService() api.RuntimeService {
	return hlFailRuntime{RuntimeService: e.Engine.RuntimeService(), failInstance: e.failInstance, failActs: e.failRunActs}
}

// TaskService 按开关返回错误注入的任务服务。
// @return 任务服务
func (e hlFailEngine) TaskService() api.TaskService {
	// 任务服务注入固定错误
	if e.failTasks {
		return hlFailTasks{TaskService: e.Engine.TaskService()}
	}
	return e.Engine.TaskService()
}

// TestHighlightErrorBranches 高亮服务各引擎错误分支的传播路径。
func TestHighlightErrorBranches(t *testing.T) {
	fx := newHLFixture(t)
	instance, task := fx.hlStart("hlLinear", nil)
	ctx := context.Background()
	// 历史活动查询错误
	svc := NewHighlightService(hlFailEngine{Engine: fx.engine, failHistActs: true})
	if _, err := svc.FlowHighlight(ctx, instance.ID); err == nil {
		t.Fatal("历史活动查询错误应传播")
	}
	// BPMN 模型读取错误
	svc = NewHighlightService(hlFailEngine{Engine: fx.engine, failModel: true})
	if _, err := svc.FlowHighlight(ctx, instance.ID); err == nil {
		t.Fatal("模型读取错误应传播")
	}
	// 空模型（无流程）：已走连线为空但不报错
	svc = NewHighlightService(hlFailEngine{Engine: fx.engine, emptyModel: true})
	data, err := svc.FlowHighlight(ctx, instance.ID)
	if err != nil {
		t.Fatalf("空模型不应报错: %v", err)
	}
	if len(data.TakenFlowIDs) != 0 {
		t.Fatalf("空模型已走连线应为空: %v", data.TakenFlowIDs)
	}
	// 运行态活动查询错误
	svc = NewHighlightService(hlFailEngine{Engine: fx.engine, failRunActs: true})
	if _, err := svc.FlowHighlight(ctx, instance.ID); err == nil {
		t.Fatal("运行态活动查询错误应传播")
	}
	// 待办任务查询错误
	svc = NewHighlightService(hlFailEngine{Engine: fx.engine, failTasks: true})
	if _, err := svc.FlowHighlight(ctx, instance.ID); err == nil {
		t.Fatal("待办任务查询错误应传播")
	}
	// 运行态与历史实例均查不到
	svc = NewHighlightService(hlFailEngine{Engine: fx.engine, failInstance: true, failHisInstance: true})
	if _, err := svc.FlowHighlight(ctx, instance.ID); err == nil {
		t.Fatal("实例双查失败应报错")
	}
	_ = task
}

// TestHighlightHistoricFallback 运行态查不到实例（SQL 引擎已结束形态）时回退历史实例。
func TestHighlightHistoricFallback(t *testing.T) {
	fx := newHLFixture(t)
	instance, task := fx.hlStart("hlLinear", nil)
	fx.hlComplete(task.ID, "alice", nil)
	fx.hlComplete(fx.hlTask(instance.ID).ID, "alice", nil)
	// 运行态实例查询恒 404（模拟 SQL 引擎已结束实例）
	svc := NewHighlightService(hlFailEngine{Engine: fx.engine, failInstance: true})
	data, err := svc.FlowHighlight(context.Background(), instance.ID)
	if err != nil {
		t.Fatalf("历史回退高亮查询失败: %v", err)
	}
	// 回退语义：Active/Todo 为空、Completed 全历史、连线全走
	if len(data.ActiveActivityIDs) != 0 || len(data.TodoActivityIDs) != 0 {
		t.Fatalf("历史回退 Active/Todo 应为空: %+v", data)
	}
	hlAssertSet(t, "Completed", data.CompletedActivityIDs, "apply", "end", "handle", "start")
	hlAssertSet(t, "Taken", data.TakenFlowIDs, "f0", "f1", "f2")
}

// TestHlCollectTakenFlows 拓扑收集的防御分支：顺序与索引不一致时跳过。
func TestHlCollectTakenFlows(t *testing.T) {
	appeared := map[string]bool{"a": true, "b": true}
	out := make([]string, 0, 2)
	// order 携带不存在的元素 Id（防御分支）+ 两端齐备的正常连线
	hlCollectTakenFlows(
		map[string]model.FlowElement{"f1": &model.SequenceFlow{BaseElement: model.BaseElement{ID: "f1"}, SourceRef: "a", TargetRef: "b"}},
		[]string{"ghost", "f1"},
		appeared, &out,
	)
	hlAssertSet(t, "Taken", out, "f1")
}
