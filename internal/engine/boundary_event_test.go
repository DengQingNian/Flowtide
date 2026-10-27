package engine

import (
	"context"
	"testing"

	"github.com/flowable-go/flowable/api"
)

const boundaryMessageXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <message id="bm" name="cancelOrder"/>
  <process id="boundaryMessage" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="task"/>
    <userTask id="task" name="Wait"/>
    <boundaryEvent id="cancel" attachedToRef="task">
      <messageEventDefinition messageRef="bm"/>
    </boundaryEvent>
    <sequenceFlow id="f2" sourceRef="task" targetRef="normalEnd"/>
    <sequenceFlow id="f3" sourceRef="cancel" targetRef="cancelEnd"/>
    <endEvent id="normalEnd"/>
    <endEvent id="cancelEnd"/>
  </process>
</definitions>`

// TestBoundaryMessage_Interrupting 验证中断边界消息。
// @param t 测试上下文
func TestBoundaryMessage_Interrupting(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, boundaryMessageXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "boundaryMessage"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	triggered, err := engine.MessageEventReceived(context.Background(), "cancelOrder", nil)
	if err != nil || triggered != 1 {
		t.Fatalf("边界消息触发失败: %v %d", err, triggered)
	}
	if len(listTasks(t, engine)) != 0 {
		t.Fatalf("中断边界消息应删除任务")
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("边界消息路径应结束实例")
	}
}

const boundarySignalXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <signal id="bs" name="notify"/>
  <process id="boundarySignal" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="task"/>
    <userTask id="task" name="Wait"/>
    <boundaryEvent id="notify" attachedToRef="task" cancelActivity="false">
      <signalEventDefinition signalRef="bs"/>
    </boundaryEvent>
    <sequenceFlow id="f2" sourceRef="task" targetRef="normalEnd"/>
    <sequenceFlow id="f3" sourceRef="notify" targetRef="notifyEnd"/>
    <endEvent id="normalEnd"/>
    <endEvent id="notifyEnd"/>
  </process>
</definitions>`

// TestBoundarySignal_NonInterrupting 验证非中断边界信号。
// @param t 测试上下文
func TestBoundarySignal_NonInterrupting(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, boundarySignalXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "boundarySignal"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	triggered, err := engine.SignalEventReceived(context.Background(), "notify", nil)
	if err != nil || triggered != 1 {
		t.Fatalf("边界信号触发失败: %v %d", err, triggered)
	}
	if len(listTasks(t, engine)) != 1 {
		t.Fatalf("非中断边界信号应保留任务")
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if got.Ended {
		t.Fatalf("原任务未完成，实例不应结束")
	}
}

// TestBoundaryMessage_SubscriptionCleanup 验证任务完成后订阅清理。
// @param t 测试上下文
func TestBoundaryMessage_SubscriptionCleanup(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, boundaryMessageXML)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "boundaryMessage"}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	task := listTasks(t, engine)[0]
	if err := engine.Complete(context.Background(), task.ID, api.CompleteTaskRequest{}); err != nil {
		t.Fatalf("完成任务失败: %v", err)
	}
	if triggered, _ := engine.MessageEventReceived(context.Background(), "cancelOrder", nil); triggered != 0 {
		t.Fatalf("任务完成后订阅应清理")
	}
}
