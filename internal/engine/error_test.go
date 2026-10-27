package engine

import (
	"context"
	"testing"

	"github.com/flowable-go/flowable/api"
)

const errorBoundaryXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <error id="err1" errorCode="E1"/>
  <process id="errorBoundary" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="sp"/>
    <subProcess id="sp">
      <startEvent id="ss"/>
      <sequenceFlow id="sf1" sourceRef="ss" targetRef="errorEnd"/>
      <endEvent id="errorEnd"><errorEventDefinition errorRef="err1"/></endEvent>
    </subProcess>
    <boundaryEvent id="handler" attachedToRef="sp">
      <errorEventDefinition errorRef="err1"/>
    </boundaryEvent>
    <sequenceFlow id="f2" sourceRef="sp" targetRef="normalEnd"/>
    <sequenceFlow id="f3" sourceRef="handler" targetRef="handlerTask"/>
    <userTask id="handlerTask" name="Handle Error"/>
    <sequenceFlow id="f4" sourceRef="handlerTask" targetRef="handlerEnd"/>
    <endEvent id="normalEnd"/>
    <endEvent id="handlerEnd"/>
  </process>
</definitions>`

// TestErrorBoundary_CatchesSubprocessError 验证错误边界捕获。
// @param t 测试上下文
func TestErrorBoundary_CatchesSubprocessError(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, errorBoundaryXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "errorBoundary"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 1 || tasks[0].TaskDefinitionKey != "handlerTask" {
		t.Fatalf("期望错误处理任务，实际 %+v", tasks)
	}
	if err := engine.Complete(context.Background(), tasks[0].ID, api.CompleteTaskRequest{}); err != nil {
		t.Fatalf("完成处理任务失败: %v", err)
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("错误处理后应结束")
	}
}

const errorMismatchXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <error id="err1" errorCode="E1"/>
  <error id="err2" errorCode="E2"/>
  <process id="errorMismatch" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="sp"/>
    <subProcess id="sp">
      <startEvent id="ss"/>
      <sequenceFlow id="sf1" sourceRef="ss" targetRef="errorEnd"/>
      <endEvent id="errorEnd"><errorEventDefinition errorRef="err1"/></endEvent>
    </subProcess>
    <boundaryEvent id="handler" attachedToRef="sp"><errorEventDefinition errorRef="err2"/></boundaryEvent>
    <sequenceFlow id="f2" sourceRef="sp" targetRef="normalEnd"/>
    <sequenceFlow id="f3" sourceRef="handler" targetRef="handlerEnd"/>
    <endEvent id="normalEnd"/>
    <endEvent id="handlerEnd"/>
  </process>
</definitions>`

// TestErrorBoundary_MismatchFailsInstance 验证未匹配错误使实例失败。
// @param t 测试上下文
func TestErrorBoundary_MismatchFailsInstance(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, errorMismatchXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "errorMismatch"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if len(listTasks(t, engine)) != 0 {
		t.Fatalf("未匹配错误不应产生任务")
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("未匹配错误应结束实例")
	}
	historic, _ := engine.ListHistoricProcessInstances(context.Background())
	if len(historic) != 1 || historic[0].DeleteReason == "" {
		t.Fatalf("历史应记录错误原因: %+v", historic)
	}
}

const topLevelErrorXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <error id="err" errorCode="TOP"/>
  <process id="topError" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="e"/>
    <endEvent id="e"><errorEventDefinition errorRef="err"/></endEvent>
  </process>
</definitions>`

// TestErrorEnd_TopLevel 验证顶层错误结束事件。
// @param t 测试上下文
func TestErrorEnd_TopLevel(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, topLevelErrorXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "topError"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if !instance.Ended {
		t.Fatalf("顶层错误应结束实例")
	}
	historic, _ := engine.ListHistoricProcessInstances(context.Background())
	if len(historic) != 1 || historic[0].DeleteReason != "BPMN error: TOP" {
		t.Fatalf("错误原因错误: %+v", historic)
	}
}
