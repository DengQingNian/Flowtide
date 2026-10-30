package engine

import (
	"context"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
)

const inclusiveXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="inclusive" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="gw"/>
    <inclusiveGateway id="gw" default="fc"/>
    <sequenceFlow id="fa" sourceRef="gw" targetRef="a">
      <conditionExpression>${x &gt; 0}</conditionExpression>
    </sequenceFlow>
    <sequenceFlow id="fb" sourceRef="gw" targetRef="b">
      <conditionExpression>${y &gt; 0}</conditionExpression>
    </sequenceFlow>
    <sequenceFlow id="fc" sourceRef="gw" targetRef="c"/>
    <userTask id="a" name="A"/>
    <userTask id="b" name="B"/>
    <userTask id="c" name="C"/>
    <sequenceFlow id="fja" sourceRef="a" targetRef="join"/>
    <sequenceFlow id="fjb" sourceRef="b" targetRef="join"/>
    <sequenceFlow id="fjc" sourceRef="c" targetRef="join"/>
    <inclusiveGateway id="join"/>
    <sequenceFlow id="f2" sourceRef="join" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestInclusiveGateway_MultipleBranches 验证包容网关多分支与动态汇聚。
// @param t 测试上下文
func TestInclusiveGateway_MultipleBranches(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, inclusiveXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "inclusive",
		Variables: []api.Variable{
			{Name: "x", Value: 1},
			{Name: "y", Value: 1},
		},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 2 {
		t.Fatalf("期望 2 个任务，实际 %d", len(tasks))
	}
	for _, task := range tasks {
		if err := engine.Complete(context.Background(), task.ID, api.CompleteTaskRequest{}); err != nil {
			t.Fatalf("完成任务失败: %v", err)
		}
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("包容网关汇聚后应结束")
	}
}

// TestInclusiveGateway_SingleBranch 验证单分支。
// @param t 测试上下文
func TestInclusiveGateway_SingleBranch(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, inclusiveXML)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "inclusive",
		Variables:            []api.Variable{{Name: "x", Value: 1}, {Name: "y", Value: 0}},
	}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 1 || tasks[0].TaskDefinitionKey != "a" {
		t.Fatalf("期望任务 a，实际 %+v", tasks)
	}
}

// TestInclusiveGateway_DefaultFlow 验证默认流。
// @param t 测试上下文
func TestInclusiveGateway_DefaultFlow(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, inclusiveXML)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "inclusive",
		Variables:            []api.Variable{{Name: "x", Value: 0}, {Name: "y", Value: 0}},
	}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 1 || tasks[0].TaskDefinitionKey != "c" {
		t.Fatalf("期望默认任务 c，实际 %+v", tasks)
	}
}
