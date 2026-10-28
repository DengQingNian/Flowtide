package engine

import (
	"context"
	"testing"

	"github.com/flowable-go/flowable/api"
)

const linkEventXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="linkProcess" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="throw"/>
    <intermediateThrowEvent id="throw"><linkEventDefinition name="toTask"/></intermediateThrowEvent>
    <intermediateCatchEvent id="catch"><linkEventDefinition name="toTask"/></intermediateCatchEvent>
    <sequenceFlow id="f2" sourceRef="catch" targetRef="task"/>
    <userTask id="task" name="Task" flowable:assignee="alice"/>
    <sequenceFlow id="f3" sourceRef="task" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestLinkEvents 验证链接事件跳转。
// @param t 测试上下文
func TestLinkEvents(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, linkEventXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "linkProcess"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 1 || tasks[0].TaskDefinitionKey != "task" {
		t.Fatalf("链接事件未跳转到任务: %+v", tasks)
	}
	if err := engine.Complete(context.Background(), tasks[0].ID, api.CompleteTaskRequest{UserID: "alice"}); err != nil {
		t.Fatalf("完成任务失败: %v", err)
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("流程应结束")
	}
}

// TestLinkCatchPassThrough 验证无抛出的捕获链接直接通过。
// @param t 测试上下文
func TestLinkCatchPassThrough(t *testing.T) {
	engine := newTestEngine(t)
	xml := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="linkPass" isExecutable="true"><startEvent id="s"/><sequenceFlow id="f1" sourceRef="s" targetRef="catch"/><intermediateCatchEvent id="catch"><linkEventDefinition name="x"/></intermediateCatchEvent><sequenceFlow id="f2" sourceRef="catch" targetRef="e"/><endEvent id="e"/></process></definitions>`
	deployXML(t, engine, xml)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "linkPass"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("捕获链接应直接通过并结束")
	}
}

// TestLinkEventMissingCatch 验证缺少捕获链接时报错。
// @param t 测试上下文
func TestLinkEventMissingCatch(t *testing.T) {
	engine := newTestEngine(t)
	xml := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="linkMissing" isExecutable="true"><startEvent id="s"/><sequenceFlow id="f1" sourceRef="s" targetRef="throw"/><intermediateThrowEvent id="throw"><linkEventDefinition name="missing"/></intermediateThrowEvent></process></definitions>`
	deployXML(t, engine, xml)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "linkMissing"}); err == nil {
		t.Fatalf("缺少捕获链接应报错")
	}
	if instances, _ := engine.ListProcessInstances(context.Background()); len(instances) != 0 {
		t.Fatalf("失败后应回滚，实际实例 %d", len(instances))
	}
}
