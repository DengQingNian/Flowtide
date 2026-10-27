package engine

import (
	"context"
	"testing"

	"github.com/flowable-go/flowable/api"
)

const interruptingEventSubXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <message id="em" name="escalate"/>
  <process id="eventSub" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="mainTask"/>
    <userTask id="mainTask" name="Main"/>
    <sequenceFlow id="f2" sourceRef="mainTask" targetRef="mainEnd"/>
    <endEvent id="mainEnd"/>
    <subProcess id="evt" triggeredByEvent="true">
      <startEvent id="es"><messageEventDefinition messageRef="em"/></startEvent>
      <sequenceFlow id="ef1" sourceRef="es" targetRef="evtTask"/>
      <userTask id="evtTask" name="Event Task"/>
      <sequenceFlow id="ef2" sourceRef="evtTask" targetRef="evtEnd"/>
      <endEvent id="evtEnd"/>
    </subProcess>
  </process>
</definitions>`

// TestEventSubProcess_Interrupting 验证中断型事件子流程。
// @param t 测试上下文
func TestEventSubProcess_Interrupting(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, interruptingEventSubXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "eventSub"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if len(listTasks(t, engine)) != 1 {
		t.Fatalf("期望主任务")
	}
	triggered, err := engine.MessageEventReceived(context.Background(), "escalate", nil)
	if err != nil || triggered != 1 {
		t.Fatalf("事件子流程触发失败: %v %d", err, triggered)
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 1 || tasks[0].TaskDefinitionKey != "evtTask" {
		t.Fatalf("中断后应只有事件任务: %+v", tasks)
	}
	if err := engine.Complete(context.Background(), tasks[0].ID, api.CompleteTaskRequest{}); err != nil {
		t.Fatalf("完成事件任务失败: %v", err)
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("中断型事件子流程完成后实例应结束")
	}
}

const nonInterruptingEventSubXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <signal id="esig" name="remind"/>
  <process id="eventSubNon" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="mainTask"/>
    <userTask id="mainTask" name="Main"/>
    <sequenceFlow id="f2" sourceRef="mainTask" targetRef="mainEnd"/>
    <endEvent id="mainEnd"/>
    <subProcess id="evt" triggeredByEvent="true">
      <startEvent id="es" isInterrupting="false"><signalEventDefinition signalRef="esig"/></startEvent>
      <sequenceFlow id="ef1" sourceRef="es" targetRef="evtTask"/>
      <userTask id="evtTask" name="Event Task"/>
      <sequenceFlow id="ef2" sourceRef="evtTask" targetRef="evtEnd"/>
      <endEvent id="evtEnd"/>
    </subProcess>
  </process>
</definitions>`

// TestEventSubProcess_NonInterrupting 验证非中断型事件子流程。
// @param t 测试上下文
func TestEventSubProcess_NonInterrupting(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, nonInterruptingEventSubXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "eventSubNon"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if _, err := engine.SignalEventReceived(context.Background(), "remind", nil); err != nil {
		t.Fatalf("信号触发失败: %v", err)
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 2 {
		t.Fatalf("期望主任务与事件任务，实际 %d", len(tasks))
	}
	// 完成事件任务，主任务应保留
	for _, task := range tasks {
		if task.TaskDefinitionKey == "evtTask" {
			if err := engine.Complete(context.Background(), task.ID, api.CompleteTaskRequest{}); err != nil {
				t.Fatalf("完成事件任务失败: %v", err)
			}
		}
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if got.Ended {
		t.Fatalf("主任务未完成，实例不应结束")
	}
	tasks = listTasks(t, engine)
	if len(tasks) != 1 || tasks[0].TaskDefinitionKey != "mainTask" {
		t.Fatalf("主任务应保留: %+v", tasks)
	}
}
