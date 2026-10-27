package engine

import (
	"context"
	"testing"

	"github.com/flowable-go/flowable/api"
)

const callActivityXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn">
  <process id="child" name="Child" isExecutable="true">
    <startEvent id="cs"/>
    <sequenceFlow id="cf1" sourceRef="cs" targetRef="childTask"/>
    <userTask id="childTask" name="Child Task"/>
    <sequenceFlow id="cf2" sourceRef="childTask" targetRef="ce"/>
    <endEvent id="ce"/>
  </process>
  <process id="parent" name="Parent" isExecutable="true">
    <startEvent id="ps"/>
    <sequenceFlow id="pf1" sourceRef="ps" targetRef="call"/>
    <callActivity id="call" calledElement="child"/>
    <sequenceFlow id="pf2" sourceRef="call" targetRef="pe"/>
    <endEvent id="pe"/>
  </process>
</definitions>`

// TestCallActivity_StartsChild 验证调用子流程并恢复父流程。
// @param t 测试上下文
func TestCallActivity_StartsChild(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, callActivityXML)
	parent, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "parent"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 1 || tasks[0].TaskDefinitionKey != "childTask" {
		t.Fatalf("期望子流程任务，实际 %+v", tasks)
	}
	if err := engine.Complete(context.Background(), tasks[0].ID, api.CompleteTaskRequest{}); err != nil {
		t.Fatalf("完成子流程任务失败: %v", err)
	}
	got, _ := engine.GetProcessInstance(context.Background(), parent.ID)
	if !got.Ended {
		t.Fatalf("子流程结束后父流程应结束")
	}
	instances, _ := engine.ListProcessInstances(context.Background())
	if len(instances) != 2 {
		t.Fatalf("期望父子 2 个实例，实际 %d", len(instances))
	}
}

const callActivityIOModeXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn">
  <process id="childIO" isExecutable="true">
    <startEvent id="cs"/>
    <sequenceFlow id="cf1" sourceRef="cs" targetRef="svc"/>
    <serviceTask id="svc" flowable:delegateExpression="${calc}"/>
    <sequenceFlow id="cf2" sourceRef="svc" targetRef="ce"/>
    <endEvent id="ce"/>
  </process>
  <process id="parentIO" isExecutable="true">
    <startEvent id="ps"/>
    <sequenceFlow id="pf1" sourceRef="ps" targetRef="call"/>
    <callActivity id="call" calledElement="childIO">
      <extensionElements>
        <flowable:in source="x" target="y"/>
        <flowable:out source="z" target="result"/>
      </extensionElements>
    </callActivity>
    <sequenceFlow id="pf2" sourceRef="call" targetRef="pe"/>
    <endEvent id="pe"/>
  </process>
</definitions>`

// TestCallActivity_InOutMapping 验证入参和出参映射。
// @param t 测试上下文
func TestCallActivity_InOutMapping(t *testing.T) {
	engine := newTestEngine(t)
	engine.RegisterDelegate("calc", func(ctx context.Context, execution *Execution) error {
		value, _ := execution.GetVariable("y")
		number, _ := value.(int)
		execution.SetVariable("z", number*2)
		return nil
	})
	deployXML(t, engine, callActivityIOModeXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "parentIO",
		Variables:            []api.Variable{{Name: "x", Value: 5}},
	})
	if err != nil || !instance.Ended {
		t.Fatalf("调用失败: %v %+v", err, instance)
	}
	variables, _ := engine.GetVariables(context.Background(), instance.ID)
	if variables["result"] != 10 {
		t.Fatalf("出参映射错误: %+v", variables)
	}
}

const callActivityInheritXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn">
  <process id="childInherit" isExecutable="true">
    <startEvent id="cs"/>
    <sequenceFlow id="cf1" sourceRef="cs" targetRef="ct"/>
    <userTask id="ct" name="Child" flowable:assignee="${initiator}"/>
    <sequenceFlow id="cf2" sourceRef="ct" targetRef="ce"/>
    <endEvent id="ce"/>
  </process>
  <process id="parentInherit" isExecutable="true">
    <startEvent id="ps"/>
    <sequenceFlow id="pf1" sourceRef="ps" targetRef="call"/>
    <callActivity id="call" calledElement="childInherit" flowable:inheritVariables="true"/>
    <sequenceFlow id="pf2" sourceRef="call" targetRef="pe"/>
    <endEvent id="pe"/>
  </process>
</definitions>`

// TestCallActivity_InheritVariables 验证变量继承。
// @param t 测试上下文
func TestCallActivity_InheritVariables(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, callActivityInheritXML)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "parentInherit",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 1 || tasks[0].Assignee != "alice" {
		t.Fatalf("子流程未继承变量: %+v", tasks)
	}
}

// TestCallActivity_MissingChild 验证被调用流程不存在。
// @param t 测试上下文
func TestCallActivity_MissingChild(t *testing.T) {
	engine := newTestEngine(t)
	xml := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="parentMissing" isExecutable="true"><startEvent id="s"/><sequenceFlow id="f1" sourceRef="s" targetRef="call"/><callActivity id="call" calledElement="missing"/><sequenceFlow id="f2" sourceRef="call" targetRef="e"/><endEvent id="e"/></process></definitions>`
	deployXML(t, engine, xml)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "parentMissing"}); err == nil {
		t.Fatalf("缺失子流程应报错")
	}
	instances, _ := engine.ListProcessInstances(context.Background())
	if len(instances) != 0 {
		t.Fatalf("失败后应回滚，实际实例 %d", len(instances))
	}
}
