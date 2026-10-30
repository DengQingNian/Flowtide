package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
)

const receiveTaskTriggerXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="receiveTrigger" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="recv"/>
    <receiveTask id="recv" name="Wait"/>
    <sequenceFlow id="f2" sourceRef="recv" targetRef="task"/>
    <userTask id="task" name="Task" flowable:assignee="${assignee}"/>
    <sequenceFlow id="f3" sourceRef="task" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

const catchTriggerXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="catchTrigger" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="wait"/>
    <intermediateCatchEvent id="wait" name="Wait"/>
    <sequenceFlow id="f2" sourceRef="wait" targetRef="task"/>
    <userTask id="task" name="Task"/>
    <sequenceFlow id="f3" sourceRef="task" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestTriggerReceiveTask 验证触发无消息接收任务。
// @param t 测试上下文
func TestTriggerReceiveTask(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, receiveTaskTriggerXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "receiveTrigger"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if len(listTasks(t, engine)) != 0 {
		t.Fatalf("触发前不应有任务")
	}
	executionID := waitingActivityExecutionID(t, engine, instance.ID)
	if err := engine.TriggerExecution(context.Background(), executionID, []api.Variable{{Name: "assignee", Value: "bob"}}); err != nil {
		t.Fatalf("触发失败: %v", err)
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 1 || tasks[0].Assignee != "bob" {
		t.Fatalf("触发后任务错误: %+v", tasks)
	}
	if err := engine.TriggerExecution(context.Background(), executionID, nil); !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("任务等待状态触发应 400: %v", err)
	}
}

// TestTriggerIntermediateCatch 验证触发无定义中间捕获事件。
// @param t 测试上下文
func TestTriggerIntermediateCatch(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, catchTriggerXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "catchTrigger"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if err := engine.TriggerExecution(context.Background(), waitingActivityExecutionID(t, engine, instance.ID), nil); err != nil {
		t.Fatalf("触发失败: %v", err)
	}
	if tasks := listTasks(t, engine); len(tasks) != 1 {
		t.Fatalf("触发后任务错误: %+v", tasks)
	}
}

// TestTriggerErrors 验证触发错误。
// @param t 测试上下文
func TestTriggerErrors(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, changeStateXML)
	instance, _ := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "changeState"})
	executionID := waitingActivityExecutionID(t, engine, instance.ID)
	if err := engine.TriggerExecution(context.Background(), "missing", nil); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失执行应 404: %v", err)
	}
	if err := engine.TriggerExecution(context.Background(), executionID, nil); !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("用户任务触发应 400: %v", err)
	}
	deployXML(t, engine, messageCatchXML)
	messageInstance, _ := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "msgCatch"})
	messageExecutionID := waitingActivityExecutionID(t, engine, messageInstance.ID)
	if err := engine.TriggerExecution(context.Background(), messageExecutionID, nil); !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("消息捕获触发应 400: %v", err)
	}
	if err := engine.SuspendProcessInstance(context.Background(), messageInstance.ID); err != nil {
		t.Fatalf("挂起失败: %v", err)
	}
	if err := engine.TriggerExecution(context.Background(), messageExecutionID, nil); !errors.Is(err, api.ErrConflict) {
		t.Fatalf("挂起实例触发应 409: %v", err)
	}
}
