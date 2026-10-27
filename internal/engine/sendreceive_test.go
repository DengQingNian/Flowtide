package engine

import (
	"context"
	"testing"

	"github.com/flowable-go/flowable/api"
)

const receiveTaskXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <message id="rm" name="receiveMessage"/>
  <process id="receiveTask" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="wait"/>
    <receiveTask id="wait" messageRef="rm"/>
    <sequenceFlow id="f2" sourceRef="wait" targetRef="task"/>
    <userTask id="task" name="Task"/>
    <sequenceFlow id="f3" sourceRef="task" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestReceiveTask_Message 验证接收任务等待消息。
// @param t 测试上下文
func TestReceiveTask_Message(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, receiveTaskXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "receiveTask"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if len(listTasks(t, engine)) != 0 {
		t.Fatalf("接收任务等待时不应有用户任务")
	}
	triggered, err := engine.MessageEventReceived(context.Background(), "receiveMessage", nil)
	if err != nil || triggered != 1 {
		t.Fatalf("接收任务消息触发失败: %v %d", err, triggered)
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 1 {
		t.Fatalf("接收任务后应创建任务")
	}
	if err := engine.Complete(context.Background(), tasks[0].ID, api.CompleteTaskRequest{}); err != nil {
		t.Fatalf("完成任务失败: %v", err)
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("接收任务流程应结束")
	}
}

const sendSignalCatcherXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <signal id="sig" name="sendSignal"/>
  <process id="catcher" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="wait"/>
    <intermediateCatchEvent id="wait"><signalEventDefinition signalRef="sig"/></intermediateCatchEvent>
    <sequenceFlow id="f2" sourceRef="wait" targetRef="task"/>
    <userTask id="task" name="Task"/>
    <sequenceFlow id="f3" sourceRef="task" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

const sendSignalSenderXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <signal id="sig" name="sendSignal"/>
  <process id="sender" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="send"/>
    <sendTask id="send" signalRef="sig"/>
    <sequenceFlow id="f2" sourceRef="send" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestSendTask_Signal 验证发送任务广播信号。
// @param t 测试上下文
func TestSendTask_Signal(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, sendSignalCatcherXML)
	deployXML(t, engine, sendSignalSenderXML)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "catcher"}); err != nil {
		t.Fatalf("启动捕获流程失败: %v", err)
	}
	sender, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "sender"})
	if err != nil {
		t.Fatalf("启动发送流程失败: %v", err)
	}
	if !sender.Ended {
		t.Fatalf("发送任务后发送流程应结束")
	}
	if len(listTasks(t, engine)) != 1 {
		t.Fatalf("发送信号后捕获流程应创建任务")
	}
}

// TestSendReceive_Errors 验证发送接收任务错误。
// @param t 测试上下文
func TestSendReceive_Errors(t *testing.T) {
	cases := []struct {
		key string
		xml string
	}{
		{"badReceive", `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="badReceive" isExecutable="true"><startEvent id="s"/><sequenceFlow id="f1" sourceRef="s" targetRef="r"/><receiveTask id="r"/><sequenceFlow id="f2" sourceRef="r" targetRef="e"/><endEvent id="e"/></process></definitions>`},
		{"badSend", `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="badSend" isExecutable="true"><startEvent id="s"/><sequenceFlow id="f1" sourceRef="s" targetRef="send"/><sendTask id="send"/><sequenceFlow id="f2" sourceRef="send" targetRef="e"/><endEvent id="e"/></process></definitions>`},
	}
	for _, tc := range cases {
		engine := newTestEngine(t)
		deployXML(t, engine, tc.xml)
		if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: tc.key}); err == nil {
			t.Fatalf("流程 %s 缺少事件定义应报错", tc.key)
		}
	}
}
