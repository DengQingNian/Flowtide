package engine

import (
	"context"
	"testing"
	"time"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/clock"
	"github.com/flowable-go/flowable/internal/idgen"
)

// newTestEngine 创建使用假时钟的测试引擎。
// @param t 测试上下文
// @return 引擎
func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	engine, err := New(Config{
		HistoryLevel: HistoryAudit,
		Clock:        clock.NewFake(time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)),
		IDs:          idgen.NewUUIDv7(),
	})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	return engine
}

// deployXML 部署 BPMN XML。
// @param t 测试上下文
// @param engine 引擎
// @param xml BPMN 内容
func deployXML(t *testing.T, engine *Engine, xml string) {
	t.Helper()
	_, err := engine.Deploy(context.Background(), api.DeploymentRequest{
		Name:      "test",
		Resources: map[string][]byte{"test.bpmn20.xml": []byte(xml)},
	})
	if err != nil {
		t.Fatalf("部署失败: %v", err)
	}
}

// listTasks 查询全部任务。
// @param t 测试上下文
// @param engine 引擎
// @return 任务列表
func listTasks(t *testing.T, engine *Engine) []api.Task {
	t.Helper()
	page, err := engine.ListTasks(context.Background(), api.TaskFilter{Size: 100})
	if err != nil {
		t.Fatalf("查询任务失败: %v", err)
	}
	return page.Data
}

const simpleProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="simple" name="Simple" isExecutable="true">
    <startEvent id="start" name="Start"/>
    <sequenceFlow id="flow1" sourceRef="start" targetRef="approve"/>
    <userTask id="approve" name="Approve" flowable:assignee="${initiator}"/>
    <sequenceFlow id="flow2" sourceRef="approve" targetRef="end"/>
    <endEvent id="end" name="End"/>
  </process>
</definitions>`

// TestStartProcess_ReachesUserTask 验证启动后停在用户任务。
// @param t 测试上下文
func TestStartProcess_ReachesUserTask(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)

	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		BusinessKey:          "order-1",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if instance.Ended {
		t.Fatalf("期望实例等待任务，实际已结束")
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 1 {
		t.Fatalf("期望 1 个任务，实际 %d", len(tasks))
	}
	if tasks[0].Assignee != "alice" {
		t.Fatalf("期望处理人 alice，实际 %s", tasks[0].Assignee)
	}
	if tasks[0].TaskDefinitionKey != "approve" {
		t.Fatalf("期望任务定义 approve，实际 %s", tasks[0].TaskDefinitionKey)
	}
}

// TestCompleteTask_EndsProcess 验证完成任务后流程结束。
// @param t 测试上下文
func TestCompleteTask_EndsProcess(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 1 {
		t.Fatalf("期望 1 个任务，实际 %d", len(tasks))
	}
	if err := engine.Complete(context.Background(), tasks[0].ID, api.CompleteTaskRequest{UserID: "alice"}); err != nil {
		t.Fatalf("完成任务失败: %v", err)
	}
	got, err := engine.GetProcessInstance(context.Background(), instance.ID)
	if err != nil {
		t.Fatalf("查询实例失败: %v", err)
	}
	if !got.Ended {
		t.Fatalf("期望实例已结束")
	}
	if len(listTasks(t, engine)) != 0 {
		t.Fatalf("期望任务已删除")
	}
	historic, err := engine.ListHistoricActivityInstances(context.Background(), instance.ID)
	if err != nil {
		t.Fatalf("查询历史失败: %v", err)
	}
	if len(historic) != 3 {
		t.Fatalf("期望 3 条历史活动，实际 %d: %+v", len(historic), historic)
	}
}

// TestClaimTask_Conflict 验证重复签收冲突。
// @param t 测试上下文
func TestClaimTask_Conflict(t *testing.T) {
	engine := newTestEngine(t)
	xml := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="claim" isExecutable="true"><startEvent id="start"/><sequenceFlow id="f1" sourceRef="start" targetRef="task"/><userTask id="task" name="Task"/><sequenceFlow id="f2" sourceRef="task" targetRef="end"/><endEvent id="end"/></process></definitions>`
	deployXML(t, engine, xml)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "claim"}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	task := listTasks(t, engine)[0]
	if task.Assignee != "" {
		t.Fatalf("测试流程应未分配")
	}
	if err := engine.Claim(context.Background(), task.ID, "bob"); err != nil {
		t.Fatalf("签收失败: %v", err)
	}
	if err := engine.Claim(context.Background(), task.ID, "carol"); err == nil {
		t.Fatalf("期望重复签收失败")
	}
}

const exclusiveProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="exclusive" name="Exclusive" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="gateway"/>
    <exclusiveGateway id="gateway" default="lowFlow"/>
    <sequenceFlow id="highFlow" sourceRef="gateway" targetRef="high">
      <conditionExpression>amount &gt; 100</conditionExpression>
    </sequenceFlow>
    <sequenceFlow id="lowFlow" sourceRef="gateway" targetRef="low"/>
    <userTask id="high" name="High"/>
    <userTask id="low" name="Low"/>
    <sequenceFlow id="f2" sourceRef="high" targetRef="end1"/>
    <sequenceFlow id="f3" sourceRef="low" targetRef="end2"/>
    <endEvent id="end1"/>
    <endEvent id="end2"/>
  </process>
</definitions>`

// TestExclusiveGateway_SelectsCondition 验证排他网关条件选择。
// @param t 测试上下文
func TestExclusiveGateway_SelectsCondition(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, exclusiveProcess)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "exclusive",
		Variables:            []api.Variable{{Name: "amount", Value: 200}},
	}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 1 || tasks[0].TaskDefinitionKey != "high" {
		t.Fatalf("期望 high 任务，实际 %+v", tasks)
	}
}

// TestExclusiveGateway_UsesDefault 验证排他网关默认流。
// @param t 测试上下文
func TestExclusiveGateway_UsesDefault(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, exclusiveProcess)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "exclusive",
		Variables:            []api.Variable{{Name: "amount", Value: 10}},
	}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 1 || tasks[0].TaskDefinitionKey != "low" {
		t.Fatalf("期望 low 任务，实际 %+v", tasks)
	}
}

const parallelProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="parallel" name="Parallel" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="fork"/>
    <parallelGateway id="fork"/>
    <sequenceFlow id="fa" sourceRef="fork" targetRef="taskA"/>
    <sequenceFlow id="fb" sourceRef="fork" targetRef="taskB"/>
    <userTask id="taskA" name="A"/>
    <userTask id="taskB" name="B"/>
    <sequenceFlow id="fja" sourceRef="taskA" targetRef="join"/>
    <sequenceFlow id="fjb" sourceRef="taskB" targetRef="join"/>
    <parallelGateway id="join"/>
    <sequenceFlow id="f2" sourceRef="join" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// TestParallelGateway_JoinsBranches 验证并行分支汇聚。
// @param t 测试上下文
func TestParallelGateway_JoinsBranches(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, parallelProcess)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "parallel"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 2 {
		t.Fatalf("期望 2 个任务，实际 %d", len(tasks))
	}
	if err := engine.Complete(context.Background(), tasks[0].ID, api.CompleteTaskRequest{}); err != nil {
		t.Fatalf("完成 A 失败: %v", err)
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if got.Ended {
		t.Fatalf("完成一个分支后不应结束")
	}
	remaining := listTasks(t, engine)
	if len(remaining) != 1 {
		t.Fatalf("期望剩余 1 个任务，实际 %d", len(remaining))
	}
	if err := engine.Complete(context.Background(), remaining[0].ID, api.CompleteTaskRequest{}); err != nil {
		t.Fatalf("完成 B 失败: %v", err)
	}
	got, _ = engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("两个分支完成后应结束")
	}
}

const serviceProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="service" name="Service" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="svc"/>
    <serviceTask id="svc" name="Call" flowable:delegateExpression="${noop}"/>
    <sequenceFlow id="f2" sourceRef="svc" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// TestServiceTask_Delegate 验证服务任务委托。
// @param t 测试上下文
func TestServiceTask_Delegate(t *testing.T) {
	engine := newTestEngine(t)
	engine.RegisterDelegate("noop", func(ctx context.Context, execution *Execution) error {
		execution.SetVariable("result", "done")
		return nil
	})
	deployXML(t, engine, serviceProcess)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "service"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if !instance.Ended {
		t.Fatalf("服务任务后应结束")
	}
	variables, _ := engine.GetVariables(context.Background(), instance.ID)
	if variables["result"] != "done" {
		t.Fatalf("期望 result=done，实际 %+v", variables)
	}
}

// TestServiceTask_MissingDelegateRollsBack 验证缺失委托时回滚。
// @param t 测试上下文
func TestServiceTask_MissingDelegateRollsBack(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, serviceProcess)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "service"}); err == nil {
		t.Fatalf("期望启动失败")
	}
	instances, err := engine.ListProcessInstances(context.Background())
	if err != nil {
		t.Fatalf("查询实例失败: %v", err)
	}
	if len(instances) != 0 {
		t.Fatalf("期望回滚后无实例，实际 %d", len(instances))
	}
}

// TestVariables_SetAndGet 验证变量读写。
// @param t 测试上下文
func TestVariables_SetAndGet(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if err := engine.SetVariable(context.Background(), instance.ID, "amount", 42); err != nil {
		t.Fatalf("设置变量失败: %v", err)
	}
	variables, _ := engine.GetVariables(context.Background(), instance.ID)
	if variables["amount"] != 42 {
		t.Fatalf("期望 amount=42，实际 %+v", variables)
	}
}
