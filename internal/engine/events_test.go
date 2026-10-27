package engine

import (
	"context"
	"testing"

	"github.com/flowable-go/flowable/api"
)

const messageStartXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn">
  <message id="m1" name="orderCreated"/>
  <process id="msgStart" isExecutable="true">
    <startEvent id="s"><messageEventDefinition messageRef="m1"/></startEvent>
    <sequenceFlow id="f1" sourceRef="s" targetRef="task"/>
    <userTask id="task" name="Task" flowable:assignee="${assignee}"/>
    <sequenceFlow id="f2" sourceRef="task" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestMessageStartEvent 验证消息启动事件。
// @param t 测试上下文
func TestMessageStartEvent(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, messageStartXML)
	if len(listTasks(t, engine)) != 0 {
		t.Fatalf("消息启动流程不应自动创建任务")
	}
	triggered, err := engine.MessageEventReceived(context.Background(), "orderCreated", []api.Variable{{Name: "assignee", Value: "alice"}})
	if err != nil || triggered != 1 {
		t.Fatalf("消息投递失败: %v %d", err, triggered)
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 1 || tasks[0].Assignee != "alice" {
		t.Fatalf("消息启动任务错误: %+v", tasks)
	}
	triggered, err = engine.MessageEventReceived(context.Background(), "orderCreated", nil)
	if err != nil || triggered != 1 {
		t.Fatalf("重复消息投递失败: %v %d", err, triggered)
	}
	if len(listTasks(t, engine)) != 2 {
		t.Fatalf("消息应可多次启动")
	}
	if triggered, _ := engine.MessageEventReceived(context.Background(), "unknown", nil); triggered != 0 {
		t.Fatalf("未知消息不应触发")
	}
}

const signalStartXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn">
  <signal id="sig1" name="refresh"/>
  <process id="sigStart" isExecutable="true">
    <startEvent id="s"><signalEventDefinition signalRef="sig1"/></startEvent>
    <sequenceFlow id="f1" sourceRef="s" targetRef="task"/>
    <userTask id="task" name="Task"/>
    <sequenceFlow id="f2" sourceRef="task" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestSignalStartEvent 验证信号启动事件。
// @param t 测试上下文
func TestSignalStartEvent(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, signalStartXML)
	triggered, err := engine.SignalEventReceived(context.Background(), "refresh", nil)
	if err != nil || triggered != 1 {
		t.Fatalf("信号投递失败: %v %d", err, triggered)
	}
	if len(listTasks(t, engine)) != 1 {
		t.Fatalf("信号应启动实例")
	}
	if triggered, _ := engine.SignalEventReceived(context.Background(), "unknown", nil); triggered != 0 {
		t.Fatalf("未知信号不应触发")
	}
}

const messageCatchXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn">
  <message id="m2" name="approvedMessage"/>
  <process id="msgCatch" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="wait"/>
    <intermediateCatchEvent id="wait"><messageEventDefinition messageRef="m2"/></intermediateCatchEvent>
    <sequenceFlow id="f2" sourceRef="wait" targetRef="task"/>
    <userTask id="task" name="Task" flowable:assignee="${assignee}"/>
    <sequenceFlow id="f3" sourceRef="task" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestMessageCatchEvent 验证中间消息捕获。
// @param t 测试上下文
func TestMessageCatchEvent(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, messageCatchXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "msgCatch"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if len(listTasks(t, engine)) != 0 {
		t.Fatalf("消息捕获前不应有任务")
	}
	triggered, err := engine.MessageEventReceived(context.Background(), "approvedMessage", []api.Variable{{Name: "assignee", Value: "bob"}})
	if err != nil || triggered != 1 {
		t.Fatalf("消息触发失败: %v %d", err, triggered)
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 1 || tasks[0].Assignee != "bob" {
		t.Fatalf("消息后任务错误: %+v", tasks)
	}
	if err := engine.Complete(context.Background(), tasks[0].ID, api.CompleteTaskRequest{}); err != nil {
		t.Fatalf("完成任务失败: %v", err)
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("消息流程应结束")
	}
}

const signalCatchXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <signal id="sig2" name="broadcast"/>
  <process id="signalCatch" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="wait"/>
    <intermediateCatchEvent id="wait"><signalEventDefinition signalRef="sig2"/></intermediateCatchEvent>
    <sequenceFlow id="f2" sourceRef="wait" targetRef="task"/>
    <userTask id="task" name="Task"/>
    <sequenceFlow id="f3" sourceRef="task" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestSignalCatchEvent_Broadcast 验证信号广播。
// @param t 测试上下文
func TestSignalCatchEvent_Broadcast(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, signalCatchXML)
	for i := 0; i < 2; i++ {
		if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "signalCatch"}); err != nil {
			t.Fatalf("启动失败: %v", err)
		}
	}
	triggered, err := engine.SignalEventReceived(context.Background(), "broadcast", nil)
	if err != nil || triggered != 2 {
		t.Fatalf("信号广播失败: %v %d", err, triggered)
	}
	if len(listTasks(t, engine)) != 2 {
		t.Fatalf("信号应触发两个任务")
	}
}

// TestEvents_Persistence 验证事件订阅持久化。
// @param t 测试上下文
func TestEvents_Persistence(t *testing.T) {
	path := t.TempDir() + "/state.json"
	first, _ := New(Config{StatePath: path})
	deployXML(t, first, messageCatchXML)
	if _, err := first.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "msgCatch"}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if err := first.Close(context.Background()); err != nil {
		t.Fatalf("关闭失败: %v", err)
	}
	second, err := New(Config{StatePath: path})
	if err != nil {
		t.Fatalf("恢复失败: %v", err)
	}
	defer func() { _ = second.Close(context.Background()) }()
	triggered, err := second.MessageEventReceived(context.Background(), "approvedMessage", []api.Variable{{Name: "assignee", Value: "carol"}})
	if err != nil || triggered != 1 {
		t.Fatalf("恢复后消息触发失败: %v %d", err, triggered)
	}
	if len(listTasks(t, second)) != 1 {
		t.Fatalf("恢复后应创建任务")
	}
}
