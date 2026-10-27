package engine

import (
	"context"
	"testing"

	"github.com/flowable-go/flowable/api"
)

const eventGatewayXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn">
  <message id="m1" name="first"/>
  <message id="m2" name="second"/>
  <process id="eventGateway" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="gw"/>
    <eventBasedGateway id="gw"/>
    <sequenceFlow id="fa" sourceRef="gw" targetRef="catchA"/>
    <sequenceFlow id="fb" sourceRef="gw" targetRef="catchB"/>
    <intermediateCatchEvent id="catchA"><messageEventDefinition messageRef="m1"/></intermediateCatchEvent>
    <intermediateCatchEvent id="catchB"><messageEventDefinition messageRef="m2"/></intermediateCatchEvent>
    <sequenceFlow id="f2" sourceRef="catchA" targetRef="taskA"/>
    <sequenceFlow id="f3" sourceRef="catchB" targetRef="taskB"/>
    <userTask id="taskA" name="A" flowable:assignee="${assignee}"/>
    <userTask id="taskB" name="B"/>
    <sequenceFlow id="f4" sourceRef="taskA" targetRef="endA"/>
    <sequenceFlow id="f5" sourceRef="taskB" targetRef="endB"/>
    <endEvent id="endA"/>
    <endEvent id="endB"/>
  </process>
</definitions>`

// TestEventGateway_FirstEventWins 验证事件网关竞争。
// @param t 测试上下文
func TestEventGateway_FirstEventWins(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, eventGatewayXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "eventGateway"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if len(listTasks(t, engine)) != 0 {
		t.Fatalf("事件网关等待时不应有任务")
	}
	triggered, err := engine.MessageEventReceived(context.Background(), "first", []api.Variable{{Name: "assignee", Value: "alice"}})
	if err != nil || triggered != 1 {
		t.Fatalf("第一事件触发失败: %v %d", err, triggered)
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 1 || tasks[0].TaskDefinitionKey != "taskA" || tasks[0].Assignee != "alice" {
		t.Fatalf("胜出分支任务错误: %+v", tasks)
	}
	if triggered, _ := engine.MessageEventReceived(context.Background(), "second", nil); triggered != 0 {
		t.Fatalf("竞争分支应被取消")
	}
	if err := engine.Complete(context.Background(), tasks[0].ID, api.CompleteTaskRequest{}); err != nil {
		t.Fatalf("完成任务失败: %v", err)
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("事件网关分支完成后应结束")
	}
}

// TestEventGateway_NoCatch 验证无捕获事件报错。
// @param t 测试上下文
func TestEventGateway_NoCatch(t *testing.T) {
	engine := newTestEngine(t)
	xml := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="badGateway" isExecutable="true"><startEvent id="s"/><sequenceFlow id="f1" sourceRef="s" targetRef="gw"/><eventBasedGateway id="gw"/><sequenceFlow id="f2" sourceRef="gw" targetRef="task"/><userTask id="task"/></process></definitions>`
	deployXML(t, engine, xml)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "badGateway"}); err == nil {
		t.Fatalf("无捕获事件应报错")
	}
}
