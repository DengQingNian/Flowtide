package engine

import (
	"context"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
)

const transactionXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="transactionProcess" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="tx"/>
    <transaction id="tx">
      <startEvent id="ts"/>
      <sequenceFlow id="tf1" sourceRef="ts" targetRef="cancelEnd"/>
      <endEvent id="cancelEnd"><cancelEventDefinition/></endEvent>
    </transaction>
    <boundaryEvent id="cancelBoundary" attachedToRef="tx"><cancelEventDefinition/></boundaryEvent>
    <sequenceFlow id="f2" sourceRef="tx" targetRef="normalEnd"/>
    <sequenceFlow id="f3" sourceRef="cancelBoundary" targetRef="handlerTask"/>
    <userTask id="handlerTask" name="Handle Cancel"/>
    <sequenceFlow id="f4" sourceRef="handlerTask" targetRef="handlerEnd"/>
    <endEvent id="normalEnd"/>
    <endEvent id="handlerEnd"/>
  </process>
</definitions>`

// TestTransaction_CancelBoundary 验证事务取消边界。
// @param t 测试上下文
func TestTransaction_CancelBoundary(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, transactionXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "transactionProcess"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 1 || tasks[0].TaskDefinitionKey != "handlerTask" {
		t.Fatalf("期望取消处理任务，实际 %+v", tasks)
	}
	if err := engine.Complete(context.Background(), tasks[0].ID, api.CompleteTaskRequest{}); err != nil {
		t.Fatalf("完成处理任务失败: %v", err)
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("事务取消后应结束")
	}
}

// TestTransaction_NoBoundary 验证无取消边界。
// @param t 测试上下文
func TestTransaction_NoBoundary(t *testing.T) {
	engine := newTestEngine(t)
	xml := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="txNoBoundary" isExecutable="true"><startEvent id="s"/><sequenceFlow id="f1" sourceRef="s" targetRef="tx"/><transaction id="tx"><startEvent id="ts"/><sequenceFlow id="tf" sourceRef="ts" targetRef="cancelEnd"/><endEvent id="cancelEnd"><cancelEventDefinition/></endEvent></transaction><sequenceFlow id="f2" sourceRef="tx" targetRef="e"/><endEvent id="e"/></process></definitions>`
	deployXML(t, engine, xml)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "txNoBoundary"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if !instance.Ended {
		t.Fatalf("无取消边界应结束实例")
	}
}
